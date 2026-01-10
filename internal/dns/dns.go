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
// 常量定义 (新增 IPv6 DNS)
// =============================================================================

const (
	FakeIPPoolStart  = "198.18.0.0"
	FakeIPPoolCIDR   = "198.18.0.0/15"
	FakeIPPoolSize   = 65535

	DNSCloudflare    = "1.1.1.1"
	DNSCloudflareDoH = "https://1.1.1.1/dns-query"
	DNSGoogle        = "8.8.8.8"
	DNSGoogleIPv6    = "2001:4860:4860::8888" // <--- 新增
	DNSGoogleDoH     = "https://dns.google/dns-query"
	DNSAliDNS        = "223.5.5.5"
	DNSTencent       = "119.29.29.29"

	DefaultTUNName = "XlinkTUN"
	DefaultTUNMTU  = 9000
)

// ... (Manager 结构体和 NewManager, SetLogCallback, log 方法保持不变) ...
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
// DNSConfig 和 DefaultDNSConfig 保持不变
// DNS模式配置
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
// Xray DNS配置生成 (IPv6 增强)
// =============================================================================

type XrayDNSConfig struct {
	Hosts           map[string]interface{} `json:"hosts,omitempty"`
	Servers         []interface{}          `json:"servers"`
	ClientIP        string                 `json:"clientIp,omitempty"`
	QueryStrategy   string                 `json:"queryStrategy,omitempty"` // Xray v5+ 使用 []string
	DisableCache    bool                   `json:"disableCache,omitempty"`
	DisableFallback bool                   `json:"disableFallback,omitempty"`
	Tag             string                 `json:"tag,omitempty"`
}

type XrayDNSServer struct {
	Address      string   `json:"address"`
	Port         int      `json:"port,omitempty"`
	Domains      []string `json:"domains,omitempty"`
	ExpectIPs    []string `json:"expectIPs,omitempty"`
	SkipFallback bool     `json:"skipFallback,omitempty"`
	ClientIP     string   `json:"clientIp,omitempty"`
}

// GenerateXrayDNSConfig 生成Xray DNS配置 (IPv6 增强)
func (m *Manager) GenerateXrayDNSConfig(cfg *DNSConfig, hasGeosite, hasGeoip bool) *XrayDNSConfig {
	dnsConfig := &XrayDNSConfig{
		Hosts: map[string]interface{}{
			"localhost": "127.0.0.1",
			"localhost.localdomain": "127.0.0.1",
		},
		// ⚠️【核心修复】强制同时查询 IPv4 和 IPv6
		QueryStrategy:   "UseIPv4,UseIPv6", 
		DisableCache:    false,
		DisableFallback: false,
		Tag:             "dns-internal",
	}

	switch cfg.Mode {
	case models.DNSModeFakeIP:
		dnsConfig.Servers = m.buildFakeIPDNSServers(hasGeosite)
		dnsConfig.DisableFallback = true
	case models.DNSModeTUN:
		dnsConfig.Servers = m.buildRemoteDNSServers(hasGeosite)
		dnsConfig.DisableFallback = true
	default:
		dnsConfig.Servers = m.buildSplitDNSServers(hasGeosite, hasGeoip)
	}

	if cfg.BlockAds && hasGeosite {
		dnsConfig.Hosts["geosite:category-ads-all"] = "127.0.0.1"
	}

	return dnsConfig
}

// buildFakeIPDNSServers (IPv6 增强)
func (m *Manager) buildFakeIPDNSServers(hasGeosite bool) []interface{} {
	servers := []interface{}{
		"fakedns", // FakeDNS 优先
		m.buildRemoteDNSServers(hasGeosite)[0], // 复用远程 DNS 配置
		// ⚠️【核心修复】添加一个 IPv6 DNS 作为备用
		XrayDNSServer{ Address: DNSGoogleIPv6 },
	}
	return servers
}

// buildRemoteDNSServers (IPv6 增强)
func (m *Manager) buildRemoteDNSServers(hasGeosite bool) []interface{} {
	// 主 DNS: Cloudflare (DoH)
	primaryServer := XrayDNSServer{ Address: DNSCloudflareDoH }
	if hasGeosite {
		primaryServer.Domains = []string{"geosite:geolocation-!cn"}
	}
	
	// 备用 DNS: Google (IPv6)
	backupServer := XrayDNSServer{ Address: DNSGoogleIPv6 }
	
	return []interface{}{primaryServer, backupServer}
}

// buildSplitDNSServers (IPv6 增强)
func (m *Manager) buildSplitDNSServers(hasGeosite, hasGeoip bool) []interface{} {
	servers := []interface{}{}

	if hasGeosite && hasGeoip {
		// 国内 DNS (IPv4)
		servers = append(servers, XrayDNSServer{
			Address:      DNSAliDNS,
			Port:         53,
			Domains:      []string{"geosite:cn", "geosite:geolocation-cn", "geosite:tld-cn"},
			ExpectIPs:    []string{"geoip:cn"},
			SkipFallback: true,
		})
	}
	
	// 国外 DNS (DoH)
	servers = append(servers, XrayDNSServer{ Address: DNSCloudflareDoH })
	
	// 备用 DNS (IPv6)
	servers = append(servers, XrayDNSServer{ Address: DNSGoogleIPv6 })

	return servers
}

// ... (FakeDNS, Sniffing, TUN 配置生成保持不变) ...

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
// 完整Xray配置生成 (IPv6 增强)
// =============================================================================

