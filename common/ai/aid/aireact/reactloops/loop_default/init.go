package loop_default

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		config := r.GetConfig()
		mustProcessMentionedInfo := config.GetConfigBool("MustProcessAttachedData")

		attachedDatas := task.GetAttachedDatas()

		if mustProcessMentionedInfo && len(attachedDatas) > 0 {
			err := ProcessAttachedData(r, loop, task, operator)
			if err != nil {
				log.Errorf("failed to process attached data: %v", err)
				loop.GetInvoker().AddToTimeline("error", fmt.Sprintf("failed to process attached data: %v", err))
			}
		}
	}
}
