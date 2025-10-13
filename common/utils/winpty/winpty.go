//go:build windows
// +build windows

package winpty

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// WinptyCfg 配置选项
type WinptyCfg struct {
	// DLLPrefix winpty.dll 和 winpty-agent.exe 的路径
	DLLPrefix string

	// AppName 控制台标题
	AppName string

	// Command 要执行的完整命令
	Command string

	// Dir 工作目录
	Dir string

	// Env 环境变量，格式为 VAR=VAL
	Env []string

	// Flags 传递给 agent 配置创建的标志
	Flags uint32

	// InitialCols 初始列数
	InitialCols uint32
	// InitialRows 初始行数
	InitialRows uint32
}

// CfgOptionFunc 配置选项函数类型
type CfgOptionFunc func(*WinptyCfg)

// WithDLLPrefix 设置 DLL 路径
func WithDLLPrefix(prefix string) CfgOptionFunc {
	return func(cfg *WinptyCfg) {
		cfg.DLLPrefix = prefix
	}
}

// WithAppName 设置应用程序名称
func WithAppName(name string) CfgOptionFunc {
	return func(cfg *WinptyCfg) {
		cfg.AppName = name
	}
}

// WithCommand 设置要执行的命令
func WithCommand(command string) CfgOptionFunc {
	return func(cfg *WinptyCfg) {
		cfg.Command = command
	}
}

// WithDir 设置工作目录
func WithDir(dir string) CfgOptionFunc {
	return func(cfg *WinptyCfg) {
		cfg.Dir = dir
	}
}

// WithEnv 设置环境变量
func WithEnv(env []string) CfgOptionFunc {
	return func(cfg *WinptyCfg) {
		cfg.Env = env
	}
}

// WithFlags 设置标志
func WithFlags(flags uint32) CfgOptionFunc {
	return func(cfg *WinptyCfg) {
		cfg.Flags = flags
	}
}

// WithInitialSize 设置初始大小
func WithInitialSize(cols, rows uint32) CfgOptionFunc {
	return func(cfg *WinptyCfg) {
		cfg.InitialCols = cols
		cfg.InitialRows = rows
	}
}

// WithDefaultEnv 使用默认环境变量
func WithDefaultEnv() CfgOptionFunc {
	return func(cfg *WinptyCfg) {
		cfg.Env = os.Environ()
	}
}

// WithCurrentDir 使用当前工作目录
func WithCurrentDir() CfgOptionFunc {
	return func(cfg *WinptyCfg) {
		if wd, err := os.Getwd(); err == nil {
			cfg.Dir = wd
		}
	}
}

// WinPTY 结构体
type WinPTY struct {
	StdIn  *os.File
	StdOut *os.File

	wp          uintptr
	childHandle uintptr
	closed      bool
}

// New 创建新的 WinPTY 实例
func New(opts ...CfgOptionFunc) (*WinPTY, error) {
	cfg := &WinptyCfg{
		InitialCols: 80,
		InitialRows: 24,
	}

	// 应用所有配置选项
	for _, opt := range opts {
		opt(cfg)
	}

	return NewWithCfg(cfg)
}

// NewDefault 使用默认配置创建 WinPTY 实例
func NewDefault(dllPath, command string) (*WinPTY, error) {
	return New(
		WithDLLPrefix(dllPath),
		WithCommand(command),
		WithCurrentDir(),
		WithDefaultEnv(),
	)
}

