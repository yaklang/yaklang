package loop_ssa_api_discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var phase4PersistentInstruction = strings.TrimSpace(`你是 **SyntaxFlow 静态规则扫描策略** 助手。目标：结合项目语言、攻击面发现结论，为 Phase3 构造 **Profile 库中规则** 的过滤条件（以 filter 为主：language / severity / tag / keyword，可选仅内置规则、是否排除库依赖规则），然后执行扫描。

## 约束
- 每轮先输出带 **@action** 的 JSON。
- 用 **discovery_get_status** 查看 language、ssa_program_name、代码路径与 Phase1 计数。
- 编排器对单次 **discovery_run_syntaxflow_scan** 使用**独立超长超时**（默认 90m），避免父任务流式 context 先结束出现 **scan_error=client canceled**；可用环境变量 **YAK_SSA_API_DISCOVERY_SYNTAXFLOW_TIMEOUT**（time.ParseDuration 格式，例如 2h、120m）调整。
- 用 **discovery_preview_syntaxflow_rules** 预估当前 filter 会命中多少条规则（含 rule_name 样例），调整 filter 直到合理（避免过宽拖慢、过窄 0 条）。
- 确认后调用 **discovery_run_syntaxflow_scan** 执行扫描（与 preview 相同的参数语义）。仅当 plan 合理时再运行。
- 若无 SSA 程序（ssa_ok=false），可直接 **directly_answer** 说明无法扫描。
- 完成后 **directly_answer**：概述选用的 filter 思路、预估/实际规则规模；**勿**在答案中罗列全部规则名（完整列表写入 syntaxflow_summary.json 的 rule_names）。
- **对用户可见输出（directly_answer / FINAL_ANSWER）须全中文**：章节标题须中文，禁止 Execution Summary、Key Findings 等英文标题；除路径、工具名、规则标识外避免英文段落。

## Filter 参数说明（discovery_* 工具）
- **languages**：逗号分隔，小写惯用名（如 java,go,javascript）；可留空则默认用会话 language。
- **severities**：逗号分隔严重级别（与规则库一致，如 high,critical）。
- **tags**：逗号分隔。
- **keyword**：在规则名/标题/描述等中模糊搜索。
- **builtin_only**：true 时只扫内置规则（推荐与安全基线场景）。
- **exclude_lib_rules**：true 时排除 allow_included 类库规则（FilterLibRuleKind=noLib）。
`) + ssaDiscoveryDirectlyAnswerTitleBlock(3, "Phase3: SyntaxFlow静态规则扫描", "")

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func buildSyntaxFlowRuleFilterFromAction(action *aicommon.Action, sess *store.DiscoverySession) (*ypb.SyntaxFlowRuleFilter, []string) {
	f := &ypb.SyntaxFlowRuleFilter{}
	var warnings []string
	sessionLang := ""
	if sess != nil {
		sessionLang = strings.ToLower(strings.TrimSpace(sess.Language))
	}
	if langs := splitCSV(action.GetString("languages")); len(langs) > 0 {
		if sessionLang != "" && !languageListMatchesSession(langs, sessionLang) {
			warnings = append(warnings, fmt.Sprintf(
				"ignored AI languages=%q; locked to session language %q",
				strings.Join(langs, ","), sessionLang,
			))
		} else {
			for _, l := range langs {
				f.Language = append(f.Language, strings.ToLower(l))
			}
		}
	}
	if len(f.Language) == 0 {
		if sessionLang != "" {
			f.Language = []string{sessionLang}
		}
	}
	if sev := splitCSV(action.GetString("severities")); len(sev) > 0 {
		f.Severity = sev
	}
	if tags := splitCSV(action.GetString("tags")); len(tags) > 0 {
		f.Tag = tags
	}
	if kw := strings.TrimSpace(action.GetString("keyword")); kw != "" {
		f.Keyword = kw
	}
	if action.GetBool("builtin_only", false) {
		f.FilterRuleKind = yakit.FilterBuiltinRuleTrue
	}
	if action.GetBool("exclude_lib_rules", false) {
		f.FilterLibRuleKind = yakit.FilterLibRuleFalse
	}
	return f, warnings
}

