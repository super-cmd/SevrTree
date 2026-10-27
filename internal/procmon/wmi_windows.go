//go:build windows

package procmon

import "github.com/yusufpapurcu/wmi"

// wmiProcess 是 Win32_Process 的精简视图。
type wmiProcess struct {
	PID            int32
	ParentPID      int32
	CommandLine    string
	ExecutablePath string
	Name           string
}

// Win32_Process 必须与 WMI 类同名，wmi.CreateQuery 依据该名称生成查询。
type Win32_Process struct {
	ProcessId       uint32
	ParentProcessId uint32
	CommandLine     string
	ExecutablePath  string
	Name            string
}

// queryProcessTable 通过 WMI 获取全量进程命令行与父进程信息。
// 相比逐个读取 PEB，一次查询即可覆盖同机全部进程（含高权限进程）。
func queryProcessTable() (map[int32]wmiProcess, error) {
	var rows []Win32_Process
	query := wmi.CreateQuery(&rows, "")
	if err := wmi.Query(query, &rows); err != nil {
		return nil, err
	}
	out := make(map[int32]wmiProcess, len(rows))
	for _, r := range rows {
		out[int32(r.ProcessId)] = wmiProcess{
			PID:            int32(r.ProcessId),
			ParentPID:      int32(r.ParentProcessId),
			CommandLine:    r.CommandLine,
			ExecutablePath: r.ExecutablePath,
			Name:           r.Name,
		}
	}
	return out, nil
}
