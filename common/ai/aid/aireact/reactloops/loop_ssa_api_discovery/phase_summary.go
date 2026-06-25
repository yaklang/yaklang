package loop_ssa_api_discovery

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const pipelineSummaryTotalPhases = 5

// phase2ReportHeadingTitle 阶段 2 报告首行标题用语（与 discovery_report.md 制式一致，避免重复「阶段 2」前缀）。
const phase2ReportHeadingTitle = "API 与架构分析"

// phase2PipelineNextStep 阶段 1 总结「下一步」指向阶段 2 时的用语。
const phase2PipelineNextStep = "阶段 2：API 与架构分析"

// ssaDiscoveryReportLanguageZH 对用户可见 Markdown（directly_answer、落盘报告）的中文与章节约束，供各阶段 prompt 拼接。
const ssaDiscoveryReportLanguageZH = `

## 语言与格式（对用户可见输出，须遵守）
- **使用简体中文**撰写正文与章节标题；除代码块、文件路径、URL、HTTP 状态码、工具名、API 路径、专有名词外，**不要使用英文段落或英文章节标题**。
- 禁止使用如 Execution Summary、Key Findings、Next Steps、Pipeline Stage、Status 等英文标题；请改为「执行摘要」「主要发现」「后续步骤」「阶段小结」「状态」等中文。
`

// ssaDiscoveryDirectlyAnswerTitleBlock 与 Phase2 落盘报告相同的「首行一级标题」制式说明，拼入各阶段持久指令；由模型在 directly_answer / FINAL_ANSWER 中原样输出，不再由运行时插包头。
func ssaDiscoveryDirectlyAnswerTitleBlock(phaseIndex int, phaseTitle, subStepLabel string) string {
	total := pipelineSummaryTotalPhases
	title := strings.TrimSpace(phaseTitle)
	sub := strings.TrimSpace(subStepLabel)
	var line string
	if phaseIndex > 0 && sub != "" {
		line = fmt.Sprintf("# [阶段 %d/%d - %s] %s 完成报告", phaseIndex, total, sub, title)
	} else if phaseIndex > 0 {
		line = fmt.Sprintf("# [阶段 %d/%d] %s 完成报告", phaseIndex, total, title)
	} else {
		line = fmt.Sprintf("# %s 完成报告", title)
	}
	return fmt.Sprintf(`

# 对用户可见报告（directly_answer / <|FINAL_ANSWER|>）格式（与 Phase2 discovery_report 制式一致）
报告**首行一级标题**必须为以下一行（勿在其前再写其他一级标题）：
%s

第二行起撰写完整 Markdown 正文；须包含小节 **## 阶段概览**（本阶段在流水线中的编号、名称与结论要点）。随后按需分节（如执行摘要、主要发现、后续步骤等），通篇简体中文。
`, line)
}

func phase5ReportFirstLineTitle(stepName, stepTitle string) string {
	sub := "Step1"
	switch stepName {
	case "step1_auth":
		sub = "Step1"
	case "step2_verify":
		sub = "Step2"
	case "step3_greybox":
		sub = "Step3"
	}
	return fmt.Sprintf("# [阶段 4/%d - %s] %s 完成报告", pipelineSummaryTotalPhases, sub, stepTitle)
}

func phase5EmbeddedReportPreamble(stepName, stepTitle string) string {
	return fmt.Sprintf("报告**首行一级标题**必须为以下一行（勿在其前再写其他一级标题）：\n%s\n\n", phase5ReportFirstLineTitle(stepName, stepTitle))
}

// PhaseSummaryData 阶段结束时的统一制式总结（由 emitPhaseSummary 渲染为 Markdown）。
type PhaseSummaryData struct {
	PhaseTitle string
	Duration   time.Duration
	KeyMetrics map[string]int
	Highlights []string
	Warnings   []string
	NextStep   string

	PhaseIndex       int    // 主阶段序号 1–5；Phase4 子步骤仍为 4
	TotalPhases      int    // 默认 0 时按 6 处理
	SubStepLabel     string // 如 Step0、Step1；主阶段留空
	PhaseObjective   string // 本阶段目标（一句话）
	Status           string // 完成 / 有警告 / 异常；空则按规则推导
}

