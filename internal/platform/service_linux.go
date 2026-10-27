//go:build linux

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ServiceName 是注册到 systemd 的单元名。
const ServiceName = "sevrtree"

const (
	unitName = "sevrtree.service"
	unitPath = "/etc/systemd/system/" + unitName
)

// ServiceStatus 返回单元是否已安装及当前运行状态。
func ServiceStatus() (bool, string) {
	if _, err := os.Stat(unitPath); err != nil {
		return false, ""
	}
	out, _ := exec.Command("systemctl", "is-active", unitName).Output()
	state := strings.TrimSpace(string(out))
	if state == "" {
		state = "unknown"
	}
	return true, state
}

// InstallAutostart 写入 systemd 单元并设为开机自启。
func InstallAutostart(exePath, workDir, logDir string) error {
	if os.Geteuid() != 0 {
		return errors.New("需要 root 权限才能安装 systemd 服务")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	unit := fmt.Sprintf(`[Unit]
Description=SevrTree 进程依赖与启动管理
After=network.target

[Service]
Type=simple
ExecStart=%s
WorkingDirectory=%s
Restart=on-failure
RestartSec=5
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=multi-user.target
`, quoteUnitValue(exePath), quoteUnitValue(workDir),
		filepath.Join(logDir, "service.out.log"), filepath.Join(logDir, "service.err.log"))

	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("写入 systemd 单元失败: %w", err)
	}
	if out, err := systemctl("daemon-reload"); err != nil {
		return fmt.Errorf("重载 systemd 失败: %s %s", err, out)
	}
	if out, err := systemctl("enable", unitName); err != nil {
		return fmt.Errorf("设置开机自启失败: %s %s", err, out)
	}
	return nil
}

// UninstallAutostart 停止并移除 systemd 单元。
func UninstallAutostart(_ string) error {
	if os.Geteuid() != 0 {
		return errors.New("需要 root 权限才能卸载 systemd 服务")
	}
	_, _ = systemctl("stop", unitName)
	_, _ = systemctl("disable", unitName)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 systemd 单元失败: %w", err)
	}
	_, _ = systemctl("daemon-reload")
	return nil
}

// StartService 启动 systemd 单元。
func StartService(_ string) error {
	if out, err := systemctl("start", unitName); err != nil {
		return fmt.Errorf("启动服务失败: %s %s", err, out)
	}
	return nil
}

// StopService 停止 systemd 单元。
func StopService(_ string) error {
	if out, err := systemctl("stop", unitName); err != nil {
		return fmt.Errorf("停止服务失败: %s %s", err, out)
	}
	return nil
}

func systemctl(args ...string) (string, error) {
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// quoteUnitValue 在值包含空白或引号时按 systemd 语法加引号。
func quoteUnitValue(v string) string {
	if !strings.ContainsAny(v, " \t\"") {
		return v
	}
	return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
}