// NewWithCfg 使用配置结构体创建 WinPTY 实例
func NewWithCfg(cfg *WinptyCfg) (*WinPTY, error) {
	// 加载 WinPTY DLL
	dll, err := LoadDLL(cfg.DLLPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to load winpty dll: %v", err)
	}

	// 创建 agent 配置
	agentCfg, err := createAgentCfg(dll, cfg.Flags)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent config: %v", err)
	}
	defer dll.ConfigFree.Call(agentCfg)

	// 设置初始大小
	if cfg.InitialCols <= 0 {
		cfg.InitialCols = 80
	}
	if cfg.InitialRows <= 0 {
		cfg.InitialRows = 24
	}
	dll.ConfigSetInitialSize.Call(agentCfg, uintptr(cfg.InitialCols), uintptr(cfg.InitialRows))

	// 打开 winpty
	var openErr uintptr
	defer dll.ErrorFree.Call(openErr)
	wp, _, _ := dll.Open.Call(agentCfg, uintptr(unsafe.Pointer(&openErr)))

	if wp == 0 {
		return nil, fmt.Errorf("failed to open winpty: %s", GetErrorMessage(dll, openErr))
	}

	// 获取管道名称
	stdin_name, _, _ := dll.ConinName.Call(wp)
	stdout_name, _, _ := dll.ConoutName.Call(wp)

	// 创建文件句柄
	obj := &WinPTY{wp: wp}

	stdin_handle, err := syscall.CreateFile(
		(*uint16)(unsafe.Pointer(stdin_name)),
		syscall.GENERIC_WRITE,
		0, nil,
		syscall.OPEN_EXISTING,
		0, 0,
	)
	if err != nil {
		dll.Free.Call(wp)
		return nil, fmt.Errorf("failed to create stdin handle: %v", err)
	}
	obj.StdIn = os.NewFile(uintptr(stdin_handle), "winpty-stdin")

	stdout_handle, err := syscall.CreateFile(
		(*uint16)(unsafe.Pointer(stdout_name)),
		syscall.GENERIC_READ,
		0, nil,
		syscall.OPEN_EXISTING,
		0, 0,
	)
	if err != nil {
		obj.StdIn.Close()
		dll.Free.Call(wp)
		return nil, fmt.Errorf("failed to create stdout handle: %v", err)
	}
	obj.StdOut = os.NewFile(uintptr(stdout_handle), "winpty-stdout")

	// 创建 spawn 配置并启动进程
	spawnCfg, err := createSpawnCfg(
		dll,
		WINPTY_SPAWN_FLAG_AUTO_SHUTDOWN,
		cfg.AppName,
		cfg.Command,
		cfg.Dir,
		cfg.Env,
	)
	if err != nil {
		obj.Close()
		return nil, fmt.Errorf("failed to create spawn config: %v", err)
	}
	defer dll.SpawnConfigFree.Call(spawnCfg)

	var spawnErr uintptr
	defer dll.ErrorFree.Call(spawnErr)

	spawnRet, _, _ := dll.Spawn.Call(
		wp,
		spawnCfg,
		uintptr(unsafe.Pointer(&obj.childHandle)),
		0,
		0,
		uintptr(unsafe.Pointer(&spawnErr)),
	)

	if spawnRet == 0 {
		obj.Close()
		return nil, fmt.Errorf("failed to spawn process: %s", GetErrorMessage(dll, spawnErr))
	}

	return obj, nil
}

// SetSize 设置终端大小
func (w *WinPTY) SetSize(cols, rows uint32) error {
	if w.closed {
		return fmt.Errorf("winpty is closed")
	}
	if cols == 0 || rows == 0 {
		return fmt.Errorf("invalid size: cols=%d, rows=%d", cols, rows)
	}

	// 获取全局 DLL 实例
	dll := GetDLL()
	if dll == nil {
		return fmt.Errorf("winpty dll not loaded")
	}

	dll.SetSize.Call(w.wp, uintptr(cols), uintptr(rows), 0)
	return nil
}

// GetProcessHandle 获取子进程句柄
func (w *WinPTY) GetProcessHandle() uintptr {
	return w.childHandle
}

// IsClosed 检查是否已关闭
func (w *WinPTY) IsClosed() bool {
	return w.closed
}

// Close 关闭 WinPTY 实例
func (w *WinPTY) Close() error {
	if w.closed {
		return nil
	}

	// 关闭文件句柄
	if w.StdIn != nil {
		w.StdIn.Close()
	}
	if w.StdOut != nil {
		w.StdOut.Close()
	}

	// 关闭子进程句柄
	if w.childHandle != 0 {
		syscall.CloseHandle(syscall.Handle(w.childHandle))
	}

	// 释放 winpty 资源
	if w.wp != 0 {
		dll := GetDLL()
		if dll != nil {
			dll.Free.Call(w.wp)
		}
	}

	w.closed = true
	return nil
}
