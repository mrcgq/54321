// Package dns 提供DNS防泄露功能
package dns

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"xlink-wails/internal/models"
)

// =============================================================================
// 常量定义
// =============================================================================

const (
	// Fake-IP 地址池
	FakeIPPoolStart = "198.18.0.0"
	FakeIPPoolCIDR  = "198.18.0.0/15" // 198.18.0.0 - 198.19.255.255
	FakeIPPoolSize  = 65535

	// DNS服务器
	DNSCloudflare    = "1.1.1.1"
	DNSCloudflareDoH = "https://1.1.1.1/dns-query"
	DNSGoogle        = "8.8.8.8"
	DNSGoogleDoH     = "https://dns.google/dns-query"
	DNSAliDNS        = "223.5.5.5"
	DNSTencent       = "119.29.29.29"

	// TUN配置
	DefaultTUNName = "XlinkTUN"
	DefaultTUNMTU  = 9000
)

// =============================================================================
// DNS管理器
// =============================================================================

// Manager DNS防泄露管理器
type Manager struct {
	mu sync.RWMutex

	exeDir   string
	mode     int // DNS模式
	tunName  string
	isActive bool

	// Fake-IP 映射表
	fakeIPMap     map[string]string // domain -> fake IP
	reverseFakeIP map[string]string // fake IP -> domain
	nextFakeIP    uint32

	// 原始系统DNS（用于恢复）
	originalDNS []string

	// 日志回调
	logCallback func(level, message string)
}

// NewManager 创建DNS管理器
func NewManager(exeDir string) *Manager {
	return &Manager{
		exeDir:        exeDir,
		tunName:       DefaultTUNName,
		fakeIPMap:     make(map[string]string),
		reverseFakeIP: make(map[string]string),
		nextFakeIP:    ipToUint32(net.ParseIP(FakeIPPoolStart)),
	}
}

// SetLogCallback 设置日志回调
func (m *Manager) SetLogCallback(cb func(level, message string)) {
	m.logCallback = cb
}

// log 记录日志
func (m *Manager) log(level, message string) {
	if m.logCallback != nil {
		m.logCallback(level, message)
	}
}

// =============================================================================
// DNS模式配置
// =============================================================================

// DNSConfig DNS配置
type DNSConfig struct {
	Mode            int      `json:"mode"`
	CustomUpstream  []string `json:"custom_upstream,omitempty"`
	EnableFakeIP    bool     `json:"enable_fake_ip"`
	FakeIPFilter    []string `json:"fake_ip_filter,omitempty"` // 不使用Fake-IP的域名
	EnableSniffing  bool     `json:"enable_sniffing"`
	EnableTUN       bool     `json:"enable_tun"`
	TUNName         string   `json:"tun_name,omitempty"`
	TUNMTU          int      `json:"tun_mtu,omitempty"`
	HijackDNS       bool     `json:"hijack_dns"`
	BlockAds        bool     `json:"block_ads"`
}

// DefaultDNSConfig 默认DNS配置
func DefaultDNSConfig() *DNSConfig {
	return &DNSConfig{
		Mode:           models.DNSModeFakeIP,
		EnableFakeIP:   true,
		EnableSniffing: true,
		EnableTUN:      false,
		TUNName:        DefaultTUNName,
		TUNMTU:         DefaultTUNMTU,
		HijackDNS:      true,
		BlockAds:       true,
		FakeIPFilter: []string{
			// 这些域名不使用Fake-IP，直接解析
			"+.lan",
			"+.local",
			"+.localhost",
			"localhost",
			"*.localdomain",
			// Windows网络检测
			"dns.msftncsi.com",
			"www.msftncsi.com",
			"www.msftconnecttest.com",
			// 时间同步
			"time.windows.com",
			"time.nist.gov",
			"pool.ntp.org",
		},
	}
}

// =============================================================================
// Xray DNS配置生成
// =============================================================================

// XrayDNSConfig Xray的DNS配置结构
type XrayDNSConfig struct {
	Hosts           map[string]interface{} `json:"hosts,omitempty"`
	Servers         []interface{}          `json:"servers"`
	ClientIP        string                 `json:"clientIp,omitempty"`
	QueryStrategy   string                 `json:"queryStrategy,omitempty"`
	DisableCache    bool                   `json:"disableCache,omitempty"`
	DisableFallback bool                   `json:"disableFallback,omitempty"`
	Tag             string                 `json:"tag,omitempty"`
}

