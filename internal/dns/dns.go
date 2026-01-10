// internal/dns/dns.go
// Package dns 提供DNS防泄露功能（支持IPv4/IPv6双栈）
package dns

import (
	"encoding/json"
	"fmt"
	"math/big"
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
	// IPv4 Fake-IP 地址池
	FakeIPPoolStart = "198.18.0.0"
	FakeIPPoolCIDR  = "198.18.0.0/15" // 198.18.0.0 - 198.19.255.255
	FakeIPPoolSize  = 65535

	// IPv6 Fake-IP 地址池 (使用 fc00::/18 私有地址段)
	FakeIPv6PoolStart = "fc00::"
	FakeIPv6PoolCIDR  = "fc00::/18"
	FakeIPv6PoolSize  = 65535

	// IPv4 DNS服务器
	DNSCloudflare    = "1.1.1.1"
	DNSCloudflareAlt = "1.0.0.1"
	DNSCloudflareDoH = "https://1.1.1.1/dns-query"
	DNSGoogle        = "8.8.8.8"
	DNSGoogleAlt     = "8.8.4.4"
	DNSGoogleDoH     = "https://dns.google/dns-query"
	DNSAliDNS        = "223.5.5.5"
	DNSAliDNSAlt     = "223.6.6.6"
	DNSTencent       = "119.29.29.29"
	DNSTencentAlt    = "182.254.116.116"

	// IPv6 DNS服务器
	DNSCloudflareIPv6    = "2606:4700:4700::1111"
	DNSCloudflareIPv6Alt = "2606:4700:4700::1001"
	DNSGoogleIPv6        = "2001:4860:4860::8888"
	DNSGoogleIPv6Alt     = "2001:4860:4860::8844"
	DNSAliDNSIPv6        = "2400:3200::1"
	DNSAliDNSIPv6Alt     = "2400:3200:baba::1"
	DNSTencentIPv6       = "2402:4e00::"
	DNSTencentIPv6Alt    = "2402:4e00:1::"

	// DoH/DoT 服务器（支持IPv6）
	DNSCloudflareDoT = "tls://1.1.1.1"
	DNSGoogleDoT     = "tls://dns.google"
	DNSAliDoH        = "https://dns.alidns.com/dns-query"
	DNSTencentDoH    = "https://doh.pub/dns-query"

	// TUN配置
	DefaultTUNName = "XlinkTUN"
	DefaultTUNMTU  = 9000

	// TUN IPv4/IPv6 地址
	DefaultTUNIPv4 = "198.18.0.1/16"
	DefaultTUNIPv6 = "fdfe:dcba:9876::1/126"
)

// =============================================================================
// IP版本枚举
// =============================================================================

type IPVersion int

const (
	IPVersionAuto IPVersion = iota // 自动检测
	IPVersionIPv4                  // 仅IPv4
	IPVersionIPv6                  // 仅IPv6
	IPVersionDual                  // 双栈
)

// =============================================================================
// DNS管理器
// =============================================================================

// Manager DNS防泄露管理器
type Manager struct {
	mu sync.RWMutex

	exeDir    string
	mode      int // DNS模式
	tunName   string
	isActive  bool
	ipVersion IPVersion

	// IPv4 Fake-IP 映射表
	fakeIPMap     map[string]string // domain -> fake IPv4
	reverseFakeIP map[string]string // fake IPv4 -> domain
	nextFakeIP    uint32

	// IPv6 Fake-IP 映射表
	fakeIPv6Map     map[string]string // domain -> fake IPv6
	reverseFakeIPv6 map[string]string // fake IPv6 -> domain
	nextFakeIPv6    *big.Int

	// 原始系统DNS（用于恢复）
	originalDNSv4 []string
	originalDNSv6 []string

	// 日志回调
	logCallback func(level, message string)
}

// NewManager 创建DNS管理器
func NewManager(exeDir string) *Manager {
	return &Manager{
		exeDir:          exeDir,
		tunName:         DefaultTUNName,
		ipVersion:       IPVersionDual,
		fakeIPMap:       make(map[string]string),
		reverseFakeIP:   make(map[string]string),
		fakeIPv6Map:     make(map[string]string),
		reverseFakeIPv6: make(map[string]string),
		nextFakeIP:      ipv4ToUint32(net.ParseIP(FakeIPPoolStart)),
		nextFakeIPv6:    ipv6ToBigInt(net.ParseIP(FakeIPv6PoolStart)),
	}
}

// SetLogCallback 设置日志回调
func (m *Manager) SetLogCallback(cb func(level, message string)) {
	m.logCallback = cb
}

// SetIPVersion 设置IP版本偏好
func (m *Manager) SetIPVersion(version IPVersion) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ipVersion = version
}

