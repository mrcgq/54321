// Package engine 管理xlink内核和xray进程的生命周期
package engine

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
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
	XlinkBinaryName = "xlink-cli-binary.exe"
	XrayBinaryName  = "xray.exe"

	// 进程启动超时
	StartTimeout = 10 * time.Second

	// 进程停止超时
	StopTimeout = 5 * time.Second

	// 健康检查间隔
	HealthCheckInterval = 5 * time.Second
)

// =============================================================================
// 进程状态
// =============================================================================

// ProcessInfo 进程信息
type ProcessInfo struct {
	Cmd        *exec.Cmd
	Pid        int
	StartTime  time.Time
	StdoutPipe io.ReadCloser
	StderrPipe io.ReadCloser
	Cancel     context.CancelFunc
}

// EngineInstance 单个引擎实例
type EngineInstance struct {
	mu sync.RWMutex

	NodeID   string
	NodeName string
	Status   string

	// Xlink 核心进程
	XlinkProcess *ProcessInfo

	// Xray 前端进程（智能分流模式）
	XrayProcess *ProcessInfo

	// 内部端口（智能分流时Xlink监听的端口）
	InternalPort int

	// 日志回调
	LogCallback func(level, category, message string)

	// 状态回调
	StatusCallback func(status string, err error)
}

// =============================================================================
// 引擎管理器
// =============================================================================

// Manager 引擎管理器
type Manager struct {
	mu        sync.RWMutex
	exeDir    string
	instances map[string]*EngineInstance // key: NodeID

	// 全局日志回调
	globalLogCallback func(nodeID, nodeName, level, category, message string)

	// 全局状态回调
	globalStatusCallback func(nodeID, status string, err error)
}

// NewManager 创建引擎管理器
func NewManager(exeDir string) *Manager {
	return &Manager{
		exeDir:    exeDir,
		instances: make(map[string]*EngineInstance),
	}
}

// SetLogCallback 设置全局日志回调
func (m *Manager) SetLogCallback(cb func(nodeID, nodeName, level, category, message string)) {
	m.globalLogCallback = cb
}

// SetStatusCallback 设置全局状态回调
func (m *Manager) SetStatusCallback(cb func(nodeID, status string, err error)) {
	m.globalStatusCallback = cb
}

// =============================================================================
// 启动引擎
// =============================================================================

// StartNode 启动节点引擎
func (m *Manager) StartNode(node *models.NodeConfig, configPath string) error {
	m.mu.Lock()

	// 检查是否已运行
	if inst, exists := m.instances[node.ID]; exists {
		if inst.Status == models.StatusRunning {
			m.mu.Unlock()
			return fmt.Errorf("节点已在运行中")
		}
		// 清理旧实例
		m.stopInstanceLocked(node.ID)
	}

	// 创建新实例
	instance := &EngineInstance{
		NodeID:   node.ID,
		NodeName: node.Name,
		Status:   models.StatusStarting,
		LogCallback: func(level, category, message string) {
			if m.globalLogCallback != nil {
				m.globalLogCallback(node.ID, node.Name, level, category, message)
			}
		},
		StatusCallback: func(status string, err error) {
			if m.globalStatusCallback != nil {
				m.globalStatusCallback(node.ID, status, err)
			}
		},
	}

	m.instances[node.ID] = instance
	m.mu.Unlock()

	// 通知状态变更
	instance.StatusCallback(models.StatusStarting, nil)

	// 启动Xlink核心
	if err := m.startXlinkProcess(instance, node, configPath); err != nil {
		instance.mu.Lock()
		instance.Status = models.StatusError
		instance.mu.Unlock()
		instance.StatusCallback(models.StatusError, err)
		return err
	}

	// 如果是智能分流模式，启动Xray
	if node.RoutingMode == models.RoutingModeSmart {
		xrayConfigPath := strings.Replace(configPath, "config_core_", "config_xray_", 1)
		if err := m.startXrayProcess(instance, xrayConfigPath); err != nil {
			// 停止已启动的Xlink
			m.stopXlinkProcess(instance)
			instance.mu.Lock()
			instance.Status = models.StatusError
			instance.mu.Unlock()
			instance.StatusCallback(models.StatusError, err)
			return err
		}
	}

	// 更新状态为运行中
	instance.mu.Lock()
	instance.Status = models.StatusRunning
	instance.mu.Unlock()
	instance.StatusCallback(models.StatusRunning, nil)

	// 启动健康检查
	go m.healthCheckLoop(instance)

	return nil
}