func derivePhaseSummaryStatus(s PhaseSummaryData) string {
	st := strings.TrimSpace(s.Status)
	if st != "" {
		return st
	}
	nw, nh := len(s.Warnings), len(s.Highlights)
	if nw > 0 && nh > 0 {
		return "有警告"
	}
	if nw > 0 {
		return "异常"
	}
	return "完成"
}

func buildPhaseSummaryMarkdown(summary PhaseSummaryData) string {
	total := summary.TotalPhases
	if total <= 0 {
		total = pipelineSummaryTotalPhases
	}

	var titleLine string
	idx := summary.PhaseIndex
	if idx > 0 {
		sub := strings.TrimSpace(summary.SubStepLabel)
		if sub != "" {
			titleLine = fmt.Sprintf("# [阶段 %d/%d - %s] %s 完成报告", idx, total, sub, summary.PhaseTitle)
		} else {
			titleLine = fmt.Sprintf("# [阶段 %d/%d] %s 完成报告", idx, total, summary.PhaseTitle)
		}
	} else {
		titleLine = fmt.Sprintf("# %s 完成报告", summary.PhaseTitle)
	}

	status := derivePhaseSummaryStatus(summary)
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(titleLine)
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("> 耗时: %s | 状态: %s\n", formatPhaseDuration(summary.Duration), status))
	sb.WriteString("\n")

	if obj := strings.TrimSpace(summary.PhaseObjective); obj != "" {
		sb.WriteString("## 目标\n")
		sb.WriteString(obj)
		sb.WriteString("\n\n")
	}

	if len(summary.KeyMetrics) > 0 {
		sb.WriteString("## 关键指标\n")
		sb.WriteString("| 指标 | 值 |\n")
		sb.WriteString("|------|----|\n")
		keys := make([]string, 0, len(summary.KeyMetrics))
		for k := range summary.KeyMetrics {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", strings.ReplaceAll(k, "|", "\\|"), summary.KeyMetrics[k]))
		}
		sb.WriteString("\n")
	}

	if len(summary.Highlights) > 0 {
		sb.WriteString("## 成果\n")
		for _, h := range summary.Highlights {
			sb.WriteString(fmt.Sprintf("- %s\n", h))
		}
		sb.WriteString("\n")
	}

	if len(summary.Warnings) > 0 {
		sb.WriteString("## 警告\n")
		for _, w := range summary.Warnings {
			sb.WriteString(fmt.Sprintf("- [!] %s\n", w))
		}
		sb.WriteString("\n")
	}

	if ns := strings.TrimSpace(summary.NextStep); ns != "" {
		sb.WriteString("## 下一步\n")
		sb.WriteString(fmt.Sprintf("> %s\n", ns))
	}

	sb.WriteString("---\n")
	return sb.String()
}

func formatPhaseDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	return d.Round(time.Second).String()
}

// buildPhaseDirectlyAnswerOverride 覆盖默认 directly_answer；阶段标题由持久指令中的制式要求模型写入正文首行（与 Phase2 报告一致），此处不做包头拼接。
// streamToUser 为 true 时（阶段 1 子循环）将答案推送到用户界面并发文件产物；Phase5 等可设为 false，仅落盘与时间线。
func buildPhaseDirectlyAnswerOverride(phaseIndex int, phaseTitle, subStepLabel string, streamToUser bool) reactloops.ReActLoopOption {
	return reactloops.WithOverrideLoopAction(&reactloops.LoopAction{
		ActionType: "directly_answer",
		Description: fmt.Sprintf(
			"Answer via answer_payload or FINAL_ANSWER。首行标题与结构须遵守持久指令「对用户可见报告」制式（阶段 %s）；通篇简体中文。",
			phaseTitle,
		),
		Options: []aitool.ToolOption{
			aitool.WithStringParam(
				"answer_payload",
				aitool.WithParam_Description(`USE THIS FIELD ONLY IF @action is 'directly_answer' AND answer is short (≤200 chars). For long answers, leave this empty and use '<|FINAL_ANSWER_...|>' tags after JSON. CRITICAL: answer_payload and <|FINAL_ANSWER_...|> are STRICTLY MUTUALLY EXCLUSIVE - never use both simultaneously.`),
			),
		},
		AITagStreamFields: []*reactloops.LoopAITagField{
			{
				TagName:      "FINAL_ANSWER",
				VariableName: "tag_final_answer",
				AINodeId:     "re-act-loop-answer-payload",
				ContentType:  aicommon.TypeTextMarkdown,
			},
		},
		StreamFields: []*reactloops.LoopStreamField{
			{
				FieldName:   "answer_payload",
				AINodeId:    "re-act-loop-answer-payload",
				ContentType: aicommon.TypeTextMarkdown,
			},
		},
		ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			payload := action.GetString("answer_payload")
			if payload == "" {
				payload = action.GetInvokeParams("next_action").GetString("answer_payload")
			}
			if payload == "" {
				tagPayload := loop.Get("tag_final_answer")
				if tagPayload != "" {
					payload = tagPayload
				}
			}
			if payload == "" {
				return utils.Error("answer_payload is required for ActionDirectlyAnswer but empty")
			}
			loop.Set("directly_answer_payload", payload)
			return nil
		},
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			invoker := loop.GetInvoker()
			payload := loop.Get(`directly_answer_payload`)
			if payload == "" {
				payload = loop.Get("tag_final_answer")
			}
			if payload == "" {
				operator.Fail("directly_answer action must have 'answer_payload' field")
				return
			}
			invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
			if streamToUser {
				invoker.EmitResultAfterStream(payload)
			}
			invoker.AddToTimeline("directly_answer", fmt.Sprintf("user input: \n"+
				"%s\n"+
				"ai directly answer:\n"+
				"%v",
				utils.PrefixLines(loop.GetCurrentTask().GetUserInput(), "  > "),
				utils.PrefixLines(payload, "  | "),
			))
			operator.Exit()
		},
	})
}

