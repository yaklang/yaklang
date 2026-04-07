package loop_http_fuzztest

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

var loopActionDirectlyAnswerHTTPFuzztest = &reactloops.LoopAction{
	ActionType:  "directly_answer",
	Description: "只用于回答 HTTP 安全测试过程中的简短问题，例如当前发现、下一步计划、为何怀疑某漏洞、为何需要人工审核。回答前会保留当前会话中的原始包、当前生效包和 merge 摘要。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"answer_payload",
			aitool.WithParam_Description(`仅在回答简短测试过程问答时使用。短答案可放 answer_payload；长答案请使用 <|FINAL_ANSWER_...|> 标签。不要把 fuzz、改包或整包生成动作伪装成 directly_answer。`),
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
		if payload == "" {
			payload = strings.TrimSpace(loop.Get("tag_final_answer"))
		}
		if payload == "" {
			return utils.Error("answer_payload is required for directly_answer but empty")
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

		persistLoopHTTPFuzzSessionContext(loop, "directly_answer")
		invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
		invoker.EmitResultAfterStream(payload)

		var timeline strings.Builder
		timeline.WriteString("user input:\n")
		timeline.WriteString(utils.PrefixLines(loop.GetCurrentTask().GetUserInput(), "  > "))
		timeline.WriteString("\nai directly answer:\n")
		timeline.WriteString(utils.PrefixLines(payload, "  | "))
		if currentSummary := getCurrentRequestSummary(loop); currentSummary != "" {
			timeline.WriteString("\ncurrent request summary:\n")
			timeline.WriteString(utils.PrefixLines(currentSummary, "  = "))
		}
		if changeSummary := strings.TrimSpace(loop.Get("request_change_summary")); changeSummary != "" {
			timeline.WriteString("\nlatest merge summary:\n")
			timeline.WriteString(utils.PrefixLines(utils.ShrinkTextBlock(changeSummary, 800), "  ~ "))
		}
		if decision := strings.TrimSpace(loop.Get("request_review_decision")); decision != "" {
			timeline.WriteString(fmt.Sprintf("\nrequest review decision: %s", decision))
		}
		invoker.AddToTimeline("directly_answer", timeline.String())
		operator.Exit()
	},
}
