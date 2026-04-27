package loop_syntaxflow_scan

import (
	"bytes"
	_ "embed"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var persistentInstruction string

//go:embed prompts/reactive_data.txt
var reactiveData string

//go:embed prompts/reflection_output_example.txt
var outputExample string

// interpretLoopName is a package-scoped sub-loop; no global schema constant required (same pattern as code_audit internal phases).
const interpretLoopName = "syntaxflow_scan_interpret"

const loopVarOrchestratorParentTaskID = "sf_orchestrator_parent_task_id"

const (
	// LoopVarInterpretLog 逐次解读/轮询的追加日志，供终局「完整报告」引用。
	LoopVarInterpretLog  = "sf_scan_interpret_log"
	loopVarInterpretTick = "sf_scan_interpret_tick_index"
)

// AppendSfScanInterpretLog 追加一条可持久化在 Loop 上的解读记录，并写一条节流到时间线。
func AppendSfScanInterpretLog(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, taskID, line string) {
	if loop == nil || line == "" {
		return
	}
	prev := loop.Get(LoopVarInterpretLog)
	tick := 0
	if ts := loop.Get(loopVarInterpretTick); ts != "" {
		tick, _ = strconv.Atoi(ts)
	}
	tick++
	loop.Set(loopVarInterpretTick, strconv.Itoa(tick))
	entry := fmt.Sprintf("[%s] #%d task_id=%s %s\n", time.Now().Format(time.RFC3339), tick, taskID, line)
	s := prev + entry
	const maxRune = 200000
	if len(s) > maxRune*4 {
		s = s[len(s)-maxRune*4:]
	}
	loop.Set(LoopVarInterpretLog, s)
	if r != nil {
		r.AddToTimeline("syntaxflow_scan", utils.ShrinkTextBlock(entry, 2000))
	}
}

const (
	// NodeIDSyntaxFlowStageReport 主对话区流式 Markdown（与 EVENT_TYPE_STRUCTURED 的 progress 节点区分）
	NodeIDSyntaxFlowStageReport = "syntaxflow_scan_stage_report"
)

var stageMarkdownOnce sync.Map

func dedupKey(taskID, phaseKey string) string {
	if taskID == "" {
		taskID = "_"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(taskID + "|" + phaseKey))
	return fmt.Sprintf("%x", h.Sum32())
}

// EmitSyntaxFlowStageMarkdown 向主对话区推送**引擎组装**的阶段性 Markdown（与 EmitSyntaxFlowScanPhase 的 JSON 进度互补）。
// parentTaskID 优先用编排父任务的 Id（P2 时尚未绑定 interpret 的 currentTask）；空则回退 GetCurrentTask()。
// phaseKey 用于去重（同 parentTask+phase 只发一次）。title/body 会截断。
func EmitSyntaxFlowStageMarkdown(loop *reactloops.ReActLoop, parentTaskID, phaseKey, title, body string) {
	if loop == nil {
		return
	}
	taskID := strings.TrimSpace(parentTaskID)
	if taskID == "" {
		if task := loop.GetCurrentTask(); task != nil {
			taskID = task.GetId()
		}
	}
	if _, loaded := stageMarkdownOnce.LoadOrStore(dedupKey(taskID, phaseKey), true); loaded {
		return
	}
	em := loop.GetEmitter()
	if em == nil {
		return
	}
	t := strings.TrimSpace(title)
	if t == "" {
		t = "SyntaxFlow 阶段"
	}
	b := strings.TrimSpace(body)
	if b == "" {
		b = "（无正文）"
	}
	doc := fmt.Sprintf("## %s\n\n%s\n", t, utils.ShrinkTextBlock(b, 12000))
	if _, err := em.EmitTextMarkdownStreamEvent(
		NodeIDSyntaxFlowStageReport,
		strings.NewReader(doc),
		taskID,
		func() {},
	); err != nil {
		log.Debugf("syntaxflow stage markdown: %v", err)
	}
}

// EmitSyntaxFlowUserStageMarkdown 用户向、章节化长文档：写入 `sfu.LoopVarSFUserStageLog` 并推主对话；`phaseKey` 须唯一以允许心跳等重复型帧去重不冲突（如 p1_hb_3）。
// fullDocument 可含以 `#` 开头的顶级标题，不再包一层 `##`。
func EmitSyntaxFlowUserStageMarkdown(loop *reactloops.ReActLoop, parentTaskID, phaseKey, fullDocument string) {
	if loop == nil {
		return
	}
	taskID := strings.TrimSpace(parentTaskID)
	if taskID == "" {
		if task := loop.GetCurrentTask(); task != nil {
			taskID = task.GetId()
		}
	}
	if _, loaded := stageMarkdownOnce.LoadOrStore(dedupKey(taskID, phaseKey), true); loaded {
		return
	}
	doc := strings.TrimSpace(fullDocument)
	if doc == "" {
		return
	}
	AppendUserStageLog(loop, doc)
	if len(doc) > 16000 {
		doc = utils.ShrinkTextBlock(doc, 16000) + "\n"
	} else {
		doc += "\n"
	}
	em := loop.GetEmitter()
	if em == nil {
		return
	}
	if _, err := em.EmitTextMarkdownStreamEvent(
		NodeIDSyntaxFlowStageReport,
		strings.NewReader(doc),
		taskID,
		func() {},
	); err != nil {
		log.Debugf("syntaxflow user stage markdown: %v", err)
	}
}

// EngineSnapshotBodyForInterpret 进入解读/报告物化时使用的**确定性**纯文本块（不依赖模型）。
func EngineSnapshotBodyForInterpret(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	return fmt.Sprintf(
		"- **task_id / runtime_id**: %s\n"+
			"- **session_mode**（attach/解读说明）: %s\n"+
			"- **config 推断**（sf_scan_config_inferred 1=路径推断）: %s\n"+
			"- **sf_scan_final_report_due**（1=终局大报告）: %s\n"+
			"- **sf_scan_risk_converged**（1=风险侧可成稿）: %s\n\n"+
			"### 各阶段用户向累计 `sf_scan_user_stage_log`（截断）\n```\n%s\n```\n\n"+
			"### 编译/管线 `sf_scan_compile_meta`\n```\n%s\n```\n\n"+
			"### pipeline 摘要 `sf_scan_pipeline_summary`\n```\n%s\n```\n\n"+
			"### 扫描行终态 `sf_scan_scan_end_summary`（若已有）\n```\n%s\n```\n\n"+
			"### preface 头（截断）\n```\n%s\n```\n\n"+
			"### risk 列表头（若已有）\n- total_hint: %s\n```\n%s\n```\n",
		loop.Get(sfu.LoopVarSyntaxFlowTaskID),
		loop.Get(sfu.LoopVarSyntaxFlowScanSessionMode),
		loop.Get("sf_scan_config_inferred"),
		loop.Get(sfu.LoopVarSFFinalReportDue),
		loop.Get(sfu.LoopVarSFRiskConverged),
		utils.ShrinkTextBlock(loop.Get(sfu.LoopVarSFUserStageLog), 8000),
		utils.ShrinkTextBlock(loop.Get(sfu.LoopVarSFCompileMeta), 2000),
		utils.ShrinkTextBlock(loop.Get(sfu.LoopVarSFPipelineSummary), 8000),
		utils.ShrinkTextBlock(loop.Get(sfu.LoopVarSFScanEndSummary), 4000),
		utils.ShrinkTextBlock(loop.Get("sf_scan_review_preface"), 8000),
		loop.Get("ssa_risk_total_hint"),
		utils.ShrinkTextBlock(loop.Get("ssa_risk_list_summary"), 8000),
	)
}

const (
	minDirectAnswerWhenFinalReport = 2000
	maxDirectAnswerFirstIterShort  = 500
)

// loopActionDirectlyAnswerSyntaxflowScan 解读子环的 directly_answer：防止首轮不调用工具就短答结束；终局时要求足够篇幅。
var loopActionDirectlyAnswerSyntaxflowScan = &reactloops.LoopAction{
	ActionType:  "directly_answer",
	Description: "Directly answer; for final merged report when sf_scan_final_report_due=1 use a long body or <|FINAL_ANSWER|> tag. Before tools on iter 0, do not use only a short status line.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"answer_payload",
			aitool.WithParam_Description("Short payload only if not final report; for long Markdown leave empty and use <|FINAL_ANSWER|> tag. Mutually exclusive with FINAL_ANSWER tag."),
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
	ActionVerifier: directlyAnswerSyntaxflowScanVerifier,
	ActionHandler:  directlyAnswerSyntaxflowScanHandler,
}

func directAnswerPayloadText(loop *reactloops.ReActLoop, action *aicommon.Action) string {
	payload := action.GetString("answer_payload")
	if payload == "" {
		payload = action.GetInvokeParams("next_action").GetString("answer_payload")
	}
	if payload == "" {
		return strings.TrimSpace(loop.Get("tag_final_answer"))
	}
	return strings.TrimSpace(payload)
}

func directlyAnswerSyntaxflowScanVerifier(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	payload := directAnswerPayloadText(loop, action)
	if payload == "" {
		return utils.Error("answer_payload or FINAL_ANSWER tag is required for directly_answer but both are empty")
	}
	iter := loop.GetCurrentIterationIndex()
	finalDue := strings.TrimSpace(loop.Get(sfu.LoopVarSFFinalReportDue)) == "1"
	if finalDue && len([]rune(payload)) < minDirectAnswerWhenFinalReport {
		return utils.Errorf("sf_scan_final_report_due=1 时终局大报告须至少 %d 字（当前 %d）。请用 reload_* 取全量后，用 Markdown 分节长文或 <|FINAL_ANSWER|> 流式输出完整报告。",
			minDirectAnswerWhenFinalReport, len([]rune(payload)))
	}
	if !finalDue && iter == 0 {
		used := strings.TrimSpace(loop.Get("sf_interpret_tool_used")) == "1"
		if !used && len([]rune(payload)) < maxDirectAnswerFirstIterShort {
			return utils.Error("首轮过短且尚未调用 reload_syntaxflow_scan_session / reload_ssa_risk_overview / set_ssa_risk_review_target 之一。请先使用工具以 DB 数据为准，或输出长文（约 ≥500 字）。")
		}
	}
	loop.Set("directly_answer_payload", payload)
	return nil
}

func directlyAnswerSyntaxflowScanHandler(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
	invoker := loop.GetInvoker()
	payload := loop.Get("directly_answer_payload")
	if payload == "" {
		payload = strings.TrimSpace(loop.Get("tag_final_answer"))
	}
	if payload == "" {
		operator.Fail("directly_answer: empty payload")
		return
	}
	invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
	invoker.EmitResultAfterStream(payload)
	invoker.AddToTimeline("directly_answer", fmt.Sprintf("user input: \n%s\nai directly answer:\n%v",
		utils.PrefixLines(loop.GetCurrentTask().GetUserInput(), "  > "),
		utils.PrefixLines(payload, "  | "),
	))
	operator.Exit()
}

// WithReloadSSARiskOverviewAction registers reload_ssa_risk_overview: re-query SSA risks with structured filter/limit and refresh reactive preface fields.
func WithReloadSSARiskOverviewAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"reload_ssa_risk_overview",
		"Re-run the SSA risk list query using the SSA project database and update ssa_risk_overview_preface / ssa_risk_list_summary / ssa_risk_total_hint. Without parameters, reuses the last effective filter stored on the loop (ssa_overview_filter_json) or falls back to attachments and loop vars like the init task. Use filter_json for a full ypb.SSARisksFilter (protojson); runtime_id accepts comma-separated SSA runtime ids.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("limit", aitool.WithParam_Description("Max number of risk rows to sample for the preface (default 40).")),
			aitool.WithStringParam("search", aitool.WithParam_Description("Fuzzy search string; sets SSARisksFilter.Search when non-empty.")),
			aitool.WithStringParam("runtime_id", aitool.WithParam_Description("SSA runtime id(s), comma-separated; each non-empty token is merged into SSARisksFilter.RuntimeID.")),
			aitool.WithStringParam("program_name", aitool.WithParam_Description("Program name; merged into SSARisksFilter.ProgramName when non-empty.")),
			aitool.WithStringParam("filter_json", aitool.WithParam_Description("Full SSARisksFilter as JSON (google.protobuf JSON / protojson). When set, used as the base filter before applying search/runtime_id/program_name overrides.")),
		},
		func(_ *reactloops.ReActLoop, _ *aicommon.Action) error {
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			task := operator.GetTask()
			db := sfu.GetSSADB()
			filter := MergeReloadSSARiskOverviewFilter(loop, task, action)
			limit := int64(action.GetInt("limit"))
			summary := ApplySSARiskOverviewDB(loop, r, db, task, filter, limit)
			operator.Feedback(fmt.Sprintf("[reload_ssa_risk_overview] updated overview context (%d runes).\n%s", len([]rune(summary)), summary))
			operator.Continue()
		},
	)
}

