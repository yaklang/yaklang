package coordinator

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
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
	resultOptions := []aitool.ToolOption{
		aitool.WithStringParam("summary", aitool.WithParam_Description("本次执行尝试的实际结果摘要。"), aitool.WithParam_Required()),
		aitool.WithStringArrayParam("artifacts", aitool.WithParam_Description("实际产出的文件或其他 artifacts 引用。")),
		aitool.WithStringArrayParam("evidence_ids", aitool.WithParam_Description("已经保存到 session 的真实 Evidence ID 列表。")),
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
		registerAction("submit_task_result", "提交本次执行尝试的结果；不代表任务验收通过，也不结束循环。", resultOptions, func(loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
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
