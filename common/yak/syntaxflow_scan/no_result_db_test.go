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

// noResultDBStructProgram calls a concrete API so a struct-mode rule can match
// it: the struct stage matches real call sites inside the compile unit.
const noResultDBStructProgram = `package main

import "net/http"

func run() {
	http.ListenAndServe(":80", nil)
}
`

func noResultDBTestRule() *ypb.SyntaxFlowRuleInput {
	return &ypb.SyntaxFlowRuleInput{
		Content: `desc(mode: "ssa", language: golang, title: "no-result-db test")
	sink(* as $target);
	alert $target`,
		Language: "golang",
	}
}

// noResultDBStructRule is the struct-stage counterpart. The struct stage runs
// inside the compile unit and persists its own audit rows and risks, so it
// needs the read-only flag independently of the ssa stage.
func noResultDBStructRule() *ypb.SyntaxFlowRuleInput {
	return &ypb.SyntaxFlowRuleInput{
		Content: `desc(mode: "struct", language: golang, title: "no-result-db struct test")
	http.ListenAndServe as $call;
	http.ListenAndServe(* as $a) as $call;
	alert $call`,
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
	return runNoResultDBScanWithRule(t, programName, noResultDB, noResultDBTestRule())
}

// runNoResultDBScanWithRule runs one scan with the given rule and reports what
// it persisted. The struct stage takes its own path through the pipeline, so
// this takes the rule as a parameter.
func runNoResultDBScanWithRule(t *testing.T, programName string, noResultDB bool, rule *ypb.SyntaxFlowRuleInput) (risks int64, auditRows int64) {
	return runNoResultDBScanWithSource(t, programName, noResultDB, rule, noResultDBTestProgram)
}

// runNoResultDBScanWithSource is the same scan with an explicit program body,
// so a struct-mode rule can supply code its pattern actually matches.
func runNoResultDBScanWithSource(t *testing.T, programName string, noResultDB bool, rule *ypb.SyntaxFlowRuleInput, src string) (risks int64, auditRows int64) {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "main.go"),
		[]byte(src),
		0o644,
	))

	var runtimeID string
	_, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("golang"),
		ssaconfig.WithSetProgramName(programName),
		ssaconfig.WithRuleInput(rule),
		ssaconfig.WithScanIgnoreLanguage(true),
		ssaconfig.WithSyntaxFlowNoResultDB(noResultDB),
		// Without a mode the pipeline is compile-only and runs no rules at all.
		// The struct rule needs the struct stage explicitly selected to reach
		// the persistence path this test guards.
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode),
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

// TestScanProject_NoResultDB_StructStageKeepsDatabaseUntouched covers the
// struct stage, which writes its own audit rows and risks from inside the
// compile unit. The ssa-stage flag does not reach it: the option has to be
// translated at the point the struct compile options are assembled.
func TestScanProject_NoResultDB_StructStageKeepsDatabaseUntouched(t *testing.T) {
	const programName = "no-result-db-struct-test-program"
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
	})

	risks, auditRows := runNoResultDBScanWithSource(t, programName, true, noResultDBStructRule(), noResultDBStructProgram)
	require.Zero(t, risks, "the struct stage may not write risk rows when results stay out of the database")
	require.Zero(t, auditRows, "the struct stage may not write rule-result rows when results stay out of the database")
}
