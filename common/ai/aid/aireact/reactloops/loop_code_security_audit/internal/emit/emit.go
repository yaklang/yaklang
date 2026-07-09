// Package loop_code_security_audit — audit_emit.go
//
// User-facing UI stream for the code-security-audit focused mode lifecycle.
// Emit sparingly: phase boundaries, scan plan, per-category outcomes, findings,
// verification conclusions, and warnings — not per-tool traces (those stay in timeline/log).
package emit

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// emitAuditStructured sends machine-readable events for the frontend (optional rendering).
func Structured(loop *reactloops.ReActLoop, eventID string, payload map[string]any) {
	if loop == nil {
		return
	}
	emitter := loop.GetEmitter()
	if emitter == nil {
		return
	}
	if payload == nil {
		payload = make(map[string]any)
	}
	if task := loop.GetCurrentTask(); task != nil {
		payload["task_id"] = task.GetId()
	}
	_, _ = emitter.EmitJSON(schema.EVENT_TYPE_STRUCTURED, eventID, payload)
}

// ─── Orchestrator / Phase 1 ───────────────────────────────────────────

func ReconStart(loop *reactloops.ReActLoop, scanTarget string) {
	if loop == nil {
		return
	}
	reactloops.EmitActionLog(loop, util.ReconNodeID, "Phase 1：项目探索 / Phase 1: Project reconnaissance")
	if scanTarget != "" {
		reactloops.EmitActionLog(loop, util.ReconNodeID,
			fmt.Sprintf("扫描目标: %s / Scan target: %s", scanTarget, scanTarget))
	}
}

func ReconComplete(loop *reactloops.ReActLoop, projectPath, techStack, reconFile string, incomplete bool) {
	if loop == nil {
		return
	}
	if incomplete {
		reactloops.EmitActionLog(loop, util.ReconNodeID,
			"[警告] Phase 1 未完成探索 / Phase 1 recon incomplete — proceeding without full context")
		Structured(loop, "code_audit_recon_done", map[string]any{
			"incomplete": true,
		})
		return
	}
	shortPath := projectPath
	if i := strings.LastIndex(projectPath, "/"); i >= 0 && i+1 < len(projectPath) {
		shortPath = projectPath[i+1:]
	}
	lines := fmt.Sprintf(
		"Phase 1 完成 / Phase 1 complete\n项目: %s | 技术栈: %s",
		shortPath, utils.ShrinkString(techStack, 120),
	)
	if reconFile != "" {
		lines += fmt.Sprintf("\n背景报告: %s", reconFile)
	}
	reactloops.EmitActionLog(loop, util.ReconNodeID, lines)
	Structured(loop, "code_audit_recon_done", map[string]any{
		"project_path": projectPath,
		"tech_stack":   techStack,
		"recon_file":   reconFile,
		"incomplete":   false,
	})
}

func SkipVerify(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	reactloops.EmitActionLog(loop, util.VerifyNodeID,
		"跳过 Phase 3：未发现疑似漏洞 / Skipping verify: no findings")
	Structured(loop, "code_audit_skip_verify", map[string]any{
		"reason": "no_findings",
	})
}

func PipelineDone(loop *reactloops.ReActLoop, reportPath string, reportBytes int, stats model.AuditStats) {
	if loop == nil {
		return
	}
	lines := fmt.Sprintf(
		"审计流水线完成 / Audit pipeline complete\n报告: %s (%d bytes)\n确认 %d | 待复核 %d | 已排除 %d",
		reportPath, reportBytes,
		stats.ConfirmedCount, stats.UncertainCount, stats.SafeCount,
	)
	reactloops.EmitActionLog(loop, util.ReportNodeID, lines)
	Structured(loop, "code_audit_done", map[string]any{
		"report_path":  reportPath,
		"report_bytes": reportBytes,
		"confirmed":    stats.ConfirmedCount,
		"uncertain":    stats.UncertainCount,
		"safe":         stats.SafeCount,
		"high":         stats.HighCount,
		"medium":       stats.MediumCount,
		"low":          stats.LowCount,
	})
}

// ─── Phase 2 ──────────────────────────────────────────────────────────

func Phase2ScanPlan(loop *reactloops.ReActLoop, categories []model.VulnCategory) {
	if loop == nil || len(categories) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("扫描计划：%d 个类别 / Scan plan: %d categories\n", len(categories), len(categories)))
	ids := make([]string, 0, len(categories))
	for i, c := range categories {
		if i >= 12 {
			b.WriteString(fmt.Sprintf("  ... 另有 %d 个类别\n", len(categories)-12))
			break
		}
		b.WriteString(fmt.Sprintf("  %d. %s (%s)\n", i+1, c.Name, c.ID))
		ids = append(ids, c.ID)
	}
	reactloops.EmitActionLog(loop, util.ScanNodeID, b.String())
	reactloops.EmitStatus(loop, fmt.Sprintf("Phase 2：%d 个类别待扫描 / Phase 2: %d categories", len(categories), len(categories)))
	Structured(loop, "code_audit_scan_plan", map[string]any{
		"category_count": len(categories),
		"category_ids":   ids,
	})
}