// GetIPVersion 获取当前IP版本设置
func (m *Manager) GetIPVersion() IPVersion {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ipVersion
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
	Mode            int       `json:"mode"`
	IPVersion       IPVersion `json:"ip_version"`
	CustomUpstream  []string  `json:"custom_upstream,omitempty"`
	EnableFakeIP    bool      `json:"enable_fake_ip"`
	FakeIPFilter    []string  `json:"fake_ip_filter,omitempty"` // 不使用Fake-IP的域名
	EnableSniffing  bool      `json:"enable_sniffing"`
	EnableTUN       bool      `json:"enable_tun"`
	TUNName         string    `json:"tun_name,omitempty"`
	TUNMTU          int       `json:"tun_mtu,omitempty"`
	HijackDNS       bool      `json:"hijack_dns"`
	BlockAds        bool      `json:"block_ads"`
	PreferIPv6      bool      `json:"prefer_ipv6"`       // 优先使用IPv6
	EnableIPv6      bool      `json:"enable_ipv6"`       // 启用IPv6支持
	IPv6Only        bool      `json:"ipv6_only"`         // 仅使用IPv6
	DisableIPv6     bool      `json:"disable_ipv6"`      // 禁用IPv6
}

// DefaultDNSConfig 默认DNS配置
func DefaultDNSConfig() *DNSConfig {
	return &DNSConfig{
		Mode:           models.DNSModeFakeIP,
		IPVersion:      IPVersionDual,
		EnableFakeIP:   true,
		EnableSniffing: true,
		EnableTUN:      false,
		TUNName:        DefaultTUNName,
		TUNMTU:         DefaultTUNMTU,
		HijackDNS:      true,
		BlockAds:       true,
		EnableIPv6:     true,
		PreferIPv6:     false,
		FakeIPFilter: []string{
			// 本地域名不使用Fake-IP
			"+.lan",
			"+.local",
			"+.localhost",
			"localhost",
			"*.localdomain",
			"*.home.arpa",
			// Windows网络检测
			"dns.msftncsi.com",
			"www.msftncsi.com",
			"www.msftconnecttest.com",
			"ipv6.msftconnecttest.com",
			// 时间同步
			"time.windows.com",
			"time.nist.gov",
			"pool.ntp.org",
			"ntp.ubuntu.com",
			// IPv6测试
			"ipv6.google.com",
			"ipv6.test-ipv6.com",
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
	Address       string   `json:"address"`
	Port          int      `json:"port,omitempty"`
	Domains       []string `json:"domains,omitempty"`
	ExpectIPs     []string `json:"expectIPs,omitempty"`
	SkipFallback  bool     `json:"skipFallback,omitempty"`
	ClientIP      string   `json:"clientIp,omitempty"`
	QueryStrategy string   `json:"queryStrategy,omitempty"` // UseIP, UseIPv4, UseIPv6
}

// GenerateXrayDNSConfig 生成Xray DNS配置
func (m *Manager) GenerateXrayDNSConfig(cfg *DNSConfig, hasGeosite, hasGeoip bool) *XrayDNSConfig {
	dnsConfig := &XrayDNSConfig{
		Hosts: map[string]interface{}{
			"localhost": m.generateLocalhostHosts(cfg),
		},
		QueryStrategy:   m.getQueryStrategy(cfg),
		DisableCache:    false,
		DisableFallback: false,
		Tag:             "dns-internal",
	}

	switch cfg.Mode {
	case models.DNSModeFakeIP:
		// Fake-IP 模式：使用FakeDNS + 远程DNS
		dnsConfig.Servers = m.buildFakeIPDNSServers(cfg, hasGeosite)
		dnsConfig.DisableFallback = true

	case models.DNSModeTUN:
		// TUN模式：完全远程解析
		dnsConfig.Servers = m.buildRemoteDNSServers(cfg, hasGeosite)
		dnsConfig.DisableFallback = true

	default:
		// 标准模式：分流DNS
		dnsConfig.Servers = m.buildSplitDNSServers(cfg, hasGeosite, hasGeoip)
	}

	// 添加广告拦截hosts
	if cfg.BlockAds && hasGeosite {
		dnsConfig.Hosts["geosite:category-ads-all"] = m.getBlockAddress(cfg)
	}

	return dnsConfig
}

// generateLocalhostHosts 生成localhost的hosts配置
func (m *Manager) generateLocalhostHosts(cfg *DNSConfig) interface{} {
	if cfg.EnableIPv6 && !cfg.DisableIPv6 {
		// 双栈模式返回数组
		return []string{"127.0.0.1", "::1"}
	}
	if cfg.IPv6Only {
		return "::1"
	}
	return "127.0.0.1"
}

// getBlockAddress 获取拦截地址
func (m *Manager) getBlockAddress(cfg *DNSConfig) interface{} {
	if cfg.EnableIPv6 && !cfg.DisableIPv6 {
		return []string{"127.0.0.1", "::1"}
	}
	if cfg.IPv6Only {
		return "::1"
	}
	return "127.0.0.1"
}

// getQueryStrategy 获取DNS查询策略
func (m *Manager) getQueryStrategy(cfg *DNSConfig) string {
	if cfg.DisableIPv6 || cfg.IPVersion == IPVersionIPv4 {
		return "UseIPv4"
	}
	if cfg.IPv6Only || cfg.IPVersion == IPVersionIPv6 {
		return "UseIPv6"
	}
	if cfg.PreferIPv6 {
		return "UseIP" // Xray会同时查询，但路由可以优先IPv6
	}
	return "UseIP" // 同时查询A和AAAA记录
}

// buildFakeIPDNSServers 构建Fake-IP模式DNS服务器列表
func (m *Manager) buildFakeIPDNSServers(cfg *DNSConfig, hasGeosite bool) []interface{} {
	servers := []interface{}{}

	// FakeDNS优先 - 根据配置决定使用哪种
	if cfg.IPv6Only {
		servers = append(servers, "fakedns+others")
	} else {
		servers = append(servers, "fakedns")
	}

	// 远程DNS作为后备（通过代理）
	remoteServer := XrayDNSServer{
		Address:       DNSCloudflareDoH,
		SkipFallback:  false,
		QueryStrategy: m.getQueryStrategy(cfg),
	}

	if hasGeosite {
		remoteServer.Domains = []string{
			"geosite:geolocation-!cn",
		}
	}

	servers = append(servers, remoteServer)

	// 如果启用IPv6，添加IPv6 DNS服务器
	if cfg.EnableIPv6 && !cfg.DisableIPv6 {
		servers = append(servers, XrayDNSServer{
			Address:       DNSGoogleDoH,
			QueryStrategy: "UseIPv6",
		})
	}

	return servers
}

// buildRemoteDNSServers 构建远程DNS服务器列表
func (m *Manager) buildRemoteDNSServers(cfg *DNSConfig, hasGeosite bool) []interface{} {
	servers := []interface{}{}

	// 主DNS：Cloudflare DoH
	primaryServer := XrayDNSServer{
		Address:       DNSCloudflareDoH,
		SkipFallback:  false,
		QueryStrategy: m.getQueryStrategy(cfg),
	}

	if hasGeosite {
		primaryServer.Domains = []string{
			"geosite:geolocation-!cn",
		}
	}

	servers = append(servers, primaryServer)

	// 备用DNS：Google DoH
	servers = append(servers, XrayDNSServer{
		Address:       DNSGoogleDoH,
		QueryStrategy: m.getQueryStrategy(cfg),
	})

	// IPv6专用DNS
	if cfg.EnableIPv6 && !cfg.DisableIPv6 && !cfg.IPv6Only {
		// 添加IPv6优先的DNS服务器
		servers = append(servers, XrayDNSServer{
			Address:       fmt.Sprintf("https://[%s]/dns-query", DNSCloudflareIPv6),
			QueryStrategy: "UseIPv6",
		})
	}

	return servers
}

// buildSplitDNSServers 构建分流DNS服务器列表
func (m *Manager) buildSplitDNSServers(cfg *DNSConfig, hasGeosite, hasGeoip bool) []interface{} {
	servers := []interface{}{}

	queryStrategy := m.getQueryStrategy(cfg)

	// 国内域名使用国内DNS
	if hasGeosite && hasGeoip {
		// IPv4国内DNS
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
			SkipFallback:  true,
			QueryStrategy: queryStrategy,
		})

		// IPv6国内DNS
		if cfg.EnableIPv6 && !cfg.DisableIPv6 {
			servers = append(servers, XrayDNSServer{
				Address: DNSAliDNSIPv6,
				Port:    53,
				Domains: []string{
					"geosite:cn",
				},
				SkipFallback:  true,
				QueryStrategy: "UseIPv6",
			})
		}

		// 备用国内DNS
		servers = append(servers, XrayDNSServer{
			Address: DNSTencent,
			Port:    53,
			Domains: []string{
				"geosite:cn",
			},
			SkipFallback:  true,
			QueryStrategy: queryStrategy,
		})
	}

	// 国外域名使用国外DNS（通过代理）
	servers = append(servers, XrayDNSServer{
		Address:       DNSCloudflareDoH,
		QueryStrategy: queryStrategy,
	})

	// 最终后备
	if cfg.EnableIPv6 && cfg.PreferIPv6 {
		servers = append(servers, DNSGoogleIPv6)
	}
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

// GenerateFakeDNSConfig 生成FakeDNS配置（支持双栈）
func (m *Manager) GenerateFakeDNSConfig(cfg *DNSConfig) interface{} {
	configs := []XrayFakeDNSConfig{}

	// IPv4 Fake-IP池
	if !cfg.IPv6Only {
		configs = append(configs, XrayFakeDNSConfig{
			IPPool:   FakeIPPoolCIDR,
			PoolSize: FakeIPPoolSize,
		})
	}

	// IPv6 Fake-IP池
	if cfg.EnableIPv6 && !cfg.DisableIPv6 {
		configs = append(configs, XrayFakeDNSConfig{
			IPPool:   FakeIPv6PoolCIDR,
			PoolSize: FakeIPv6PoolSize,
		})
	}

	if len(configs) == 1 {
		return configs[0]
	}
	return configs
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

	destOverride := []string{
		"http",
		"tls",
		"quic",
	}

	// Fake-IP模式需要嗅探fakedns
	if cfg.EnableFakeIP {
		destOverride = append(destOverride, "fakedns")
		if cfg.EnableIPv6 {
			destOverride = append(destOverride, "fakedns+others")
		}
	}

	return &XraySniffingConfig{
		Enabled:      true,
		DestOverride: destOverride,
		MetadataOnly: false,
		RouteOnly:    false,
		DomainsExcluded: []string{
			"courier.push.apple.com",
			"Mijia Cloud",
			"+.oray.com", // 向日葵等
		},
	}
}

// =============================================================================
// TUN模式配置
// =============================================================================

// TUNConfig TUN网卡配置
type TUNConfig struct {
	Enable                 bool     `json:"enable"`
	Stack                  string   `json:"stack"`
	Device                 string   `json:"device"`
	AutoRoute              bool     `json:"auto-route"`
	AutoDetectInterface    bool     `json:"auto-detect-interface"`
	DNSHijack              []string `json:"dns-hijack"`
	MTU                    int      `json:"mtu"`
	StrictRoute            bool     `json:"strict-route"`
	Inet4Address           []string `json:"inet4-address,omitempty"`
	Inet6Address           []string `json:"inet6-address,omitempty"`
	EndpointIndependentNat bool     `json:"endpoint-independent-nat,omitempty"`
	UDPTimeout             int64    `json:"udp-timeout,omitempty"`
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
		Enable:                 true,
		Stack:                  "gvisor",
		Device:                 tunName,
		AutoRoute:              true,
		AutoDetectInterface:    true,
		MTU:                    mtu,
		StrictRoute:            false,
		EndpointIndependentNat: true,
		UDPTimeout:             300,
	}

	// 配置TUN IP地址
	if !cfg.IPv6Only {
		tunCfg.Inet4Address = []string{DefaultTUNIPv4}
	}
	if cfg.EnableIPv6 && !cfg.DisableIPv6 {
		tunCfg.Inet6Address = []string{DefaultTUNIPv6}
	}

	// DNS劫持
	if cfg.HijackDNS {
		tunCfg.DNSHijack = []string{
			"any:53",
			"tcp://any:53",
		}
		// IPv6 DNS劫持
		if cfg.EnableIPv6 && !cfg.DisableIPv6 {
			tunCfg.DNSHijack = append(tunCfg.DNSHijack,
				"[::]:53",
				"tcp://[::]:53",
			)
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
		EnableIPv6:     node.EnableIPv6,
		PreferIPv6:     node.PreferIPv6,
		DisableIPv6:    node.DisableIPv6,
		IPv6Only:       node.IPv6Only,
	}

	// 设置IP版本
	if node.DisableIPv6 {
		dnsCfg.IPVersion = IPVersionIPv4
	} else if node.IPv6Only {
		dnsCfg.IPVersion = IPVersionIPv6
	} else if node.EnableIPv6 {
		dnsCfg.IPVersion = IPVersionDual
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
		config.FakeDNS = m.GenerateFakeDNSConfig(dnsCfg)
	}

	// 入站配置
	inbound := m.generateInboundConfig(dnsCfg, listenHost, listenPort)

	config.Inbounds = []map[string]interface{}{inbound}

	// 出站配置
	config.Outbounds = m.generateOutboundConfig(dnsCfg, xlinkPort)

	// 路由配置
	config.Routing = m.generateRoutingConfig(node, dnsCfg, hasGeosite, hasGeoip)

	return config, nil
}

// generateInboundConfig 生成入站配置
func (m *Manager) generateInboundConfig(cfg *DNSConfig, listenHost string, listenPort int) map[string]interface{} {
	// 处理监听地址
	listen := listenHost
	if isIPv6Address(listenHost) && !strings.HasPrefix(listenHost, "[") {
		listen = listenHost // Xray内部处理
	}

	inbound := map[string]interface{}{
		"tag":      "socks-in",
		"listen":   listen,
		"port":     listenPort,
		"protocol": "socks",
		"settings": map[string]interface{}{
			"auth": "noauth",
			"udp":  true,
		},
	}

	// 设置本地IP（用于UDP返回）
	if cfg.EnableIPv6 && !cfg.DisableIPv6 {
		inbound["settings"].(map[string]interface{})["ip"] = "::" // 双栈
	} else {
		inbound["settings"].(map[string]interface{})["ip"] = "127.0.0.1"
	}

	// 添加嗅探配置
	sniffing := m.GenerateSniffingConfig(cfg)
	if sniffing != nil {
		inbound["sniffing"] = sniffing
	}

	return inbound
}

// generateOutboundConfig 生成出站配置
func (m *Manager) generateOutboundConfig(cfg *DNSConfig, xlinkPort int) []map[string]interface{} {
	// 确定domainStrategy
	domainStrategy := "UseIP"
	if cfg.PreferIPv6 {
		domainStrategy = "UseIPv6"
	} else if cfg.DisableIPv6 {
		domainStrategy = "UseIPv4"
	}

	outbounds := []map[string]interface{}{
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
				"domainStrategy": domainStrategy,
			},
		},
		{
			"tag":      "block",
			"protocol": "blackhole",
			"settings": map[string]interface{}{
				"response": map[string]interface{}{
					"type": "none",
				},
			},
		},
		{
			"tag":      "dns-out",
			"protocol": "dns",
			"settings": map[string]interface{}{
				"network": m.getDNSNetwork(cfg),
			},
		},
	}

	// 如果启用IPv6，添加IPv6专用出站
	if cfg.EnableIPv6 && !cfg.DisableIPv6 {
		outbounds = append(outbounds, map[string]interface{}{
			"tag":      "direct-ipv6",
			"protocol": "freedom",
			"settings": map[string]interface{}{
				"domainStrategy": "UseIPv6",
			},
		})
	}

	return outbounds
}

