package loop_ssa_api_discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

func runPhase6FinalReport(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, pl *PipelineState) error {
	snapStart := time.Now()
	rt.execStepStart("phase5.export_snapshot", "programmatic")
	snapPath, err := ExportDiscoverySnapshotJSON(rt)
	if err != nil {
		rt.execStepError("phase5.export_snapshot", "programmatic", snapStart, err, nil)
		log.Warnf("phase6 snapshot: %v", err)
		snapPath = ""
	} else {
		rt.execStepEnd("phase5.export_snapshot", "programmatic", snapStart, []string{snapPath})
	}
	dir := filepath.Join(rt.WorkDir, store.SubDirName())
	finalPath := filepath.Join(dir, "final_audit_report.md")
	_ = os.WriteFile(finalPath, []byte(""), 0o644)
	pl.SetFinalReportPath(finalPath)

	reportStart := time.Now()
	rt.execStepStart("phase5.final_report", "ai")

	var refs strings.Builder
	if snapPath != "" {
		refs.WriteString(fmt.Sprintf("- %s\n", snapPath))
	}
	if p := pl.GetDiscoveryReportPath(); p != "" {
		refs.WriteString(fmt.Sprintf("- %s\n", p))
	}
	if p := pl.GetSyntaxFlowJSONPath(); p != "" {
		refs.WriteString(fmt.Sprintf("- %s\n", p))
	}
	if rt != nil {
		refs.WriteString(buildPhase1ArtifactRefs(rt, ""))
	}

	writePrompt := fmt.Sprintf(`报告**首行一级标题**必须为以下一行（勿在其前再写其他一级标题）：
# [阶段 5/%d] Phase5：最终安全审计报告 完成报告

请撰写 **最终安全审计报告**（Markdown；除专有名词外**通篇简体中文**，章节标题须中文），综合：
1. API 与架构分析（discovery_report、快照 JSON、Phase1 工件）
2. **HTTP 端点确认真源：verified_http_apis**（discovery_read_session_data entity=verified_http_apis）；verified=true 且含 probe 证据的为已确认；verified=false 为已排除/待复核
3. SyntaxFlow：阅读 **syntaxflow_summary.json** 的 scan_meta 与 rule_names；正文仅概述规则集合
4. 漏洞验证：entity=vuln_verifications；与 syntaxflow_findings 交叉时用 discovery_read_session_data

## 必须先读取的参考文件
%s

## 报告结构
- 执行摘要与范围
- 资产与攻击面（以 verified_http_apis 为主；勿将 http_endpoints pending 误报为已确认）
- 静态代码风险（SyntaxFlow）：概述选用 filter、命中规则条数与严重度分布；**完整规则名列表以附件 syntaxflow_summary.json 为准**
- 已确认漏洞、待人工项（uncertain）
- 修复与缓解建议
- 审计局限

写得具体、可追溯，勿编造未在数据中出现的内容。
%s%s`, pipelineSummaryTotalPhases, refs.String(), ssaDiscoveryReportDataReadHint, ssaDiscoveryReportLanguageZH)

	reportOpts := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(ssaDiscoveryMaxIterations(r)),
		reactloops.WithAllowUserInteract(false),
	}
	reportOpts = append(reportOpts, discoveryReportReadActionOptions(rt)...)
	reportOpts = append(reportOpts, reactloops.WithInitTask(func(innerLoop *reactloops.ReActLoop, innerTask aicommon.AIStatefulTask, innerOp *reactloops.InitTaskOperator) {
			bindDiscoveryRuntimeInLoop(innerLoop, rt)
			innerLoop.Set("report_filename", finalPath)
			innerLoop.Set("full_report_code", "")
			innerLoop.Set("user_requirements", writePrompt)
			innerLoop.Set("collected_references", "")
			innerLoop.Set("is_modify_mode", "false")
			innerLoop.Set("available_files", "### 引用\n"+refs.String())
			innerLoop.Set("available_knowledge_bases", "")
			innerOp.Continue()
		}))
	reportLoop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_REPORT_GENERATING,
		r,
		reportOpts...,
	)
	if err != nil {
		rt.execStepError("phase5.final_report", "ai", reportStart, err, []string{finalPath})
		return err
	}
	if err := reportLoop.ExecuteWithExistedTask(newSubTask(task, "phase6_final_report")); err != nil {
		log.Warnf("phase6: %v", err)
		rt.execStepError("phase5.final_report", "ai", reportStart, err, []string{finalPath})
	} else {
		rt.execStepEnd("phase5.final_report", "ai", reportStart, []string{finalPath})
	}
	r.AddToTimeline("[ssa_phase6]", "final_audit_report: "+finalPath)

	// 轻量兜底
	if b, err := os.ReadFile(finalPath); err == nil && len(strings.TrimSpace(string(b))) < minFinalReportBytes {
		fb := generatePipelineFallbackReport(rt)
		_ = os.WriteFile(finalPath, []byte(fb), 0o644)
	}
	if b, err := os.ReadFile(finalPath); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			r.EmitResultAfterStream(s)
		}
	}
	return nil
}

func generatePipelineFallbackReport(rt *Runtime) string {
	if rt == nil || rt.Session == nil || rt.Repo == nil {
		return "# SSA API 流水线\n\n（无会话数据）\n"
	}
	s := rt.Session
	eps, _ := rt.Repo.ListHttpEndpoints(s.ID)
	gate, _ := rt.Repo.CountVerifiedHttpApiGate(s.ID)
	sf, _ := rt.Repo.ListDiscoverySyntaxFlowFindings(s.ID)
	vv, _ := rt.Repo.ListVulnVerifications(s.ID)
	sfMeta := ""
	if m, err := ParseSyntaxFlowScanMeta(s.SyntaxFlowScanMetaJSON); err == nil && m != nil {
		sfMeta = fmt.Sprintf("SyntaxFlow 扫描: rules_queued=%d risks_imported=%d source=%s", m.RulesQueued, m.RisksImported, m.Source)
	}
	return fmt.Sprintf(`# SSA API 安全流水线报告（自动生成兜底）

- Session: %s
- Code: %s
- Target: %s
- Phase: %s

## 数量
- HTTP 候选端点: %d
- verified_http_apis 已确认: %d（记录总数含拒绝 %d）
- SyntaxFlow 发现: %d
- 漏洞验证记录: %d
- %s

> 完整分析见 workdir/%s/ 下 discovery_report.md / syntaxflow_summary.json（含 scan_meta 与 rule_names） / final_audit_report.md
`, s.UUID, s.CodeRootPath, s.TargetRaw, s.Phase, len(eps), gate.Verified, gate.Total, len(sf), len(vv), sfMeta, store.SubDirName())
}
