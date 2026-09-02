package phase2

import (
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/emit"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

// finalizeCategoryScanOnLoopEnd runs when a category loop ends without complete_scan.
// It auto-marks remaining target files and records a model.ScanObservation so Phase 3/4 can proceed.
func finalizeCategoryScanOnLoopEnd(
	loop *reactloops.ReActLoop,
	r aicommon.AIInvokeRuntime,
	state *model.AuditState,
	scan *ScanState,
	category model.VulnCategory,
	reason any,
	artifacts *categoryArtifactStore,
) {
	if scan == nil || state == nil {
		return
	}

	phase := scan.CurrentPhase()
	if phase == ScanPhaseSearch {
		if scan.TargetFileCount() > 0 || scan.DiscoveryCandidateCount() > 0 {
			msg := fmt.Sprintf("[Phase2/%s] 类别循环结束前仍处于阶段A（目标 %d，fast_context 候选 %d）；编排器将尝试恢复阶段B。",
				category.ID, scan.TargetFileCount(), scan.DiscoveryCandidateCount())
			log.Warnf("[CodeAudit/Phase2] %s", msg)
			r.AddToTimeline("[SCAN_INCOMPLETE]", msg)
			emit.Phase2ScanWarning(loop, category, "stuck_phase_a_resumable", msg)
			return
		}
		msg := fmt.Sprintf("[Phase2/%s] 类别循环结束前仍处于阶段A，未进入逐文件审计。", category.ID)
		log.Warnf("[CodeAudit/Phase2] %s", msg)
		r.AddToTimeline("[SCAN_INCOMPLETE]", msg)
		emit.Phase2ScanWarning(loop, category, "stuck_phase_a", msg)
		return
	}

	remaining := scan.RemainingFiles()
	done, total := scan.Progress()
	reasonText := util.FormatLoopEndReason(reason)

	if len(remaining) == 0 && done == total && total > 0 {
		log.Infof("[CodeAudit/Phase2] Category '%s' loop ended with all files marked but no complete_scan; auto-finalizing.", category.ID)
		recordAutoFinalizedScanObservation(r, state, scan, category, reasonText, "all_marked_no_complete_scan", artifacts)
		return
	}

	if len(remaining) == 0 {
		return
	}

	// 阶段B中断但仍有剩余目标文件：不做 auto-mark、不写 observation（auto-mark
	// 会销毁可恢复性，并让编排器误判"已完成"）。只发 resumable 警告，把恢复
	// 与兜底的决定权交给编排器（见 orchestrate.go 的 resume + fallback 流程）。
	msg := fmt.Sprintf(
		"[Phase2/%s] 类别循环在阶段B中断：%d/%d 文件已审计，剩余 %d 个未 mark；编排器将尝试恢复续扫。",
		category.ID, done, total, len(remaining))
	log.Warnf("[CodeAudit/Phase2] %s", msg)
	r.AddToTimeline("[SCAN_INCOMPLETE]", msg)
	emit.Phase2ScanWarning(loop, category, "stuck_phase_b_resumable", msg)
}

func recordAutoFinalizedScanObservation(
	r aicommon.AIInvokeRuntime,
	state *model.AuditState,
	scan *ScanState,
	category model.VulnCategory,
	coverageSummary string,
	stopReason string,
	artifacts *categoryArtifactStore,
) {
	done, total := scan.Progress()
	obs := &model.ScanObservation{
		CategoryID:      category.ID,
		CategoryName:    category.Name,
		StopReason:      stopReason,
		CoverageSummary: coverageSummary,
		Status:          model.ScanStatusCompleted,
		AuditedFiles:    done,
		TargetFiles:     total,
	}
	state.AddScanObservation(obs)

	r.AddToTimeline("[SCAN_COMPLETE]",
		fmt.Sprintf("[Phase2/%s] 自动收尾完成（%d/%d 文件已 mark）\n%s", category.ID, done, total, coverageSummary))

	auditDirPath := util.AuditDir(state)
	if mkErr := os.MkdirAll(auditDirPath, 0o755); mkErr != nil {
		log.Warnf("[CodeAudit/Phase2] Failed to create audit dir for auto-finalize: %v", mkErr)
		return
	}
	persistCategoryObservation(artifacts, auditDirPath, category.ID, obs)
}