// getDNSNetwork 获取DNS网络类型
func (m *Manager) getDNSNetwork(cfg *DNSConfig) string {
	if cfg.EnableIPv6 {
		return "tcp" // TCP对IPv6更友好
	}
	return "udp"
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
		rule := m.convertUserRule(r, dnsCfg)
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

	// 私有IP直连 (IPv4)
	if hasGeoip {
		rules = append(rules, map[string]interface{}{
			"type":        "field",
			"outboundTag": "direct",
			"ip":          []string{"geoip:private"},
		})
	}

	// 私有IPv6直连
	if dnsCfg.EnableIPv6 && !dnsCfg.DisableIPv6 {
		rules = append(rules, map[string]interface{}{
			"type":        "field",
			"outboundTag": "direct",
			"ip": []string{
				"::1/128",
				"fc00::/7",
				"fe80::/10",
				"ff00::/8",
			},
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
func (m *Manager) convertUserRule(r models.RoutingRule, cfg *DNSConfig) map[string]interface{} {
	rule := map[string]interface{}{
		"type": "field",
	}

	// 确定出站标签
	target := strings.ToLower(r.Target)
	switch {
	case strings.Contains(target, "direct"):
		if cfg.PreferIPv6 && cfg.EnableIPv6 {
			rule["outboundTag"] = "direct-ipv6"
		} else {
			rule["outboundTag"] = "direct"
		}
	case strings.Contains(target, "block"):
		rule["outboundTag"] = "block"
	default:
		rule["outboundTag"] = "proxy_out"
	}

	// 根据类型设置匹配条件
	match := strings.TrimSpace(r.Match)
	ruleType := strings.ToLower(r.Type)

	switch ruleType {
	case "domain:", "domain":
		rule["domain"] = []string{"domain:" + match}
	case "regexp:", "regexp":
		rule["domain"] = []string{"regexp:" + match}
	case "geosite:", "geosite":
		rule["domain"] = []string{"geosite:" + match}
	case "geoip:", "geoip":
		rule["ip"] = []string{"geoip:" + match}
	case "ip:", "ip":
		// 检查是IPv4还是IPv6
		if isIPv6Address(match) {
			rule["ip"] = []string{match}
		} else {
			rule["ip"] = []string{match}
		}
	case "ip-cidr:", "ip-cidr", "cidr":
		rule["ip"] = []string{match}
	default:
		rule["domain"] = []string{"keyword:" + match}
	}

	return rule
}

// =============================================================================
// Fake-IP 管理 (IPv4 + IPv6)
// =============================================================================

// AllocateFakeIP 为域名分配Fake-IP (返回IPv4)
func (m *Manager) AllocateFakeIP(domain string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已分配
	if ip, exists := m.fakeIPMap[domain]; exists {
		return ip
	}

	// 分配新IP
	ip := uint32ToIPv4(m.nextFakeIP)
	ipStr := ip.String()

	m.fakeIPMap[domain] = ipStr
	m.reverseFakeIP[ipStr] = domain

	m.nextFakeIP++

	// 检查是否超出范围
	if m.nextFakeIP >= ipv4ToUint32(net.ParseIP("198.20.0.0")) {
		m.nextFakeIP = ipv4ToUint32(net.ParseIP(FakeIPPoolStart))
	}

	return ipStr
}

// AllocateFakeIPv6 为域名分配Fake-IPv6
func (m *Manager) AllocateFakeIPv6(domain string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已分配
	if ip, exists := m.fakeIPv6Map[domain]; exists {
		return ip
	}

	// 分配新IP
	ip := bigIntToIPv6(m.nextFakeIPv6)
	ipStr := ip.String()

	m.fakeIPv6Map[domain] = ipStr
	m.reverseFakeIPv6[ipStr] = domain

	// 递增
	m.nextFakeIPv6 = new(big.Int).Add(m.nextFakeIPv6, big.NewInt(1))

	// 检查是否超出范围（简化处理）
	maxIPv6 := ipv6ToBigInt(net.ParseIP("fc00:0:0:ffff::"))
	if m.nextFakeIPv6.Cmp(maxIPv6) >= 0 {
		m.nextFakeIPv6 = ipv6ToBigInt(net.ParseIP(FakeIPv6PoolStart))
	}

	return ipStr
}

// AllocateFakeIPDual 为域名分配双栈Fake-IP
func (m *Manager) AllocateFakeIPDual(domain string) (ipv4, ipv6 string) {
	ipv4 = m.AllocateFakeIP(domain)
	ipv6 = m.AllocateFakeIPv6(domain)
	return
}

// LookupFakeIP 通过Fake-IP查询域名（支持IPv4和IPv6）
func (m *Manager) LookupFakeIP(ip string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 先尝试IPv4
	if domain, exists := m.reverseFakeIP[ip]; exists {
		return domain, true
	}

	// 再尝试IPv6
	if domain, exists := m.reverseFakeIPv6[ip]; exists {
		return domain, true
	}

	// 尝试规范化后再查找
	parsed := net.ParseIP(ip)
	if parsed != nil {
		normalizedIP := parsed.String()
		if domain, exists := m.reverseFakeIP[normalizedIP]; exists {
			return domain, true
		}
		if domain, exists := m.reverseFakeIPv6[normalizedIP]; exists {
			return domain, true
		}
	}

	return "", false
}

// IsFakeIP 检查是否是Fake-IP（支持IPv4和IPv6）
func (m *Manager) IsFakeIP(ip string) bool {
	return m.IsFakeIPv4(ip) || m.IsFakeIPv6(ip)
}

// IsFakeIPv4 检查是否是Fake-IPv4
func (m *Manager) IsFakeIPv4(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}

	// 必须是IPv4
	if parsed.To4() == nil {
		return false
	}

	_, fakeNet, _ := net.ParseCIDR(FakeIPPoolCIDR)
	return fakeNet.Contains(parsed)
}

// IsFakeIPv6 检查是否是Fake-IPv6
func (m *Manager) IsFakeIPv6(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}

	// 必须是IPv6
	if parsed.To4() != nil {
		return false
	}

	_, fakeNet, _ := net.ParseCIDR(FakeIPv6PoolCIDR)
	return fakeNet.Contains(parsed)
}

// ClearFakeIPCache 清空Fake-IP缓存
func (m *Manager) ClearFakeIPCache() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fakeIPMap = make(map[string]string)
	m.reverseFakeIP = make(map[string]string)
	m.fakeIPv6Map = make(map[string]string)
	m.reverseFakeIPv6 = make(map[string]string)
	m.nextFakeIP = ipv4ToUint32(net.ParseIP(FakeIPPoolStart))
	m.nextFakeIPv6 = ipv6ToBigInt(net.ParseIP(FakeIPv6PoolStart))
}

// GetFakeIPStats 获取Fake-IP统计
func (m *Manager) GetFakeIPStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"ipv4_count": len(m.fakeIPMap),
		"ipv6_count": len(m.fakeIPv6Map),
		"total":      len(m.fakeIPMap) + len(m.fakeIPv6Map),
	}
}

