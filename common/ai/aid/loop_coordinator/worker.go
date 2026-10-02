package loop_coordinator

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// NewWorkerLoop is the only execution loop in the new PLAN architecture.
// It shares session evidence and the ordinary tool policy, but cannot spawn
// planners, specialized loops, blueprints or generic subagent runtimes.
func NewWorkerLoop(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	if cfg, ok := r.GetConfig().(*aicommon.Config); ok {
		_ = aicommon.WithLiteForgeExecutor(executeNativeHelper)(cfg)
		_ = aicommon.WithEnableFunctionCallMode(true)(cfg)
		_ = aicommon.WithAiAgreeRiskControl(NativeRiskReview)(cfg)
	}
	resultOptions := []aitool.ToolOption{
		aitool.WithStringParam("summary", aitool.WithParam_Required()), aitool.WithStringArrayParam("artifacts"), aitool.WithStringArrayParam("evidence_ids"),
	}
	preset := append([]reactloops.ReActLoopOption{}, opts...)
	preset = append(preset,
		reactloops.WithFunctionCallMode(true), reactloops.WithFunctionCallActionVariants(),
		reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowAIForge(false), reactloops.WithAllowToolCall(true),
		reactloops.WithPersistentContextProvider(func(*reactloops.ReActLoop, string) (string, error) {
			return `Execute the assigned frozen plan task. Read user information and prior results in Timeline. Use native function calls only; JSON actions cannot invoke anything. Use tools for business execution, save_evidence for durable session facts, and adjust_todolist for microscopic work. Produce concrete artifacts/evidence, call submit_task_result with the summary and actual references, then finish. Acceptance and retries belong to the coordinator.`, nil
		}),
		reactloops.WithReactiveDataBuilder(func(_ *reactloops.ReActLoop, b *bytes.Buffer, _ string) (string, error) { return b.String(), nil }),
		reactloops.WithDisablePeriodicVerification(true),
		reactloops.WithDisableLoopPerception(true),
		reactloops.WithActionFilter(func(a *reactloops.LoopAction) bool {
			switch a.ActionType {
			case "submit_task_result", "finish", "save_evidence", "adjust_todolist", "require_tool", "directly_call_tool", "ask_for_clarification", "knowledge_enhance_answer", "load_skills", "change_skill_view_offset", "load_skill_resources", "search_capabilities":
				return true
			}
			return false
		}),
		reactloops.WithRegisterLoopAction("submit_task_result", "Submit this execution attempt's results; does not accept the task or finish the loop.", resultOptions, validateActionParameters("submit_task_result", resultOptions), func(loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			if strings.TrimSpace(a.GetString("summary")) == "" {
				op.Feedback("result summary is required")
				op.Continue()
				return
			}
			loop.Set("coordinator_task_result", Result{Summary: a.GetString("summary"), Artifacts: a.GetStringSlice("artifacts"), EvidenceIDs: a.GetStringSlice("evidence_ids")})
			op.Continue()
		}),
	)
	loop, err := reactloops.NewReActLoop("pe_task", r, preset...)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"directly_answer", "request_plan", "request_plan_and_execution", "require_ai_blueprint", "dispatch_sub_react_agents", "inspect_sub_react_agents", "wait_sub_react_agents", "cancel_sub_react_agents", "report_generating", "tool_compose", "load_capability"} {
		loop.RemoveAction(name)
	}
	finish, err := loop.GetActionHandler("finish")
	if err != nil {
		return nil, err
	}
	wrapped := *finish
	original := finish.ActionHandler
	wrapped.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		if _, ok := l.GetVariable("coordinator_task_result").(Result); !ok {
			op.Feedback(fmt.Sprintf("submit_task_result is required before finishing task %s", op.GetTask().GetId()))
			op.Continue()
			return
		}
		original(l, a, op)
	}
	reactloops.WithOverrideLoopAction(&wrapped)(loop)
	return loop, nil
}
