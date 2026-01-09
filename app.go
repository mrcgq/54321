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
	"xlink-wails/internal/dns"
	"xlink-wails/internal/engine"
	"xlink-wails/internal/generator"
	"xlink-wails/internal/logger"
	"xlink-wails/internal/models"
	"xlink-wails/internal/system"
)

// App 主应用结构
type App struct {
	ctx   context.Context
	state *models.AppState

	// 管理器
	configManager   *config.Manager
	configGenerator *generator.Generator
	engineManager   *engine.Manager
	logManager      *logger.Manager
	pingManager     *logger.PingManager
	dnsManager      *dns.Manager
	tunManager      *dns.TUNManager
	leakTester      *dns.LeakTester
	autoStart       *system.AutoStartManager
	notification    *system.NotificationManager
	proxyManager    *system.ProxyManager

	// 取消函数（用于关闭时清理后台任务）
	cancelFuncs []context.CancelFunc
	cancelMu    sync.Mutex
}

// NewApp 创建新的应用实例
func NewApp() *App {
	return &App{
		state: models.NewAppState(),
	}
}

// =============================================================================
// 生命周期方法
// =============================================================================

// startup 应用启动时调用
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 1. 初始化日志管理器（最先初始化以便记录启动日志）
	a.logManager = logger.NewManager(a.state.ExeDir)
	a.logManager.SetCallback(func(entry models.LogEntry) {
		// 实时发送日志到前端
		runtime.EventsEmit(a.ctx, string(models.EventLogAppend), entry)
	})

	a.logManager.LogSystem(logger.LevelInfo, "Xlink 客户端正在启动 v"+models.AppVersion+"...")

	// 2. 初始化各子模块
	a.pingManager = logger.NewPingManager(a.state.ExeDir, a.logManager)
	a.configManager = config.NewManager(a.state.ExeDir)
	a.configGenerator = generator.NewGenerator(a.state.ExeDir)
	a.engineManager = engine.NewManager(a.state.ExeDir)
	a.dnsManager = dns.NewManager(a.state.ExeDir)
	a.leakTester = dns.NewLeakTester()
	a.proxyManager = system.NewProxyManager()
	a.notification = system.NewNotificationManager(models.AppTitle)

	// 初始化 TUN 管理器 (优先使用配置中的接口名，否则默认)
	tunName := "XlinkTUN"
	// 注意：此时配置可能还没加载，稍后加载配置后可能需要再次确认，但在NewTUNManager中主要是结构体初始化
	a.tunManager = dns.NewTUNManager(tunName)

	// 初始化自启动管理器
	var err error
	a.autoStart, err = system.NewAutoStartManager("XlinkClient")
	if err != nil {
		a.logManager.LogSystem(logger.LevelWarn, fmt.Sprintf("自启动管理器初始化失败: %v", err))
	}

	// 3. 设置引擎回调
	// 日志回调：将引擎日志转发到统一日志系统
	a.engineManager.SetLogCallback(func(nodeID, nodeName, level, category, message string) {
		a.logManager.LogNode(nodeID, nodeName, level, category, message)
	})

	// 状态回调：处理节点状态变更
	a.engineManager.SetStatusCallback(func(nodeID, status string, err error) {
		a.state.UpdateNodeStatus(nodeID, status, "")
		a.emitNodeStatus(nodeID, status)

		if err != nil {
			node := a.state.GetNode(nodeID)
			nodeName := nodeID
			if node != nil {
				nodeName = node.Name
			}
			a.logManager.LogNode(nodeID, nodeName, logger.LevelError, logger.CategorySystem, err.Error())
		}
	})

	// 4. 设置 DNS 管理器日志回调
	a.dnsManager.SetLogCallback(func(level, message string) {
		a.logManager.LogSystem(level, message)
	})

	// 5. 加载用户配置
	a.loadConfig()

	// 6. 处理自动启动逻辑
	if a.state.IsAutoStart {
		go func() {
			// 延迟一秒等待系统就绪
			time.Sleep(1 * time.Second)
			if a.state.Config.AutoStart {
				a.logManager.LogSystem(logger.LevelInfo, "触发开机自动启动...")
				a.StartAllNodes()
				a.notification.Show(models.AppTitle, "已自动启动所有节点")
			}
		}()
	}

	a.logManager.LogSystem(logger.LevelInfo, "系统初始化完成")
}

