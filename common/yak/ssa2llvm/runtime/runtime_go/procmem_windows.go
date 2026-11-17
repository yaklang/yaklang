//go:build windows

package main

/*
#include <windows.h>
*/
import "C"

import "unsafe"

// tryReadProcessMemory copies len(dst) bytes from addr without faulting.
// ReadProcessMemory reports an invalid address through its return value
// instead of raising SEH, which is what the fault-free probe needs.
func tryReadProcessMemory(addr uintptr, dst []byte) int {
	if len(dst) == 0 || addr == 0 {
		return -1
	}
	var read C.SIZE_T
	ok := C.ReadProcessMemory(
		C.GetCurrentProcess(),
		C.LPCVOID(unsafe.Pointer(addr)),
		C.LPVOID(unsafe.Pointer(&dst[0])),
		C.SIZE_T(len(dst)),
		&read,
	)
	if ok == 0 || read == 0 {
		return -1
	}
	if int(read) > len(dst) {
		return len(dst)
	}
	return int(read)
}