// WithReloadSyntaxFlowScanSessionAction registers reload_syntaxflow_scan_session: reload scan task + SSA risk sample for a task_id from DB.
func WithReloadSyntaxFlowScanSessionAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"reload_syntaxflow_scan_session",
		"Load SyntaxFlowScanTask and a sample of SSA risks for the given task_id (SSA runtime id) from the database, then refresh sf_scan_review_preface, sf_scan_task_id, and sf_scan_session_mode=attach. Equivalent to a successful attach path in the syntaxflow_scan init task.",
		[]aitool.ToolOption{
			aitool.WithStringParam("task_id", aitool.WithParam_Description("SyntaxFlow scan task id (UUID), same as SSA Risk runtime_id for that scan."), aitool.WithParam_Required(true)),
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetString("task_id") == "" {
				return utils.Error("task_id is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			taskID := action.GetString("task_id")
			db := sfu.GetSSADB()
			if db == nil {
				r.AddToTimeline("syntaxflow_scan", "reload_syntaxflow_scan_session: 无 SSA 数据库连接")
				operator.Feedback("reload_syntaxflow_scan_session failed: SSA database not available")
				operator.Continue()
				return
			}
			res, err := LoadScanSessionResult(db, taskID, DefaultRiskSampleLimit)
			if err != nil {
				log.Warnf("[syntaxflow_scan] reload LoadScanSessionResult: %v", err)
				r.AddToTimeline("syntaxflow_scan", fmt.Sprintf("reload failed task_id=%s: %v", taskID, err))
				operator.Feedback(fmt.Sprintf("reload_syntaxflow_scan_session failed: %v", err))
				operator.Continue()
				return
			}
			loop.Set("sf_scan_task_id", taskID)
			loop.Set("sf_scan_session_mode", "attach")
			preface := "下列信息来自数据库（扫描任务 + 该 runtime 下 SSA Risk 列表），仅可在此基础上解读；不得编造未列出的 risk id。\n\n" + res.Preface
			loop.Set("sf_scan_review_preface", preface)
			AppendSfScanInterpretLog(loop, r, taskID, "reload_syntaxflow_scan_session: 已刷新任务与 risk 样本")
			r.AddToTimeline("syntaxflow_scan", utils.ShrinkTextBlock(preface, 4000))
			operator.Feedback(preface)
			operator.Continue()
		},
	)
}

