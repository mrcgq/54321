//go:build !windows
// +build !windows

package dns

import (
	"fmt"
)

// TUNManager 非Windows平台TUN管理器
type TUNManager struct {
	tunName string
	isUp    bool
}

// NewTUNManager 创建TUN管理器
func NewTUNManager(tunName string) *TUNManager {
	return &TUNManager{
		tunName: tunName,
	}
}

// IsAdministrator 检查是否有root权限
func (t *TUNManager) IsAdministrator() bool {
	// Unix系统检查UID
	return false // 简化实现
}

// CheckWintunDriver 非Windows无需检查
func (t *TUNManager) CheckWintunDriver(exeDir string) bool {
	return true
}

// SetupTUN 配置TUN
func (t *TUNManager) SetupTUN(tunIP, gateway string, mtu int) error {
	return fmt.Errorf("TUN模式在当前平台暂不支持")
}

// AddRoute 添加路由
func (t *TUNManager) AddRoute(destination, mask, gateway string) error {
	return fmt.Errorf("暂不支持")
}

// DeleteRoute 删除路由
func (t *TUNManager) DeleteRoute(destination, mask string) error {
	return fmt.Errorf("暂不支持")
}

// SetupDefaultRoute 设置默认路由
func (t *TUNManager) SetupDefaultRoute(tunGateway string, excludeIPs []string) error {
	return fmt.Errorf("暂不支持")
}

// GetDefaultGateway 获取默认网关
func (t *TUNManager) GetDefaultGateway() (string, error) {
	return "", fmt.Errorf("暂不支持")
}

// RestoreRoute 恢复路由
func (t *TUNManager) RestoreRoute(originalGateway string) error {
	return fmt.Errorf("暂不支持")
}

// SetDNSForInterface 设置DNS
func (t *TUNManager) SetDNSForInterface(dns []string) error {
	return fmt.Errorf("暂不支持")
}

// FlushDNSCache 刷新DNS缓存
func (t *TUNManager) FlushDNSCache() error {
	return fmt.Errorf("暂不支持")
}