// startXlinkProcess 启动Xlink核心进程
func (m *Manager) startXlinkProcess(inst *EngineInstance, node *models.NodeConfig, configPath string) error {
	xlinkPath := filepath.Join(m.exeDir, XlinkBinaryName)

	// 检查可执行文件
	if _, err := os.Stat(xlinkPath); os.IsNotExist(err) {
		return fmt.Errorf("核心文件不存在: %s", XlinkBinaryName)
	}

	// 构建命令行参数
	args := []string{"-c", configPath}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())

	// 创建进程
	cmd := exec.CommandContext(ctx, xlinkPath, args...)
	cmd.Dir = m.exeDir

	// 隐藏窗口（Windows）
	m.hideWindow(cmd)

	// 创建管道
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("创建stdout管道失败: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("创建stderr管道失败: %w", err)
	}

	// 启动进程
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("启动Xlink进程失败: %w", err)
	}

	inst.mu.Lock()
	inst.XlinkProcess = &ProcessInfo{
		Cmd:        cmd,
		Pid:        cmd.Process.Pid,
		StartTime:  time.Now(),
		StdoutPipe: stdout,
		StderrPipe: stderr,
		Cancel:     cancel,
	}
	inst.mu.Unlock()

	// 启动日志读取
	go m.readProcessOutput(inst, "xlink", stdout)
	go m.readProcessOutput(inst, "xlink", stderr)

	// 等待进程退出
	go m.waitProcess(inst, "xlink", cmd)

	inst.LogCallback("info", "系统", fmt.Sprintf("Xlink核心已启动 (PID: %d)", cmd.Process.Pid))

	return nil
}

// startXrayProcess 启动Xray进程
func (m *Manager) startXrayProcess(inst *EngineInstance, configPath string) error {
	xrayPath := filepath.Join(m.exeDir, XrayBinaryName)

	// 检查可执行文件
	if _, err := os.Stat(xrayPath); os.IsNotExist(err) {
		return fmt.Errorf("Xray文件不存在: %s（智能分流模式需要此文件）", XrayBinaryName)
	}

	// 检查配置文件
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("Xray配置文件不存在: %s", configPath)
	}

	// 构建命令行
	args := []string{"run", "-c", configPath}

	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, xrayPath, args...)
	cmd.Dir = m.exeDir

	m.hideWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("创建Xray stdout管道失败: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("创建Xray stderr管道失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("启动Xray进程失败: %w", err)
	}

	inst.mu.Lock()
	inst.XrayProcess = &ProcessInfo{
		Cmd:        cmd,
		Pid:        cmd.Process.Pid,
		StartTime:  time.Now(),
		StdoutPipe: stdout,
		StderrPipe: stderr,
		Cancel:     cancel,
	}
	inst.mu.Unlock()

	go m.readProcessOutput(inst, "xray", stdout)
	go m.readProcessOutput(inst, "xray", stderr)
	go m.waitProcess(inst, "xray", cmd)

	inst.LogCallback("info", "系统", fmt.Sprintf("Xray前端已启动 (PID: %d)", cmd.Process.Pid))

	return nil
}

// =============================================================================
// 停止引擎
// =============================================================================

// StopNode 停止节点引擎
func (m *Manager) StopNode(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.stopInstanceLocked(nodeID)
}

// stopInstanceLocked 停止实例（需要持有锁）
func (m *Manager) stopInstanceLocked(nodeID string) error {
	inst, exists := m.instances[nodeID]
	if !exists {
		return nil
	}

	inst.mu.Lock()
	defer inst.mu.Unlock()

	// 停止Xray
	if inst.XrayProcess != nil {
		m.terminateProcess(inst.XrayProcess)
		inst.XrayProcess = nil
	}

	// 停止Xlink
	if inst.XlinkProcess != nil {
		m.terminateProcess(inst.XlinkProcess)
		inst.XlinkProcess = nil
	}

	inst.Status = models.StatusStopped

	// 通知状态变更
	if inst.StatusCallback != nil {
		go inst.StatusCallback(models.StatusStopped, nil)
	}

	if inst.LogCallback != nil {
		go inst.LogCallback("info", "系统", "节点已停止")
	}

	delete(m.instances, nodeID)

	return nil
}

// StopAll 停止所有节点
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for nodeID := range m.instances {
		m.stopInstanceLocked(nodeID)
	}
}

// terminateProcess 终止进程
func (m *Manager) terminateProcess(proc *ProcessInfo) {
	if proc == nil || proc.Cmd == nil || proc.Cmd.Process == nil {
		return
	}

	// 取消上下文
	if proc.Cancel != nil {
		proc.Cancel()
	}

	// 关闭管道
	if proc.StdoutPipe != nil {
		proc.StdoutPipe.Close()
	}
	if proc.StderrPipe != nil {
		proc.StderrPipe.Close()
	}

	// 尝试优雅终止
	done := make(chan struct{})
	go func() {
		proc.Cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 进程已退出
	case <-time.After(StopTimeout):
		// 强制终止
		proc.Cmd.Process.Kill()
	}
}

