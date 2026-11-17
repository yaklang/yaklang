//go:build linux

package main

import (
	"bytes"
	"os"
	"strconv"
)

// runtimeReadExecRanges parses the executable mappings of the current process.
// It reports false when /proc/self/maps cannot be read, which makes callers
// fall back to permissive pointer handling.
func runtimeReadExecRanges() ([]runtimeExecRange, bool) {
	data, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		return nil, false
	}
	var ranges []runtimeExecRange
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		fields := bytes.Fields(line)
		if len(fields) < 2 || !bytes.Contains(fields[1], []byte{'x'}) {
			continue
		}
		bounds := fields[0]
		dash := bytes.IndexByte(bounds, '-')
		if dash <= 0 || dash+1 >= len(bounds) {
			continue
		}
		start, err1 := strconv.ParseUint(string(bounds[:dash]), 16, 64)
		end, err2 := strconv.ParseUint(string(bounds[dash+1:]), 16, 64)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		ranges = append(ranges, runtimeExecRange{
			start: uintptr(start),
			end:   uintptr(end),
		})
	}
	return ranges, true
}