func languageListMatchesSession(langs []string, sessionLang string) bool {
	sessionLang = strings.ToLower(strings.TrimSpace(sessionLang))
	for _, l := range langs {
		if strings.EqualFold(strings.TrimSpace(l), sessionLang) {
			return true
		}
	}
	return false
}

func verifyPhase4ScanAllowsFinish(loop *reactloops.ReActLoop) error {
	rt := getRuntime(loop)
	if rt == nil || rt.Session == nil {
		return nil
	}
	if !rt.Session.SSACompileOK || strings.TrimSpace(rt.Session.SSAProgramName) == "" {
		return nil
	}
	if strings.TrimSpace(loop.Get("syntaxflow_scan_done")) != "1" {
		return utils.Error("Phase4：请先调用 discovery_run_syntaxflow_scan 完成 SyntaxFlow 扫描，或使用 preview 确认 0 条规则时仍须执行 run 以落库元数据，再 directly_answer。")
	}
	return nil
}

var phase4DirectlyAnswerAction = &reactloops.LoopAction{
	ActionType: "directly_answer",
	Description: "Phase4 小结（略述 filter 与规则规模）。完整 rule_names 见 syntaxflow_summary.json。FINAL_ANSWER 须简体中文与中文章节标题。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam("answer_payload", aitool.WithParam_Description("简要中文总结。")),
	},
	AITagStreamFields: []*reactloops.LoopAITagField{
		{TagName: "FINAL_ANSWER", VariableName: "tag_final_answer", AINodeId: "re-act-loop-answer-payload", ContentType: aicommon.TypeTextMarkdown},
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: "answer_payload", AINodeId: "re-act-loop-answer-payload", ContentType: aicommon.TypeTextMarkdown},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		if err := verifyPhase4ScanAllowsFinish(loop); err != nil {
			return err
		}
		payload := action.GetString("answer_payload")
		if payload == "" {
			payload = action.GetInvokeParams("next_action").GetString("answer_payload")
		}
		if payload == "" {
			if tag := loop.Get("tag_final_answer"); tag != "" {
				payload = tag
			}
		}
		if payload == "" {
			return utils.Error("answer_payload required")
		}
		loop.Set("directly_answer_payload", payload)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		invoker := loop.GetInvoker()
		payload := loop.Get("directly_answer_payload")
		if payload == "" {
			payload = loop.Get("tag_final_answer")
		}
		if payload == "" {
			operator.Fail("empty answer")
			return
		}
		invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
		invoker.EmitResultAfterStream(payload)
		invoker.AddToTimeline("directly_answer", fmt.Sprintf("phase4_syntaxflow\n%s", utils.ShrinkString(payload, 4000)))
		operator.Exit()
	},
}

var phase4FinishOverride = &reactloops.LoopAction{
	ActionType: "finish",
	Description: "结束 Phase4（需已完成 discovery_run_syntaxflow_scan；无 SSA 时可提前结束）。",
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		if err := verifyPhase4ScanAllowsFinish(loop); err != nil {
			operator.Feedback(err.Error())
			operator.Continue()
			return
		}
		loop.GetInvoker().AddToTimeline("finish", "phase4_syntaxflow finished")
		operator.Exit()
	},
}

func syntaxflowToolParams() []aitool.ToolOption {
	return []aitool.ToolOption{
		aitool.WithStringParam("languages", aitool.WithParam_Description("逗号分隔，如 java,spring；空则使用会话 language")),
		aitool.WithStringParam("severities", aitool.WithParam_Description("逗号分隔严重级别")),
		aitool.WithStringParam("tags", aitool.WithParam_Description("逗号分隔标签")),
		aitool.WithStringParam("keyword", aitool.WithParam_Description("关键词模糊搜规则元数据")),
		aitool.WithBoolParam("builtin_only", aitool.WithParam_Description("仅内置规则")),
		aitool.WithBoolParam("exclude_lib_rules", aitool.WithParam_Description("排除库依赖类规则 (noLib)")),
	}
}

