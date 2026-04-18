package loop_http_flow_analyze

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

var loopActionDirectlyAnswerHTTPFlowAnalyze = &reactloops.LoopAction{
	ActionType:  "directly_answer",
	Description: "Answer the user's HTTP traffic analysis question or summarize analysis findings. Use answer_payload for short answers; use FINAL_ANSWER AITAG for longer Markdown reports with tables, lists, or structured content.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"answer_payload",
			aitool.WithParam_Description(`Short answer text. For longer Markdown content with multiple sections, lists or tables, leave this empty and use the <|FINAL_ANSWER_...|> tag instead. answer_payload and <|FINAL_ANSWER_...|> are mutually exclusive.`),
		),
	},
	AITagStreamFields: []*reactloops.LoopAITagField{
		{
			TagName:      "FINAL_ANSWER",
			VariableName: "tag_final_answer",
			AINodeId:     "re-act-loop-answer-payload",
			ContentType:  aicommon.TypeTextMarkdown,
		},
	},
	StreamFields: []*reactloops.LoopStreamField{
		{
			FieldName:   "answer_payload",
			AINodeId:    "re-act-loop-answer-payload",
			ContentType: aicommon.TypeTextMarkdown,
		},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		payload := strings.TrimSpace(action.GetString("answer_payload"))
		if payload == "" {
			payload = strings.TrimSpace(action.GetInvokeParams("next_action").GetString("answer_payload"))
		}
		tagPayload := strings.TrimSpace(loop.Get("tag_final_answer"))
		if payload != "" && tagPayload != "" {
			return utils.Error("directly_answer requires exactly one of answer_payload or FINAL_ANSWER tag, but both were provided")
		}
		if payload == "" {
			payload = tagPayload
		}
		if payload == "" {
			return utils.Error("directly_answer requires answer_payload or FINAL_ANSWER tag, but both are empty")
		}
		loop.Set("directly_answer_payload", payload)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		invoker := loop.GetInvoker()
		payload := strings.TrimSpace(loop.Get("directly_answer_payload"))
		if payload == "" {
			payload = strings.TrimSpace(loop.Get("tag_final_answer"))
		}
		if payload == "" {
			operator.Fail("directly_answer action must have 'answer_payload' field")
			return
		}

		recordMetaAction(loop, "directly_answer",
			"answer user traffic analysis question",
			utils.ShrinkTextBlock(payload, 240))
		markDirectlyAnswered(loop)
		invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
		invoker.EmitResultAfterStream(payload)
		invoker.AddToTimeline("directly_answer", "HTTP flow analysis directly answered: "+utils.ShrinkTextBlock(payload, 400))
		operator.Exit()
	},
}
