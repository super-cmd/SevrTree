package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"sevrtree/internal/platform"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": s.cfg.Version,
		"time":    time.Now(),
	})
}

func (s *Server) handleSystem(w http.ResponseWriter, _ *http.Request) {
	info := platform.System()
	installed, state := platform.ServiceStatus()

	writeJSON(w, http.StatusOK, map[string]any{
		"system": info,
		"app": map[string]any{
			"version":       s.cfg.Version,
			"addr":          s.cfg.Addr,
			"dataDir":       s.cfg.DataDir,
			"logDir":        s.cfg.LogDir,
			"startedAt":     s.startedAt,
			"uptimeSeconds": int64(time.Since(s.startedAt).Seconds()),
		},
		"autostart": map[string]any{
			"serviceName": platform.ServiceName,
			"installed":   installed,
			"state":       state,
		},
		"sequence": s.manager.Sequence(),
	})
}

func (s *Server) handleAutostart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}

	exe, err := os.Executable()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法定位当前程序路径")
		return
	}
	exeDir := filepath.Dir(exe)

	switch req.Action {
	case "install":
		err = platform.InstallAutostart(exe, exeDir, s.cfg.LogDir)
	case "uninstall":
		err = platform.UninstallAutostart(exe)
	case "start":
		err = platform.StartService(exe)
	case "stop":
		err = platform.StopService(exe)
	default:
		writeError(w, http.StatusBadRequest, "不支持的操作: "+req.Action)
		return
	}

	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	installed, state := platform.ServiceStatus()
	_ = s.store.AddAudit(s.auth.Username(), "开机自启操作", req.Action, clientIP(r), "state="+state)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"installed": installed,
		"state":     state,
	})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	logs, err := s.store.ListAudit(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取审计日志失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}
