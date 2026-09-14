package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func newCoverageTestState() *AuditState {
	state := NewAuditState()
	state.AddScanObservation(&ScanObservation{
		CategoryID:   "sql-injection",
		CategoryName: "SQL注入",
		StopReason:   "files_all_audited",
		Status:       ScanStatusCompleted,
		AuditedFiles: 5,
		TargetFiles:  5,
	})
	state.AddScanObservation(&ScanObservation{
		CategoryID:   "race-condition",
		CategoryName: "竞态条件",
		StopReason:   "auto_finalized_on_timeout",
		Status:       ScanStatusPartial,
		AuditedFiles: 1,
		TargetFiles:  3,
	})
	state.AddScanObservation(&ScanObservation{
		CategoryID:   "resource-exhaustion",
		CategoryName: "资源耗尽",
		StopReason:   "not_run_interrupted",
		Status:       ScanStatusNotRun,
	})
	return state
}

func TestIncompleteScanObservations(t *testing.T) {
	state := newCoverageTestState()
	incomplete := IncompleteScanObservations(state)
	require.Len(t, incomplete, 2)
	require.Equal(t, "race-condition", incomplete[0].CategoryID)
	require.Equal(t, "resource-exhaustion", incomplete[1].CategoryID)
}

func TestBuildCategoryCoverageTable_All(t *testing.T) {
	state := newCoverageTestState()
	table := BuildCategoryCoverageTable(state, true)
	require.Contains(t, table, "SQL注入")
	require.Contains(t, table, "竞态条件")
	require.Contains(t, table, "资源耗尽")
	require.Contains(t, table, "1/3")
	require.Contains(t, table, "auto_finalized_on_timeout")
	require.NotContains(t, table, "未知")
}

func TestBuildCategoryCoverageTable_IncompleteOnly(t *testing.T) {
	state := newCoverageTestState()
	table := BuildCategoryCoverageTable(state, false)
	require.NotContains(t, table, "SQL注入", "completed categories must be excluded in the incomplete-only table")
	require.Contains(t, table, "竞态条件")
	require.Contains(t, table, "资源耗尽")
}

func TestBuildCategoryCoverageTable_LegacyEmptyStatus(t *testing.T) {
	// 旧版本快照恢复的 observation 无 Status 字段：保守视为未完成。
	state := NewAuditState()
	state.AddScanObservation(&ScanObservation{CategoryID: "legacy-cat", CategoryName: "旧类别"})
	incomplete := IncompleteScanObservations(state)
	require.Len(t, incomplete, 1)
	table := BuildCategoryCoverageTable(state, false)
	require.Contains(t, table, "未知（状态缺失）")
}

func TestAppendMissingCoverageAppendix_NoIncomplete(t *testing.T) {
	state := NewAuditState()
	state.AddScanObservation(&ScanObservation{
		CategoryID: "sql-injection", Status: ScanStatusCompleted, AuditedFiles: 2, TargetFiles: 2,
	})
	out, appended := AppendMissingCoverageAppendix("报告正文", state)
	require.False(t, appended)
	require.Equal(t, "报告正文", out)
}

func TestAppendMissingCoverageAppendix_WithIncomplete(t *testing.T) {
	state := newCoverageTestState()
	out, appended := AppendMissingCoverageAppendix("报告正文", state)
	require.True(t, appended)
	require.True(t, strings.HasPrefix(out, "报告正文\n\n---"))
	require.Contains(t, out, "附录：Phase2 类别扫描覆盖状态")
	require.Contains(t, out, "竞态条件")
	require.Contains(t, out, "资源耗尽")
	require.NotContains(t, out, "SQL注入|")
}

func TestAppendMissingCoverageAppendix_EmptyState(t *testing.T) {
	state := NewAuditState()
	out, appended := AppendMissingCoverageAppendix("报告正文", state)
	require.False(t, appended)
	require.Equal(t, "报告正文", out)
}

func TestScanObservationJSONRoundTrip(t *testing.T) {
	// 持久化兼容：新字段带 omitempty，旧 JSON 反序列化不破。
	obs := &ScanObservation{
		CategoryID:   "race-condition",
		Status:       ScanStatusPartial,
		AuditedFiles: 1,
		TargetFiles:  3,
	}
	data, err := json.Marshal(obs)
	require.NoError(t, err)
	require.Contains(t, string(data), `"status":"partial"`)
	require.Contains(t, string(data), `"audited_files":1`)
	require.Contains(t, string(data), `"target_files":3`)

	var restored ScanObservation
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, ScanStatusPartial, restored.Status)
	require.Equal(t, 1, restored.AuditedFiles)
	require.Equal(t, 3, restored.TargetFiles)

	// 旧格式（无新字段）反序列化成功且零值。
	legacy := []byte(`{"category_id":"x","stop_reason":"files_all_audited"}`)
	var legacyObs ScanObservation
	require.NoError(t, json.Unmarshal(legacy, &legacyObs))
	require.Equal(t, "", legacyObs.Status)
	require.Equal(t, 0, legacyObs.TargetFiles)
}