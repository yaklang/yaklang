package loop_coordinator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

const instruction = `You coordinate a plan and its execution tasks. Read user information in Timeline.
Explore with read/search tools, save_evidence, and clarification. Call create_plan with a structured plan and plan_document, submit it for approval, then dispatch ready tasks.
Respond with native function calls. Prose, JSON actions and response-format JSON schemas cannot invoke actions.
Business commands, scripts and changes belong in pe_task workers. Coordinator tools permit bounded exploration and report writing only.
start_tasks returns immediately. inspect_tasks/wait_tasks deliver execution results. review_task accepts or rejects an inspected attempt based on its evidence and artifacts. Execution completion is not acceptance; dependencies start only after acceptance.
Modify a draft with its exact version and submit it again. Cancel and wait for active affected tasks before changing their briefs or retrying an upstream task.
Use PLAN STATUS for scheduling and the microscopic TODO list for your own next steps. Finish only after every approved task is accepted, all results observed and all TODOs resolved. Use write_report for artifact reports and directly_answer for messages. Both stay in this coordinator loop.`

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

var ActionNames = []string{"create_plan", "modify_plan", "submit_plan", "start_tasks", "inspect_tasks", "wait_tasks", "review_task", "retry_task", "cancel_tasks", "write_report"}

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
	version := aitool.WithIntegerParam("plan_version", aitool.WithParam_Description("Exact current draft version returned by create_plan/modify_plan."), aitool.WithParam_Required())
	ids := aitool.WithStringArrayParam("task_ids", aitool.WithParam_Description("Logical task IDs; omitted means all ready tasks (start), or all approved tasks (inspect/wait/cancel)."))
	attempt := aitool.WithIntegerParam("attempt_id", aitool.WithParam_Description("Exact attempt_id from the latest inspected result."), aitool.WithParam_Required())
	plan := aitool.WithStructParam("plan", []aitool.PropertyOption{aitool.WithParam_Required()},
		str("name", "Plan name."), str("goal", "Overall goal."),
		aitool.WithStructArrayParam("tasks", []aitool.PropertyOption{aitool.WithParam_Required()}, nil,
			str("name", "Task name."), str("goal", "Frozen execution brief."), str("identifier", "Stable, unique semantic identifier; retain it for unchanged tasks."), aitool.WithStringArrayParam("depends_on", aitool.WithParam_Description("Semantic identifiers of prerequisites; explicit [] means independent."))))
	definitions := []struct {
		name, description string
		options           []aitool.ToolOption
	}{
		{"create_plan", "Create and validate a draft; does not approve or dispatch it.", []aitool.ToolOption{plan, str("plan_document", "Stable Markdown plan document.")}},
		{"modify_plan", "Replace a draft at an exact version; approved work remains unchanged until submission.", []aitool.ToolOption{version, plan, str("plan_document", "Complete replacement Markdown document.")}},
		{"submit_plan", "Submit the draft through the existing user review channel and adopt the approved tree.", []aitool.ToolOption{version}},
		{"start_tasks", "Dispatch selected ready tasks without waiting; never dispatch unapproved dependencies.", []aitool.ToolOption{ids}},
		{"inspect_tasks", "Read task states, attempts, results and artifact/evidence references.", []aitool.ToolOption{ids}},
		{"wait_tasks", "Wait for results or a control change; timeout does not cancel tasks.", []aitool.ToolOption{ids, aitool.WithStringParam("mode", aitool.WithParam_Description("any (default): first update; all: all selected dispatched attempts settle. User input or plan changes interrupt either mode.")), aitool.WithIntegerParam("timeout_seconds", aitool.WithParam_Description("Default 30, maximum 60 seconds."))}},
		{"review_task", "Accept or reject a completed inspected attempt using its actual evidence and artifacts.", []aitool.ToolOption{str("task_id", "Logical task ID."), attempt, str("decision", "accept or reject"), str("reason", "Evidence-backed review conclusion, with artifact/evidence references.")}},
		{"retry_task", "Retry a settled attempt; invalidates downstream results. Active dependents must first stop.", []aitool.ToolOption{str("task_id", "Logical task ID."), attempt, str("reason", "Why the current attempt needs another execution.")}},
		{"cancel_tasks", "Request cancellation; cancelling is not settled until the worker has exited.", []aitool.ToolOption{ids, str("reason", "Reason for stopping selected tasks.")}},
		{"write_report", "Write a Markdown report artifact in this loop and publish the existing report_finish event.", []aitool.ToolOption{str("title", "Report title."), str("markdown", "Complete report content."), str("summary", "Short report summary for the existing UI.")}},
	}
	opts := make([]reactloops.ReActLoopOption, 0, len(definitions))
	for _, definition := range definitions {
		d := definition
		opts = append(opts, reactloops.WithRegisterLoopAction(d.name, d.description, d.options, validateActionParameters(d.name, d.options), func(loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
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
				op.Feedback(fmt.Sprintf("%s: %#v", d.name, value))
			}
			op.Continue()
		}))
	}
	return opts
}

// NewLoop uses native function calls and the shared Timeline assembly.
// The tool guard is per-loop, so execution workers retain their own tool policy.
func NewLoop(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	if cfg, ok := r.GetConfig().(*aicommon.Config); ok {
		_ = aicommon.WithLiteForgeExecutor(executeNativeHelper)(cfg)
		_ = aicommon.WithEnableFunctionCallMode(true)(cfg)
		_ = aicommon.WithAiAgreeRiskControl(NativeRiskReview)(cfg)
		_ = aicommon.WithDisableDynamicPlanning(true)(cfg)
	}
	preset := []reactloops.ReActLoopOption{
		reactloops.WithFunctionCallActionVariants(), reactloops.WithAllowToolCall(true), reactloops.WithAllowUserInteract(true), reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowAIForge(false),
		reactloops.WithPersistentContextProvider(func(l *reactloops.ReActLoop, _ string) (string, error) {
			if planningOnly(l) {
				return instruction + "\nThis invocation is planning-only: submit the draft for approval, then finish. Do not dispatch tasks or write the execution report.", nil
			}
			return instruction, nil
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
	// This is a protocol invariant, not a user/configurable fallback.
	preset = append(preset, reactloops.WithFunctionCallMode(true), reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true))
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
	answerCopy.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		l.GetEmitter().EmitTextMarkdownStreamEvent("re-act-loop-answer-payload", strings.NewReader(a.GetString("answer_payload")), "")
		op.Continue()
	}
	answerCopy.FunctionCallAction = nil
	answerCopy.Options = reactloops.NativeDirectlyAnswerOptions()
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
