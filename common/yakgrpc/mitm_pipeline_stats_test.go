//go:build !yakit_exclude

package yakgrpc

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestMITMPipelineTrackerRequestStages(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	now := base
	tracker := newMITMPipelineTracker("session-test")
	tracker.startedAt = base
	tracker.now = func() time.Time { return now }
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	require.NoError(t, err)

	tracker.requestObserved(req)
	now = now.Add(100 * time.Millisecond)
	tracker.manualWaitStarted(req, false)
	now = now.Add(500 * time.Millisecond)
	stats := tracker.snapshot(0, 40960)
	require.Equal(t, uint64(1), stats.GetRequestTotal())
	require.Equal(t, int64(1), stats.GetManualActive())
	require.Equal(t, int64(500), stats.GetOldestManualAgeMs())

	tracker.manualWaitFinished(req, false)
	now = now.Add(100 * time.Millisecond)
	tracker.requestDispatched(req, false)
	now = now.Add(time.Second)
	stats = tracker.snapshot(3, 40960)
	require.Equal(t, uint64(1), stats.GetDispatchTotal())
	require.Equal(t, int64(1), stats.GetUpstreamActive())
	require.Equal(t, int64(1000), stats.GetOldestUpstreamAgeMs())
	require.Equal(t, int64(3), stats.GetDatabaseWriteQueueDepth())

	tracker.upstreamCompleted(req, true)
	tracker.upstreamCompleted(req, true) // response mirror fallback must not double count
	stats = tracker.snapshot(0, 40960)
	require.Equal(t, uint64(1), stats.GetUpstreamCompletedTotal())
	require.Equal(t, int64(1), stats.GetResponseProcessingActive())

	tracker.manualWaitStarted(req, true)
	now = now.Add(250 * time.Millisecond)
	stats = tracker.snapshot(0, 40960)
	require.Equal(t, int64(1), stats.GetManualActive())
	require.Equal(t, int64(250), stats.GetOldestManualAgeMs())

	tracker.manualWaitFinished(req, true)
	tracker.responseMirrored(req)
	stats = tracker.snapshot(0, 40960)
	require.Equal(t, int64(1), stats.GetResponseProcessingActive())
	tracker.responseProcessingFinished(req)
	stats = tracker.snapshot(0, 40960)
	require.Equal(t, uint64(1), stats.GetResponseMirroredTotal())
	require.Zero(t, stats.GetActiveTotal())
}

func TestMITMPipelineTrackerLocalResponseAndPersistence(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	now := base
	tracker := newMITMPipelineTracker("session-test")
	tracker.startedAt = base
	tracker.now = func() time.Time { return now }
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	require.NoError(t, err)

	tracker.requestObserved(req)
	tracker.requestDispatched(req, true)
	tracker.upstreamCompleted(req, false)
	tracker.responseMirrored(req)
	tracker.responseProcessingFinished(req)
	stats := tracker.snapshot(0, 40960)
	require.Zero(t, stats.GetDispatchTotal())
	require.Zero(t, stats.GetUpstreamCompletedTotal())
	require.Equal(t, uint64(1), stats.GetResponseMirroredTotal())

	flow := &schema.HTTPFlow{}
	tracker.flowBuilt()
	tracker.persistEnqueued(flow)
	now = now.Add(750 * time.Millisecond)
	stats = tracker.snapshot(2, 40960)
	require.Equal(t, uint64(1), stats.GetFlowBuiltTotal())
	require.Equal(t, uint64(1), stats.GetPersistEnqueuedTotal())
	require.Equal(t, int64(1), stats.GetPersistActive())
	require.Equal(t, int64(750), stats.GetOldestPersistAgeMs())

	flow.ID = 1
	tracker.persistFinished(flow, true)
	stats = tracker.snapshot(0, 40960)
	require.Equal(t, uint64(1), stats.GetPersistedTotal())
	require.Zero(t, stats.GetPersistActive())
}

func TestMITMPipelineTrackerDoesNotCountFailedUpstreamRoundTrip(t *testing.T) {
	tracker := newMITMPipelineTracker("session-test")
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	require.NoError(t, err)

	tracker.requestObserved(req)
	tracker.requestDispatched(req, false)
	tracker.upstreamCompleted(req, false)

	stats := tracker.snapshot(0, 40960)
	require.Equal(t, uint64(1), stats.GetDispatchTotal())
	require.Zero(t, stats.GetUpstreamCompletedTotal())
	require.Zero(t, stats.GetUpstreamActive())
	require.Equal(t, int64(1), stats.GetResponseProcessingActive())
}

func TestMITMPipelineTrackerDropIsTerminal(t *testing.T) {
	tracker := newMITMPipelineTracker("session-test")
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	require.NoError(t, err)

	tracker.requestObserved(req)
	tracker.requestDropped(req)
	tracker.requestDropped(req)
	stats := tracker.snapshot(0, 40960)
	require.Equal(t, uint64(1), stats.GetDroppedTotal())
	require.Zero(t, stats.GetActiveTotal())
}