// Phase2AuditVulnerabilityTypes emits the finalized vulnerability types for the frontend sub-agent card.
func Phase2AuditVulnerabilityTypes(loop *reactloops.ReActLoop, categories []model.VulnCategory) {
	if loop == nil || len(categories) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("审计漏洞类型：%d 类 / Audit vulnerability types: %d\n", len(categories), len(categories)))
	ids := make([]string, 0, len(categories))
	names := make([]string, 0, len(categories))
	for i, c := range categories {
		if i < 12 {
			b.WriteString(fmt.Sprintf("  %d. %s (%s)\n", i+1, c.Name, c.ID))
		}
		ids = append(ids, c.ID)
		names = append(names, c.Name)
	}
	if len(categories) > 12 {
		b.WriteString(fmt.Sprintf("  ... 另有 %d 类\n", len(categories)-12))
	}
	reactloops.EmitActionLog(loop, util.ScanNodeID, b.String())
	reactloops.EmitStatus(loop, fmt.Sprintf("已确定 %d 类审计漏洞类型 / %d vulnerability types confirmed", len(categories), len(categories)))
	Structured(loop, "code_audit_vulnerability_types", map[string]any{
		"type_count": len(categories),
		"type_ids":   ids,
		"type_names": names,
	})
}

func Phase2AllCategoriesDone(loop *reactloops.ReActLoop, categoryCount, findingCount int) {
	if loop == nil {
		return
	}
	reactloops.EmitActionLog(loop, util.ScanNodeID,
		fmt.Sprintf("Phase 2 完成 / Phase 2 complete：%d 类别，%d findings / %d categories, %d findings",
			categoryCount, findingCount, categoryCount, findingCount))
	Structured(loop, "code_audit_scan_complete", map[string]any{
		"category_count": categoryCount,
		"finding_count":  findingCount,
	})
}

func Phase2CategoryOutcome(
	loop *reactloops.ReActLoop,
	index, total int,
	category model.VulnCategory,
	findingCount int,
	incomplete bool,
) {
	if loop == nil {
		return
	}
	label := "[完成]"
	if incomplete {
		label = "[警告]"
	}
	reactloops.EmitActionLog(loop, util.ScanNodeID, fmt.Sprintf(
		"%s [%d/%d] %s (%s) — 本类 findings: %d / Category done, findings: %d",
		label, index, total, category.Name, category.ID, findingCount, findingCount,
	))
	if incomplete {
		reactloops.EmitStatus(loop, fmt.Sprintf("[警告] 类别 %s 可能未完成 / Category %s may be incomplete", category.ID, category.ID))
	} else {
		reactloops.EmitProgress(loop, index, total, "扫描进度", "Scan progress")
	}
	Structured(loop, "code_audit_scan_category_done", map[string]any{
		"category_id":   category.ID,
		"category_name": category.Name,
		"index":         index,
		"total":         total,
		"finding_count": findingCount,
		"incomplete":    incomplete,
	})
}

func Phase2FindingAdded(loop *reactloops.ReActLoop, category model.VulnCategory, f *model.Finding, totalFindings int) {
	if loop == nil || f == nil {
		return
	}
	shortFile := f.File
	if i := strings.LastIndex(f.File, "/"); i >= 0 && i+1 < len(f.File) {
		shortFile = f.File[i+1:]
	}
	reactloops.EmitActionLog(loop, util.ScanNodeID, fmt.Sprintf(
		"Finding [%s] %s %s — %s:%d (%s) / %s",
		category.Name, f.Severity, f.ID, shortFile, f.Line, utils.ShrinkString(f.Title, 80), f.Title,
	))
	reactloops.EmitStatus(loop, fmt.Sprintf("已发现 %d 个 finding / %d findings so far", totalFindings, totalFindings))
}

func Phase2CategoryScanComplete(loop *reactloops.ReActLoop, category model.VulnCategory, findingCount int, coveragePreview string) {
	if loop == nil {
		return
	}
	lines := fmt.Sprintf("类别扫描完成 [%s] %s — %d findings / Category scan done: %d findings",
		category.Name, category.ID, findingCount, findingCount)
	if coveragePreview != "" {
		lines += "\n" + utils.ShrinkString(coveragePreview, 300)
	}
	reactloops.EmitActionLog(loop, util.ScanNodeID, lines)
	Structured(loop, "code_audit_scan_category_complete", map[string]any{
		"category_id":      category.ID,
		"category_name":    category.Name,
		"finding_count":    findingCount,
		"coverage_preview": utils.ShrinkString(coveragePreview, 500),
	})
}

