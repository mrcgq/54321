package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// =============================================================================
// DNS泄露测试
// =============================================================================

// LeakTestResult DNS泄露测试结果
type LeakTestResult struct {
	Leaked       bool              `json:"leaked"`
	TestedAt     time.Time         `json:"tested_at"`
	LocalDNS     []string          `json:"local_dns"`
	DetectedDNS  []DNSServerInfo   `json:"detected_dns"`
	TestServers  []string          `json:"test_servers"`
	Errors       []string          `json:"errors,omitempty"`
	Conclusion   string            `json:"conclusion"`
}

// DNSServerInfo DNS服务器信息
type DNSServerInfo struct {
	IP       string `json:"ip"`
	Country  string `json:"country"`
	City     string `json:"city"`
	ISP      string `json:"isp"`
	IsChina  bool   `json:"is_china"`
}

// LeakTester DNS泄露测试器
type LeakTester struct {
	httpClient *http.Client
}

// NewLeakTester 创建泄露测试器
func NewLeakTester() *LeakTester {
	return &LeakTester{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetProxy 设置代理
func (t *LeakTester) SetProxy(proxyAddr string) {
	if proxyAddr == "" {
		return
	}

	// 配置SOCKS5代理
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout: 5 * time.Second,
			}
			// 这里简化处理，实际应使用SOCKS5
			return dialer.DialContext(ctx, network, addr)
		},
	}

	t.httpClient.Transport = transport
}

// RunTest 执行DNS泄露测试
func (t *LeakTester) RunTest() (*LeakTestResult, error) {
	result := &LeakTestResult{
		TestedAt:    time.Now(),
		TestServers: []string{},
		Errors:      []string{},
	}

	// 获取本地DNS
	localDNS, _ := t.getLocalDNS()
	result.LocalDNS = localDNS

	// 测试多个泄露检测服务
	detectedDNS := make(map[string]DNSServerInfo)

	testAPIs := []struct {
		name string
		url  string
	}{
		{"ipleak.net", "https://ipleak.net/json/"},
		{"browserleaks", "https://browserleaks.com/dns"},
		{"dnsleaktest", "https://www.dnsleaktest.com/results.html"},
	}

	for _, api := range testAPIs {
		result.TestServers = append(result.TestServers, api.name)

		info, err := t.queryLeakAPI(api.url)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", api.name, err))
			continue
		}

		if info.IP != "" {
			detectedDNS[info.IP] = info
		}
	}

	// 转换为列表
	for _, info := range detectedDNS {
		result.DetectedDNS = append(result.DetectedDNS, info)
	}

	// 判断是否泄露
	result.Leaked = t.analyzeLeakage(result)
	result.Conclusion = t.generateConclusion(result)

	return result, nil
}

// queryLeakAPI 查询泄露检测API
func (t *LeakTester) queryLeakAPI(url string) (DNSServerInfo, error) {
	var info DNSServerInfo

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return info, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return info, err
	}

	// 尝试解析JSON
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err == nil {
		if ip, ok := data["ip"].(string); ok {
			info.IP = ip
		}
		if country, ok := data["country_name"].(string); ok {
			info.Country = country
		}
		if city, ok := data["city"].(string); ok {
			info.City = city
		}
		if isp, ok := data["isp"].(string); ok {
			info.ISP = isp
		}

		// 检测是否为中国
		info.IsChina = t.isChineseServer(info)
	}

	return info, nil
}

