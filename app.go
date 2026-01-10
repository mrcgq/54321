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

	// 1. 初始化日志管理器
	a.logManager = logger.NewManager(a.state.ExeDir)
	a.logManager.SetCallback(func(entry models.LogEntry) {
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

	// 初始化 TUN 管理器
	tunName := "XlinkTUN"
	a.tunManager = dns.NewTUNManager(tunName)

	// 初始化自启动管理器
	var err error
	a.autoStart, err = system.NewAutoStartManager("XlinkClient")
	if err != nil {
		a.logManager.LogSystem(logger.LevelWarn, fmt.Sprintf("自启动管理器初始化失败: %v", err))
	}

	// 3. 设置引擎回调
	a.engineManager.SetLogCallback(func(nodeID, nodeName, level, category, message string) {
		a.logManager.LogNode(nodeID, nodeName, level, category, message)
	})

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

	// 🚀【核心逻辑】后端自动托管：恢复上次运行的节点
	// 无论前端是否加载完成，后端都会独立启动代理
	lastID := a.state.Config.LastRunningNodeID
	if lastID != "" {
		go func() {
			// 稍等片刻，确保资源释放或环境就绪
			time.Sleep(500 * time.Millisecond)
			
			node := a.state.GetNode(lastID)
			if node != nil {
				a.logManager.LogSystem(logger.LevelInfo, fmt.Sprintf("正在自动恢复上次运行的节点: %s", node.Name))
				if err := a.StartNode(lastID); err != nil {
					a.logManager.LogSystem(logger.LevelError, fmt.Sprintf("自动恢复失败: %v", err))
				} else {
					a.notification.Show(models.AppTitle, fmt.Sprintf("已恢复运行: %s", node.Name))
				}
			}
		}()
	}

	// 6. 处理系统级开机自启逻辑 (如需隐藏窗口等，可在此处扩展)
	if a.state.IsAutoStart {
		// 实际上有了上面的自动恢复，这里主要用于一些 UI 行为，比如自动最小化
		a.logManager.LogSystem(logger.LevelInfo, "检测到系统开机自启启动")
	}

	a.logManager.LogSystem(logger.LevelInfo, "系统初始化完成")
}

// shutdown 应用关闭时调用
func (a *App) shutdown(ctx context.Context) {
	a.logManager.LogSystem(logger.LevelInfo, "正在关闭应用...")

	// 停止 Ping 测试
	if a.pingManager != nil {
		a.pingManager.StopPing()
	}

	// 停止引擎
	if a.engineManager != nil {
		a.engineManager.StopAll()
	}

	// 恢复系统代理
	if a.proxyManager != nil {
		a.proxyManager.RestoreSystemProxy()
	}

	// 清理临时文件
	if a.configGenerator != nil {
		a.configGenerator.CleanupAllConfigs()
	}

	// 保存配置
	a.saveConfig()

	// 停止日志
	if a.logManager != nil {
		a.logManager.Stop()
	}

	// 取消上下文
	a.cancelMu.Lock()
	for _, cancel := range a.cancelFuncs {
		cancel()
	}
	a.cancelMu.Unlock()
}

// =============================================================================
// 窗口控制 API
// =============================================================================

func (a *App) ShowWindow() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
}

func (a *App) HideWindow() {
	runtime.WindowHide(a.ctx)
}

func (a *App) Quit() {
	runtime.Quit(a.ctx)
}

// =============================================================================
// 节点管理 API
// =============================================================================

func (a *App) GetNodes() []models.NodeConfig {
	a.state.Mu.RLock()
	defer a.state.Mu.RUnlock()

	nodes := make([]models.NodeConfig, len(a.state.Config.Nodes))
	copy(nodes, a.state.Config.Nodes)

	for i := range nodes {
		if es, ok := a.state.EngineStatuses[nodes[i].ID]; ok {
			nodes[i].Status = es.Status
		} else {
			nodes[i].Status = models.StatusStopped
		}
	}
	return nodes
}

