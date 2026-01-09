// Package main 包含应用主逻辑和前端绑定
package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"xlink-wails/internal/config"
	"xlink-wails/internal/models"
)

// App 主应用结构
type App struct {
	ctx   context.Context
	state *models.AppState

	// 配置管理器 [步骤2新增]
	configManager *config.Manager

	// 日志缓冲
	logBuffer   []models.LogEntry
	logBufferMu sync.Mutex

	// 取消函数（用于关闭时清理）
	cancelFuncs []context.CancelFunc
	cancelMu    sync.Mutex
}

// NewApp 创建新的应用实例
func NewApp() *App {
	return &App{
		state:     models.NewAppState(),
		logBuffer: make([]models.LogEntry, 0, 1000),
	}
}

// =============================================================================
// 生命周期方法
// =============================================================================

// startup 应用启动时调用 [步骤2修改]
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 初始化配置管理器 [步骤2新增]
	a.configManager = config.NewManager(a.state.ExeDir)

	// 加载配置
	a.loadConfig()

	// 如果是自动启动，启动所有节点
	if a.state.IsAutoStart {
		go func() {
			time.Sleep(time.Second) // 等待UI就绪
			a.StartAllNodes()
		}()
	}

	// 启动日志刷新定时器
	go a.logFlushLoop()

	a.appendSystemLog("info", "系统", "Xlink 客户端已启动 v"+models.AppVersion)
}

// shutdown 应用关闭时调用
func (a *App) shutdown(ctx context.Context) {
	a.appendSystemLog("info", "系统", "正在关闭...")

	// 停止所有节点
	a.StopAllNodes()

	// 保存配置
	a.saveConfig()

	// 取消所有后台任务
	a.cancelMu.Lock()
	for _, cancel := range a.cancelFuncs {
		cancel()
	}
	a.cancelMu.Unlock()
}

// =============================================================================
// 窗口控制
// =============================================================================

// ShowWindow 显示主窗口
func (a *App) ShowWindow() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
}

// HideWindow 隐藏主窗口（最小化到托盘）
func (a *App) HideWindow() {
	runtime.WindowHide(a.ctx)
}

// Quit 退出应用
func (a *App) Quit() {
	runtime.Quit(a.ctx)
}

// =============================================================================
// 节点管理 API（供前端调用）
// =============================================================================

// GetNodes 获取所有节点列表
func (a *App) GetNodes() []models.NodeConfig {
	a.state.mu.RLock()
	defer a.state.mu.RUnlock()

	// 返回副本，包含状态信息
	nodes := make([]models.NodeConfig, len(a.state.Config.Nodes))
	copy(nodes, a.state.Config.Nodes)

	// 填充运行状态
	for i := range nodes {
		if es, ok := a.state.EngineStatuses[nodes[i].ID]; ok {
			nodes[i].Status = es.Status
		} else {
			nodes[i].Status = models.StatusStopped
		}
	}

	return nodes
}

// GetNode 获取单个节点
func (a *App) GetNode(id string) *models.NodeConfig {
	return a.state.GetNode(id)
}

// AddNode 添加新节点
func (a *App) AddNode(name string) (*models.NodeConfig, error) {
	a.state.mu.Lock()
	defer a.state.mu.Unlock()

	if len(a.state.Config.Nodes) >= models.MaxNodes {
		return nil, fmt.Errorf("节点数量已达上限 (%d)", models.MaxNodes)
	}

	node := models.NewDefaultNode(name)
	a.state.Config.Nodes = append(a.state.Config.Nodes, node)

	go a.saveConfig()
	a.emitEvent(models.EventConfigChanged, nil)

	return &node, nil
}

