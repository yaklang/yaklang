//go:build windows
// +build windows

package routewrapper

import (
	"os/exec"
	"sync"
	"syscall"
)

var procSetConsoleOutputCP, procGetConsoleOutputCP uintptr
var mtx sync.Mutex

func init() {
	kernel32, err := syscall.LoadLibrary("kernel32.dll")
	if err != nil {
		panic(err.Error())
	}
	procSetConsoleOutputCP, err = syscall.GetProcAddress(kernel32, "SetConsoleOutputCP")
	if err != nil {
		panic(err.Error())
	}
	procGetConsoleOutputCP, err = syscall.GetProcAddress(kernel32, "GetConsoleOutputCP")
	if err != nil {
		panic(err.Error())
	}
}

func setConsoleOutputCP(cp uint) error {
	r, _, e := syscall.Syscall(procSetConsoleOutputCP, 1, uintptr(cp), 0, 0)
	if uint32(r) != 0 {
		return nil
	} else {
		return error(e)
	}
}

func getConsoleOutputCP() uint {
	r, _, _ := syscall.Syscall(procGetConsoleOutputCP, 0, 0, 0, 0)
	return uint(r)
}

func onBeforeCommandRun(cmd *exec.Cmd) (interface{}, error) {
	mtx.Lock()
	oldCP := getConsoleOutputCP()
	err := setConsoleOutputCP(437)
	if err != nil {
		mtx.Unlock()
		return nil, err
	}
	return oldCP, nil
}

func onAfterCommandRun(ctx interface{}, cmd *exec.Cmd) error {
	mtx.Unlock()
	return setConsoleOutputCP(ctx.(uint))
}
