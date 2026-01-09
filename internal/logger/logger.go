// Package logger 提供统一的日志管理系统
package logger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"xlink-wails/internal/models"
)

// =============================================================================
// 常量
// =============================================================================

const (
	// 日志缓冲区大小
	BufferSize = 10000

	// 日志刷新间隔
	FlushInterval = 100 * time.Millisecond

	// 日志文件保留天数
	LogRetentionDays = 7

	// 单个日志文件最大大小 (MB)
	MaxLogFileSizeMB = 10

	// 日志目录名
	LogDirName = "logs"
)

// 日志级别
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// 日志类别
const (
	CategorySystem  = "系统"
	CategoryEngine  = "内核"
	CategoryTunnel  = "隧道"
	CategoryRule    = "规则"
	CategoryLB      = "负载"
	CategoryStats   = "统计"
	CategoryPing    = "测速"
	CategoryXray    = "Xray"
	CategoryDNS     = "DNS"
)

// =============================================================================
// 日志管理器
// =============================================================================

// Manager 日志管理器
type Manager struct {
	mu sync.RWMutex

	// 日志缓冲区
	buffer    []models.LogEntry
	bufferPos int

	// 文件日志
	exeDir      string
	logFile     *os.File
	logFilePath string

	// 回调函数
	onNewLog func(entry models.LogEntry)

	// 控制
	flushTicker *time.Ticker
	stopChan    chan struct{}
	stopped     bool

	// 日志解析器
	parsers []LogParser
}

// LogParser 日志解析器接口
type LogParser interface {
	CanParse(line string) bool
	Parse(line string) (level, category, message string)
}

// NewManager 创建日志管理器
func NewManager(exeDir string) *Manager {
	m := &Manager{
		buffer:   make([]models.LogEntry, BufferSize),
		exeDir:   exeDir,
		stopChan: make(chan struct{}),
		parsers:  defaultParsers(),
	}

	// 初始化日志文件
	m.initLogFile()

	// 启动刷新协程
	go m.flushLoop()

	return m
}

// initLogFile 初始化日志文件
func (m *Manager) initLogFile() {
	logDir := filepath.Join(m.exeDir, LogDirName)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return
	}

	// 清理旧日志
	m.cleanOldLogs(logDir)

	// 创建今日日志文件
	today := time.Now().Format("2006-01-02")
	m.logFilePath = filepath.Join(logDir, fmt.Sprintf("xlink_%s.log", today))

	file, err := os.OpenFile(m.logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}

	m.logFile = file
}

// cleanOldLogs 清理旧日志文件
func (m *Manager) cleanOldLogs(logDir string) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -LogRetentionDays)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(logDir, entry.Name()))
		}
	}
}

// =============================================================================
// 日志记录
// =============================================================================

// Log 记录日志
func (m *Manager) Log(nodeID, nodeName, level, category, message string) {
	entry := models.LogEntry{
		Timestamp: time.Now(),
		NodeID:    nodeID,
		NodeName:  nodeName,
		Level:     level,
		Category:  category,
		Message:   message,
	}

	m.appendEntry(entry)
}

// LogSystem 记录系统日志
func (m *Manager) LogSystem(level, message string) {
	m.Log("", "系统", level, CategorySystem, message)
}

// LogNode 记录节点日志
func (m *Manager) LogNode(nodeID, nodeName, level, category, message string) {
	m.Log(nodeID, nodeName, level, category, message)
}

// appendEntry 追加日志条目
func (m *Manager) appendEntry(entry models.LogEntry) {
	m.mu.Lock()

	// 环形缓冲区
	m.buffer[m.bufferPos%BufferSize] = entry
	m.bufferPos++

	m.mu.Unlock()

	// 写入文件
	m.writeToFile(entry)

	// 回调通知
	if m.onNewLog != nil {
		m.onNewLog(entry)
	}
}

// writeToFile 写入日志文件
func (m *Manager) writeToFile(entry models.LogEntry) {
	if m.logFile == nil {
		return
	}

	// 检查是否需要轮转
	m.checkRotate()

	// 格式化日志行
	line := fmt.Sprintf("[%s] [%s] [%s] [%s] %s\n",
		entry.Timestamp.Format("2006-01-02 15:04:05.000"),
		entry.NodeName,
		entry.Level,
		entry.Category,
		entry.Message,
	)

	m.logFile.WriteString(line)
}