// WithSetSSARiskReviewTargetAction registers set_ssa_risk_review_target: switch the focused SSA risk id mid-session without new attachments.
func WithSetSSARiskReviewTargetAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"set_ssa_risk_review_target",
		"Set the active SSA risk primary key for this ssa_risk_review loop (loop var ssa_risk_id). After changing, use the ssa-risk tool with the new risk_id before drawing conclusions.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("risk_id", aitool.WithParam_Description("SSA Risk database id (positive integer)."), aitool.WithParam_Required(true)),
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetInt("risk_id") <= 0 {
				return utils.Error("risk_id must be a positive integer")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			id := int64(action.GetInt("risk_id"))
			loop.Set(sfu.LoopVarSSARiskID, fmt.Sprintf("%d", id))
			invoker := loop.GetInvoker()
			msg := fmt.Sprintf("目标 SSA Risk ID 已切换为 %d。请先使用 ssa-risk 工具拉取该条（risk_id=%d, get_full_code 视需要设为 true）。", id, id)
			invoker.AddToTimeline("ssa_risk_review", msg)
			operator.Feedback(msg)
			operator.Continue()
		},
	)
}

// SFAuditCodeSearchHint is appended to SyntaxFlow code-audit rule prompts so the model greps the tree before writing rules.
func SFAuditCodeSearchHint() string {
	return `

【代码搜索 / 专注阅读】在编写或修改 SyntaxFlow 规则前，请使用 grep、read_file、find_file 在已探索的项目路径内缩小 Source/Sink 与框架入口，避免仅凭猜测写规则；优先在可疑目录（handler、controller、router）上缩小范围后再 grep。`
}