// XrayDNSServer DNS服务器配置
type XrayDNSServer struct {
	Address      string   `json:"address"`
	Port         int      `json:"port,omitempty"`
	Domains      []string `json:"domains,omitempty"`
	ExpectIPs    []string `json:"expectIPs,omitempty"`
	SkipFallback bool     `json:"skipFallback,omitempty"`
	ClientIP     string   `json:"clientIp,omitempty"`
}

// GenerateXrayDNSConfig 生成Xray DNS配置
func (m *Manager) GenerateXrayDNSConfig(cfg *DNSConfig, hasGeosite, hasGeoip bool) *XrayDNSConfig {
	dnsConfig := &XrayDNSConfig{
		Hosts: map[string]interface{}{
			"localhost": "127.0.0.1",
		},
		QueryStrategy:   "UseIP",
		DisableCache:    false,
		DisableFallback: false,
		Tag:             "dns-internal",
	}

	switch cfg.Mode {
	case models.DNSModeFakeIP:
		// Fake-IP 模式：使用FakeDNS + 远程DNS
		dnsConfig.Servers = m.buildFakeIPDNSServers(hasGeosite)
		dnsConfig.DisableFallback = true

	case models.DNSModeTUN:
		// TUN模式：完全远程解析
		dnsConfig.Servers = m.buildRemoteDNSServers(hasGeosite)
		dnsConfig.DisableFallback = true

	default:
		// 标准模式：分流DNS
		dnsConfig.Servers = m.buildSplitDNSServers(hasGeosite, hasGeoip)
	}

	// 添加广告拦截hosts
	if cfg.BlockAds && hasGeosite {
		dnsConfig.Hosts["geosite:category-ads-all"] = "127.0.0.1"
	}

	return dnsConfig
}

// buildFakeIPDNSServers 构建Fake-IP模式DNS服务器列表
func (m *Manager) buildFakeIPDNSServers(hasGeosite bool) []interface{} {
	servers := []interface{}{}

	// FakeDNS优先
	servers = append(servers, "fakedns")

	// 远程DNS作为后备（通过代理）
	remoteServer := XrayDNSServer{
		Address:      DNSCloudflareDoH,
		SkipFallback: false,
	}

	if hasGeosite {
		// 只对非中国域名使用远程DNS
		remoteServer.Domains = []string{
			"geosite:geolocation-!cn",
		}
	}

	servers = append(servers, remoteServer)

	return servers
}

// buildRemoteDNSServers 构建远程DNS服务器列表
func (m *Manager) buildRemoteDNSServers(hasGeosite bool) []interface{} {
	servers := []interface{}{}

	// 主DNS：Cloudflare DoH
	primaryServer := XrayDNSServer{
		Address:      DNSCloudflareDoH,
		SkipFallback: false,
	}

	if hasGeosite {
		primaryServer.Domains = []string{
			"geosite:geolocation-!cn",
		}
	}

	servers = append(servers, primaryServer)

	// 备用DNS：Google DoH
	servers = append(servers, XrayDNSServer{
		Address: DNSGoogleDoH,
	})

	return servers
}

// buildSplitDNSServers 构建分流DNS服务器列表
func (m *Manager) buildSplitDNSServers(hasGeosite, hasGeoip bool) []interface{} {
	servers := []interface{}{}

	// 国内域名使用国内DNS
	if hasGeosite && hasGeoip {
		servers = append(servers, XrayDNSServer{
			Address: DNSAliDNS,
			Port:    53,
			Domains: []string{
				"geosite:cn",
				"geosite:geolocation-cn",
				"geosite:tld-cn",
			},
			ExpectIPs: []string{
				"geoip:cn",
			},
			SkipFallback: true,
		})

		// 备用国内DNS
		servers = append(servers, XrayDNSServer{
			Address: DNSTencent,
			Port:    53,
			Domains: []string{
				"geosite:cn",
			},
			SkipFallback: true,
		})
	}

	// 国外域名使用国外DNS（通过代理）
	servers = append(servers, XrayDNSServer{
		Address: DNSCloudflareDoH,
	})

	// 最终后备
	servers = append(servers, DNSGoogle)

	return servers
}

// =============================================================================
// FakeDNS配置生成
// =============================================================================

// XrayFakeDNSConfig FakeDNS配置
type XrayFakeDNSConfig struct {
	IPPool   string `json:"ipPool"`
	PoolSize int    `json:"poolSize"`
}

// GenerateFakeDNSConfig 生成FakeDNS配置
func (m *Manager) GenerateFakeDNSConfig() *XrayFakeDNSConfig {
	return &XrayFakeDNSConfig{
		IPPool:   FakeIPPoolCIDR,
		PoolSize: FakeIPPoolSize,
	}
}

