package platform

import (
	"runtime"

	gopshost "github.com/shirou/gopsutil/v4/host"
	gopsmem "github.com/shirou/gopsutil/v4/mem"
)

// Info 是系统概览信息。
type Info struct {
	Hostname        string  `json:"hostname"`
	OS              string  `json:"os"`
	Platform        string  `json:"platform"`
	PlatformVersion string  `json:"platformVersion"`
	Arch            string  `json:"arch"`
	UptimeSeconds   uint64  `json:"uptimeSeconds"`
	CPUCount        int     `json:"cpuCount"`
	MemTotal        uint64  `json:"memTotal"`
	MemUsed         uint64  `json:"memUsed"`
	MemPercent      float64 `json:"memPercent"`
	Elevated        bool    `json:"elevated"`
}

// System 采集当前系统概览。
func System() Info {
	info := Info{
		Arch:     runtime.GOARCH,
		CPUCount: runtime.NumCPU(),
		Elevated: IsElevated(),
	}
	if h, err := gopshost.Info(); err == nil && h != nil {
		info.Hostname = h.Hostname
		info.OS = h.OS
		info.Platform = h.Platform
		info.PlatformVersion = h.PlatformVersion
		info.UptimeSeconds = h.Uptime
	}
	if vm, err := gopsmem.VirtualMemory(); err == nil && vm != nil {
		info.MemTotal = vm.Total
		info.MemUsed = vm.Used
		info.MemPercent = vm.UsedPercent
	}
	return info
}
