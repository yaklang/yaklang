package sysproc

import (
	"context"
	"fmt"
	"github.com/gobwas/glob"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"log"
	"sync"
	"time"
)

type OnProcessCreateFunc func(p ProcessInfo)

// OnProcessExitFunc 是进程退出时的回调函数类型
type OnProcessExitFunc func(p ProcessInfo)

// supervisor 结构体用于管理监控循环的生命周期，而无需修改 ProcessesMonitor
type supervisor struct {
	cancelFunc      context.CancelFunc
	wg              sync.WaitGroup
	onProcessCreate OnProcessCreateFunc
	onProcessExit   OnProcessExitFunc
	checkInterval   time.Duration
}
type ProcessesMonitor struct {
	patternLock sync.Mutex
	globPattern map[string]*glob.Glob

	watchProcessLock  sync.Mutex
	watchingProcesses map[int32]context.CancelFunc

	// 这个 ctx 将由 Start 方法初始化和管理
	ctx               context.Context
	activeProcessLock sync.Mutex
	activeProcesses   map[int32]ProcessInfo // 修改为空结构体 `struct{}` 为 `ProcessInfo` 以存储信息用于回调

	// 内部的 supervisor，用于解耦生命周期控制
	supervisor supervisor
}

// NewProcessesMonitor 创建并初始化一个新的进程监控器
func NewProcessesMonitor() *ProcessesMonitor {
	return &ProcessesMonitor{
		globPattern:       make(map[string]*glob.Glob),
		watchingProcesses: make(map[int32]context.CancelFunc),
		activeProcesses:   make(map[int32]ProcessInfo),
	}
}

// Start 启动进程监控.
// onProcessCreate: 匹配的进程出现时的回调.
// onProcessExit: 匹配的进程消失时的回调.
// checkInterval: 扫描进程列表的时间间隔.
func (pm *ProcessesMonitor) Start(onProcessCreate OnProcessCreateFunc, onProcessExit OnProcessExitFunc, checkInterval time.Duration) {
	log.Println("ProcessesMonitor starting...")
	// 初始化 supervisor 和 context
	// 每次调用 Start 都会重置监控循环
	baseCtx, cancel := context.WithCancel(context.Background())
	pm.ctx = baseCtx
	pm.supervisor.cancelFunc = cancel

	// 提供默认的空回调，防止 nil 调用
	if onProcessCreate == nil {
		pm.supervisor.onProcessCreate = func(p ProcessInfo) {}
	} else {
		pm.supervisor.onProcessCreate = onProcessCreate
	}
	if onProcessExit == nil {
		pm.supervisor.onProcessExit = func(p ProcessInfo) {}
	} else {
		pm.supervisor.onProcessExit = onProcessExit
	}

	if checkInterval <= 0 {
		pm.supervisor.checkInterval = 5 * time.Second
	} else {
		pm.supervisor.checkInterval = checkInterval
	}
	pm.supervisor.wg.Add(1)
	go pm.monitorLoop()
}

// Stop 停止进程监控
func (pm *ProcessesMonitor) Stop() {
	log.Println("ProcessesMonitor stopping...")
	if pm.supervisor.cancelFunc != nil {
		pm.supervisor.cancelFunc() // 发送停止信号
	}
	pm.supervisor.wg.Wait() // 等待 monitorLoop 优雅退出
	log.Println("ProcessesMonitor stopped.")
}

// monitorLoop 是核心的后台 goroutine，定期扫描和比较进程
func (pm *ProcessesMonitor) monitorLoop() {
	defer pm.supervisor.wg.Done()
	ticker := time.NewTicker(pm.supervisor.checkInterval)
	defer ticker.Stop()
	// 在循环开始前，先执行一次扫描，初始化 activeProcesses 列表
	log.Println("Performing initial process scan...")
	pm.scanAndNotify()
	for {
		select {
		case <-pm.ctx.Done():
			// 如果接收到停止信号，清理并退出
			pm.clearActiveProcesses()
			return
		case <-ticker.C:
			//按时执行扫描
			pm.scanAndNotify()
		}
	}
}

