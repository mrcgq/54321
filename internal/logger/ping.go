package logger

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"xlink-wails/internal/models"
)

// =============================================================================
// Ping 测试管理器
// =============================================================================

// PingManager Ping测试管理器
type PingManager struct {
	exeDir string
	logger *Manager

	// 当前运行的测试
	mu         sync.Mutex
	activePing *PingSession
}

// PingSession 单次Ping测试会话
type PingSession struct {
	NodeID    string
	NodeName  string
	StartTime time.Time
	Cancel    context.CancelFunc
	Results   []models.PingResult
	Done      chan struct{}
}

// PingReport 测试报告
type PingReport struct {
	NodeID      string              `json:"node_id"`
	NodeName    string              `json:"node_name"`
	StartTime   time.Time           `json:"start_time"`
	EndTime     time.Time           `json:"end_time"`
	Duration    time.Duration       `json:"duration"`
	TotalCount  int                 `json:"total_count"`
	SuccessCount int                `json:"success_count"`
	FailCount   int                 `json:"fail_count"`
	AvgLatency  int                 `json:"avg_latency"`
	MinLatency  int                 `json:"min_latency"`
	MaxLatency  int                 `json:"max_latency"`
	Results     []models.PingResult `json:"results"`
}

// NewPingManager 创建Ping管理器
func NewPingManager(exeDir string, logger *Manager) *PingManager {
	return &PingManager{
		exeDir: exeDir,
		logger: logger,
	}
}

// =============================================================================
// Ping 测试执行
// =============================================================================

// StartPing 启动Ping测试
func (pm *PingManager) StartPing(
	node *models.NodeConfig,
	onResult func(models.PingResult),
	onComplete func(PingReport),
) error {
	pm.mu.Lock()

	// 如果有正在运行的测试，取消它
	if pm.activePing != nil {
		pm.activePing.Cancel()
		pm.activePing = nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

	session := &PingSession{
		NodeID:    node.ID,
		NodeName:  node.Name,
		StartTime: time.Now(),
		Cancel:    cancel,
		Results:   make([]models.PingResult, 0),
		Done:      make(chan struct{}),
	}

	pm.activePing = session
	pm.mu.Unlock()

	// 异步执行测试
	go pm.runPing(ctx, session, node, onResult, onComplete)

	return nil
}

// StopPing 停止当前Ping测试
func (pm *PingManager) StopPing() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.activePing != nil {
		pm.activePing.Cancel()
		pm.activePing = nil
	}
}

// runPing 执行Ping测试
func (pm *PingManager) runPing(
	ctx context.Context,
	session *PingSession,
	node *models.NodeConfig,
	onResult func(models.PingResult),
	onComplete func(PingReport),
) {
	defer close(session.Done)
	defer func() {
		pm.mu.Lock()
		if pm.activePing == session {
			pm.activePing = nil
		}
		pm.mu.Unlock()
	}()

	// 日志
	pm.logger.LogNode(node.ID, node.Name, LevelInfo, CategoryPing, "开始延迟测试...")

	// 构建命令
	xlinkPath := filepath.Join(pm.exeDir, "xlink-cli-binary.exe")

	// 准备服务器列表
	servers := strings.ReplaceAll(node.Server, "\r\n", ";")
	servers = strings.ReplaceAll(servers, "\n", ";")

	args := []string{
		"--ping",
		"--server=" + servers,
		"--key=" + node.SecretKey,
	}

	if node.IP != "" {
		args = append(args, "--ip="+node.IP)
	}

	cmd := exec.CommandContext(ctx, xlinkPath, args...)
	cmd.Dir = pm.exeDir

	// 隐藏窗口
	hideWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		pm.logger.LogNode(node.ID, node.Name, LevelError, CategoryPing, fmt.Sprintf("创建管道失败: %v", err))
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		pm.logger.LogNode(node.ID, node.Name, LevelError, CategoryPing, fmt.Sprintf("创建管道失败: %v", err))
		return
	}

	if err := cmd.Start(); err != nil {
		pm.logger.LogNode(node.ID, node.Name, LevelError, CategoryPing, fmt.Sprintf("启动测速失败: %v", err))
		return
	}

	// 读取输出
	resultChan := make(chan models.PingResult, 100)
	var wg sync.WaitGroup

	wg.Add(2)
	go pm.readPingOutput(stdout, resultChan, &wg)
	go pm.readPingOutput(stderr, resultChan, &wg)

	// 收集结果
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for result := range resultChan {
		session.Results = append(session.Results, result)

		// 记录日志
		if result.Latency >= 0 {
			pm.logger.LogNode(node.ID, node.Name, LevelInfo, CategoryPing,
				fmt.Sprintf("%s - 延迟: %dms", result.Server, result.Latency))
		} else {
			pm.logger.LogNode(node.ID, node.Name, LevelWarn, CategoryPing,
				fmt.Sprintf("%s - 失败: %s", result.Server, result.Error))
		}

		// 回调
		if onResult != nil {
			onResult(result)
		}
	}

	// 等待进程结束
	cmd.Wait()

	// 生成报告
	report := pm.generateReport(session)

	// 记录报告
	pm.logReport(node.ID, node.Name, report)

	// 回调
	if onComplete != nil {
		onComplete(report)
	}
}

