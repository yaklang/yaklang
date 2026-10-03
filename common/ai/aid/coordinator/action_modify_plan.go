package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func actionModifyPlan() coordinatorAction {
	return coordinatorAction{
		name: "modify_plan", description: "按精确版本完整替换草案；新版本获批前，原已批准计划继续有效。", options: []aitool.ToolOption{planVersionParameter(), planParameter(), requiredString("plan_document", "完整替换后的 Markdown 计划文档。")},
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			return c.ModifyPlan(op.GetContext(), uint64(a.GetInt("plan_version")), planData(a), a.GetString("plan_document"))
		},
	}
}
