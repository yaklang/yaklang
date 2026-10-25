package ssadb

import (
	"os"
	"strconv"
	"strings"
)

// indexSortMemoryFloorBytes is the free memory the host must still have before
// an index build is allowed to sort in memory. One index build for a large
// project needs a few hundred megabytes; the floor leaves room for the compile
// program that is still resident.
const indexSortMemoryFloorBytes int64 = 6 << 30

// memoryHeadroomForIndexSort reports whether the host can afford an in-memory
// index sort. Anything unexpected (non-Linux, unreadable /proc, missing field)
// returns false so the caller keeps the safer on-disk sorter.
func memoryHeadroomForIndexSort() bool {
	available, ok := availableMemoryBytes()
	return ok && available >= indexSortMemoryFloorBytes
}

func availableMemoryBytes() (int64, bool) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "MemAvailable:" {
			continue
		}
		kib, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || kib <= 0 {
			return 0, false
		}
		return kib * 1024, true
	}
	return 0, false
}