// =============================================================================
// 日志读取
// =============================================================================

// readProcessOutput 读取进程输出
func (m *Manager) readProcessOutput(inst *EngineInstance, source string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	// 增大缓冲区
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// 解析并转发日志
		m.parseAndForwardLog(inst, source, line)
	}
}

// parseAndForwardLog 解析并转发日志
func (m *Manager) parseAndForwardLog(inst *EngineInstance, source, line string) {
	if inst.LogCallback == nil {
		return
	}

	level := "info"
	category := "内核"
	message := line

	// 解析日志级别和类别
	switch {
	case strings.Contains(line, "[ERR]") || strings.Contains(line, "error") || strings.Contains(line, "Error"):
		level = "error"
	case strings.Contains(line, "[WARN]") || strings.Contains(line, "warning"):
		level = "warn"
	case strings.Contains(line, "[DEBUG]"):
		level = "debug"
	}

	// 解析日志类别
	switch {
	case strings.Contains(line, "Tunnel ->"):
		category = "隧道"
		message = m.parseTunnelLog(line)
	case strings.Contains(line, "Rule Hit"):
		category = "规则"
		message = m.parseRuleHitLog(line)
	case strings.Contains(line, "LB ->"):
		category = "负载"
		message = m.parseLBLog(line)
	case strings.Contains(line, "[Stats]"):
		category = "统计"
		message = m.parseStatsLog(line)
	case strings.Contains(line, "Ping") || strings.Contains(line, "Delay"):
		category = "测速"
	case source == "xray":
		category = "Xray"
	}

	// 移除日志中的前缀标记
	message = strings.TrimPrefix(message, "[CLI] ")
	message = strings.TrimPrefix(message, "[Core] ")

	inst.LogCallback(level, category, message)
}

// parseTunnelLog 解析隧道日志
func (m *Manager) parseTunnelLog(line string) string {
	// Tunnel -> example.com (SNI) >>> real-server.com (Real) | Latency: 100ms
	var sni, real, latency string

	if idx := strings.Index(line, "Latency:"); idx != -1 {
		latency = strings.TrimSpace(line[idx+8:])
	}

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
		return fmt.Sprintf("隧道建立: %s ==> %s [延迟: %s]", sni, real, latency)
	}

	return line
}

// parseRuleHitLog 解析规则命中日志
func (m *Manager) parseRuleHitLog(line string) string {
	// Rule Hit -> target.com | SNI: proxy-node.com (Rule: keyword)
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
		return fmt.Sprintf("命中: %s -> %s (关键词: %s)", target, node, rule)
	}

	return line
}

// parseLBLog 解析负载均衡日志
func (m *Manager) parseLBLog(line string) string {
	// LB -> target.com | SNI: proxy.com | Algo: random
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
		return fmt.Sprintf("访问: %s -> %s (策略: %s)", target, node, algo)
	}

	return line
}

// parseStatsLog 解析统计日志
func (m *Manager) parseStatsLog(line string) string {
	// [Stats] target.com | Up: 1.2KB | Down: 5.6MB | Time: 10.5s
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
		return fmt.Sprintf("结束: %s (上行:%s / 下行:%s) 时长:%s", target, up, down, duration)
	}

	return line
}

// =============================================================================
// 进程监控
// =============================================================================

// waitProcess 等待进程退出
func (m *Manager) waitProcess(inst *EngineInstance, source string, cmd *exec.Cmd) {
	err := cmd.Wait()

	inst.mu.Lock()
	status := inst.Status
	inst.mu.Unlock()

	// 如果不是正常停止，报告错误
	if status == models.StatusRunning {
		errMsg := "进程意外退出"
		if err != nil {
			errMsg = fmt.Sprintf("%s进程异常退出: %v", source, err)
		}

		inst.mu.Lock()
		inst.Status = models.StatusError
		inst.mu.Unlock()

		if inst.LogCallback != nil {
			inst.LogCallback("error", "系统", errMsg)
		}
		if inst.StatusCallback != nil {
			inst.StatusCallback(models.StatusError, fmt.Errorf(errMsg))
		}
	}
}

// healthCheckLoop 健康检查循环
func (m *Manager) healthCheckLoop(inst *EngineInstance) {
	ticker := time.NewTicker(HealthCheckInterval)
	defer ticker.Stop()

	for {
		<-ticker.C

		inst.mu.RLock()
		status := inst.Status
		xlinkProc := inst.XlinkProcess
		inst.mu.RUnlock()

		// 如果已停止，退出检查
		if status != models.StatusRunning {
			return
		}

		// 检查Xlink进程
		if xlinkProc != nil && xlinkProc.Cmd != nil && xlinkProc.Cmd.Process != nil {
			// 尝试发送信号0检查进程是否存在
			if err := xlinkProc.Cmd.Process.Signal(os.Signal(nil)); err != nil {
				// 进程已退出
				if inst.LogCallback != nil {
					inst.LogCallback("error", "系统", "检测到进程已退出，正在重启...")
				}

				// 可以在这里实现自动重启逻辑
				// m.RestartNode(inst.NodeID)
				return
			}
		}
	}
}

