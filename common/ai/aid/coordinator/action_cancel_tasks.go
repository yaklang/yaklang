package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func actionCancelTasks() coordinatorAction {
	return coordinatorAction{
		name: "cancel_tasks", description: "请求取消任务；worker 实际退出前 cancelling 不代表任务已经停止。", options: []aitool.ToolOption{taskIDsParameter(), requiredString("reason", "停止选定任务的原因。")},
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			return nil, c.CancelTasks(a.GetStringSlice("task_ids"), a.GetString("reason"))
		},
	}
}