// UpdateNode 更新节点配置
func (a *App) UpdateNode(node models.NodeConfig) error {
	a.state.mu.Lock()
	defer a.state.mu.Unlock()

	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == node.ID {
			// 保留运行时状态
			node.Status = a.state.Config.Nodes[i].Status
			node.InternalPort = a.state.Config.Nodes[i].InternalPort
			a.state.Config.Nodes[i] = node

			go a.saveConfig()
			a.emitEvent(models.EventConfigChanged, nil)
			return nil
		}
	}

	return fmt.Errorf("节点不存在: %s", node.ID)
}

// DeleteNode 删除节点
func (a *App) DeleteNode(id string) error {
	a.state.mu.Lock()
	defer a.state.mu.Unlock()

	// 检查是否正在运行
	if es, ok := a.state.EngineStatuses[id]; ok && es.Status == models.StatusRunning {
		return fmt.Errorf("请先停止节点再删除")
	}

	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == id {
			a.state.Config.Nodes = append(
				a.state.Config.Nodes[:i],
				a.state.Config.Nodes[i+1:]...,
			)

			delete(a.state.EngineStatuses, id)

			go a.saveConfig()
			a.emitEvent(models.EventConfigChanged, nil)
			return nil
		}
	}

	return fmt.Errorf("节点不存在: %s", id)
}

// DuplicateNode 复制节点
func (a *App) DuplicateNode(id string) (*models.NodeConfig, error) {
	a.state.mu.Lock()
	defer a.state.mu.Unlock()

	if len(a.state.Config.Nodes) >= models.MaxNodes {
		return nil, fmt.Errorf("节点数量已达上限")
	}

	var srcNode *models.NodeConfig
	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == id {
			srcNode = &a.state.Config.Nodes[i]
			break
		}
	}

	if srcNode == nil {
		return nil, fmt.Errorf("节点不存在: %s", id)
	}

	// 创建副本
	newNode := *srcNode
	newNode.ID = models.GenerateUUID()
	newNode.Name = srcNode.Name + " (副本)"
	newNode.Status = models.StatusStopped

	// 深拷贝规则
	newNode.Rules = make([]models.RoutingRule, len(srcNode.Rules))
	copy(newNode.Rules, srcNode.Rules)

	a.state.Config.Nodes = append(a.state.Config.Nodes, newNode)

	go a.saveConfig()
	a.emitEvent(models.EventConfigChanged, nil)

	return &newNode, nil
}

// =============================================================================
// 节点控制 API
// =============================================================================

// StartNode 启动指定节点
func (a *App) StartNode(id string) error {
	node := a.state.GetNode(id)
	if node == nil {
		return fmt.Errorf("节点不存在: %s", id)
	}

	a.appendNodeLog(id, node.Name, "info", "系统", "正在启动...")

	// TODO: 步骤3实现引擎启动逻辑
	a.state.UpdateNodeStatus(id, models.StatusRunning, "")
	a.emitNodeStatus(id, models.StatusRunning)

	return nil
}

// StopNode 停止指定节点
func (a *App) StopNode(id string) error {
	node := a.state.GetNode(id)
	if node == nil {
		return fmt.Errorf("节点不存在: %s", id)
	}

	a.appendNodeLog(id, node.Name, "info", "系统", "正在停止...")

	// TODO: 步骤3实现引擎停止逻辑
	a.state.UpdateNodeStatus(id, models.StatusStopped, "")
	a.emitNodeStatus(id, models.StatusStopped)

	return nil
}

// StartAllNodes 启动所有节点
func (a *App) StartAllNodes() error {
	a.state.mu.RLock()
	nodes := make([]models.NodeConfig, len(a.state.Config.Nodes))
	copy(nodes, a.state.Config.Nodes)
	a.state.mu.RUnlock()

	for _, node := range nodes {
		if err := a.StartNode(node.ID); err != nil {
			a.appendSystemLog("error", "系统", fmt.Sprintf("启动节点 %s 失败: %v", node.Name, err))
		}
	}

	return nil
}

