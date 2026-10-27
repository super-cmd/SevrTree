//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

// ServiceName 是注册到 Windows 服务控制管理器的服务名。
const ServiceName = "SevrTree"

var statePattern = regexp.MustCompile(`STATE\s*:\s*\d+\s+(\w+)`)

// ServiceStatus 返回服务是否已安装及当前状态。
func ServiceStatus() (installed bool, state string) {
	out, err := runHidden("sc.exe", "query", ServiceName)
	if err != nil {
		if strings.Contains(out, "1060") {
			return false, ""
		}
		return false, ""
	}
	if m := statePattern.FindStringSubmatch(out); len(m) == 2 {
		return true, m[1]
	}
	return true, "UNKNOWN"
}

// NSSMPath 定位 nssm.exe，优先使用程序同目录下的副本。
func NSSMPath(exePath string) (string, error) {
	candidate := filepath.Join(filepath.Dir(exePath), "nssm.exe")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	if p, err := exec.LookPath("nssm.exe"); err == nil {
		return p, nil
	}
	return "", errors.New("未找到 nssm.exe，请将其放置在程序同目录下")
}

// InstallAutostart 将当前程序注册为开机自启的 Windows 服务。
func InstallAutostart(exePath, workDir, logDir string) error {
	if !IsElevated() {
		return errors.New("需要管理员权限才能安装系统服务")
	}
	nssm, err := NSSMPath(exePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}

	installed, _ := ServiceStatus()
	if !installed {
		if out, err := runHidden(nssm, "install", ServiceName, exePath); err != nil {
			return fmt.Errorf("安装服务失败: %s %s", err, out)
		}
	}

	settings := [][]string{
		{"set", ServiceName, "Application", exePath},
		{"set", ServiceName, "AppDirectory", workDir},
		{"set", ServiceName, "Start", "SERVICE_AUTO_START"},
		{"set", ServiceName, "DisplayName", "SevrTree 进程依赖与启动管理"},
		{"set", ServiceName, "Description", "采集进程依赖树并管理自启任务的执行顺序"},
		{"set", ServiceName, "AppStdout", filepath.Join(logDir, "service.out.log")},
		{"set", ServiceName, "AppStderr", filepath.Join(logDir, "service.err.log")},
		{"set", ServiceName, "AppRotateFiles", "1"},
		{"set", ServiceName, "AppRotateOnline", "1"},
		{"set", ServiceName, "AppRotateBytes", "10485760"},
	}
	for _, s := range settings {
		if out, err := runHidden(nssm, s...); err != nil {
			return fmt.Errorf("配置服务参数失败(%s): %s %s", s[2], err, out)
		}
	}
	return nil
}

// UninstallAutostart 停止并移除 Windows 服务。
func UninstallAutostart(exePath string) error {
	if !IsElevated() {
		return errors.New("需要管理员权限才能卸载系统服务")
	}
	nssm, err := NSSMPath(exePath)
	if err != nil {
		return err
	}
	installed, _ := ServiceStatus()
	if !installed {
		return nil
	}
	_, _ = runHidden(nssm, "stop", ServiceName)
	if out, err := runHidden(nssm, "remove", ServiceName, "confirm"); err != nil {
		return fmt.Errorf("卸载服务失败: %s %s", err, out)
	}
	return nil
}

// StartService / StopService 控制已安装服务的运行状态。
func StartService(exePath string) error {
	nssm, err := NSSMPath(exePath)
	if err != nil {
		return err
	}
	if out, err := runHidden(nssm, "start", ServiceName); err != nil {
		return fmt.Errorf("启动服务失败: %s %s", err, out)
	}
	return nil
}

// StopService 停止已安装的服务。
func StopService(exePath string) error {
	nssm, err := NSSMPath(exePath)
	if err != nil {
		return err
	}
	if out, err := runHidden(nssm, "stop", ServiceName); err != nil {
		return fmt.Errorf("停止服务失败: %s %s", err, out)
	}
	return nil
}

func runHidden(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
