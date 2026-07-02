// Package loop_code_security_audit — phase2_guards.go
//
// ToolInvokeGuard hooks for Phase 2 per-category scan loops. These enforce the A/B state
// machine at tool-invocation time (see invoke_toolcall.go), complementing prompt text and
// custom actions (lock_target_files, mark_file_done).
//
// Phase A guards
//   - buildPhase2PhaseASpotReadGuard: force lock_target_files after too many spot reads
//
// Phase B guards
//   - buildPhase2PhaseBReadSpinGuard: force mark_file_done after too many reads on one target
//   - discovery / trace grep policies live in phase2_grep_guard.go
//
// Feedback formatters in this file are shared with custom action handlers in phase2_scan.go.
package phase2

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// phase2MaxSpotReadsBeforeLock: phase-A read_file calls allowed between lock_target_files.
	phase2MaxSpotReadsBeforeLock = 5

	// phase2MaxPhaseBReadsPerFile: phase-B read_file calls on one target before mark_file_done.
	phase2MaxPhaseBReadsPerFile = 2
)

// buildPhase2PhaseASpotReadGuard prevents phase-A agents from spot-reading indefinitely
// without committing candidates via lock_target_files.
func buildPhase2PhaseASpotReadGuard(scan *ScanState) reactloops.ToolInvokeGuard {
	return func(toolName string, _ aitool.InvokeParams) (bool, string) {
		if toolName != "read_file" || scan.CurrentPhase() != ScanPhaseSearch {
			return true, ""
		}
		if scan.PhaseASpotReadCount() < phase2MaxSpotReadsBeforeLock {
			return true, ""
		}
		return false, fmt.Sprintf(
			"[错误] 阶段A 已连续抽查 %d 次 read_file 仍未 lock_target_files（上限 %d）。\n"+
				"下一 action **必须**是 lock_target_files(done=false) 纳入已读候选；\n"+
				"对每个 fast_context 候选：read → lock，全部纳入后才能 done=true。\n"+
				"不要继续 read_file，直到完成 lock。",
			scan.PhaseASpotReadCount(),
			phase2MaxSpotReadsBeforeLock,
		)
	}
}

// formatPathListForFeedback renders a numbered path list for agent-facing error messages.
func formatPathListForFeedback(paths []string, maxShow int) string {
	if len(paths) == 0 {
		return "  （无）\n"
	}
	var b strings.Builder
	for i, p := range paths {
		if i >= maxShow {
			b.WriteString(fmt.Sprintf("  ... 另有 %d 个文件未列出\n", len(paths)-maxShow))
			break
		}
		b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, p))
	}
	return b.String()
}

// buildPhase2PhaseBReadSpinGuard prevents re-reading the same locked target without marking it done.
// Reads of non-target paths or already-marked files are not counted (see phase2_read_file_guard.go).
func buildPhase2PhaseBReadSpinGuard(scan *ScanState) reactloops.ToolInvokeGuard {
	return func(toolName string, params aitool.InvokeParams) (bool, string) {
		if toolName != "read_file" || scan.CurrentPhase() != ScanPhaseAudit {
			return true, ""
		}
		if len(params) == 0 {
			return true, ""
		}
		file := strings.TrimSpace(utils.InterfaceToString(params["file"]))
		if file == "" || !scan.IsTargetFile(file) || scan.IsFileAudited(file) {
			return true, ""
		}
		count := scan.PhaseBReadCount(file)
		if count < phase2MaxPhaseBReadsPerFile {
			return true, ""
		}
		return false, formatPhaseBReadSpinFeedback(file, count, scan)
	}
}

func formatPhaseBReadSpinFeedback(file string, readCount int, scan *ScanState) string {
	return fmt.Sprintf(
		"[错误] 阶段B 已对 %q read_file %d 次仍未 mark_file_done（上限 %d）。\n"+
			"下一 action **必须**是 mark_file_done(file_path=%q, audit_summary=...)。\n"+
			"若无漏洞也要 mark；全部 mark 后才能 complete_scan。\n"+
			"当前待审计文件：\n%s",
		file,
		readCount,
		phase2MaxPhaseBReadsPerFile,
		file,
		formatPathListForFeedback(scan.RemainingFiles(), 20),
	)
}

// formatCompleteScanBlockedFeedback is used when complete_scan is called before all targets are attributed.
func formatCompleteScanBlockedFeedback(scan *ScanState, state *model.AuditState, categoryID, projectRoot string) string {
	done, total := scan.Progress()
	remaining := scan.RemainingFiles()
	if len(remaining) > 0 {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("[错误] 尚有 %d/%d 个目标文件未完成归属，禁止调用 complete_scan。\n", total-done, total))
		b.WriteString("每个 lock 的文件须：add_finding + mark(disposition=finding)，或 mark(disposition=not_vul)。\n")
		b.WriteString("待处理文件：\n")
		for _, f := range remaining {
			hint := formatRemainingFileAttributionHint(scan, state, categoryID, f, projectRoot)
			b.WriteString(fmt.Sprintf("  - %s%s\n", f, hint))
		}
		return b.String()
	}
	if ok, msg := validateAllTargetsAttributed(scan, state, categoryID, projectRoot); !ok {
		return msg
	}
	return "[错误] complete_scan 被拒绝：目标文件归属状态异常。"
}

func formatMarkFileDoneNotTargetFeedback(filePath string, scan *ScanState) string {
	remaining := scan.RemainingFiles()
	return fmt.Sprintf(
		"[错误] %q 不在本类别已 lock 的目标文件列表中，无法 mark_file_done。\n"+
			"file_path 必须与 lock_target_files 纳入的路径完全一致（绝对路径）。\n"+
			"当前待审计文件：\n%s",
		filePath,
		formatPathListForFeedback(remaining, 20),
	)
}

func formatMarkFileDoneAlreadyDoneFeedback(filePath string, scan *ScanState) string {
	remaining := scan.RemainingFiles()
	next := ""
	if len(remaining) > 0 {
		next = remaining[0]
	}
	return fmt.Sprintf(
		"[提示] %q 已经 mark_file_done，无需重复标记。\n"+
			"还有 %d 个文件待审计。下一个：%s\n"+
			"请 read_file → mark_file_done，全部完成后才能 complete_scan。",
		filePath,
		len(remaining),
		next,
	)
}