// shutdown 应用关闭时调用
func (a *App) shutdown(ctx context.Context) {
	a.logManager.LogSystem(logger.LevelInfo, "正在关闭应用...")

	// 1. 停止正在进行的 Ping 测试
	if a.pingManager != nil {
		a.pingManager.StopPing()
	}

	// 2. 停止所有运行中的节点引擎
	if a.engineManager != nil {
		a.engineManager.StopAll()
	}

	// 3. 恢复系统代理设置 (防止退出后断网)
	if a.proxyManager != nil {
		if err := a.proxyManager.RestoreSystemProxy(); err != nil {
			a.logManager.LogSystem(logger.LevelError, fmt.Sprintf("恢复系统代理失败: %v", err))
		}
	}

	// 4. 清理临时配置文件
	if a.configGenerator != nil {
		a.configGenerator.CleanupAllConfigs()
	}

	// 5. 保存当前配置
	a.saveConfig()

	// 6. 停止日志管理器 (刷新缓冲区到磁盘)
	if a.logManager != nil {
		a.logManager.Stop()
	}

	// 7. 取消所有上下文
	a.cancelMu.Lock()
	for _, cancel := range a.cancelFuncs {
		cancel()
	}
	a.cancelMu.Unlock()
}

// =============================================================================
// 窗口控制 API
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
// 节点管理 API
// =============================================================================

// GetNodes 获取所有节点列表 (包含运行时状态)
func (a *App) GetNodes() []models.NodeConfig {
	a.state.Mu.RLock()
	defer a.state.Mu.RUnlock()

	// 深拷贝节点列表，避免并发读写问题
	nodes := make([]models.NodeConfig, len(a.state.Config.Nodes))
	copy(nodes, a.state.Config.Nodes)

	// 填充运行时状态
	for i := range nodes {
		if es, ok := a.state.EngineStatuses[nodes[i].ID]; ok {
			nodes[i].Status = es.Status
		} else {
			nodes[i].Status = models.StatusStopped
		}
	}

	return nodes
}

// GetNode 获取单个节点配置
func (a *App) GetNode(id string) *models.NodeConfig {
	return a.state.GetNode(id)
}

// AddNode 添加新节点
func (a *App) AddNode(name string) (*models.NodeConfig, error) {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()

	if len(a.state.Config.Nodes) >= models.MaxNodes {
		return nil, fmt.Errorf("节点数量已达上限 (%d)", models.MaxNodes)
	}

	node := models.NewDefaultNode(name)
	a.state.Config.Nodes = append(a.state.Config.Nodes, node)

	// 异步保存并通知前端
	go a.saveConfig()
	//a.emitEvent(models.EventConfigChanged, nil)

	return &node, nil
}

// UpdateNode 更新节点配置
func (a *App) UpdateNode(node models.NodeConfig) error {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()

	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == node.ID {
			// 保留运行时状态和内部端口
			node.Status = a.state.Config.Nodes[i].Status
			node.InternalPort = a.state.Config.Nodes[i].InternalPort
			a.state.Config.Nodes[i] = node

			go a.saveConfig()
			
			// 🛑【核心修复】注释掉下面这行！
			// 不要在这里广播事件，否则会导致前端死循环刷新！
			// a.emitEvent(models.EventConfigChanged, nil) 
			
			return nil
		}
	}

	return fmt.Errorf("节点不存在: %s", node.ID)
}

// DeleteNode 删除节点
func (a *App) DeleteNode(id string) error {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()

	// 检查节点是否正在运行
	if es, ok := a.state.EngineStatuses[id]; ok && es.Status == models.StatusRunning {
		return fmt.Errorf("请先停止节点再删除")
	}

	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == id {
			// 删除节点
			a.state.Config.Nodes = append(
				a.state.Config.Nodes[:i],
				a.state.Config.Nodes[i+1:]...,
			)

			// 清理状态
			delete(a.state.EngineStatuses, id)

			// 清理关联的临时配置文件
			go a.configGenerator.CleanupConfigs(id)

			go a.saveConfig()
			a.emitEvent(models.EventConfigChanged, nil)
			return nil
		}
	}

	return fmt.Errorf("节点不存在: %s", id)
}

