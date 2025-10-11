//go:build windows
// +build windows

package winpty

import (
	"fmt"
	"sync"
	"syscall"
)

// WinPTY 常量定义
const (
	WINPTY_SPAWN_FLAG_AUTO_SHUTDOWN            = 1
	WINPTY_FLAG_ALLOW_CURPROC_DESKTOP_CREATION = 0x8
)

// WinptyDLL 封装所有 WinPTY DLL 函数
type WinptyDLL struct {
	dll *syscall.LazyDLL

	// 错误处理相关
	ErrorCode *syscall.LazyProc
	ErrorMsg  *syscall.LazyProc
	ErrorFree *syscall.LazyProc

	// Agent 配置相关
	ConfigNew             *syscall.LazyProc
	ConfigFree            *syscall.LazyProc
	ConfigSetInitialSize  *syscall.LazyProc
	ConfigSetMouseMode    *syscall.LazyProc
	ConfigSetAgentTimeout *syscall.LazyProc

	// Agent 启动相关
	Open         *syscall.LazyProc
	AgentProcess *syscall.LazyProc

	// I/O 管道相关
	ConinName  *syscall.LazyProc
	ConoutName *syscall.LazyProc
	ConerrName *syscall.LazyProc

	// Agent RPC 调用相关
	SpawnConfigNew  *syscall.LazyProc
	SpawnConfigFree *syscall.LazyProc
	Spawn           *syscall.LazyProc
	SetSize         *syscall.LazyProc
	Free            *syscall.LazyProc
}

var (
	globalWinptyDLL *WinptyDLL
	dllMutex        sync.RWMutex
)

// LoadDLL 加载 WinPTY DLL 并返回函数结构体
func LoadDLL(dllPath string) (*WinptyDLL, error) {
	// 使用读锁检查是否已经加载
	dllMutex.RLock()
	if globalWinptyDLL != nil {
		dllMutex.RUnlock()
		return globalWinptyDLL, nil
	}
	dllMutex.RUnlock()

	// 使用写锁进行加载
	dllMutex.Lock()
	defer dllMutex.Unlock()

	// 双重检查，防止在获取写锁期间其他 goroutine 已经加载了
	if globalWinptyDLL != nil {
		return globalWinptyDLL, nil
	}

	dll := syscall.NewLazyDLL(dllPath)

	// 检查 DLL 是否可用
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("failed to load winpty.dll from %s: %v", dllPath, err)
	}

	// 创建 WinptyDLL 结构体并加载所有函数
	winptyDLL := &WinptyDLL{
		dll: dll,

		// 错误处理相关
		ErrorCode: dll.NewProc("winpty_error_code"),
		ErrorMsg:  dll.NewProc("winpty_error_msg"),
		ErrorFree: dll.NewProc("winpty_error_free"),

		// Agent 配置相关
		ConfigNew:             dll.NewProc("winpty_config_new"),
		ConfigFree:            dll.NewProc("winpty_config_free"),
		ConfigSetInitialSize:  dll.NewProc("winpty_config_set_initial_size"),
		ConfigSetMouseMode:    dll.NewProc("winpty_config_set_mouse_mode"),
		ConfigSetAgentTimeout: dll.NewProc("winpty_config_set_agent_timeout"),

		// Agent 启动相关
		Open:         dll.NewProc("winpty_open"),
		AgentProcess: dll.NewProc("winpty_agent_process"),

		// I/O 管道相关
		ConinName:  dll.NewProc("winpty_conin_name"),
		ConoutName: dll.NewProc("winpty_conout_name"),
		ConerrName: dll.NewProc("winpty_conerr_name"),

		// Agent RPC 调用相关
		SpawnConfigNew:  dll.NewProc("winpty_spawn_config_new"),
		SpawnConfigFree: dll.NewProc("winpty_spawn_config_free"),
		Spawn:           dll.NewProc("winpty_spawn"),
		SetSize:         dll.NewProc("winpty_set_size"),
		Free:            dll.NewProc("winpty_free"),
	}

	// 验证关键函数是否存在
	if err := winptyDLL.validateFunctions(); err != nil {
		return nil, fmt.Errorf("failed to validate winpty functions: %v", err)
	}

	// 设置全局实例
	globalWinptyDLL = winptyDLL
	return globalWinptyDLL, nil
}

// validateFunctions 验证关键函数是否可用
func (w *WinptyDLL) validateFunctions() error {
	// 检查关键函数是否存在
	criticalFuncs := []*syscall.LazyProc{
		w.ErrorFree,
		w.ConfigNew,
		w.ConfigFree,
		w.Open,
		w.ConinName,
		w.ConoutName,
		w.SpawnConfigNew,
		w.Spawn,
		w.Free,
	}

	for _, proc := range criticalFuncs {
		if err := proc.Find(); err != nil {
			return fmt.Errorf("critical function %s not found: %v", proc.Name, err)
		}
	}

	return nil
}

// GetDLL 获取全局 DLL 实例（如果存在）
func GetDLL() *WinptyDLL {
	dllMutex.RLock()
	defer dllMutex.RUnlock()
	return globalWinptyDLL
}

// IsLoaded 检查 DLL 是否已加载
func IsLoaded() bool {
	dllMutex.RLock()
	defer dllMutex.RUnlock()
	return globalWinptyDLL != nil
}

// ResetDLL 重置全局 DLL 实例（主要用于测试）
func ResetDLL() {
	dllMutex.Lock()
	defer dllMutex.Unlock()
	globalWinptyDLL = nil
}
