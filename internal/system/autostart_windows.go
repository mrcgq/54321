//go:build windows
// +build windows

package system

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const (
	registryKey  = `Software\Microsoft\Windows\CurrentVersion\Run`
	registryName = "XLinkClient"
)

// isEnabledWindows 检查Windows开机自启状态
func (m *AutoStartManager) isEnabledWindows() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	_, _, err = key.GetStringValue(registryName)
	return err == nil
}

// enableWindows 启用Windows开机自启
func (m *AutoStartManager) enableWindows() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表失败: %w", err)
	}
	defer key.Close()

	// 添加 -autostart 参数
	value := fmt.Sprintf(`"%s" -autostart`, m.exePath)
	if err := key.SetStringValue(registryName, value); err != nil {
		return fmt.Errorf("写入注册表失败: %w", err)
	}

	return nil
}

// disableWindows 禁用Windows开机自启
func (m *AutoStartManager) disableWindows() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表失败: %w", err)
	}
	defer key.Close()

	if err := key.DeleteValue(registryName); err != nil {
		// 如果值不存在，不算错误
		if err != registry.ErrNotExist {
			return fmt.Errorf("删除注册表项失败: %w", err)
		}
	}

	return nil
}
