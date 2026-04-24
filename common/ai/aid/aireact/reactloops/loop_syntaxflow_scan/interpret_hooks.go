package loop_syntaxflow_scan

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const loopVarOrchestratorParentTaskID = "sf_orchestrator_parent_task_id"

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
