package attacks

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnalyzeIR_ExtractsFunctionNames(t *testing.T) {
	ir := `
define i64 @check() {
entry:
  ret i64 42
}

define i64 @helper() {
entry:
  ret i64 1
}

define i32 @main() {
entry:
  ret i32 0
}
`
	result := AnalyzeIR(ir)
	require.Contains(t, result.FunctionNames, "check")
	require.Contains(t, result.FunctionNames, "helper")
	require.Contains(t, result.FunctionNames, "main")
}

func TestAnalyzeIR_ExtractsCallEdges(t *testing.T) {
	ir := `
define i64 @check() {
entry:
  %0 = call i64 @helper()
  ret i64 %0
}

define i64 @helper() {
entry:
  ret i64 42
}
`
	result := AnalyzeIR(ir)
	found := false
	for _, edge := range result.CallEdges {
		if edge[1] == "helper" {
			found = true
			break
		}
	}
	require.True(t, found, "should find call edge to helper")
}

func TestAnalyzeIR_CountsBasicBlocks(t *testing.T) {
	ir := `
define i64 @check() {
entry:
  br i1 true, label %then, label %else

then:
  br label %merge

else:
  br label %merge

merge:
  ret i64 0
}
`
	result := AnalyzeIR(ir)
	require.GreaterOrEqual(t, result.DistinctBlocks, 3)
}

func TestEvaluateBaseline(t *testing.T) {
	ir := `
define i64 @one() {
entry:
  ret i64 40
}

define i64 @two() {
entry:
  ret i64 2
}

define i64 @check() {
entry:
  %0 = call i64 @one()
  %1 = call i64 @two()
  %2 = add i64 %0, %1
  ret i64 %2
}

define i32 @main() {
entry:
  %r = call i64 @check()
  %exit = trunc i64 %r to i32
  ret i32 %exit
}
`
	result := AnalyzeIR(ir)
	report := Evaluate("baseline", result)

	require.NotNil(t, report)
	require.Equal(t, "baseline", report.Label)
	require.True(t, len(report.Metrics) >= 4, "should have at least 4 metrics")

	// Baseline IR should be fully exposed: function names and call graph visible.
	for _, m := range report.Metrics {
		t.Logf("  %s: %.2f", m.Name, m.Value)
	}

	// Function names should be recoverable (score 0.0).
	nameMetric := findMetric(report, "recover-function-names")
	require.NotNil(t, nameMetric)
	require.Equal(t, 0.0, nameMetric.Value, "baseline should have visible function names")

	// Call graph should be recoverable (score 0.0).
	callMetric := findMetric(report, "recover-call-graph")
	require.NotNil(t, callMetric)
	require.Equal(t, 0.0, callMetric.Value, "baseline should have visible call graph")
}

func TestThreatLevelString(t *testing.T) {
	require.Equal(t, "static", ThreatStatic.String())
	require.Equal(t, "symbolic", ThreatSymbolic.String())
	require.Equal(t, "dynamic", ThreatDynamic.String())
}

func findMetric(report *Report, name string) *Metric {
	for i := range report.Metrics {
		if report.Metrics[i].Name == name {
			return &report.Metrics[i]
		}
	}
	return nil
}