// DuplicateNode 复制节点
func (a *App) DuplicateNode(id string) (*models.NodeConfig, error) {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()

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

	// 深度复制规则切片
	newNode.Rules = make([]models.RoutingRule, len(srcNode.Rules))
	copy(newNode.Rules, srcNode.Rules)

	a.state.Config.Nodes = append(a.state.Config.Nodes, newNode)

	go a.saveConfig()
	a.emitEvent(models.EventConfigChanged, nil)

	return &newNode, nil
}

// =============================================================================
// 节点控制 API (启动/停止/测速)
// =============================================================================

// StartNode 启动指定节点
func (a *App) StartNode(id string) error {
	node := a.state.GetNode(id)
	if node == nil {
		return fmt.Errorf("节点不存在: %s", id)
	}

	a.logManager.LogNode(id, node.Name, logger.LevelInfo, logger.CategorySystem, "正在启动...")

	// 1. 生成配置文件 (包含 Xlink 核心配置和可能的 Xray 配置)
	configPath, err := a.generateNodeConfig(node)
	if err != nil {
		errMsg := fmt.Sprintf("生成配置失败: %v", err)
		a.logManager.LogNode(id, node.Name, logger.LevelError, logger.CategorySystem, errMsg)
		return fmt.Errorf(errMsg)
	}

	// 2. 调用引擎管理器启动进程
	if err := a.engineManager.StartNode(node, configPath); err != nil {
		return err
	}

	return nil
}

// StopNode 停止指定节点
func (a *App) StopNode(id string) error {
	node := a.state.GetNode(id)
	if node == nil {
		return fmt.Errorf("节点不存在: %s", id)
	}

	a.logManager.LogNode(id, node.Name, logger.LevelInfo, logger.CategorySystem, "正在停止...")

	return a.engineManager.StopNode(id)
}

// StartAllNodes 启动所有配置的节点
func (a *App) StartAllNodes() error {
	a.state.Mu.RLock()
	nodes := make([]models.NodeConfig, len(a.state.Config.Nodes))
	copy(nodes, a.state.Config.Nodes)
	a.state.Mu.RUnlock()

	var lastErr error
	for _, node := range nodes {
		if err := a.StartNode(node.ID); err != nil {
			a.logManager.LogSystem(logger.LevelError, fmt.Sprintf("启动节点 %s 失败: %v", node.Name, err))
			lastErr = err
		}
	}

	return lastErr
}

// StopAllNodes 停止所有节点
func (a *App) StopAllNodes() error {
	a.engineManager.StopAll()
	return nil
}

// PingTest 对指定节点进行延迟测试
func (a *App) PingTest(id string) error {
	node := a.state.GetNode(id)
	if node == nil {
		return fmt.Errorf("节点不存在: %s", id)
	}

	a.logManager.LogNode(id, node.Name, logger.LevelInfo, logger.CategoryPing, "正在启动延迟测试...")

	go func() {
		err := a.pingManager.StartPing(
			node,
			func(result models.PingResult) {
				// 单次结果回调
				a.emitEvent(models.EventPingResult, result)
			},
			func(report logger.PingReport) {
				// 完成报告回调
				a.emitEvent(models.EventPingComplete, report)
			},
		)

		if err != nil {
			a.logManager.LogNode(id, node.Name, logger.LevelError, logger.CategoryPing, fmt.Sprintf("测速启动失败: %v", err))
		}
	}()

	return nil
}

// StopPingTest 停止当前正在进行的 Ping 测试
func (a *App) StopPingTest() {
	a.pingManager.StopPing()
}

// BatchPingTest 批量测试所有节点 (Beta)
func (a *App) BatchPingTest() error {
	a.state.Mu.RLock()
	nodes := make([]*models.NodeConfig, len(a.state.Config.Nodes))
	for i := range a.state.Config.Nodes {
		nodes[i] = &a.state.Config.Nodes[i]
	}
	a.state.Mu.RUnlock()

	go func() {
		results := a.pingManager.BatchPing(nodes, func(current, total int, result logger.BatchPingResult) {
			a.emitEvent(models.EventPingBatchProgress, map[string]interface{}{
				"current": current,
				"total":   total,
				"result":  result,
			})
		})
		a.emitEvent(models.EventPingBatchComplete, results)
	}()

	return nil
}

