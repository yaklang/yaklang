package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"time"
)

func actionWaitMessages() coordinatorAction {
	return coordinatorAction{name: "wait_messages", description: "等待 inbox 消息；默认30秒检查一次运行时，空超时不轮询模型、不取消任务、不结束运行。", options: []aitool.ToolOption{aitool.WithIntegerParam("timeout_seconds", aitool.WithParam_Description("等待检查间隔，默认30秒，最大60秒；与任务调度无关。"))}, execute: func(c *Controller, l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
		return nil, c.WaitMessages(op.GetContext(), time.Duration(a.GetInt("timeout_seconds"))*time.Second, func() {
			l.UserStatus("正在等待 inbox 消息", "Waiting for inbox", aicommon.WithStatusCode("plan.waiting_for_messages"))
		})
	}}
}