// =============================================================================
// 流量嗅探配置
// =============================================================================

// XraySniffingConfig 流量嗅探配置
type XraySniffingConfig struct {
	Enabled         bool     `json:"enabled"`
	DestOverride    []string `json:"destOverride"`
	MetadataOnly    bool     `json:"metadataOnly"`
	RouteOnly       bool     `json:"routeOnly"`
	DomainsExcluded []string `json:"domainsExcluded,omitempty"`
}

// GenerateSniffingConfig 生成嗅探配置
func (m *Manager) GenerateSniffingConfig(cfg *DNSConfig) *XraySniffingConfig {
	if !cfg.EnableSniffing {
		return nil
	}

	return &XraySniffingConfig{
		Enabled: true,
		DestOverride: []string{
			"http",
			"tls",
			"quic",
			"fakedns", // 重要：嗅探FakeDNS
		},
		MetadataOnly: false,
		RouteOnly:    false, // false = 使用嗅探到的域名替换目标地址
		DomainsExcluded: []string{
			// 排除某些域名不被嗅探替换
			"courier.push.apple.com", // Apple推送
		},
	}
}

// =============================================================================
// TUN模式配置
// =============================================================================

// TUNConfig TUN网卡配置
type TUNConfig struct {
	Enable              bool     `json:"enable"`
	Stack               string   `json:"stack"`               // gvisor, system, lwip
	Device              string   `json:"device"`              // 网卡名称
	AutoRoute           bool     `json:"auto-route"`          // 自动配置路由
	AutoDetectInterface bool     `json:"auto-detect-interface"` // 自动检测出口网卡
	DNSHijack           []string `json:"dns-hijack"`          // DNS劫持
	MTU                 int      `json:"mtu"`
	StrictRoute         bool     `json:"strict-route"`        // 严格路由模式
	Inet4Address        []string `json:"inet4-address,omitempty"`
	Inet6Address        []string `json:"inet6-address,omitempty"`
	EndpointIndependentNat bool  `json:"endpoint-independent-nat,omitempty"`
}

// GenerateTUNConfig 生成TUN配置
func (m *Manager) GenerateTUNConfig(cfg *DNSConfig) *TUNConfig {
	if !cfg.EnableTUN {
		return nil
	}

	tunName := cfg.TUNName
	if tunName == "" {
		tunName = DefaultTUNName
	}

	mtu := cfg.TUNMTU
	if mtu <= 0 {
		mtu = DefaultTUNMTU
	}

	tunCfg := &TUNConfig{
		Enable:              true,
		Stack:               "gvisor", // 推荐gvisor，兼容性好
		Device:              tunName,
		AutoRoute:           true,
		AutoDetectInterface: true,
		MTU:                 mtu,
		StrictRoute:         false,
		Inet4Address:        []string{"198.18.0.1/16"}, // TUN网卡IP
		EndpointIndependentNat: true,
	}

	// DNS劫持
	if cfg.HijackDNS {
		tunCfg.DNSHijack = []string{
			"any:53",        // UDP DNS
			"tcp://any:53",  // TCP DNS
		}
	}

	return tunCfg
}

// =============================================================================
// 完整Xray配置生成
// =============================================================================

// XrayFullConfig 完整的Xray配置
type XrayFullConfig struct {
	Log       map[string]interface{}   `json:"log"`
	DNS       *XrayDNSConfig           `json:"dns,omitempty"`
	FakeDNS   interface{}              `json:"fakedns,omitempty"`
	Inbounds  []map[string]interface{} `json:"inbounds"`
	Outbounds []map[string]interface{} `json:"outbounds"`
	Routing   map[string]interface{}   `json:"routing"`
}

