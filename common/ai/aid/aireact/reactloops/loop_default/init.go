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

		attachedDatas := task.GetAttachedDatas()
		attachedResources := reactloops.RunAttachedExtraResourcesInit(r, loop, attachedDatas)

		// Original logic: process attached data (knowledge bases, files, etc.)
		mustProcessMentionedInfo := config.GetConfigBool("MustProcessAttachedData")
		if mustProcessMentionedInfo && hasAttachedKnowledgeBaseResource(attachedResources) {
			loop.LoadingStatus("开始处理用户提及的数据（@ Mentionup） / Start to process user-mentioned data (@ Mentionup)")
			err := ProcessAttachedData(r, loop, task, operator, attachedResources)
			if err != nil {
				log.Errorf("failed to process attached data: %v", err)
				loop.GetInvoker().AddToTimeline("error", fmt.Sprintf("failed to process attached data: %v", err))
			}
		}

		// exit early
		if failed, reason := operator.IsFailed(); operator.IsDone() || failed {
			if reason != nil && reason.Error() != "" {
				r.AddToTimeline("attached_materials_handler", reason.Error())
			}
			r.AddToTimeline("end", "Attached Materials Handler Decide to exit.")
			return
		}

		// === Intent recognition phase ===
		// Skip intent recognition when disabled (e.g. test environments).
		// The intent recognition sub-loop (loop_intent) shares the same AI callback,
		// which in test environments with mocked AI would consume mock responses
		// intended for the main loop, causing test failures.
		if config.GetConfigBool("DisableIntentRecognition") {
			log.Infof("intent recognition disabled via config, skipping")
		} else {
			loop.LoadingStatus("开始意图识别 / Start intent recognition")
			userInput := task.GetUserInput()
			capabilityNameMatches := reactloops.MatchCapabilitiesByTextWithConfig(r.GetConfig(), userInput)

			scale := ClassifyInputScale(userInput)
			log.Infof("input scale classified as %s for input length %d runes", scale.String(), len([]rune(userInput)))

			needsDeepIntent := false

			if scale.IsMicroOrSmall() {
				loop.LoadingStatus("快速意图识别 / Fast intent recognition")
				result := FastIntentMatch(r, userInput)
				if result != nil {
					applyCapabilityMatchesToFastMatchResult(result, capabilityNameMatches)
					applyFastMatchResult(r, loop, result)
					if result.IsSimpleQuery {
						log.Infof("simple query detected, skipping deep intent recognition")
					} else if result.NeedsDeepAnalysis() {
						log.Infof("short input with no fast matches detected, escalating to deep intent recognition")
						needsDeepIntent = true
					}
				}
			} else {
				needsDeepIntent = true
			}

			if needsDeepIntent {
				loop.LoadingStatus("深度意图识别 / Deep intent recognition")
				log.Infof("invoking deep intent recognition (scale=%s)", scale.String())
				deepResult := executeDeepIntentRecognition(r, loop, task)
				if deepResult != nil {
					reactloops.ApplyCapabilityMatchesToDeepIntentResult(deepResult, capabilityNameMatches)
					applyDeepIntentResult(r, loop, deepResult)
				} else {
					log.Infof("deep intent recognition returned no result, proceeding with default loop")
				}
			}
		}
	}
}
