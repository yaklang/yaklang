package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func actionInspectTasks() coordinatorAction {
	return coordinatorAction{
		name: "inspect_tasks", description: "读取任务状态、执行尝试、结果及 artifacts/Evidence 引用，并标记结果已被观察。", options: []aitool.ToolOption{taskIDsParameter()},
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			return c.InspectTasks(a.GetStringSlice("task_ids"))
		},
	}
}