func (a *App) GetNode(id string) *models.NodeConfig {
	return a.state.GetNode(id)
}

func (a *App) AddNode(name string) (*models.NodeConfig, error) {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()

	if len(a.state.Config.Nodes) >= models.MaxNodes {
		return nil, fmt.Errorf("节点数量已达上限 (%d)", models.MaxNodes)
	}

	node := models.NewDefaultNode(name)
	a.state.Config.Nodes = append(a.state.Config.Nodes, node)

	go a.saveConfig()
	// 前端增删列表，需要通知
	a.emitEvent(models.EventConfigChanged, nil)

	return &node, nil
}

// UpdateNode 更新节点配置 (⚠️死循环阻断：不广播事件)
func (a *App) UpdateNode(node models.NodeConfig) error {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()

	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == node.ID {
			node.Status = a.state.Config.Nodes[i].Status
			node.InternalPort = a.state.Config.Nodes[i].InternalPort
			a.state.Config.Nodes[i] = node

			go a.saveConfig()
			
			// ❌ 不要广播，防止死循环
			// a.emitEvent(models.EventConfigChanged, nil)
			
			return nil
		}
	}
	return fmt.Errorf("节点不存在: %s", node.ID)
}

func (a *App) DeleteNode(id string) error {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()

	if es, ok := a.state.EngineStatuses[id]; ok && es.Status == models.StatusRunning {
		return fmt.Errorf("请先停止节点再删除")
	}

	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == id {
			a.state.Config.Nodes = append(a.state.Config.Nodes[:i], a.state.Config.Nodes[i+1:]...)
			delete(a.state.EngineStatuses, id)
			go a.configGenerator.CleanupConfigs(id)
			go a.saveConfig()
			
			// 删除操作需要通知前端刷新列表
			a.emitEvent(models.EventConfigChanged, nil)
			return nil
		}
	}
	return fmt.Errorf("节点不存在: %s", id)
}

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

	newNode := *srcNode
	newNode.ID = models.GenerateUUID()
	newNode.Name = srcNode.Name + " (副本)"
	newNode.Status = models.StatusStopped
	newNode.Rules = make([]models.RoutingRule, len(srcNode.Rules))
	copy(newNode.Rules, srcNode.Rules)

	a.state.Config.Nodes = append(a.state.Config.Nodes, newNode)

	go a.saveConfig()
	a.emitEvent(models.EventConfigChanged, nil)

	return &newNode, nil
}

// =============================================================================
// 节点控制 API (启动/停止)
// =============================================================================

// StartNode 启动指定节点
func (a *App) StartNode(id string) error {
	node := a.state.GetNode(id)
	if node == nil {
		return fmt.Errorf("节点不存在: %s", id)
	}

	a.logManager.LogNode(id, node.Name, logger.LevelInfo, logger.CategorySystem, "正在启动...")

	configPath, err := a.generateNodeConfig(node)
	if err != nil {
		errMsg := fmt.Sprintf("生成配置失败: %v", err)
		a.logManager.LogNode(id, node.Name, logger.LevelError, logger.CategorySystem, errMsg)
		return fmt.Errorf(errMsg)
	}

	if err := a.engineManager.StartNode(node, configPath); err != nil {
		return err
	}

	// 🚀【核心修改】启动成功，记录状态
	a.state.Mu.Lock()
	a.state.Config.LastRunningNodeID = id
	a.state.Mu.Unlock()
	go a.saveConfig()

	return nil
}

// StopNode 停止指定节点
func (a *App) StopNode(id string) error {
	node := a.state.GetNode(id)
	if node == nil {
		return fmt.Errorf("节点不存在: %s", id)
	}

	a.logManager.LogNode(id, node.Name, logger.LevelInfo, logger.CategorySystem, "正在停止...")

	err := a.engineManager.StopNode(id)

	// 🚀【核心修改】停止后，清除记录
	a.state.Mu.Lock()
	if a.state.Config.LastRunningNodeID == id {
		a.state.Config.LastRunningNodeID = ""
	}
	a.state.Mu.Unlock()
	go a.saveConfig()

	return err
}

