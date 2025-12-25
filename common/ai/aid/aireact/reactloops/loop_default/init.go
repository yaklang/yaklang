package loop_default

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
		config := r.GetConfig()
		mustProcessMentionedInfo := config.GetConfigBool("MustProcessAttachedData")

		ragAttachedData, _ := config.GetConfig("attached_rags")
		aiTools, _ := config.GetConfig("attached_ai_tools")

		hasAttachedData := ragAttachedData != nil && aiTools != nil

		if mustProcessMentionedInfo && hasAttachedData {
			ProcessAttachedData(r)
		}
		return nil
	}
}
