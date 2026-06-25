package loop_ssa_api_discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

func runPhase5Step0Checklist(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, pl *PipelineState) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return
	}
	step := "phase4.step0.checklist"
	started := time.Now()
	rt.execStepStart(step, "programmatic")
	sid := rt.Session.ID

	findings, _ := rt.Repo.ListDiscoverySyntaxFlowFindings(sid)
	targets, _ := ListProbeTargets(rt)
	endpoints, _ := rt.Repo.ListHttpEndpoints(sid)

	epByHandler := map[string]*store.HttpEndpoint{}
	for i := range endpoints {
		ep := &endpoints[i]
		key := strings.ToLower(strings.TrimSpace(ep.HandlerClass))
		if key != "" {
			epByHandler[key] = ep
		}
	}

	var items []VulnChecklistItem
	var unmatched []string

	for _, f := range findings {
		item, matched := associateFindingToEndpoints(f, targets, endpoints, epByHandler)
		if !matched {
			unmatched = append(unmatched, fmt.Sprintf("finding #%d (%s) %s", f.ID, f.Severity, f.Title))
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Priority > items[j].Priority })

	if err := rt.Repo.ReplaceVulnChecklistItems(sid, vulnChecklistItemsToStore(items)); err != nil {
		log.Warnf("ssa_api_discovery: replace vuln_checklist_items: %v", err)
	}

	dir := filepath.Join(rt.WorkDir, store.SubDirName())
	_ = os.MkdirAll(dir, 0o755)
	checklistPath, err := ExportVulnChecklistJSON(rt)
	if err != nil {
		checklistPath = filepath.Join(dir, "vuln_checklist.json")
		log.Warnf("ssa_api_discovery: export vuln_checklist.json: %v", err)
	}
	pl.SetVulnChecklistPath(checklistPath)

	// sub-report
	reportPath := filepath.Join(dir, "step0_vuln_checklist.md")
	_ = os.WriteFile(reportPath, []byte(""), 0o644)
	pl.SetStep0ReportPath(reportPath)
	runPhase5StepReport(r, task, rt, pl, "step0_vuln_checklist", "Phase4 Step0: 静态扫描汇总与待检清单",
		fmt.Sprintf(`根据 discovery_read_session_data entity=vuln_checklist_items 中的待检清单数据，撰写中文 Markdown 报告。
内容：
1. 静态发现总数: %d
2. 已关联端点的发现数 / 未关联的发现数
3. 按严重度分布表格
4. 前 20 条高优先级待检项清单（含端点路径、漏洞类型、优先级）
5. 未关联到端点的高危发现列表（需人工关注）
`, len(items)),
		checklistPath, reportPath)

	// emit summary
	sevDist := map[string]int{}
	assocCount := 0
	for _, it := range items {
		sevDist[it.Severity]++
		if it.EndpointID > 0 || it.VerifiedHttpApiID > 0 {
			assocCount++
		}
	}
	metrics := map[string]int{
		"静态发现总数":  len(items),
		"关联到端点的发现": assocCount,
		"未关联发现":    len(items) - assocCount,
	}
	for sev, cnt := range sevDist {
		metrics[fmt.Sprintf("严重度-%s", sev)] = cnt
	}
	var warnings []string
	if len(items) == 0 {
		warnings = append(warnings, "syntaxflow_findings 为空：Step0 清单无待检项；Step3 灰盒扫描不依赖静态发现，仍将执行")
	}
	if len(unmatched) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d 个高危发现未关联到端点", len(unmatched)))
	}
	emitPhaseSummary(r, rt, pl, PhaseSummaryData{
		PhaseTitle:     "Phase4 Step0: 静态扫描汇总与待检清单",
		PhaseIndex:     4,
		TotalPhases:    pipelineSummaryTotalPhases,
		SubStepLabel:   "Step0",
		PhaseObjective: "汇总静态 SyntaxFlow 严重度分布与端点关联情况，梳理动态验证待检清单。",
		KeyMetrics:     metrics,
		Warnings:       warnings,
		NextStep:       "Step1: API鉴权探测",
	})
	rt.execStepEnd(step, "programmatic", started, []string{checklistPath, reportPath})
}

func severityToPriority(sev string) int {
	switch strings.ToLower(sev) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium", "mid", "middle":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func runPhase5StepReport(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask,
	rt *Runtime, pl *PipelineState, stepName, stepTitle, writePrompt, refFile, reportPath string) {

	step := fmt.Sprintf("phase4.report.%s", stepName)
	started := time.Now()
	rt.execStepStart(step, "ai")

	writePrompt = phase5EmbeddedReportPreamble(stepName, stepTitle) + writePrompt + ssaDiscoveryReportDataReadHint + ssaDiscoveryReportLanguageZH

	reportOpts := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(ssaDiscoveryMaxIterations(r)),
		reactloops.WithAllowUserInteract(false),
	}
	reportOpts = append(reportOpts, discoveryReportReadActionOptions(rt)...)
	reportOpts = append(reportOpts, reactloops.WithInitTask(func(innerLoop *reactloops.ReActLoop, innerTask aicommon.AIStatefulTask, innerOp *reactloops.InitTaskOperator) {
			bindDiscoveryRuntimeInLoop(innerLoop, rt)
			innerLoop.Set("report_filename", reportPath)
			innerLoop.Set("full_report_code", "")
			innerLoop.Set("user_requirements", writePrompt)
			innerLoop.Set("collected_references", "")
			innerLoop.Set("is_modify_mode", "false")
			var filesHint strings.Builder
			filesHint.WriteString("### 引用文件\n")
			if refFile != "" {
				filesHint.WriteString(fmt.Sprintf("- %s\n", refFile))
			}
			innerLoop.Set("available_files", filesHint.String()+buildPhase1ArtifactRefs(rt, store.DiscoverySnapshotPath(rt.WorkDir)))
			innerLoop.Set("available_knowledge_bases", "")
			innerOp.Continue()
		}))
	reportLoop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_REPORT_GENERATING,
		r,
		reportOpts...,
	)
	if err != nil {
		log.Warnf("phase5 %s report loop build: %v", stepName, err)
		rt.execStepError(step, "ai", started, err, []string{reportPath})
		return
	}
	sub := newSubTask(task, fmt.Sprintf("phase5_%s_report", stepName))
	if err := reportLoop.ExecuteWithExistedTask(sub); err != nil {
		log.Warnf("phase5 %s report: %v", stepName, err)
		rt.execStepError(step, "ai", started, err, []string{reportPath})
	} else {
		rt.execStepEnd(step, "ai", started, []string{reportPath})
	}
	if b, err := os.ReadFile(reportPath); err == nil && len(strings.TrimSpace(string(b))) < minStepReportBytes {
		fb := fmt.Sprintf("# %s\n\n## 阶段概览\n本报告由编排器自动兜底生成。\n\n%s\n", stepTitle, writePrompt)
		_ = os.WriteFile(reportPath, []byte(fb), 0o644)
	}
	if b, err := os.ReadFile(reportPath); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			r.EmitResultAfterStream(s)
		}
	}
	r.AddToTimeline(fmt.Sprintf("[ssa_phase5_%s]", stepName), fmt.Sprintf("report: %s", reportPath))
}
