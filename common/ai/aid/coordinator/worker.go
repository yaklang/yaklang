package coordinator

import (
	"bytes"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

var workerInstruction = promptloader.MustLoad("ai/aid/coordinator/worker_instruction.txt")

// NewWorkerLoop is the only execution loop in the new PLAN architecture.
// It shares session evidence and the ordinary tool policy, but cannot spawn
// planners, specialized loops, blueprints or generic subagent runtimes.
func NewWorkerLoop(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	if cfg, ok := r.GetConfig().(*aicommon.Config); ok {
		_ = aicommon.WithLiteForgeExecutor(executeNativeHelper)(cfg)
		_ = aicommon.WithAiAgreeRiskControl(NativeRiskReview)(cfg)
	}
	preset := append([]reactloops.ReActLoopOption{}, opts...)
	preset = append(preset,
		reactloops.WithFunctionCallActionVariants(),
		reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowAIForge(false), reactloops.WithAllowToolCall(true),
		reactloops.WithPersistentInstruction(workerInstruction),
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
		actionSubmitTaskResult(),
	)
	loop, err := reactloops.NewReActLoop("pe_task", r, preset...)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"directly_answer", "request_plan", "request_plan_and_execution", "require_ai_blueprint", "dispatch_sub_react_agents", "inspect_sub_react_agents", "wait_sub_react_agents", "cancel_sub_react_agents", "report_generating", "tool_compose", "load_capability"} {
		loop.RemoveAction(name)
	}
	if err := configureWorkerFinish(loop); err != nil {
		return nil, err
	}
	return loop, nil
}
