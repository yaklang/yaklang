package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func actionSubmitPlan() coordinatorAction {
	return coordinatorAction{
		name: "submit_plan", description: "提交当前计划供用户审核；等待时锁定内容，批准最终内容后移交 EXEC，不提前执行业务任务。", options: nil,
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			err := c.SubmitPlan(op.GetContext())
			return map[string]any{"phase": c.Snapshot().Phase, "review_pending": c.Snapshot().ReviewPending}, err
		},
	}
}
