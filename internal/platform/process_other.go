//go:build !windows

package platform

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// IsElevated 在类 Unix 系统上以 uid 是否为 0 判断。
func IsElevated() bool { return os.Geteuid() == 0 }

// ConfigureCmd 在类 Unix 系统上创建独立进程组。
func ConfigureCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillTree 结束整个进程组。
func KillTree(pid int, force bool) error {
	if pid <= 0 {
		return errors.New("PID 不合法")
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	if err := syscall.Kill(-pid, sig); err == nil {
		return nil
	}
	return syscall.Kill(pid, sig)
}
