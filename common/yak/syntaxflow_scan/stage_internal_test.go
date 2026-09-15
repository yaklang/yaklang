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
