// Package procmon 采集本机进程信息：父子关系、启动者、命令行、监听端口与依赖服务。
package procmon

import "time"

// ChainItem 表示进程祖先链上的一环。
type ChainItem struct {
	PID     int32  `json:"pid"`
	Name    string `json:"name"`
	Cmdline string `json:"cmdline"`
}

// Proc 是单个进程的完整视图。
type Proc struct {
	PID          int32       `json:"pid"`
	PPID         int32       `json:"ppid"`
	Name         string      `json:"name"`
	Exe          string      `json:"exe"`
	Cmdline      string      `json:"cmdline"`
	User         string      `json:"user"`
	CreatedAt    int64       `json:"createdAt"`
	CPU          float64     `json:"cpu"`
	MemRSS       uint64      `json:"memRss"`
	Status       string      `json:"status"`
	Ports        []uint32    `json:"ports"`
	Service      string      `json:"service"`
	ServiceKind  string      `json:"serviceKind"`
	Dependencies []string    `json:"dependencies"`
	Chain        []ChainItem `json:"chain"`
	Launcher     string      `json:"launcher"`
	Managed      bool        `json:"managed"`
	TaskID       *int64      `json:"taskId,omitempty"`
}

// Edge 表示一条依赖边：From 依赖 To。
type Edge struct {
	From int32  `json:"from"`
	To   int32  `json:"to"`
	Kind string `json:"kind"`
}

// ServiceNode 表示被识别出的服务（如 nginx / mysql / redis）。
type ServiceNode struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	PIDs    []int32  `json:"pids"`
	Ports   []uint32 `json:"ports"`
	Clients int      `json:"clients"`
	Managed bool     `json:"managed"`
}

// Snapshot 是一次采集的完整结果。
type Snapshot struct {
	CollectedAt time.Time     `json:"collectedAt"`
	Processes   []Proc        `json:"processes"`
	Edges       []Edge        `json:"edges"`
	Services    []ServiceNode `json:"services"`
	Total       int           `json:"total"`
}