// GenerateFullXrayConfig 生成完整的Xray配置
func (m *Manager) GenerateFullXrayConfig(
	node *models.NodeConfig,
	xlinkPort int,
	hasGeosite, hasGeoip bool,
) (*XrayFullConfig, error) {

	dnsCfg := &DNSConfig{
		Mode:           node.DNSMode,
		EnableFakeIP:   node.DNSMode == models.DNSModeFakeIP,
		EnableSniffing: node.EnableSniffing,
		EnableTUN:      node.DNSMode == models.DNSModeTUN,
		HijackDNS:      true,
		BlockAds:       true,
	}

	// 解析监听地址
	listenHost, listenPort := m.parseListenAddr(node.Listen)

	config := &XrayFullConfig{
		Log: map[string]interface{}{
			"loglevel": "warning",
		},
	}

	// DNS配置
	config.DNS = m.GenerateXrayDNSConfig(dnsCfg, hasGeosite, hasGeoip)

	// FakeDNS配置
	if dnsCfg.EnableFakeIP {
		config.FakeDNS = []interface{}{
			m.GenerateFakeDNSConfig(),
		}
	}

	// 入站配置
	inbound := map[string]interface{}{
		"tag":      "socks-in",
		"listen":   listenHost,
		"port":     listenPort,
		"protocol": "socks",
		"settings": map[string]interface{}{
			"auth": "noauth",
			"udp":  true,
			"ip":   "127.0.0.1",
		},
	}

	// 添加嗅探配置
	sniffing := m.GenerateSniffingConfig(dnsCfg)
	if sniffing != nil {
		inbound["sniffing"] = sniffing
	}

	config.Inbounds = []map[string]interface{}{inbound}

	// 出站配置
	config.Outbounds = []map[string]interface{}{
		{
			"tag":      "proxy_out",
			"protocol": "socks",
			"settings": map[string]interface{}{
				"servers": []map[string]interface{}{
					{
						"address": "127.0.0.1",
						"port":    xlinkPort,
					},
				},
			},
		},
		{
			"tag":      "direct",
			"protocol": "freedom",
			"settings": map[string]interface{}{
				"domainStrategy": "UseIP", // 使用解析后的IP
			},
		},
		{
			"tag":      "block",
			"protocol": "blackhole",
			"settings": map[string]interface{}{},
		},
		{
			"tag":      "dns-out",
			"protocol": "dns",
			"settings": map[string]interface{}{},
		},
	}

	// 路由配置
	config.Routing = m.generateRoutingConfig(node, dnsCfg, hasGeosite, hasGeoip)

	return config, nil
}

// generateRoutingConfig 生成路由配置
func (m *Manager) generateRoutingConfig(
	node *models.NodeConfig,
	dnsCfg *DNSConfig,
	hasGeosite, hasGeoip bool,
) map[string]interface{} {

	// 域名策略
	domainStrategy := "AsIs"
	if dnsCfg.EnableFakeIP {
		domainStrategy = "IPIfNonMatch"
	}

	routing := map[string]interface{}{
		"domainStrategy": domainStrategy,
		"domainMatcher":  "hybrid",
		"rules":          []map[string]interface{}{},
	}

	rules := []map[string]interface{}{}

	// DNS请求路由到dns-out
	rules = append(rules, map[string]interface{}{
		"type":        "field",
		"inboundTag":  []string{"socks-in"},
		"port":        53,
		"outboundTag": "dns-out",
	})

	// 用户自定义规则
	for _, r := range node.Rules {
		rule := m.convertUserRule(r)
		if rule != nil {
			rules = append(rules, rule)
		}
	}

	// 广告拦截
	if dnsCfg.BlockAds && hasGeosite {
		rules = append(rules, map[string]interface{}{
			"type":        "field",
			"outboundTag": "block",
			"domain":      []string{"geosite:category-ads-all"},
		})
	}

	// 拦截BT流量
	rules = append(rules, map[string]interface{}{
		"type":        "field",
		"outboundTag": "block",
		"protocol":    []string{"bittorrent"},
	})

	// 私有IP直连
	if hasGeoip {
		rules = append(rules, map[string]interface{}{
			"type":        "field",
			"outboundTag": "direct",
			"ip":          []string{"geoip:private"},
		})
	}

	// 中国IP直连
	if hasGeoip {
		rules = append(rules, map[string]interface{}{
			"type":        "field",
			"outboundTag": "direct",
			"ip":          []string{"geoip:cn"},
		})
	}

	// 中国域名直连
	if hasGeosite {
		rules = append(rules, map[string]interface{}{
			"type":        "field",
			"outboundTag": "direct",
			"domain":      []string{"geosite:cn", "geosite:geolocation-cn"},
		})
	}

	// 默认走代理
	rules = append(rules, map[string]interface{}{
		"type":        "field",
		"outboundTag": "proxy_out",
		"port":        "0-65535",
	})

	routing["rules"] = rules

	return routing
}

// convertUserRule 转换用户规则
func (m *Manager) convertUserRule(r models.RoutingRule) map[string]interface{} {
	rule := map[string]interface{}{
		"type": "field",
	}

	// 确定出站标签
	target := strings.ToLower(r.Target)
	switch {
	case strings.Contains(target, "direct"):
		rule["outboundTag"] = "direct"
	case strings.Contains(target, "block"):
		rule["outboundTag"] = "block"
	default:
		rule["outboundTag"] = "proxy_out"
	}

	// 根据类型设置匹配条件
	switch r.Type {
	case "domain:":
		rule["domain"] = []string{"domain:" + r.Match}
	case "regexp:":
		rule["domain"] = []string{"regexp:" + r.Match}
	case "geosite:":
		rule["domain"] = []string{"geosite:" + r.Match}
	case "geoip:":
		rule["ip"] = []string{"geoip:" + r.Match}
	default:
		rule["domain"] = []string{"keyword:" + r.Match}
	}

	return rule
}