// StartAllNodes 启动所有节点
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
	
	// 清除记录
	a.state.Mu.Lock()
	a.state.Config.LastRunningNodeID = ""
	a.state.Mu.Unlock()
	go a.saveConfig()
	
	return nil
}

// PingTest 延迟测试
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
				a.emitEvent(models.EventPingResult, result)
			},
			func(report logger.PingReport) {
				a.emitEvent(models.EventPingComplete, report)
			},
		)

		if err != nil {
			a.logManager.LogNode(id, node.Name, logger.LevelError, logger.CategoryPing, fmt.Sprintf("测速启动失败: %v", err))
		}
	}()

	return nil
}

func (a *App) StopPingTest() {
	a.pingManager.StopPing()
}

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

func (a *App) GetNodeStatus(id string) string {
	return a.engineManager.GetStatus(id)
}

func (a *App) GetAllNodeStatuses() map[string]models.EngineStatus {
	return a.engineManager.GetAllStatuses()
}

// =============================================================================
// 规则/导入导出/设置 等其他 API (逻辑不变，仅确保 Mu 使用正确)
// =============================================================================

func (a *App) AddRule(nodeID string, rule models.RoutingRule) error {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()
	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == nodeID {
			rule.ID = models.GenerateUUID()
			a.state.Config.Nodes[i].Rules = append(a.state.Config.Nodes[i].Rules, rule)
			go a.saveConfig()
			return nil
		}
	}
	return fmt.Errorf("节点不存在")
}

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
			return fmt.Errorf("规则不存在")
		}
	}
	return fmt.Errorf("节点不存在")
}

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
			return fmt.Errorf("规则不存在")
		}
	}
	return fmt.Errorf("节点不存在")
}

func (a *App) GetPresetRules(presetName string) []string {
	return generator.GetPresetRules(presetName)
}

func (a *App) GetAllPresets() []string {
	return []string{"block-ads", "direct-cn", "proxy-common", "proxy-streaming", "privacy"}
}

func (a *App) ApplyPreset(nodeID, presetName string) error {
	rules := generator.GetPresetRules(presetName)
	if rules == nil { return fmt.Errorf("预设不存在") }
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()
	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == nodeID {
			for _, ruleStr := range rules {
				parts := strings.SplitN(ruleStr, ",", 2)
				if len(parts) != 2 { continue }
				rule := models.RoutingRule{ID: models.GenerateUUID(), Target: parts[1]}
				left := parts[0]
				switch {
				case strings.HasPrefix(left, "geosite:"): rule.Type = "geosite:"; rule.Match = strings.TrimPrefix(left, "geosite:")
				case strings.HasPrefix(left, "geoip:"): rule.Type = "geoip:"; rule.Match = strings.TrimPrefix(left, "geoip:")
				default: rule.Type = ""; rule.Match = left
				}
				a.state.Config.Nodes[i].Rules = append(a.state.Config.Nodes[i].Rules, rule)
			}
			go a.saveConfig()
			return nil
		}
	}
	return fmt.Errorf("节点不存在")
}

func (a *App) ImportFromClipboard() (int, error) {
	text, err := runtime.ClipboardGetText(a.ctx)
	if err != nil { return 0, err }
	imported, err := a.configManager.ImportNodes(text)
	if err != nil { return 0, err }
	a.state.Mu.Lock()
	a.state.Config = a.configManager.GetConfig()
	a.state.Mu.Unlock()
	go a.saveConfig()
	a.emitEvent(models.EventConfigChanged, nil)
	return len(imported), nil
}

func (a *App) ExportToClipboard(id string) error {
	uri, err := a.configManager.ExportNode(id)
	if err != nil { return err }
	return runtime.ClipboardSetText(a.ctx, uri)
}