func buildDiscoveryPreviewSyntaxFlowRules() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_preview_syntaxflow_rules",
		"根据 filter 参数预估将参与 SyntaxFlow 扫描的规则数量，并返回至多 40 条 rule_name 样例（全量在 run 后写入 syntaxflow_summary.json）。",
		syntaxflowToolParams(),
		func(l *reactloops.ReActLoop, action *aicommon.Action) error { return nil },
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			_, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			f, langWarnings := buildSyntaxFlowRuleFilterFromAction(action, sess)
			names, err := queryRuleNamesForFilter(f)
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			sample := names
			if len(sample) > 40 {
				sample = sample[:40]
			}
			out := map[string]any{
				"rules_matched":  len(names),
				"sample_names":   sample,
				"filter_summary": filterToSummary(f),
			}
			if len(langWarnings) > 0 {
				out["language_warnings"] = langWarnings
			}
			b, _ := json.MarshalIndent(out, "", "  ")
			op.Feedback(string(b))
			op.Continue()
		},
	)
}

func buildDiscoveryRunSyntaxFlowScan(pl *PipelineState) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_run_syntaxflow_scan",
		"按 filter 执行 SyntaxFlow 扫描（写入 SSARisk → discovery_syntaxflow_findings、会话 syntax_flow_scan_meta_json、syntaxflow_summary.json 含完整 rule_names）。",
		syntaxflowToolParams(),
		func(l *reactloops.ReActLoop, action *aicommon.Action) error { return nil },
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}
			if utils.IsNil(ctx) {
				ctx = context.Background()
			}
			if !sess.SSACompileOK || strings.TrimSpace(sess.SSAProgramName) == "" {
				if err := RunSyntaxFlowScan(ctx, rt, pl, nil, "tool_skip_no_ssa"); err != nil {
					op.Feedback(fmt.Sprintf("skip: %v", err))
				} else {
					op.Feedback("skipped: no SSA program; summary + meta written")
				}
				loop.Set("syntaxflow_scan_done", "1")
				op.Continue()
				return
			}
			f, langWarnings := buildSyntaxFlowRuleFilterFromAction(action, sess)
			if err := RunSyntaxFlowScan(ctx, rt, pl, f, "ai"); err != nil {
				op.Feedback(fmt.Sprintf("run scan: %v", err))
				op.Continue()
				return
			}
			meta, _ := ParseSyntaxFlowScanMeta(rt.Session.SyntaxFlowScanMetaJSON)
			loop.Set("syntaxflow_scan_done", "1")
			if meta != nil {
				loop.Set("syntaxflow_rules_queued", fmt.Sprintf("%d", meta.RulesQueued))
				loop.Set("syntaxflow_risks_imported", fmt.Sprintf("%d", meta.RisksImported))
			}
			fb := map[string]any{}
			if meta != nil {
				fb["rules_queued"] = meta.RulesQueued
				fb["risks_imported"] = meta.RisksImported
				fb["source"] = meta.Source
			}
			if len(langWarnings) > 0 {
				fb["language_warnings"] = langWarnings
			}
			b, _ := json.MarshalIndent(fb, "", "  ")
			op.Feedback("scan ok: " + string(b))
			op.Continue()
		},
	)
}

func buildPhase4SyntaxFlowLoop(r aicommon.AIInvokeRuntime, rt *Runtime, pl *PipelineState) (*reactloops.ReActLoop, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}
	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(ssaDiscoveryMaxIterations(r)),
		reactloops.WithAllowToolCall(true),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithPersistentInstruction(phase4PersistentInstruction),
		reactloops.WithPeriodicVerificationInterval(1000),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			metaLine := ""
			if rt.Session != nil && rt.Session.SyntaxFlowScanMetaJSON != "" {
				m, _ := ParseSyntaxFlowScanMeta(rt.Session.SyntaxFlowScanMetaJSON)
				if m != nil {
					metaLine = fmt.Sprintf("last_scan: rules_queued=%d risks_imported=%d source=%s executed=%v",
						m.RulesQueued, m.RisksImported, m.Source, m.Executed)
				}
			}
			preanalysisHints := readPreanalysisHintsForReactive(rt)
			return fmt.Sprintf(`<|REACTIVE_PHASE4_%s|>
session_uuid: %s
sqlite: %s
ssa_program_name: %s
language: %s
ssa_ok: %v
%s
discovery_report: %s
syntaxflow_summary_path: %s
%s

上轮反馈:
%s
<|END_%s|>`,
				nonce, pl.SessionUUID, pl.SQLitePath,
				rt.Session.SSAProgramName, rt.Session.Language, rt.Session.SSACompileOK,
				preanalysisHints,
				pl.GetDiscoveryReportPath(), pl.GetSyntaxFlowJSONPath(),
				metaLine,
				feedbacker.String(), nonce), nil
		}),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			setRuntime(loop, rt)
			loop.Set("discovery_session_uuid", rt.Session.UUID)
			loop.Set("discovery_sqlite_path", rt.SQLitePath)
			loop.Set("discovery_phase", rt.Session.Phase)
			loop.Set("syntaxflow_scan_done", "")
			op.Continue()
		}),
		buildDiscoveryGetStatus(),
		buildDiscoveryPreviewSyntaxFlowRules(),
		buildDiscoveryRunSyntaxFlowScan(pl),
		reactloops.WithOverrideLoopAction(phase4DirectlyAnswerAction),
		reactloops.WithOverrideLoopAction(phase4FinishOverride),
	}
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_PHASE4_SYNTAXFLOW, r, preset...)
}

