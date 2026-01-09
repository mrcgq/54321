//go:build !windows
// +build !windows

package system

// Windows 特定方法的占位实现
func (m *AutoStartManager) isEnabledWindows() bool {
	return false
}

func (m *AutoStartManager) enableWindows() error {
	return nil
}

func (m *AutoStartManager) disableWindows() error {
	return nil
}
