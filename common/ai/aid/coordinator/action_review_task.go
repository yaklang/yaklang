package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func actionReviewTask() coordinatorAction {
	return coordinatorAction{
		name: "review_task", description: "依据 Timeline 中的真实 Evidence 和 artifacts，接受或拒绝当前已结算的执行尝试。", options: []aitool.ToolOption{requiredString("task_id", "逻辑任务 ID。"), attemptParameter(), requiredString("decision", "accept 表示接受；reject 表示拒绝。"), requiredString("reason", "有证据支持的验收结论，包含 artifacts/Evidence 引用。")},
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			return nil, c.ReviewTask(a.GetString("task_id"), uint64(a.GetInt("attempt_id")), a.GetString("decision"), a.GetString("reason"))
		},
	}
}
