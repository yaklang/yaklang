package syntaxflow_scan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestScanProject_CoversModesWithoutMergingSiblingCalls(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main
func run(cmd string) {
    sink(cmd)
    sink(cmd)
}
func sink(any) {}`), 0o644))
	report := sfreport.NewReport(sfreport.IRifyReportType)
	_, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("golang"),
		ssaconfig.WithSetProgramName(uuid.NewString()),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{Language: "golang", Content: `
desc(mode: "struct", language: golang, title: "cover siblings")
sink(* as $arg) as $call
alert $call`}),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{Language: "golang", Content: `
desc(mode: "ssa", language: golang, title: "cover siblings")
sink(* as $arg) as $call
alert $call`}),
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode),
		syntaxflow_scan.WithReporter(report),
	)
	require.NoError(t, err)
	// Both calls share one feature hash, so the ssa rule of the following mode
	// covers each of them. They sit at different statements, which the cover key
	// carries, so each call keeps its own row.
	require.Len(t, report.Risks, 2, "different statements must stay separate findings")
	positions := make(map[string]struct{}, len(report.Risks))
	for _, risk := range report.Risks {
		require.Equal(t, string(schema.SFR_MODE_SSA), risk.ScanMode,
			"the later scan mode covers the earlier finding")
		positions[risk.CodeRange] = struct{}{}
	}
	require.Len(t, positions, 2, "each sibling statement keeps its own position")
}