// GetNodeStatus 获取节点运行状态字符串
func (a *App) GetNodeStatus(id string) string {
	return a.engineManager.GetStatus(id)
}

// GetAllNodeStatuses 获取所有节点的详细运行状态
func (a *App) GetAllNodeStatuses() map[string]models.EngineStatus {
	return a.engineManager.GetAllStatuses()
}

// =============================================================================
// 规则管理 API
// =============================================================================

// AddRule 为指定节点添加分流规则
func (a *App) AddRule(nodeID string, rule models.RoutingRule) error {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()

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

// UpdateRule 更新规则
func (a *App) UpdateRule(nodeID string, rule models.RoutingRule) error {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()

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

// DeleteRule 删除规则
func (a *App) DeleteRule(nodeID, ruleID string) error {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()

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
// 预设规则 API (Generator集成)
// =============================================================================

// GetPresetRules 获取指定名称的预设规则列表
func (a *App) GetPresetRules(presetName string) []string {
	return generator.GetPresetRules(presetName)
}

// GetAllPresets 获取所有可用预设名称
func (a *App) GetAllPresets() []string {
	return []string{
		"block-ads",
		"direct-cn",
		"proxy-common",
		"proxy-streaming",
		"privacy",
	}
}

// ApplyPreset 应用预设规则到节点
func (a *App) ApplyPreset(nodeID, presetName string) error {
	rules := generator.GetPresetRules(presetName)
	if rules == nil {
		return fmt.Errorf("预设不存在: %s", presetName)
	}

	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()

	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == nodeID {
			for _, ruleStr := range rules {
				// 简单的 CSV 解析: type:match,target
				parts := strings.SplitN(ruleStr, ",", 2)
				if len(parts) != 2 {
					continue
				}

				rule := models.RoutingRule{
					ID:     models.GenerateUUID(),
					Target: parts[1],
				}

				left := parts[0]
				switch {
				case strings.HasPrefix(left, "geosite:"):
					rule.Type = "geosite:"
					rule.Match = strings.TrimPrefix(left, "geosite:")
				case strings.HasPrefix(left, "geoip:"):
					rule.Type = "geoip:"
					rule.Match = strings.TrimPrefix(left, "geoip:")
				default:
					rule.Type = ""
					rule.Match = left
				}

				a.state.Config.Nodes[i].Rules = append(a.state.Config.Nodes[i].Rules, rule)
			}

			go a.saveConfig()
			return nil
		}
	}

	return fmt.Errorf("节点不存在: %s", nodeID)
}

// =============================================================================
// 导入导出 API
// =============================================================================

// ImportFromClipboard 从剪贴板导入节点 (支持 xlink:// 协议)
func (a *App) ImportFromClipboard() (int, error) {
	text, err := runtime.ClipboardGetText(a.ctx)
	if err != nil {
		return 0, fmt.Errorf("读取剪贴板失败: %v", err)
	}

	imported, err := a.configManager.ImportNodes(text)
	if err != nil {
		return 0, err
	}

	a.state.Mu.Lock()
	a.state.Config = a.configManager.GetConfig()
	a.state.Mu.Unlock()

	go a.saveConfig()
	a.emitEvent(models.EventConfigChanged, nil)

	a.logManager.LogSystem(logger.LevelInfo, fmt.Sprintf("成功导入 %d 个节点", len(imported)))

	return len(imported), nil
}

// ExportToClipboard 导出单个节点到剪贴板
func (a *App) ExportToClipboard(id string) error {
	uri, err := a.configManager.ExportNode(id)
	if err != nil {
		return err
	}

	if err := runtime.ClipboardSetText(a.ctx, uri); err != nil {
		return fmt.Errorf("写入剪贴板失败: %v", err)
	}

	a.logManager.LogSystem(logger.LevelInfo, "配置已复制到剪贴板")
	return nil
}

// ExportAllToClipboard 导出所有节点到剪贴板
func (a *App) ExportAllToClipboard() error {
	a.state.Mu.RLock()
	nodes := a.state.Config.Nodes
	a.state.Mu.RUnlock()

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

	a.logManager.LogSystem(logger.LevelInfo, fmt.Sprintf("已导出 %d 个节点到剪贴板", len(uris)))
	return nil
}

// =============================================================================
// 备份管理 API
// =============================================================================

// ListBackups 列出所有配置文件备份
func (a *App) ListBackups() []string {
	return a.configManager.ListBackups()
}

// RestoreBackup 从备份恢复配置
func (a *App) RestoreBackup(backupName string) error {
	if err := a.configManager.RestoreBackup(backupName); err != nil {
		return err
	}

	// 重新加载到内存
	a.state.Mu.Lock()
	a.state.Config = a.configManager.GetConfig()
	a.state.Mu.Unlock()

	a.emitEvent(models.EventConfigChanged, nil)
	a.logManager.LogSystem(logger.LevelInfo, fmt.Sprintf("已从备份恢复: %s", backupName))

	return nil
}

// =============================================================================
// 设置管理 API
// =============================================================================

// GetSettings 获取全局设置
func (a *App) GetSettings() models.AppConfig {
	a.state.Mu.RLock()
	defer a.state.Mu.RUnlock()
	return *a.state.Config
}

// UpdateSettings 更新全局设置
func (a *App) UpdateSettings(cfg models.AppConfig) error {
	a.state.Mu.Lock()
	// 保持节点列表不变，只更新设置项
	cfg.Nodes = a.state.Config.Nodes
	a.state.Config = &cfg
	a.state.Mu.Unlock()

	go a.saveConfig()
	return nil
}

// SetAutoStart 设置开机自启动
func (a *App) SetAutoStart(enabled bool) error {
	if a.autoStart == nil {
		return fmt.Errorf("自动启动管理器未初始化")
	}

	var err error
	if enabled {
		err = a.autoStart.Enable()
	} else {
		err = a.autoStart.Disable()
	}

	if err != nil {
		return err
	}

	a.state.Mu.Lock()
	a.state.Config.AutoStart = enabled
	a.state.Mu.Unlock()

	go a.saveConfig()
	return nil
}

// GetAutoStart 获取当前开机自启状态
func (a *App) GetAutoStart() bool {
	if a.autoStart == nil {
		return false
	}
	return a.autoStart.IsEnabled()
}

// =============================================================================
// DNS 防泄露 API
// =============================================================================

// GetDNSModes 获取支持的 DNS 模式列表
func (a *App) GetDNSModes() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"value":       models.DNSModeStandard,
			"label":       "标准模式",
			"description": "使用系统默认DNS，可能泄露",
			"recommended": false,
		},
		{
			"value":       models.DNSModeFakeIP,
			"label":       "Fake-IP 模式",
			"description": "本地返回虚假IP，域名通过代理解析，有效防止泄露",
			"recommended": true,
		},
		{
			"value":       models.DNSModeTUN,
			"label":       "TUN 全局接管",
			"description": "创建虚拟网卡接管所有流量，需要管理员权限",
			"recommended": false,
		},
	}
}

