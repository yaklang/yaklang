package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func actionModifyPlan() coordinatorAction {
	options := modifyPlanParameters()
	return coordinatorAction{
		name: "modify_plan", description: "原子修改当前文档及任务定义；同组件覆盖与 patch 互斥，失败保留原计划。", options: options,
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			return c.ModifyPlan(op.GetContext(), modifyArguments(a, options))
		},
	}
}
