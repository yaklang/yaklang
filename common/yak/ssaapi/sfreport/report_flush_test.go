package sfreport

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

// A snapshot is published only when the document changed. Re-adding the same
// finding leaves the document identical, so it must not schedule a new one.
func TestReport_SnapshotWaitsForAChange(t *testing.T) {
	disableFlushThrottle(t)
	report := newWritableReport(t)
	same := &Risk{Hash: "same", Title: "same"}
	for i := 0; i < 5*reportFlushEvery; i++ {
		report.AddRisks(same)
	}
	require.Len(t, report.Risks, 1)
	require.False(t, report.dirty, "one snapshot covers the only finding")
	require.False(t, report.lastFlush.IsZero())
}

// The same rule applies to risk updates: one snapshot per change, not one per
// event, and a delete is a change too.
func TestReport_SnapshotsFollowCreatesUpdatesAndDeletes(t *testing.T) {
	disableFlushThrottle(t)
	report := newWritableReport(t)
	create := schema.RiskUpdateItem{
		Mode: schema.RiskUpdateCreate,
		Risk: &schema.SSARisk{Hash: "one", Title: "one", RiskType: "demo"},
	}
	require.NoError(t, report.ApplyRiskUpdate(create))
	require.Len(t, report.Risks, 1)
	require.False(t, report.dirty)

	replacement := schema.RiskUpdateItem{
		Mode:    schema.RiskUpdateUpdate,
		OldHash: "one",
		Risk:    &schema.SSARisk{Hash: "two", Title: "two", RiskType: "demo"},
	}
	require.NoError(t, report.ApplyRiskUpdate(replacement))
	require.Len(t, report.Risks, 1)
	require.False(t, report.dirty, "the replacement is already in the snapshot")

	// A delete changes the document as well: the snapshot no longer matches.
	require.NoError(t, report.ApplyRiskUpdate(schema.RiskUpdateItem{
		Mode:    schema.RiskUpdateDelete,
		OldHash: "two",
	}))
	require.Empty(t, report.Risks)
	require.False(t, report.dirty)
}

// Findings arrive continuously during a scan, so the interval is what bounds
// how often the whole document is written. Everything that happens inside one
// interval collapses into a single snapshot.
func TestReport_RewriteChurnPublishesAtMostOncePerInterval(t *testing.T) {
	report := newWritableReport(t)
	for i := 0; i < reportFlushEvery; i++ {
		require.NoError(t, report.ApplyRiskUpdate(schema.RiskUpdateItem{
			Mode: schema.RiskUpdateCreate,
			Risk: &schema.SSARisk{Hash: fmt.Sprintf("hash-0-%d", i), Title: "base", RiskType: "demo"},
		}))
	}
	require.True(t, report.dirty, "findings added inside the interval wait for it")
	firstSnapshot := report.lastFlush
	require.False(t, firstSnapshot.IsZero(), "the first change publishes at once")

	// Rewrite every finding under a new hash: the document size is unchanged,
	// but its content is not, so one snapshot is due - not one per rewrite.
	for round := 0; round < 3; round++ {
		for i := 0; i < reportFlushEvery; i++ {
			require.NoError(t, report.ApplyRiskUpdate(schema.RiskUpdateItem{
				Mode:    schema.RiskUpdateUpdate,
				OldHash: fmt.Sprintf("hash-%d-%d", round, i),
				Risk: &schema.SSARisk{
					Hash:     fmt.Sprintf("hash-%d-%d", round+1, i),
					Title:    "rewritten",
					RiskType: "demo",
				},
			}))
		}
	}
	require.Len(t, report.Risks, reportFlushEvery)
	require.Equal(t, firstSnapshot, report.lastFlush,
		"the document changed, but the interval has not elapsed")
	require.True(t, report.dirty, "the on-disk snapshot is stale")

	// Once the interval elapsed, the accumulated change publishes one snapshot.
	report.mu.Lock()
	report.lastFlush = time.Now().Add(-2 * reportFlushMinIntervalValue)
	report.mu.Unlock()
	require.NoError(t, report.ApplyRiskUpdate(schema.RiskUpdateItem{
		Mode: schema.RiskUpdateCreate,
		Risk: &schema.SSARisk{Hash: "trigger", Title: "trigger", RiskType: "demo"},
	}))
	require.False(t, report.dirty)
	require.True(t, report.lastFlush.After(firstSnapshot))
}

// newWritableReport returns a report with a destination, which is what makes a
// snapshot possible at all: a report that is never saved has nowhere to
// publish and keeps its change flag set.
func newWritableReport(t *testing.T) *Report {
	t.Helper()
	report := NewReport(IRifyReportType)
	require.NoError(t, report.SetWriter(new(bytes.Buffer)))
	t.Cleanup(func() {
		if flusher := report.currentFlusher(); flusher != nil {
			flusher.stopAndWait()
		}
	})
	return report
}

// disableFlushThrottle keeps the change tracking independent of wall time. A
// separate test covers the throttle itself.
func disableFlushThrottle(t *testing.T) {
	t.Helper()
	prev := reportFlushMinIntervalValue
	reportFlushMinIntervalValue = 0
	t.Cleanup(func() { reportFlushMinIntervalValue = prev })
}

// Snapshots are crash resilience, not a live feed: after one publish, the next
// change must wait for the interval.
func TestReport_FlushThrottleHonoursInterval(t *testing.T) {
	prev := reportFlushMinIntervalValue
	reportFlushMinIntervalValue = 2 * time.Minute
	t.Cleanup(func() { reportFlushMinIntervalValue = prev })

	report := newWritableReport(t)

	report.AddRisks(&Risk{Hash: "first"})
	require.False(t, report.dirty)
	require.False(t, report.lastFlush.IsZero())

	// A change inside the interval must not publish.
	report.AddRisks(&Risk{Hash: "second"})
	require.True(t, report.dirty, "throttled snapshot")

	// Once the interval elapsed, the change publishes.
	report.mu.Lock()
	report.lastFlush = time.Now().Add(-2 * reportFlushMinIntervalValue)
	report.mu.Unlock()
	report.AddRisks(&Risk{Hash: "third"})
	require.False(t, report.dirty)
	require.Len(t, report.Risks, 3)
}
