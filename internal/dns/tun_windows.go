//go:build windows
// +build windows

package dns

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// =============================================================================
// Windows TUN 管理
// =============================================================================

// TUNManager Windows TUN管理器
type TUNManager struct {
	tunName string
	isUp    bool
}

// NewTUNManager 创建TUN管理器
func NewTUNManager(tunName string) *TUNManager {
	if tunName == "" {
		tunName = DefaultTUNName
	}
	return &TUNManager{
		tunName: tunName,
	}
}

// IsAdministrator 检查是否以管理员身份运行
func (t *TUNManager) IsAdministrator() bool {
	cmd := exec.Command("net", "session")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err := cmd.Run()
	return err == nil
}

// CheckWintunDriver 检查wintun驱动是否存在
func (t *TUNManager) CheckWintunDriver(exeDir string) bool {
	// 检查wintun.dll是否存在
	paths := []string{
		exeDir + "\\wintun.dll",
		"C:\\Windows\\System32\\wintun.dll",
	}

	for _, p := range paths {
		if fileExists(p) {
			return true
		}
	}

	return false
}

// SetupTUN 配置TUN网卡
func (t *TUNManager) SetupTUN(tunIP, gateway string, mtu int) error {
	if !t.IsAdministrator() {
		return fmt.Errorf("需要管理员权限")
	}

	// 配置IP地址
	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%s", t.tunName),
		"source=static",
		fmt.Sprintf("addr=%s", tunIP),
		"mask=255.255.0.0",
		fmt.Sprintf("gateway=%s", gateway),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("配置TUN IP失败: %v", err)
	}

	// 设置MTU
	cmd = exec.Command("netsh", "interface", "ipv4", "set", "subinterface",
		t.tunName,
		fmt.Sprintf("mtu=%d", mtu),
		"store=persistent",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Run() // 忽略错误

	t.isUp = true
	return nil
}

// AddRoute 添加路由
func (t *TUNManager) AddRoute(destination, mask, gateway string) error {
	cmd := exec.Command("route", "add", destination, "mask", mask, gateway, "metric", "1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// DeleteRoute 删除路由
func (t *TUNManager) DeleteRoute(destination, mask string) error {
	cmd := exec.Command("route", "delete", destination, "mask", mask)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// SetupDefaultRoute 设置默认路由走TUN
func (t *TUNManager) SetupDefaultRoute(tunGateway string, excludeIPs []string) error {
	// 先获取原始默认网关
	originalGateway, err := t.GetDefaultGateway()
	if err != nil {
		return err
	}

	// 为排除的IP添加直连路由
	for _, ip := range excludeIPs {
		t.AddRoute(ip, "255.255.255.255", originalGateway)
	}

	// 删除原始默认路由
	t.DeleteRoute("0.0.0.0", "0.0.0.0")

	// 添加新的默认路由
	return t.AddRoute("0.0.0.0", "0.0.0.0", tunGateway)
}

// GetDefaultGateway 获取默认网关
func (t *TUNManager) GetDefaultGateway() (string, error) {
	cmd := exec.Command("route", "print", "0.0.0.0")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// 解析输出找到网关
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "0.0.0.0") && !strings.Contains(line, "On-link") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				return fields[2], nil
			}
		}
	}

	return "", fmt.Errorf("未找到默认网关")
}

// RestoreRoute 恢复原始路由
func (t *TUNManager) RestoreRoute(originalGateway string) error {
	// 删除TUN路由
	t.DeleteRoute("0.0.0.0", "0.0.0.0")

	// 恢复原始默认路由
	return t.AddRoute("0.0.0.0", "0.0.0.0", originalGateway)
}

// SetDNSForInterface 为TUN接口设置DNS
func (t *TUNManager) SetDNSForInterface(dns []string) error {
	if len(dns) == 0 {
		return nil
	}

	cmd := exec.Command("netsh", "interface", "ip", "set", "dns",
		fmt.Sprintf("name=%s", t.tunName),
		"source=static",
		fmt.Sprintf("addr=%s", dns[0]),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := cmd.Run(); err != nil {
		return err
	}

	// 添加备用DNS
	for i := 1; i < len(dns); i++ {
		cmd = exec.Command("netsh", "interface", "ip", "add", "dns",
			fmt.Sprintf("name=%s", t.tunName),
			fmt.Sprintf("addr=%s", dns[i]),
			fmt.Sprintf("index=%d", i+1),
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		cmd.Run()
	}

	return nil
}

// FlushDNSCache 刷新DNS缓存
func (t *TUNManager) FlushDNSCache() error {
	cmd := exec.Command("ipconfig", "/flushdns")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	_, err := syscall.GetFileAttributes(syscall.StringToUTF16Ptr(path))
	return err == nil
}
