package coordinator

import (
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func actionWaitTasks() coordinatorAction {
	return coordinatorAction{
		name: "wait_tasks", description: "按需等待指定任务的结果或控制变化；空闲时系统已自动等待，无需反复调用本动作。超时不取消任务。", options: []aitool.ToolOption{taskIDsParameter(), aitool.WithStringParam("mode", aitool.WithParam_Description("any（默认）：等待首次更新；all：等待选中的已派发尝试全部结算。新发现、用户补充或计划变更会唤醒两种等待。")), aitool.WithIntegerParam("timeout_seconds", aitool.WithParam_Description("默认 30 秒，最多 60 秒。"))},
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			observed, _ := loop.GetVariable("coordinator_observed_user_revision").(uint64)
			if observed != c.Snapshot().UserRevision {
				tasks, err := c.InspectTasks(a.GetStringSlice("task_ids"))
				return WaitResult{Reason: "changed", Tasks: tasks}, err
			}
			return c.WaitTasksMode(op.GetContext(), a.GetStringSlice("task_ids"), time.Duration(a.GetInt("timeout_seconds"))*time.Second, a.GetString("mode"))
		},
	}
}
