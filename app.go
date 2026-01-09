// Package main 包含应用主逻辑和前端绑定
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"xlink-wails/internal/config"
	"xlink-wails/internal/engine" // [新增] 引入引擎包
	"xlink-wails/internal/models"
)

// App 主应用结构
type App struct {
	ctx   context.Context
	state *models.AppState

	// 配置管理器
	configManager *config.Manager

	// 引擎管理器 [新增]
	engineManager *engine.Manager

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

// startup 应用启动时调用
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 初始化配置管理器
	a.configManager = config.NewManager(a.state.ExeDir)

	// 初始化引擎管理器 [新增]
	a.engineManager = engine.NewManager(a.state.ExeDir)

	// 设置引擎日志回调 [新增]
	a.engineManager.SetLogCallback(func(nodeID, nodeName, level, category, message string) {
		a.appendNodeLog(nodeID, nodeName, level, category, message)
	})

	// 设置引擎状态回调 [新增]
	a.engineManager.SetStatusCallback(func(nodeID, status string, err error) {
		// 更新内存状态
		a.state.UpdateNodeStatus(nodeID, status, "")
		// 通知前端
		a.emitNodeStatus(nodeID, status)

		if err != nil {
			node := a.state.GetNode(nodeID)
			nodeName := nodeID
			if node != nil {
				nodeName = node.Name
			}
			a.appendNodeLog(nodeID, nodeName, "error", "系统", err.Error())
		}
	})

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

	// 停止所有引擎 [新增]
	if a.engineManager != nil {
		a.engineManager.StopAll()
	}

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
// 节点控制 API [重写]
// =============================================================================

// StartNode 启动指定节点
func (a *App) StartNode(id string) error {
	node := a.state.GetNode(id)
	if node == nil {
		return fmt.Errorf("节点不存在: %s", id)
	}

	a.appendNodeLog(id, node.Name, "info", "系统", "正在启动...")

	// 生成配置文件
	configPath, err := a.generateNodeConfig(node)
	if err != nil {
		errMsg := fmt.Sprintf("生成配置失败: %v", err)
		a.appendNodeLog(id, node.Name, "error", "系统", errMsg)
		return fmt.Errorf(errMsg)
	}

	// 启动引擎
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

	a.appendNodeLog(id, node.Name, "info", "系统", "正在停止...")

	// 调用引擎管理器停止
	return a.engineManager.StopNode(id)
}

// StartAllNodes 启动所有节点
func (a *App) StartAllNodes() error {
	a.state.mu.RLock()
	nodes := make([]models.NodeConfig, len(a.state.Config.Nodes))
	copy(nodes, a.state.Config.Nodes)
	a.state.mu.RUnlock()

	var lastErr error
	for _, node := range nodes {
		if err := a.StartNode(node.ID); err != nil {
			a.appendSystemLog("error", "系统", fmt.Sprintf("启动节点 %s 失败: %v", node.Name, err))
			lastErr = err
		}
	}

	return lastErr
}

// StopAllNodes 停止所有节点
func (a *App) StopAllNodes() error {
	// 调用引擎管理器停止所有
	a.engineManager.StopAll()
	return nil
}

// PingTest 延迟测试
func (a *App) PingTest(id string) error {
	node := a.state.GetNode(id)
	if node == nil {
		return fmt.Errorf("节点不存在: %s", id)
	}

	a.appendNodeLog(id, node.Name, "info", "测速", "正在启动延迟测试...")

	go func() {
		err := a.engineManager.PingTest(node, func(result models.PingResult) {
			var msg string
			if result.Latency >= 0 {
				msg = fmt.Sprintf("%s - 延迟: %dms", result.Server, result.Latency)
			} else {
				msg = fmt.Sprintf("%s - 失败: %s", result.Server, result.Error)
			}
			a.appendNodeLog(id, node.Name, "info", "测速", msg)

			// 发送结果到前端
			a.emitEvent(models.EventPingResult, result)
		})

		if err != nil {
			a.appendNodeLog(id, node.Name, "error", "测速", fmt.Sprintf("测速失败: %v", err))
		} else {
			a.appendNodeLog(id, node.Name, "info", "测速", "延迟测试完成")
		}
	}()

	return nil
}

// GetNodeStatus 获取节点状态 [新增]
func (a *App) GetNodeStatus(id string) string {
	return a.engineManager.GetStatus(id)
}

// GetAllNodeStatuses 获取所有节点状态 [新增]
func (a *App) GetAllNodeStatuses() map[string]models.EngineStatus {
	return a.engineManager.GetAllStatuses()
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
// 导入导出 API
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

// ExportAllToClipboard 导出所有节点到剪贴板
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
// 备份管理 API
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
// 配置文件路径 API
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
// 配置持久化
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

// =============================================================================
// 配置文件生成（临时占位，步骤4完整实现）
// =============================================================================

// generateNodeConfig 生成节点配置文件
func (a *App) generateNodeConfig(node *models.NodeConfig) (string, error) {
	// 确定监听地址
	listenAddr := node.Listen
	if node.RoutingMode == models.RoutingModeSmart {
		// 智能分流模式，Xlink监听内部端口
		node.InternalPort = a.engineManager.FindFreePort()
		listenAddr = fmt.Sprintf("127.0.0.1:%d", node.InternalPort)
	}

	// 生成Xlink配置
	configPath := filepath.Join(a.state.ExeDir, fmt.Sprintf("config_core_%s.json", node.ID))

	// 准备服务器列表
	servers := strings.ReplaceAll(node.Server, "\r\n", ";")
	servers = strings.ReplaceAll(servers, "\n", ";")

	// 准备Token
	tokenStr := node.SecretKey
	if node.FallbackIP != "" {
		tokenStr = node.SecretKey + "|" + node.FallbackIP
	}

	// 策略
	strategy := models.GetStrategyString(node.StrategyMode)

	// 序列化规则
	var rulesStr string
	for _, r := range node.Rules {
		if rulesStr != "" {
			rulesStr += "\\r\\n"
		}
		rulesStr += r.Type + r.Match + "," + r.Target
	}

	// 写入配置文件
	configJSON := fmt.Sprintf(`{
  "inbounds": [{"tag": "socks-in", "listen": "%s", "protocol": "socks"}],
  "outbounds": [{
    "tag": "proxy",
    "protocol": "ech-proxy",
    "settings": {
      "server": "%s",
      "server_ip": "%s",
      "token": "%s",
      "strategy": "%s",
      "rules": "%s",
      "global_keep_alive": false,
      "s5": "%s"
    }
  }]
}`, listenAddr, servers, node.IP, tokenStr, strategy, rulesStr, node.Socks5)

	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		return "", fmt.Errorf("写入配置文件失败: %w", err)
	}

	// 如果是智能分流模式，生成Xray配置
	if node.RoutingMode == models.RoutingModeSmart {
		xrayConfigPath := filepath.Join(a.state.ExeDir, fmt.Sprintf("config_xray_%s.json", node.ID))
		if err := a.generateXrayConfig(node, xrayConfigPath); err != nil {
			return "", err
		}
	}

	return configPath, nil
}

// generateXrayConfig 生成Xray配置文件
func (a *App) generateXrayConfig(node *models.NodeConfig, configPath string) error {
	// 解析监听地址
	listenHost := "127.0.0.1"
	listenPort := "10808"

	if idx := strings.LastIndex(node.Listen, ":"); idx != -1 {
		listenHost = node.Listen[:idx]
		listenPort = node.Listen[idx+1:]
	}

	// 检查geo文件
	hasGeosite := fileExists(filepath.Join(a.state.ExeDir, "geosite.dat"))
	hasGeoip := fileExists(filepath.Join(a.state.ExeDir, "geoip.dat"))

	// 构建配置
	var rules []string

	// 用户自定义规则
	for _, r := range node.Rules {
		if r.Type == "geosite:" || r.Type == "geoip:" {
			outbound := "proxy_out"
			if strings.Contains(r.Target, "direct") {
				outbound = "direct"
			} else if strings.Contains(r.Target, "block") {
				outbound = "block"
			}

			matcher := "domain"
			if r.Type == "geoip:" {
				matcher = "ip"
			}

			rules = append(rules, fmt.Sprintf(
				`      { "type": "field", "outboundTag": "%s", "%s": ["%s%s"] }`,
				outbound, matcher, r.Type, r.Match,
			))
		}
	}

	// 默认规则
	if hasGeosite {
		rules = append(rules, `      { "type": "field", "outboundTag": "block", "domain": ["geosite:category-ads-all"] }`)
	}
	rules = append(rules, `      { "type": "field", "outboundTag": "block", "protocol": ["bittorrent"] }`)
	if hasGeoip {
		rules = append(rules, `      { "type": "field", "outboundTag": "direct", "ip": ["geoip:private", "geoip:cn"] }`)
	}
	if hasGeosite {
		rules = append(rules, `      { "type": "field", "outboundTag": "direct", "domain": ["geosite:cn"] }`)
	}

	rulesStr := strings.Join(rules, ",\n")

	configJSON := fmt.Sprintf(`{
  "log": { "loglevel": "warning" },
  "inbounds": [{
    "listen": "%s", "port": %s, "protocol": "socks",
    "settings": {"auth": "noauth", "udp": true, "ip": "127.0.0.1"}
  }],
  "outbounds": [
    { "protocol": "socks", "settings": { "servers": [ {"address": "127.0.0.1", "port": %d} ] }, "tag": "proxy_out" },
    { "protocol": "freedom", "tag": "direct" },
    { "protocol": "blackhole", "tag": "block" }
  ],
  "routing": {
    "domainStrategy": "AsIs",
    "rules": [
%s
    ]
  }
}`, listenHost, listenPort, node.InternalPort, rulesStr)

	return os.WriteFile(configPath, []byte(configJSON), 0644)
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
