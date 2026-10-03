package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"time"
)

func actionWaitMessages() coordinatorAction {
	return coordinatorAction{name: "wait_messages", description: "请求在本轮全部动作完成后等待新消息；已有未审核结果或其他可执行工作时先处理它们。空超时不轮询模型、不取消任务、不结束运行。", options: []aitool.ToolOption{aitool.WithIntegerParam("timeout_seconds", aitool.WithParam_Description("运行时检查间隔，默认30秒，最大60秒；普通发现最多合并5秒。"))}, execute: func(c *Controller, l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
		d := currentDecision(l)
		d.yield = true
		d.timeout = time.Duration(a.GetInt("timeout_seconds")) * time.Second
		return map[string]string{"status": "wait_requested", "detail": "本轮动作结束后由运行时检查可执行工作并等待新消息。"}, nil
	}}
}
