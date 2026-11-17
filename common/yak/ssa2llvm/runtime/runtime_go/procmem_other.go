//go:build !linux && !windows

package main

// tryReadProcessMemory has no fault-free implementation on this platform, so
// every probe reports "unreadable". Callers then fall back to treating the
// value as an integer instead of dereferencing a possibly invalid pointer.
func tryReadProcessMemory(addr uintptr, dst []byte) int {
	return -1
}