// TestDNSLeak 执行 DNS 泄露测试
func (a *App) TestDNSLeak() (*dns.LeakTestResult, error) {
	a.logManager.LogSystem(logger.LevelInfo, "开始 DNS 泄露测试...")

	result, err := a.leakTester.RunTest()
	if err != nil {
		a.logManager.LogSystem(logger.LevelError, fmt.Sprintf("DNS 泄露测试失败: %v", err))
		return nil, err
	}

	if result.Leaked {
		a.logManager.LogSystem(logger.LevelWarn, "⚠️ 检测到 DNS 泄露!")
	} else {
		a.logManager.LogSystem(logger.LevelInfo, "✓ DNS 未泄露")
	}

	a.logManager.LogSystem(logger.LevelInfo, result.Conclusion)

	return result, nil
}

// QuickDNSLeakCheck 快速 DNS/IP 检查
func (a *App) QuickDNSLeakCheck(nodeID string) (map[string]interface{}, error) {
	node := a.state.GetNode(nodeID)
	if node == nil {
		return nil, fmt.Errorf("节点不存在")
	}

	// 使用节点的监听地址作为代理进行测试
	isChina, ip, err := a.leakTester.QuickLeakCheck(node.Listen)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"ip":        ip,
		"is_leaked": isChina,
		"message":   fmt.Sprintf("检测IP: %s (中国IP: %v)", ip, isChina),
	}, nil
}

