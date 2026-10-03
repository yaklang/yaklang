package coordinator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var instruction = promptloader.MustLoad("ai/aid/coordinator/instruction.txt")
var planningOnlyInstruction = promptloader.MustLoad("ai/aid/coordinator/planning_only.txt")

func WithController(c *Controller) reactloops.ReActLoopOption {
	return func(loop *reactloops.ReActLoop) { loop.Set("coordinator_controller", c) }
}

// WithPlanningOnly retains the old plan-only entrypoint without adding a
// separate planner loop. Its finish gate requires approval, not execution.
func WithPlanningOnly() reactloops.ReActLoopOption {
	return func(loop *reactloops.ReActLoop) { loop.Set("coordinator_planning_only", true) }
}

func planningOnly(loop *reactloops.ReActLoop) bool {
	value, _ := loop.GetVariable("coordinator_planning_only").(bool)
	return value
}

func controller(loop *reactloops.ReActLoop) *Controller {
	c, _ := loop.GetVariable("coordinator_controller").(*Controller)
	return c
}

var ActionNames = []string{"create_plan", "modify_plan", "inspect_plan", "submit_plan", "start_tasks", "inspect_tasks", "wait_tasks", "review_task", "retry_task", "cancel_tasks", "write_report"}

// Providers can return malformed native arguments. Validate the advertised
// types before getters coerce them (e.g. null task_ids must not become all tasks).
func validateActionParameters(name string, options []aitool.ToolOption) reactloops.LoopActionVerifierFunc {
	tool := aitool.NewWithoutCallback(name, options...)
	return func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
		params := action.GetParams()
		delete(params, "@action")
		if valid, problems := tool.ValidateParams(params); !valid {
			return fmt.Errorf("invalid %s arguments: %s", name, strings.Join(problems, "; "))
		}
		return nil
	}
}

