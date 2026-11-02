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
	require.False(t, HasMemoryHeadroom(available+1), "a request above the reported capacity must fail")
	require.True(t, HasMemoryHeadroom(available), "a request at the reported capacity must pass")
}
