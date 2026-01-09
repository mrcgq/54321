package system

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// =============================================================================
// 系统工具函数
// =============================================================================

// GetAppDataDir 获取应用数据目录
func GetAppDataDir(appName string) (string, error) {
	var baseDir string

	switch runtime.GOOS {
	case "windows":
		baseDir = os.Getenv("APPDATA")
	case "darwin":
		baseDir = filepath.Join(os.Getenv("HOME"), "Library", "Application Support")
	case "linux":
		baseDir = filepath.Join(os.Getenv("HOME"), ".config")
	default:
		return "", fmt.Errorf("不支持的操作系统")
	}

	appDir := filepath.Join(baseDir, appName)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", err
	}

	return appDir, nil
}

// GetExeDir 获取可执行文件所在目录
func GetExeDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
}

// IsAdmin 检查是否以管理员权限运行
func IsAdmin() bool {
	switch runtime.GOOS {
	case "windows":
		return isAdminWindows()
	default:
		return os.Getuid() == 0
	}
}

// isAdminWindows Windows管理员检查
func isAdminWindows() bool {
	cmd := exec.Command("net", "session")
	err := cmd.Run()
	return err == nil
}

// OpenURL 使用默认浏览器打开URL
func OpenURL(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("不支持的操作系统")
	}

	return cmd.Start()
}

// OpenFolder 打开文件夹
func OpenFolder(path string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	default:
		return fmt.Errorf("不支持的操作系统")
	}

	return cmd.Start()
}

// GetLocalIP 获取本地IP地址
func GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	return "127.0.0.1", nil
}

// GetNetworkInterfaces 获取网络接口列表
func GetNetworkInterfaces() ([]NetworkInterface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var result []NetworkInterface
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue // 跳过回环接口
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		var ips []string
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				ips = append(ips, ipnet.IP.String())
			}
		}

		result = append(result, NetworkInterface{
			Name:  iface.Name,
			MAC:   iface.HardwareAddr.String(),
			IPs:   ips,
			IsUp:  iface.Flags&net.FlagUp != 0,
			MTU:   iface.MTU,
		})
	}

	return result, nil
}

// NetworkInterface 网络接口信息
type NetworkInterface struct {
	Name  string   `json:"name"`
	MAC   string   `json:"mac"`
	IPs   []string `json:"ips"`
	IsUp  bool     `json:"is_up"`
	MTU   int      `json:"mtu"`
}

// IsPortAvailable 检查端口是否可用
func IsPortAvailable(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// FindAvailablePort 查找可用端口
func FindAvailablePort(start, end int) (int, error) {
	for port := start; port <= end; port++ {
		if IsPortAvailable(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("在 %d-%d 范围内没有可用端口", start, end)
}

// GetSystemInfo 获取系统信息
func GetSystemInfo() SystemInfo {
	hostname, _ := os.Hostname()

	return SystemInfo{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Hostname: hostname,
		NumCPU:   runtime.NumCPU(),
		GoVersion: runtime.Version(),
	}
}

// SystemInfo 系统信息
type SystemInfo struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Hostname  string `json:"hostname"`
	NumCPU    int    `json:"num_cpu"`
	GoVersion string `json:"go_version"`
}

// SanitizePath 清理路径中的危险字符
func SanitizePath(path string) string {
	// 移除可能的路径遍历攻击
	path = strings.ReplaceAll(path, "..", "")
	path = strings.ReplaceAll(path, "~", "")
	return filepath.Clean(path)
}