// StopAllNodes 停止所有节点
func (a *App) StopAllNodes() error {
	a.state.mu.RLock()
	nodes := make([]models.NodeConfig, len(a.state.Config.Nodes))
	copy(nodes, a.state.Config.Nodes)
	a.state.mu.RUnlock()

	for _, node := range nodes {
		if es, ok := a.state.EngineStatuses[node.ID]; ok && es.Status == models.StatusRunning {
			if err := a.StopNode(node.ID); err != nil {
				a.appendSystemLog("error", "系统", fmt.Sprintf("停止节点 %s 失败: %v", node.Name, err))
			}
		}
	}

	return nil
}

// PingTest 延迟测试
func (a *App) PingTest(id string) error {
	node := a.state.GetNode(id)
	if node == nil {
		return fmt.Errorf("节点不存在: %s", id)
	}

	a.appendNodeLog(id, node.Name, "info", "测速", "正在启动延迟测试...")

	// TODO: 步骤5实现测速逻辑

	return nil
}

// =============================================================================
// 规则管理 API
// =============================================================================

// AddRule 添加分流规则
func (a *App) AddRule(nodeID string, rule models.RoutingRule) error {
	a.state.mu.Lock()
	defer a.state.mu.Unlock()

	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == nodeID {
			if len(a.state.Config.Nodes[i].Rules) >= models.MaxRules {
				return fmt.Errorf("规则数量已达上限 (%d)", models.MaxRules)
			}

			rule.ID = models.GenerateUUID()
			a.state.Config.Nodes[i].Rules = append(a.state.Config.Nodes[i].Rules, rule)

			go a.saveConfig()
			return nil
		}
	}

	return fmt.Errorf("节点不存在: %s", nodeID)
}

// UpdateRule 更新分流规则
func (a *App) UpdateRule(nodeID string, rule models.RoutingRule) error {
	a.state.mu.Lock()
	defer a.state.mu.Unlock()

	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == nodeID {
			for j := range a.state.Config.Nodes[i].Rules {
				if a.state.Config.Nodes[i].Rules[j].ID == rule.ID {
					a.state.Config.Nodes[i].Rules[j] = rule
					go a.saveConfig()
					return nil
				}
			}
			return fmt.Errorf("规则不存在: %s", rule.ID)
		}
	}

	return fmt.Errorf("节点不存在: %s", nodeID)
}

// DeleteRule 删除分流规则
func (a *App) DeleteRule(nodeID, ruleID string) error {
	a.state.mu.Lock()
	defer a.state.mu.Unlock()

	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == nodeID {
			rules := a.state.Config.Nodes[i].Rules
			for j := range rules {
				if rules[j].ID == ruleID {
					a.state.Config.Nodes[i].Rules = append(rules[:j], rules[j+1:]...)
					go a.saveConfig()
					return nil
				}
			}
			return fmt.Errorf("规则不存在: %s", ruleID)
		}
	}

	return fmt.Errorf("节点不存在: %s", nodeID)
}

// =============================================================================
// 导入导出 API [步骤2完整实现]
// =============================================================================

// ImportFromClipboard 从剪贴板导入
func (a *App) ImportFromClipboard() (int, error) {
	text, err := runtime.ClipboardGetText(a.ctx)
	if err != nil {
		return 0, fmt.Errorf("读取剪贴板失败: %v", err)
	}

	imported, err := a.configManager.ImportNodes(text)
	if err != nil {
		return 0, err
	}

	// 更新状态
	a.state.mu.Lock()
	a.state.Config = a.configManager.GetConfig()
	a.state.mu.Unlock()

	// 保存并通知前端
	go a.saveConfig()
	a.emitEvent(models.EventConfigChanged, nil)

	a.appendSystemLog("info", "系统", fmt.Sprintf("成功导入 %d 个节点", len(imported)))

	return len(imported), nil
}

