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
	"strconv"
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
	DNSGoogleIPv6    = "2001:4860:4860::8888"
	DNSGoogleDoH     = "https://dns.google/dns-query"
	DNSAliDNS        = "223.5.5.5"
	DNSTencent       = "119.29.29.29"

	DefaultTUNName = "XlinkTUN"
	DefaultTUNMTU  = 9000
)

// =============================================================================
// DNS管理器
// =============================================================================

type Manager struct {
	mu            sync.RWMutex
	exeDir        string
	mode          int
	tunName       string
	isActive      bool
	fakeIPMap     map[string]string
	reverseFakeIP map[string]string
	nextFakeIP    uint32
	originalDNS   []string
	logCallback   func(level, message string)
}

func NewManager(exeDir string) *Manager {
	return &Manager{
		exeDir:        exeDir,
		tunName:       DefaultTUNName,
		fakeIPMap:     make(map[string]string),
		reverseFakeIP: make(map[string]string),
		nextFakeIP:    ipToUint32(net.ParseIP(FakeIPPoolStart)),
	}
}

func (m *Manager) SetLogCallback(cb func(level, message string)) {
	m.logCallback = cb
}

func (m *Manager) log(level, message string) {
	if m.logCallback != nil {
		m.logCallback(level, message)
	}
}

// =============================================================================
// DNS模式配置
// =============================================================================

type DNSConfig struct {
	Mode           int
	CustomDNS      string
	EnableFakeIP   bool
	EnableSniffing bool
	EnableTUN      bool
	BlockAds       bool
}

// =============================================================================
// Xray DNS配置生成 (IPv6 增强)
// =============================================================================

type XrayDNSConfig struct {
	Hosts         map[string]interface{} `json:"hosts,omitempty"`
	Servers       []interface{}          `json:"servers"`
	QueryStrategy string                 `json:"queryStrategy,omitempty"`
	Tag           string                 `json:"tag,omitempty"`
}

type XrayDNSServer struct {
	Address   string   `json:"address"`
	Port      int      `json:"port,omitempty"`
	Domains   []string `json:"domains,omitempty"`
	ExpectIPs []string `json:"expectIPs,omitempty"`
}

func (m *Manager) GenerateXrayDNSConfig(cfg *DNSConfig, hasGeosite, hasGeoip bool) *XrayDNSConfig {
	dnsConfig := &XrayDNSConfig{
		Hosts: map[string]interface{}{
			"localhost": "127.0.0.1",
		},
		QueryStrategy: "UseIPv4,UseIPv6",
		Tag:           "dns-internal",
	}

	switch cfg.Mode {
	case models.DNSModeFakeIP:
		dnsConfig.Servers = m.buildFakeIPDNSServers(hasGeosite, cfg.CustomDNS)
	case models.DNSModeTUN:
		dnsConfig.Servers = m.buildRemoteDNSServers(hasGeosite, cfg.CustomDNS)
	default:
		dnsConfig.Servers = m.buildSplitDNSServers(hasGeosite, hasGeoip, cfg.CustomDNS)
	}

	if cfg.BlockAds && hasGeosite {
		dnsConfig.Hosts["geosite:category-ads-all"] = "127.0.0.1"
	}

	return dnsConfig
}

func (m *Manager) buildFakeIPDNSServers(hasGeosite bool, customDNS string) []interface{} {
	servers := []interface{}{
		"fakedns",
		m.buildRemoteDNSServers(hasGeosite, customDNS)[0],
		XrayDNSServer{Address: DNSGoogleIPv6},
	}
	return servers
}

func (m *Manager) buildRemoteDNSServers(hasGeosite bool, customDNS string) []interface{} {
	remoteAddress := DNSCloudflareDoH
	if customDNS != "" {
		remoteAddress = customDNS
	}
	primaryServer := XrayDNSServer{Address: remoteAddress}
	if hasGeosite {
		primaryServer.Domains = []string{"geosite:geolocation-!cn"}
	}
	return []interface{}{primaryServer, XrayDNSServer{Address: DNSGoogleIPv6}}
}

func (m *Manager) buildSplitDNSServers(hasGeosite, hasGeoip bool, customDNS string) []interface{} {
	servers := []interface{}{}
	if hasGeosite && hasGeoip {
		servers = append(servers, XrayDNSServer{
			Address:   DNSAliDNS,
			Port:      53,
			Domains:   []string{"geosite:cn", "geosite:geolocation-cn"},
			ExpectIPs: []string{"geoip:cn"},
		})
	}
	remoteAddress := DNSCloudflareDoH
	if customDNS != "" {
		remoteAddress = customDNS
	}
	servers = append(servers, XrayDNSServer{Address: remoteAddress})
	servers = append(servers, XrayDNSServer{Address: DNSGoogleIPv6})
	return servers
}

// =============================================================================
// 其他配置生成
// =============================================================================

type XrayFakeDNSConfig struct {
	IPPool   string `json:"ipPool"`
	PoolSize int    `json:"poolSize"`
}

func (m *Manager) GenerateFakeDNSConfig() *XrayFakeDNSConfig {
	return &XrayFakeDNSConfig{IPPool: FakeIPPoolCIDR, PoolSize: FakeIPPoolSize}
}

type XraySniffingConfig struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride"`
}

func (m *Manager) GenerateSniffingConfig(cfg *DNSConfig) *XraySniffingConfig {
	if !cfg.EnableSniffing {
		return nil
	}
	return &XraySniffingConfig{Enabled: true, DestOverride: []string{"http", "tls", "quic", "fakedns"}}
}

