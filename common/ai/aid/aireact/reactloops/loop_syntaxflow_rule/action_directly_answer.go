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
	ActionType:  "directly_answer",
	Description: "Directly answer with the 'answer_payload' field",
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
		return utils.Error("answer_payload is required for ActionDirectlyAnswer but empty")
	}

	// 1. When user provided code sample, verify must have been called and matched=true
	sfHasCodeSample := utils.InterfaceToBoolean(loop.Get("sf_has_code_sample"))
	if sfHasCodeSample {
		sfVerifyMatched := utils.InterfaceToBoolean(loop.Get("sf_verify_matched"))
		if !sfVerifyMatched {
			return utils.Error("Cannot directly_answer: 有正例（用户提供的漏洞样例=file://、UNSAFE）时必须先调用 check-syntaxflow-syntax 并传入 path、sample_code、filename、language 进行正例自检，得到 matched=true。请 require_tool check-syntaxflow-syntax，并在参数中提供 path、sample_code、filename、language。")
		}
	}

	// 2. Pass payload to handler（规则代码由 replace_payload 检测并丢弃，以文件内容为准）
	loop.Set("directly_answer_payload", payload)
	return nil
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
	invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
	invoker.EmitResultAfterStream(payload)
	invoker.AddToTimeline("directly_answer", fmt.Sprintf("user input: \n"+
		"%s\n"+
		"ai directly answer:\n"+
		"%v",
		utils.PrefixLines(loop.GetCurrentTask().GetUserInput(), "  > "),
		utils.PrefixLines(payload, "  | "),
	))
	operator.Exit()
}
