package coordinator

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func actionStartTasks() coordinatorAction {
	return coordinatorAction{
		name: "start_tasks", description: "立即派发选定的可执行任务，不等待完成；仅执行已批准且前置任务均已验收的任务。", options: []aitool.ToolOption{taskIDsParameter()},
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			if planningOnly(loop) {
				return nil, fmt.Errorf("this plan-only run must finish after approval; execution starts through the existing approved-plan entrypoint")
			}
			return c.StartTasks(a.GetStringSlice("task_ids"))
		},
	}
}