// ExportToClipboard 导出节点到剪贴板
func (a *App) ExportToClipboard(id string) error {
	uri, err := a.configManager.ExportNode(id)
	if err != nil {
		return err
	}

	if err := runtime.ClipboardSetText(a.ctx, uri); err != nil {
		return fmt.Errorf("写入剪贴板失败: %v", err)
	}

	a.appendSystemLog("info", "系统", "配置已复制到剪贴板")
	return nil
}

// ExportAllToClipboard 导出所有节点到剪贴板 [步骤2新增]
func (a *App) ExportAllToClipboard() error {
	a.state.mu.RLock()
	nodes := a.state.Config.Nodes
	a.state.mu.RUnlock()

	var uris []string
	for _, node := range nodes {
		uri, err := a.configManager.ExportNode(node.ID)
		if err == nil {
			uris = append(uris, uri)
		}
	}

	if len(uris) == 0 {
		return fmt.Errorf("没有可导出的节点")
	}

	text := strings.Join(uris, "\n")
	if err := runtime.ClipboardSetText(a.ctx, text); err != nil {
		return fmt.Errorf("写入剪贴板失败: %v", err)
	}

	a.appendSystemLog("info", "系统", fmt.Sprintf("已导出 %d 个节点到剪贴板", len(uris)))
	return nil
}

// =============================================================================
// 备份管理 API [步骤2新增]
// =============================================================================

// ListBackups 列出所有备份
func (a *App) ListBackups() []string {
	return a.configManager.ListBackups()
}

// RestoreBackup 从备份恢复
func (a *App) RestoreBackup(backupName string) error {
	if err := a.configManager.RestoreBackup(backupName); err != nil {
		return err
	}

	// 重新加载配置
	a.state.mu.Lock()
	a.state.Config = a.configManager.GetConfig()
	a.state.mu.Unlock()

	a.emitEvent(models.EventConfigChanged, nil)
	a.appendSystemLog("info", "系统", fmt.Sprintf("已从备份恢复: %s", backupName))

	return nil
}

// =============================================================================
// 设置 API
// =============================================================================

// GetSettings 获取应用设置
func (a *App) GetSettings() models.AppConfig {
	a.state.mu.RLock()
	defer a.state.mu.RUnlock()
	return *a.state.Config
}

// UpdateSettings 更新应用设置
func (a *App) UpdateSettings(cfg models.AppConfig) error {
	a.state.mu.Lock()
	// 保留节点列表
	cfg.Nodes = a.state.Config.Nodes
	a.state.Config = &cfg
	a.state.mu.Unlock()

	go a.saveConfig()
	return nil
}

// SetAutoStart 设置开机自启
func (a *App) SetAutoStart(enabled bool) error {
	// TODO: 步骤9实现
	a.state.mu.Lock()
	a.state.Config.AutoStart = enabled
	a.state.mu.Unlock()

	go a.saveConfig()
	return nil
}

// GetAutoStart 获取开机自启状态
func (a *App) GetAutoStart() bool {
	a.state.mu.RLock()
	defer a.state.mu.RUnlock()
	return a.state.Config.AutoStart
}

// =============================================================================
// 配置文件路径 API [步骤2新增]
// =============================================================================

// GetConfigPath 获取配置文件路径
func (a *App) GetConfigPath() string {
	return filepath.Join(a.state.ExeDir, config.ConfigFileName)
}

// OpenConfigFolder 打开配置文件所在文件夹
func (a *App) OpenConfigFolder() error {
	return runtime.BrowserOpenURL(a.ctx, a.state.ExeDir)
}

// =============================================================================
// 日志系统
// =============================================================================

// GetLogs 获取日志（分页）
func (a *App) GetLogs(limit int) []models.LogEntry {
	a.logBufferMu.Lock()
	defer a.logBufferMu.Unlock()

	if limit <= 0 || limit > len(a.logBuffer) {
		limit = len(a.logBuffer)
	}

	start := len(a.logBuffer) - limit
	if start < 0 {
		start = 0
	}

	result := make([]models.LogEntry, limit)
	copy(result, a.logBuffer[start:])

	return result
}

