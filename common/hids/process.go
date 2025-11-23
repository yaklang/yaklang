package hids

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/guard"
	"github.com/yaklang/yaklang/common/log"
)

// ProcessInfo 进程信息结构体
type ProcessInfo struct {
	Pid            int      `json:"pid"`
	Ppid           int32    `json:"ppid"`
	Name           string   `json:"name"`
	Exe            string   `json:"exe"`
	Cmdline        string   `json:"cmdline"`
	Status         string   `json:"status"`
	Username       string   `json:"username"`
	CPUPercent     float64  `json:"cpu_percent"`
	MemoryPercent  float32  `json:"memory_percent"`
	MemoryInfo     uint64   `json:"memory_info"` // RSS in bytes
	CreateTime     int64    `json:"create_time"`
	OpenFiles      []string `json:"open_files"`       // 打开的文件列表
	OpenFilesCount int      `json:"open_files_count"` // 打开的文件数量
	Connections    []int    `json:"connections"`      // 连接数量（按类型分组）
	NumThreads     int32    `json:"num_threads"`
	NumFDs         int32    `json:"num_fds"`
	ChildrenPid    []int32  `json:"children_pid"`
}

// GetAllProcesses 获取所有进程信息
// 使用本地 guard 包的实现，基于 ps 命令
// Example:
// ```
// procs = hids.GetAllProcesses()
//
//	for proc in procs {
//	    println(proc.Pid, proc.Name, proc.CPUPercent)
//	}
//
// ```
func GetAllProcesses() []*ProcessInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	psProcs, err := guard.CallPsAux(ctx)
	if err != nil {
		log.Errorf("get processes failed: %v", err)
		return nil
	}

	var procs []*ProcessInfo
	for _, psProc := range psProcs {
		proc := psProcessToProcessInfo(psProc)
		procs = append(procs, proc)
	}

	return procs
}

// psProcessToProcessInfo 将 guard.PsProcess 转换为 ProcessInfo
func psProcessToProcessInfo(psProc *guard.PsProcess) *ProcessInfo {
	proc := &ProcessInfo{
		Pid:           psProc.Pid,
		Ppid:          psProc.ParentPid,
		Name:          psProc.ProcessName,
		Cmdline:       psProc.Command,
		Status:        psProc.Stat,
		Username:      psProc.User,
		CPUPercent:    psProc.CPUPercent,
		MemoryPercent: float32(psProc.MEMPercent),
		MemoryInfo:    uint64(psProc.Rss) * 1024, // RSS 转换为字节
		ChildrenPid:   psProc.ChildrenPid,
	}

	return proc
}

// GetProcessCount 获取当前进程数量
// Example:
// ```
// count = hids.GetProcessCount()
// println(count)
// ```
func GetProcessCount() int {
	procs := GetAllProcesses()
	return len(procs)
}

// GetProcessByPid 根据 PID 获取进程信息
// Example:
// ```
// proc = hids.GetProcessByPid(1234)
//
//	if proc != nil {
//	    println(proc.Name, proc.CPUPercent)
//	}
//
// ```
func GetProcessByPid(pid int) *ProcessInfo {
	procs := GetAllProcesses()
	for _, p := range procs {
		if p.Pid == pid {
			return p
		}
	}
	return nil
}
