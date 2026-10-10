package syntaxflow_scan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const coverProgramSource = `package main

func run(cmd string) {
	sink(cmd)
}

func sink(any) {}
`

// TestScanProject_StructFindingIsCoveredBySSA is the end-to-end cover: the
// same call is reported by a struct rule and by an SSA rule of the following
// stage, and the report keeps one entry, the later mode's.
func TestScanProject_StructFindingIsCoveredBySSA(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "main.go"),
		[]byte(coverProgramSource),
		0o644,
	))

	report := sfreport.NewReport(sfreport.IRifyReportType)
	_, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("golang"),
		ssaconfig.WithSetProgramName("cover-e2e-program"),
		ssaconfig.WithScanIgnoreLanguage(true),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "struct", language: golang, title: "cover e2e")
sink(* as $arg) as $call;
alert $call`,
			Language: "golang",
		}),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "ssa", language: golang, title: "cover e2e")
sink(* as $arg) as $call;
alert $call`,
			Language: "golang",
		}),
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode),
		syntaxflow_scan.WithReporter(report),
	)
	require.NoError(t, err)

	require.Len(t, report.Risks, 1, "the report keeps one entry per finding")
	for _, risk := range report.Risks {
		require.Equal(t, string(schema.SFR_MODE_SSA), risk.ScanMode,
			"the later scan mode covers the earlier finding")
	}
}

// TestScanProject_SuppliedRuntimeStillCovers proves a caller-supplied runtime
// is wired to the report as well: the runtime is the scan's control point, so
// the report must receive its decisions even when the caller created it.
func TestScanProject_SuppliedRuntimeStillCovers(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "main.go"),
		[]byte(coverProgramSource),
		0o644,
	))

	report := sfreport.NewReport(sfreport.IRifyReportType)
	rt := ssaapi.NewScanRuntime()
	_, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("golang"),
		ssaconfig.WithSetProgramName("cover-supplied-runtime"),
		ssaconfig.WithScanIgnoreLanguage(true),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "struct", language: golang, title: "cover supplied")
sink(* as $arg) as $call;
alert $call`,
			Language: "golang",
		}),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "ssa", language: golang, title: "cover supplied")
sink(* as $arg) as $call;
alert $call`,
			Language: "golang",
		}),
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode),
		syntaxflow_scan.WithReporter(report),
		syntaxflow_scan.WithScanRuntime(rt),
	)
	require.NoError(t, err)
	require.Len(t, report.Risks, 1, "the report still applies the supplied runtime's decisions")
}
