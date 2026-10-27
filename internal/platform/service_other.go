//go:build !windows

package platform

import "errors"

// ServiceName 在类 Unix 平台保留同名常量以便接口统一。
const ServiceName = "SevrTree"

// ServiceStatus 在非 Windows 平台恒为未安装。
func ServiceStatus() (bool, string) { return false, "" }

// InstallAutostart 在非 Windows 平台暂不支持通过 nssm 注册。
func InstallAutostart(_, _, _ string) error {
	return errors.New("开机自启服务安装目前仅支持 Windows")
}

// UninstallAutostart 在非 Windows 平台不做处理。
func UninstallAutostart(_ string) error { return nil }

// StartService 在非 Windows 平台不做处理。
func StartService(_ string) error { return errors.New("仅支持 Windows") }

// StopService 在非 Windows 平台不做处理。
func StopService(_ string) error { return errors.New("仅支持 Windows") }
