package syntaxflow_scan

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// A failing detection stage after a successful sibling keeps the run
// successful: the product rule is "any successful detection stage wins".
func TestStageOutcomeRecorder_AnyDetectionStageSucceeds(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.record(StageCollect, nil)
	recorder.record(StageInspect, nil)
	recorder.record(StageAnalyze, fmt.Errorf("ssa rule set failed"))

	require.True(t, recorder.Succeeded(), "one successful detection stage must win")

	var failed []StageOutcome
	for _, outcome := range recorder.Outcomes() {
		if !outcome.Succeeded() {
			failed = append(failed, outcome)
		}
	}
	require.Len(t, failed, 1)
	require.Equal(t, StageAnalyze, failed[0].Stage)
	require.Equal(t, StageStatusFailed, failed[0].Status)
	require.NotEmpty(t, failed[0].Error)
}

// Collect alone is not a successful scan.
func TestStageOutcomeRecorder_CollectAloneIsNotSuccess(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.record(StageCollect, nil)
	recorder.record(StageInspect, fmt.Errorf("source rule set failed"))

	require.False(t, recorder.Succeeded())
}

// Compile-only success still requires collect plus the compile stage.
func TestStageOutcomeRecorder_CompileOnlySuccess(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.record(StageCollect, nil)
	recorder.record(StageCompile, nil)

	require.True(t, recorder.Succeeded())
	require.Equal(t, "编译源码", StageCompile.DisplayName())
	require.Equal(t, "compile", StageCompile.LegacyPhase())
}

// No mode at all resolves to compile-only; explicit modes stay as selected.
func TestResolveProductModes(t *testing.T) {
	compileOnly := resolveProductModes(&Config{})
	require.True(t, compileOnly.compileOnly)
	require.False(t, compileOnly.source)
	require.False(t, compileOnly.review)
	require.False(t, compileOnly.analyze)

	noMode := resolveProductModes(&Config{ScanTaskCallback: &ScanTaskCallback{}})
	require.True(t, noMode.compileOnly)

	cfg := &Config{ScanTaskCallback: &ScanTaskCallback{}}
	cfg.scanModes = []string{SourceMode, SSAMode}
	selected := resolveProductModes(cfg)
	require.False(t, selected.compileOnly)
	require.True(t, selected.source)
	require.False(t, selected.review)
	require.True(t, selected.analyze)
}

// Unknown mode values must not silently degrade into compile-only.
func TestResolveProductModes_UnknownValueFallsBackToAll(t *testing.T) {
	cfg := &Config{ScanTaskCallback: &ScanTaskCallback{}}
	cfg.scanModes = []string{"not-a-mode"}
	selected := resolveProductModes(cfg)
	require.False(t, selected.compileOnly)
	require.True(t, selected.source)
	require.True(t, selected.review)
	require.True(t, selected.analyze)
}

// A stage outcome carries the evidence a platform renders: status, error,
// wall time, and the rule/risk counters observed while the stage ran.
func TestStageOutcomeRecorder_CarriesStageMetrics(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.enter(StageInspect)
	recorder.observe(StageInspect, &RuleProcessInfoList{TotalQuery: 12, RiskCount: 3})
	recorder.record(StageInspect, fmt.Errorf("rule set aborted"))

	require.Len(t, recorder.Outcomes(), 1)
	outcome := recorder.Outcomes()[0]
	require.Equal(t, StageInspect, outcome.Stage)
	require.Equal(t, StageStatusFailed, outcome.Status)
	require.Equal(t, "rule set aborted", outcome.Error)
	require.EqualValues(t, 12, outcome.RuleCount)
	require.EqualValues(t, 3, outcome.RiskCount)
}

// Streamed risks are attributed to the stage that produced them, and counted
// cumulatively when a stage streams several batches.
func TestStageOutcomeRecorder_AccumulatesStreamedRisks(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.enter(StageAnalyze)
	recorder.addRisk(StageAnalyze, 2)
	recorder.addRisk(StageAnalyze, 5)
	recorder.record(StageAnalyze, nil)

	outcome := recorder.Outcomes()[0]
	require.True(t, outcome.Succeeded())
	require.EqualValues(t, 7, outcome.RiskCount)
}
