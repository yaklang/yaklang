package sysproc

import (
	"context"
	"fmt"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"log"
	"sync"
	"time"
)

type ProcessBasicInfo struct {
	Pid     int32
	Exe     string
	Cmdline string
	Name    string
}

func NewProcessBasicInfo(p *process.Process) (*ProcessBasicInfo, error) {
	exe, err := p.Exe()
	if err != nil {
		return nil, err
	}
	cmdline, err := p.Cmdline()
	if err != nil {
		return nil, err
	}
	name, err := p.Name()
	if err != nil {
		return nil, err
	}
	return &ProcessBasicInfo{
		Pid:     p.Pid,
		Exe:     exe,
		Cmdline: cmdline,
		Name:    name,
	}, nil
}

type OnProcessCreateFunc func(ctx context.Context, p *ProcessBasicInfo)

// OnProcessExitFunc 是进程退出时的回调函数类型
type OnProcessExitFunc func(ctx context.Context, p *ProcessBasicInfo)

// supervisor 结构体用于管理监控循环的生命周期，而无需修改 ProcessesWatcher
type supervisor struct {
	cancelFunc      context.CancelFunc
	wg              sync.WaitGroup
	onProcessCreate OnProcessCreateFunc
	onProcessExit   OnProcessExitFunc
	checkInterval   time.Duration
}
type ProcessesWatcher struct {
	// 这个 ctx 将由 Start 方法初始化和管理
	ctx               context.Context
	activeProcessLock sync.Mutex
	activeProcesses   map[int32]*ProcessBasicInfo

	// 内部的 supervisor，用于解耦生命周期控制
	supervisor supervisor
}

// NewProcessesWatcher 创建并初始化一个新的进程监控器
func NewProcessesWatcher() *ProcessesWatcher {
	return &ProcessesWatcher{
		activeProcesses: make(map[int32]*ProcessBasicInfo),
	}
}

// Start 启动进程监控.
// onProcessCreate: 匹配的进程出现时的回调.
// onProcessExit: 匹配的进程消失时的回调.
// checkInterval: 扫描进程列表的时间间隔.
func (pw *ProcessesWatcher) Start(onProcessCreate OnProcessCreateFunc, onProcessExit OnProcessExitFunc, checkInterval time.Duration) {
	log.Println("ProcessesWatcher starting...")
	// 初始化 supervisor 和 context
	// 每次调用 Start 都会重置监控循环
	baseCtx, cancel := context.WithCancel(context.Background())
	pw.ctx = baseCtx
	pw.supervisor.cancelFunc = cancel

	// 提供默认的空回调，防止 nil 调用
	if onProcessCreate == nil {
		pw.supervisor.onProcessCreate = func(ctx context.Context, p *ProcessBasicInfo) {}
	} else {
		pw.supervisor.onProcessCreate = onProcessCreate
	}
	if onProcessExit == nil {
		pw.supervisor.onProcessExit = func(ctx context.Context, p *ProcessBasicInfo) {}
	} else {
		pw.supervisor.onProcessExit = onProcessExit
	}

	if checkInterval <= 0 {
		pw.supervisor.checkInterval = 5 * time.Second
	} else {
		pw.supervisor.checkInterval = checkInterval
	}
	pw.supervisor.wg.Add(1)
	go pw.monitorLoop()
}

// Stop 停止进程监控
func (pw *ProcessesWatcher) Stop() {
	log.Println("ProcessesWatcher stopping...")
	if pw.supervisor.cancelFunc != nil {
		pw.supervisor.cancelFunc() // 发送停止信号
	}
	pw.supervisor.wg.Wait() // 等待 monitorLoop 优雅退出
	log.Println("ProcessesWatcher stopped.")
}

// monitorLoop 是核心的后台 goroutine，定期扫描和比较进程
func (pw *ProcessesWatcher) monitorLoop() {
	defer pw.supervisor.wg.Done()
	ticker := time.NewTicker(pw.supervisor.checkInterval)
	defer ticker.Stop()
	// 在循环开始前，先执行一次扫描，初始化 activeProcesses 列表
	log.Println("Performing initial process scan...")
	pw.scanAndNotify()
	for {
		select {
		case <-pw.ctx.Done():
			// 如果接收到停止信号, 则退出循环
			return
		case <-ticker.C:
			//按时执行扫描
			pw.scanAndNotify()
		}
	}
}

// scanAndNotify 执行一次完整的进程扫描、比较和通知
func (pw *ProcessesWatcher) scanAndNotify() {
	currentProcesses, err := pw.getAllProcesses()
	if err != nil {
		log.Printf("Error scanning processes: %v", err)
		return
	}
	pw.activeProcessLock.Lock()
	defer pw.activeProcessLock.Unlock()
	currentPIDs := make(map[int32]struct{})
	// 阶段 1: 检查新进程
	for pid, pInfo := range currentProcesses {
		currentPIDs[pid] = struct{}{}
		if _, exists := pw.activeProcesses[pid]; !exists {
			pw.activeProcesses[pid] = pInfo
			go pw.supervisor.onProcessCreate(pw.ctx, pInfo)
		}
	}
	// 阶段 2: 检查已退出进程
	for pid, pInfo := range pw.activeProcesses {
		if _, exists := currentPIDs[pid]; !exists {
			go pw.supervisor.onProcessExit(pw.ctx, pInfo)
			delete(pw.activeProcesses, pid)
		}
	}
}

// getAllProcesses 获取当前系统上所有能正常获取信息的进程列表
func (pw *ProcessesWatcher) getAllProcesses() (map[int32]*ProcessBasicInfo, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("failed to list processes: %w", err)
	}
	allProcessInfo := make(map[int32]*ProcessBasicInfo)
	for _, p := range procs {
		pInfo, err := NewProcessBasicInfo(p)
		if err != nil {
			continue // 忽略无法获取信息的进程
		}

		allProcessInfo[p.Pid] = pInfo
	}
	return allProcessInfo, nil
}

// GetAllProcesses 获取当前系统所有进程的信息快照
func (pw *ProcessesWatcher) GetAllProcesses() ([]*process.Process, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("failed to list all processes: %w", err)
	}
	allProcsInfo := make([]*process.Process, 0, len(procs))
	for _, p := range procs {
		allProcsInfo = append(allProcsInfo, p)
	}
	return allProcsInfo, nil
}

func (pw *ProcessesWatcher) DetectProcessConnections(pid int32, limit int) ([]net.ConnectionStat, error) {
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
