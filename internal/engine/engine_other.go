//go:build !windows
// +build !windows

package engine

import (
	"os/exec"
	"syscall"
)

// hideWindow 非Windows平台无需隐藏窗口
func (m *Manager) hideWindow(cmd *exec.Cmd) {
	// Unix系统：设置进程组，方便后续kill整个组
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// killProcessTree 终止进程树（Unix）
func (m *Manager) killProcessTree(pid int) error {
	// 发送SIGKILL到进程组
	return syscall.Kill(-pid, syscall.SIGKILL)
}
