// Package models 定义所有数据结构
package models

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// 常量定义
// =============================================================================

const (
	AppVersion = "22.0.0"
	AppTitle   = "Xlink客户端 v" + AppVersion

	MaxNodes    = 50
	MaxRules    = 300
	MaxNameLen  = 128
	MaxURLLen   = 8192
	MaxRulesLen = 16384
)

// 路由模式
const (
	RoutingModeGlobal = 0 // 全局代理
	RoutingModeSmart  = 1 // 智能分流
)

// 负载均衡策略
const (
	StrategyRandom = 0 // 随机
	StrategyRR     = 1 // 轮询
	StrategyHash   = 2 // 哈希
)

// DNS 防泄露模式
const (
	DNSModeStandard = 0 // 标准模式（可能泄露）
	DNSModeFakeIP   = 1 // Fake-IP 模式（推荐）
	DNSModeTUN      = 2 // TUN 全局接管（最安全）
)

// 节点运行状态
const (
	StatusStopped  = "stopped"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusError    = "error"
)

// =============================================================================
// 核心数据结构
// =============================================================================

// RoutingRule 单条分流规则
type RoutingRule struct {
	ID     string `json:"id"`     // 唯一ID (UUID)
	Type   string `json:"type"`   // 类型: "", "domain:", "regexp:", "geosite:", "geoip:"
	Match  string `json:"match"`  // 匹配内容
	Target string `json:"target"` // 目标节点
}

// NodeConfig 单个节点的完整配置
type NodeConfig struct {
	// 基本信息
	ID   string `json:"id"`   // 唯一ID (UUID)
	Name string `json:"name"` // 节点别名

	// 连接配置
	Listen     string `json:"listen"`      // 本地监听地址 (如 127.0.0.1:10808)
	Server     string `json:"server"`      // 服务器地址池 (多个用换行或分号分隔)
	IP         string `json:"ip"`          // 全局指定IP
	Token      string `json:"token"`       // 认证Token
	SecretKey  string `json:"secret_key"`  // 加密密钥
	FallbackIP string `json:"fallback_ip"` // 回源IP
	Socks5     string `json:"socks5"`      // 上游SOCKS5代理

	// 路由与策略
	RoutingMode  int `json:"routing_mode"`  // 路由模式
	StrategyMode int `json:"strategy_mode"` // 负载策略

	// DNS 防泄露配置
	DNSMode        int    `json:"dns_mode"`        // DNS模式
	CustomDNS      string `json:"custom_dns"`      // 自定义DNS服务器
	EnableSniffing bool   `json:"enable_sniffing"` // 启用流量嗅探

	// 分流规则
	Rules []RoutingRule `json:"rules"`

	// 运行时状态 (不持久化)
	Status       string `json:"-"` // 运行状态
	InternalPort int    `json:"-"` // 内部端口（智能分流时使用）

	// 已弃用字段兼容
	RulesStr string `json:"rules_str,omitempty"` // 旧版规则字符串
}

// AppConfig 全局应用配置
type AppConfig struct {
	Nodes          []NodeConfig `json:"nodes"`            // 所有节点
	AutoStart      bool         `json:"auto_start"`       // 开机自启
	MinimizeToTray bool         `json:"minimize_to_tray"` // 最小化到托盘
	Theme          string       `json:"theme"`            // 主题: "light", "dark", "system"
	Language       string       `json:"language"`         // 语言: "zh-CN", "en-US"

	// DNS 全局设置
	GlobalDNSMode    int    `json:"global_dns_mode"`    // 全局DNS模式
	TUNInterfaceName string `json:"tun_interface_name"` // TUN网卡名称

	// 🚀【核心新增】记录上次运行的节点 ID，实现自动恢复
	LastRunningNodeID string `json:"last_running_node_id"`
}

// =============================================================================
// 运行时状态结构
// =============================================================================

