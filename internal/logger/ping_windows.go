//go:build windows
// +build windows

package logger

import (
	"os/exec"
	"syscall"
)

func init() {
	// 重新定义hideWindow函数
}

// hideWindow 隐藏Windows控制台窗口
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
