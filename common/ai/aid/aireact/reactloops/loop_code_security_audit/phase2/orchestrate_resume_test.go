package phase2

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

// newPhaseBInterruptedScanState 构造一个"阶段B中断"状态的 ScanState：
// 已 commit 3 个目标文件，其中 1 个已 mark，2 个未 mark（模拟 wall-clock
// timeout 在阶段B中途砍断）。
func newPhaseBInterruptedScanState(t *testing.T) *ScanState {
	t.Helper()
	scan := newScanState()
	scan.AddTargetFiles([]string{"/abs/app/a.php", "/abs/app/b.php", "/abs/app/c.php"})
	scan.CommitToAudit()
	markFileDoneForTest(scan, "/abs/app/a.php")
	return scan
}

// ── shouldResumeCategoryScanFromPhaseB ──

func TestShouldResumeCategoryScanFromPhaseB_InterruptedMidAudit(t *testing.T) {
	scan := newPhaseBInterruptedScanState(t)
	require.True(t, shouldResumeCategoryScanFromPhaseB(scan))
}

func TestShouldResumeCategoryScanFromPhaseB_StillInPhaseA(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/abs/app/a.php"})
	require.False(t, shouldResumeCategoryScanFromPhaseB(scan), "phase A must not match the B-resume condition")
}

func TestShouldResumeCategoryScanFromPhaseB_NoTargets(t *testing.T) {
	scan := newScanState()
	require.False(t, shouldResumeCategoryScanFromPhaseB(scan))
}

func TestShouldResumeCategoryScanFromPhaseB_AllDone(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/abs/app/a.php"})
	scan.CommitToAudit()
	markFileDoneForTest(scan, "/abs/app/a.php")
	require.False(t, shouldResumeCategoryScanFromPhaseB(scan), "fully marked categories are not resumable")
}

func TestShouldResumeCategoryScanFromPhaseB_Nil(t *testing.T) {
	require.False(t, shouldResumeCategoryScanFromPhaseB(nil))
}

// ── prepareCategoryResume ──

func TestPrepareCategoryResume_PhaseBInterruptedClearsResidue(t *testing.T) {
	scan := newPhaseBInterruptedScanState(t)
	// 模拟中断前遗留的 read/grep 计数（会触发 read-repeat / trace-grep guard）。
	scan.BumpPhaseBRead("/abs/app/b.php")
	scan.BumpPhaseBRead("/abs/app/b.php")
	scan.BumpPhaseBGrep("/abs/app/c.php")
	require.Equal(t, 2, scan.PhaseBReadCount("/abs/app/b.php"))
	require.Equal(t, 1, scan.PhaseBGrepCount("/abs/app/c.php"))

	require.True(t, prepareCategoryResume(scan, "sql-injection"))

	// 残留计数被清零，且未 mark 文件保持未 mark（可恢复性未被销毁）。
	require.Equal(t, 0, scan.PhaseBReadCount("/abs/app/b.php"))
	require.Equal(t, 0, scan.PhaseBGrepCount("/abs/app/c.php"))
	require.Equal(t, []string{"/abs/app/b.php", "/abs/app/c.php"}, scan.RemainingFiles())
	require.Equal(t, ScanPhaseAudit, scan.CurrentPhase())
}

func TestPrepareCategoryResume_PhaseAInterruptedCommits(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/abs/app/a.php"})
	require.True(t, prepareCategoryResume(scan, "sql-injection"))
	require.Equal(t, ScanPhaseAudit, scan.CurrentPhase())
}

func TestPrepareCategoryResume_PhaseBAllDoneNotResumable(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/abs/app/a.php"})
	scan.CommitToAudit()
	markFileDoneForTest(scan, "/abs/app/a.php")
	require.False(t, prepareCategoryResume(scan, "sql-injection"))
}

func TestPrepareCategoryResume_NilScanState(t *testing.T) {
	require.False(t, prepareCategoryResume(nil, "sql-injection"))
}

// ── finalizeCategoryScanOnLoopEnd：阶段B中断不得 auto-mark / 不得写 observation ──

