package loop_http_flow_analyze

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

var loopActionDirectlyAnswerHTTPFlowAnalyze = &reactloops.LoopAction{
	ActionType: "directly_answer",
	Description: `Answer the user's HTTP traffic analysis question or summarize collected HTTP flow evidence.
[OUTPUT FORMAT OPTIONS]
* For short plain text answers (< 200 chars):
Use the answer_payload field inside the Action JSON.
* For longer reports with Markdown formatting:
Use the <|FINAL_ANSWER_...|> tag OUTSIDE the Action JSON.
NOTE: Even when using the external tag, a foundational Action JSON structure is still REQUIRED, but the answer_payload field must be left empty.
[CRITICAL RULES]
* You MUST use EXACTLY ONE of either answer_payload or the external FINAL_ANSWER tag.
* They are strictly mutually exclusive, and at least one is required.
* NEVER nest the <|FINAL_ANSWER_...|> tag inside the answer_payload field.
[PROCESS CONTROL]
* The directly_answer action ONLY delivers the answer; the execution loop CONTINUES afterward.
* You MUST explicitly invoke the finish action to terminate the process.`,
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"answer_payload",
			aitool.WithParam_Description(`Short plain text answer (< 200 chars). For longer Markdown reports, leave this field EMPTY and use <|FINAL_ANSWER_...|> tag OUTSIDE the action JSON instead. NEVER include <|FINAL_ANSWER_...|> markers or tag content inside this field.`),
			aitool.WithParam_Required(true),
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
		if payload != "" && tagPayload != "" && payload != tagPayload {
			return utils.Error("directly_answer requires exactly one of answer_payload or FINAL_ANSWER tag, but both were provided")
		}
		if payload == "" {
			payload = tagPayload
		}
		if payload == "" {
			// 用 WrapDirectlyAnswerError 升级为带 nonce 的 AITAG 提示, 让 AI 重试时能
			// 用 FINAL_ANSWER tag 自纠正, 避免 5 次重试黑洞 + fatal abort.
			// 关键词: directly_answer ActionVerifier AITAG hint, 5 次重试黑洞修复
			return reactloops.WrapDirectlyAnswerError(loop, utils.Error("directly_answer requires answer_payload or FINAL_ANSWER tag, but both are empty"))
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
		invoker.AddToTimeline("directly_answer", "HTTP flow analysis directly answered: "+payload)

		// directly_answer 绝不 Exit: emit 完答复后统一交给 DirectlyAnswerContinue
		// 追加 timeline + 续跑, 终结只能由显式 finish action 完成. 与 buildin 对齐.
		// 关键词: directly_answer 永不 Exit, http_flow_analyze 复用单源, finish 唯一终结器
		reactloops.DirectlyAnswerContinue(loop, action, operator)
	},
}
