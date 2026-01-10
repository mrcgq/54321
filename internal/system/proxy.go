package system

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
)

// =============================================================================
// 系统代理设置 (Windows API 增强版)
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
	// 保存原始设置 (仅第一次)
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
// Windows 实现 (包含 API 刷新逻辑)
// =============================================================================

var (
	modwininet            = syscall.NewLazyDLL("wininet.dll")
	procInternetSetOption = modwininet.NewProc("InternetSetOptionW")
)

// refreshSystemProxy 通知系统刷新代理设置
func refreshSystemProxy() {
	// INTERNET_OPTION_SETTINGS_CHANGED = 39
	// INTERNET_OPTION_REFRESH = 37
	// Call 接收 uintptr 类型参数，0 会自动转换
	procInternetSetOption.Call(0, 39, 0, 0)
	procInternetSetOption.Call(0, 37, 0, 0)
}

func (p *ProxyManager) setWindowsProxy(server string, port int) error {
	// ⚠️【核心逻辑】添加 socks= 前缀
	// 强制 Windows 使用 SOCKS 协议连接本地端口
	proxyServer := fmt.Sprintf("socks=%s:%d", server, port)

	// 1. 设置代理服务器地址
	cmd := exec.Command("reg", "add",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyServer", "/t", "REG_SZ", "/d", proxyServer, "/f")
	if err := cmd.Run(); err != nil {
		return err
	}

	// 2. 启用代理 (ProxyEnable = 1)
	cmd = exec.Command("reg", "add",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f")
	if err := cmd.Run(); err != nil {
		return err
	}

	// 3. 设置绕过列表 (本地回环不走代理)
	bypassList := "localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;<local>"
	cmd = exec.Command("reg", "add",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyOverride", "/t", "REG_SZ", "/d", bypassList, "/f")
	if err := cmd.Run(); err != nil {
		return err
	}

	// 4. 通知系统立即刷新
	refreshSystemProxy()
	return nil
}

func (p *ProxyManager) clearWindowsProxy() error {
	// 禁用代理 (ProxyEnable = 0)
	cmd := exec.Command("reg", "add",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f")
	
	if err := cmd.Run(); err != nil {
		return err
	}

	// 通知系统立即刷新
	refreshSystemProxy()
	return nil
}

func (p *ProxyManager) getWindowsProxy() (*ProxySettings, error) {
	settings := &ProxySettings{}

	// 检查是否启用
	cmd := exec.Command("reg", "query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		"/v", "ProxyEnable")
	output, err := cmd.Output()
	// 输出包含 0x1 表示启用
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
				// 输出格式通常为: ProxyServer    REG_SZ    socks=127.0.0.1:10808
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					raw := parts[len(parts)-1]
					// 移除可能存在的协议前缀，只保留 ip:port
					raw = strings.TrimPrefix(raw, "socks=")
					raw = strings.TrimPrefix(raw, "http://")
					settings.Server = raw
				}
			}
		}
	}

	return settings, nil
}

// =============================================================================
// macOS 实现 (保持不变)
// =============================================================================

func (p *ProxyManager) setMacOSProxy(server string, port int) error {
	services, err := p.getMacOSNetworkServices()
	if err != nil {
		return err
	}

	for _, service := range services {
		cmd := exec.Command("networksetup", "-setsocksfirewallproxy", service, server, fmt.Sprintf("%d", port))
		cmd.Run()
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

func (p *ProxyManager) getMacOSNetworkServices() ([]string, error) {
	cmd := exec.Command("networksetup", "-listallnetworkservices")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var services []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "*") {
			services = append(services, line)
		}
	}
	return services, nil
}

func (p *ProxyManager) getMacOSProxy() (*ProxySettings, error) {
	return &ProxySettings{}, nil
}

// =============================================================================
// Linux 实现 (保持不变)
// =============================================================================

func (p *ProxyManager) setLinuxProxy(server string, port int) error {
	exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual").Run()
	exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "host", server).Run()
	exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "port", fmt.Sprintf("%d", port)).Run()
	return nil
}

func (p *ProxyManager) clearLinuxProxy() error {
	return exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none").Run()
}

func (p *ProxyManager) getLinuxProxy() (*ProxySettings, error) {
	return &ProxySettings{}, nil
}
