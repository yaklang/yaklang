package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func actionSubmitPlan() coordinatorAction {
	return coordinatorAction{
		name: "submit_plan", description: "提交草案供用户审核并采用批准的任务树；重复提交同一已批准版本不再次审核。批准后调用 start_tasks 自动推进执行。", options: []aitool.ToolOption{planVersionParameter()},
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			err := c.SubmitPlan(op.GetContext(), uint64(a.GetInt("plan_version")))
			return c.planReceipt(), err
		},
	}
}
