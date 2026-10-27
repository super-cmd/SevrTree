// Package manager 管理受管任务的运行生命周期：启动、停止、重启、
// 启动顺序编排、健康检查与运行日志。
package manager

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sevrtree/internal/platform"
	"sevrtree/internal/store"
)

// Status 是任务运行状态。
type Status string

// 任务状态常量。
const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusFailed   Status = "failed"
)

// Runtime 是任务的实时运行信息。
type Runtime struct {
	TaskID    int64     `json:"taskId"`
	Name      string    `json:"name"`
	Status    Status    `json:"status"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
	Restarts  int       `json:"restarts"`
	ExitCode  *int      `json:"exitCode,omitempty"`
	LastError string    `json:"lastError,omitempty"`
	LogPath   string    `json:"logPath,omitempty"`
}

type instance struct {
	mu            sync.RWMutex
	name          string
	status        Status
	pid           int
	cmd           *exec.Cmd
	startedAt     time.Time
	restarts      int
	exitCode      *int
	lastError     string
	logPath       string
	logFile       *os.File
	ring          *ringBuffer
	stopRequested bool
	done          chan struct{}
	doneClosed    bool
}

func newInstance(task *store.Task) *instance {
	return &instance{
		name:   task.Name,
		status: StatusStopped,
		ring:   newRing(3000),
		done:   make(chan struct{}),
	}
}

func (i *instance) runtime(taskID int64) Runtime {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return Runtime{
		TaskID:    taskID,
		Name:      i.name,
		Status:    i.status,
		PID:       i.pid,
		StartedAt: i.startedAt,
		Restarts:  i.restarts,
		ExitCode:  i.exitCode,
		LastError: i.lastError,
		LogPath:   i.logPath,
	}
}

func (i *instance) isActive() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.status == StatusStarting || i.status == StatusRunning || i.status == StatusStopping
}

func (i *instance) stopFlag() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.stopRequested
}

func (i *instance) restartCount() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.restarts
}

func (i *instance) currentPID() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.pid
}

func (i *instance) appendLog(stream, text string) {
	i.ring.append(stream, text)
}

func (i *instance) logs(limit int) []LogLine {
	return i.ring.snapshot(limit)
}

func (i *instance) markStarting(logPath string) {
	i.mu.Lock()
	i.status = StatusStarting
	i.logPath = logPath
	i.lastError = ""
	i.exitCode = nil
	i.stopRequested = false
	i.mu.Unlock()
}

func (i *instance) markRunning(cmd *exec.Cmd, file *os.File) {
	i.mu.Lock()
	i.status = StatusRunning
	i.cmd = cmd
	i.pid = cmd.Process.Pid
	i.startedAt = time.Now()
	i.logFile = file
	if i.doneClosed {
		i.done = make(chan struct{})
		i.doneClosed = false
	}
	i.mu.Unlock()
}

func (i *instance) markStopping() {
	i.mu.Lock()
	i.status = StatusStopping
	i.stopRequested = true
	i.mu.Unlock()
}

func (i *instance) markFailed(msg string) {
	i.mu.Lock()
	i.status = StatusFailed
	i.lastError = msg
	i.pid = 0
	i.mu.Unlock()
}

func (i *instance) markExited(code int, err error) {
	i.mu.Lock()
	if i.status != StatusFailed {
		i.status = StatusStopped
	}
	i.exitCode = &code
	i.pid = 0
	if err != nil && i.lastError == "" {
		i.lastError = err.Error()
	}
	if !i.doneClosed {
		close(i.done)
		i.doneClosed = true
	}
	file := i.logFile
	i.logFile = nil
	i.mu.Unlock()
	if file != nil {
		_ = file.Close()
	}
}

func (i *instance) bumpRestart() {
	i.mu.Lock()
	i.restarts++
	i.mu.Unlock()
}

func (i *instance) doneChan() <-chan struct{} {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.done
}

// Manager 持有全部受管任务的运行实例。
type Manager struct {
	store      *store.Store
	logger     *slog.Logger
	taskLogDir string

	mu           sync.Mutex
	instances    map[int64]*instance
	shuttingDown bool
	wg           sync.WaitGroup

	seqMu sync.Mutex
	seq   SequenceState
}

// New 创建任务管理器。
func New(st *store.Store, taskLogDir string, logger *slog.Logger) *Manager {
	return &Manager{
		store:      st,
		logger:     logger,
		taskLogDir: taskLogDir,
		instances:  make(map[int64]*instance),
	}
}

// Snapshot 返回全部任务的运行态。
func (m *Manager) Snapshot() []Runtime {
	m.mu.Lock()
	ids := make([]int64, 0, len(m.instances))
	for id := range m.instances {
		ids = append(ids, id)
	}
	instances := make(map[int64]*instance, len(m.instances))
	for id, inst := range m.instances {
		instances[id] = inst
	}
	m.mu.Unlock()

	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	out := make([]Runtime, 0, len(ids))
	for _, id := range ids {
		out = append(out, instances[id].runtime(id))
	}
	return out
}

// ManagedPIDs 返回 PID 到任务 ID 的映射，供进程采集器标记托管进程。
func (m *Manager) ManagedPIDs() map[int32]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[int32]int64, len(m.instances))
	for id, inst := range m.instances {
		if pid := inst.currentPID(); pid > 0 {
			out[int32(pid)] = id
		}
	}
	return out
}

// Start 启动指定任务。
func (m *Manager) Start(taskID int64) error {
	task, err := m.store.GetTask(taskID)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shuttingDown {
		return errors.New("程序正在退出")
	}
	if inst, ok := m.instances[taskID]; ok && inst.isActive() {
		return fmt.Errorf("%s 已在运行", task.Name)
	}

	inst, ok := m.instances[taskID]
	if !ok {
		inst = newInstance(task)
		m.instances[taskID] = inst
	}
	if err := m.launch(inst, task); err != nil {
		inst.markFailed(err.Error())
		m.logger.Error("任务启动失败", "task", task.Name, "error", err)
		return err
	}
	m.logger.Info("任务已启动", "task", task.Name, "pid", inst.currentPID())
	return nil
}

func (m *Manager) launch(inst *instance, task *store.Task) error {
	if err := os.MkdirAll(m.taskLogDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(m.taskLogDir,
		sanitizeName(task.Name)+"-"+time.Now().Format("20060102-150405")+".log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("创建任务日志失败: %w", err)
	}

	cmd, err := buildCommand(task)
	if err != nil {
		_ = file.Close()
		return err
	}

	inst.markStarting(logPath)
	outWriter := newLineWriter(inst, "stdout")
	errWriter := newLineWriter(inst, "stderr")
	cmd.Stdout = io.MultiWriter(file, outWriter)
	cmd.Stderr = io.MultiWriter(file, errWriter)

	if err := cmd.Start(); err != nil {
		_ = file.Close()
		outWriter.Flush()
		errWriter.Flush()
		return fmt.Errorf("启动失败: %w", err)
	}

	inst.markRunning(cmd, file)
	m.wg.Add(1)
	go m.waitLoop(inst, task, cmd, outWriter, errWriter)
	return nil
}

func (m *Manager) waitLoop(inst *instance, task *store.Task, cmd *exec.Cmd, out, errW *lineWriter) {
	defer m.wg.Done()

	err := cmd.Wait()
	out.Flush()
	errW.Flush()

	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}
	inst.markExited(code, err)
	m.logger.Info("任务已退出", "task", task.Name, "exitCode", code)

	m.maybeRestart(inst, task)
}

func (m *Manager) maybeRestart(inst *instance, task *store.Task) {
	if inst.stopFlag() || !task.RestartOnExit {
		return
	}
	if inst.restartCount() >= task.MaxRestarts {
		return
	}

	m.mu.Lock()
	shutting := m.shuttingDown
	m.mu.Unlock()
	if shutting {
		return
	}

	m.logger.Warn("任务异常退出，准备自动重启", "task", task.Name, "restarts", inst.restartCount())
	time.Sleep(3 * time.Second)

	inst.bumpRestart()
	if err := m.relaunch(inst, task); err != nil {
		inst.markFailed("自动重启失败: " + err.Error())
		m.logger.Error("自动重启失败", "task", task.Name, "error", err)
	}
}

func (m *Manager) relaunch(inst *instance, task *store.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shuttingDown {
		return errors.New("程序正在退出")
	}
	return m.launch(inst, task)
}

// Stop 停止指定任务，先尝试优雅结束，超时后强制结束进程树。
func (m *Manager) Stop(taskID int64) error {
	task, err := m.store.GetTask(taskID)
	if err != nil {
		return err
	}

	m.mu.Lock()
	inst := m.instances[taskID]
	m.mu.Unlock()

	if inst == nil || !inst.isActive() {
		return fmt.Errorf("%s 未在运行", task.Name)
	}

	inst.markStopping()
	pid := inst.currentPID()
	if pid <= 0 {
		return errors.New("进程 PID 不可用")
	}

	timeout := time.Duration(task.StopTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	if err := platform.KillTree(pid, false); err != nil {
		m.logger.Debug("优雅结束进程失败", "task", task.Name, "pid", pid, "error", err)
	}

	select {
	case <-inst.doneChan():
	case <-time.After(timeout):
		m.logger.Warn("优雅结束超时，强制结束进程树", "task", task.Name, "pid", pid)
		if err := platform.KillTree(pid, true); err != nil {
			m.logger.Error("强制结束失败", "task", task.Name, "pid", pid, "error", err)
		}
		select {
		case <-inst.doneChan():
		case <-time.After(5 * time.Second):
			inst.markExited(-1, errors.New("进程未响应强制结束"))
		}
	}

	m.logger.Info("任务已停止", "task", task.Name)
	return nil
}

// Restart 重启指定任务。
func (m *Manager) Restart(taskID int64) error {
	m.mu.Lock()
	inst := m.instances[taskID]
	m.mu.Unlock()
	if inst != nil && inst.isActive() {
		if err := m.Stop(taskID); err != nil {
			return err
		}
	}
	return m.Start(taskID)
}

// Logs 返回任务最近的内存日志。
func (m *Manager) Logs(taskID int64, limit int) ([]LogLine, bool) {
	m.mu.Lock()
	inst := m.instances[taskID]
	m.mu.Unlock()
	if inst == nil {
		return nil, false
	}
	return inst.logs(limit), true
}

// KillAll 强制结束全部运行中的任务，用于程序退出。
func (m *Manager) KillAll() {
	m.mu.Lock()
	m.shuttingDown = true
	pids := make([]int, 0, len(m.instances))
	for _, inst := range m.instances {
		if pid := inst.currentPID(); pid > 0 {
			inst.markStopping()
			pids = append(pids, pid)
		}
	}
	m.mu.Unlock()

	for _, pid := range pids {
		_ = platform.KillTree(pid, true)
	}
}

// Shutdown 结束所有任务并等待收尾。
func (m *Manager) Shutdown(timeout time.Duration) {
	m.KillAll()
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func (m *Manager) isShuttingDown() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shuttingDown
}

func buildCommand(task *store.Task) (*exec.Cmd, error) {
	exe := strings.TrimSpace(task.Exe)
	if exe == "" {
		return nil, errors.New("可执行文件路径为空")
	}
	if _, err := os.Stat(exe); err != nil {
		if _, lookErr := exec.LookPath(exe); lookErr != nil {
			return nil, fmt.Errorf("可执行文件不存在: %s", exe)
		}
	}

	ext := strings.ToLower(filepath.Ext(exe))
	var cmd *exec.Cmd
	switch ext {
	case ".bat", ".cmd":
		args := append([]string{"/c", exe}, task.Args...)
		cmd = exec.Command("cmd.exe", args...)
	case ".ps1":
		args := append([]string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", exe}, task.Args...)
		cmd = exec.Command("powershell.exe", args...)
	case ".sh":
		cmd = exec.Command("bash", append([]string{exe}, task.Args...)...)
	default:
		cmd = exec.Command(exe, task.Args...)
	}

	if task.WorkDir != "" {
		cmd.Dir = task.WorkDir
	} else {
		cmd.Dir = filepath.Dir(exe)
	}
	cmd.Env = os.Environ()
	for k, v := range task.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	platform.ConfigureCmd(cmd)
	return cmd, nil
}

func sanitizeName(name string) string {
	replacer := strings.NewReplacer(
		`\`, "_", "/", "_", ":", "_", "*", "_", "?", "_",
		`"`, "_", "<", "_", ">", "_", "|", "_", " ", "_",
	)
	out := replacer.Replace(name)
	if out == "" {
		return "task"
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

// waitPort 等待本机端口进入监听状态。
func waitPort(port int, timeout time.Duration) bool {
	if port <= 0 {
		return true
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 400*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}