func Phase2ScanWarning(loop *reactloops.ReActLoop, category model.VulnCategory, kind, message string) {
	if loop == nil {
		return
	}
	prefix := "[警告]"
	switch kind {
	case "stuck_phase_a", "auto_finalize", "incomplete":
		prefix = "[警告]"
	default:
		prefix = "[信息]"
	}
	catLabel := category.ID
	if category.Name != "" {
		catLabel = category.Name
	}
	reactloops.EmitActionLog(loop, util.ScanNodeID,
		fmt.Sprintf("%s [%s] %s", prefix, catLabel, utils.ShrinkString(message, 500)))
	Structured(loop, "code_audit_scan_warning", map[string]any{
		"category_id":   category.ID,
		"category_name": category.Name,
		"kind":          kind,
		"message":       message,
	})
}

// ─── Phase 3 ──────────────────────────────────────────────────────────

func Phase3VerifyStart(loop *reactloops.ReActLoop, total int) {
	if loop == nil || total <= 0 {
		return
	}
	reactloops.EmitActionLog(loop, util.VerifyNodeID,
		fmt.Sprintf("开始验证 %d 个 findings / Verifying %d findings", total, total))
	reactloops.EmitStatus(loop, fmt.Sprintf("漏洞验证中 (%d) / Verifying findings (%d)", total, total))
}

// Phase3VerifyScope emits the verification scope grouped by vulnerability type for the frontend.
func Phase3VerifyScope(loop *reactloops.ReActLoop, findings []*model.Finding, byCategory map[string][]string) {
	if loop == nil || len(findings) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("验证范围：%d 个 finding，%d 类漏洞 / Verify scope: %d findings, %d types\n",
		len(findings), len(byCategory), len(findings), len(byCategory)))
	categoryIDs := make([]string, 0, len(byCategory))
	for cat := range byCategory {
		categoryIDs = append(categoryIDs, cat)
	}
	sort.Strings(categoryIDs)
	for i, cat := range categoryIDs {
		if i >= 12 {
			b.WriteString(fmt.Sprintf("  ... 另有 %d 类\n", len(categoryIDs)-12))
			break
		}
		b.WriteString(fmt.Sprintf("  • %s: %s\n", cat, strings.Join(byCategory[cat], ", ")))
	}
	reactloops.EmitActionLog(loop, util.VerifyNodeID, b.String())
	reactloops.EmitStatus(loop, fmt.Sprintf("已确定验证范围 (%d findings) / Verification scope confirmed", len(findings)))
	Structured(loop, "code_audit_verify_scope", map[string]any{
		"finding_count": len(findings),
		"type_count":    len(byCategory),
		"category_ids":  categoryIDs,
		"by_category":   byCategory,
	})
}

func Phase3ConcludeFinding(
	loop *reactloops.ReActLoop,
	findingID string,
	status model.VerifyStatus,
	verified, total int,
	title string,
) {
	if loop == nil {
		return
	}
	reactloops.EmitActionLog(loop, util.VerifyNodeID, fmt.Sprintf(
		"验证 %s → %s (%d/%d) %s / Verified %s → %s",
		findingID, status, verified, total,
		utils.ShrinkString(title, 60), findingID, status,
	))
	if total > 0 {
		reactloops.EmitProgress(loop, verified, total, "验证进度", "Verify progress")
	}
}

func VerifyComplete(loop *reactloops.ReActLoop, summary string, stats model.AuditStats) {
	if loop == nil {
		return
	}
	lines := fmt.Sprintf(
		"Phase 3 完成 / Phase 3 complete\n确认 %d | 待复核 %d | 已排除 %d / confirmed %d | uncertain %d | safe %d",
		stats.ConfirmedCount, stats.UncertainCount, stats.SafeCount,
		stats.ConfirmedCount, stats.UncertainCount, stats.SafeCount,
	)
	if summary != "" {
		lines += "\n" + utils.ShrinkString(summary, 300)
	}
	reactloops.EmitActionLog(loop, util.VerifyNodeID, lines)
	Structured(loop, "code_audit_verify_complete", map[string]any{
		"confirmed": stats.ConfirmedCount,
		"uncertain": stats.UncertainCount,
		"safe":      stats.SafeCount,
		"summary":   summary,
	})
}

// ─── Phase 4 ──────────────────────────────────────────────────────────

func Phase4ReportComplete(loop *reactloops.ReActLoop, reportPath string, stats model.AuditStats) {
	if loop == nil {
		return
	}
	reactloops.EmitActionLog(loop, util.ReportNodeID, fmt.Sprintf(
		"报告已生成 / Report ready: %s\n高危 %d | 中危 %d | 低危 %d | 待复核 %d",
		reportPath, stats.HighCount, stats.MediumCount, stats.LowCount, stats.UncertainCount,
	))
	reactloops.EmitStatus(loop, "报告生成完成 / Report generation complete")
	Structured(loop, "code_audit_report_done", map[string]any{
		"report_path": reportPath,
		"high":        stats.HighCount,
		"medium":      stats.MediumCount,
		"low":         stats.LowCount,
		"uncertain":   stats.UncertainCount,
	})
}

func ReportFallback(loop *reactloops.ReActLoop, reportPath string) {
	if loop == nil {
		return
	}
	reactloops.EmitActionLog(loop, util.ReportNodeID,
		fmt.Sprintf("[警告] 使用兜底报告 / Fallback report: %s", reportPath))
}
