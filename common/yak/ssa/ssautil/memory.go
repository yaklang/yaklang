package ssautil

import "github.com/yaklang/yaklang/common/utils/memorybudget"

// AvailableMemoryBytes reports the memory the OS can hand to this process
// without swapping. memorybudget reads the platform metric (host RAM plus the
// cgroup limit), so this is not Linux-only.
//
// ok is false when the platform reports nothing. Callers must treat that as
// unknown: a missing metric is not evidence that memory is exhausted.
func AvailableMemoryBytes() (int64, bool) {
	available := memorybudget.Available()
	return available, available > 0
}

// HasMemoryHeadroom reports whether at least floor bytes are available now.
// An unknown host returns false so callers keep their conservative path.
func HasMemoryHeadroom(floor int64) bool {
	available, ok := AvailableMemoryBytes()
	return ok && available >= floor
}
