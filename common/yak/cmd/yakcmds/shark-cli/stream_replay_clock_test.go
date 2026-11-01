package sharkcli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOfflinePreviewClockCannotCloseAnotherCaptureDomain(t *testing.T) {
	store := newStreamStore(8, 4096)
	store.captureReplay = true
	defer store.finish()
	first := tcpPacket(t, 1, 99, "", false, true, false, false)
	first.ci.Timestamp = time.Unix(100, 0)
	store.add(first)
	later := tcpPacket(t, 2, 103, "def", false, false, false, false)
	later.ci.Timestamp = time.Unix(105, 0)
	store.add(later)
	foreign := tcpPacket(t, 3, 499, "", false, true, false, false)
	foreign.ci.InterfaceIndex = 1
	foreign.ci.Timestamp = time.Unix(100000, 0)
	store.add(foreign)
	missing := tcpPacket(t, 4, 100, "abc", false, false, false, false)
	missing.ci.Timestamp = time.Unix(106, 0)
	store.add(missing)
	require.Equal(t, first.streamID, missing.streamID)
	snapshot := store.snapshot(first.streamID)
	require.NotNil(t, snapshot)
	require.Empty(t, snapshot.closed, "another interface's future epoch is not this peer's FIN or idle timeout")
	require.Zero(t, snapshot.gaps, "an unrelated epoch must not force the pending suffix across a gap")
	require.Equal(t, []byte("abcdef"), snapshot.prefix[0])
	store.tick(time.Now().Add(24 * time.Hour))
	require.Empty(t, store.snapshot(first.streamID).closed, "wall-clock processing time does not end an offline observation")
	store.finish()
	require.Equal(t, "capture ended", store.snapshot(first.streamID).closed)
}
