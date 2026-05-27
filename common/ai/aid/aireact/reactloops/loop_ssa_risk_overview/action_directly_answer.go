package loop_ssa_risk_overview

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

var loopActionDirectlyAnswerSSARiskOverview = &reactloops.LoopAction{
	ActionType:  "directly_answer",
	Description: "Deliver the SSA risk overview conclusion to the user once. Base it on FINDINGS + preface — synthesize patterns, severity, and next steps; do not repeat the full risk id list. Put the full answer in answer_payload only.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"answer_payload",
			aitool.WithParam_Description("Complete user-facing answer in Markdown. Required. Do not use FINAL_ANSWER AITag."),
			aitool.WithParam_Required(true),
		),
	},
	// No StreamFields / AITagStreamFields: live streaming to re-act-loop-answer-payload during JSON
	// parse caused duplicate UI bubbles when combined with EmitResultAfterStream, and on AI retries
	// (grpc.log iteration 2: empty response retry + dual answer_payload stream ends).
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		payload := strings.TrimSpace(action.GetString("answer_payload"))
		if payload == "" {
			payload = strings.TrimSpace(action.GetInvokeParams("next_action").GetString("answer_payload"))
		}
		if payload == "" {
			return reactloops.WrapDirectlyAnswerError(loop, utils.Error("directly_answer requires non-empty answer_payload"))
		}
		loop.Set("directly_answer_payload", payload)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		if hasFinalAnswerDelivered(loop) {
			operator.Exit()
			return
		}
		invoker := loop.GetInvoker()
		payload := strings.TrimSpace(loop.Get("directly_answer_payload"))
		if payload == "" {
			operator.Fail("directly_answer action must have 'answer_payload' field")
			return
		}

		recordMetaAction(loop, "directly_answer", "synthesize overview for user", utils.ShrinkTextBlock(payload, 240))
		markDirectlyAnswered(loop)
		markFinalAnswerDelivered(loop)

		invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
		// Yakit/IRify 对话气泡走 re-act-loop-answer-payload；EmitResultAfterStream("result") 仅 timeline/CLI。
		taskID := ""
		if task := loop.GetCurrentTask(); task != nil {
			taskID = task.GetId()
		}
		if emitter := loop.GetEmitter(); emitter != nil {
			_, _ = emitter.EmitTextMarkdownStreamEvent(
				"re-act-loop-answer-payload",
				strings.NewReader(payload),
				taskID,
				func() {},
			)
		} else {
			invoker.EmitResultAfterStream(payload)
		}
		invoker.AddToTimeline("ssa_risk_overview_answer",
			fmt.Sprintf("SSA risk overview answered (approx_total=%s):\n%s",
				strings.TrimSpace(loop.Get("ssa_risk_total_hint")),
				utils.ShrinkTextBlock(payload, 2000)))
		operator.Exit()
	},
}