func actionOptions() []reactloops.ReActLoopOption {
	str := func(name, description string) aitool.ToolOption {
		return aitool.WithStringParam(name, aitool.WithParam_Description(description), aitool.WithParam_Required())
	}
	version := aitool.WithIntegerParam("plan_version", aitool.WithParam_Description("create_plan 或 modify_plan 返回的当前草案版本，必须精确匹配。"), aitool.WithParam_Required())
	ids := aitool.WithStringArrayParam("task_ids", aitool.WithParam_Description("逻辑任务 ID 列表；start_tasks 省略时选择全部可派发任务，inspect/wait/cancel 省略时选择全部已批准任务。"))
	attempt := aitool.WithIntegerParam("attempt_id", aitool.WithParam_Description("最近一次已观察结果中的 attempt_id，必须精确匹配。"), aitool.WithParam_Required())
	plan := planParameter()
	definitions := []struct {
		name, description string
		options           []aitool.ToolOption
	}{
		{"create_plan", "创建并校验计划草案；不批准、不派发任务。", []aitool.ToolOption{plan, str("plan_document", "稳定的 Markdown 计划文档。")}},
		{"modify_plan", "按精确版本完整替换草案；新版本获批前，原已批准计划继续有效。", []aitool.ToolOption{version, plan, str("plan_document", "完整替换后的 Markdown 计划文档。")}},
		{"inspect_plan", "读取完整草案和已批准计划，包括版本、文档和任务 DAG；不批准、不修改计划。", nil},
		{"submit_plan", "提交草案供用户审核并采用批准的任务树；重复提交同一已批准版本不再次审核。批准后调用 start_tasks 自动推进执行。", []aitool.ToolOption{version}},
		{"start_tasks", "立即派发选定的可执行任务，不等待完成；仅执行已批准且前置任务均已验收的任务。", []aitool.ToolOption{ids}},
		{"inspect_tasks", "读取任务状态、执行尝试、结果及 artifacts/Evidence 引用，并标记结果已被观察。", []aitool.ToolOption{ids}},
		{"wait_tasks", "等待任务结果或控制变化；超时不取消任务。", []aitool.ToolOption{ids, aitool.WithStringParam("mode", aitool.WithParam_Description("any（默认）：等待首次更新；all：等待选中的已派发尝试全部结算。用户补充或计划变更会唤醒两种等待。")), aitool.WithIntegerParam("timeout_seconds", aitool.WithParam_Description("默认 30 秒，最多 60 秒。"))}},
		{"review_task", "依据真实 Evidence 和 artifacts，接受或拒绝已结束且已观察的执行尝试。", []aitool.ToolOption{str("task_id", "逻辑任务 ID。"), attempt, str("decision", "accept 表示接受；reject 表示拒绝。"), str("reason", "有证据支持的验收结论，包含 artifacts/Evidence 引用。")}},
		{"retry_task", "重试已结算的尝试并使下游结果失效；仍在运行的受影响任务必须先停止。", []aitool.ToolOption{str("task_id", "逻辑任务 ID。"), attempt, str("reason", "当前尝试需要重新执行的原因。")}},
		{"cancel_tasks", "请求取消任务；worker 实际退出前 cancelling 不代表任务已经停止。", []aitool.ToolOption{ids, str("reason", "停止选定任务的原因。")}},
		{"write_report", "在当前协调循环写入 Markdown 报告产物，并发布兼容的 report_finish 事件。", []aitool.ToolOption{str("title", "报告标题。"), str("markdown", "完整报告正文。"), str("summary", "供现有界面展示的报告摘要。")}},
	}
	opts := make([]reactloops.ReActLoopOption, 0, len(definitions))
	for _, definition := range definitions {
		d := definition
		opts = append(opts, registerAction(d.name, d.description, d.options, func(loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			c := controller(loop)
			if c == nil {
				op.Fail("coordinator controller missing")
				return
			}
			var value any
			var err error
			ids := a.GetStringSlice("task_ids")
			version := uint64(a.GetInt("plan_version"))
			attempt := uint64(a.GetInt("attempt_id"))
			switch d.name {
			case "create_plan":
				value, err = c.CreatePlan(op.GetContext(), planData(a), a.GetString("plan_document"))
			case "modify_plan":
				value, err = c.ModifyPlan(op.GetContext(), version, planData(a), a.GetString("plan_document"))
			case "inspect_plan":
				s := c.Snapshot()
				value = map[string]any{"draft_version": s.DraftVersion, "approved_version": s.ApprovedVersion, "submitted_version": s.SubmittedVersion, "draft": s.Draft, "approved": s.Approved}
			case "submit_plan":
				err = c.SubmitPlan(op.GetContext(), version)
				s := c.Snapshot()
				value = map[string]uint64{"draft_version": s.DraftVersion, "approved_version": s.ApprovedVersion}
			case "start_tasks":
				if planningOnly(loop) {
					err = fmt.Errorf("this plan-only run must finish after approval; execution starts through the existing approved-plan entrypoint")
					break
				}
				value, err = c.StartTasks(ids)
			case "inspect_tasks":
				value, err = c.InspectTasks(ids)
			case "wait_tasks":
				observed, _ := loop.GetVariable("coordinator_observed_user_revision").(uint64)
				if observed != c.Snapshot().UserRevision {
					var tasks []Attempt
					tasks, err = c.InspectTasks(ids)
					value = WaitResult{Reason: "changed", Tasks: tasks}
					break
				}
				value, err = c.WaitTasksMode(op.GetContext(), ids, time.Duration(a.GetInt("timeout_seconds"))*time.Second, a.GetString("mode"))
			case "review_task":
				err = c.ReviewTask(a.GetString("task_id"), attempt, a.GetString("decision"), a.GetString("reason"))
			case "retry_task":
				value, err = c.RetryTask(a.GetString("task_id"), attempt, a.GetString("reason"))
			case "cancel_tasks":
				err = c.CancelTasks(ids, a.GetString("reason"))
			case "write_report":
				if strings.TrimSpace(a.GetString("markdown")) == "" {
					err = fmt.Errorf("report content is empty")
					break
				}
				path := loop.GetInvoker().EmitFileArtifactWithExt("coordinator-report", ".md", a.GetString("markdown"))
				if path == "" {
					err = fmt.Errorf("report artifact could not be written")
					break
				}
				loop.Set("coordinator_report_path", path)
				value = path
				loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_REPORT_FINISH, "report-finish", map[string]any{"report_path": path, "title": a.GetString("title"), "summary_markdown": a.GetString("summary")})
			}
			if err != nil {
				op.Feedback(fmt.Sprintf("%s rejected: %v", d.name, err))
			} else {
				encoded, _ := json.Marshal(value)
				op.Feedback(fmt.Sprintf("%s: %s", d.name, encoded))
			}
			op.Continue()
		}))
	}
	return opts
}

