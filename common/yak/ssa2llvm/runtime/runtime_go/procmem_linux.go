//go:build linux

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// tryReadProcessMemory copies len(dst) bytes from addr without faulting.
// process_vm_readv returns EFAULT instead of delivering SIGSEGV, which
// C.GoString cannot: a bad pointer there is a fatal, unrecovered crash.
// A single iovec that crosses into an unmapped page fails as a whole, so
// callers keep each read inside one page.
func tryReadProcessMemory(addr uintptr, dst []byte) int {
	if len(dst) == 0 || addr == 0 {
		return -1
	}
	n, err := unix.ProcessVMReadv(os.Getpid(), []unix.Iovec{{
		Base: &dst[0],
		Len:  uint64(len(dst)),
	}}, []unix.RemoteIovec{{
		Base: addr,
		Len:  len(dst),
	}}, 0)
	if err != nil || n <= 0 {
		return -1
	}
	if n > len(dst) {
		n = len(dst)
	}
	return n
}
