// Package generator 处理配置文件的生成、验证和清理
package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"xlink-wails/internal/models"
)

// =============================================================================
// 常量定义
// =============================================================================

const (
	// XlinkConfigTemplate Xlink 核心配置文件名模板
	XlinkConfigTemplate = "config_core_%s.json"
	
	// XrayConfigTemplate Xray 前端配置文件名模板
	// 虽然生成逻辑在 dns 包，但文件名约定保留在此处
	XrayConfigTemplate = "config_xray_%s.json"
)

// =============================================================================
// 预设规则定义 (原 templates.go 内容整合)
// =============================================================================

// PresetRules 预设规则集
var PresetRules = map[string][]string{
	// 广告拦截
	"block-ads": {
		"geosite:category-ads-all,block",
		"geosite:category-ads,block",
	},

	// 中国直连
	"direct-cn": {
		"geosite:cn,direct",
		"geoip:cn,direct",
		"geosite:geolocation-cn,direct",
	},

	// 常用代理
	"proxy-common": {
		"geosite:google,proxy",
		"geosite:youtube,proxy",
		"geosite:twitter,proxy",
		"geosite:facebook,proxy",
		"geosite:telegram,proxy",
		"geosite:github,proxy",
		"geosite:openai,proxy",
	},

	// 流媒体
	"proxy-streaming": {
		"geosite:netflix,proxy",
		"geosite:disney,proxy",
		"geosite:hbo,proxy",
		"geosite:spotify,proxy",
		"geosite:tiktok,proxy",
	},

	// 隐私保护
	"privacy": {
		"geosite:category-porn,block",
		"geosite:category-gambling,block",
	},
}

// DNSModeDescriptions DNS模式说明文本
var DNSModeDescriptions = map[int]string{
	models.DNSModeStandard: `标准模式 (可能泄露DNS)
- 使用系统默认DNS
- 分流依赖IP规则
- 适合对隐私要求不高的场景`,

	models.DNSModeFakeIP: `Fake-IP 模式 (推荐)
- 本地返回虚假IP
- 真实域名通过代理解析
- 有效防止DNS泄露
- 兼容性好`,

	models.DNSModeTUN: `TUN 全局接管 (最安全)
- 创建虚拟网卡接管所有流量
- 完全杜绝DNS泄露
- 需要管理员权限`,
}

// =============================================================================
// 生成器结构体
// =============================================================================

// Generator 配置生成器
type Generator struct {
	exeDir string
}

// NewGenerator 创建配置生成器
func NewGenerator(exeDir string) *Generator {
	return &Generator{
		exeDir: exeDir,
	}
}

// =============================================================================
// Xlink 配置结构 (JSON 映射)
// =============================================================================

// XlinkConfig Xlink核心配置
type XlinkConfig struct {
	Inbounds  []XlinkInbound  `json:"inbounds"`
	Outbounds []XlinkOutbound `json:"outbounds"`
}

// XlinkInbound 入站配置
type XlinkInbound struct {
	Tag      string `json:"tag"`
	Listen   string `json:"listen"`
	Protocol string `json:"protocol"`
}

// XlinkOutbound 出站配置
type XlinkOutbound struct {
	Tag      string              `json:"tag"`
	Protocol string              `json:"protocol"`
	Settings XlinkProxySettings  `json:"settings"`
}

// XlinkProxySettings 代理设置
type XlinkProxySettings struct {
	Server          string `json:"server"`
	ServerIP        string `json:"server_ip,omitempty"`
	Token           string `json:"token"`
	Strategy        string `json:"strategy"`
	Rules           string `json:"rules,omitempty"`
	GlobalKeepAlive bool   `json:"global_keep_alive"`
	S5              string `json:"s5,omitempty"`
}

// =============================================================================
// 核心生成逻辑
// =============================================================================