// buildInterpretEngineInitTask 首帧进入解读子环前推送**引擎快照** Markdown，再 Continue（不替代多轮 ReAct）。
func buildInterpretEngineInitTask(r aicommon.AIInvokeRuntime) func(*reactloops.ReActLoop, aicommon.AIStatefulTask, *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		parentID := strings.TrimSpace(loop.Get(loopVarOrchestratorParentTaskID))
		if parentID == "" && task != nil {
			parentID = task.GetId()
		}
		EmitSyntaxFlowStageMarkdown(loop, parentID, "p3_interpret_engine_init", "阶段3·解读子环（引擎快照）", EngineSnapshotBodyForInterpret(loop))
		r.AddToTimeline("syntaxflow_scan", "interpret: 引擎快照已推送到对话区 / engine snapshot markdown emitted")
		op.Continue()
	}
}

// buildInterpretPostIterationHook 记录是否已使用专用工具，供 directly_answer 校验；可选轻量「中间发现」时间线。
func buildInterpretPostIterationHook(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		if isDone {
			return
		}
		la := loop.GetLastAction()
		if la == nil {
			return
		}
		switch la.ActionType {
		case "reload_syntaxflow_scan_session", "reload_ssa_risk_overview", "set_ssa_risk_review_target":
			loop.Set("sf_interpret_tool_used", "1")
			r.AddToTimeline("syntaxflow_scan", fmt.Sprintf("interpret iter %d: tool %s 已执行", iteration+1, la.ActionType))
			// 与 http_flow 的 FINDINGS 累积类似：简短可并入「中间发现」键，供终局与报告输入引用
			prev := strings.TrimSpace(loop.Get("sf_scan_findings_doc"))
			line := fmt.Sprintf("### 迭代 %d\n- 工具: `%s`\n\n", iteration+1, la.ActionType)
			if prev == "" {
				loop.Set("sf_scan_findings_doc", line)
			} else {
				loop.Set("sf_scan_findings_doc", prev+"\n"+line)
			}
		}
	})
}

