package phase2

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/emit"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

func emitPhase2Structured(loop *reactloops.ReActLoop, eventID string, payload map[string]any) {
	emit.Structured(loop, eventID, payload)
}

// emitPhase2LockTargetFiles surfaces lock_target_files to the code-audit-scan stream.
func emitPhase2LockTargetFiles(
	loop *reactloops.ReActLoop,
	category model.VulnCategory,
	added, total int,
	done bool,
	reason string,
	batchFiles []string,
) {
	if loop == nil {
		return
	}
	doneLabel := "追加目标"
	if done {
		doneLabel = "进入阶段B"
	}
	lines := fmt.Sprintf(
		"lock_target_files [%s] %s：本轮 +%d，累计 %d 个目标文件 / Lock targets (%s): +%d, total %d",
		category.Name, doneLabel, added, total, category.ID, added, total,
	)
	if reason != "" {
		lines += "\n理由: " + reason
	}
	if len(batchFiles) > 0 {
		lines += "\n本轮纳入:\n" + formatPathListForFeedback(batchFiles, 15)
	}
	reactloops.EmitActionLog(loop, util.ScanNodeID, lines)
	if done {
		reactloops.EmitStatus(loop, fmt.Sprintf("阶段B 逐文件审计：%s (%d 个文件) / Phase B audit: %s", category.Name, total, category.ID))
	} else {
		reactloops.EmitStatus(loop, fmt.Sprintf("阶段A 已锁定 %d 个候选 / Phase A: %d targets locked", total, total))
	}
	emitPhase2Structured(loop, "code_audit_scan_lock_targets", map[string]any{
		"category_id":   category.ID,
		"category_name": category.Name,
		"added":         added,
		"total":         total,
		"done":          done,
		"reason":        reason,
		"batch_files":   batchFiles,
	})
}

// emitPhase2FastContextResult surfaces fast_context discovery output to the UI stream.
func emitPhase2FastContextResult(
	loop *reactloops.ReActLoop,
	category model.VulnCategory,
	paths []string,
	summaryMarkdown string,
) {
	if loop == nil {
		return
	}
	lines := fmt.Sprintf(
		"fast_context [%s] 返回 %d 个候选（须全部 read+lock）/ FastContext: %d candidates (read each, then lock)\n%s",
		category.Name, len(paths), len(paths),
		formatPathListForFeedback(paths, 12),
	)
	if summaryMarkdown != "" {
		lines += "\n摘要: " + utils.ShrinkString(strings.TrimSpace(summaryMarkdown), 400)
	}
	reactloops.EmitActionLog(loop, util.ScanNodeID, lines)
	reactloops.EmitStatus(loop, fmt.Sprintf("阶段A：%d 个 fast_context 候选 / Phase A: %d candidates", len(paths), len(paths)))
	emitPhase2Structured(loop, "code_audit_scan_fast_context", map[string]any{
		"category_id":     category.ID,
		"category_name":   category.Name,
		"candidate_count": len(paths),
		"candidates":      paths,
	})
}

// emitPhase2MarkFileDone surfaces mark_file_done to the code-audit-scan stream.
func emitPhase2MarkFileDone(
	loop *reactloops.ReActLoop,
	category model.VulnCategory,
	filePath string,
	done, total, remaining int,
	auditSummary string,
) {
	if loop == nil {
		return
	}
	short := filePath
	if i := strings.LastIndex(filePath, "/"); i >= 0 && i+1 < len(filePath) {
		short = filePath[i+1:]
	}
	lines := fmt.Sprintf(
		"mark_file_done [%s] %s（%d/%d）/ File audited: %s (%d/%d)",
		category.Name, short, done, total, short, done, total,
	)
	if auditSummary != "" {
		lines += "\n摘要: " + auditSummary
	}
	if remaining > 0 {
		lines += fmt.Sprintf("\n剩余 %d 个文件待审计 / %d files remaining", remaining, remaining)
	} else {
		lines += "\n全部目标文件已审计，请 complete_scan / All files done — call complete_scan"
	}
	reactloops.EmitActionLog(loop, util.ScanNodeID, lines)
	reactloops.EmitStatus(loop, fmt.Sprintf("审计进度 %d/%d / Progress %d/%d", done, total, done, total))
	emitPhase2Structured(loop, "code_audit_scan_mark_file_done", map[string]any{
		"category_id":   category.ID,
		"category_name": category.Name,
		"file_path":     filePath,
		"audit_done":    done,
		"audit_total":   total,
		"remaining":     remaining,
		"audit_summary": auditSummary,
	})
}
