package hids

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ProcessEventType 进程事件类型
type ProcessEventType string

const (
	ProcessEventCreate ProcessEventType = "create" // 进程创建
	ProcessEventExit   ProcessEventType = "exit"   // 进程退出
)

// ProcessEvent 进程事件
type ProcessEvent struct {
	Type      ProcessEventType `json:"type"`
	Process   *ProcessInfo     `json:"process"`
	Timestamp int64            `json:"timestamp"`
}

// ProcessWhitelistRule 进程白名单规则
type ProcessWhitelistRule struct {
	Name        string `json:"name"`         // 进程名（精确匹配）
	NamePattern string `json:"name_pattern"` // 进程名正则
	ExePath     string `json:"exe_path"`     // 可执行文件路径（精确匹配）
	ExePattern  string `json:"exe_pattern"`  // 可执行文件路径正则
	ExeHash     string `json:"exe_hash"`     // 可执行文件哈希（MD5或SHA256）
	Username    string `json:"username"`     // 用户名
	CmdPattern  string `json:"cmd_pattern"`  // 命令行正则
}

// ProcessMonitor 进程监控器
type ProcessMonitor struct {
	ctx            context.Context
	cancel         context.CancelFunc
	interval       time.Duration
	knownProcesses map[int32]*ProcessInfo
	mu             sync.RWMutex

	// 回调函数
	onProcessCreate func(event *ProcessEvent)
	onProcessExit   func(event *ProcessEvent)

	// 白名单规则
	whitelist []*ProcessWhitelistRule

	// 运行状态
	running bool
}

// ProcessMonitorOption 进程监控器配置选项
type ProcessMonitorOption func(*ProcessMonitor)

// WithProcessMonitorInterval 设置监控间隔
// Example:
// ```
// monitor = hids.NewProcessMonitor(hids.WithProcessMonitorInterval(2))
// ```
func WithProcessMonitorInterval(seconds float64) ProcessMonitorOption {
	return func(m *ProcessMonitor) {
		m.interval = time.Duration(seconds * float64(time.Second))
	}
}

// WithOnProcessCreate 设置进程创建回调
// Example:
// ```
//
//	monitor = hids.NewProcessMonitor(hids.WithOnProcessCreate(func(event) {
//	    println("New process:", event.Process.Name, "PID:", event.Process.Pid)
//	}))
//
// ```
func WithOnProcessCreate(callback func(event *ProcessEvent)) ProcessMonitorOption {
	return func(m *ProcessMonitor) {
		m.onProcessCreate = callback
	}
}

// WithOnProcessExit 设置进程退出回调
// Example:
// ```
//
//	monitor = hids.NewProcessMonitor(hids.WithOnProcessExit(func(event) {
//	    println("Process exited:", event.Process.Name, "PID:", event.Process.Pid)
//	}))
//
// ```
func WithOnProcessExit(callback func(event *ProcessEvent)) ProcessMonitorOption {
	return func(m *ProcessMonitor) {
		m.onProcessExit = callback
	}
}

// WithWhitelist 设置进程白名单规则
// Example:
// ```
// rules = [hids.NewWhitelistRule()]
// rules[0].Name = "nginx"
// monitor = hids.NewProcessMonitor(hids.WithWhitelist(rules))
// ```
func WithWhitelist(rules []*ProcessWhitelistRule) ProcessMonitorOption {
	return func(m *ProcessMonitor) {
		m.whitelist = rules
	}
}

// NewWhitelistRule 创建新的白名单规则
// Example:
// ```
// rule = hids.NewWhitelistRule()
// rule.Name = "nginx"
// rule.ExePath = "/usr/sbin/nginx"
// ```
func NewWhitelistRule() *ProcessWhitelistRule {
	return &ProcessWhitelistRule{}
}

// NewProcessMonitor 创建进程监控器
// Example:
// ```
// monitor = hids.NewProcessMonitor(
//
//	hids.WithProcessMonitorInterval(1),
//	hids.WithOnProcessCreate(func(event) {
//	    println("New process:", event.Process.Name)
//	}),
//
// )
// ```
func NewProcessMonitor(opts ...ProcessMonitorOption) *ProcessMonitor {
	m := &ProcessMonitor{
		interval:       5 * time.Second,
		knownProcesses: make(map[int32]*ProcessInfo),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Start 启动进程监控
// Example:
// ```
// monitor = hids.NewProcessMonitor()
// err = monitor.Start()
// time.Sleep(10)
// monitor.Stop()
// ```
func (m *ProcessMonitor) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return utils.Error("process monitor is already running")
	}
	m.running = true
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.mu.Unlock()

	// 初始化已知进程列表
	if err := m.initKnownProcesses(); err != nil {
		return err
	}

	go m.monitorLoop()
	return nil
}

// Stop 停止进程监控
// Example:
// ```
// monitor.Stop()
// ```
func (m *ProcessMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	m.running = false
	if m.cancel != nil {
		m.cancel()
	}
}

// IsRunning 检查监控器是否在运行
func (m *ProcessMonitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// initKnownProcesses 初始化已知进程列表
func (m *ProcessMonitor) initKnownProcesses() error {
	procs, err := PS()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range procs {
		m.knownProcesses[p.Pid] = p
	}

	return nil
}

// monitorLoop 监控循环
func (m *ProcessMonitor) monitorLoop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkProcesses()
		}
	}
}

