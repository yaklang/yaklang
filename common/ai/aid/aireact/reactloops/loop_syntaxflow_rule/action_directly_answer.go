package loop_syntaxflow_rule

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/syntaxflowtools"
	"github.com/yaklang/yaklang/common/utils"
)

// loopAction_DirectlyAnswerSyntaxFlow is a custom directly_answer for write_syntaxflow_rule loop.
// It enforces: verify must be called when sf_has_code_sample; 规则内容由 replace_payload 从文件读取展示。
var loopAction_DirectlyAnswerSyntaxFlow = &reactloops.LoopAction{
	ActionType: "directly_answer",
	Description: "Directly answer with the 'answer_payload' field. " +
		"For ordinary tasks, continue or finish according to Current/TODO state after delivery. " +
		"Do not invoke directly_answer twice without an effective todo_delta in the same CURRENT-TASK. " +
		"Carry a non-empty todo_delta whenever the answer changes or schedules follow-up TODO state.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"answer_payload",
			aitool.WithParam_Description(`USE THIS FIELD ONLY IF @action is 'directly_answer' AND answer is short (≤200 chars). For long answers, leave this empty and use '<|FINAL_ANSWER_...|>' tags after JSON. ⚠️ CRITICAL: answer_payload and <|FINAL_ANSWER_...|> are STRICTLY MUTUALLY EXCLUSIVE - never use both simultaneously.`),
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
	ActionVerifier: directlyAnswerSyntaxFlowVerifier,
	ActionHandler:  directlyAnswerSyntaxFlowHandler,
}

func directlyAnswerSyntaxFlowVerifier(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	payload := action.GetString("answer_payload")
	if payload == "" {
		payload = action.GetInvokeParams("next_action").GetString("answer_payload")
	}
	if payload == "" {
		tagPayload := loop.Get("tag_final_answer")
		if tagPayload != "" {
			payload = tagPayload
		}
	}
	if payload == "" {
		// 用 WrapDirectlyAnswerError 升级为带 nonce 的 AITAG 提示, 让 AI 重试时能
		// 用 FINAL_ANSWER tag 自纠正, 避免 5 次重试黑洞 + fatal abort.
		// 关键词: directly_answer ActionVerifier AITAG hint, 5 次重试黑洞修复
		return reactloops.WrapDirectlyAnswerError(loop, utils.Error("answer_payload is required for ActionDirectlyAnswer but empty"))
	}

	// 1. When user provided code sample, verify must have been called and matched=true
	sfHasCodeSample := utils.InterfaceToBoolean(loop.Get("sf_has_code_sample"))
	if sfHasCodeSample {
		sfVerifyMatched := utils.InterfaceToBoolean(loop.Get("sf_verify_matched"))
		if !sfVerifyMatched {
			return utils.Error("Cannot directly_answer: 有正例（用户提供的漏洞样例=file://、UNSAFE）时必须先调用 check-syntaxflow-syntax 并传入 path、sample_code、filename、language 进行正例自检，得到 matched=true。请 require_tool check-syntaxflow-syntax，并在参数中提供 path、sample_code、filename、language。")
		}
	}

	// 2. Pass payload to the shared duplicate guard and handler store.
	return reactloops.FinishDirectlyAnswerVerification(loop, action, payload)
}

func directlyAnswerSyntaxFlowHandler(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
	invoker := loop.GetInvoker()
	payload := loop.Get("directly_answer_payload")
	if payload == "" {
		payload = loop.Get("tag_final_answer")
	}
	if payload == "" {
		operator.Fail("directly_answer action must have 'answer_payload' field")
		return
	}
	// 展示最终结果时必须从实际文件读取规则内容，禁止使用 AI 自行生成的内容
	sfFilename := loop.Get("sf_filename")
	if sfFilename != "" {
		payload = syntaxflowtools.ReplacePayloadRuleWithFileContent(payload, sfFilename)
	}
	answerPath := invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
	invoker.EmitResultAfterStream(payload)
	userInputPreview := utils.ShrinkTextBlock(loop.GetCurrentTask().GetUserInput(), 200)
	answerPreview := utils.ShrinkTextBlock(payload, 300)
	invoker.AddToTimeline(reactloops.TimelineEntryAssistantOutput, fmt.Sprintf(
		"user input preview:\n%s\n\nanswer preview:\n%s\n\nanswer file: %s",
		userInputPreview,
		answerPreview,
		answerPath,
	))

	// 共享收口负责续跑、simple_query 自动收口和重复答复防抖。
	reactloops.DirectlyAnswerContinue(loop, action, operator)
}