// IsTUNSupported 检查系统是否支持 TUN 模式
func (a *App) IsTUNSupported() map[string]interface{} {
	result := map[string]interface{}{
		"supported":     false,
		"is_admin":      false,
		"driver_exists": false,
		"message":       "",
	}

	result["is_admin"] = a.tunManager.IsAdministrator()
	result["driver_exists"] = a.tunManager.CheckWintunDriver(a.state.ExeDir)

	if result["is_admin"].(bool) && result["driver_exists"].(bool) {
		result["supported"] = true
		result["message"] = "TUN 模式可用"
	} else {
		if !result["is_admin"].(bool) {
			result["message"] = "需要以管理员身份运行"
		} else if !result["driver_exists"].(bool) {
			result["message"] = "缺少 wintun.dll 驱动"
		}
	}

	return result
}

// UpdateDNSConfig 更新节点的 DNS 配置
func (a *App) UpdateDNSConfig(nodeID string, mode int, enableSniffing bool) error {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()

	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == nodeID {
			a.state.Config.Nodes[i].DNSMode = mode
			a.state.Config.Nodes[i].EnableSniffing = enableSniffing

			go a.saveConfig()
			a.logManager.LogSystem(logger.LevelInfo,
				fmt.Sprintf("节点 %s DNS模式已更新: %s",
					a.state.Config.Nodes[i].Name,
					models.GetDNSModeString(mode)))
			return nil
		}
	}
	return fmt.Errorf("节点不存在")
}

// ClearFakeIPCache 清空 Fake-IP 映射缓存
func (a *App) ClearFakeIPCache() {
	a.dnsManager.ClearFakeIPCache()
	a.logManager.LogSystem(logger.LevelInfo, "Fake-IP 缓存已清空")
}

// FlushDNSCache 刷新系统 DNS 缓存
func (a *App) FlushDNSCache() error {
	err := a.tunManager.FlushDNSCache()
	if err == nil {
		a.logManager.LogSystem(logger.LevelInfo, "系统 DNS 缓存已刷新")
	} else {
		a.logManager.LogSystem(logger.LevelError, fmt.Sprintf("刷新 DNS 缓存失败: %v", err))
	}
	return err
}

// =============================================================================
// 日志系统 API
// =============================================================================

// GetLogs 获取日志 (支持分页)
func (a *App) GetLogs(limit int) []models.LogEntry {
	return a.logManager.GetLogs(limit)
}

// GetLogsByNode 获取指定节点的日志
func (a *App) GetLogsByNode(nodeID string, limit int) []models.LogEntry {
	return a.logManager.GetLogsByNode(nodeID, limit)
}

// ClearLogs 清空日志
func (a *App) ClearLogs() {
	a.logManager.Clear()
}

// ExportLogs 导出日志到文件
func (a *App) ExportLogs(format string) (string, error) {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("xlink_logs_%s.%s", timestamp, format)

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: filename,
		Filters: []runtime.FileFilter{
			{DisplayName: "日志文件", Pattern: "*." + format},
		},
	})

	if err != nil || path == "" {
		return "", err
	}

	if err := a.logManager.ExportToFile(path, format); err != nil {
		return "", err
	}

	return path, nil
}

// OpenLogFolder 打开日志文件夹
func (a *App) OpenLogFolder() error {
	return system.OpenFolder(a.logManager.GetLogDir())
}

// =============================================================================
// 系统工具 API
// =============================================================================

// OpenConfigFolder 打开配置文件所在文件夹
func (a *App) OpenConfigFolder() error {
	return system.OpenFolder(a.state.ExeDir)
}

// GetSystemInfo 获取系统信息
func (a *App) GetSystemInfo() system.SystemInfo {
	return system.GetSystemInfo()
}

// SetSystemProxy 设置系统代理
func (a *App) SetSystemProxy(nodeID string) error {
	node := a.state.GetNode(nodeID)
	if node == nil {
		return fmt.Errorf("节点不存在")
	}

	// 解析监听地址
	parts := strings.Split(node.Listen, ":")
	if len(parts) != 2 {
		return fmt.Errorf("监听地址格式错误")
	}

	var port int
	fmt.Sscanf(parts[1], "%d", &port)

	return a.proxyManager.SetSystemProxy(parts[0], port)
}

