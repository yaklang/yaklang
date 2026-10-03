package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func actionRetryTask() coordinatorAction {
	return coordinatorAction{
		name: "retry_task", description: "重试已结算的尝试并使下游结果失效；仍在运行的受影响任务必须先停止。", options: []aitool.ToolOption{requiredString("task_id", "逻辑任务 ID。"), attemptParameter(), requiredString("reason", "当前尝试需要重新执行的原因。")},
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			return c.RetryTask(a.GetString("task_id"), uint64(a.GetInt("attempt_id")), a.GetString("reason"))
		},
	}
}