// ClearLogs 清空日志
func (a *App) ClearLogs() {
	a.logBufferMu.Lock()
	a.logBuffer = a.logBuffer[:0]
	a.logBufferMu.Unlock()
}

// appendSystemLog 追加系统日志
func (a *App) appendSystemLog(level, category, message string) {
	a.appendLog(models.LogEntry{
		Timestamp: time.Now(),
		NodeID:    "",
		NodeName:  "系统",
		Level:     level,
		Category:  category,
		Message:   message,
	})
}

// appendNodeLog 追加节点日志
func (a *App) appendNodeLog(nodeID, nodeName, level, category, message string) {
	a.appendLog(models.LogEntry{
		Timestamp: time.Now(),
		NodeID:    nodeID,
		NodeName:  nodeName,
		Level:     level,
		Category:  category,
		Message:   message,
	})
}

// appendLog 追加日志到缓冲区
func (a *App) appendLog(entry models.LogEntry) {
	a.logBufferMu.Lock()
	defer a.logBufferMu.Unlock()

	// 限制缓冲区大小
	if len(a.logBuffer) >= 10000 {
		a.logBuffer = a.logBuffer[1000:]
	}

	a.logBuffer = append(a.logBuffer, entry)
}

// logFlushLoop 定期刷新日志到前端
func (a *App) logFlushLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	lastLen := 0
	for {
		select {
		case <-ticker.C:
			a.logBufferMu.Lock()
			currentLen := len(a.logBuffer)
			if currentLen > lastLen {
				// 发送新日志
				newLogs := a.logBuffer[lastLen:]
				for _, log := range newLogs {
					a.emitEvent(models.EventLogAppend, log)
				}
				lastLen = currentLen
			}
			a.logBufferMu.Unlock()
		case <-a.ctx.Done():
			return
		}
	}
}

// =============================================================================
// 事件系统
// =============================================================================

// emitEvent 发送事件到前端
func (a *App) emitEvent(eventType models.EventType, payload interface{}) {
	runtime.EventsEmit(a.ctx, string(eventType), payload)
}

// emitNodeStatus 发送节点状态更新
func (a *App) emitNodeStatus(nodeID, status string) {
	a.emitEvent(models.EventNodeStatus, map[string]string{
		"node_id": nodeID,
		"status":  status,
	})
}

// =============================================================================
// 配置持久化 [步骤2完整实现]
// =============================================================================

// loadConfig 加载配置
func (a *App) loadConfig() {
	cfg, err := a.configManager.Load()
	if err != nil {
		a.appendSystemLog("error", "系统", fmt.Sprintf("加载配置失败: %v", err))
		// 使用默认配置
		cfg = &models.AppConfig{
			Nodes: []models.NodeConfig{
				models.NewDefaultNode("默认节点"),
			},
			Theme:    "system",
			Language: "zh-CN",
		}
	}

	a.state.mu.Lock()
	a.state.Config = cfg
	a.state.mu.Unlock()

	a.appendSystemLog("info", "系统", fmt.Sprintf("已加载 %d 个节点配置", len(cfg.Nodes)))
}

// saveConfig 保存配置
func (a *App) saveConfig() {
	a.state.mu.RLock()
	a.configManager.UpdateConfig(a.state.Config)
	a.state.mu.RUnlock()

	if err := a.configManager.Save(); err != nil {
		a.appendSystemLog("error", "系统", fmt.Sprintf("保存配置失败: %v", err))
	}
}

// =============================================================================
// 版本信息
// =============================================================================

// GetVersion 获取版本信息
func (a *App) GetVersion() string {
	return models.AppVersion
}

// GetAppTitle 获取应用标题
func (a *App) GetAppTitle() string {
	return models.AppTitle
}