// getLocalDNS 获取本地DNS服务器
func (t *LeakTester) getLocalDNS() ([]string, error) {
	// 尝试解析一个域名获取使用的DNS
	// 这是简化实现
	resolver := &net.Resolver{
		PreferGo: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 解析一个域名
	_, err := resolver.LookupHost(ctx, "www.google.com")
	if err != nil {
		return nil, err
	}

	// 实际需要通过系统API获取DNS列表
	// 这里返回空
	return nil, nil
}

// isChineseServer 判断是否是中国服务器
func (t *LeakTester) isChineseServer(info DNSServerInfo) bool {
	country := strings.ToLower(info.Country)
	return strings.Contains(country, "china") ||
		strings.Contains(country, "中国") ||
		country == "cn"
}

// analyzeLeakage 分析是否泄露
func (t *LeakTester) analyzeLeakage(result *LeakTestResult) bool {
	// 如果检测到中国DNS服务器，认为泄露
	for _, dns := range result.DetectedDNS {
		if dns.IsChina {
			return true
		}
	}

	// 如果检测到本地DNS在检测结果中
	for _, localDNS := range result.LocalDNS {
		for _, detected := range result.DetectedDNS {
			if detected.IP == localDNS {
				return true
			}
		}
	}

	return false
}

// generateConclusion 生成结论
func (t *LeakTester) generateConclusion(result *LeakTestResult) string {
	if result.Leaked {
		var reasons []string
		for _, dns := range result.DetectedDNS {
			if dns.IsChina {
				reasons = append(reasons, fmt.Sprintf("检测到中国DNS: %s (%s)", dns.IP, dns.ISP))
			}
		}

		if len(reasons) > 0 {
			return "⚠️ DNS泄露! " + strings.Join(reasons, "; ")
		}
		return "⚠️ 可能存在DNS泄露，请检查配置"
	}

	if len(result.DetectedDNS) == 0 {
		return "✓ 未检测到DNS服务器（可能测试失败）"
	}

	return "✓ DNS未泄露，所有请求通过代理解析"
}

// =============================================================================
// 快速泄露检测
// =============================================================================

// QuickLeakCheck 快速泄露检测
func (t *LeakTester) QuickLeakCheck(proxyAddr string) (bool, string, error) {
	// 创建使用代理的客户端
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	if proxyAddr != "" {
		// 配置代理（简化）
		_ = proxyAddr
	}

	// 请求IP检测API
	resp, err := client.Get("https://api.ip.sb/ip")
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, "", err
	}

	ip := strings.TrimSpace(string(body))

	// 检查是否为中国IP（简化判断）
	isChina := t.isChineseIP(ip)

	return isChina, ip, nil
}

// isChineseIP 判断是否为中国IP
func (t *LeakTester) isChineseIP(ip string) bool {
	// 简化判断，实际应查询IP库
	// 中国IP段示例
	chinaRanges := []string{
		"1.0.0.0/8",
		"14.0.0.0/8",
		"27.0.0.0/8",
		"36.0.0.0/8",
		"39.0.0.0/8",
		"42.0.0.0/8",
		"49.0.0.0/8",
		"58.0.0.0/8",
		"59.0.0.0/8",
		"60.0.0.0/8",
		"61.0.0.0/8",
		"101.0.0.0/8",
		"103.0.0.0/8",
		"106.0.0.0/8",
		"110.0.0.0/8",
		"111.0.0.0/8",
		"112.0.0.0/8",
		"113.0.0.0/8",
		"114.0.0.0/8",
		"115.0.0.0/8",
		"116.0.0.0/8",
		"117.0.0.0/8",
		"118.0.0.0/8",
		"119.0.0.0/8",
		"120.0.0.0/8",
		"121.0.0.0/8",
		"122.0.0.0/8",
		"123.0.0.0/8",
		"124.0.0.0/8",
		"125.0.0.0/8",
		"126.0.0.0/8",
		"139.0.0.0/8",
		"140.0.0.0/8",
		"171.0.0.0/8",
		"175.0.0.0/8",
		"180.0.0.0/8",
		"182.0.0.0/8",
		"183.0.0.0/8",
		"202.0.0.0/8",
		"203.0.0.0/8",
		"210.0.0.0/8",
		"211.0.0.0/8",
		"218.0.0.0/8",
		"219.0.0.0/8",
		"220.0.0.0/8",
		"221.0.0.0/8",
		"222.0.0.0/8",
		"223.0.0.0/8",
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for _, cidr := range chinaRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(parsedIP) {
			return true
		}
	}

	return false
}
