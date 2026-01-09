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
	XlinkConfigTemplate = "config_core_%s.json"
	XrayConfigTemplate  = "config_xray_%s.json"
)

// =============================================================================
// 预设规则定义
// =============================================================================

var PresetRules = map[string][]string{
	"block-ads": {
		"geosite:category-ads-all,block",
		"geosite:category-ads,block",
	},
	"direct-cn": {
		"geosite:cn,direct",
		"geoip:cn,direct",
		"geosite:geolocation-cn,direct",
	},
	"proxy-common": {
		"geosite:google,proxy",
		"geosite:youtube,proxy",
		"geosite:twitter,proxy",
		"geosite:facebook,proxy",
		"geosite:telegram,proxy",
		"geosite:github,proxy",
		"geosite:openai,proxy",
	},
	"proxy-streaming": {
		"geosite:netflix,proxy",
		"geosite:disney,proxy",
		"geosite:hbo,proxy",
		"geosite:spotify,proxy",
		"geosite:tiktok,proxy",
	},
	"privacy": {
		"geosite:category-porn,block",
		"geosite:category-gambling,block",
	},
}

var DNSModeDescriptions = map[int]string{
	models.DNSModeStandard: "标准模式 (可能泄露DNS)\n- 使用系统默认DNS\n- 分流依赖IP规则",
	models.DNSModeFakeIP:   "Fake-IP 模式 (推荐)\n- 本地返回虚假IP\n- 真实域名通过代理解析\n- 有效防止DNS泄露",
	models.DNSModeTUN:      "TUN 全局接管 (最安全)\n- 创建虚拟网卡接管所有流量\n- 完全杜绝DNS泄露\n- 需要管理员权限",
}

// =============================================================================
// 生成器结构体
// =============================================================================

type Generator struct {
	exeDir string
}

func NewGenerator(exeDir string) *Generator {
	return &Generator{exeDir: exeDir}
}

// =============================================================================
// Xlink 配置结构
// =============================================================================

type XlinkConfig struct {
	Inbounds  []XlinkInbound  `json:"inbounds"`
	Outbounds []XlinkOutbound `json:"outbounds"`
}

type XlinkInbound struct {
	Tag      string `json:"tag"`
	Listen   string `json:"listen"`
	Protocol string `json:"protocol"`
}

type XlinkOutbound struct {
	Tag      string              `json:"tag"`
	Protocol string              `json:"protocol"`
	Settings XlinkProxySettings  `json:"settings"`
}

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

func (g *Generator) GenerateXlinkConfig(node *models.NodeConfig, listenAddr string) (string, error) {
	configPath := filepath.Join(g.exeDir, fmt.Sprintf(XlinkConfigTemplate, node.ID))

	servers := normalizeServerList(node.Server)

	// ⚠️【核心修复】
	// 之前错误地使用了 SecretKey 作为 Token。
	// 正确逻辑：优先使用 Token (认证密码)。如果 Token 为空，才尝试使用 SecretKey (兼容性)。
	// 格式：Token|FallbackIP
	mainToken := node.Token
	if mainToken == "" {
		mainToken = node.SecretKey
	}
	tokenStr := buildTokenString(mainToken, node.FallbackIP)

	strategy := models.GetStrategyString(node.StrategyMode)
	rules := serializeRules(node.Rules)

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
					Token:           tokenStr, // 修复后的 Token
					Strategy:        strategy,
					Rules:           rules,
					GlobalKeepAlive: false,
					S5:              node.Socks5,
				},
			},
		},
	}

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
// 辅助方法
// =============================================================================

func GetPresetRules(presetName string) []string {
	if rules, ok := PresetRules[presetName]; ok {
		return rules
	}
	return nil
}

func (g *Generator) ValidateNodeConfig(node *models.NodeConfig) error {
	if node.Listen == "" {
		return fmt.Errorf("监听地址不能为空")
	}
	if node.Server == "" {
		return fmt.Errorf("服务器地址不能为空")
	}
	if !strings.Contains(node.Listen, ":") {
		return fmt.Errorf("监听地址格式错误，应为 host:port")
	}
	return nil
}

func (g *Generator) CleanupConfigs(nodeID string) error {
	xlinkPath := filepath.Join(g.exeDir, fmt.Sprintf(XlinkConfigTemplate, nodeID))
	xrayPath := filepath.Join(g.exeDir, fmt.Sprintf(XrayConfigTemplate, nodeID))
	os.Remove(xlinkPath)
	os.Remove(xrayPath)
	return nil
}

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
// 内部工具
// =============================================================================

func normalizeServerList(servers string) string {
	result := strings.ReplaceAll(servers, "\r\n", ";")
	result = strings.ReplaceAll(result, "\n", ";")
	result = strings.ReplaceAll(result, "\r", ";")
	result = strings.ReplaceAll(result, "，", ";")
	result = strings.ReplaceAll(result, ",", ";")
	
	for strings.Contains(result, ";;") {
		result = strings.ReplaceAll(result, ";;", ";")
	}
	return strings.Trim(result, ";")
}

func buildTokenString(token, fallbackIP string) string {
	if fallbackIP == "" {
		return token
	}
	return token + "|" + fallbackIP
}

func serializeRules(rules []models.RoutingRule) string {
	if len(rules) == 0 {
		return ""
	}
	var lines []string
	for _, r := range rules {
		line := r.Type + r.Match + "," + r.Target
		lines = append(lines, line)
	}
	return strings.Join(lines, "\\r\\n")
}
