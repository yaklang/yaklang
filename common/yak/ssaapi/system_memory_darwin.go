//go:build darwin

package ssaapi

import "golang.org/x/sys/unix"

func systemMemoryTotalBytes() int64 {
	v, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return int64(v)
}
