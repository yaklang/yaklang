package sfreport

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

// Re-adding a hash that is already in the report must not count as a new
// finding. A scan can deliver the same risk through many update events; the
// old call counter then rewrote the whole document every 500 events.
func TestReport_AddRisksCountsOnlyNewFindings(t *testing.T) {
	report := NewReport(IRifyReportType)
	same := &Risk{Hash: "same", Title: "same"}
	for i := 0; i < 5*reportFlushEvery; i++ {
		report.AddRisks(same)
	}
	require.Len(t, report.Risks, 1)
	require.Zero(t, report.flushedRisks, "no snapshot: the document holds one finding")

	// Replacing the body under the same hash does not grow the document.
	for i := 0; i < reportFlushEvery; i++ {
		report.AddRisks(&Risk{Hash: "same", Title: fmt.Sprintf("body-%d", i)})
	}
	require.Len(t, report.Risks, 1)
	require.Zero(t, report.flushedRisks)

	// Unique findings still reach the snapshot threshold and reset the baseline.
	threshold := NewReport(IRifyReportType)
	for i := 0; i < reportFlushEvery; i++ {
		threshold.AddRisks(&Risk{Hash: fmt.Sprintf("unique-%d", i)})
	}
	require.Len(t, threshold.Risks, reportFlushEvery)
	require.Equal(t, reportFlushEvery, threshold.flushedRisks)
}

// The same rule applies to risk updates: only growth in the document counts.
func TestReport_ApplyRiskUpdateCountsOnlyNewFindings(t *testing.T) {
	report := NewReport(IRifyReportType)
	item := schema.RiskUpdateItem{
		Mode: schema.RiskUpdateCreate,
		Risk: &schema.SSARisk{Hash: "one", Title: "one", RiskType: "demo"},
	}
	for i := 0; i < 5*reportFlushEvery; i++ {
		require.NoError(t, report.ApplyRiskUpdate(item))
	}
	require.Len(t, report.Risks, 1)
	require.Zero(t, report.flushedRisks)

	// Replacing a risk under a new hash keeps the document the same size, so
	// it must not masquerade as newly collected findings.
	replacement := schema.RiskUpdateItem{
		Mode:    schema.RiskUpdateUpdate,
		OldHash: "one",
		Risk:    &schema.SSARisk{Hash: "two", Title: "two", RiskType: "demo"},
	}
	require.NoError(t, report.ApplyRiskUpdate(replacement))
	require.Len(t, report.Risks, 1)
	require.Zero(t, report.flushedRisks)

	threshold := NewReport(IRifyReportType)
	for i := 0; i < reportFlushEvery; i++ {
		require.NoError(t, threshold.ApplyRiskUpdate(schema.RiskUpdateItem{
			Mode: schema.RiskUpdateCreate,
			Risk: &schema.SSARisk{Hash: fmt.Sprintf("new-%d", i), Title: "new", RiskType: "demo"},
		}))
	}
	require.Len(t, threshold.Risks, reportFlushEvery)
	require.Equal(t, reportFlushEvery, threshold.flushedRisks)
}

// A create/delete churn that keeps the document size steady must not publish
// another snapshot: the scan rewrites one feature many times, and each rewrite
// used to serialize the whole 408MB document.
func TestReport_RewriteChurnDoesNotFlush(t *testing.T) {
	report := NewReport(IRifyReportType)
	for i := 0; i < reportFlushEvery; i++ {
		require.NoError(t, report.ApplyRiskUpdate(schema.RiskUpdateItem{
			Mode: schema.RiskUpdateCreate,
			Risk: &schema.SSARisk{Hash: fmt.Sprintf("hash-0-%d", i), Title: "base", RiskType: "demo"},
		}))
	}
	require.Equal(t, reportFlushEvery, report.flushedRisks)

	// Rewrite every finding under a new hash: document size is unchanged.
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
	require.Equal(t, reportFlushEvery, report.flushedRisks,
		"rewrites must not move the snapshot baseline")
}