// =============================================================================
// 完整Xray配置生成
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
		CustomDNS:      node.CustomDNS,
		EnableFakeIP:   node.DNSMode == models.DNSModeFakeIP,
		EnableSniffing: node.EnableSniffing,
		EnableTUN:      node.DNSMode == models.DNSModeTUN,
		BlockAds:       true,
	}
	
	listenHost, listenPort := m.parseListenAddr(node.Listen)
	
	config := &XrayFullConfig{
		Log: map[string]interface{}{"loglevel": "warning"},
		DNS: m.GenerateXrayDNSConfig(dnsCfg, hasGeosite, hasGeoip),
	}

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
		{"tag": "block", "protocol": "blackhole"},
		{"tag": "dns-out", "protocol": "dns"},
	}
	
	config.Routing = m.generateRoutingConfig(node, dnsCfg, hasGeosite, hasGeoip)
	
	return config, nil
}

func (m *Manager) generateRoutingConfig(node *models.NodeConfig, dnsCfg *DNSConfig, hasGeosite, hasGeoip bool) map[string]interface{} {
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

	rules = append(rules, map[string]interface{}{"type": "field", "outboundTag": "dns-out", "port": 53})
	
	for _, r := range node.Rules {
		if rule := m.convertUserRule(r); rule != nil {
			rules = append(rules, rule)
		}
	}

	if dnsCfg.BlockAds && hasGeosite {
		rules = append(rules, map[string]interface{}{"type": "field", "outboundTag": "block", "domain": []string{"geosite:category-ads-all"}})
	}
	
	rules = append(rules, map[string]interface{}{"type": "field", "outboundTag": "block", "protocol": []string{"bittorrent"}})
	
	if hasGeoip {
		rules = append(rules, map[string]interface{}{"type": "field", "outboundTag": "direct", "ip": []string{"geoip:private", "geoip:cn"}})
		rules = append(rules, map[string]interface{}{"type": "field", "outboundTag": "direct", "ip": []string{"geoip:private6", "geoip:cn6"}}) // IPv6 直连
	}
	
	if hasGeosite {
		rules = append(rules, map[string]interface{}{"type": "field", "outboundTag": "direct", "domain": []string{"geosite:cn"}})
	}
	
	// 默认代理
	rules = append(rules, map[string]interface{}{
		"type": "field",
		"outboundTag": "proxy_out",
		"network": []string{"tcp", "udp"},
	})
	
	routing["rules"] = rules
	return routing
}

// =============================================================================
// 辅助函数
// =============================================================================

func (m *Manager) convertUserRule(r models.RoutingRule) map[string]interface{} {
	rule := map[string]interface{}{"type": "field"}
	target := strings.ToLower(r.Target)
	switch {
	case strings.Contains(target, "direct"): rule["outboundTag"] = "direct"
	case strings.Contains(target, "block"): rule["outboundTag"] = "block"
	default: rule["outboundTag"] = "proxy_out"
	}
	switch r.Type {
	case "domain:": rule["domain"] = []string{"domain:" + r.Match}
	case "regexp:": rule["domain"] = []string{"regexp:" + r.Match}
	case "geosite:": rule["domain"] = []string{"geosite:" + r.Match}
	case "geoip:": rule["ip"] = []string{"geoip:" + r.Match}
	default: rule["domain"] = []string{"keyword:" + r.Match}
	}
	return rule
}

func (m *Manager) WriteXrayConfig(config *XrayFullConfig, path string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil { return err }
	return os.WriteFile(path, data, 0644)
}

func (m *Manager) parseListenAddr(addr string) (host string, port int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "127.0.0.1", 10808
	}
	p, _ := strconv.Atoi(portStr)
	return host, p
}

func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil { return 0 }
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func uint32ToIP(n uint32) net.IP {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}

func (m *Manager) FileExists(filename string) bool {
	_, err := os.Stat(filepath.Join(m.exeDir, filename))
	return err == nil
}

// =============================================================================
// Fake-IP 管理
// =============================================================================

// ClearFakeIPCache 清空Fake-IP缓存
func (m *Manager) ClearFakeIPCache() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fakeIPMap = make(map[string]string)
	m.reverseFakeIP = make(map[string]string)
	m.nextFakeIP = ipToUint32(net.ParseIP(FakeIPPoolStart))
}

// =============================================================================
// 系统DNS操作
// =============================================================================

// GetSystemDNS 获取系统DNS设置
func (m *Manager) GetSystemDNS() ([]string, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("仅支持Windows")
	}

	cmd := exec.Command("netsh", "interface", "ip", "show", "dns")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var dns []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
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

	cmd := exec.Command("netsh", "interface", "ip", "set", "dns",
		fmt.Sprintf("name=%s", interfaceName), "source=static", fmt.Sprintf("addr=%s", dns[0]))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("设置主DNS失败: %v", err)
	}

	for i := 1; i < len(dns); i++ {
		cmd = exec.Command("netsh", "interface", "ip", "add", "dns",
			fmt.Sprintf("name=%s", interfaceName), fmt.Sprintf("addr=%s", dns[i]), "index=2")
		cmd.Run()
	}
	return nil
}

// ResetSystemDNS 重置系统DNS为自动获取
func (m *Manager) ResetSystemDNS(interfaceName string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("仅支持Windows")
	}
	cmd := exec.Command("netsh", "interface", "ip", "set", "dns",
		fmt.Sprintf("name=%s", interfaceName), "source=dhcp")
	return cmd.Run()
}
