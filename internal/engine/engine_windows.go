//go:build windows
// +build windows

package engine

import (
	"os/exec"
	"syscall"
)

// hideWindow 隐藏Windows控制台窗口
func (m *Manager) hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

// killProcessTree 终止进程树（Windows）
func (m *Manager) killProcessTree(pid int) error {
	// 使用taskkill命令终止进程树
	kill := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return kill.Run()
}
