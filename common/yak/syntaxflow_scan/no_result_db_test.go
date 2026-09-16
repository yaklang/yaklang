package syntaxflow_scan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const noResultDBTestProgram = `package main

func run(cmd string) {
	sink(cmd)
}

func sink(any) {}
`

func noResultDBTestRule() *ypb.SyntaxFlowRuleInput {
	return &ypb.SyntaxFlowRuleInput{
		Content: `desc(mode: "ssa", language: golang, title: "no-result-db test")
	sink(* as $target);
	alert $target`,
		Language: "golang",
	}
}

// countAuditResultsForProgram counts how many rule-result rows the scan wrote
// for this program.
func countAuditResultsForProgram(t *testing.T, programName string) int64 {
	t.Helper()
	var count int64
	err := ssadb.GetDB().Model(&ssadb.AuditResult{}).
		Where("program_name = ?", programName).
		Count(&count).Error
	require.NoError(t, err)
	return count
}

// countSSARisksForRuntime counts risks attributed to this scan run.
func countSSARisksForRuntime(t *testing.T, runtimeID string) int64 {
	t.Helper()
	var count int64
	err := ssadb.GetDB().Model(&schema.SSARisk{}).
		Where("runtime_id = ?", runtimeID).
		Count(&count).Error
	require.NoError(t, err)
	return count
}

func runNoResultDBScan(t *testing.T, programName string, noResultDB bool) (risks int64, auditRows int64) {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "main.go"),
		[]byte(noResultDBTestProgram),
		0o644,
	))

	var runtimeID string
	err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("golang"),
		ssaconfig.WithSetProgramName(programName),
		ssaconfig.WithRuleInput(noResultDBTestRule()),
		ssaconfig.WithScanIgnoreLanguage(true),
		ssaconfig.WithSyntaxFlowNoResultDB(noResultDB),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.TaskID != "" {
				runtimeID = r.TaskID
			}
		}),
	)
	require.NoError(t, err)

	auditRows = countAuditResultsForProgram(t, programName)
	if runtimeID != "" {
		risks = countSSARisksForRuntime(t, runtimeID)
	}
	return risks, auditRows
}

// TestScanProject_NoResultDB_KeepsDatabaseUntouched is the regression guard for
// scanning a database that only holds IR: with the flag on, the scan must not
// write rule results back into that database.
func TestScanProject_NoResultDB_KeepsDatabaseUntouched(t *testing.T) {
	const programName = "no-result-db-test-program"
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
	})

	risks, auditRows := runNoResultDBScan(t, programName, true)
	require.Zero(t, risks, "no SSA risk rows may be written when results stay out of the database")
	require.Zero(t, auditRows, "no rule-result rows may be written when results stay out of the database")
}