// scanAndNotify 执行一次完整的进程扫描、比较和通知
func (pm *ProcessesMonitor) scanAndNotify() {
	currentProcesses, err := pm.getFilteredProcesses()
	if err != nil {
		log.Printf("Error scanning processes: %v", err)
		return
	}
	pm.activeProcessLock.Lock()
	defer pm.activeProcessLock.Unlock()
	currentPIDs := make(map[int32]struct{})
	// 阶段 1: 检查新进程
	for pid, pInfo := range currentProcesses {
		currentPIDs[pid] = struct{}{}
		if _, exists := pm.activeProcesses[pid]; !exists {
			pm.activeProcesses[pid] = pInfo
			go pm.supervisor.onProcessCreate(pInfo)
		}
	}
	// 阶段 2: 检查已退出进程
	for pid, pInfo := range pm.activeProcesses {
		if _, exists := currentPIDs[pid]; !exists {
			go pm.supervisor.onProcessExit(pInfo)
			delete(pm.activeProcesses, pid)
		}
	}
}

// getFilteredProcesses 获取当前系统上所有符合 glob 模式的进程列表
func (pm *ProcessesMonitor) getFilteredProcesses() (map[int32]ProcessInfo, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("failed to list processes: %w", err)
	}
	matchingProcs := make(map[int32]ProcessInfo)
	for _, p := range procs {
		exe, err := p.Exe()
		if err != nil {
			continue // 忽略无法获取信息的进程
		}
		if pm.matches(exe) {
			matchingProcs[p.Pid] = ProcessInfo{
				Process: p,
			}
		}
	}
	return matchingProcs, nil
}

// matches 检查给定的可执行文件路径是否匹配任何一个已添加的 glob 模式
func (pm *ProcessesMonitor) matches(exePath string) bool {
	pm.patternLock.Lock()
	defer pm.patternLock.Unlock()
	// 如果没有设置任何模式，则默认不匹配任何进程
	if len(pm.globPattern) == 0 {
		return false
	}
	//for _, g := range pm.globPattern {
	//	if (exePath) {
	//		return true
	//	}
	//}
	return false
}

// clearActiveProcesses 在监控停止时，将所有当前活跃的进程视为 "退出"
func (pm *ProcessesMonitor) clearActiveProcesses() {
	pm.activeProcessLock.Lock()
	defer pm.activeProcessLock.Unlock()
	log.Println("Clearing active processes as 'exited'...")
	for _, pInfo := range pm.activeProcesses {
		go pm.supervisor.onProcessExit(pInfo)
	}

	// 清空 map
	pm.activeProcesses = make(map[int32]ProcessInfo)
}

// GetAllProcesses 获取当前系统所有进程的信息快照
func (pm *ProcessesMonitor) GetAllProcesses() ([]ProcessInfo, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("failed to list all processes: %w", err)
	}
	allProcsInfo := make([]ProcessInfo, 0, len(procs))
	for _, p := range procs {
		exe, _ := p.Exe()
		ppid, _ := p.Ppid()
		cmdline, _ := p.Cmdline()
		allProcsInfo = append(allProcsInfo, ProcessInfo{
			PID:        p.Pid,
			PPID:       ppid,
			Executable: exe,
			Cmdline:    cmdline,
		})
	}
	return allProcsInfo, nil
}

/* 以下是您提供的、未修改的代码 */
func (pm *ProcessesMonitor) AddGlobPattern(pattern string) error {
	pm.patternLock.Lock()
	defer pm.patternLock.Unlock()
	if _, exists := pm.globPattern[pattern]; exists {
		return nil
	}
	// 在 Go 1.22 以下版本，*glob.Glob 不能直接赋给 glob.Glob，因此这里做了修改
	// 但为了保持接口一致性，我们在这里存储指针
	g, err := glob.Compile(pattern)
	if err != nil {
		return err
	}
	pm.globPattern[pattern] = &g
	return nil
}

func (pm *ProcessesMonitor) WatchProcesses(ctx context.Context, pid int32) error {
	pm.watchProcessLock.Lock()
	defer pm.watchProcessLock.Unlock()
	if _, exists := pm.watchingProcesses[pid]; exists {
		return nil
	}
	_, cancel := context.WithCancel(ctx)
	pm.watchingProcesses[pid] = cancel
	return nil
}

func (pm *ProcessesMonitor) DetectProcessConnections(pid int32, limit int) ([]net.ConnectionStat, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}
	conns, err := p.Connections()
	if err != nil {
		return nil, err
	}

	total := len(conns)
	if limit > 0 && total > limit {
		total = limit
	}
	return conns[:total], nil
}
