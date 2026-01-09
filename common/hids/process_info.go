package hids

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/yaklang/yaklang/common/utils"
)

// ProcessInfo 进程详细信息结构
type ProcessInfo struct {
	Pid         int32    `json:"pid"`
	PPid        int32    `json:"ppid"`
	Name        string   `json:"name"`
	Username    string   `json:"username"`
	Exe         string   `json:"exe"`
	Cmdline     string   `json:"cmdline"`
	Cwd         string   `json:"cwd"`
	Status      string   `json:"status"`
	CreateTime  int64    `json:"create_time"`
	CPUPercent  float64  `json:"cpu_percent"`
	MemPercent  float32  `json:"mem_percent"`
	NumThreads  int32    `json:"num_threads"`
	IsRunning   bool     `json:"is_running"`
	ChildrenPid []int32  `json:"children_pid"`
	Nice        int32    `json:"nice"`
	NumFds      int32    `json:"num_fds"`
	Uids        []uint32 `json:"uids"`
	Gids        []uint32 `json:"gids"`
}

// ProcessFilter 进程过滤器配置
type ProcessFilter struct {
	Pid         int32
	PPid        int32
	Name        string
	NamePattern string
	Username    string
	Status      string
	CmdPattern  string
	ExePattern  string
}

// NewProcessFilter 创建新的进程过滤器
// Example:
// ```
// filter = hids.NewProcessFilter()
// filter.Name = "nginx"
// processes = hids.PS(filter)
// ```
func NewProcessFilter() *ProcessFilter {
	return &ProcessFilter{}
}

// getProcessInfoFromProcess 从gopsutil的Process对象获取详细信息
func getProcessInfoFromProcess(p *process.Process) (*ProcessInfo, error) {
	info := &ProcessInfo{
		Pid: p.Pid,
	}

	// 获取基本信息，忽略部分错误
	info.PPid, _ = p.Ppid()
	info.Name, _ = p.Name()
	info.Username, _ = p.Username()
	info.Exe, _ = p.Exe()
	info.Cmdline, _ = p.Cmdline()
	info.Cwd, _ = p.Cwd()

	// 获取状态
	statusSlice, _ := p.Status()
	if len(statusSlice) > 0 {
		info.Status = strings.Join(statusSlice, ",")
	}

	info.CreateTime, _ = p.CreateTime()
	info.CPUPercent, _ = p.CPUPercent()
	info.MemPercent, _ = p.MemoryPercent()
	info.NumThreads, _ = p.NumThreads()
	info.IsRunning, _ = p.IsRunning()
	info.Nice, _ = p.Nice()
	info.NumFds, _ = p.NumFDs()

	// 获取UIDs和GIDs
	uids, err := p.Uids()
	if err == nil {
		info.Uids = []uint32{}
		for _, uid := range uids {
			info.Uids = append(info.Uids, uint32(uid))
		}
	}
	gids, err := p.Gids()
	if err == nil {
		info.Gids = []uint32{}
		for _, gid := range gids {
			info.Gids = append(info.Gids, uint32(gid))
		}
	}

	// 获取子进程
	children, _ := p.Children()
	if len(children) > 0 {
		info.ChildrenPid = make([]int32, len(children))
		for i, child := range children {
			info.ChildrenPid[i] = child.Pid
		}
	}

	return info, nil
}

// 辅助函数：匹配正则表达式
func matchPattern(pattern, value string) bool {
	if pattern == "" {
		return true
	}
	matched, err := regexp.MatchString(pattern, value)
	if err != nil {
		return strings.Contains(value, pattern)
	}
	return matched
}

// filterProcess 检查进程是否匹配过滤器
func filterProcess(info *ProcessInfo, filter *ProcessFilter) bool {
	if filter == nil {
		return true
	}

	if filter.Pid > 0 && info.Pid != filter.Pid {
		return false
	}
	if filter.PPid > 0 && info.PPid != filter.PPid {
		return false
	}
	if filter.Name != "" && !strings.Contains(strings.ToLower(info.Name), strings.ToLower(filter.Name)) {
		return false
	}
	if filter.NamePattern != "" && !matchPattern(filter.NamePattern, info.Name) {
		return false
	}
	if filter.Username != "" && info.Username != filter.Username {
		return false
	}
	if filter.Status != "" && !strings.Contains(strings.ToLower(info.Status), strings.ToLower(filter.Status)) {
		return false
	}
	if filter.CmdPattern != "" && !matchPattern(filter.CmdPattern, info.Cmdline) {
		return false
	}
	if filter.ExePattern != "" && !matchPattern(filter.ExePattern, info.Exe) {
		return false
	}

	return true
}

// PS 获取进程列表，可选择使用过滤器
// Example:
// ```
// // 获取所有进程
// processes, err = hids.PS()
//
// // 使用过滤器
// filter = hids.NewProcessFilter()
// filter.Name = "nginx"
// processes, err = hids.PS(filter)
// ```
func PS(filters ...*ProcessFilter) ([]*ProcessInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, utils.Errorf("failed to get process list: %v", err)
	}

	var filter *ProcessFilter
	if len(filters) > 0 {
		filter = filters[0]
	}

	var result []*ProcessInfo
	for _, p := range procs {
		info, err := getProcessInfoFromProcess(p)
		if err != nil {
			continue
		}
		if filterProcess(info, filter) {
			result = append(result, info)
		}
	}

	return result, nil
}

