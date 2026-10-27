package procmon

import (
	"log/slog"
	"math"
	"net"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	gopsnet "github.com/shirou/gopsutil/v4/net"
	gopsprocess "github.com/shirou/gopsutil/v4/process"
)

const (
	snapshotCache = 1500 * time.Millisecond
	connCache     = 2000 * time.Millisecond
	wmiCache      = 4000 * time.Millisecond
	workerCount   = 16
)

type cpuSample struct {
	total float64
}

// Collector 负责周期采集进程快照，并缓存开销较大的系统调用结果。
type Collector struct {
	managed func() map[int32]int64
	logger  *slog.Logger

	cpuMu    sync.Mutex
	cpuPrev  map[int32]cpuSample
	prevAt   time.Time
	cpuCount float64

	connMu sync.Mutex
	conns  []gopsnet.ConnectionStat
	connAt time.Time

	wmiMu  sync.Mutex
	wmiMap map[int32]wmiProcess
	wmiAt  time.Time

	snapMu sync.Mutex
	snap   *Snapshot
	snapAt time.Time
}

// New 创建采集器。managed 用于标出由 SevrTree 托管启动的进程。
func New(managed func() map[int32]int64, logger *slog.Logger) *Collector {
	return &Collector{
		managed:  managed,
		logger:   logger,
		cpuPrev:  make(map[int32]cpuSample),
		cpuCount: float64(runtimeCPUCount()),
	}
}

// Snapshot 返回最近一次采集结果，命中缓存时直接复用。
func (c *Collector) Snapshot() (*Snapshot, error) {
	c.snapMu.Lock()
	defer c.snapMu.Unlock()

	if c.snap != nil && time.Since(c.snapAt) < snapshotCache {
		return c.snap, nil
	}
	s, err := c.collect()
	if err != nil {
		return nil, err
	}
	c.snap, c.snapAt = s, time.Now()
	return s, nil
}

func (c *Collector) collect() (*Snapshot, error) {
	processes, err := gopsprocess.Processes()
	if err != nil {
		return nil, err
	}

	wmiMap := c.wmiTable()
	conns := c.connectionTable()

	listeners := make(map[uint32]int32)
	perPID := make(map[int32][]gopsnet.ConnectionStat)
	for _, cn := range conns {
		if cn.Laddr.Port > 0 {
			perPID[cn.Pid] = append(perPID[cn.Pid], cn)
		}
		if cn.Status == "LISTEN" && cn.Laddr.Port > 0 {
			listeners[cn.Laddr.Port] = cn.Pid
		}
	}

	result := make([]Proc, len(processes))
	sem := make(chan struct{}, workerCount)
	var wg sync.WaitGroup
	for i, p := range processes {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, p *gopsprocess.Process) {
			defer wg.Done()
			defer func() { <-sem }()
			result[i] = c.readProcess(p, wmiMap, perPID)
		}(i, p)
	}
	wg.Wait()

	c.fillCPU(processes, result)

	sort.Slice(result, func(i, j int) bool { return result[i].PID < result[j].PID })

	index := make(map[int32]*Proc, len(result))
	for i := range result {
		index[result[i].PID] = &result[i]
	}

	managed := map[int32]int64{}
	if c.managed != nil {
		managed = c.managed()
	}

	// 祖先链、启动者、托管标记
	for i := range result {
		p := &result[i]
		if pid, ok := managed[p.PID]; ok {
			p.Managed = true
			taskID := pid
			p.TaskID = &taskID
		}
		p.Chain = buildChain(p, index)
		p.Launcher = describeLauncher(index[p.PPID])
	}

	edges, services := analyzeDependencies(result, index, listeners, perPID)

	return &Snapshot{
		CollectedAt: time.Now(),
		Processes:   result,
		Edges:       edges,
		Services:    services,
		Total:       len(result),
	}, nil
}

