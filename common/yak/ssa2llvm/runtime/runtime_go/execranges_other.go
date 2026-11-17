//go:build !linux

package main

// runtimeReadExecRanges reports false: there is no /proc/self/maps equivalent
// wired up yet, so callers keep treating unknown pointers as executable.
func runtimeReadExecRanges() ([]runtimeExecRange, bool) {
	return nil, false
}