func (a *App) ExportAllToClipboard() error {
	a.state.Mu.RLock()
	nodes := a.state.Config.Nodes
	a.state.Mu.RUnlock()
	var uris []string
	for _, node := range nodes {
		if uri, err := a.configManager.ExportNode(node.ID); err == nil { uris = append(uris, uri) }
	}
	if len(uris) == 0 { return fmt.Errorf("没有节点") }
	return runtime.ClipboardSetText(a.ctx, strings.Join(uris, "\n"))
}

func (a *App) ListBackups() []string { return a.configManager.ListBackups() }

func (a *App) RestoreBackup(backupName string) error {
	if err := a.configManager.RestoreBackup(backupName); err != nil { return err }
	a.state.Mu.Lock()
	a.state.Config = a.configManager.GetConfig()
	a.state.Mu.Unlock()
	a.emitEvent(models.EventConfigChanged, nil)
	return nil
}

func (a *App) GetSettings() models.AppConfig {
	a.state.Mu.RLock()
	defer a.state.Mu.RUnlock()
	return *a.state.Config
}

func (a *App) UpdateSettings(cfg models.AppConfig) error {
	a.state.Mu.Lock()
	cfg.Nodes = a.state.Config.Nodes
	cfg.LastRunningNodeID = a.state.Config.LastRunningNodeID // 保护运行记录
	a.state.Config = &cfg
	a.state.Mu.Unlock()
	go a.saveConfig()
	return nil
}

func (a *App) SetAutoStart(enabled bool) error {
	if a.autoStart == nil { return fmt.Errorf("自启未初始化") }
	var err error
	if enabled { err = a.autoStart.Enable() } else { err = a.autoStart.Disable() }
	if err != nil { return err }
	a.state.Mu.Lock()
	a.state.Config.AutoStart = enabled
	a.state.Mu.Unlock()
	go a.saveConfig()
	return nil
}

func (a *App) GetAutoStart() bool {
	if a.autoStart == nil { return false }
	return a.autoStart.IsEnabled()
}

func (a *App) GetDNSModes() []map[string]interface{} {
	return []map[string]interface{}{
		{"value": models.DNSModeStandard, "label": "标准模式", "description": "系统默认DNS", "recommended": false},
		{"value": models.DNSModeFakeIP, "label": "Fake-IP 模式", "description": "推荐，防泄露", "recommended": true},
		{"value": models.DNSModeTUN, "label": "TUN 全局接管", "description": "需管理员权限", "recommended": false},
	}
}

func (a *App) TestDNSLeak() (*dns.LeakTestResult, error) {
	return a.leakTester.RunTest()
}

func (a *App) QuickDNSLeakCheck(nodeID string) (map[string]interface{}, error) {
	node := a.state.GetNode(nodeID)
	if node == nil { return nil, fmt.Errorf("节点不存在") }
	isChina, ip, err := a.leakTester.QuickLeakCheck(node.Listen)
	if err != nil { return nil, err }
	return map[string]interface{}{"ip": ip, "is_leaked": isChina}, nil
}

func (a *App) IsTUNSupported() map[string]interface{} {
	isAdmin := a.tunManager.IsAdministrator()
	driver := a.tunManager.CheckWintunDriver(a.state.ExeDir)
	return map[string]interface{}{"supported": isAdmin && driver, "is_admin": isAdmin, "driver_exists": driver}
}

func (a *App) UpdateDNSConfig(nodeID string, mode int, enableSniffing bool) error {
	a.state.Mu.Lock()
	defer a.state.Mu.Unlock()
	for i := range a.state.Config.Nodes {
		if a.state.Config.Nodes[i].ID == nodeID {
			a.state.Config.Nodes[i].DNSMode = mode
			a.state.Config.Nodes[i].EnableSniffing = enableSniffing
			go a.saveConfig()
			return nil
		}
	}
	return fmt.Errorf("节点不存在")
}

