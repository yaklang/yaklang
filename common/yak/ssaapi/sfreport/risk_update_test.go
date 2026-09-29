package sfreport

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/sarif"
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

// TestReport_ApplyRiskUpdateDropsCoveredBody proves the report applies the
// scan's cover decision: the replaced finding leaves the report.
func TestReport_ApplyRiskUpdateDropsCoveredBody(t *testing.T) {
	report := NewReport(IRifyReportType)
	source := coveredRisk("hash-source", "feature-1", string(schema.SFR_MODE_SOURCE))
	report.AddRisks(&Risk{Hash: source.Hash, RiskFeatureHash: source.RiskFeatureHash})
	require.Len(t, report.Risks, 1)

	ssa := coveredRisk("hash-ssa", "feature-1", string(schema.SFR_MODE_SSA))
	require.NoError(t, report.ApplyRiskUpdate(schema.RiskUpdateItem{
		Risk:    ssa,
		OldID:   1,
		OldHash: source.Hash,
	}))
	require.Empty(t, report.Risks, "the covered body is dropped; the new body comes from its rich result")
	require.Zero(t, report.RiskNums)
}

// TestSarifReport_ApplyRiskUpdateDropsCoveredResult proves the SARIF document
// keeps one alert per finding when a later mode covers an earlier one.
func TestSarifReport_ApplyRiskUpdateDropsCoveredResult(t *testing.T) {
	report, err := NewSarifReport()
	require.NoError(t, err)

	source := coveredRisk("hash-source", "feature-1", string(schema.SFR_MODE_SOURCE))
	report.resultByHash = map[string]int{"feature-1": 0}
	report.run.Results = append(report.run.Results, sarif.NewRuleResult("test-rule").
		WithMessage(sarif.NewTextMessage("finding")).
		WithPartialFingerPrints(map[string]interface{}{
			SarifFingerprintKey: source.RiskFeatureHash,
		}))
	require.Len(t, report.run.Results, 1)

	ssa := coveredRisk("hash-ssa", "feature-1", string(schema.SFR_MODE_SSA))
	require.NoError(t, report.ApplyRiskUpdate(schema.RiskUpdateItem{
		Risk:    ssa,
		OldID:   1,
		OldHash: source.Hash,
	}))
	require.Empty(t, report.run.Results, "the covered alert is removed before the richer one is added")
}

// TestReport_SourceFindingIsCoveredBySSA runs the two modes through the scan
// runtime's own decision and checks the report keeps one entry, the later
// mode's, for a source finding the deep scan covers.
func TestReport_SourceFindingIsCoveredBySSA(t *testing.T) {
	runtime := ssaapi.NewScanRuntime()
	report := NewReport(IRifyReportType)
	report.SetKeeper(runtime.KeepRisk)
	runtime.ListenRisk(schema.RiskUpdateHandlerFunc(report.ApplyRiskUpdate))

	source := coveredRisk("hash-source", "feature-cover", string(schema.SFR_MODE_SOURCE))
	require.True(t, runtime.SubmitRisk(source))
	report.AddRisks(&Risk{Hash: source.Hash, RiskFeatureHash: source.RiskFeatureHash, ScanMode: source.ScanMode})
	require.Len(t, report.Risks, 1, "the source stage contributes its finding")

	ssa := coveredRisk("hash-ssa", "feature-cover", string(schema.SFR_MODE_SSA))
	require.True(t, runtime.SubmitRisk(ssa))
	require.Empty(t, report.Risks, "the cover decision drops the earlier body")

	// The richer result of the later stage adds its own body.
	report.AddRisks(&Risk{Hash: ssa.Hash, RiskFeatureHash: ssa.RiskFeatureHash, ScanMode: ssa.ScanMode})

	require.Len(t, report.Risks, 1, "one entry per finding")
	for _, risk := range report.Risks {
		require.Equal(t, string(schema.SFR_MODE_SSA), risk.ScanMode)
	}
}
