//go:build !windows
// +build !windows

package logger

import "os/exec"

// hideWindow 非Windows平台无需隐藏窗口
func hideWindow(cmd *exec.Cmd) {
	// 空实现
}
