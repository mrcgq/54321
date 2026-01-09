package system

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// =============================================================================
// 系统代理设置
// =============================================================================

// ProxyManager 系统代理管理器
type ProxyManager struct {
	originalSettings *ProxySettings
}

// ProxySettings 代理设置
type ProxySettings struct {
	Enabled    bool
	Server     string
	Port       int
	BypassList []string
}

// NewProxyManager 创建代理管理器
func NewProxyManager() *ProxyManager {
	return &ProxyManager{}
}

// SetSystemProxy 设置系统代理
func (p *ProxyManager) SetSystemProxy(server string, port int) error {
	// 保存原始设置
	if p.originalSettings == nil {
		settings, _ := p.GetSystemProxy()
		p.originalSettings = settings
	}

	switch runtime.GOOS {
	case "windows":
		return p.setWindowsProxy(server, port)
	case "darwin":
		return p.setMacOSProxy(server, port)
	case "linux":
		return p.setLinuxProxy(server, port)
	default:
		return fmt.Errorf("不支持的操作系统")
	}
}

// ClearSystemProxy 清除系统代理
func (p *ProxyManager) ClearSystemProxy() error {
	switch runtime.GOOS {
	case "windows":
		return p.clearWindowsProxy()
	case "darwin":
		return p.clearMacOSProxy()
	case "linux":
		return p.clearLinuxProxy()
	default:
		return fmt.Errorf("不支持的操作系统")
	}
}

// RestoreSystemProxy 恢复原始代理设置
func (p *ProxyManager) RestoreSystemProxy() error {
	if p.originalSettings == nil {
		return p.ClearSystemProxy()
	}

	if p.originalSettings.Enabled {
		return p.SetSystemProxy(p.originalSettings.Server, p.originalSettings.Port)
	}
	return p.ClearSystemProxy()
}

// GetSystemProxy 获取当前系统代理设置
func (p *ProxyManager) GetSystemProxy() (*ProxySettings, error) {
	switch runtime.GOOS {
	case "windows":
		return p.getWindowsProxy()
	case "darwin":
		return p.getMacOSProxy()
	case "linux":
		return p.getLinuxProxy()
	default:
		return nil, fmt.Errorf("不支持的操作系统")
	}
}

// =============================================================================
// Windows 实现
// =============================================================================

func (p *ProxyManager) setWindowsProxy(server string, port int) error {
	proxyServer := fmt.Sprintf("%s:%d", server, port)

	// 启用代理
	cmd := exec.Command("reg", "add",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f")
	if err := cmd.Run(); err != nil {
		return err
	}

	// 设置代理服务器
	cmd = exec.Command("reg", "add",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyServer", "/t", "REG_SZ", "/d", proxyServer, "/f")
	if err := cmd.Run(); err != nil {
		return err
	}

	// 设置绕过列表
	bypassList := "localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;<local>"
	cmd = exec.Command("reg", "add",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyOverride", "/t", "REG_SZ", "/d", bypassList, "/f")
	return cmd.Run()
}

func (p *ProxyManager) clearWindowsProxy() error {
	cmd := exec.Command("reg", "add",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f")
	return cmd.Run()
}

func (p *ProxyManager) getWindowsProxy() (*ProxySettings, error) {
	settings := &ProxySettings{}

	// 检查是否启用
	cmd := exec.Command("reg", "query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyEnable")
	output, err := cmd.Output()
	if err == nil && strings.Contains(string(output), "0x1") {
		settings.Enabled = true
	}

	// 获取代理服务器
	cmd = exec.Command("reg", "query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyServer")
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "ProxyServer") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					settings.Server = parts[len(parts)-1]
				}
			}
		}
	}

	return settings, nil
}

// =============================================================================
// macOS 实现
// =============================================================================

func (p *ProxyManager) setMacOSProxy(server string, port int) error {
	// 获取网络服务名称
	services, err := p.getMacOSNetworkServices()
	if err != nil {
		return err
	}

	for _, service := range services {
		// 设置SOCKS代理
		cmd := exec.Command("networksetup", "-setsocksfirewallproxy", service, server, fmt.Sprintf("%d", port))
		cmd.Run()

		// 启用SOCKS代理
		cmd = exec.Command("networksetup", "-setsocksfirewallproxystate", service, "on")
		cmd.Run()
	}

	return nil
}

func (p *ProxyManager) clearMacOSProxy() error {
	services, err := p.getMacOSNetworkServices()
	if err != nil {
		return err
	}

	for _, service := range services {
		cmd := exec.Command("networksetup", "-setsocksfirewallproxystate", service, "off")
		cmd.Run()
	}

	return nil
}

func (p *ProxyManager) getMacOSProxy() (*ProxySettings, error) {
	return &ProxySettings{}, nil
}

func (p *ProxyManager) getMacOSNetworkServices() ([]string, error) {
	cmd := exec.Command("networksetup", "-listallnetworkservices")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var services []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines[1:] { // 跳过第一行
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "*") {
			services = append(services, line)
		}
	}

	return services, nil
}

// =============================================================================
// Linux 实现
// =============================================================================

func (p *ProxyManager) setLinuxProxy(server string, port int) error {
	// 设置环境变量（仅对当前用户会话有效）
	proxyURL := fmt.Sprintf("socks5://%s:%d", server, port)

	// 使用 gsettings 设置 GNOME 代理
	cmd := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual")
	cmd.Run()

	cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "host", server)
	cmd.Run()

	cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "port", fmt.Sprintf("%d", port))
	cmd.Run()

	_ = proxyURL
	return nil
}

func (p *ProxyManager) clearLinuxProxy() error {
	cmd := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none")
	return cmd.Run()
}

func (p *ProxyManager) getLinuxProxy() (*ProxySettings, error) {
	return &ProxySettings{}, nil
}
