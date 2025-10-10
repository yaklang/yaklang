//go:build windows && 386
// +build windows,386

package winpty

import (
	"fmt"
	"syscall"
	"unsafe"
)

// createAgentCfg 创建 Agent 配置 (32位版本)
func createAgentCfg(dll *WinptyDLL, flags uint32) (uintptr, error) {
	var errorPtr uintptr

	// 检查 DLL 是否可用
	if err := dll.ErrorFree.Find(); err != nil {
		return 0, fmt.Errorf("winpty dll not available: %v", err)
	}

	defer dll.ErrorFree.Call(errorPtr)

	// 32位系统需要额外的填充参数，因为 winpty 期望 UINT64
	agentCfg, _, _ := dll.ConfigNew.Call(
		uintptr(flags),
		uintptr(0), // 填充参数
		uintptr(unsafe.Pointer(&errorPtr)),
	)

	if agentCfg == 0 {
		return 0, fmt.Errorf("unable to create agent config: %s", GetErrorMessage(dll, errorPtr))
	}

	return agentCfg, nil
}

// createSpawnCfg 创建 Spawn 配置 (32位版本)
func createSpawnCfg(dll *WinptyDLL, flags uint32, appname, cmdline, cwd string, env []string) (uintptr, error) {
	var errorPtr uintptr
	defer dll.ErrorFree.Call(errorPtr)

	// 转换字符串为 UTF16 指针
	cmdLineStr, err := syscall.UTF16PtrFromString(cmdline)
	if err != nil {
		return 0, fmt.Errorf("failed to convert command line to UTF16: %v", err)
	}

	appNameStr, err := syscall.UTF16PtrFromString(appname)
	if err != nil {
		return 0, fmt.Errorf("failed to convert app name to UTF16: %v", err)
	}

	cwdStr, err := syscall.UTF16PtrFromString(cwd)
	if err != nil {
		return 0, fmt.Errorf("failed to convert working directory to UTF16: %v", err)
	}

	envStr, err := UTF16PtrFromStringArray(env)
	if err != nil {
		return 0, fmt.Errorf("failed to convert environment to UTF16: %v", err)
	}

	// 32位系统需要额外的填充参数
	spawnCfg, _, _ := dll.SpawnConfigNew.Call(
		uintptr(flags),
		uintptr(0), // 填充参数
		uintptr(unsafe.Pointer(appNameStr)),
		uintptr(unsafe.Pointer(cmdLineStr)),
		uintptr(unsafe.Pointer(cwdStr)),
		uintptr(unsafe.Pointer(envStr)),
		uintptr(unsafe.Pointer(&errorPtr)),
	)

	if spawnCfg == 0 {
		return 0, fmt.Errorf("unable to create spawn config: %s", GetErrorMessage(dll, errorPtr))
	}

	return spawnCfg, nil
}
