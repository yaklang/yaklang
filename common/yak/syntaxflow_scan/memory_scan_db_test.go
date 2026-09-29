package syntaxflow_scan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// countSSARisksForProgram counts every risk row the database holds for a
// program, whichever stage produced it.
func countSSARisksForProgram(t *testing.T, programName string) int64 {
	t.Helper()
	var count int64
	err := ssadb.GetDB().Model(&schema.SSARisk{}).
		Where("program_name = ?", programName).
		Count(&count).Error
	require.NoError(t, err)
	return count
}

// TestScanProject_MemoryScanWritesNoDatabaseRows proves the memory scan path
// simply drops the database handling: findings still reach the report and the
// callbacks, but neither an audit row nor a risk row is written. The struct
// stage runs inside the compile, so this also covers the path that used to
// save its results directly.
func TestScanProject_MemoryScanWritesNoDatabaseRows(t *testing.T) {
	const programName = "memory-scan-no-db-rows"
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
	})

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "main.go"),
		[]byte(coverProgramSource),
		0o644,
	))

	report := sfreport.NewReport(sfreport.IRifyReportType)
	var streamedRisks int64
	_, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("golang"),
		ssaconfig.WithSetProgramName(programName),
		ssaconfig.WithScanIgnoreLanguage(true),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "struct", language: golang, title: "memory scan")
sink(* as $arg) as $call;
alert $call`,
			Language: "golang",
		}),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "ssa", language: golang, title: "memory scan")
sink(* as $arg) as $call;
alert $call`,
			Language: "golang",
		}),
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode),
		syntaxflow_scan.WithReporter(report),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				streamedRisks += int64(r.Result.RiskCount())
			}
		}),
	)
	require.NoError(t, err)

	require.Greater(t, streamedRisks, int64(0), "the scan still delivers its findings")
	require.Len(t, report.Risks, 1, "the report keeps one entry per finding")
	require.Equal(t, int64(0), countAuditRowsForProgram(t, programName),
		"a memory scan must not write audit result rows")
	require.Equal(t, int64(0), countSSARisksForProgram(t, programName),
		"a memory scan must not write risk rows")
}
