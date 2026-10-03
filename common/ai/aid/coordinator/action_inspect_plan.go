package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func actionInspectPlan() coordinatorAction {
	return coordinatorAction{
		name: "inspect_plan", description: "核对草案、批准和提交版本；完整文档、任务书及 DAG 已在 SemiDynamic1 的 PLAN DEFINITION / PLAN DOCUMENT 中，本动作只返回版本和上下文位置。不批准、不修改计划。", options: nil,
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			return c.planReceipt(), nil
		},
	}
}