// =============================================================================
// Ping测试
// =============================================================================

// PingTest 执行延迟测试
func (m *Manager) PingTest(node *models.NodeConfig, callback func(result models.PingResult)) error {
	xlinkPath := filepath.Join(m.exeDir, XlinkBinaryName)

	if _, err := os.Stat(xlinkPath); os.IsNotExist(err) {
		return fmt.Errorf("核心文件不存在: %s", XlinkBinaryName)
	}

	// 准备服务器列表
	servers := strings.ReplaceAll(node.Server, "\r\n", ";")
	servers = strings.ReplaceAll(servers, "\n", ";")

	// 构建命令
	args := []string{
		"--ping",
		"--server=" + servers,
		"--key=" + node.SecretKey,
	}

	if node.IP != "" {
		args = append(args, "--ip="+node.IP)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, xlinkPath, args...)
	cmd.Dir = m.exeDir

	m.hideWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动测速进程失败: %w", err)
	}

	// 读取输出
	go func() {
		scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
		for scanner.Scan() {
			line := scanner.Text()
			result := m.parsePingResult(line)
			if result != nil {
				callback(*result)
			}
		}
	}()

	// 等待完成
	return cmd.Wait()
}

// parsePingResult 解析Ping测试结果
func (m *Manager) parsePingResult(line string) *models.PingResult {
	line = strings.TrimSpace(line)

	// 格式: server.com:443 | Delay: 100ms
	// 或: server.com:443 | Error: connection refused
	if !strings.Contains(line, "|") {
		return nil
	}

	parts := strings.SplitN(line, "|", 2)
	if len(parts) != 2 {
		return nil
	}

	server := strings.TrimSpace(parts[0])
	info := strings.TrimSpace(parts[1])

	result := &models.PingResult{
		Server:  server,
		Latency: -1,
	}

	if strings.HasPrefix(info, "Delay:") {
		delayStr := strings.TrimSpace(strings.TrimPrefix(info, "Delay:"))
		delayStr = strings.TrimSuffix(delayStr, "ms")
		var delay int
		if _, err := fmt.Sscanf(delayStr, "%d", &delay); err == nil {
			result.Latency = delay
		}
	} else if strings.HasPrefix(info, "Error:") {
		result.Error = strings.TrimSpace(strings.TrimPrefix(info, "Error:"))
	}

	return result
}

// =============================================================================
// 工具函数
// =============================================================================

// GetStatus 获取节点状态
func (m *Manager) GetStatus(nodeID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if inst, exists := m.instances[nodeID]; exists {
		inst.mu.RLock()
		defer inst.mu.RUnlock()
		return inst.Status
	}

	return models.StatusStopped
}

// GetAllStatuses 获取所有节点状态
func (m *Manager) GetAllStatuses() map[string]models.EngineStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make(map[string]models.EngineStatus)

	for nodeID, inst := range m.instances {
		inst.mu.RLock()
		status := models.EngineStatus{
			NodeID:    nodeID,
			Status:    inst.Status,
			StartTime: time.Time{},
		}
		if inst.XlinkProcess != nil {
			status.PID = inst.XlinkProcess.Pid
			status.StartTime = inst.XlinkProcess.StartTime
		}
		if inst.XrayProcess != nil {
			status.XrayPID = inst.XrayProcess.Pid
		}
		inst.mu.RUnlock()

		statuses[nodeID] = status
	}

	return statuses
}

// IsRunning 检查节点是否运行中
func (m *Manager) IsRunning(nodeID string) bool {
	return m.GetStatus(nodeID) == models.StatusRunning
}

// FindFreePort 查找空闲端口
func (m *Manager) FindFreePort() int {
	// 尝试随机端口
	rand.Seed(time.Now().UnixNano())

	for i := 0; i < 100; i++ {
		port := 49152 + rand.Intn(16383)

		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port
		}
	}

	// 让系统分配
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 49152 + rand.Intn(16383)
	}
	defer ln.Close()

	return ln.Addr().(*net.TCPAddr).Port
}

// GetExeDir 获取可执行文件目录
func (m *Manager) GetExeDir() string {
	return m.exeDir

// stopXlinkProcess 停止 Xlink 进程辅助方法
func (m *Manager) stopXlinkProcess(inst *EngineInstance) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.XlinkProcess != nil {
		m.terminateProcess(inst.XlinkProcess)
		inst.XlinkProcess = nil
	}
}
}
