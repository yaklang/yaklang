package syntaxflow_scan_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestScan_DatabaseModeWritesResultAndRiskThroughSaver verifies the database
// consumer of the scan runtime: the query itself no longer writes, the saver
// writes the result row, the audit graph and the risk row.
func TestScan_DatabaseModeWritesResultAndRiskThroughSaver(t *testing.T) {
	const progID = "db-saver-test-program"
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.go", `package main

func run(cmd string) {
	sink(cmd)
}

func sink(any) {}
`)
	_, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithProgramName(progID),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	})

	var runtimeID string
	err = syntaxflow_scan.Scan(context.Background(),
		ssaconfig.WithScanControlMode(ssaconfig.ControlModeStart),
		ssaconfig.WithProgramNames(progID),
		ssaconfig.WithScanIgnoreLanguage(true),
		ssaconfig.WithSyntaxFlowResultKind(ssaconfig.SFResultSaveDatabase),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "ssa", language: golang, title: "db saver test")
sink(* as $arg) as $call;
alert $call`,
			Language: "golang",
		}),
		syntaxflow_scan.WithScanResultCallback(func(sr *syntaxflow_scan.ScanResult) {
			if sr != nil && sr.TaskID != "" {
				runtimeID = sr.TaskID
			}
		}),
	)
	require.NoError(t, err)
	require.NotEmpty(t, runtimeID)

	var resultCount int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.AuditResult{}).
		Where("program_name = ?", progID).Count(&resultCount).Error)
	require.Greater(t, resultCount, int64(0), "the result saver writes the result row")

	var riskCount int64
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).
		Where("runtime_id = ?", runtimeID).Count(&riskCount).Error)
	require.Equal(t, int64(1), riskCount, "the risk saver writes one row per finding")
}

// TestScan_DatabaseModeNoSaveRiskWritesNothing verifies the same path with the
// persistence switch off: findings are still emitted, nothing is stored.
func TestScan_DatabaseModeNoSaveRiskWritesNothing(t *testing.T) {
	const progID = "db-saver-nosave-program"
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.go", `package main

func run(cmd string) {
	sink(cmd)
}

func sink(any) {}
`)
	_, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithProgramName(progID),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	})

	var runtimeID string
	err = syntaxflow_scan.Scan(context.Background(),
		ssaconfig.WithScanControlMode(ssaconfig.ControlModeStart),
		ssaconfig.WithProgramNames(progID),
		ssaconfig.WithScanIgnoreLanguage(true),
		ssaconfig.WithSyntaxFlowResultKind(ssaconfig.SFResultSaveDatabase),
		ssaconfig.WithNoSaveRisk(true),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "ssa", language: golang, title: "db saver no-save test")
sink(* as $arg) as $call;
alert $call`,
			Language: "golang",
		}),
		syntaxflow_scan.WithScanResultCallback(func(sr *syntaxflow_scan.ScanResult) {
			if sr != nil && sr.TaskID != "" {
				runtimeID = sr.TaskID
			}
		}),
	)
	require.NoError(t, err)

	var resultCount int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.AuditResult{}).
		Where("program_name = ?", progID).Count(&resultCount).Error)
	require.Zero(t, resultCount, "no-save-risk keeps audit rows out of the database")

	var riskCount int64
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).
		Where("runtime_id = ?", runtimeID).Count(&riskCount).Error)
	require.Zero(t, riskCount, "no-save-risk keeps risk rows out of the database")
}

// TestScan_MemoryModeWritesNothing covers the memory scan: no database
// consumer is registered at all, so the query streams its findings to the
// callbacks and the database stays untouched. This is the scan-side
// equivalent of "no result database": the handling is dropped, not stubbed.
func TestScan_MemoryModeWritesNothing(t *testing.T) {
	const progID = "db-saver-memory-program"
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.go", `package main

func run(cmd string) {
	sink(cmd)
}

func sink(any) {}
`)
	_, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithProgramName(progID),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	})

	var runtimeID string
	var emittedRisks int64
	err = syntaxflow_scan.Scan(context.Background(),
		ssaconfig.WithScanControlMode(ssaconfig.ControlModeStart),
		ssaconfig.WithProgramNames(progID),
		ssaconfig.WithScanIgnoreLanguage(true),
		ssaconfig.WithSyntaxFlowResultKind(ssaconfig.SFResultSaveMemory),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "ssa", language: golang, title: "memory scan test")
sink(* as $arg) as $call;
alert $call`,
			Language: "golang",
		}),
		syntaxflow_scan.WithScanResultCallback(func(sr *syntaxflow_scan.ScanResult) {
			if sr == nil {
				return
			}
			if sr.TaskID != "" {
				runtimeID = sr.TaskID
			}
			if sr.Result != nil {
				emittedRisks += int64(sr.Result.RiskCount())
			}
		}),
	)
	require.NoError(t, err)
	require.NotEmpty(t, runtimeID)
	require.Greater(t, emittedRisks, int64(0), "the memory scan still emits its findings")

	var resultCount int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.AuditResult{}).
		Where("program_name = ?", progID).Count(&resultCount).Error)
	require.Zero(t, resultCount, "a memory scan writes no audit row")

	var riskCount int64
	require.NoError(t, ssadb.GetDB().Model(&schema.SSARisk{}).
		Where("runtime_id = ?", runtimeID).Count(&riskCount).Error)
	require.Zero(t, riskCount, "a memory scan writes no risk row")
}
