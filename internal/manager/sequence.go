package manager

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"sevrtree/internal/store"
)

// SequenceState 描述一次启动 / 停止编排的执行进度。
type SequenceState struct {
	Running   bool      `json:"running"`
	Mode      string    `json:"mode"`
	Current   string    `json:"current"`
	Message   string    `json:"message"`
	StartedAt time.Time `json:"startedAt"`
	Started   int       `json:"started"`
	Failed    int       `json:"failed"`
}

// Sequence 返回当前编排状态。
func (m *Manager) Sequence() SequenceState {
	m.seqMu.Lock()
	defer m.seqMu.Unlock()
	return m.seq
}

// StartAllAsync 按依赖顺序异步启动全部任务。
// boot 为 true 时只启动勾选了「开机自启」的任务。
func (m *Manager) StartAllAsync(boot bool) error {
	m.seqMu.Lock()
	if m.seq.Running {
		m.seqMu.Unlock()
		return errors.New("已有编排任务正在执行")
	}
	m.seq = SequenceState{
		Running:   true,
		Mode:      "start",
		Message:   "准备启动",
		StartedAt: time.Now(),
	}
	m.seqMu.Unlock()

	go m.runStartSequence(boot)
	return nil
}

// StopAllAsync 按反向依赖顺序异步停止全部运行中的任务。
func (m *Manager) StopAllAsync() error {
	m.seqMu.Lock()
	if m.seq.Running {
		m.seqMu.Unlock()
		return errors.New("已有编排任务正在执行")
	}
	m.seq = SequenceState{
		Running:   true,
		Mode:      "stop",
		Message:   "准备停止",
		StartedAt: time.Now(),
	}
	m.seqMu.Unlock()

	go m.runStopSequence()
	return nil
}

func (m *Manager) runStartSequence(boot bool) {
	tasks, err := m.store.ListTasks()
	if err != nil {
		m.finishSequence("读取任务配置失败: " + err.Error())
		return
	}

	filtered := make([]*store.Task, 0, len(tasks))
	for _, t := range tasks {
		if !t.Enabled {
			continue
		}
		if boot && !t.AutoStart {
			continue
		}
		filtered = append(filtered, t)
	}
	if len(filtered) == 0 {
		m.finishSequence("没有需要启动的任务")
		return
	}

	order, err := topoSort(filtered)
	if err != nil {
		m.finishSequence(err.Error())
		return
	}

	byID := make(map[int64]*store.Task, len(filtered))
	for _, t := range filtered {
		byID[t.ID] = t
	}

	started, failed := 0, 0
	for _, task := range order {
		if m.isShuttingDown() {
			break
		}
		m.updateSequence(task.Name, "正在启动: "+task.Name, started, failed)

		if task.DelaySeconds > 0 && !m.sleepWithShutdown(time.Duration(task.DelaySeconds)*time.Second) {
			break
		}

		if missing := m.checkDependencies(task, byID); missing != "" {
			failed++
			m.logger.Warn("依赖未就绪，跳过启动", "task", task.Name, "missing", missing)
			continue
		}

		if err := m.Start(task.ID); err != nil {
			if !strings.Contains(err.Error(), "已在运行") {
				failed++
			}
			m.logger.Warn("编排启动失败", "task", task.Name, "error", err)
			continue
		}
		started++

		if task.HealthPort > 0 {
			m.updateSequence(task.Name, fmt.Sprintf("等待 %s 端口 %d 就绪", task.Name, task.HealthPort), started, failed)
			if !waitPort(task.HealthPort, 30*time.Second) {
				m.logger.Warn("健康检查端口未就绪", "task", task.Name, "port", task.HealthPort)
			}
		}
	}

	m.finishSequence(fmt.Sprintf("启动完成：成功 %d 个，失败/跳过 %d 个", started, failed))
}

