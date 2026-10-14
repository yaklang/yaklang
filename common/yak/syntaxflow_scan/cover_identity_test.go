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

func TestScanProject_SameFeatureHashCoversAcrossModes(t *testing.T) {
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
	// Both calls disassemble to the same feature, so they are one finding.
	// The ssa rule covers the struct rule because that feature hash matches.
	require.Len(t, report.Risks, 1)
	for _, risk := range report.Risks {
		require.Equal(t, string(schema.SFR_MODE_SSA), risk.ScanMode)
	}
}
