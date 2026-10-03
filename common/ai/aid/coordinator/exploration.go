package coordinator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func explorationEnabled(l *reactloops.ReActLoop) bool {
	cfg, ok := l.GetConfig().(*aicommon.Config)
	return ok && cfg.EnableSubagentsInPlan && controller(l).Snapshot().Phase == PhasePlan
}

func configureExploration(loop *reactloops.ReActLoop) error {
	// The same policy protects schema handlers and direct SubmitSubAgents calls.
	reactloops.WithSubAgentSubmissionPolicy(func(jobs []reactloops.SubAgentJob, _ reactloops.SubAgentOptions) (reactloops.SubAgentOptions, error) {
		if !explorationEnabled(loop) {
			return reactloops.SubAgentOptions{}, fmt.Errorf("PLAN exploration is disabled")
		}
		c := controller(loop)
		c.mu.Lock()
		err := c.checkLocked()
		locked := c.state.ReviewPending || c.reviewing
		c.mu.Unlock()
		if err != nil {
			return reactloops.SubAgentOptions{}, err
		}
		if locked {
			return reactloops.SubAgentOptions{}, fmt.Errorf("plan is locked for review")
		}
		for _, job := range jobs {
			if job.LoopName != "" && job.LoopName != schema.AI_REACT_LOOP_NAME_DEFAULT {
				return reactloops.SubAgentOptions{}, fmt.Errorf("exploration must use the default investigation loop")
			}
		}
		return reactloops.SubAgentOptions{TimelineMode: reactloops.SubAgentTimelineFork, ConfigureLoop: func(child *reactloops.ReActLoop) { configureInvestigator(child, loop) }}, nil
	})(loop)
	c := controller(loop)
	c.mu.Lock()
	c.explorationCheck = func() error {
		if m := loop.GetSubAgentManager(); m != nil {
			if reason := m.FinishBlockReason(loop.SubAgentModelSeenRevision(), false); reason != "" {
				return fmt.Errorf("cannot submit plan: %s", reason)
			}
		}
		return nil
	}
	c.mu.Unlock()
	if !explorationEnabled(loop) {
		return nil
	}
	dispatch, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS)
	if err != nil {
		return err
	}
	wrapped := *dispatch
	wrapped.Description = "派发范围明确、相互独立的调查子 Agent；仅调查与保存共享证据，不执行正式计划。"
	wrapped.NativeDescription = wrapped.Description
	wrapped.Options = []aitool.ToolOption{aitool.WithStructArrayParam("dispatches", []aitool.PropertyOption{aitool.WithParam_Required(), aitool.WithParam_Description("独立调查问题；必须提供输入、边界、证据来源和交付标准。")}, nil,
		requiredString("goal", "调查目标、明确边界及输入文件。"), aitool.WithStringParam("identifier", aitool.WithParam_Description("调查任务标识。")), aitool.WithStringParam("task_name", aitool.WithParam_Description("显示名称。")), aitool.WithStringParam("result_contract", aitool.WithParam_Description("需要的发现、证据引用及结论。")), aitool.WithStringParam("context_mode", aitool.WithParam_Description("fork 继承派发时的上下文；task_only 只读取显式任务书。"), aitool.WithParam_EnumString("fork", "task_only")),
	)}
	wrapped.NativeOptions = wrapped.Options
	wrapped.FunctionCallAction = nil
	wrapped.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		jobs, err := reactloops.ParseDispatchJobs(a)
		if err != nil {
			op.Feedback(err.Error())
			op.Continue()
			return
		}
		receipt, err := l.SubmitSubAgents(op.GetTask(), jobs, reactloops.SubAgentOptions{}, fmt.Sprintf("%s:%d", op.GetTask().GetUUID(), l.GetCurrentIterationIndex()))
		if err != nil {
			op.Feedback("探索派发拒绝：" + err.Error())
			op.Continue()
			return
		}
		data, _ := json.Marshal(receipt)
		l.GetInvoker().AddToTimeline("plan_exploration_dispatched", string(data))
		op.Feedback("调查已派发，结果与共享证据通过 Timeline 交付。")
		op.Continue()
	}
	reactloops.WithOverrideLoopAction(&wrapped)(loop)
	return nil
}