// GenerateXlinkConfig 生成 Xlink 核心配置文件
func (g *Generator) GenerateXlinkConfig(node *models.NodeConfig, listenAddr string) (string, error) {
	// 配置文件路径
	configPath := filepath.Join(g.exeDir, fmt.Sprintf(XlinkConfigTemplate, node.ID))

	// 1. 准备服务器列表（将换行转为分号）
	servers := normalizeServerList(node.Server)

	// 2. 准备Token（包含回源IP）
	token := buildTokenString(node.SecretKey, node.FallbackIP)

	// 3. 获取策略字符串
	strategy := models.GetStrategyString(node.StrategyMode)

	// 4. 序列化规则 (注意：Xlink 使用 \r\n 分隔规则)
	rules := serializeRules(node.Rules)

	// 5. 构建配置对象
	config := XlinkConfig{
		Inbounds: []XlinkInbound{
			{
				Tag:      "socks-in",
				Listen:   listenAddr,
				Protocol: "socks",
			},
		},
		Outbounds: []XlinkOutbound{
			{
				Tag:      "proxy",
				Protocol: "ech-proxy",
				Settings: XlinkProxySettings{
					Server:          servers,
					ServerIP:        node.IP,
					Token:           token,
					Strategy:        strategy,
					Rules:           rules,
					GlobalKeepAlive: false,
					S5:              node.Socks5,
				},
			},
		},
	}

	// 6. 序列化并写入文件
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return "", fmt.Errorf("写入配置文件失败: %w", err)
	}

	return configPath, nil
}

// =============================================================================
// 辅助方法 (Preset & Validation)
// =============================================================================

// GetPresetRules 获取预设规则
func GetPresetRules(presetName string) []string {
	if rules, ok := PresetRules[presetName]; ok {
		return rules
	}
	return nil
}

// ValidateNodeConfig 验证节点配置参数的合法性
func (g *Generator) ValidateNodeConfig(node *models.NodeConfig) error {
	if node.Listen == "" {
		return fmt.Errorf("监听地址不能为空")
	}

	if node.Server == "" {
		return fmt.Errorf("服务器地址不能为空")
	}

	// 验证监听地址格式 (host:port)
	if !strings.Contains(node.Listen, ":") {
		return fmt.Errorf("监听地址格式错误，应为 host:port")
	}

	// 验证端口范围
	_, port := parseListenAddr(node.Listen)
	if port < 1 || port > 65535 {
		return fmt.Errorf("端口号超出范围 (1-65535)")
	}

	return nil
}

// =============================================================================
// 文件清理
// =============================================================================

// CleanupConfigs 清理指定节点的临时配置文件
func (g *Generator) CleanupConfigs(nodeID string) error {
	xlinkPath := filepath.Join(g.exeDir, fmt.Sprintf(XlinkConfigTemplate, nodeID))
	xrayPath := filepath.Join(g.exeDir, fmt.Sprintf(XrayConfigTemplate, nodeID))

	// 忽略删除错误（文件可能不存在）
	os.Remove(xlinkPath)
	os.Remove(xrayPath)

	return nil
}

// CleanupAllConfigs 清理所有以 config_ 开头的临时 JSON 文件
func (g *Generator) CleanupAllConfigs() error {
	pattern := filepath.Join(g.exeDir, "config_*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	for _, f := range files {
		os.Remove(f)
	}

	return nil
}

// =============================================================================
// 内部工具函数
// =============================================================================

// normalizeServerList 规范化服务器列表字符串
func normalizeServerList(servers string) string {
	// 将各种换行符统一转为分号
	result := strings.ReplaceAll(servers, "\r\n", ";")
	result = strings.ReplaceAll(result, "\n", ";")
	result = strings.ReplaceAll(result, "\r", ";")

	// 移除重复的分号
	for strings.Contains(result, ";;") {
		result = strings.ReplaceAll(result, ";;", ";")
	}
	// 移除首尾分号
	result = strings.Trim(result, ";")

	return result
}

// buildTokenString 构建 Xlink Token 字符串 (Key|Fallback)
func buildTokenString(secretKey, fallbackIP string) string {
	if fallbackIP == "" {
		return secretKey
	}
	return secretKey + "|" + fallbackIP
}

// serializeRules 序列化用户规则为 Xlink 字符串格式
func serializeRules(rules []models.RoutingRule) string {
	if len(rules) == 0 {
		return ""
	}

	var lines []string
	for _, r := range rules {
		// 格式: type:match,target (例如 geosite:google,proxy)
		line := r.Type + r.Match + "," + r.Target
		lines = append(lines, line)
	}

	// 使用转义的换行符 \r\n，因为这将被放入 JSON 字符串中
	return strings.Join(lines, "\\r\\n")
}

// parseListenAddr 解析 host:port 字符串
func parseListenAddr(addr string) (host string, port int) {
	host = "127.0.0.1"
	port = 10808

	if addr == "" {
		return
	}

	idx := strings.LastIndex(addr, ":")
	if idx == -1 {
		return
	}

	host = addr[:idx]
	// 忽略错误，使用默认值
	fmt.Sscanf(addr[idx+1:], "%d", &port)

	return
}
