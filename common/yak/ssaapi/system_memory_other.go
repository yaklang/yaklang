//go:build !linux && !darwin

package ssaapi

// systemMemoryTotalBytes is only implemented on platforms with a reliable
// system-memory source (Linux /proc/meminfo, Darwin hw.memsize). On other
// platforms the legacy large-project memory limit is skipped (review A12).
func systemMemoryTotalBytes() int64 {
	return 0
}