func configureInvestigator(child, parent *reactloops.ReActLoop) {
	if cfg, ok := child.GetConfig().(*aicommon.Config); ok {
		cfg.EnablePlanAndExec = false
		cfg.EnableSubagentsInPlan = false
		cfg.EnableDispatchSubReactAgents = false
		// Runtime policy guards execution as well as declarations, including late actions.
		_ = aicommon.WithReActActionPolicy(func(loopName, action string) bool {
			if loopName != schema.AI_REACT_LOOP_NAME_DEFAULT || !aicommon.IsReActActionAllowed(parent.GetConfig(), loopName, action) {
				return false
			}
			switch action {
			case "finish", "directly_answer", "save_evidence", "adjust_todolist", "require_tool", "directly_call_tool":
				return true
			}
			return false
		})(cfg)
	}
	reactloops.WithAllowPlanAndExec(false)(child)
	reactloops.WithAllowAIForge(false)(child)
	reactloops.WithActionFilter(func(a *reactloops.LoopAction) bool {
		switch a.ActionType {
		case "finish", "directly_answer", "save_evidence", "adjust_todolist", "require_tool", "directly_call_tool":
			return true
		}
		return false
	})(child)
	reactloops.WithToolInvokeGuard(func(name string, p aitool.InvokeParams) (bool, string) {
		if name != "write_file" && AllowedTool(name, p, "") {
			return true, ""
		}
		return false, "探索子 Agent 仅允许调查读取和搜索，不能执行命令或写入业务内容。"
	})(child)
	reactloops.WithPersistentContextProvider(func(_ *reactloops.ReActLoop, _ string) (string, error) {
		return "你是调查子 Agent。仅在指定范围读取、搜索，使用 save_evidence 保存明确共享的证据，交付来源和结论；不可修改计划、执行业务命令或派发其他 Agent。", nil
	})(child)
	action, err := child.GetActionHandler("save_evidence")
	if err != nil {
		return
	}
	wrapped := *action
	wrapped.FunctionCallAction = nil
	wrapped.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		action.ActionHandler(l, a, op)
		if done, _ := op.IsTerminated(); done {
			return
		}
		id, content := a.GetString("evidence_id"), a.GetString("evidence_content")
		if content == "" {
			content = a.GetInvokeParams("next_action").GetString("evidence_content")
		}
		if id == "" {
			id = a.GetInvokeParams("next_action").GetString("evidence_id")
		}
		before := parent.GetConfig().GetSessionEvidenceRendered()
		if _, err := reactloops.SaveSessionEvidence(parent.GetConfig(), id, content); err != nil {
			op.Fail(err)
			return
		}
		if parent.GetConfig().GetSessionEvidenceRendered() != before {
			if m := parent.GetSubAgentManager(); m != nil {
				m.NotifyDiscovery()
			}
		}
	}
	reactloops.WithOverrideLoopAction(&wrapped)(child)
}

func waitForExploration(ctx context.Context, l *reactloops.ReActLoop) error {
	m := l.GetSubAgentManager()
	if m == nil {
		return nil
	}
	jobs, err := m.Inspect(nil)
	if err != nil {
		return err
	}
	active := false
	for _, job := range jobs {
		switch job.State {
		case "completed", "failed", "cancelled", "timed_out":
			active = active || job.CleanupPending
		default:
			active = true
		}
	}
	if !active {
		return nil
	}
	after, _ := l.GetVariable("coordinator_exploration_cursor").(uint64)
	cursor, changed := m.ChangeCursor()
	if cursor != after {
		return nil
	}
	c := controller(l)
	c.mu.Lock()
	userChanged := c.eventRevision != l.GetVariable("coordinator_event_revision")
	control := c.changed
	c.mu.Unlock()
	if userChanged {
		return nil
	}
	l.UserStatus("正在等待调查子 Agent 的发现或退出通知", "Waiting for investigation discoveries or completion", aicommon.WithStatusCode("plan.waiting_for_exploration"))
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changed:
		return nil
	case <-control:
		return nil
	}
}