func (c *Collector) readProcess(p *gopsprocess.Process, wmiMap map[int32]wmiProcess, perPID map[int32][]gopsnet.ConnectionStat) (info Proc) {
	info.PID = p.Pid
	defer func() {
		if r := recover(); r != nil {
			c.logger.Debug("采集进程信息异常", "pid", p.Pid, "error", r)
		}
	}()

	if w, ok := wmiMap[p.Pid]; ok {
		info.PPID = w.ParentPID
		info.Name = w.Name
		info.Exe = w.ExecutablePath
		info.Cmdline = w.CommandLine
	}
	if info.Name == "" {
		if name, err := p.Name(); err == nil {
			info.Name = name
		}
	}
	if info.Exe == "" {
		if exe, err := p.Exe(); err == nil {
			info.Exe = exe
		}
	}
	if info.Cmdline == "" {
		if cmd, err := p.Cmdline(); err == nil {
			info.Cmdline = cmd
		}
	}
	if info.PPID == 0 {
		if ppid, err := p.Ppid(); err == nil {
			info.PPID = ppid
		}
	}
	if user, err := p.Username(); err == nil {
		info.User = user
	}
	if ct, err := p.CreateTime(); err == nil {
		info.CreatedAt = ct
	}
	if st, err := p.Status(); err == nil && len(st) > 0 {
		info.Status = st[0]
	}
	if mi, err := p.MemoryInfo(); err == nil && mi != nil {
		info.MemRSS = mi.RSS
	}

	var ports []uint32
	for _, cn := range perPID[p.Pid] {
		if cn.Status == "LISTEN" {
			ports = append(ports, cn.Laddr.Port)
		}
	}
	info.Ports = sortedUniquePorts(ports)
	info.Service, info.ServiceKind = detectService(info.Name, info.Exe, info.Ports)
	return info
}

func (c *Collector) fillCPU(processes []*gopsprocess.Process, result []Proc) {
	now := time.Now()

	c.cpuMu.Lock()
	defer c.cpuMu.Unlock()

	wall := now.Sub(c.prevAt).Seconds()
	hasPrev := !c.prevAt.IsZero() && wall > 0.2 && wall < 15
	next := make(map[int32]cpuSample, len(processes))

	for i, p := range processes {
		times, err := p.Times()
		if err != nil || times == nil {
			continue
		}
		total := times.User + times.System
		next[p.Pid] = cpuSample{total: total}
		if !hasPrev {
			continue
		}
		prev, ok := c.cpuPrev[p.Pid]
		if !ok {
			continue
		}
		delta := total - prev.total
		if delta <= 0 {
			continue
		}
		result[i].CPU = math.Round(delta/wall/c.cpuCount*1000) / 10
	}

	c.cpuPrev = next
	c.prevAt = now
}

func (c *Collector) connectionTable() []gopsnet.ConnectionStat {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if time.Since(c.connAt) < connCache {
		return c.conns
	}
	conns, err := gopsnet.Connections("tcp")
	if err != nil {
		c.logger.Debug("读取 TCP 连接失败", "error", err)
		return c.conns
	}
	c.conns, c.connAt = conns, time.Now()
	return conns
}

func (c *Collector) wmiTable() map[int32]wmiProcess {
	c.wmiMu.Lock()
	defer c.wmiMu.Unlock()
	if time.Since(c.wmiAt) < wmiCache {
		return c.wmiMap
	}
	table, err := queryProcessTable()
	if err != nil {
		c.logger.Debug("WMI 查询进程失败", "error", err)
		return c.wmiMap
	}
	c.wmiMap, c.wmiAt = table, time.Now()
	return table
}

func buildChain(self *Proc, index map[int32]*Proc) []ChainItem {
	chain := make([]ChainItem, 0, 6)
	seen := map[int32]struct{}{self.PID: {}}
	ppid := self.PPID
	for depth := 0; depth < 12 && ppid > 0; depth++ {
		if _, ok := seen[ppid]; ok {
			break
		}
		seen[ppid] = struct{}{}
		parent, ok := index[ppid]
		if !ok {
			chain = append(chain, ChainItem{PID: ppid, Name: "已退出进程", Cmdline: ""})
			break
		}
		chain = append(chain, ChainItem{PID: parent.PID, Name: parent.Name, Cmdline: parent.Cmdline})
		ppid = parent.PPID
	}
	return chain
}