func phase4EnsureSyntaxFlowScan(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, pl *PipelineState) {
	if rt == nil || rt.Session == nil {
		return
	}
	ctx := task.GetContext()
	if utils.IsNil(ctx) {
		ctx = context.Background()
	}
	reactStart := time.Now()
	rt.execStepStart("phase3.syntaxflow_react", "ai")
	phase4, err := buildPhase4SyntaxFlowLoop(r, rt, pl)
	if err == nil {
		if err2 := phase4.ExecuteWithExistedTask(newSubTask(task, "phase4_syntaxflow")); err2 != nil {
			rt.execStepError("phase3.syntaxflow_react", "ai", reactStart, err2, nil)
			log.Warnf("ssa_api_discovery: phase4 loop: %v", err2)
		} else {
			rt.execStepEnd("phase3.syntaxflow_react", "ai", reactStart, []string{store.SyntaxflowSummaryPath(rt.WorkDir)})
		}
	} else {
		rt.execStepError("phase3.syntaxflow_react", "ai", reactStart, err, nil)
		log.Warnf("ssa_api_discovery: phase4 build loop: %v", err)
	}
	reloadRuntimeSession(rt)
	fallbackStart := time.Now()
	rt.execStepStart("phase3.syntaxflow_fallback", "programmatic")
	tryPhase4FallbackScans(ctx, rt, pl)
	reloadRuntimeSession(rt)
	rt.execStepEnd("phase3.syntaxflow_fallback", "programmatic", fallbackStart, []string{store.SyntaxflowSummaryPath(rt.WorkDir)})
	if strings.TrimSpace(rt.Session.SyntaxFlowScanMetaJSON) == "" {
		ensureStart := time.Now()
		rt.execStepStart("phase3.syntaxflow_ensure", "programmatic")
		if err := RunSyntaxFlowScan(ctx, rt, pl, nil, "ensure_summary"); err != nil {
			rt.execStepError("phase3.syntaxflow_ensure", "programmatic", ensureStart, err, []string{store.SyntaxflowSummaryPath(rt.WorkDir)})
		} else {
			rt.execStepEnd("phase3.syntaxflow_ensure", "programmatic", ensureStart, []string{store.SyntaxflowSummaryPath(rt.WorkDir)})
		}
		reloadRuntimeSession(rt)
	}
}

func tryPhase4FallbackScans(ctx context.Context, rt *Runtime, pl *PipelineState) {
	if rt == nil || rt.Session == nil {
		return
	}
	if !rt.Session.SSACompileOK || strings.TrimSpace(rt.Session.SSAProgramName) == "" {
		return
	}
	if syntaxflowScanNeedsFallback(rt) {
		_ = RunSyntaxFlowScan(ctx, rt, pl, FallbackSyntaxFlowRuleFilter(rt.Session), "fallback_language_builtin")
		reloadRuntimeSession(rt)
	}
	if syntaxflowScanNeedsFallback(rt) {
		_ = RunSyntaxFlowScan(ctx, rt, pl, nil, "fallback_full_table")
		reloadRuntimeSession(rt)
	}
}