type XrayFullConfig struct {
	Log       map[string]interface{}   `json:"log"`
	DNS       *XrayDNSConfig           `json:"dns,omitempty"`
	FakeDNS   interface{}              `json:"fakedns,omitempty"`
	Inbounds  []map[string]interface{} `json:"inbounds"`
	Outbounds []map[string]interface{} `json:"outbounds"`
	Routing   map[string]interface{}   `json:"routing"`
}

func (m *Manager) GenerateFullXrayConfig(node *models.NodeConfig, xlinkPort int, hasGeosite, hasGeoip bool) (*XrayFullConfig, error) {
	dnsCfg := &DNSConfig{
		Mode:           node.DNSMode,
		EnableFakeIP:   node.DNSMode == models.DNSModeFakeIP,
		EnableSniffing: node.EnableSniffing,
		EnableTUN:      node.DNSMode == models.DNSModeTUN,
		HijackDNS:      true,
		BlockAds:       true,
	}
	
	listenHost, listenPort := m.parseListenAddr(node.Listen)
	
	config := &XrayFullConfig{
		Log: map[string]interface{}{"loglevel": "warning"},
	}

	config.DNS = m.GenerateXrayDNSConfig(dnsCfg, hasGeosite, hasGeoip)

	if dnsCfg.EnableFakeIP {
		config.FakeDNS = []interface{}{m.GenerateFakeDNSConfig()}
	}
	
	inbound := map[string]interface{}{
		"tag":      "socks-in",
		"listen":   listenHost,
		"port":     listenPort,
		"protocol": "socks",
		"settings": map[string]interface{}{"auth": "noauth", "udp": true, "ip": "127.0.0.1"},
	}
	
	if sniffing := m.GenerateSniffingConfig(dnsCfg); sniffing != nil {
		inbound["sniffing"] = sniffing
	}

	config.Inbounds = []map[string]interface{}{inbound}

	config.Outbounds = []map[string]interface{}{
		{"tag": "proxy_out", "protocol": "socks", "settings": map[string]interface{}{"servers": []map[string]interface{}{{"address": "127.0.0.1", "port": xlinkPort}}}},
		{"tag": "direct", "protocol": "freedom", "settings": map[string]interface{}{"domainStrategy": "UseIP"}},
		{"tag": "block", "protocol": "blackhole", "settings": map[string]interface{}{}},
		{"tag": "dns-out", "protocol": "dns", "settings": map[string]interface{}{}},
	}
	
	config.Routing = m.generateRoutingConfig(node, dnsCfg, hasGeosite, hasGeoip)
	
	return config, nil
}

// generateRoutingConfig (IPv6 增强)
func (m *Manager) generateRoutingConfig(node *models.NodeConfig, dnsCfg *DNSConfig, hasGeosite, hasGeoip bool) map[string]interface{} {
	domainStrategy := "AsIs"
	if dnsCfg.EnableFakeIP {
		domainStrategy = "IPIfNonMatch"
	}

	routing := map[string]interface{}{
		"domainStrategy": domainStrategy,
		"domainMatcher":  "hybrid",
		"rules":          []map[string]interface{}{},
		"strategy":       "rules", // 明确路由策略
	}
	
	rules := []map[string]interface{}{}

	rules = append(rules, map[string]interface{}{
		"type": "field", "inboundTag": []string{"socks-in"}, "port": 53, "outboundTag": "dns-out",
	})
	
	// 用户规则
	for _, r := range node.Rules {
		if rule := m.convertUserRule(r); rule != nil {
			rules = append(rules, rule)
		}
	}

	// 广告拦截
	if dnsCfg.BlockAds && hasGeosite {
		rules = append(rules, map[string]interface{}{
			"type": "field", "outboundTag": "block", "domain": []string{"geosite:category-ads-all"},
		})
	}
	
	// BT 拦截
	rules = append(rules, map[string]interface{}{
		"type": "field", "outboundTag": "block", "protocol": []string{"bittorrent"},
	})
	
	// ⚠️【核心修复】添加 IPv6 直连规则
	if hasGeoip {
		rules = append(rules, map[string]interface{}{
			"type": "field", "outboundTag": "direct", "ip": []string{"geoip:private", "geoip:private6"},
		})
		rules = append(rules, map[string]interface{}{
			"type": "field", "outboundTag": "direct", "ip": []string{"geoip:cn", "geoip:cn6"},
		})
	}
	
	if hasGeosite {
		rules = append(rules, map[string]interface{}{
			"type": "field", "outboundTag": "direct", "domain": []string{"geosite:cn", "geosite:geolocation-cn"},
		})
	}
	
	// 默认代理
	rules = append(rules, map[string]interface{}{
		"type": "field", "outboundTag": "proxy_out", "network": "tcp,udp",
	})
	
	routing["rules"] = rules
	return routing
}

// ... (convertUserRule, Fake-IP管理, 系统DNS, 工具函数等保持不变) ...

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

func (m *Manager) LookupFakeIP(ip string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	domain, exists := m.reverseFakeIP[ip]
	return domain, exists
}

func (m *Manager) IsFakeIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}

	_, fakeNet, _ := net.ParseCIDR(FakeIPPoolCIDR)
	return fakeNet.Contains(parsed)
}

func (m *Manager) ClearFakeIPCache() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fakeIPMap = make(map[string]string)
	m.reverseFakeIP = make(map[string]string)
	m.nextFakeIP = ipToUint32(net.ParseIP(FakeIPPoolStart))
}

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

func (m *Manager) WriteXrayConfig(config *XrayFullConfig, path string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

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

func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func uint32ToIP(n uint32) net.IP {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}

func (m *Manager) FileExists(filename string) bool {
	path := filepath.Join(m.exeDir, filename)
	_, err := os.Stat(path)
	return err == nil
}
