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
	require.Equal(t, 1, report.sinceFlush, "only the first add is a new finding")

	// A replacement risk under the same hash does not grow the document.
	for i := 0; i < reportFlushEvery; i++ {
		report.AddRisks(&Risk{Hash: "same", Title: fmt.Sprintf("body-%d", i)})
	}
	require.Equal(t, 1, report.sinceFlush)

	// Unique findings still reach the snapshot threshold and reset it.
	threshold := NewReport(IRifyReportType)
	for i := 0; i < reportFlushEvery; i++ {
		threshold.AddRisks(&Risk{Hash: fmt.Sprintf("unique-%d", i)})
	}
	require.Len(t, threshold.Risks, reportFlushEvery)
	require.Zero(t, threshold.sinceFlush)
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
	require.Equal(t, 1, report.sinceFlush)

	// Replacing a risk under a new hash keeps the count unchanged and must not
	// masquerade as a newly collected finding.
	replacement := schema.RiskUpdateItem{
		Mode:    schema.RiskUpdateUpdate,
		OldHash: "one",
		Risk:    &schema.SSARisk{Hash: "two", Title: "two", RiskType: "demo"},
	}
	require.NoError(t, report.ApplyRiskUpdate(replacement))
	require.Len(t, report.Risks, 1)
	require.Equal(t, 1, report.sinceFlush)

	threshold := NewReport(IRifyReportType)
	for i := 0; i < reportFlushEvery; i++ {
		require.NoError(t, threshold.ApplyRiskUpdate(schema.RiskUpdateItem{
			Mode: schema.RiskUpdateCreate,
			Risk: &schema.SSARisk{Hash: fmt.Sprintf("new-%d", i), Title: "new", RiskType: "demo"},
		}))
	}
	require.Len(t, threshold.Risks, reportFlushEvery)
	require.Zero(t, threshold.sinceFlush)
}
