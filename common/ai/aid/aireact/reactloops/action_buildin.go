package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func buildExitBlockedByTodoMessage(actionName string, items []aicommon.VerificationTodoItem) string {
	if len(items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, aicommon.FormatVerificationTodoLine(item))
	}
	return fmt.Sprintf(
		"current task still has %d active TODO item(s); %s cannot exit until each one is explicitly closed via adjust_todolist or verification next_movements with op=done / op=delete / op=skip.\nRemaining TODOs:\n%s",
		len(items),
		actionName,
		strings.Join(lines, "\n"),
	)
}

var loopAction_Finish = &LoopAction{
	ActionType: "finish",
	Description: "Mark the current task as finished and exit the loop IMMEDIATELY. " +
		"PREFERRED completion action whenever evidence/results are already present in the timeline " +
		"(tool outputs are captured automatically and the system will synthesize a summary). " +
		"Do NOT precede this action with bash echo/cat/tee/printf calls that only restate facts " +
		"already produced by earlier tool calls — that wastes iterations. " +
		"CRITICAL: if the current task still owns active TODO items, finish will be rejected until those TODOs are explicitly closed. " +
		"Use 'directly_answer' instead only when the user explicitly needs a structured Markdown " +
		"answer emitted to the chat right now. Add 'human_readable_thought' only if a brief closing note is needed.",
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
		if items := aicommon.GetBlockingVerificationTodoItems(loop.GetConfig(), loop.GetCurrentTask()); len(items) > 0 {
			msg := buildExitBlockedByTodoMessage("finish", items)
			loop.invoker.AddToTimeline("[FINISH_BLOCKED_BY_TODO]", msg)
			operator.Feedback(msg)
			operator.Continue()
			return
		}
		loop.invoker.AddToTimeline("finish", "AI decided mark the current Task is finished")
		operator.Exit()
	},
}

var loopAction_DirectlyAnswer = &LoopAction{
	ActionType: "directly_answer",
	Description: "Answer the user directly via 'answer_payload' or FINAL_ANSWER tag. For simple direct answers, omit 'human_readable_thought'. " +
		"CRITICAL: directly_answer is blocked while the current task still owns active TODO items; close them first with adjust_todolist or verification next_movements.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"answer_payload",
			aitool.WithParam_Description(`USE THIS FIELD ONLY IF @action is 'directly_answer' AND answer is short (≤200 chars). For long answers, leave this empty and use '<|FINAL_ANSWER_...|>' tags after JSON. CRITICAL: answer_payload and <|FINAL_ANSWER_...|> are STRICTLY MUTUALLY EXCLUSIVE - never use both simultaneously.`),
		),
	},
	AITagStreamFields: []*LoopAITagField{
		{
			TagName:      "FINAL_ANSWER",
			VariableName: "tag_final_answer",
			AINodeId:     "re-act-loop-answer-payload",
			ContentType:  aicommon.TypeTextMarkdown,
		},
	},
	StreamFields: []*LoopStreamField{
		{
			FieldName:   "answer_payload",
			AINodeId:    "re-act-loop-answer-payload",
			ContentType: aicommon.TypeTextMarkdown,
		},
	},
	ActionVerifier: func(loop *ReActLoop, action *aicommon.Action) error {
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
			// 用 WrapDirectlyAnswerError 把纯文字错误升级为带 nonce AITAG 示例的
			// 复合错误, 让 RetryPromptBuilder 把 hint 注入下一轮 prompt, AI 在 1-2 次
			// 重试内就能用 FINAL_ANSWER tag 自纠正, 避免 5 次重试黑洞 + fatal abort.
			// 关键词: directly_answer ActionVerifier AITAG hint, 5 次重试黑洞修复
			return WrapDirectlyAnswerError(loop, utils.Error("answer_payload is required for ActionDirectlyAnswer but empty"))
		}
		loop.Set("directly_answer_payload", payload)
		return nil
	},
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
		invoker := loop.GetInvoker()
		payload := loop.Get(`directly_answer_payload`)
		if payload == "" {
			payload = loop.Get("tag_final_answer")
		}

		if payload == "" {
			operator.Fail("directly_answer action must have 'answer_payload' field")
			return
		}
		if items := aicommon.GetBlockingVerificationTodoItems(loop.GetConfig(), loop.GetCurrentTask()); len(items) > 0 {
			msg := buildExitBlockedByTodoMessage("directly_answer", items)
			invoker.AddToTimeline("[DIRECT_ANSWER_BLOCKED_BY_TODO]", msg)
			operator.Feedback(msg)
			operator.Continue()
			return
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
	},
}
