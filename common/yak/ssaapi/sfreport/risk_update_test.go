package sfreport

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func coveredRisk(hash, feature, mode string) *schema.SSARisk {
	return &schema.SSARisk{
		Hash:            hash,
		RiskFeatureHash: feature,
		ScanMode:        mode,
		FromRule:        "rule-" + mode,
		Title:           "finding",
		TitleVerbose:    "finding",
		RiskType:        "custom",
		Severity:        schema.SFR_SEVERITY_HIGH,
		ProgramName:     "cover-program",
		CodeSourceUrl:   "file:///main.go",
		CodeRange:       "1:1-1:2",
	}
}

// TestReport_ApplyRiskUpdateReplacesCoveredBody proves one update both drops
// the earlier body and records the later one.
func TestReport_ApplyRiskUpdateReplacesCoveredBody(t *testing.T) {
	report := NewReport(IRifyReportType)
	source := coveredRisk("hash-source", "feature-1", string(schema.SFR_MODE_SOURCE))
	require.NoError(t, report.ApplyRiskUpdate(schema.RiskUpdateItem{Risk: source}))
	require.Len(t, report.Risks, 1)

	ssa := coveredRisk("hash-ssa", "feature-1", string(schema.SFR_MODE_SSA))
	require.NoError(t, report.ApplyRiskUpdate(schema.RiskUpdateItem{
		Risk:    ssa,
		OldID:   1,
		OldHash: source.Hash,
	}))
	require.Len(t, report.Risks, 1, "one entry per finding")
	require.Equal(t, 1, report.RiskNums)
	got := report.Risks[ssa.Hash]
	require.NotNil(t, got)
	require.Equal(t, string(schema.SFR_MODE_SSA), got.ScanMode)
}

// TestSarifReport_ApplyRiskUpdateReplacesCoveredResult proves one update
// replaces the earlier alert with the later mode's alert.
func TestSarifReport_ApplyRiskUpdateReplacesCoveredResult(t *testing.T) {
	report, err := NewSarifReport()
	require.NoError(t, err)

	source := coveredRisk("hash-source", "feature-1", string(schema.SFR_MODE_SOURCE))
	require.NoError(t, report.ApplyRiskUpdate(schema.RiskUpdateItem{Risk: source}))
	require.Len(t, report.run.Results, 1)

	ssa := coveredRisk("hash-ssa", "feature-1", string(schema.SFR_MODE_SSA))
	require.NoError(t, report.ApplyRiskUpdate(schema.RiskUpdateItem{
		Risk:    ssa,
		OldID:   1,
		OldHash: source.Hash,
	}))
	require.Len(t, report.run.Results, 1, "one alert per finding")
	fp, ok := report.run.Results[0].PartialFingerprints[SarifFingerprintKey].(string)
	require.True(t, ok)
	require.Equal(t, ssa.RiskFeatureHash, fp)
}

// TestReport_SourceFindingIsCoveredBySSA runs the two modes through the scan
// runtime's own decision and checks the report keeps one entry, the later
// mode's, for a source finding the deep scan covers.
func TestReport_SourceFindingIsCoveredBySSA(t *testing.T) {
	runtime := ssaapi.NewScanRuntime()
	report := NewReport(IRifyReportType)
	runtime.ListenRisk(report)

	source := coveredRisk("hash-source", "feature-cover", string(schema.SFR_MODE_SOURCE))
	require.True(t, runtime.SubmitRisk(source))
	require.Len(t, report.Risks, 1, "the source stage contributes its finding")

	ssa := coveredRisk("hash-ssa", "feature-cover", string(schema.SFR_MODE_SSA))
	require.True(t, runtime.SubmitRisk(ssa))

	require.Len(t, report.Risks, 1, "one entry per finding")
	for _, risk := range report.Risks {
		require.Equal(t, string(schema.SFR_MODE_SSA), risk.ScanMode)
		require.Equal(t, ssa.Hash, risk.Hash)
	}
}