func emitPhaseSummary(r aicommon.AIInvokeRuntime, rt *Runtime, pl *PipelineState, summary PhaseSummaryData) {
	text := buildPhaseSummaryMarkdown(summary)
	r.EmitResultAfterStream(text)
	r.AddToTimeline("[pipeline_summary]", text)
	if pl != nil {
		pl.AppendSummaryLog(text)
	}
	log.Infof("ssa_api_discovery: %s", summary.PhaseTitle)
}

func collectPhase1Summary(rt *Runtime, elapsed time.Duration) PhaseSummaryData {
	s := PhaseSummaryData{
		PhaseTitle:     "Phase1: API 发现与请求验证",
		Duration:       elapsed,
		KeyMetrics:     map[string]int{},
		NextStep:       phase2PipelineNextStep,
		PhaseIndex:     1,
		TotalPhases:    pipelineSummaryTotalPhases,
		PhaseObjective: "Yak 预分析、代码通读、ReAct 探测验证，落库 verified_http_apis",
	}
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return s
	}
	sess := rt.Session
	eps, _ := rt.Repo.ListHttpEndpoints(sess.ID)
	comps, _ := rt.Repo.ListComponents(sess.ID)
	secm, _ := rt.Repo.ListSecurityMechanisms(sess.ID)
	gate, _ := rt.Repo.CountVerifiedHttpApiGate(sess.ID)

	s.KeyMetrics["候选HTTP端点"] = len(eps)
	s.KeyMetrics["已确认API"] = gate.Verified
	s.KeyMetrics["API验证记录"] = gate.Total
	s.KeyMetrics["已拒绝API"] = gate.Rejected
	s.KeyMetrics["路由候选"] = countRouteCandidates(rt)
	s.KeyMetrics["架构组件"] = len(comps)
	s.KeyMetrics["安全机制"] = len(secm)

	if sess.Language != "" {
		s.Highlights = append(s.Highlights, fmt.Sprintf("识别语言: %s", sess.Language))
	}
	if sess.SSACompileOK {
		s.Highlights = append(s.Highlights, fmt.Sprintf("SSA编译成功 (程序: %s, 文件: %d)", sess.SSAProgramName, sess.SSAFileCount))
	} else if sess.SSACompileError != "" {
		s.Warnings = append(s.Warnings, fmt.Sprintf("SSA编译失败: %s", sess.SSACompileError))
	}
	if sess.TargetReachable {
		s.Highlights = append(s.Highlights, fmt.Sprintf("目标可达: %s", EffectiveTargetBaseURL(sess)))
	} else if sess.TargetRaw != "" {
		s.Warnings = append(s.Warnings, fmt.Sprintf("目标不可达: %s", sess.TargetRaw))
	}
	return s
}