// checkRotate 检查是否需要日志轮转
func (m *Manager) checkRotate() {
	if m.logFile == nil {
		return
	}

	info, err := m.logFile.Stat()
	if err != nil {
		return
	}

	// 检查日期变更
	today := time.Now().Format("2006-01-02")
	expectedPath := filepath.Join(m.exeDir, LogDirName, fmt.Sprintf("xlink_%s.log", today))

	if m.logFilePath != expectedPath {
		m.logFile.Close()
		m.logFilePath = expectedPath
		m.logFile, _ = os.OpenFile(m.logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		return
	}

	// 检查文件大小
	if info.Size() > int64(MaxLogFileSizeMB*1024*1024) {
		m.logFile.Close()

		// 重命名当前文件
		timestamp := time.Now().Format("150405")
		newPath := strings.Replace(m.logFilePath, ".log", fmt.Sprintf("_%s.log", timestamp), 1)
		os.Rename(m.logFilePath, newPath)

		// 创建新文件
		m.logFile, _ = os.OpenFile(m.logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	}
}

// =============================================================================
// 日志查询
// =============================================================================

// GetLogs 获取最近的日志
func (m *Manager) GetLogs(limit int) []models.LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > BufferSize {
		limit = BufferSize
	}

	count := m.bufferPos
	if count > BufferSize {
		count = BufferSize
	}

	if limit > count {
		limit = count
	}

	result := make([]models.LogEntry, limit)

	// 从最新的开始取
	for i := 0; i < limit; i++ {
		idx := (m.bufferPos - 1 - i + BufferSize) % BufferSize
		if m.bufferPos-1-i < 0 {
			break
		}
		result[limit-1-i] = m.buffer[idx]
	}

	return result
}

// GetLogsByNode 获取指定节点的日志
func (m *Manager) GetLogsByNode(nodeID string, limit int) []models.LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []models.LogEntry

	count := m.bufferPos
	if count > BufferSize {
		count = BufferSize
	}

	for i := 0; i < count && len(result) < limit; i++ {
		idx := (m.bufferPos - 1 - i + BufferSize) % BufferSize
		if m.bufferPos-1-i < 0 {
			break
		}

		entry := m.buffer[idx]
		if entry.NodeID == nodeID {
			result = append([]models.LogEntry{entry}, result...)
		}
	}

	return result
}

// GetLogsByLevel 获取指定级别的日志
func (m *Manager) GetLogsByLevel(level string, limit int) []models.LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []models.LogEntry

	count := m.bufferPos
	if count > BufferSize {
		count = BufferSize
	}

	for i := 0; i < count && len(result) < limit; i++ {
		idx := (m.bufferPos - 1 - i + BufferSize) % BufferSize
		if m.bufferPos-1-i < 0 {
			break
		}

		entry := m.buffer[idx]
		if entry.Level == level {
			result = append([]models.LogEntry{entry}, result...)
		}
	}

	return result
}

// Clear 清空日志缓冲区
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.buffer = make([]models.LogEntry, BufferSize)
	m.bufferPos = 0
}

// =============================================================================
// 日志解析
// =============================================================================

// ParseAndLog 解析原始日志并记录
func (m *Manager) ParseAndLog(nodeID, nodeName, rawLog string) {
	lines := strings.Split(rawLog, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "\r")
		line = strings.TrimSuffix(line, "\r")

		if line == "" {
			continue
		}

		level, category, message := m.parseLine(line)
		m.LogNode(nodeID, nodeName, level, category, message)
	}
}

// parseLine 解析单行日志
func (m *Manager) parseLine(line string) (level, category, message string) {
	// 默认值
	level = LevelInfo
	category = CategoryEngine
	message = line

	// 尝试各个解析器
	for _, parser := range m.parsers {
		if parser.CanParse(line) {
			return parser.Parse(line)
		}
	}

	// 基本级别检测
	switch {
	case strings.Contains(line, "[ERR]") || strings.Contains(line, "error") || strings.Contains(line, "Error"):
		level = LevelError
	case strings.Contains(line, "[WARN]") || strings.Contains(line, "warning"):
		level = LevelWarn
	case strings.Contains(line, "[DEBUG]"):
		level = LevelDebug
	}

	// 移除常见前缀
	message = strings.TrimPrefix(message, "[CLI] ")
	message = strings.TrimPrefix(message, "[Core] ")

	return
}

// =============================================================================
// 内置日志解析器
// =============================================================================

