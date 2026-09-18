package syntaxflow_scan

import (
	"encoding/json"
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
func TestProductStageRankDoesNotRegress(t *testing.T) {
	require.Greater(t, ProductStageRank(StageReview), ProductStageRank(StageInspect))
	require.Greater(t, ProductStageRank(StageAnalyze), ProductStageRank(StageReview))
	require.Equal(t, ProductStageRank(StageReview), ProductStageRank(StageCompile))
}

func TestStageOutcomeJSONKeepsZeroDetectionCounts(t *testing.T) {
	raw, err := json.Marshal(StageOutcome{Stage: StageInspect, Status: StageStatusSucceeded})
	require.NoError(t, err)
	require.Contains(t, string(raw), `"rule_count":0`)
	require.Contains(t, string(raw), `"risk_count":0`)
}

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
func TestParseCompileScale(t *testing.T) {
	info := parseCompileScale(`ssa-compile-scale:{"total_files":12,"handler_files":8,"prehandler_files":12,"total_bytes":4096}`)
	require.NotNil(t, info)
	require.EqualValues(t, 12, info.TotalFiles)
	require.EqualValues(t, 8, info.HandlerFiles)
	require.EqualValues(t, 4096, info.TotalBytes)
	require.Nil(t, parseCompileScale("compiling php"))
}

func TestStageOutcomeRecorder_ObserveScale(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.observeScale(&RuleProcessInfoList{TotalFiles: 12, TotalBytes: 4096, TotalLines: 80})
	require.EqualValues(t, 12, recorder.scale.TotalFiles)
	require.EqualValues(t, 4096, recorder.scale.TotalBytes)
	require.EqualValues(t, 80, recorder.scale.TotalLines)
}

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

func TestSkippedRequestedStages_DetectsUnstartedDetection(t *testing.T) {
	mode := productModeSelection{source: true, review: true, analyze: true}
	skipped := skippedRequestedStages(mode, []StageOutcome{
		{Stage: StageCollect, Status: StageStatusSucceeded},
		{Stage: StageInspect, Status: StageStatusSucceeded},
		{Stage: StageReview, Status: StageStatusSucceeded},
	})
	require.Equal(t, []string{string(StageAnalyze)}, skipped)

	none := skippedRequestedStages(productModeSelection{source: true}, []StageOutcome{
		{Stage: StageInspect, Status: StageStatusFailed, Error: "rule failed"},
	})
	require.Empty(t, none)
}

func TestMergeStageInfoKeepsCompileScaleAndStructRules(t *testing.T) {
	scale := &RuleProcessInfoList{TotalFiles: 12, TotalBytes: 4096, TotalLines: 80}
	rules := &RuleProcessInfoList{
		TotalQuery:    2,
		FinishedQuery: 2,
		SuccessQuery:  2,
		RiskCount:     3,
		Rules:         []*RuleProcessInfo{{RuleName: "php-struct", Finished: true, RiskCount: 3}},
	}
	got := mergeStageInfo(scale, rules)
	require.EqualValues(t, 12, got.TotalFiles)
	require.EqualValues(t, 80, got.TotalLines)
	require.EqualValues(t, 2, got.TotalQuery)
	require.EqualValues(t, 3, got.RiskCount)
	require.Len(t, got.Rules, 1)
	require.Equal(t, "php-struct", got.Rules[0].RuleName)
}