// collectPhase1BlockSummary 阶段 1 结束后的合并总结（含 Phase1A/B/C），仅应 emit 一次。
func collectPhase1BlockSummary(rt *Runtime, blockElapsed time.Duration, authSkipped bool, authSkipReason string, authElapsed time.Duration) PhaseSummaryData {
	base := collectPhase1Summary(rt, blockElapsed)
	base.PhaseTitle = "阶段 1：API 发现与请求验证"
	base.NextStep = phase2PipelineNextStep
	base.PhaseObjective = "Phase1A 并发预分析、Phase1B 代码通读、Phase1C 探测子循环写入 verified_http_apis（已合并原路由校准/鉴权/HTTP 初验）。"
	base.SubStepLabel = ""
	base.Duration = blockElapsed
	base.Highlights = append(base.Highlights, "HTTP 初验已在 Phase1 完成，无独立 Phase3 全量 coverage 子循环")
	if authSkipped && authSkipReason != "" {
		base.Highlights = append(base.Highlights, "鉴权: "+authSkipReason)
	} else if rt != nil && rt.Repo != nil && rt.Session != nil {
		verified, _ := rt.Repo.ListVerifiedAuthCredentials(rt.Session.ID)
		if len(verified) > 0 {
			base.Highlights = append(base.Highlights, fmt.Sprintf("已验证鉴权凭证 %d 条", len(verified)))
		}
	}
	_ = authElapsed
	return base
}

func collectPhase4Summary(rt *Runtime, elapsed time.Duration) PhaseSummaryData {
	s := PhaseSummaryData{
		PhaseTitle:     "Phase3: SyntaxFlow静态规则扫描",
		Duration:       elapsed,
		KeyMetrics:     map[string]int{},
		NextStep:       "Phase4: 动态漏洞验证与检测",
		PhaseIndex:     3,
		TotalPhases:    pipelineSummaryTotalPhases,
		PhaseObjective: "使用 SyntaxFlow 规则进行静态代码扫描；HTTP 端点确认以 verified_http_apis 为准",
	}
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return s
	}
	s.Highlights = append(s.Highlights, "静态扫描基于 SSA，HTTP 端点确认以 verified_http_apis 为准")
	findings, _ := rt.Repo.ListDiscoverySyntaxFlowFindings(rt.Session.ID)
	s.KeyMetrics["静态发现总数"] = len(findings)

	sevCount := map[string]int{}
	for _, f := range findings {
		sev := strings.ToLower(f.Severity)
		if sev == "" {
			sev = "unknown"
		}
		sevCount[sev]++
	}
	for sev, cnt := range sevCount {
		s.KeyMetrics[fmt.Sprintf("严重度-%s", sev)] = cnt
	}

	if meta, err := ParseSyntaxFlowScanMeta(rt.Session.SyntaxFlowScanMetaJSON); err == nil && meta != nil {
		s.KeyMetrics["扫描规则数"] = meta.RulesQueued
		s.Highlights = append(s.Highlights, fmt.Sprintf("扫描来源: %s", meta.Source))
	}
	return s
}

func collectStep1Summary(rt *Runtime) PhaseSummaryData {
	s := PhaseSummaryData{
		PhaseTitle:     "Phase4 Step1: API鉴权探测",
		KeyMetrics:     map[string]int{},
		NextStep:       "Step2: 静态发现动态验证",
		PhaseIndex:     4,
		TotalPhases:    pipelineSummaryTotalPhases,
		SubStepLabel:   "Step1",
		PhaseObjective: "检查并刷新/API 探测鉴权凭证，保证后续动态验证可携带有效身份。",
	}
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return s
	}
	creds, _ := rt.Repo.ListAuthCredentials(rt.Session.ID)
	s.KeyMetrics["获取凭证数"] = len(creds)
	verified := 0
	for _, c := range creds {
		if c.Verified {
			verified++
		}
	}
	s.KeyMetrics["已验证凭证"] = verified
	if verified == 0 && len(creds) > 0 {
		s.Warnings = append(s.Warnings, "获取了凭证但未通过验证")
	}
	if len(creds) == 0 {
		s.Warnings = append(s.Warnings, "未获取到任何鉴权凭证，后续验证将以匿名方式进行")
	}
	return s
}