func (m *Manager) runStopSequence() {
	tasks, err := m.store.ListTasks()
	if err != nil {
		m.finishSequence("读取任务配置失败: " + err.Error())
		return
	}

	order, err := topoSort(tasks)
	if err != nil {
		// 依赖存在环路时退化为按排序权重停止，保证仍能收尾。
		order = tasks
	}

	stopped, failed := 0, 0
	for i := len(order) - 1; i >= 0; i-- {
		if m.isShuttingDown() {
			break
		}
		task := order[i]
		m.updateSequence(task.Name, "正在停止: "+task.Name, stopped, failed)
		if err := m.Stop(task.ID); err != nil {
			if !strings.Contains(err.Error(), "未在运行") {
				failed++
				m.logger.Warn("编排停止失败", "task", task.Name, "error", err)
			}
			continue
		}
		stopped++
	}
	m.finishSequence(fmt.Sprintf("停止完成：已停止 %d 个，异常 %d 个", stopped, failed))
}

func (m *Manager) updateSequence(current, message string, started, failed int) {
	m.seqMu.Lock()
	m.seq.Current = current
	m.seq.Message = message
	m.seq.Started = started
	m.seq.Failed = failed
	m.seqMu.Unlock()
}

func (m *Manager) finishSequence(message string) {
	m.seqMu.Lock()
	m.seq.Running = false
	m.seq.Current = ""
	m.seq.Message = message
	m.seqMu.Unlock()
	m.logger.Info("编排结束", "message", message)
}

func (m *Manager) sleepWithShutdown(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if m.isShuttingDown() {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
	return true
}

func (m *Manager) checkDependencies(task *store.Task, set map[int64]*store.Task) string {
	if len(task.DependsOn) == 0 {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var missing []string
	for _, depID := range task.DependsOn {
		if _, ok := set[depID]; !ok {
			continue
		}
		inst, ok := m.instances[depID]
		if !ok || !inst.isActive() {
			name := fmt.Sprintf("任务#%d", depID)
			if dep, ok := set[depID]; ok {
				name = dep.Name
			}
			missing = append(missing, name)
		}
	}
	return strings.Join(missing, "、")
}

// topoSort 依据依赖关系做拓扑排序，同级按排序权重与 ID 稳定输出。
func topoSort(tasks []*store.Task) ([]*store.Task, error) {
	byID := make(map[int64]*store.Task, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
	}

	indegree := make(map[int64]int, len(tasks))
	adjacency := make(map[int64][]int64, len(tasks))
	for _, t := range tasks {
		if _, ok := indegree[t.ID]; !ok {
			indegree[t.ID] = 0
		}
		for _, dep := range t.DependsOn {
			if dep == t.ID {
				return nil, fmt.Errorf("任务「%s」不能依赖自身", t.Name)
			}
			if _, ok := byID[dep]; !ok {
				continue
			}
			adjacency[dep] = append(adjacency[dep], t.ID)
			indegree[t.ID]++
		}
	}

	ready := make([]*store.Task, 0, len(tasks))
	for _, t := range tasks {
		if indegree[t.ID] == 0 {
			ready = append(ready, t)
		}
	}
	sortReady(ready)

	ordered := make([]*store.Task, 0, len(tasks))
	for len(ready) > 0 {
		current := ready[0]
		ready = ready[1:]
		ordered = append(ordered, current)

		for _, nextID := range adjacency[current.ID] {
			indegree[nextID]--
			if indegree[nextID] == 0 {
				if next, ok := byID[nextID]; ok {
					ready = append(ready, next)
					sortReady(ready)
				}
			}
		}
	}

	if len(ordered) != len(tasks) {
		return nil, errors.New("任务依赖存在环路，请检查依赖配置")
	}
	return ordered, nil
}

func sortReady(tasks []*store.Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].SortOrder != tasks[j].SortOrder {
			return tasks[i].SortOrder < tasks[j].SortOrder
		}
		return tasks[i].ID < tasks[j].ID
	})
}
