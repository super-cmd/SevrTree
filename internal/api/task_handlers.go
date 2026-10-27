package api

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"sevrtree/internal/manager"
	"sevrtree/internal/procmon"
	"sevrtree/internal/store"
)

func (s *Server) handleProcesses(w http.ResponseWriter, _ *http.Request) {
	snapshot, err := s.collector.Snapshot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "采集进程信息失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleProcessDetail(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.Atoi(r.PathValue("pid"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "PID 不合法")
		return
	}
	snapshot, err := s.collector.Snapshot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "采集进程信息失败: "+err.Error())
		return
	}
	for _, p := range snapshot.Processes {
		if int(p.PID) == pid {
			writeJSON(w, http.StatusOK, p)
			return
		}
	}
	writeError(w, http.StatusNotFound, "进程不存在或已退出")
}

type taskView struct {
	*store.Task
	Runtime *manager.Runtime `json:"runtime,omitempty"`
}

func (s *Server) handleListTasks(w http.ResponseWriter, _ *http.Request) {
	tasks, err := s.store.ListTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取任务失败: "+err.Error())
		return
	}
	runtimes := make(map[int64]manager.Runtime)
	for _, rt := range s.manager.Snapshot() {
		runtimes[rt.TaskID] = rt
	}

	views := make([]taskView, 0, len(tasks))
	for _, t := range tasks {
		view := taskView{Task: t}
		if rt, ok := runtimes[t.ID]; ok {
			rtCopy := rt
			view.Runtime = &rtCopy
		}
		views = append(views, view)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tasks":    views,
		"sequence": s.manager.Sequence(),
	})
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var task store.Task
	if err := decodeJSON(r, &task); err != nil {
		writeError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	task.ID = 0
	if err := task.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := s.store.CreateTask(&task)
	if err != nil {
		writeError(w, http.StatusBadRequest, "创建任务失败: "+err.Error())
		return
	}
	_ = s.store.AddAudit(s.auth.Username(), "创建任务", task.Name, clientIP(r), task.Exe)
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "任务 ID 不合法")
		return
	}
	var task store.Task
	if err := decodeJSON(r, &task); err != nil {
		writeError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	task.ID = id
	if err := task.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.UpdateTask(&task); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "任务不存在")
			return
		}
		writeError(w, http.StatusBadRequest, "更新任务失败: "+err.Error())
		return
	}
	_ = s.store.AddAudit(s.auth.Username(), "更新任务", task.Name, clientIP(r), task.Exe)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "任务 ID 不合法")
		return
	}
	task, err := s.store.GetTask(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "任务不存在")
		return
	}
	if err := s.manager.Stop(id); err != nil {
		// 未在运行时忽略即可，仅记录日志。
		s.logger.Debug("删除前停止任务", "task", task.Name, "error", err)
	}
	if err := s.store.DeleteTask(id); err != nil {
		writeError(w, http.StatusBadRequest, "删除任务失败: "+err.Error())
		return
	}
	_ = s.store.AddAudit(s.auth.Username(), "删除任务", task.Name, clientIP(r), "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleStartTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseTaskID(w, r)
	if !ok {
		return
	}
	task, err := s.store.GetTask(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "任务不存在")
		return
	}
	if err := s.manager.Start(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.store.AddAudit(s.auth.Username(), "启动任务", task.Name, clientIP(r), "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleStopTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseTaskID(w, r)
	if !ok {
		return
	}
	task, err := s.store.GetTask(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "任务不存在")
		return
	}
	if err := s.manager.Stop(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.store.AddAudit(s.auth.Username(), "停止任务", task.Name, clientIP(r), "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleRestartTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseTaskID(w, r)
	if !ok {
		return
	}
	task, err := s.store.GetTask(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "任务不存在")
		return
	}
	if err := s.manager.Restart(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.store.AddAudit(s.auth.Username(), "重启任务", task.Name, clientIP(r), "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleTaskLogs(w http.ResponseWriter, r *http.Request) {
	id, ok := parseTaskID(w, r)
	if !ok {
		return
	}
	limit := 300
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 3000 {
			limit = n
		}
	}
	lines, exists := s.manager.Logs(id, limit)
	if !exists {
		writeJSON(w, http.StatusOK, map[string]any{"lines": []any{}, "logPath": ""})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

func (s *Server) handleReorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "排序列表不能为空")
		return
	}
	if err := s.store.ReorderTasks(req.IDs); err != nil {
		writeError(w, http.StatusBadRequest, "调整顺序失败: "+err.Error())
		return
	}
	_ = s.store.AddAudit(s.auth.Username(), "调整启动顺序", "", clientIP(r), "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleStartAll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Boot bool `json:"boot"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := s.manager.StartAllAsync(req.Boot); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	_ = s.store.AddAudit(s.auth.Username(), "编排启动", "", clientIP(r), "boot="+strconv.FormatBool(req.Boot))
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) handleStopAll(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.StopAllAsync(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	_ = s.store.AddAudit(s.auth.Username(), "编排停止", "", clientIP(r), "")
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

// handleImportProcess 把正在运行的进程转换成一个托管任务，并默认勾选开机自启。
func (s *Server) handleImportProcess(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.Atoi(r.PathValue("pid"))
	if err != nil || pid <= 0 {
		writeError(w, http.StatusBadRequest, "PID 不合法")
		return
	}

	snapshot, err := s.collector.Snapshot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "采集进程信息失败: "+err.Error())
		return
	}
	var target *procmon.Proc
	for i := range snapshot.Processes {
		if int(snapshot.Processes[i].PID) == pid {
			target = &snapshot.Processes[i]
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "进程不存在或已退出")
		return
	}
	if target.Exe == "" {
		writeError(w, http.StatusBadRequest, "无法读取该进程的可执行文件路径，请以管理员身份运行 SevrTree 后重试")
		return
	}

	args := procmon.ArgsFromCmdline(target.Cmdline)
	tasks, err := s.store.ListTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取任务失败: "+err.Error())
		return
	}
	for _, t := range tasks {
		if strings.EqualFold(t.Exe, target.Exe) && equalArgs(t.Args, args) {
			writeError(w, http.StatusConflict, "该进程已在托管任务中："+t.Name)
			return
		}
	}

	name := procmon.SuggestTaskName(target.Exe, target.Name, target.Cmdline)
	task := &store.Task{
		Name:               name,
		Exe:                target.Exe,
		Args:               args,
		WorkDir:            filepath.Dir(target.Exe),
		Env:                map[string]string{},
		Enabled:            true,
		AutoStart:          true,
		SortOrder:          len(tasks),
		StopTimeoutSeconds: 15,
		MaxRestarts:        3,
		DependsOn:          []int64{},
	}
	if len(target.Ports) > 0 {
		task.HealthPort = int(target.Ports[0])
	}

	id, err := s.store.CreateTask(task)
	if err != nil && strings.Contains(err.Error(), "已存在") {
		task.Name = fmt.Sprintf("%s-%d", name, pid)
		id, err = s.store.CreateTask(task)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "创建任务失败: "+err.Error())
		return
	}

	_ = s.store.AddAudit(s.auth.Username(), "从进程加入自启", task.Name, clientIP(r),
		fmt.Sprintf("pid=%d exe=%s", pid, target.Exe))
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      id,
		"name":    task.Name,
		"exe":     task.Exe,
		"args":    task.Args,
		"port":    task.HealthPort,
		"fromPid": pid,
	})
}

func equalArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func parseTaskID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "任务 ID 不合法")
		return 0, false
	}
	return id, true
}
