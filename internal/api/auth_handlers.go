package api

import (
	"net/http"
	"time"

	"sevrtree/internal/auth"
)

type loginRequest struct {
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}

	ip := clientIP(r)
	token, err := s.auth.Login(ip, req.Password)
	if err != nil {
		switch err {
		case auth.ErrRateLimited:
			writeError(w, http.StatusTooManyRequests, err.Error())
		default:
			writeError(w, http.StatusUnauthorized, "口令错误")
		}
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.cfg.SessionTTL / time.Second),
	})
	_ = s.store.AddAudit(s.auth.Username(), "登录", "", ip, "口令登录成功")
	writeJSON(w, http.StatusOK, map[string]any{
		"username": s.auth.Username(),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		s.auth.Logout(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	_ = s.store.AddAudit(s.auth.Username(), "退出登录", "", clientIP(r), "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"username":      s.auth.Username(),
		"authenticated": true,
		"version":       s.cfg.Version,
	})
}

type changePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := s.auth.ChangePassword(req.OldPassword, req.NewPassword); err != nil {
		if err == auth.ErrInvalidCredentials {
			writeError(w, http.StatusBadRequest, "原口令错误")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.store.AddAudit(s.auth.Username(), "修改口令", "", clientIP(r), "口令已更新，全部会话失效")
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