func collectStep2Summary(rt *Runtime) PhaseSummaryData {
	s := PhaseSummaryData{
		PhaseTitle:     "Phase4 Step2: 静态发现动态验证",
		KeyMetrics:     map[string]int{},
		NextStep:       step3SummaryNextStep(rt),
		PhaseIndex:     4,
		TotalPhases:    pipelineSummaryTotalPhases,
		SubStepLabel:   "Step2",
		PhaseObjective: "基于静态 SyntaxFlow 等待项，对靶机做有限动态验证，记录 vuln_verifications。",
	}
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return s
	}
	vv, _ := rt.Repo.ListVulnVerifications(rt.Session.ID)
	s.KeyMetrics["验证总数"] = len(vv)
	statusCount := map[string]int{}
	for _, v := range vv {
		statusCount[v.Status]++
	}
	for st, cnt := range statusCount {
		s.KeyMetrics[fmt.Sprintf("状态-%s", st)] = cnt
	}
	return s
}

func step3SummaryNextStep(rt *Runtime) string {
	if rt != nil && rt.Phase4Mode() == Phase4ModeBatchScan {
		return "Step3: 灰盒漏洞批量检测"
	}
	return "Step3: 深度挖掘漏洞检测"
}

func collectStep3Summary(rt *Runtime) PhaseSummaryData {
	title := "Phase4 Step3: 深度挖掘漏洞检测"
	objective := "对每个 probe-ready 接口独立 ReAct，覆盖全量 vuln_type 并写入 endpoint_vuln_probes。"
	if rt != nil && rt.Phase4Mode() == Phase4ModeBatchScan {
		title = "Phase4 Step3: 灰盒漏洞批量检测"
		objective = "对 HTTP 端点执行 AI 增强灰盒批量检测，写入 dynamic_vuln_findings。"
	}
	s := PhaseSummaryData{
		PhaseTitle:     title,
		KeyMetrics:     map[string]int{},
		NextStep:       "Phase5: 最终报告",
		PhaseIndex:     4,
		TotalPhases:    pipelineSummaryTotalPhases,
		SubStepLabel:   "Step3",
		PhaseObjective: objective,
	}
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return s
	}
	df, _ := rt.Repo.ListDynamicVulnFindings(rt.Session.ID)
	if rt.Phase4Mode() == Phase4ModeBatchScan {
		s.KeyMetrics["灰盒发现总数"] = len(df)
	} else {
		s.KeyMetrics["动态发现总数"] = len(df)
		if n, err := rt.Repo.CountEndpointVulnProbes(rt.Session.ID, 0); err == nil {
			s.KeyMetrics["endpoint_vuln_probes"] = n
		}
	}
	statusCount := map[string]int{}
	typeCount := map[string]int{}
	for _, d := range df {
		statusCount[d.Status]++
		typeCount[d.VulnType]++
	}
	for st, cnt := range statusCount {
		s.KeyMetrics[fmt.Sprintf("状态-%s", st)] = cnt
	}
	for vt, cnt := range typeCount {
		s.KeyMetrics[fmt.Sprintf("类型-%s", vt)] = cnt
	}
	return s
}

func collectPhase5Summary(rt *Runtime, elapsed time.Duration) PhaseSummaryData {
	s := PhaseSummaryData{
		PhaseTitle:     "Phase4: 动态漏洞验证与检测",
		Duration:       elapsed,
		KeyMetrics:     map[string]int{},
		NextStep:       "Phase5: 生成最终审计报告",
		PhaseIndex:     4,
		TotalPhases:    pipelineSummaryTotalPhases,
		PhaseObjective: "执行动态漏洞验证：鉴权刷新、静态发现验证、灰盒批量检测",
	}
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return s
	}
	sid := rt.Session.ID
	vv, _ := rt.Repo.ListVulnVerifications(sid)
	ac, _ := rt.Repo.ListAuthCredentials(sid)
	df, _ := rt.Repo.ListDynamicVulnFindings(sid)

	s.KeyMetrics["鉴权凭证"] = len(ac)
	s.KeyMetrics["静态验证记录"] = len(vv)
	s.KeyMetrics["灰盒发现"] = len(df)

	confirmed := 0
	for _, v := range vv {
		if v.Status == "confirmed" {
			confirmed++
		}
	}
	s.KeyMetrics["已确认漏洞(静态验证)"] = confirmed

	dynConfirmed := 0
	for _, d := range df {
		if d.Status == "confirmed" {
			dynConfirmed++
		}
	}
	s.KeyMetrics["已确认漏洞(灰盒)"] = dynConfirmed
	return s
}