func describeLauncher(parent *Proc) string {
	if parent == nil {
		return "无父进程"
	}
	lower := strings.ToLower(parent.Name)

	switch {
	case lower == "explorer.exe":
		return "explorer.exe（桌面手动启动）"
	case lower == "services.exe":
		return "services.exe（Windows 服务）"
	case lower == "wininit.exe" || lower == "smss.exe" || lower == "csrss.exe" || lower == "winlogon.exe":
		return parent.Name + "（系统启动）"
	case lower == "sevrtree.exe":
		return "SevrTree（托管启动）"
	}

	script := extractScript(parent.Cmdline)
	if script != "" && !strings.EqualFold(script, parent.Name) {
		return parent.Name + " → " + script
	}
	return parent.Name
}

var scriptExtensions = []string{".bat", ".cmd", ".ps1", ".sh", ".py", ".pyw", ".js", ".mjs", ".jar", ".vbs"}

// extractScript 从父进程命令行中解析被执行的脚本名，用于还原真实启动入口。
func extractScript(cmdline string) string {
	if cmdline == "" {
		return ""
	}
	for _, token := range splitCommandLine(cmdline) {
		clean := strings.Trim(token, `"'`)
		if clean == "" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(clean))
		for _, known := range scriptExtensions {
			if ext == known {
				return filepath.Base(clean)
			}
		}
	}
	return ""
}

// ArgsFromCmdline 从完整命令行中剥离可执行文件令牌，返回可用于重新启动的参数列表。
func ArgsFromCmdline(cmdline string) []string {
	tokens := splitCommandLine(cmdline)
	if len(tokens) <= 1 {
		return []string{}
	}
	return append([]string{}, tokens[1:]...)
}

// SuggestTaskName 依据进程信息推导建议的任务名称：
// 优先使用命令行中的脚本文件名，其次使用可执行文件名。
func SuggestTaskName(exe, name, cmdline string) string {
	if script := extractScript(cmdline); script != "" {
		base := strings.TrimSuffix(script, filepath.Ext(script))
		if base != "" {
			return base
		}
	}
	base := filepath.Base(exe)
	if base == "" || base == "." {
		base = name
	}
	if trimmed := strings.TrimSuffix(base, filepath.Ext(base)); trimmed != "" {
		return trimmed
	}
	return base
}

// SuggestWorkDir 推断适合作为任务工作目录的路径：
// 优先取命令行中脚本或 jar 的绝对路径所在目录，其次回溯 Python 虚拟环境根目录，
// 最后退回可执行文件所在目录。
func SuggestWorkDir(exe, cmdline string) string {
	for _, token := range splitCommandLine(cmdline) {
		clean := strings.Trim(token, `"'`)
		if clean == "" || !filepath.IsAbs(clean) || !isScriptToken(clean) {
			continue
		}
		return filepath.Dir(clean)
	}
	if root := venvRoot(exe); root != "" {
		return root
	}
	return filepath.Dir(exe)
}

func isScriptToken(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, known := range scriptExtensions {
		if ext == known {
			return true
		}
	}
	return false
}

// venvRoot 识别虚拟环境布局，返回项目根目录，例如 <project>/.venv/Scripts/python.exe。
func venvRoot(exe string) string {
	venvDir := filepath.Dir(filepath.Dir(exe))
	switch strings.ToLower(filepath.Base(venvDir)) {
	case ".venv", "venv", "env", ".env":
		return filepath.Dir(venvDir)
	}
	return ""
}

// splitCommandLine 按空白切分命令行，保留引号内的空格。
func splitCommandLine(s string) []string {
	var out []string
	var b strings.Builder
	quote := rune(0)
	inToken := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
		case r == '"' || r == '\'':
			quote = r
			inToken = true
		case r == ' ' || r == '\t':
			if inToken || b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
				inToken = false
			}
		default:
			b.WriteRune(r)
			inToken = true
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func localAddressSet() map[string]struct{} {
	set := map[string]struct{}{
		"127.0.0.1": {},
		"::1":       {},
		"0.0.0.0":   {},
		"::":        {},
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return set
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP != nil {
				set[ipnet.IP.String()] = struct{}{}
			}
		}
	}
	return set
}