// =============================================================================
// 系统DNS操作（Windows - 支持IPv6）
// =============================================================================

// SystemDNSInfo 系统DNS信息
type SystemDNSInfo struct {
	InterfaceName string   `json:"interface_name"`
	IPv4DNS       []string `json:"ipv4_dns"`
	IPv6DNS       []string `json:"ipv6_dns"`
	IsDHCP        bool     `json:"is_dhcp"`
}

// GetSystemDNS 获取系统DNS设置
func (m *Manager) GetSystemDNS() ([]SystemDNSInfo, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("仅支持Windows")
	}

	var results []SystemDNSInfo

	// 获取所有网络接口
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		// 跳过非活动接口
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		info := SystemDNSInfo{
			InterfaceName: iface.Name,
		}

		// 获取IPv4 DNS
		ipv4DNS, _ := m.getInterfaceDNS(iface.Name, false)
		info.IPv4DNS = ipv4DNS

		// 获取IPv6 DNS
		ipv6DNS, _ := m.getInterfaceDNS(iface.Name, true)
		info.IPv6DNS = ipv6DNS

		if len(ipv4DNS) > 0 || len(ipv6DNS) > 0 {
			results = append(results, info)
		}
	}

	return results, nil
}

// getInterfaceDNS 获取指定接口的DNS
func (m *Manager) getInterfaceDNS(interfaceName string, ipv6 bool) ([]string, error) {
	var cmd *exec.Cmd
	if ipv6 {
		cmd = exec.Command("netsh", "interface", "ipv6", "show", "dns", fmt.Sprintf("name=%s", interfaceName))
	} else {
		cmd = exec.Command("netsh", "interface", "ip", "show", "dns", fmt.Sprintf("name=%s", interfaceName))
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return m.parseDNSOutput(string(output), ipv6), nil
}

// parseDNSOutput 解析DNS输出
func (m *Manager) parseDNSOutput(output string, ipv6 bool) []string {
	var dns []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 尝试解析IP地址
		// 支持格式: "DNS Servers: 8.8.8.8" 或直接IP
		parts := strings.Split(line, ":")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			ip := net.ParseIP(part)
			if ip != nil {
				isV6 := ip.To4() == nil
				if ipv6 == isV6 {
					dns = append(dns, ip.String())
				}
			}
		}
	}

	return dns
}