func defaultParsers() []LogParser {
	return []LogParser{
		&TunnelParser{},
		&RuleHitParser{},
		&LoadBalanceParser{},
		&StatsParser{},
		&PingParser{},
		&XrayParser{},
	}
}

// TunnelParser 隧道日志解析器
type TunnelParser struct{}

func (p *TunnelParser) CanParse(line string) bool {
	return strings.Contains(line, "Tunnel ->")
}

func (p *TunnelParser) Parse(line string) (level, category, message string) {
	level = LevelInfo
	category = CategoryTunnel

	var sni, real, latency string

	// 提取延迟
	if idx := strings.Index(line, "Latency:"); idx != -1 {
		latency = strings.TrimSpace(line[idx+8:])
	}

	// 提取SNI和真实服务器
	if idx := strings.Index(line, "Tunnel ->"); idx != -1 {
		rest := line[idx+9:]
		parts := strings.Split(rest, ">>>")
		if len(parts) >= 2 {
			sni = strings.TrimSpace(strings.Split(parts[0], "(")[0])
			realPart := strings.TrimSpace(parts[1])
			real = strings.TrimSpace(strings.Split(realPart, "(")[0])
		}
	}

	if sni != "" && real != "" {
		message = fmt.Sprintf("隧道建立: %s ==> %s [延迟: %s]", sni, real, latency)
	} else {
		message = line
	}

	return
}

// RuleHitParser 规则命中解析器
type RuleHitParser struct{}

func (p *RuleHitParser) CanParse(line string) bool {
	return strings.Contains(line, "Rule Hit")
}

func (p *RuleHitParser) Parse(line string) (level, category, message string) {
	level = LevelInfo
	category = CategoryRule

	var target, node, rule string

	if idx := strings.Index(line, "Rule Hit ->"); idx != -1 {
		rest := line[idx+11:]
		parts := strings.Split(rest, "|")
		if len(parts) >= 2 {
			target = strings.TrimSpace(parts[0])

			sniPart := parts[1]
			if sniIdx := strings.Index(sniPart, "SNI:"); sniIdx != -1 {
				nodeRest := sniPart[sniIdx+4:]
				node = strings.TrimSpace(strings.Split(nodeRest, "(")[0])
			}
			if ruleIdx := strings.Index(sniPart, "(Rule:"); ruleIdx != -1 {
				rule = strings.TrimSuffix(strings.TrimSpace(sniPart[ruleIdx+6:]), ")")
			}
		}
	}

	if target != "" && node != "" {
		message = fmt.Sprintf("命中: %-25s -> %s (关键词: %s)", target, node, rule)
	} else {
		message = line
	}

	return
}

// LoadBalanceParser 负载均衡解析器
type LoadBalanceParser struct{}

func (p *LoadBalanceParser) CanParse(line string) bool {
	return strings.Contains(line, "LB ->")
}

func (p *LoadBalanceParser) Parse(line string) (level, category, message string) {
	level = LevelInfo
	category = CategoryLB

	var target, node, algo string

	if idx := strings.Index(line, "LB ->"); idx != -1 {
		rest := line[idx+5:]
		parts := strings.Split(rest, "|")

		if len(parts) >= 1 {
			target = strings.TrimSpace(parts[0])
		}
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(p, "SNI:") {
				node = strings.TrimSpace(p[4:])
			} else if strings.HasPrefix(p, "Algo:") {
				algo = strings.TrimSpace(p[5:])
			}
		}
	}

	// 翻译策略名称
	switch algo {
	case "random":
		algo = "随机"
	case "rr":
		algo = "轮询"
	case "hash":
		algo = "哈希"
	}

	if target != "" && node != "" {
		message = fmt.Sprintf("访问: %-25s -> %s (策略: %s)", target, node, algo)
	} else {
		message = line
	}

	return
}

// StatsParser 统计日志解析器
type StatsParser struct{}

func (p *StatsParser) CanParse(line string) bool {
	return strings.Contains(line, "[Stats]")
}

func (p *StatsParser) Parse(line string) (level, category, message string) {
	level = LevelInfo
	category = CategoryStats

	var target, up, down, duration string

	if idx := strings.Index(line, "[Stats]"); idx != -1 {
		rest := line[idx+7:]
		parts := strings.Split(rest, "|")

		if len(parts) >= 1 {
			target = strings.TrimSpace(parts[0])
		}
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(p, "Up:") {
				up = strings.TrimSpace(p[3:])
			} else if strings.HasPrefix(p, "Down:") {
				down = strings.TrimSpace(p[5:])
			} else if strings.HasPrefix(p, "Time:") {
				duration = strings.TrimSpace(p[5:])
			}
		}
	}

	if target != "" {
		message = fmt.Sprintf("结束: %s (上行:%s / 下行:%s) 时长:%s", target, up, down, duration)
	} else {
		message = line
	}

	return
}

