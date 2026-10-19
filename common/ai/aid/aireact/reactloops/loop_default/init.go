package loop_default

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		config := r.GetConfig()

		attachedDatas := aicommon.NonEmptyAttachedResources(task.GetAttachedDatas())
		reactloops.RunAttachedExtraResourcesInit(r, loop, attachedDatas)
		// Local greeting classification must remain available when expensive
		// synchronous enrichment is disabled. It enables one-answer completion
		// without restoring model-backed intent or knowledge initialization.
		if !config.GetConfigBool("DisableIntentRecognition") && len(attachedDatas) == 0 &&
			greetingPatterns.MatchString(strings.TrimSpace(task.GetUserInput())) {
			applyFastMatchResult(r, loop, &FastMatchResult{IsSimpleQuery: true})
			return
		}

		// Attachments remain available to the main loop. Expensive enrichment is
		// opt-in so the first response can start before intent/knowledge calls.
		if !config.GetConfigBool("AllowSyncInitContext") {
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
			loop.UserStatus("开始意图识别", "Start intent recognition")
			userInput := task.GetUserInput()
			capabilityNameMatches := reactloops.MatchCapabilitiesByTextWithConfig(r.GetConfig(), userInput)

			scale := ClassifyInputScale(userInput)
			log.Infof("input scale classified as %s for input length %d runes", scale.String(), len([]rune(userInput)))

			needsDeepIntent := false

			if scale.IsMicroOrSmall() {
				loop.UserStatus("快速意图识别", "Fast intent recognition")
				result := FastIntentMatch(r, userInput)
				if result != nil {
					applyCapabilityMatchesToFastMatchResult(result, capabilityNameMatches)
					applyFastMatchResult(r, loop, result)
					if result.IsSimpleQuery {
						log.Infof("simple query detected, skipping deep intent recognition")
					} else if result.NeedsDeepAnalysis() {
						log.Infof("short input with no fast matches detected, escalating to deep intent recognition")
						needsDeepIntent = true
					} else {
						loop.RecallMemoryForIntent(task, "fast_match:"+result.ContextSummary)
					}
				}
			} else {
				needsDeepIntent = true
			}

			if needsDeepIntent {
				loop.UserStatus("深度意图识别", "Deep intent recognition")
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