// registerAction supplies separate text-stream and native function definitions,
// with one Chinese parameter contract and one execution/validation path.
func registerAction(name, description string, options []aitool.ToolOption, handler reactloops.LoopActionHandlerFunc) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(name, description+"文本流模式通过 @action 选择本动作，参数遵循当前 JSON Schema。", options, nil, validateActionParameters(name, options), handler, func(action *reactloops.LoopAction) {
		action.NativeDescription = description + "原生调用通过函数名选择本动作，参数写入 arguments。"
		action.NativeOptions = append([]aitool.ToolOption{}, options...)
	})
}

// NewLoop uses the main loop's configured action protocol and Timeline assembly.
// The tool guard is per-loop, so execution workers retain their own tool policy.
func NewLoop(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	// These are planning preferences, scoped to this coordinator's role
	// instructions (semi-dynamic 2), never worker instructions or high static.
	var preferences strings.Builder
	if global := yakit.GetCachedAIGlobalConfig(); global != nil && strings.TrimSpace(global.GetAIPlanPrompt()) != "" {
		fmt.Fprintf(&preferences, "\n<planning_preferences>\n%s\n</planning_preferences>\n", global.GetAIPlanPrompt())
	}
	if cfg, ok := r.GetConfig().(*aicommon.Config); ok && strings.TrimSpace(cfg.PlanPrompt) != "" {
		fmt.Fprintf(&preferences, "\n<user_planning_preferences>\n%s\n</user_planning_preferences>\n", cfg.PlanPrompt)
	}
	if cfg, ok := r.GetConfig().(*aicommon.Config); ok {
		_ = aicommon.WithLiteForgeExecutor(executeNativeHelper)(cfg)
		_ = aicommon.WithAiAgreeRiskControl(NativeRiskReview)(cfg)
		_ = aicommon.WithDisableDynamicPlanning(true)(cfg)
	}
	preset := []reactloops.ReActLoopOption{
		reactloops.WithFunctionCallActionVariants(), reactloops.WithAllowToolCall(true), reactloops.WithAllowUserInteract(true), reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowAIForge(false),
		reactloops.WithPersistentContextProvider(func(l *reactloops.ReActLoop, _ string) (string, error) {
			text, err := utils.RenderTemplate(instruction, map[string]any{"FunctionCallMode": l.FunctionCallModeEnabled()})
			if err != nil {
				return "", err
			}
			if controller(l).Snapshot().Approved == nil {
				text += preferences.String()
			}
			if planningOnly(l) {
				return text + "\n" + planningOnlyInstruction, nil
			}
			return text, nil
		}),
		reactloops.WithReactiveDataBuilder(func(_ *reactloops.ReActLoop, b *bytes.Buffer, _ string) (string, error) { return b.String(), nil }),
		reactloops.WithToolInvokeGuard(func(name string, params aitool.InvokeParams) (bool, string) {
			workdir := ""
			if cfg, ok := r.GetConfig().(*aicommon.Config); ok {
				workdir = cfg.GetOrCreateWorkDir()
			}
			if AllowedTool(name, params, workdir) {
				return true, ""
			}
			return false, "Coordinator cannot execute this tool. Put business execution in a plan task."
		}),
		reactloops.WithActionFilter(func(a *reactloops.LoopAction) bool {
			for _, name := range ActionNames {
				if a.ActionType == name {
					return true
				}
			}
			switch a.ActionType {
			case "finish", "directly_answer", "save_evidence", "adjust_todolist", "require_tool", "directly_call_tool", "ask_for_clarification", "knowledge_enhance_answer", "load_skills", "change_skill_view_offset", "load_skill_resources", "search_capabilities":
				return true
			}
			return false
		}),
	}
	preset = append(preset, actionOptions()...)
	preset = append(preset, opts...)
	preset = append(preset, reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true))
	loop, err := reactloops.NewReActLoop(Name, r, preset...)
	if err != nil {
		return nil, err
	}
	if controller(loop) == nil {
		return nil, fmt.Errorf("coordinator requires an owning Session or WithController")
	}

	// Preserve the existing TODO/goal/subagent completion gates as well.
	reactloops.WithPlanStatusProvider(func() string {
		c := controller(loop)
		s := c.Snapshot()
		loop.Set("coordinator_observed_user_revision", s.UserRevision)
		return s.PromptStatus()
	})(loop)
	finish, err := loop.GetActionHandler("finish")
	if err != nil {
		return nil, err
	}
	wrapped := *finish
	original := finish.ActionHandler
	wrapped.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		check := controller(l).CanFinish
		if planningOnly(l) {
			check = controller(l).CanFinishPlanning
		}
		if err := check(); err != nil {
			op.Feedback(fmt.Sprintf("finish rejected: %v", err))
			op.Continue()
			return
		}
		requireReport := false
		if cfg, ok := l.GetConfig().(*aicommon.Config); ok {
			requireReport = cfg.GenerateReport
		}
		if host, ok := controller(l).host.(interface{ ReportRequired() bool }); ok {
			requireReport = host.ReportRequired()
		}
		if !planningOnly(l) && requireReport && l.Get("coordinator_report_path") == "" {
			op.Feedback("write_report is required before finishing this PLAN")
			op.Continue()
			return
		}
		gate := reactloops.NewActionHandlerOperator(op.GetTask())
		original(l, a, gate)
		op.Feedback(gate.GetFeedback().String())
		if done, err := gate.IsTerminated(); !done {
			op.Continue()
			return
		} else if err != nil {
			op.Fail(err)
			return
		}
		observed, _ := l.GetVariable("coordinator_observed_user_revision").(uint64)
		finalize := controller(l).Finalize
		if planningOnly(l) {
			finalize = controller(l).FinalizePlanning
		}
		if err := finalize(observed); err != nil {
			op.Feedback(err.Error())
			op.Continue()
			return
		}
		op.Exit()
	}
	reactloops.WithOverrideLoopAction(&wrapped)(loop)
	answer, err := loop.GetActionHandler("directly_answer")
	if err != nil {
		return nil, err
	}
	answerCopy := *answer
	answerCopy.Description = "向用户说明进展或答复，之后继续协调。短答复使用 answer_payload；长答复使用主循环声明的 FINAL_ANSWER AITAG，两者不可同时输出。"
	answerCopy.NativeDescription = "向用户说明进展或答复，之后继续协调；完整正文写入 arguments.answer_payload，不使用外置 AITAG。"
	answerCopy.Options = []aitool.ToolOption{aitool.WithStringParam("answer_payload", aitool.WithParam_Description("短答复正文；长答复使用主循环声明的 FINAL_ANSWER AITAG，两者不可同时输出。"))}
	answerCopy.NativeOptions = []aitool.ToolOption{aitool.WithStringParam("answer_payload", aitool.WithParam_Description("向用户交付的完整答复，支持 Markdown 或代码；不使用外置 AITAG。"), aitool.WithParam_Required())}
	answerCopy.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		if !l.FunctionCallModeEnabled() {
			answer.ActionHandler(l, a, op)
			return
		}
		l.GetEmitter().EmitTextMarkdownStreamEvent("re-act-loop-answer-payload", strings.NewReader(a.GetString("answer_payload")), "")
		op.Continue()
	}
	answerCopy.FunctionCallAction = nil
	reactloops.WithOverrideLoopAction(&answerCopy)(loop)
	return loop, nil
}

func planData(a *aicommon.Action) string {
	data, _ := json.Marshal(a.GetInvokeParams("plan"))
	return string(data)
}

func init() {
	_ = reactloops.RegisterLoopFactory(Name, NewLoop,
		reactloops.WithLoopDescription("Plan coordination: explore, approve, dispatch, wait, review, replan and report through compatible PLAN channels."),
		reactloops.WithVerboseName("Coordinator"), reactloops.WithVerboseNameZh("任务协调"),
		reactloops.WithLoopDescriptionZh("计划协调运行体：探索、审批、调度、等待、验收、重试和报告，兼容已有 PLAN 与 Yakit 通道。"))
}
