package sfreport

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReportAddRisks_LaterModeCoversEarlier(t *testing.T) {
	report := NewReport(IRifyReportType)
	report.AddRisks(&Risk{
		Hash:            "source-hash",
		ScanMode:        "source",
		RiskFeatureHash: "feature-mysql",
		RiskType:        "sql-injection",
		CodeSourceURL:   "a.php",
		Line:            4,
		ProgramName:     "p",
		Title:           "source",
	})
	report.AddRisks(&Risk{
		Hash:            "ssa-hash",
		ScanMode:        "ssa",
		RiskFeatureHash: "feature-ir",
		RiskType:        "sql-injection",
		CodeSourceURL:   "a.php",
		Line:            4,
		ProgramName:     "p",
		Title:           "ssa",
	})

	require.Nil(t, report.GetRisk("source-hash"))
	got := report.GetRisk("ssa-hash")
	require.NotNil(t, got)
	require.Equal(t, "ssa", got.ScanMode)
	require.Equal(t, 1, report.RiskNums)
}

func TestReportAddRisks_UnfinishedLaterStageKeepsFloor(t *testing.T) {
	report := NewReport(IRifyReportType)
	report.AddRisks(&Risk{
		Hash:            "struct-hash",
		ScanMode:        "struct",
		RiskFeatureHash: "feature-const",
		RiskType:        "sql-injection",
		CodeSourceURL:   "a.php",
		Line:            8,
		ProgramName:     "p",
	})
	require.NotNil(t, report.GetRisk("struct-hash"))
	require.Equal(t, 1, report.RiskNums)
}
