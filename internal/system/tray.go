package system

import (
	"sync"
)

// =============================================================================
// 系统托盘管理
// =============================================================================

// TrayManager 系统托盘管理器
type TrayManager struct {
	mu          sync.RWMutex
	isVisible   bool
	tooltip     string
	menuItems   []TrayMenuItem
	onClick     func()
	onDblClick  func()
}

// TrayMenuItem 托盘菜单项
type TrayMenuItem struct {
	ID       string
	Label    string
	Enabled  bool
	Checked  bool
	OnClick  func()
	SubMenu  []TrayMenuItem
}

// NewTrayManager 创建托盘管理器
func NewTrayManager() *TrayManager {
	return &TrayManager{
		isVisible: true,
		tooltip:   "Xlink 客户端",
	}
}

// SetTooltip 设置托盘提示文字
func (t *TrayManager) SetTooltip(tooltip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tooltip = tooltip
}

// SetMenuItems 设置菜单项
func (t *TrayManager) SetMenuItems(items []TrayMenuItem) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.menuItems = items
}

// SetOnClick 设置单击回调
func (t *TrayManager) SetOnClick(handler func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onClick = handler
}

// SetOnDoubleClick 设置双击回调
func (t *TrayManager) SetOnDoubleClick(handler func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onDblClick = handler
}

// Show 显示托盘图标
func (t *TrayManager) Show() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.isVisible = true
}

// Hide 隐藏托盘图标
func (t *TrayManager) Hide() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.isVisible = false
}

// UpdateStatus 更新状态图标
func (t *TrayManager) UpdateStatus(isRunning bool, nodeCount int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if isRunning {
		t.tooltip = "Xlink 客户端 - 运行中"
		if nodeCount > 0 {
			t.tooltip = "Xlink 客户端 - " + string(rune(nodeCount)) + " 个节点运行中"
		}
	} else {
		t.tooltip = "Xlink 客户端 - 已停止"
	}
}
