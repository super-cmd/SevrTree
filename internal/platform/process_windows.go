// Package platform 封装操作系统差异：权限判断、子进程配置、进程树终止与开机自启。
package platform

import (
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

// IsElevated 判断当前进程是否以管理员权限运行。
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// ConfigureCmd 为 Windows 子进程隐藏控制台窗口并创建独立进程组，
// 便于后续按进程树整体结束。
func ConfigureCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
}

// KillTree 结束指定 PID 及其全部子进程。
func KillTree(pid int, force bool) error {
	args := []string{"/PID", strconv.Itoa(pid), "/T"}
	if force {
		args = append(args, "/F")
	}
	cmd := exec.Command("taskkill", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