// PingParser Ping测试解析器
type PingParser struct{}

func (p *PingParser) CanParse(line string) bool {
	return strings.Contains(line, "Ping") ||
		strings.Contains(line, "Delay:") ||
		strings.Contains(line, "Successful Nodes") ||
		strings.Contains(line, "Failed Nodes")
}

func (p *PingParser) Parse(line string) (level, category, message string) {
	level = LevelInfo
	category = CategoryPing

	switch {
	case strings.Contains(line, "Ping Test Report"):
		message = "--- 延迟测试报告 ---"
	case strings.Contains(line, "Successful Nodes"):
		message = "成功节点:"
	case strings.Contains(line, "Failed Nodes"):
		message = "失败节点:"
		level = LevelWarn
	case strings.Contains(line, "Error:"):
		level = LevelError
		message = line
	default:
		message = line
	}

	return
}

// XrayParser Xray日志解析器
type XrayParser struct {
	// 匹配Xray日志格式
	pattern *regexp.Regexp
}

func (p *XrayParser) CanParse(line string) bool {
	return strings.Contains(line, "Xray") ||
		strings.Contains(line, "accepted") ||
		strings.Contains(line, "tunneling") ||
		strings.HasPrefix(line, "202") // 日期开头
}

func (p *XrayParser) Parse(line string) (level, category, message string) {
	level = LevelInfo
	category = CategoryXray

	switch {
	case strings.Contains(line, "error") || strings.Contains(line, "failed"):
		level = LevelError
	case strings.Contains(line, "warning"):
		level = LevelWarn
	}

	// 简化消息
	message = line

	// 尝试提取关键信息
	if strings.Contains(line, "accepted") {
		message = "接受新连接"
	} else if strings.Contains(line, "tunneling") {
		// 提取目标地址
		if idx := strings.Index(line, "tunneling"); idx != -1 {
			rest := line[idx:]
			message = rest
		}
	}

	return
}

// =============================================================================
// 回调与控制
// =============================================================================

// SetCallback 设置新日志回调
func (m *Manager) SetCallback(cb func(entry models.LogEntry)) {
	m.onNewLog = cb
}

// flushLoop 刷新循环
func (m *Manager) flushLoop() {
	m.flushTicker = time.NewTicker(FlushInterval)
	defer m.flushTicker.Stop()

	for {
		select {
		case <-m.flushTicker.C:
			if m.logFile != nil {
				m.logFile.Sync()
			}
		case <-m.stopChan:
			return
		}
	}
}

// Stop 停止日志管理器
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return
	}

	m.stopped = true
	close(m.stopChan)

	if m.logFile != nil {
		m.logFile.Sync()
		m.logFile.Close()
		m.logFile = nil
	}
}

// =============================================================================
// 日志导出
// =============================================================================

// ExportToFile 导出日志到文件
func (m *Manager) ExportToFile(path string, format string) error {
	logs := m.GetLogs(BufferSize)

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	switch format {
	case "json":
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(logs)

	case "csv":
		writer.WriteString("时间,节点,级别,类别,消息\n")
		for _, log := range logs {
			line := fmt.Sprintf("%s,%s,%s,%s,%s\n",
				log.Timestamp.Format("2006-01-02 15:04:05"),
				log.NodeName,
				log.Level,
				log.Category,
				strings.ReplaceAll(log.Message, ",", "，"),
			)
			writer.WriteString(line)
		}
		return nil

	default: // txt
		for _, log := range logs {
			line := fmt.Sprintf("[%s] [%s] [%s] [%s] %s\n",
				log.Timestamp.Format("2006-01-02 15:04:05"),
				log.NodeName,
				log.Level,
				log.Category,
				log.Message,
			)
			writer.WriteString(line)
		}
		return nil
	}
}

// GetLogFilePath 获取当前日志文件路径
func (m *Manager) GetLogFilePath() string {
	return m.logFilePath
}

// GetLogDir 获取日志目录
func (m *Manager) GetLogDir() string {
	return filepath.Join(m.exeDir, LogDirName)
}