// SetSystemDNS 设置系统DNS（需要管理员权限）
func (m *Manager) SetSystemDNS(interfaceName string, ipv4DNS, ipv6DNS []string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("仅支持Windows")
	}

	var errs []string

	// 设置IPv4 DNS
	if len(ipv4DNS) > 0 {
		if err := m.setInterfaceDNS(interfaceName, ipv4DNS, false); err != nil {
			errs = append(errs, fmt.Sprintf("IPv4: %v", err))
		}
	}

	// 设置IPv6 DNS
	if len(ipv6DNS) > 0 {
		if err := m.setInterfaceDNS(interfaceName, ipv6DNS, true); err != nil {
			errs = append(errs, fmt.Sprintf("IPv6: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("设置DNS失败: %s", strings.Join(errs, "; "))
	}

	return nil
}

// setInterfaceDNS 设置指定接口的DNS
func (m *Manager) setInterfaceDNS(interfaceName string, dns []string, ipv6 bool) error {
	if len(dns) == 0 {
		return nil
	}

	var protocol string
	if ipv6 {
		protocol = "ipv6"
	} else {
		protocol = "ip"
	}

	// 设置主DNS
	cmd := exec.Command("netsh", "interface", protocol, "set", "dns",
		fmt.Sprintf("name=%s", interfaceName),
		"source=static",
		fmt.Sprintf("addr=%s", dns[0]),
		"validate=no",
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("设置主DNS失败: %v", err)
	}

	// 添加备用DNS
	for i := 1; i < len(dns); i++ {
		cmd = exec.Command("netsh", "interface", protocol, "add", "dns",
			fmt.Sprintf("name=%s", interfaceName),
			fmt.Sprintf("addr=%s", dns[i]),
			fmt.Sprintf("index=%d", i+1),
			"validate=no",
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

	// 重置IPv4 DNS
	cmd := exec.Command("netsh", "interface", "ip", "set", "dns",
		fmt.Sprintf("name=%s", interfaceName),
		"source=dhcp",
	)
	if err := cmd.Run(); err != nil {
		m.log("warn", fmt.Sprintf("重置IPv4 DNS失败: %v", err))
	}

	// 重置IPv6 DNS
	cmd = exec.Command("netsh", "interface", "ipv6", "set", "dns",
		fmt.Sprintf("name=%s", interfaceName),
		"source=dhcp",
	)
	if err := cmd.Run(); err != nil {
		m.log("warn", fmt.Sprintf("重置IPv6 DNS失败: %v", err))
	}

	return nil
}

// =============================================================================
// IPv6 检测
// =============================================================================

// CheckIPv6Support 检测系统IPv6支持状态
func (m *Manager) CheckIPv6Support() *IPv6SupportInfo {
	info := &IPv6SupportInfo{
		HasIPv6Interface: false,
		HasIPv6Address:   false,
		HasIPv6Gateway:   false,
		IPv6Addresses:    []string{},
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return info
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			if ip.To4() == nil && ip.To16() != nil {
				// 这是IPv6地址
				info.HasIPv6Interface = true

				// 检查是否是全局单播地址
				if ip.IsGlobalUnicast() && !ip.IsLinkLocalUnicast() {
					info.HasIPv6Address = true
					info.IPv6Addresses = append(info.IPv6Addresses, ip.String())
				}
			}
		}
	}

	// 尝试连接IPv6测试服务器
	if info.HasIPv6Address {
		conn, err := net.DialTimeout("tcp6", "[2001:4860:4860::8888]:53", 3*1e9)
		if err == nil {
			conn.Close()
			info.HasIPv6Gateway = true
			info.IPv6Connectivity = true
		}
	}

	return info
}

// IPv6SupportInfo IPv6支持信息
type IPv6SupportInfo struct {
	HasIPv6Interface bool     `json:"has_ipv6_interface"`
	HasIPv6Address   bool     `json:"has_ipv6_address"`
	HasIPv6Gateway   bool     `json:"has_ipv6_gateway"`
	IPv6Connectivity bool     `json:"ipv6_connectivity"`
	IPv6Addresses    []string `json:"ipv6_addresses"`
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

// parseListenAddr 解析监听地址（支持IPv4、IPv6、域名）
func (m *Manager) parseListenAddr(addr string) (host string, port int) {
	host = "127.0.0.1"
	port = 10808

	if addr == "" {
		return
	}

	addr = strings.TrimSpace(addr)

	// 处理IPv6格式 [::1]:8080 或 [2001:db8::1]:8080
	if strings.HasPrefix(addr, "[") {
		// 找到 ]:
		idx := strings.LastIndex(addr, "]:")
		if idx != -1 {
			host = addr[1:idx] // 去掉方括号
			fmt.Sscanf(addr[idx+2:], "%d", &port)
			return
		}
		// 可能只有 [::1] 没有端口
		if strings.HasSuffix(addr, "]") {
			host = addr[1 : len(addr)-1]
			return
		}
	}

	// 处理标准格式 host:port
	// 需要区分 IPv6 裸地址（如 ::1）和 host:port
	lastColon := strings.LastIndex(addr, ":")
	if lastColon == -1 {
		// 没有冒号，可能是纯主机名或IPv4
		host = addr
		return
	}

	// 检查是否是IPv6地址（包含多个冒号）
	if strings.Count(addr, ":") > 1 {
		// 尝试解析为纯IPv6地址
		ip := net.ParseIP(addr)
		if ip != nil {
			host = addr
			return
		}
	}

	// 标准 host:port 格式
	host = addr[:lastColon]
	fmt.Sscanf(addr[lastColon+1:], "%d", &port)

	return
}

// ipv4ToUint32 IPv4转uint32
func ipv4ToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// uint32ToIPv4 uint32转IPv4
func uint32ToIPv4(n uint32) net.IP {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}

// ipv6ToBigInt IPv6转big.Int
func ipv6ToBigInt(ip net.IP) *big.Int {
	ip = ip.To16()
	if ip == nil {
		return big.NewInt(0)
	}
	return new(big.Int).SetBytes(ip)
}

// bigIntToIPv6 big.Int转IPv6
func bigIntToIPv6(n *big.Int) net.IP {
	b := n.Bytes()
	// 补齐到16字节
	ip := make(net.IP, 16)
	if len(b) <= 16 {
		copy(ip[16-len(b):], b)
	} else {
		copy(ip, b[len(b)-16:])
	}
	return ip
}

// isIPv6Address 检查是否是IPv6地址
func isIPv6Address(addr string) bool {
	// 去掉可能的方括号
	addr = strings.Trim(addr, "[]")
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	return ip.To4() == nil
}

// isIPv4Address 检查是否是IPv4地址
func isIPv4Address(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	return ip.To4() != nil
}

// isDomainName 检查是否是域名
func isDomainName(addr string) bool {
	// 去掉端口
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		// 检查是否是IPv6
		if strings.Count(addr, ":") == 1 {
			addr = addr[:idx]
		}
	}

	// 不是IP地址就是域名
	ip := net.ParseIP(addr)
	return ip == nil && len(addr) > 0
}

// normalizeIP 规范化IP地址
func normalizeIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed != nil {
		return parsed.String()
	}
	return ip
}

// FormatIPv6ForURL 格式化IPv6地址用于URL
func FormatIPv6ForURL(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	if parsed.To4() != nil {
		return ip // IPv4不需要方括号
	}
	return "[" + parsed.String() + "]"
}

// FileExists 检查文件是否存在
func (m *Manager) FileExists(filename string) bool {
	path := filepath.Join(m.exeDir, filename)
	_, err := os.Stat(path)
	return err == nil
}

// =============================================================================
// DNS服务器预设
// =============================================================================

// DNSPreset DNS预设配置
type DNSPreset struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IPv4        []string `json:"ipv4"`
	IPv6        []string `json:"ipv6"`
	DoH         string   `json:"doh,omitempty"`
	DoT         string   `json:"dot,omitempty"`
}

// GetDNSPresets 获取DNS预设列表
func (m *Manager) GetDNSPresets() []DNSPreset {
	return []DNSPreset{
		{
			Name:        "Cloudflare",
			Description: "Cloudflare DNS - 快速安全",
			IPv4:        []string{DNSCloudflare, DNSCloudflareAlt},
			IPv6:        []string{DNSCloudflareIPv6, DNSCloudflareIPv6Alt},
			DoH:         DNSCloudflareDoH,
			DoT:         DNSCloudflareDoT,
		},
		{
			Name:        "Google",
			Description: "Google Public DNS",
			IPv4:        []string{DNSGoogle, DNSGoogleAlt},
			IPv6:        []string{DNSGoogleIPv6, DNSGoogleIPv6Alt},
			DoH:         DNSGoogleDoH,
			DoT:         DNSGoogleDoT,
		},
		{
			Name:        "阿里DNS",
			Description: "阿里公共DNS - 国内推荐",
			IPv4:        []string{DNSAliDNS, DNSAliDNSAlt},
			IPv6:        []string{DNSAliDNSIPv6, DNSAliDNSIPv6Alt},
			DoH:         DNSAliDoH,
		},
		{
			Name:        "腾讯DNS",
			Description: "腾讯DNSPod Public DNS",
			IPv4:        []string{DNSTencent, DNSTencentAlt},
			IPv6:        []string{DNSTencentIPv6, DNSTencentIPv6Alt},
			DoH:         DNSTencentDoH,
		},
	}
}
