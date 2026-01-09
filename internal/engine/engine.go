// Package engine 管理xlink内核和xray进程的生命周期
package engine

import (
	"bufio"
	"context"
	"fmt"
	"io"

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
	StartTimeout    = 10 * time.Second
	StopTimeout     = 2 * time.Second
	HealthCheckInterval = 5 * time.Second
)

// =============================================================================
// 进程状态
// =============================================================================

type ProcessInfo struct {
	Cmd        *exec.Cmd
	Pid        int
	StartTime  time.Time
	StdoutPipe io.ReadCloser
	StderrPipe io.ReadCloser
	Cancel     context.CancelFunc
}

type EngineInstance struct {
	mu sync.RWMutex
	NodeID   string
	NodeName string
	Status   string
	XlinkProcess *ProcessInfo
	XrayProcess *ProcessInfo
	InternalPort int
	LogCallback func(level, category, message string)
	StatusCallback func(status string, err error)
}

// =============================================================================
// 引擎管理器
// =============================================================================

type Manager struct {
	mu        sync.RWMutex
	exeDir    string
	instances map[string]*EngineInstance
	globalLogCallback func(nodeID, nodeName, level, category, message string)
	globalStatusCallback func(nodeID, status string, err error)
}

func NewManager(exeDir string) *Manager {
	return &Manager{
		exeDir:    exeDir,
		instances: make(map[string]*EngineInstance),
	}
}

func (m *Manager) SetLogCallback(cb func(nodeID, nodeName, level, category, message string)) {
	m.globalLogCallback = cb
}

func (m *Manager) SetStatusCallback(cb func(nodeID, status string, err error)) {
	m.globalStatusCallback = cb
}

// =============================================================================
// 启动引擎
// =============================================================================

func (m *Manager) StartNode(node *models.NodeConfig, configPath string) error {
	m.mu.Lock()
	if inst, exists := m.instances[node.ID]; exists {
		if inst.Status == models.StatusRunning {
			m.mu.Unlock()
			m.stopInstanceLocked(node.ID)
			m.mu.Lock()
		} else {
			m.stopInstanceLocked(node.ID)
		}
	}

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

	instance.StatusCallback(models.StatusStarting, nil)

	if err := m.startXlinkProcess(instance, node, configPath); err != nil {
		m.cleanupInstance(instance, err)
		return err
	}

	if node.RoutingMode == models.RoutingModeSmart {
		xrayConfigPath := strings.Replace(configPath, "config_core_", "config_xray_", 1)
		if err := m.startXrayProcess(instance, xrayConfigPath); err != nil {
			m.stopXlinkProcess(instance)
			m.cleanupInstance(instance, err)
			return err
		}
	}

	instance.mu.Lock()
	instance.Status = models.StatusRunning
	instance.mu.Unlock()
	instance.StatusCallback(models.StatusRunning, nil)

	go m.healthCheckLoop(instance)

	return nil
}

func (m *Manager) cleanupInstance(inst *EngineInstance, err error) {
	inst.mu.Lock()
	inst.Status = models.StatusError
	inst.mu.Unlock()
	inst.StatusCallback(models.StatusError, err)
	
	m.mu.Lock()
	delete(m.instances, inst.NodeID)
	m.mu.Unlock()
}

func (m *Manager) startXlinkProcess(inst *EngineInstance, node *models.NodeConfig, configPath string) error {
	xlinkPath := filepath.Join(m.exeDir, XlinkBinaryName)
	if _, err := os.Stat(xlinkPath); os.IsNotExist(err) {
		return fmt.Errorf("核心文件不存在: %s", XlinkBinaryName)
	}

	absConfigPath, _ := filepath.Abs(configPath)
	args := []string{"-c", absConfigPath}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, xlinkPath, args...)
	cmd.Dir = m.exeDir
	m.hideWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil { cancel(); return err }
	stderr, err := cmd.StderrPipe()
	if err != nil { cancel(); return err }

	if err := cmd.Start(); err != nil { cancel(); return err }

	inst.mu.Lock()
	inst.XlinkProcess = &ProcessInfo{
		Cmd: cmd, Pid: cmd.Process.Pid, StartTime: time.Now(),
		StdoutPipe: stdout, StderrPipe: stderr, Cancel: cancel,
	}
	inst.mu.Unlock()

	go m.readProcessOutput(inst, "xlink", stdout)
	go m.readProcessOutput(inst, "xlink", stderr)
	go m.waitProcess(inst, "xlink", cmd)

	inst.LogCallback("info", "系统", fmt.Sprintf("Xlink核心已启动 (PID: %d)", cmd.Process.Pid))
	return nil
}

