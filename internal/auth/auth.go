// Package auth 提供口令登录、会话管理与登录失败限流。
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"sevrtree/internal/store"
)

// 认证相关错误。
var (
	ErrInvalidCredentials = errors.New("口令错误")
	ErrRateLimited        = errors.New("尝试过于频繁，请稍后再试")
	ErrUnauthorized       = errors.New("未登录或会话已过期")
)

const (
	settingPasswordHash = "admin.password_hash"
	settingUsername     = "admin.username"

	maxFailures   = 5
	failureWindow = 5 * time.Minute
	lockDuration  = 5 * time.Minute
)

type failureState struct {
	count    int
	firstAt  time.Time
	lockedTo time.Time
}

// Service 维护内存态会话，口令散列持久化在 SQLite。
type Service struct {
	store    *store.Store
	ttl      time.Duration
	username string
	logger   *slog.Logger

	mu       sync.Mutex
	sessions map[string]time.Time
	failures map[string]*failureState
}

// NewService 初始化认证服务；若首次运行则生成管理员口令。
// 返回的 initialPassword 仅在首次生成时非空，用于在控制台提示用户。
func NewService(st *store.Store, ttl time.Duration, logger *slog.Logger) (*Service, string, error) {
	s := &Service{
		store:    st,
		ttl:      ttl,
		logger:   logger,
		sessions: make(map[string]time.Time),
		failures: make(map[string]*failureState),
	}

	if name, ok, err := st.GetSetting(settingUsername); err != nil {
		return nil, "", err
	} else if ok && name != "" {
		s.username = name
	} else {
		s.username = "admin"
		_ = st.SetSetting(settingUsername, s.username)
	}

	_, exists, err := st.GetSetting(settingPasswordHash)
	if err != nil {
		return nil, "", err
	}
	if exists {
		return s, "", nil
	}

	password := strings.TrimSpace(envPassword())
	generated := password == ""
	if generated {
		password = randomPassword(14)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return nil, "", err
	}
	if err := st.SetSetting(settingPasswordHash, string(hash)); err != nil {
		return nil, "", err
	}
	if generated {
		return s, password, nil
	}
	return s, "", nil
}

// Username 返回当前管理员账号。
func (s *Service) Username() string { return s.username }

// Login 校验口令并签发会话令牌。
func (s *Service) Login(ip, password string) (string, error) {
	if err := s.checkLimit(ip); err != nil {
		return "", err
	}

	hash, ok, err := s.store.GetSetting(settingPasswordHash)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		s.recordFailure(ip)
		s.logger.Warn("登录失败", "ip", ip)
		return "", ErrInvalidCredentials
	}

	s.clearFailure(ip)
	token := randomToken()
	s.mu.Lock()
	s.sessions[token] = time.Now().Add(s.ttl)
	s.mu.Unlock()
	s.logger.Info("登录成功", "user", s.username, "ip", ip)
	return token, nil
}

// Validate 校验令牌，并在有效期内自动续期。
func (s *Service) Validate(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[token]
	if !ok {
		return false
	}
	now := time.Now()
	if now.After(exp) {
		delete(s.sessions, token)
		return false
	}
	s.sessions[token] = now.Add(s.ttl)
	return true
}

// Logout 注销会话。
func (s *Service) Logout(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// ChangePassword 校验旧口令后更新为新口令，并清理全部会话。
func (s *Service) ChangePassword(oldPassword, newPassword string) error {
	if len(strings.TrimSpace(newPassword)) < 8 {
		return errors.New("新口令至少 8 位")
	}
	hash, ok, err := s.store.GetSetting(settingPasswordHash)
	if err != nil {
		return err
	}
	if !ok || bcrypt.CompareHashAndPassword([]byte(hash), []byte(oldPassword)) != nil {
		return ErrInvalidCredentials
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return err
	}
	if err := s.store.SetSetting(settingPasswordHash, string(newHash)); err != nil {
		return err
	}
	s.mu.Lock()
	s.sessions = make(map[string]time.Time)
	s.mu.Unlock()
	return nil
}

func (s *Service) checkLimit(ip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.failures[ip]
	if !ok {
		return nil
	}
	now := time.Now()
	if now.Before(st.lockedTo) {
		return ErrRateLimited
	}
	if now.Sub(st.firstAt) > failureWindow {
		delete(s.failures, ip)
	}
	return nil
}

func (s *Service) recordFailure(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	st, ok := s.failures[ip]
	if !ok || now.Sub(st.firstAt) > failureWindow {
		st = &failureState{firstAt: now}
		s.failures[ip] = st
	}
	st.count++
	if st.count >= maxFailures {
		st.lockedTo = now.Add(lockDuration)
		st.count = 0
		st.firstAt = now
	}
}

func (s *Service) clearFailure(ip string) {
	s.mu.Lock()
	delete(s.failures, ip)
	s.mu.Unlock()
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}

func randomPassword(n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			b[i] = alphabet[i%len(alphabet)]
			continue
		}
		b[i] = alphabet[idx.Int64()]
	}
	return string(b)
}

func envPassword() string {
	return strings.TrimSpace(os.Getenv("SEVRTREE_ADMIN_PASSWORD"))
}