func (a *App) ClearFakeIPCache() { a.dnsManager.ClearFakeIPCache() }
func (a *App) FlushDNSCache() error { return a.tunManager.FlushDNSCache() }

func (a *App) GetLogs(limit int) []models.LogEntry { return a.logManager.GetLogs(limit) }
func (a *App) GetLogsByNode(nodeID string, limit int) []models.LogEntry { return a.logManager.GetLogsByNode(nodeID, limit) }
func (a *App) ClearLogs() { a.logManager.Clear() }
func (a *App) ExportLogs(format string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{DefaultFilename: "logs." + format})
	if err != nil || path == "" { return "", err }
	return path, a.logManager.ExportToFile(path, format)
}
func (a *App) OpenLogFolder() error { return system.OpenFolder(a.logManager.GetLogDir()) }
func (a *App) OpenConfigFolder() error { return system.OpenFolder(a.state.ExeDir) }
func (a *App) GetSystemInfo() system.SystemInfo { return system.GetSystemInfo() }
func (a *App) SetSystemProxy(nodeID string) error {
	node := a.state.GetNode(nodeID)
	if node == nil { return fmt.Errorf("节点不存在") }
	parts := strings.Split(node.Listen, ":")
	var port int
	fmt.Sscanf(parts[1], "%d", &port)
	return a.proxyManager.SetSystemProxy(parts[0], port)
}
func (a *App) ClearSystemProxy() error { return a.proxyManager.ClearSystemProxy() }
func (a *App) ShowNotification(title, message string) error { return a.notification.Show(title, message) }
func (a *App) GetVersion() string { return models.AppVersion }
func (a *App) GetAppTitle() string { return models.AppTitle }

// =============================================================================
// 私有
// =============================================================================

func (a *App) loadConfig() {
	cfg, err := a.configManager.Load()
	if err != nil {
		cfg = &models.AppConfig{
			Nodes: []models.NodeConfig{models.NewDefaultNode("默认节点")},
			Theme: "system", Language: "zh-CN", GlobalDNSMode: models.DNSModeFakeIP,
		}
	}
	a.state.Mu.Lock()
	a.state.Config = cfg
	a.state.Mu.Unlock()
}

func (a *App) saveConfig() {
	a.state.Mu.RLock()
	a.configManager.UpdateConfig(a.state.Config)
	a.state.Mu.RUnlock()
	a.configManager.Save()
}

func (a *App) generateNodeConfig(node *models.NodeConfig) (string, error) {
	if err := a.configGenerator.ValidateNodeConfig(node); err != nil { return "", err }
	
	listenAddr := node.Listen
	if node.RoutingMode == models.RoutingModeSmart {
		node.InternalPort = a.engineManager.FindFreePort()
		listenAddr = fmt.Sprintf("127.0.0.1:%d", node.InternalPort)
	}

	xlinkPath, err := a.configGenerator.GenerateXlinkConfig(node, listenAddr)
	if err != nil { return "", err }

	if node.RoutingMode == models.RoutingModeSmart {
		xrayPath := filepath.Join(a.state.ExeDir, fmt.Sprintf(generator.XrayConfigTemplate, node.ID))
		hasGeosite := a.dnsManager.FileExists("geosite.dat")
		hasGeoip := a.dnsManager.FileExists("geoip.dat")
		cfg, err := a.dnsManager.GenerateFullXrayConfig(node, node.InternalPort, hasGeosite, hasGeoip)
		if err != nil { return "", err }
		if err := a.dnsManager.WriteXrayConfig(cfg, xrayPath); err != nil { return "", err }
	}
	return xlinkPath, nil
}

func (a *App) emitEvent(t models.EventType, p interface{}) { runtime.EventsEmit(a.ctx, string(t), p) }
func (a *App) emitNodeStatus(id, s string) { a.emitEvent(models.EventNodeStatus, map[string]string{"node_id": id, "status": s}) }
