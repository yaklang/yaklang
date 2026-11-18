package ssautil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The index scheduler asks ssautil whether the host still has memory for an
// in-memory sort. The metric must come from the platform probe, not from a
// Linux-only file read, and an unknown host must report "no headroom" so the
// caller keeps its conservative path.
func TestAvailableMemoryBytesIsPlatformNeutral(t *testing.T) {
	available, ok := AvailableMemoryBytes()
	if !ok {
		require.Zero(t, available, "an unknown host must not report a byte count")
		require.False(t, HasMemoryHeadroom(1), "unknown host must not grant headroom")
		return
	}
	require.Positive(t, available)
	require.True(t, HasMemoryHeadroom(1), "any known capacity grants a 1-byte request")
}

// The probe samples live memory on every call, so asserting a capacity boundary
// through it compares two different numbers. Keep those assertions on the pure
// decision instead.
func TestMemoryHeadroomBoundary(t *testing.T) {
	require.True(t, memoryHeadroom(1, true, 1), "a request at the reported capacity must pass")
	require.False(t, memoryHeadroom(1, true, 2), "a request above the reported capacity must fail")
	require.True(t, memoryHeadroom(0, true, 0))
	require.False(t, memoryHeadroom(0, false, 0), "unknown capacity must not grant headroom")
	require.False(t, memoryHeadroom(1<<40, false, 1), "unknown capacity must not grant headroom")
}