// GetProcessByPid 根据PID获取进程详细信息
// Example:
// ```
// info, err = hids.GetProcessByPid(1234)
//
//	if err == nil {
//	    println("Process Name:", info.Name)
//	    println("Process User:", info.Username)
//	}
//
// ```
func GetProcessByPid(pid int32) (*ProcessInfo, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil, utils.Errorf("process %d not found: %v", pid, err)
	}
	return getProcessInfoFromProcess(p)
}

// GetProcessChildren 获取进程的所有子进程
// Example:
// ```
// children, err = hids.GetProcessChildren(1234)
//
//	for _, child := range children {
//	    println("Child PID:", child.Pid, "Name:", child.Name)
//	}
//
// ```
func GetProcessChildren(pid int32) ([]*ProcessInfo, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil, utils.Errorf("process %d not found: %v", pid, err)
	}

	children, err := p.Children()
	if err != nil {
		return nil, utils.Errorf("failed to get children of process %d: %v", pid, err)
	}

	var result []*ProcessInfo
	for _, child := range children {
		info, err := getProcessInfoFromProcess(child)
		if err != nil {
			continue
		}
		result = append(result, info)
	}

	return result, nil
}

// GetProcessParent 获取进程的父进程信息
// Example:
// ```
// parent, err = hids.GetProcessParent(1234)
//
//	if err == nil {
//	    println("Parent PID:", parent.Pid, "Name:", parent.Name)
//	}
//
// ```
func GetProcessParent(pid int32) (*ProcessInfo, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil, utils.Errorf("process %d not found: %v", pid, err)
	}

	parent, err := p.Parent()
	if err != nil {
		return nil, utils.Errorf("failed to get parent of process %d: %v", pid, err)
	}

	return getProcessInfoFromProcess(parent)
}

// ProcessTreeNode 进程树节点
type ProcessTreeNode struct {
	Info     *ProcessInfo       `json:"info"`
	Children []*ProcessTreeNode `json:"children"`
}

// GetProcessTree 获取进程树（从指定PID开始，或从init进程开始）
// Example:
// ```
// // 获取指定进程的进程树
// tree, err = hids.GetProcessTree(1234)
//
// // 获取整个系统的进程树
// tree, err = hids.GetProcessTree(1)
// ```
func GetProcessTree(rootPid int32) (*ProcessTreeNode, error) {
	// 获取所有进程
	procs, err := PS()
	if err != nil {
		return nil, err
	}

	// 建立PID到进程信息的映射
	pidMap := make(map[int32]*ProcessInfo)
	for _, p := range procs {
		pidMap[p.Pid] = p
	}

	// 检查根进程是否存在
	rootInfo, ok := pidMap[rootPid]
	if !ok {
		return nil, utils.Errorf("root process %d not found", rootPid)
	}

	// 建立父子关系映射
	childrenMap := make(map[int32][]*ProcessInfo)
	for _, p := range procs {
		childrenMap[p.PPid] = append(childrenMap[p.PPid], p)
	}

	// 递归构建树
	var buildTree func(info *ProcessInfo) *ProcessTreeNode
	buildTree = func(info *ProcessInfo) *ProcessTreeNode {
		node := &ProcessTreeNode{
			Info: info,
		}
		children := childrenMap[info.Pid]
		for _, child := range children {
			node.Children = append(node.Children, buildTree(child))
		}
		return node
	}

	return buildTree(rootInfo), nil
}

// GetProcessAncestors 获取进程的所有祖先进程（父进程链）
// Example:
// ```
// ancestors, err = hids.GetProcessAncestors(1234)
//
//	for _, ancestor := range ancestors {
//	    println("Ancestor PID:", ancestor.Pid, "Name:", ancestor.Name)
//	}
//
// ```
func GetProcessAncestors(pid int32) ([]*ProcessInfo, error) {
	var ancestors []*ProcessInfo
	currentPid := pid

	visited := make(map[int32]bool)
	for {
		if visited[currentPid] {
			break // 防止循环
		}
		visited[currentPid] = true

		p, err := process.NewProcess(currentPid)
		if err != nil {
			break
		}

		ppid, err := p.Ppid()
		if err != nil || ppid == 0 || ppid == currentPid {
			break
		}

		parent, err := process.NewProcess(ppid)
		if err != nil {
			break
		}

		info, err := getProcessInfoFromProcess(parent)
		if err != nil {
			break
		}

		ancestors = append(ancestors, info)
		currentPid = ppid
	}

	return ancestors, nil
}

// KillProcess 终止进程
// Example:
// ```
// err = hids.KillProcess(1234)
// ```
func KillProcess(pid int32) error {
	p, err := process.NewProcess(pid)
	if err != nil {
		return utils.Errorf("process %d not found: %v", pid, err)
	}
	return p.Kill()
}

// GetCurrentProcessInfo 获取当前进程信息
// Example:
// ```
// info, err = hids.GetCurrentProcessInfo()
// println("Current PID:", info.Pid)
// ```
func GetCurrentProcessInfo() (*ProcessInfo, error) {
	pid := int32(os.Getpid())
	return GetProcessByPid(pid)
}

// String 进程信息字符串表示
func (p *ProcessInfo) String() string {
	return fmt.Sprintf("Process[%d] %s (User: %s, PPID: %d, Status: %s)",
		p.Pid, p.Name, p.Username, p.PPid, p.Status)
}
