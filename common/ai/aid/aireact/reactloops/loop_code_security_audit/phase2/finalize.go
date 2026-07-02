package phase2

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/emit"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
) {
	if scan == nil || state == nil {
		return
	}

	phase := scan.CurrentPhase()
	if phase == ScanPhaseSearch {
		if scan.TargetFileCount() > 0 {
			msg := fmt.Sprintf("[Phase2/%s] 类别循环结束前仍处于阶段A，但已纳入 %d 个目标；编排器将尝试恢复阶段B。",
				category.ID, scan.TargetFileCount())
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
	reasonText := formatLoopEndReason(reason)

	if len(remaining) == 0 && done == total && total > 0 {
		log.Infof("[CodeAudit/Phase2] Category '%s' loop ended with all files marked but no complete_scan; auto-finalizing.", category.ID)
		recordAutoFinalizedScanObservation(r, state, scan, category, reasonText, "all_marked_no_complete_scan")
		return
	}

	if len(remaining) == 0 {
		return
	}

	var autoMarked []string
	for _, filePath := range remaining {
		scan.MarkFileDoneWithDisposition(filePath, FileDispositionNotVul)
		scan.ClearPhaseBReads(filePath)
		scan.ClearPhaseBGreps(filePath)
		autoMarked = append(autoMarked, filePath)
	}

	summary := fmt.Sprintf(
		"类别循环结束前自动收尾：%d 个文件未显式 mark_file_done，已由系统代为标记。\n"+
			"原因：%s\n未 mark 文件：\n%s",
		len(autoMarked),
		reasonText,
		strings.Join(autoMarked, "\n"),
	)
	log.Warnf("[CodeAudit/Phase2] Category '%s' auto-finalized %d remaining files: %v",
		category.ID, len(autoMarked), autoMarked)
	r.AddToTimeline("[SCAN_AUTO_FINALIZE]", fmt.Sprintf("[Phase2/%s] %s", category.ID, summary))
	emit.Phase2ScanWarning(loop, category, "auto_finalize", summary)

	recordAutoFinalizedScanObservation(r, state, scan, category, summary, "auto_finalized_on_loop_end")
}

func formatLoopEndReason(reason any) string {
	if reason == nil {
		return "loop 正常/异常结束但未调用 complete_scan"
	}
	switch v := reason.(type) {
	case error:
		if v != nil {
			return v.Error()
		}
	case fmt.Stringer:
		return v.String()
	default:
		text := strings.TrimSpace(utils.InterfaceToString(v))
		if text != "" {
			return text
		}
	}
	return "loop 结束但未调用 complete_scan"
}

func recordAutoFinalizedScanObservation(
	r aicommon.AIInvokeRuntime,
	state *model.AuditState,
	scan *ScanState,
	category model.VulnCategory,
	coverageSummary string,
	stopReason string,
) {
	done, total := scan.Progress()
	obs := &model.ScanObservation{
		CategoryID:      category.ID,
		CategoryName:    category.Name,
		StopReason:      stopReason,
		CoverageSummary: coverageSummary,
	}
	state.AddScanObservation(obs)

	r.AddToTimeline("[SCAN_COMPLETE]",
		fmt.Sprintf("[Phase2/%s] 自动收尾完成（%d/%d 文件已 mark）\n%s", category.ID, done, total, coverageSummary))

	auditDirPath := util.AuditDir(state)
	if mkErr := os.MkdirAll(auditDirPath, 0o755); mkErr != nil {
		log.Warnf("[CodeAudit/Phase2] Failed to create audit dir for auto-finalize: %v", mkErr)
		return
	}
	obsFile := filepath.Join(auditDirPath, "scan_observations.md")
	if err := state.PersistScanObservations(obsFile); err != nil {
		log.Warnf("[CodeAudit/Phase2] Persist scan_observations failed: %v", err)
	}
	persistCategoryFindings(state, category.ID, auditDirPath)
}

func persistCategoryFindings(state *model.AuditState, categoryID, auditDirPath string) {
	var catFindings []*model.Finding
	for _, f := range state.GetFindings() {
		if f.Category == categoryID {
			catFindings = append(catFindings, f)
		}
	}
	catFile := filepath.Join(auditDirPath, fmt.Sprintf("findings_%s.json", categoryID))
	if data, err := json.MarshalIndent(catFindings, "", "  "); err == nil {
		if err := os.WriteFile(catFile, data, 0o644); err != nil {
			log.Warnf("[CodeAudit/Phase2] Write category findings file failed: %v", err)
		}
	}
}
