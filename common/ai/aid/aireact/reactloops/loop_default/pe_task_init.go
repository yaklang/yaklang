package loop_default

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

// buildPETaskInitTask creates the init handler for plan-execution tasks (pe_task).
// Unlike buildInitTask for the default loop, this handler unconditionally
// runs deep intent recognition without scale classification or fast matching,
// because each PE sub-task benefits from full capability discovery regardless
// of input length.
func buildPETaskInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		config := r.GetConfig()

		if config.GetConfigBool("DisableIntentRecognition") {
			log.Infof("pe_task: intent recognition disabled via config, skipping")
			return
		}

		loop.LoadingStatus("深度意图识别 / Deep intent recognition")
		log.Infof("pe_task: invoking deep intent recognition directly")

		deepResult := executeDeepIntentRecognition(r, loop, task)
		if deepResult != nil {
			applyDeepIntentResult(r, loop, deepResult)
		} else {
			log.Infof("pe_task: deep intent recognition returned no result, proceeding normally")
		}
	}
}
