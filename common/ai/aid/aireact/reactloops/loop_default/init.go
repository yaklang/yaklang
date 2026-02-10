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

		// Original logic: process attached data (knowledge bases, files, etc.)
		mustProcessMentionedInfo := config.GetConfigBool("MustProcessAttachedData")
		attachedDatas := task.GetAttachedDatas()
		if mustProcessMentionedInfo && len(attachedDatas) > 0 {
			loop.LoadingStatus("开始处理用户提及的数据（@ Mentionup） / Start to process user-mentioned data (@ Mentionup)")
			err := ProcessAttachedData(r, loop, task, operator)
			if err != nil {
				log.Errorf("failed to process attached data: %v", err)
				loop.GetInvoker().AddToTimeline("error", fmt.Sprintf("failed to process attached data: %v", err))
			}
		}

		// exit early
		if failed, reason := operator.IsFailed(); operator.IsDone() || failed {
			if reason.Error() != "" {
				r.AddToTimeline("attached_materials_handler", reason.Error())
			}
			r.AddToTimeline("end", "Attached Materials Handler Decide to exit.")
			return
		}

		loop.LoadingStatus("开始意图识别 / Start intent recognition")
		userInput := task.GetUserInput()

		// === Intent recognition phase ===
		scale := ClassifyInputScale(userInput)
		log.Infof("input scale classified as %s for input length %d runes", scale.String(), len([]rune(userInput)))

		// Decide whether to use fast matching or deep intent recognition.
		//
		// Flow:
		//   Micro/Small input → FastIntentMatch (rules + BM25)
		//     ├── greeting/simple query → done, fast path
		//     ├── has matches (tools/forges/loops) → done, apply context
		//     └── NO matches, NOT simple query → escalate to deep intent
		//         (e.g. "我想做渗透测试" — short but composite task)
		//
		//   Medium/Large/XLarge input → deep intent recognition directly
		needsDeepIntent := false

		if scale.IsMicroOrSmall() {
			loop.LoadingStatus("快速意图识别 / Fast intent recognition")
			// Fast mode: rules + BM25 matching
			result := FastIntentMatch(r, userInput)
			if result != nil {
				applyFastMatchResult(r, loop, result)
				if result.IsSimpleQuery {
					log.Infof("simple query detected, skipping deep intent recognition")
				} else if result.NeedsDeepAnalysis() {
					// Short input but nothing matched — this likely represents
					// a composite task that BM25 keyword search can't resolve.
					// Examples: "我想做渗透测试", "帮我做安全评估", "security audit"
					// Escalate to deep intent recognition for task decomposition.
					log.Infof("short input with no fast matches detected, escalating to deep intent recognition")
					needsDeepIntent = true
				}
				// else: fast matches found, context applied, proceed with default loop
			}
		} else {
			needsDeepIntent = true
		}

		if needsDeepIntent {
			loop.LoadingStatus("深度意图识别 / Deep intent recognition")
			log.Infof("invoking deep intent recognition (scale=%s)", scale.String())
			deepResult := executeDeepIntentRecognition(r, loop, task)
			if deepResult != nil {
				applyDeepIntentResult(r, loop, deepResult)
			} else {
				log.Infof("deep intent recognition returned no result, proceeding with default loop")
			}
		}
		// === End intent recognition phase ===
	}
}