func TestFinalizeCategoryScanOnLoopEnd_PhaseBInterruptedDoesNotAutoMark(t *testing.T) {
	scan := newPhaseBInterruptedScanState(t)
	state := model.NewAuditState()
	r := mock.NewMockInvoker(context.Background())
	category := model.VulnCategory{ID: "sql-injection", Name: "SQL注入"}

	finalizeCategoryScanOnLoopEnd(nil, r, state, scan, category, errors.New("wall-clock timeout"), nil)

	// 可恢复性必须保留：剩余文件未 mark，observation 未写入。
	require.Equal(t, []string{"/abs/app/b.php", "/abs/app/c.php"}, scan.RemainingFiles())
	require.Empty(t, state.GetScanObservations(),
		"phase-B interruption must not write an observation (it would fake completion for the orchestrator)")
	// 且它仍满足恢复条件。
	require.True(t, shouldResumeCategoryScanFromPhaseB(scan))
}

func TestFinalizeCategoryScanOnLoopEnd_PhaseBAllMarkedStillRecords(t *testing.T) {
	// 全部 mark 但没调 complete_scan：维持原 auto-finalize 行为（不算可恢复）。
	scan := newScanState()
	scan.AddTargetFiles([]string{"/abs/app/a.php"})
	scan.CommitToAudit()
	markFileDoneForTest(scan, "/abs/app/a.php")
	state := model.NewAuditState()
	r := mock.NewMockInvoker(context.Background())
	category := model.VulnCategory{ID: "sql-injection", Name: "SQL注入"}

	finalizeCategoryScanOnLoopEnd(nil, r, state, scan, category, nil, nil)

	require.Len(t, state.GetScanObservations(), 1)
	require.Equal(t, "all_marked_no_complete_scan", state.GetScanObservations()[0].StopReason)
}

// ── fallbackFinalizeCategoryScan：兜底必须补 observation 并保持原不变量 ──

func TestFallbackFinalizeCategoryScan_PartialMarksRemainingAndRecords(t *testing.T) {
	scan := newPhaseBInterruptedScanState(t)
	scan.BumpPhaseBRead("/abs/app/b.php")
	state := model.NewAuditState()
	state.WorkDir = t.TempDir()
	r := mock.NewMockInvoker(context.Background())
	category := model.VulnCategory{ID: "sql-injection", Name: "SQL注入"}

	fallbackFinalizeCategoryScan(r, state, scan, category, errors.New("budget exhausted"), nil)

	// 兜底把剩余文件标记为 not_audited（未审计，区别于 not_vul），并写入 observation。
	require.True(t, scan.AllDone())
	require.Equal(t, FileDispositionNotAudited, scan.GetFileDisposition("/abs/app/b.php"))
	require.Equal(t, FileDispositionNotAudited, scan.GetFileDisposition("/abs/app/c.php"))
	require.Equal(t, 0, scan.PhaseBReadCount("/abs/app/b.php"))
	require.Len(t, state.GetScanObservations(), 1)
	obs := state.GetScanObservations()[0]
	require.Equal(t, "auto_finalized_on_timeout", obs.StopReason)
	require.Equal(t, "sql-injection", obs.CategoryID)
	// observation 持久化产物也落盘（scan_observations.json 分片）。
	_, err := os.Stat(filepath.Join(state.WorkDir, "audit"))
	require.NoError(t, err)
}

func TestFallbackFinalizeCategoryScan_NeverStartedRecordsNotRun(t *testing.T) {
	state := model.NewAuditState()
	state.WorkDir = t.TempDir()
	r := mock.NewMockInvoker(context.Background())
	category := model.VulnCategory{ID: "race-condition", Name: "竞态条件"}

	// 排队即死：scanState 从未构建。
	fallbackFinalizeCategoryScan(r, state, nil, category, context.Canceled, nil)

	require.Len(t, state.GetScanObservations(), 1)
	obs := state.GetScanObservations()[0]
	require.Equal(t, "not_run_interrupted", obs.StopReason)
	require.Equal(t, "race-condition", obs.CategoryID)
}

// ── buildCategoryScanOutcome ──

func TestBuildCategoryScanOutcome_TotalAndIncomplete(t *testing.T) {
	state := model.NewAuditState()
	catJob := categoryScanJob{
		category: model.VulnCategory{ID: "sql-injection", Name: "SQL注入"},
		index:    3,
		total:    12,
	}

	// 无 observation → incomplete。
	outcome := buildCategoryScanOutcome(state, catJob)
	require.True(t, outcome.incomplete)
	require.Equal(t, 3, outcome.index)
	require.Equal(t, 12, outcome.total)

	// 有 observation → complete。
	state.AddScanObservation(&model.ScanObservation{CategoryID: "sql-injection"})
	outcome = buildCategoryScanOutcome(state, catJob)
	require.False(t, outcome.incomplete)
}
