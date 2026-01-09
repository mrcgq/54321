// Package system 提供系统级功能
package system

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// =============================================================================
// 开机自启动管理
// =============================================================================

// AutoStartManager 开机自启动管理器
type AutoStartManager struct {
	appName string
	exePath string
}

// NewAutoStartManager 创建自启动管理器
func NewAutoStartManager(appName string) (*AutoStartManager, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取程序路径失败: %w", err)
	}

	return &AutoStartManager{
		appName: appName,
		exePath: exePath,
	}, nil
}

// IsEnabled 检查是否已启用开机自启
func (m *AutoStartManager) IsEnabled() bool {
	switch runtime.GOOS {
	case "windows":
		return m.isEnabledWindows()
	case "darwin":
		return m.isEnabledMacOS()
	case "linux":
		return m.isEnabledLinux()
	default:
		return false
	}
}

// Enable 启用开机自启
func (m *AutoStartManager) Enable() error {
	switch runtime.GOOS {
	case "windows":
		return m.enableWindows()
	case "darwin":
		return m.enableMacOS()
	case "linux":
		return m.enableLinux()
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// Disable 禁用开机自启
func (m *AutoStartManager) Disable() error {
	switch runtime.GOOS {
	case "windows":
		return m.disableWindows()
	case "darwin":
		return m.disableMacOS()
	case "linux":
		return m.disableLinux()
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// =============================================================================
// Linux 实现
// =============================================================================

func (m *AutoStartManager) isEnabledLinux() bool {
	autostartPath := m.getLinuxAutostartPath()
	_, err := os.Stat(autostartPath)
	return err == nil
}

func (m *AutoStartManager) enableLinux() error {
	autostartDir := filepath.Join(os.Getenv("HOME"), ".config", "autostart")
	if err := os.MkdirAll(autostartDir, 0755); err != nil {
		return err
	}

	desktopEntry := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Exec="%s" -autostart
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
`, m.appName, m.exePath)

	return os.WriteFile(m.getLinuxAutostartPath(), []byte(desktopEntry), 0644)
}

func (m *AutoStartManager) disableLinux() error {
	return os.Remove(m.getLinuxAutostartPath())
}

func (m *AutoStartManager) getLinuxAutostartPath() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "autostart", m.appName+".desktop")
}

// =============================================================================
// macOS 实现
// =============================================================================

func (m *AutoStartManager) isEnabledMacOS() bool {
	plistPath := m.getMacOSPlistPath()
	_, err := os.Stat(plistPath)
	return err == nil
}

func (m *AutoStartManager) enableMacOS() error {
	launchAgentsDir := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return err
	}

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.xlink.client</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>-autostart</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`, m.exePath)

	return os.WriteFile(m.getMacOSPlistPath(), []byte(plist), 0644)
}

func (m *AutoStartManager) disableMacOS() error {
	return os.Remove(m.getMacOSPlistPath())
}

func (m *AutoStartManager) getMacOSPlistPath() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.xlink.client.plist")
}
