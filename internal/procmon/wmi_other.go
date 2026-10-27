//go:build !windows

package procmon

// wmiProcess 在非 Windows 平台不参与采集。
type wmiProcess struct {
	PID            int32
	ParentPID      int32
	CommandLine    string
	ExecutablePath string
	Name           string
}

// queryProcessTable 在非 Windows 平台返回空表，采集回退到 gopsutil。
func queryProcessTable() (map[int32]wmiProcess, error) {
	return nil, nil
}