// =============================================================================
// Fake-IP 管理
// =============================================================================

// AllocateFakeIP 为域名分配Fake-IP
func (m *Manager) AllocateFakeIP(domain string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已分配
	if ip, exists := m.fakeIPMap[domain]; exists {
		return ip
	}

	// 分配新IP
	ip := uint32ToIP(m.nextFakeIP)
	ipStr := ip.String()

	m.fakeIPMap[domain] = ipStr
	m.reverseFakeIP[ipStr] = domain

	m.nextFakeIP++

	// 检查是否超出范围
	if m.nextFakeIP >= ipToUint32(net.ParseIP("198.20.0.0")) {
		// 重置（简单处理，实际应该回收）
		m.nextFakeIP = ipToUint32(net.ParseIP(FakeIPPoolStart))
	}

	return ipStr
}

// LookupFakeIP 通过Fake-IP查询域名
func (m *Manager) LookupFakeIP(ip string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	domain, exists := m.reverseFakeIP[ip]
	return domain, exists
}

// IsFakeIP 检查是否是Fake-IP
func (m *Manager) IsFakeIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}

	_, fakeNet, _ := net.ParseCIDR(FakeIPPoolCIDR)
	return fakeNet.Contains(parsed)
}

// ClearFakeIPCache 清空Fake-IP缓存
func (m *Manager) ClearFakeIPCache() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fakeIPMap = make(map[string]string)
	m.reverseFakeIP = make(map[string]string)
	m.nextFakeIP = ipToUint32(net.ParseIP(FakeIPPoolStart))
}

// =============================================================================
// 系统DNS操作（Windows）
// =============================================================================

// GetSystemDNS 获取系统DNS设置
func (m *Manager) GetSystemDNS() ([]string, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("仅支持Windows")
	}

	// 使用netsh获取DNS
	cmd := exec.Command("netsh", "interface", "ip", "show", "dns")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// 解析输出（简化处理）
	var dns []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 查找IP地址模式
		if ip := net.ParseIP(line); ip != nil {
			dns = append(dns, ip.String())
		}
	}

	return dns, nil
}

// SetSystemDNS 设置系统DNS（需要管理员权限）
func (m *Manager) SetSystemDNS(interfaceName string, dns []string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("仅支持Windows")
	}

	if len(dns) == 0 {
		return fmt.Errorf("DNS列表为空")
	}

	// 设置主DNS
	cmd := exec.Command("netsh", "interface", "ip", "set", "dns",
		fmt.Sprintf("name=%s", interfaceName),
		"source=static",
		fmt.Sprintf("addr=%s", dns[0]),
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("设置主DNS失败: %v", err)
	}

	// 添加备用DNS
	for i := 1; i < len(dns); i++ {
		cmd = exec.Command("netsh", "interface", "ip", "add", "dns",
			fmt.Sprintf("name=%s", interfaceName),
			fmt.Sprintf("addr=%s", dns[i]),
			"index=2",
		)
		cmd.Run() // 忽略错误
	}

	return nil
}

// ResetSystemDNS 重置系统DNS为自动获取
func (m *Manager) ResetSystemDNS(interfaceName string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("仅支持Windows")
	}

	cmd := exec.Command("netsh", "interface", "ip", "set", "dns",
		fmt.Sprintf("name=%s", interfaceName),
		"source=dhcp",
	)

	return cmd.Run()
}

// =============================================================================
// 配置文件写入
// =============================================================================

// WriteXrayConfig 写入Xray配置文件
func (m *Manager) WriteXrayConfig(config *XrayFullConfig, path string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// =============================================================================
// 工具函数
// =============================================================================

// parseListenAddr 解析监听地址
func (m *Manager) parseListenAddr(addr string) (host string, port int) {
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
	fmt.Sscanf(addr[idx+1:], "%d", &port)

	return
}

// ipToUint32 IP转uint32
func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// uint32ToIP uint32转IP
func uint32ToIP(n uint32) net.IP {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}

// FileExists 检查文件是否存在
func (m *Manager) FileExists(filename string) bool {
	path := filepath.Join(m.exeDir, filename)
	_, err := os.Stat(path)
	return err == nil
}