// EngineStatus 引擎运行状态
type EngineStatus struct {
	NodeID       string    `json:"node_id"`
	Status       string    `json:"status"`
	StartTime    time.Time `json:"start_time"`
	PID          int       `json:"pid"`
	XrayPID      int       `json:"xray_pid,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

// LogEntry 日志条目
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	NodeID    string    `json:"node_id"`
	NodeName  string    `json:"node_name"`
	Level     string    `json:"level"`    // "info", "warn", "error", "debug"
	Category  string    `json:"category"` // "系统", "内核", "规则", "负载", "统计", "测速"
	Message   string    `json:"message"`
}

// LogFilter 日志过滤选项
type LogFilter struct {
	NodeID     string     `json:"node_id,omitempty"`
	Levels     []string   `json:"levels,omitempty"`
	Categories []string   `json:"categories,omitempty"`
	Search     string     `json:"search,omitempty"`
	StartTime  *time.Time `json:"start_time,omitempty"`
	EndTime    *time.Time `json:"end_time,omitempty"`
	Limit      int        `json:"limit,omitempty"`
}

// PingResult 延迟测试结果
type PingResult struct {
	Server  string `json:"server"`
	Latency int    `json:"latency"` // 毫秒, -1 表示失败
	Error   string `json:"error,omitempty"`
}

// PingStatus Ping状态
type PingStatus struct {
	IsRunning   bool   `json:"is_running"`
	NodeID      string `json:"node_id"`
	StartTime   string `json:"start_time"`
	TestedCount int    `json:"tested_count"`
	TotalCount  int    `json:"total_count"`
}

// =============================================================================
// 前后端通信事件
// =============================================================================

// EventType 事件类型
type EventType string

const (
	EventLogAppend         EventType = "log:append"
	EventNodeStatus        EventType = "node:status"
	EventPingResult        EventType = "ping:result"
	EventPingComplete      EventType = "ping:complete"
	EventPingBatchProgress EventType = "ping:batch:progress"
	EventPingBatchComplete EventType = "ping:batch:complete"
	EventConfigChanged     EventType = "config:changed"
)

// Event 前后端事件结构
type Event struct {
	Type    EventType   `json:"type"`
	Payload interface{} `json:"payload"`
}

// =============================================================================
// 应用状态管理器
// =============================================================================

// AppState 全局应用状态（线程安全）
type AppState struct {
	Mu             sync.RWMutex
	Config         *AppConfig
	EngineStatuses map[string]*EngineStatus // key: NodeID
	CurrentNodeID  string
	ExeDir         string
	IsAutoStart    bool // 是否由开机自启触发
}

// NewAppState 创建新的应用状态
func NewAppState() *AppState {
	return &AppState{
		Config: &AppConfig{
			Nodes:            make([]NodeConfig, 0),
			Theme:            "system",
			Language:         "zh-CN",
			MinimizeToTray:   true,
			GlobalDNSMode:    DNSModeFakeIP,
			TUNInterfaceName: "XlinkTUN",
		},
		EngineStatuses: make(map[string]*EngineStatus),
	}
}

// GetNode 获取节点（线程安全）
func (s *AppState) GetNode(id string) *NodeConfig {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	for i := range s.Config.Nodes {
		if s.Config.Nodes[i].ID == id {
			return &s.Config.Nodes[i]
		}
	}
	return nil
}

// GetNodeByIndex 通过索引获取节点
func (s *AppState) GetNodeByIndex(index int) *NodeConfig {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	if index >= 0 && index < len(s.Config.Nodes) {
		return &s.Config.Nodes[index]
	}
	return nil
}

// UpdateNodeStatus 更新节点状态
func (s *AppState) UpdateNodeStatus(nodeID, status string, errMsg string) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if es, ok := s.EngineStatuses[nodeID]; ok {
		es.Status = status
		es.ErrorMessage = errMsg
	} else {
		s.EngineStatuses[nodeID] = &EngineStatus{
			NodeID:       nodeID,
			Status:       status,
			ErrorMessage: errMsg,
		}
	}
	// 同步到节点配置 (内存中)
	for i := range s.Config.Nodes {
		if s.Config.Nodes[i].ID == nodeID {
			s.Config.Nodes[i].Status = status
			break
		}
	}
}

// =============================================================================
// 工具函数
// =============================================================================

// NewDefaultNode 创建默认节点配置
func NewDefaultNode(name string) NodeConfig {
	return NodeConfig{
		ID:             GenerateUUID(),
		Name:           name,
		Listen:         "127.0.0.1:10808",
		Server:         "cdn.worker.dev:443",
		Token:          "my-password",
		SecretKey:      "my-secret-key-888",
		RoutingMode:    RoutingModeGlobal,
		StrategyMode:   StrategyRandom,
		DNSMode:        DNSModeFakeIP,
		EnableSniffing: true,
		Rules:          make([]RoutingRule, 0),
		Status:         StatusStopped,
	}
}

// GenerateUUID 生成UUID v4
func GenerateUUID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// 回退到时间戳方式
		return time.Now().Format("20060102150405") + "-" + randomHex(8)
	}

	// 设置版本号(v4)和变体
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// randomHex 生成随机十六进制字符串
func randomHex(n int) string {
	b := make([]byte, n/2+1)
	rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

// GetStrategyString 获取策略字符串
func GetStrategyString(mode int) string {
	switch mode {
	case StrategyRR:
		return "rr"
	case StrategyHash:
		return "hash"
	default:
		return "random"
	}
}

// GetDNSModeString 获取DNS模式描述
func GetDNSModeString(mode int) string {
	switch mode {
	case DNSModeFakeIP:
		return "Fake-IP (推荐)"
	case DNSModeTUN:
		return "TUN 全局接管"
	default:
		return "标准模式"
	}
}