func (m *Manager) startXrayProcess(inst *EngineInstance, configPath string) error {
	xrayPath := filepath.Join(m.exeDir, XrayBinaryName)
	if _, err := os.Stat(xrayPath); os.IsNotExist(err) {
		return fmt.Errorf("Xray文件不存在: %s", XrayBinaryName)
	}

	absConfigPath, _ := filepath.Abs(configPath)
	args := []string{"run", "-c", absConfigPath}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, xrayPath, args...)
	cmd.Dir = m.exeDir
	m.hideWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil { cancel(); return err }
	stderr, err := cmd.StderrPipe()
	if err != nil { cancel(); return err }

	if err := cmd.Start(); err != nil { cancel(); return err }

	inst.mu.Lock()
	inst.XrayProcess = &ProcessInfo{
		Cmd: cmd, Pid: cmd.Process.Pid, StartTime: time.Now(),
		StdoutPipe: stdout, StderrPipe: stderr, Cancel: cancel,
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

func (m *Manager) StopNode(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopInstanceLocked(nodeID)
}

func (m *Manager) stopInstanceLocked(nodeID string) error {
	inst, exists := m.instances[nodeID]
	if !exists { return nil }

	inst.mu.Lock()
	inst.Status = models.StatusStopped
	
	if inst.XrayProcess != nil {
		m.terminateProcess(inst.XrayProcess)
		inst.XrayProcess = nil
	}
	if inst.XlinkProcess != nil {
		m.terminateProcess(inst.XlinkProcess)
		inst.XlinkProcess = nil
	}
	inst.mu.Unlock()

	if inst.StatusCallback != nil {
		go inst.StatusCallback(models.StatusStopped, nil)
	}
	if inst.LogCallback != nil {
		go inst.LogCallback("info", "系统", "节点已停止")
	}

	delete(m.instances, nodeID)
	return nil
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for nodeID := range m.instances {
		m.stopInstanceLocked(nodeID)
	}
}

func (m *Manager) terminateProcess(proc *ProcessInfo) {
	if proc == nil || proc.Cmd == nil || proc.Cmd.Process == nil {
		return
	}
	if proc.Cancel != nil { proc.Cancel() }
	if proc.StdoutPipe != nil { proc.StdoutPipe.Close() }
	if proc.StderrPipe != nil { proc.StderrPipe.Close() }

	// 强制杀进程树
	if err := m.killProcessTree(proc.Pid); err != nil {
		proc.Cmd.Process.Kill()
	}
	proc.Cmd.Wait()
}

// =============================================================================
// 日志与监控
// =============================================================================

func (m *Manager) readProcessOutput(inst *EngineInstance, source string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		m.parseAndForwardLog(inst, source, scanner.Text())
	}
}

func (m *Manager) parseAndForwardLog(inst *EngineInstance, source, line string) {
	if inst.LogCallback == nil || line == "" { return }
	
	level := "info"
	category := "内核"
	message := line
	lowerLine := strings.ToLower(line)

	if strings.Contains(lowerLine, "error") || strings.Contains(lowerLine, "[err]") {
		level = "error"
	} else if strings.Contains(lowerLine, "warn") {
		level = "warn"
	}

	if strings.Contains(line, "Tunnel ->") {
		category = "隧道"
		message = m.parseTunnelLog(line)
	} else if strings.Contains(line, "Rule Hit") {
		category = "规则"
		message = m.parseRuleHitLog(line)
	} else if strings.Contains(line, "LB ->") {
		category = "负载"
		message = m.parseLBLog(line)
	} else if strings.Contains(line, "[Stats]") {
		category = "统计"
		message = m.parseStatsLog(line)
	} else if source == "xray" {
		category = "Xray"
	}

	message = strings.TrimPrefix(message, "[CLI] ")
	message = strings.TrimPrefix(message, "[Core] ")
	inst.LogCallback(level, category, message)
}

func (m *Manager) parseTunnelLog(line string) string {
	if idx := strings.Index(line, "Tunnel ->"); idx != -1 { return line[idx:] }
	return line
}
func (m *Manager) parseRuleHitLog(line string) string {
	if idx := strings.Index(line, "Rule Hit"); idx != -1 { return line[idx:] }
	return line
}
func (m *Manager) parseLBLog(line string) string {
	if idx := strings.Index(line, "LB ->"); idx != -1 { return line[idx:] }
	return line
}
func (m *Manager) parseStatsLog(line string) string {
	if idx := strings.Index(line, "[Stats]"); idx != -1 { return line[idx:] }
	return line
}

