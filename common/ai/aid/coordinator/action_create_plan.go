package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func actionCreatePlan() coordinatorAction {
	return coordinatorAction{
		name: "create_plan", description: "创建并校验计划草案；不批准、不派发任务。", options: []aitool.ToolOption{planParameter(), requiredString("plan_document", "稳定的 Markdown 计划文档。")},
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			return c.CreatePlan(op.GetContext(), planData(a), a.GetString("plan_document"))
		},
	}
}
