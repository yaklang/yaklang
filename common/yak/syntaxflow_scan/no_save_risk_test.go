package syntaxflow_scan_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const noSaveRiskTestProgram = `package main

func run(cmd string) {
	sink(cmd)
}

func sink(any) {}
`

// noSaveRiskStructProgram calls a concrete API so a struct-mode rule can match
// it: the struct stage matches real call sites inside the compile unit.
const noSaveRiskStructProgram = `package main

import "net/http"

func run() {
	http.ListenAndServe(":80", nil)
}
`

func noSaveRiskTestRule() *ypb.SyntaxFlowRuleInput {
	return &ypb.SyntaxFlowRuleInput{
		Content: `desc(mode: "ssa", language: golang, title: "no-save-risk test")
	sink(* as $target);
	alert $target`,
		Language: "golang",
	}
}

// noSaveRiskStructRule is the struct-stage counterpart. The struct stage runs
// inside the compile unit and reaches the final result-save path separately
// from the SSA stage.
func noSaveRiskStructRule() *ypb.SyntaxFlowRuleInput {
	return &ypb.SyntaxFlowRuleInput{
		Content: `desc(mode: "struct", language: golang, title: "no-save-risk struct test")
	http.ListenAndServe as $call;
	http.ListenAndServe(* as $a) as $call;
	alert $call`,
		Language: "golang",
	}
}

// countAuditResultsForProgram counts how many rule-result rows the scan wrote
// for this program.
func countAuditRowsForProgram(t *testing.T, programName string) int64 {
	t.Helper()
	var resultCount int64
	err := ssadb.GetDB().Model(&ssadb.AuditResult{}).
		Where("program_name = ?", programName).
		Count(&resultCount).Error
	require.NoError(t, err)
	var nodeCount int64
	err = ssadb.GetDB().Model(&ssadb.AuditNode{}).
		Where("program_name = ?", programName).
		Count(&nodeCount).Error
	require.NoError(t, err)
	var edgeCount int64
	err = ssadb.GetDB().Model(&ssadb.AuditEdge{}).
		Where("program_name = ?", programName).
		Count(&edgeCount).Error
	require.NoError(t, err)
	return resultCount + nodeCount + edgeCount
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

func runNoSaveRiskScan(t *testing.T, programName string, noSaveRisk bool) (risks int64, auditRows int64, emittedRisks int64, task *schema.SyntaxFlowScanTask) {
	return runNoSaveRiskScanWithRule(t, programName, noSaveRisk, noSaveRiskTestRule())
}

// runNoSaveRiskScanWithRule runs one scan with the given rule and reports what
// it persisted. The struct stage takes its own path through the pipeline, so
// this takes the rule as a parameter.
func runNoSaveRiskScanWithRule(t *testing.T, programName string, noSaveRisk bool, rule *ypb.SyntaxFlowRuleInput) (risks int64, auditRows int64, emittedRisks int64, task *schema.SyntaxFlowScanTask) {
	return runNoSaveRiskScanWithSource(t, programName, noSaveRisk, rule, noSaveRiskTestProgram)
}

// runNoSaveRiskScanWithSource is the same scan with an explicit program body,
// so a struct-mode rule can supply code its pattern actually matches.
func runNoSaveRiskScanWithSource(t *testing.T, programName string, noSaveRisk bool, rule *ypb.SyntaxFlowRuleInput, src string) (risks int64, auditRows int64, emittedRisks int64, task *schema.SyntaxFlowScanTask) {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "main.go"),
		[]byte(src),
		0o644,
	))

	var runtimeID string
	var streamedRiskCount atomic.Int64
	_, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("golang"),
		ssaconfig.WithSetProgramName(programName),
		ssaconfig.WithRuleInput(rule),
		ssaconfig.WithScanIgnoreLanguage(true),
		ssaconfig.WithNoSaveRisk(noSaveRisk),
		// Without a mode the pipeline is compile-only and runs no rules at all.
		// The struct rule needs the struct stage explicitly selected to reach
		// the persistence path this test guards.
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.TaskID != "" {
				runtimeID = r.TaskID
			}
			if r != nil && r.Result != nil {
				streamedRiskCount.Add(int64(r.Result.RiskCount()))
			}
		}),
	)
	require.NoError(t, err)

	auditRows = countAuditRowsForProgram(t, programName)
	if runtimeID == "" {
		var stored schema.SyntaxFlowScanTask
		err = ssadb.GetDB().Where("programs LIKE ?", "%"+programName+"%").Order("id DESC").First(&stored).Error
		require.NoError(t, err)
		task = &stored
		runtimeID = stored.TaskId
	} else {
		task, err = schema.GetSyntaxFlowScanTaskById(ssadb.GetDB(), runtimeID)
		require.NoError(t, err)
	}
	risks = countSSARisksForRuntime(t, runtimeID)
	return risks, auditRows, streamedRiskCount.Load(), task
}

// TestScanProject_NoSaveRisk_PreservesTaskWithoutResultRows verifies that task
// state is saved while risks and audit graph data are not.
func TestScanProject_NoSaveRisk_PreservesTaskWithoutResultRows(t *testing.T) {
	const programName = "no-save-risk-test-program"
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
	})

	risks, auditRows, emittedRisks, task := runNoSaveRiskScan(t, programName, true)
	require.Zero(t, risks, "no SSA risk rows may be written when results stay out of the database")
	require.Zero(t, auditRows, "no audit result, node, or edge rows may be written")
	require.NotNil(t, task)
	require.NotEmpty(t, task.TaskId)
	require.Equal(t, schema.SYNTAXFLOWSCAN_DONE, task.Status)
	require.Greater(t, emittedRisks, int64(0))
	require.Equal(t, emittedRisks, task.RiskCount)
}

// TestScanProject_NoSaveRisk_StructStageUsesFinalSaveGuard covers the struct
// stage, which saves after program metadata is available.
func TestScanProject_NoSaveRisk_StructStageUsesFinalSaveGuard(t *testing.T) {
	const programName = "no-save-risk-struct-test-program"
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
	})

	risks, auditRows, _, task := runNoSaveRiskScanWithSource(t, programName, true, noSaveRiskStructRule(), noSaveRiskStructProgram)
	require.Zero(t, risks, "the struct stage may not write risk rows when results stay out of the database")
	require.Zero(t, auditRows, "the struct stage may not write audit result, node, or edge rows")
	require.NotNil(t, task)
	require.NotEmpty(t, task.TaskId)
	require.Equal(t, schema.SYNTAXFLOWSCAN_DONE, task.Status)
}