// checkProcesses 检查进程变化
func (m *ProcessMonitor) checkProcesses() {
	procs, err := PS()
	if err != nil {
		log.Errorf("failed to get process list: %v", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	currentPids := make(map[int32]bool)
	now := time.Now().Unix()

	// 检查新进程
	for _, p := range procs {
		currentPids[p.Pid] = true

		if _, known := m.knownProcesses[p.Pid]; !known {
			// 新进程
			m.knownProcesses[p.Pid] = p

			if m.onProcessCreate != nil {
				event := &ProcessEvent{
					Type:      ProcessEventCreate,
					Process:   p,
					Timestamp: now,
				}
				go m.onProcessCreate(event)
			}
		}
	}

	// 检查退出的进程
	for pid, info := range m.knownProcesses {
		if !currentPids[pid] {
			// 进程已退出
			delete(m.knownProcesses, pid)

			if m.onProcessExit != nil {
				event := &ProcessEvent{
					Type:      ProcessEventExit,
					Process:   info,
					Timestamp: now,
				}
				go m.onProcessExit(event)
			}
		}
	}
}

// IsWhitelisted 检查进程是否在白名单中
// Example:
// ```
// info, _ = hids.GetProcessByPid(1234)
// isWhite = monitor.IsWhitelisted(info)
// ```
func (m *ProcessMonitor) IsWhitelisted(info *ProcessInfo) bool {
	if len(m.whitelist) == 0 {
		return false
	}

	for _, rule := range m.whitelist {
		if m.matchWhitelistRule(info, rule) {
			return true
		}
	}

	return false
}

// matchWhitelistRule 检查进程是否匹配白名单规则
func (m *ProcessMonitor) matchWhitelistRule(info *ProcessInfo, rule *ProcessWhitelistRule) bool {
	// 检查进程名
	if rule.Name != "" && info.Name != rule.Name {
		return false
	}

	// 检查进程名正则
	if rule.NamePattern != "" {
		matched, _ := regexp.MatchString(rule.NamePattern, info.Name)
		if !matched {
			return false
		}
	}

	// 检查可执行文件路径
	if rule.ExePath != "" && info.Exe != rule.ExePath {
		return false
	}

	// 检查可执行文件路径正则
	if rule.ExePattern != "" {
		matched, _ := regexp.MatchString(rule.ExePattern, info.Exe)
		if !matched {
			return false
		}
	}

	// 检查用户名
	if rule.Username != "" && info.Username != rule.Username {
		return false
	}

	// 检查命令行正则
	if rule.CmdPattern != "" {
		matched, _ := regexp.MatchString(rule.CmdPattern, info.Cmdline)
		if !matched {
			return false
		}
	}

	// 检查可执行文件哈希
	if rule.ExeHash != "" && info.Exe != "" {
		hash := getFileHash(info.Exe)
		if hash != "" && !strings.EqualFold(hash, rule.ExeHash) {
			return false
		}
	}

	return true
}

// AddWhitelistRule 添加白名单规则
// Example:
// ```
// rule = hids.NewWhitelistRule()
// rule.Name = "nginx"
// monitor.AddWhitelistRule(rule)
// ```
func (m *ProcessMonitor) AddWhitelistRule(rule *ProcessWhitelistRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.whitelist = append(m.whitelist, rule)
}

// ClearWhitelist 清空白名单
func (m *ProcessMonitor) ClearWhitelist() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.whitelist = nil
}

// getFileHash 获取文件哈希（优先MD5，如果提供的是SHA256则计算SHA256）
func getFileHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	// 计算MD5
	hash := md5.New()
	if _, err := io.Copy(hash, f); err != nil {
		return ""
	}

	return hex.EncodeToString(hash.Sum(nil))
}

// GetFileHashMD5 获取文件MD5哈希
// Example:
// ```
// hash = hids.GetFileHashMD5("/usr/bin/nginx")
// ```
func GetFileHashMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// GetFileHashSHA256 获取文件SHA256哈希
// Example:
// ```
// hash = hids.GetFileHashSHA256("/usr/bin/nginx")
// ```
func GetFileHashSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// WatchProcess 简单的进程监控函数，监控指定时长后返回事件列表
// Example:
// ```
// events, err = hids.WatchProcess(5) // 监控5秒
// for _, event := range events {
//
//	println(event.Type, event.Process.Name, event.Process.Pid)
//
// }
// ```
func WatchProcess(durationSeconds float64) ([]*ProcessEvent, error) {
	var events []*ProcessEvent
	var mu sync.Mutex

	monitor := NewProcessMonitor(
		WithProcessMonitorInterval(0.5),
		WithOnProcessCreate(func(event *ProcessEvent) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		}),
		WithOnProcessExit(func(event *ProcessEvent) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		}),
	)

	if err := monitor.Start(); err != nil {
		return nil, err
	}

	time.Sleep(time.Duration(durationSeconds * float64(time.Second)))
	monitor.Stop()

	return events, nil
}

// ProcessExists 检查指定PID的进程是否存在
// Example:
// ```
// exists = hids.ProcessExists(1234)
// ```
func ProcessExists(pid int32) bool {
	exists, _ := process.PidExists(pid)
	return exists
}