// ClearSystemProxy 清除系统代理
func (a *App) ClearSystemProxy() error {
	return a.proxyManager.ClearSystemProxy()
}

// ShowNotification 显示系统通知
func (a *App) ShowNotification(title, message string) error {
	return a.notification.Show(title, message)
}

// GetVersion 获取版本信息
func (a *App) GetVersion() string {
	return models.AppVersion
}

// GetAppTitle 获取应用标题
func (a *App) GetAppTitle() string {
	return models.AppTitle
}

// =============================================================================
// 内部私有方法
// =============================================================================

// loadConfig 加载配置 (带错误处理和默认值)
func (a *App) loadConfig() {
	cfg, err := a.configManager.Load()
	if err != nil {
		a.logManager.LogSystem(logger.LevelError, fmt.Sprintf("加载配置失败: %v", err))
		// 创建默认配置
		cfg = &models.AppConfig{
			Nodes: []models.NodeConfig{
				models.NewDefaultNode("默认节点"),
			},
			Theme:         "system",
			Language:      "zh-CN",
			GlobalDNSMode: models.DNSModeFakeIP,
		}
	}

	a.state.Mu.Lock()
	a.state.Config = cfg
	a.state.Mu.Unlock()

	a.logManager.LogSystem(logger.LevelInfo, fmt.Sprintf("已加载 %d 个节点配置", len(cfg.Nodes)))
}

// saveConfig 保存配置
func (a *App) saveConfig() {
	a.state.Mu.RLock()
	a.configManager.UpdateConfig(a.state.Config)
	a.state.Mu.RUnlock()

	if err := a.configManager.Save(); err != nil {
		a.logManager.LogSystem(logger.LevelError, fmt.Sprintf("保存配置失败: %v", err))
	}
}

// generateNodeConfig 生成节点配置文件 (集成 Generator 和 DNS Manager)
func (a *App) generateNodeConfig(node *models.NodeConfig) (string, error) {
	// 1. 验证配置有效性
	if err := a.configGenerator.ValidateNodeConfig(node); err != nil {
		return "", err
	}

	// 2. 确定监听地址 (智能分流模式下，Xlink 监听随机内部端口)
	listenAddr := node.Listen
	if node.RoutingMode == models.RoutingModeSmart {
		node.InternalPort = a.engineManager.FindFreePort()
		listenAddr = fmt.Sprintf("127.0.0.1:%d", node.InternalPort)
	}

	// 3. 生成 Xlink 核心配置
	xlinkConfigPath, err := a.configGenerator.GenerateXlinkConfig(node, listenAddr)
	if err != nil {
		return "", fmt.Errorf("生成Xlink配置失败: %w", err)
	}

	// 4. 如果是智能分流模式，生成 Xray 前端配置
	if node.RoutingMode == models.RoutingModeSmart {
		xrayConfigPath := filepath.Join(a.state.ExeDir, fmt.Sprintf(generator.XrayConfigTemplate, node.ID))

		// 检查 Geo 数据库文件是否存在
		hasGeosite := a.dnsManager.FileExists("geosite.dat")
		hasGeoip := a.dnsManager.FileExists("geoip.dat")

		// 生成完整的 Xray 配置 (包含 DNS 防泄露、路由、Inbound/Outbound)
		config, err := a.dnsManager.GenerateFullXrayConfig(node, node.InternalPort, hasGeosite, hasGeoip)
		if err != nil {
			return "", fmt.Errorf("生成Xray配置结构失败: %w", err)
		}

		// 写入配置文件
		if err := a.dnsManager.WriteXrayConfig(config, xrayConfigPath); err != nil {
			return "", fmt.Errorf("写入Xray配置文件失败: %w", err)
		}
	}

	return xlinkConfigPath, nil
}

// emitEvent 辅助方法：发送事件到前端
func (a *App) emitEvent(eventType models.EventType, payload interface{}) {
	runtime.EventsEmit(a.ctx, string(eventType), payload)
}

// emitNodeStatus 辅助方法：发送节点状态更新
func (a *App) emitNodeStatus(nodeID, status string) {
	a.emitEvent(models.EventNodeStatus, map[string]string{
		"node_id": nodeID,
		"status":  status,
	})
}
