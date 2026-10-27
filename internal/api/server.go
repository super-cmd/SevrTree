// Package api 提供 HTTP 接口与内嵌前端静态资源托管。
package api

import (
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"sevrtree/internal/auth"
	"sevrtree/internal/config"
	"sevrtree/internal/manager"
	"sevrtree/internal/procmon"
	"sevrtree/internal/store"
)

const sessionCookie = "sevrtree_session"

// Server 聚合各依赖并暴露 HTTP 处理器。
type Server struct {
	cfg       *config.Config
	store     *store.Store
	auth      *auth.Service
	manager   *manager.Manager
	collector *procmon.Collector
	logger    *slog.Logger
	staticFS  fs.FS
	startedAt time.Time
}

// New 创建 API 服务。
func New(
	cfg *config.Config,
	st *store.Store,
	authSvc *auth.Service,
	mgr *manager.Manager,
	col *procmon.Collector,
	logger *slog.Logger,
	staticFS fs.FS,
) *Server {
	return &Server{
		cfg:       cfg,
		store:     st,
		auth:      authSvc,
		manager:   mgr,
		collector: col,
		logger:    logger,
		staticFS:  staticFS,
		startedAt: time.Now(),
	}
}

// Handler 返回挂载了全部中间件的路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/auth/me", s.handleMe)
	mux.HandleFunc("POST /api/auth/password", s.handleChangePassword)

	mux.HandleFunc("GET /api/processes", s.handleProcesses)
	mux.HandleFunc("GET /api/processes/{pid}", s.handleProcessDetail)
	mux.HandleFunc("POST /api/processes/{pid}/task", s.handleImportProcess)

	mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	mux.HandleFunc("POST /api/tasks", s.handleCreateTask)
	mux.HandleFunc("PUT /api/tasks/{id}", s.handleUpdateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.handleDeleteTask)
	mux.HandleFunc("POST /api/tasks/{id}/start", s.handleStartTask)
	mux.HandleFunc("POST /api/tasks/{id}/stop", s.handleStopTask)
	mux.HandleFunc("POST /api/tasks/{id}/restart", s.handleRestartTask)
	mux.HandleFunc("GET /api/tasks/{id}/logs", s.handleTaskLogs)
	mux.HandleFunc("POST /api/tasks/reorder", s.handleReorder)
	mux.HandleFunc("POST /api/tasks/start-all", s.handleStartAll)
	mux.HandleFunc("POST /api/tasks/stop-all", s.handleStopAll)

	mux.HandleFunc("GET /api/system", s.handleSystem)
	mux.HandleFunc("POST /api/system/autostart", s.handleAutostart)
	mux.HandleFunc("GET /api/audit", s.handleAudit)

	mux.Handle("/", s.staticHandler())

	return s.recoverMiddleware(s.accessLogMiddleware(s.authMiddleware(mux)))
}

func (s *Server) staticHandler() http.Handler {
	sub, err := fs.Sub(s.staticFS, "dist")
	if err != nil {
		s.logger.Error("内嵌静态资源不可用", "error", err)
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "前端资源缺失，请先执行 pnpm build", http.StatusInternalServerError)
		})
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "接口不存在")
			return
		}
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean == "" {
			clean = "index.html"
		}
		if _, statErr := fs.Stat(sub, clean); statErr != nil {
			// 前端使用 history 路由，未命中的路径回退到 index.html。
			r.URL.Path = "/index.html"
		} else if strings.HasPrefix(clean, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	public := map[string]bool{
		"/api/health":     true,
		"/api/auth/login": true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || public[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || !s.auth.Validate(cookie.Value) {
			writeError(w, http.StatusUnauthorized, "未登录或会话已过期")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			return
		}
		args := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"durationMs", time.Since(start).Milliseconds(),
			"ip", clientIP(r),
			"user", s.auth.Username(),
		}
		// 前端轮询类只读接口记为调试日志，避免刷屏。
		if r.Method == http.MethodGet && isPollingPath(r.URL.Path) {
			s.logger.Debug("轮询请求", args...)
			return
		}
		s.logger.Info("请求处理完成", args...)
	})
}

func isPollingPath(path string) bool {
	switch path {
	case "/api/processes", "/api/tasks", "/api/system", "/api/audit":
		return true
	}
	return strings.HasPrefix(path, "/api/processes/")
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("请求处理异常", "path", r.URL.Path, "error", rec)
				writeError(w, http.StatusInternalServerError, "服务内部异常")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(r *http.Request, dst any) error {
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := decoder.Decode(dst); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return nil
}