// readPingOutput 读取Ping输出
func (pm *PingManager) readPingOutput(reader io.Reader, results chan<- models.PingResult, wg *sync.WaitGroup) {
	defer wg.Done()

	scanner := bufio.NewScanner(reader)
	// 匹配延迟结果的正则
	delayPattern := regexp.MustCompile(`^(.+?)\s*\|\s*Delay:\s*(\d+)ms`)
	errorPattern := regexp.MustCompile(`^(.+?)\s*\|\s*Error:\s*(.+)`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		result := models.PingResult{
			Latency: -1,
		}

		// 尝试匹配延迟
		if matches := delayPattern.FindStringSubmatch(line); len(matches) >= 3 {
			result.Server = strings.TrimSpace(matches[1])
			if latency, err := strconv.Atoi(matches[2]); err == nil {
				result.Latency = latency
			}
			results <- result
			continue
		}

		// 尝试匹配错误
		if matches := errorPattern.FindStringSubmatch(line); len(matches) >= 3 {
			result.Server = strings.TrimSpace(matches[1])
			result.Error = strings.TrimSpace(matches[2])
			results <- result
			continue
		}

		// 尝试简单格式
		if strings.Contains(line, "|") {
			parts := strings.SplitN(line, "|", 2)
			if len(parts) == 2 {
				result.Server = strings.TrimSpace(parts[0])
				info := strings.TrimSpace(parts[1])

				if strings.HasPrefix(info, "Delay:") {
					delayStr := strings.TrimSpace(strings.TrimPrefix(info, "Delay:"))
					delayStr = strings.TrimSuffix(delayStr, "ms")
					if latency, err := strconv.Atoi(delayStr); err == nil {
						result.Latency = latency
					}
				} else if strings.HasPrefix(info, "Error:") {
					result.Error = strings.TrimSpace(strings.TrimPrefix(info, "Error:"))
				}

				if result.Server != "" {
					results <- result
				}
			}
		}
	}
}

// generateReport 生成测试报告
func (pm *PingManager) generateReport(session *PingSession) PingReport {
	report := PingReport{
		NodeID:     session.NodeID,
		NodeName:   session.NodeName,
		StartTime:  session.StartTime,
		EndTime:    time.Now(),
		TotalCount: len(session.Results),
		Results:    session.Results,
		MinLatency: -1,
		MaxLatency: -1,
	}

	report.Duration = report.EndTime.Sub(report.StartTime)

	var totalLatency int64

	for _, r := range session.Results {
		if r.Latency >= 0 {
			report.SuccessCount++
			totalLatency += int64(r.Latency)

			if report.MinLatency < 0 || r.Latency < report.MinLatency {
				report.MinLatency = r.Latency
			}
			if r.Latency > report.MaxLatency {
				report.MaxLatency = r.Latency
			}
		} else {
			report.FailCount++
		}
	}

	if report.SuccessCount > 0 {
		report.AvgLatency = int(totalLatency / int64(report.SuccessCount))
	}

	// 按延迟排序结果
	sort.Slice(report.Results, func(i, j int) bool {
		// 失败的排在后面
		if report.Results[i].Latency < 0 {
			return false
		}
		if report.Results[j].Latency < 0 {
			return true
		}
		return report.Results[i].Latency < report.Results[j].Latency
	})

	return report
}

// logReport 记录报告
func (pm *PingManager) logReport(nodeID, nodeName string, report PingReport) {
	pm.logger.LogNode(nodeID, nodeName, LevelInfo, CategoryPing, "--- 测试完成 ---")
	pm.logger.LogNode(nodeID, nodeName, LevelInfo, CategoryPing,
		fmt.Sprintf("总计: %d | 成功: %d | 失败: %d",
			report.TotalCount, report.SuccessCount, report.FailCount))

	if report.SuccessCount > 0 {
		pm.logger.LogNode(nodeID, nodeName, LevelInfo, CategoryPing,
			fmt.Sprintf("延迟: 平均 %dms | 最小 %dms | 最大 %dms",
				report.AvgLatency, report.MinLatency, report.MaxLatency))
	}

	// 记录最快的3个节点
	pm.logger.LogNode(nodeID, nodeName, LevelInfo, CategoryPing, "最快节点:")
	count := 0
	for _, r := range report.Results {
		if r.Latency >= 0 && count < 3 {
			pm.logger.LogNode(nodeID, nodeName, LevelInfo, CategoryPing,
				fmt.Sprintf("  #%d %s (%dms)", count+1, r.Server, r.Latency))
			count++
		}
	}
}

// =============================================================================
// 批量Ping（测试多个节点）
// =============================================================================

// BatchPingResult 批量测试结果
type BatchPingResult struct {
	NodeID   string      `json:"node_id"`
	NodeName string      `json:"node_name"`
	Report   *PingReport `json:"report"`
	Error    string      `json:"error,omitempty"`
}

// BatchPing 批量测试多个节点
func (pm *PingManager) BatchPing(
	nodes []*models.NodeConfig,
	onProgress func(current, total int, result BatchPingResult),
) []BatchPingResult {
	results := make([]BatchPingResult, 0, len(nodes))
	total := len(nodes)

	for i, node := range nodes {
		result := BatchPingResult{
			NodeID:   node.ID,
			NodeName: node.Name,
		}

		// 创建等待通道
		done := make(chan PingReport, 1)

		err := pm.StartPing(node, nil, func(report PingReport) {
			done <- report
		})

		if err != nil {
			result.Error = err.Error()
		} else {
			// 等待完成（带超时）
			select {
			case report := <-done:
				result.Report = &report
			case <-time.After(30 * time.Second):
				result.Error = "测试超时"
				pm.StopPing()
			}
		}

		results = append(results, result)

		if onProgress != nil {
			onProgress(i+1, total, result)
		}
	}

	return results
}

// =============================================================================
// 平台特定函数
// =============================================================================

// hideWindow 在不同平台隐藏窗口（占位，实际在 ping_windows.go 中实现）


// func hideWindow(cmd *exec.Cmd) {
// 	// 默认空实现，Windows平台会覆盖
// }