// buildPhaseInterpretLoop builds the ReAct loop for risk interpretation, reload tools, and optional final report.
func buildPhaseInterpretLoop(r aicommon.AIInvokeRuntime, extra ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	preset := []reactloops.ReActLoopOption{
		reactloops.WithInitTask(buildInterpretEngineInitTask(r)),
		buildInterpretPostIterationHook(r),
		reactloops.WithOverrideLoopAction(loopActionDirectlyAnswerSyntaxflowScan),
		reactloops.WithAllowRAG(true),
		reactloops.WithAllowToolCall(true),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
		reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
		reactloops.WithPersistentInstruction(persistentInstruction),
		reactloops.WithReflectionOutputExample(outputExample + sfu.ReflectionOutputSharedAppendix),
		WithReloadSyntaxFlowScanSessionAction(r),
		WithReloadSSARiskOverviewAction(r),
		WithSetSSARiskReviewTargetAction(r),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			fb := strings.TrimSpace(feedbacker.String())
			return utils.RenderTemplate(reactiveData, map[string]any{
				"Preface":          loop.Get("sf_scan_review_preface"),
				"TaskID":           loop.Get("sf_scan_task_id"),
				"SessionMode":      loop.Get("sf_scan_session_mode"),
				"ConfigInferred":   loop.Get("sf_scan_config_inferred"),
				"FinalReportDue":   loop.Get(sfu.LoopVarSFFinalReportDue),
				"CompileMeta":      loop.Get(sfu.LoopVarSFCompileMeta),
				"PipelineSummary":  loop.Get(sfu.LoopVarSFPipelineSummary),
				"ScanEndSummary":   loop.Get(sfu.LoopVarSFScanEndSummary),
				"FindingsDoc":      loop.Get("sf_scan_findings_doc"),
				"InterpretLog":     loop.Get(LoopVarInterpretLog),
				"RiskListSummary":  loop.Get("ssa_risk_list_summary"),
				"RiskTotalHint":    loop.Get("ssa_risk_total_hint"),
				"FeedbackMessages": fb,
				"Nonce":            nonce,
			})
		}),
	}
	preset = append(preset, extra...)
	return reactloops.NewReActLoop(interpretLoopName, r, preset...)
}