func (m *Manager) waitProcess(inst *EngineInstance, source string, cmd *exec.Cmd) {
	err := cmd.Wait()
	inst.mu.Lock()
	status := inst.Status
	inst.mu.Unlock()

	if status == models.StatusRunning {
		errMsg := fmt.Sprintf("%s 进程意外退出", source)
		if err != nil { errMsg += fmt.Sprintf(": %v", err) }
		
		inst.mu.Lock()
		inst.Status = models.StatusError
		inst.mu.Unlock()
		
		if inst.LogCallback != nil { inst.LogCallback("error", "系统", errMsg) }
		if inst.StatusCallback != nil { inst.StatusCallback(models.StatusError, fmt.Errorf(errMsg)) }
	}
}

func (m *Manager) healthCheckLoop(inst *EngineInstance) {
	ticker := time.NewTicker(HealthCheckInterval)
	defer ticker.Stop()
	for {
		<-ticker.C
		inst.mu.RLock()
		status := inst.Status
		xlinkProc := inst.XlinkProcess
		inst.mu.RUnlock()

		if status != models.StatusRunning { return }
		if xlinkProc != nil && xlinkProc.Cmd != nil && xlinkProc.Cmd.Process != nil {
			if err := xlinkProc.Cmd.Process.Signal(os.Signal(nil)); err != nil {
				if inst.LogCallback != nil { inst.LogCallback("error", "系统", "检测到核心进程已消失") }
				m.stopInstanceLocked(inst.NodeID)
				return
			}
		}
	}
}

// =============================================================================
// Ping测试
// =============================================================================

func (m *Manager) PingTest(node *models.NodeConfig, callback func(result models.PingResult)) error {
	xlinkPath := filepath.Join(m.exeDir, XlinkBinaryName)
	if _, err := os.Stat(xlinkPath); os.IsNotExist(err) {
		return fmt.Errorf("核心文件不存在")
	}

	servers := strings.ReplaceAll(node.Server, "\r\n", ";")
	servers = strings.ReplaceAll(servers, "\n", ";")

	// ⚠️【修复】优先使用 Token 字段，与 generator 逻辑保持一致
	mainToken := node.Token
	if mainToken == "" {
		mainToken = node.SecretKey
	}

	args := []string{
		"--ping",
		"--server=" + servers,
		"--key=" + mainToken,
	}
	if node.IP != "" {
		args = append(args, "--ip="+node.IP)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, xlinkPath, args...)
	cmd.Dir = m.exeDir
	m.hideWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil { return err }
	
	if err := cmd.Start(); err != nil { return err }

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "Delay:") {
				parts := strings.Split(line, "|")
				if len(parts) >= 2 {
					server := strings.TrimSpace(parts[0])
					delayStr := strings.TrimPrefix(strings.TrimSpace(parts[1]), "Delay:")
					delayStr = strings.TrimSuffix(strings.TrimSpace(delayStr), "ms")
					var delay int
					fmt.Sscanf(delayStr, "%d", &delay)
					callback(models.PingResult{Server: server, Latency: delay})
				}
			}
		}
	}()

	// ⚠️【修复】确保 Ping 进程也被彻底清理
	err = cmd.Wait()
	// 使用 killProcessTree 兜底清理（以防万一）
	if cmd.Process != nil {
		m.killProcessTree(cmd.Process.Pid)
	}
	return err
}

// =============================================================================
// 工具函数
// =============================================================================

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

func (m *Manager) GetAllStatuses() map[string]models.EngineStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	statuses := make(map[string]models.EngineStatus)
	for nodeID, inst := range m.instances {
		inst.mu.RLock()
		status := models.EngineStatus{
			NodeID: nodeID,
			Status: inst.Status,
		}
		if inst.XlinkProcess != nil {
			status.PID = inst.XlinkProcess.Pid
			status.StartTime = inst.XlinkProcess.StartTime
		}
		inst.mu.RUnlock()
		statuses[nodeID] = status
	}
	return statuses
}

func (m *Manager) FindFreePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { return 0 }
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func (m *Manager) GetExeDir() string { return m.exeDir }

func (m *Manager) stopXlinkProcess(inst *EngineInstance) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.XlinkProcess != nil {
		m.terminateProcess(inst.XlinkProcess)
		inst.XlinkProcess = nil
	}
}
