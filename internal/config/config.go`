// Package config 处理配置文件的加载、保存和加密
package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xlink-wails/internal/models"
)

// =============================================================================
// 常量
// =============================================================================

const (
	ConfigFileName       = "xlink_config.json"
	ConfigFileNameEnc    = "xlink_config.enc"
	ConfigBackupDir      = "backups"
	MaxBackups           = 5
	EncryptionKeyEnvVar  = "XLINK_CONFIG_KEY"
	DefaultEncryptionKey = "xlink-wails-default-key-2024" // 默认密钥（生产环境应使用环境变量）
)

// =============================================================================
// 配置管理器
// =============================================================================

// Manager 配置管理器
type Manager struct {
	mu       sync.RWMutex
	exeDir   string
	config   *models.AppConfig
	filePath string
	encKey   []byte
}

// NewManager 创建配置管理器
func NewManager(exeDir string) *Manager {
	m := &Manager{
		exeDir:   exeDir,
		filePath: filepath.Join(exeDir, ConfigFileName),
		config:   &models.AppConfig{},
	}

	// 获取加密密钥
	key := os.Getenv(EncryptionKeyEnvVar)
	if key == "" {
		key = DefaultEncryptionKey
	}
	// 使用SHA256生成固定长度的密钥
	hash := sha256.Sum256([]byte(key))
	m.encKey = hash[:]

	return m
}

// =============================================================================
// 加载配置
// =============================================================================

// Load 加载配置文件
func (m *Manager) Load() (*models.AppConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 尝试按优先级加载配置
	// 1. 加密配置文件
	// 2. 明文JSON配置文件
	// 3. 旧版加密配置文件（.dat）
	// 4. 创建默认配置

	encPath := filepath.Join(m.exeDir, ConfigFileNameEnc)
	jsonPath := filepath.Join(m.exeDir, ConfigFileName)
	legacyPath := filepath.Join(m.exeDir, "xlink_config.dat")

	var config *models.AppConfig
	var err error

	// 尝试加载加密配置
	if fileExists(encPath) {
		config, err = m.loadEncrypted(encPath)
		if err == nil {
			m.config = config
			return config, nil
		}
		// 加密文件损坏，尝试其他方式
	}

	// 尝试加载明文JSON配置
	if fileExists(jsonPath) {
		config, err = m.loadJSON(jsonPath)
		if err == nil {
			m.config = config
			// 迁移到加密存储
			go m.Save()
			return config, nil
		}
	}

	// 尝试加载旧版配置（兼容C版本）
	if fileExists(legacyPath) {
		config, err = m.loadLegacy(legacyPath)
		if err == nil {
			m.config = config
			// 迁移到新格式
			go m.Save()
			return config, nil
		}
	}

	// 创建默认配置
	config = m.createDefaultConfig()
	m.config = config

	// 保存默认配置
	go m.Save()

	return config, nil
}

// loadJSON 加载明文JSON配置
func (m *Manager) loadJSON(path string) (*models.AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config models.AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 验证并修复配置
	m.validateAndFix(&config)

	return &config, nil
}

// loadEncrypted 加载加密配置
func (m *Manager) loadEncrypted(path string) (*models.AppConfig, error) {
	encData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取加密配置失败: %w", err)
	}

	// Base64解码
	ciphertext, err := base64.StdEncoding.DecodeString(string(encData))
	if err != nil {
		return nil, fmt.Errorf("Base64解码失败: %w", err)
	}

	// AES-GCM解密
	plaintext, err := m.decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("解密失败: %w", err)
	}

	var config models.AppConfig
	if err := json.Unmarshal(plaintext, &config); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	m.validateAndFix(&config)

	return &config, nil
}

// loadLegacy 加载旧版C程序配置（兼容性）
func (m *Manager) loadLegacy(path string) (*models.AppConfig, error) {
	// 旧版使用Windows DPAPI加密
	// 这里提供基本的迁移支持
	// 注意：完整的DPAPI解密需要调用Windows API

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// 尝试直接解析（如果是明文）
	var legacyData struct {
		NodeCount int `json:"node_count"`
		Nodes     []struct {
			Name        string `json:"name"`
			Listen      string `json:"listen"`
			Server      string `json:"server"`
			IP          string `json:"ip"`
			Token       string `json:"token"`
			SecretKey   string `json:"secret_key"`
			FallbackIP  string `json:"fallback_ip"`
			S5          string `json:"s5"`
			RulesStr    string `json:"rules_str"`
			RoutingMode int    `json:"routing_mode"`
			Strategy    int    `json:"strategy_mode"`
		} `json:"nodes"`
	}

	if err := json.Unmarshal(data, &legacyData); err != nil {
		// 可能是DPAPI加密的数据，无法在Go中直接解密
		// 返回错误让调用者创建默认配置
		return nil, fmt.Errorf("无法解析旧版配置: %w", err)
	}

	// 转换为新格式
	config := m.createDefaultConfig()
	config.Nodes = make([]models.NodeConfig, 0, len(legacyData.Nodes))

	for _, old := range legacyData.Nodes {
		node := models.NodeConfig{
			ID:           models.GenerateUUID(),
			Name:         old.Name,
			Listen:       old.Listen,
			Server:       old.Server,
			IP:           old.IP,
			Token:        old.Token,
			SecretKey:    old.SecretKey,
			FallbackIP:   old.FallbackIP,
			Socks5:       old.S5,
			RulesStr:     old.RulesStr,
			RoutingMode:  old.RoutingMode,
			StrategyMode: old.Strategy,
			DNSMode:      models.DNSModeFakeIP,
			Status:       models.StatusStopped,
		}

		// 解析旧版规则字符串
		node.Rules = parseRulesString(old.RulesStr)

		config.Nodes = append(config.Nodes, node)
	}

	return config, nil
}

// =============================================================================
// 保存配置
// =============================================================================

// Save 保存配置（加密）
func (m *Manager) Save() error {
	m.mu.RLock()
	config := m.config
	m.mu.RUnlock()

	if config == nil {
		return fmt.Errorf("配置为空")
	}

	// 创建备份
	m.createBackup()

	// 序列化配置
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// 加密
	ciphertext, err := m.encrypt(data)
	if err != nil {
		return fmt.Errorf("加密配置失败: %w", err)
	}

	// Base64编码
	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	// 写入文件
	encPath := filepath.Join(m.exeDir, ConfigFileNameEnc)
	if err := os.WriteFile(encPath, []byte(encoded), 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	// 同时保存明文版本（用于调试，生产环境可移除）
	jsonPath := filepath.Join(m.exeDir, ConfigFileName)
	_ = os.WriteFile(jsonPath, data, 0600)

	return nil
}

// SaveAs 保存配置到指定路径（明文）
func (m *Manager) SaveAs(path string) error {
	m.mu.RLock()
	config := m.config
	m.mu.RUnlock()

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// =============================================================================
// 配置更新
// =============================================================================

// GetConfig 获取当前配置（只读）
func (m *Manager) GetConfig() *models.AppConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// UpdateConfig 更新整个配置
func (m *Manager) UpdateConfig(config *models.AppConfig) {
	m.mu.Lock()
	m.config = config
	m.mu.Unlock()
}

// UpdateNode 更新单个节点
func (m *Manager) UpdateNode(node models.NodeConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.config.Nodes {
		if m.config.Nodes[i].ID == node.ID {
			m.config.Nodes[i] = node
			return nil
		}
	}

	return fmt.Errorf("节点不存在: %s", node.ID)
}

// AddNode 添加节点
func (m *Manager) AddNode(node models.NodeConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.config.Nodes) >= models.MaxNodes {
		return fmt.Errorf("节点数量已达上限")
	}

	m.config.Nodes = append(m.config.Nodes, node)
	return nil
}

// DeleteNode 删除节点
func (m *Manager) DeleteNode(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.config.Nodes {
		if m.config.Nodes[i].ID == id {
			m.config.Nodes = append(m.config.Nodes[:i], m.config.Nodes[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("节点不存在: %s", id)
}

// =============================================================================
// 加密解密
// =============================================================================

// encrypt 使用AES-GCM加密
func (m *Manager) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(m.encKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decrypt 使用AES-GCM解密
func (m *Manager) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(m.encKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("密文太短")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// =============================================================================
// 备份管理
// =============================================================================

// createBackup 创建配置备份
func (m *Manager) createBackup() {
	backupDir := filepath.Join(m.exeDir, ConfigBackupDir)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return
	}

	// 检查源文件是否存在
	srcPath := filepath.Join(m.exeDir, ConfigFileNameEnc)
	if !fileExists(srcPath) {
		srcPath = filepath.Join(m.exeDir, ConfigFileName)
		if !fileExists(srcPath) {
			return
		}
	}

	// 创建备份文件名
	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("config_backup_%s.enc", timestamp))

	// 复制文件
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return
	}
	_ = os.WriteFile(backupPath, data, 0600)

	// 清理旧备份
	m.cleanOldBackups(backupDir)
}

// cleanOldBackups 清理旧备份
func (m *Manager) cleanOldBackups(backupDir string) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}

	var backups []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "config_backup_") {
			backups = append(backups, e)
		}
	}

	// 保留最新的N个备份
	if len(backups) <= MaxBackups {
		return
	}

	// 按时间排序（文件名包含时间戳）
	for i := 0; i < len(backups)-MaxBackups; i++ {
		os.Remove(filepath.Join(backupDir, backups[i].Name()))
	}
}

// RestoreBackup 从备份恢复
func (m *Manager) RestoreBackup(backupName string) error {
	backupPath := filepath.Join(m.exeDir, ConfigBackupDir, backupName)
	if !fileExists(backupPath) {
		return fmt.Errorf("备份文件不存在: %s", backupName)
	}

	data, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}

	// 解密备份
	ciphertext, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return err
	}

	plaintext, err := m.decrypt(ciphertext)
	if err != nil {
		return err
	}

	var config models.AppConfig
	if err := json.Unmarshal(plaintext, &config); err != nil {
		return err
	}

	m.mu.Lock()
	m.config = &config
	m.mu.Unlock()

	return m.Save()
}

// ListBackups 列出所有备份
func (m *Manager) ListBackups() []string {
	backupDir := filepath.Join(m.exeDir, ConfigBackupDir)
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil
	}

	var backups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "config_backup_") {
			backups = append(backups, e.Name())
		}
	}

	return backups
}

// =============================================================================
// 辅助函数
// =============================================================================

// createDefaultConfig 创建默认配置
func (m *Manager) createDefaultConfig() *models.AppConfig {
	return &models.AppConfig{
		Nodes: []models.NodeConfig{
			models.NewDefaultNode("默认节点"),
		},
		AutoStart:        false,
		MinimizeToTray:   true,
		Theme:            "system",
		Language:         "zh-CN",
		GlobalDNSMode:    models.DNSModeFakeIP,
		TUNInterfaceName: "XlinkTUN",
	}
}

// validateAndFix 验证并修复配置
func (m *Manager) validateAndFix(config *models.AppConfig) {
	// 确保至少有一个节点
	if len(config.Nodes) == 0 {
		config.Nodes = []models.NodeConfig{
			models.NewDefaultNode("默认节点"),
		}
	}

	// 验证每个节点
	for i := range config.Nodes {
		node := &config.Nodes[i]

		// 确保有ID
		if node.ID == "" {
			node.ID = models.GenerateUUID()
		}

		// 确保有名称
		if node.Name == "" {
			node.Name = fmt.Sprintf("节点 %d", i+1)
		}

		// 确保有监听地址
		if node.Listen == "" {
			node.Listen = "127.0.0.1:10808"
		}

		// 初始化状态
		node.Status = models.StatusStopped

		// 解析旧版规则字符串
		if len(node.Rules) == 0 && node.RulesStr != "" {
			node.Rules = parseRulesString(node.RulesStr)
		}
	}

	// 验证主题
	if config.Theme == "" {
		config.Theme = "system"
	}

	// 验证语言
	if config.Language == "" {
		config.Language = "zh-CN"
	}
}

// parseRulesString 解析旧版规则字符串
func parseRulesString(rulesStr string) []models.RoutingRule {
	if rulesStr == "" {
		return nil
	}

	var rules []models.RoutingRule
	lines := strings.Split(rulesStr, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "\r")
		line = strings.TrimSuffix(line, "\r")

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ",", 2)
		if len(parts) != 2 {
			continue
		}

		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])

		// 移除旧版后缀
		right = strings.TrimSuffix(right, "|keep")
		right = strings.TrimSuffix(right, "|cut")

		rule := models.RoutingRule{
			ID:     models.GenerateUUID(),
			Target: right,
		}

		// 解析类型前缀
		switch {
		case strings.HasPrefix(left, "domain:"):
			rule.Type = "domain:"
			rule.Match = strings.TrimPrefix(left, "domain:")
		case strings.HasPrefix(left, "regexp:"):
			rule.Type = "regexp:"
			rule.Match = strings.TrimPrefix(left, "regexp:")
		case strings.HasPrefix(left, "geosite:"):
			rule.Type = "geosite:"
			rule.Match = strings.TrimPrefix(left, "geosite:")
		case strings.HasPrefix(left, "geoip:"):
			rule.Type = "geoip:"
			rule.Match = strings.TrimPrefix(left, "geoip:")
		default:
			rule.Type = ""
			rule.Match = left
		}

		rules = append(rules, rule)
	}

	return rules
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// =============================================================================
// 导入导出
// =============================================================================

// ExportNode 导出单个节点为xlink://链接
func (m *Manager) ExportNode(nodeID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var node *models.NodeConfig
	for i := range m.config.Nodes {
		if m.config.Nodes[i].ID == nodeID {
			node = &m.config.Nodes[i]
			break
		}
	}

	if node == nil {
		return "", fmt.Errorf("节点不存在: %s", nodeID)
	}

	return buildXlinkURI(node), nil
}

// ImportNodes 从xlink://链接导入节点
func (m *Manager) ImportNodes(text string) ([]models.NodeConfig, error) {
	lines := strings.Split(text, "\n")
	var imported []models.NodeConfig

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "xlink://") {
			continue
		}

		node, err := parseXlinkURI(line)
		if err != nil {
			continue
		}

		imported = append(imported, *node)
	}

	if len(imported) == 0 {
		return nil, fmt.Errorf("未找到有效的xlink://链接")
	}

	// 添加到配置
	m.mu.Lock()
	for _, node := range imported {
		if len(m.config.Nodes) < models.MaxNodes {
			m.config.Nodes = append(m.config.Nodes, node)
		}
	}
	m.mu.Unlock()

	return imported, nil
}

// buildXlinkURI 构建xlink://链接
func buildXlinkURI(node *models.NodeConfig) string {
	var params []string

	if node.SecretKey != "" {
		params = append(params, "key="+node.SecretKey)
	}
	if node.FallbackIP != "" {
		params = append(params, "fallback="+node.FallbackIP)
	}
	if node.IP != "" {
		params = append(params, "ip="+node.IP)
	}
	if node.Socks5 != "" {
		params = append(params, "s5="+node.Socks5)
	}
	if node.RoutingMode == models.RoutingModeSmart {
		params = append(params, "route=cn")
	}
	if node.StrategyMode != models.StrategyRandom {
		strategy := "rr"
		if node.StrategyMode == models.StrategyHash {
			strategy = "hash"
		}
		params = append(params, "strategy="+strategy)
	}
	if node.DNSMode != models.DNSModeStandard {
		dnsMode := "fakeip"
		if node.DNSMode == models.DNSModeTUN {
			dnsMode = "tun"
		}
		params = append(params, "dns="+dnsMode)
	}

	// 序列化规则
	if len(node.Rules) > 0 {
		var ruleStrs []string
		for _, r := range node.Rules {
			ruleStrs = append(ruleStrs, r.Type+r.Match+","+r.Target)
		}
		params = append(params, "rules="+strings.Join(ruleStrs, "|"))
	}

	uri := fmt.Sprintf("xlink://%s@%s", node.Token, node.Server)
	if len(params) > 0 {
		uri += "?" + strings.Join(params, "&")
	}
	uri += "#" + node.Name

	return uri
}

// parseXlinkURI 解析xlink://链接
func parseXlinkURI(uri string) (*models.NodeConfig, error) {
	if !strings.HasPrefix(uri, "xlink://") {
		return nil, fmt.Errorf("无效的URI格式")
	}

	// 移除前缀
	uri = strings.TrimPrefix(uri, "xlink://")

	node := models.NewDefaultNode("")

	// 解析名称（#后面的部分）
	if idx := strings.LastIndex(uri, "#"); idx != -1 {
		node.Name = uri[idx+1:]
		uri = uri[:idx]
	}

	// 解析参数（?后面的部分）
	if idx := strings.Index(uri, "?"); idx != -1 {
		paramStr := uri[idx+1:]
		uri = uri[:idx]

		params := strings.Split(paramStr, "&")
		for _, param := range params {
			kv := strings.SplitN(param, "=", 2)
			if len(kv) != 2 {
				continue
			}
			key, value := kv[0], kv[1]

			switch key {
			case "key":
				node.SecretKey = value
			case "fallback":
				node.FallbackIP = value
			case "ip":
				node.IP = value
			case "s5":
				node.Socks5 = value
			case "route":
				if value == "cn" {
					node.RoutingMode = models.RoutingModeSmart
				}
			case "strategy":
				switch value {
				case "rr":
					node.StrategyMode = models.StrategyRR
				case "hash":
					node.StrategyMode = models.StrategyHash
				}
			case "dns":
				switch value {
				case "fakeip":
					node.DNSMode = models.DNSModeFakeIP
				case "tun":
					node.DNSMode = models.DNSModeTUN
				}
			case "rules":
				// 解析规则
				ruleStrs := strings.Split(value, "|")
				for _, rs := range ruleStrs {
					parts := strings.SplitN(rs, ",", 2)
					if len(parts) == 2 {
						rule := models.RoutingRule{
							ID:     models.GenerateUUID(),
							Target: parts[1],
						}
						left := parts[0]
						switch {
						case strings.HasPrefix(left, "domain:"):
							rule.Type = "domain:"
							rule.Match = strings.TrimPrefix(left, "domain:")
						case strings.HasPrefix(left, "regexp:"):
							rule.Type = "regexp:"
							rule.Match = strings.TrimPrefix(left, "regexp:")
						case strings.HasPrefix(left, "geosite:"):
							rule.Type = "geosite:"
							rule.Match = strings.TrimPrefix(left, "geosite:")
						case strings.HasPrefix(left, "geoip:"):
							rule.Type = "geoip:"
							rule.Match = strings.TrimPrefix(left, "geoip:")
						default:
							rule.Type = ""
							rule.Match = left
						}
						node.Rules = append(node.Rules, rule)
					}
				}
			}
		}
	}

	// 解析token和server（token@server格式）
	if idx := strings.LastIndex(uri, "@"); idx != -1 {
		node.Token = uri[:idx]
		node.Server = uri[idx+1:]
	} else {
		node.Server = uri
	}

	// 如果没有名称，使用服务器地址
	if node.Name == "" {
		node.Name = node.Server
	}

	node.ID = models.GenerateUUID()

	return &node, nil
}
