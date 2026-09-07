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
		"current task still has %d open TODO item(s); %s cannot exit yet. Do not retry finish, bulk-close items, or express TODO changes in prose or custom tags: only the action JSON's todo_delta field changes state. Continue unresolved work with a tool action. When existing observations already prove an item terminal, close that exact open ID through todo_delta.close with an evidence-backed outcome, non-empty reason, and refs in the next action.\nRemaining TODOs:\n%s",
		len(items),
		actionName,
		strings.Join(lines, "\n"),
	)
}

func buildFinishBlockedByGoalModeMessage() string {
	return "goal mode's host-side completion gate is not open yet. Do not retry finish or clear unfinished TODOs to satisfy it. Continue the most valuable evidence-producing work, and use directly_answer only for a necessary progress update."
}

var loopAction_Finish = &LoopAction{
	ActionType: "finish",
	Description: "Request completion of the current task. The first valid request starts one soft TODO checkpoint; confirm with finish again after that checkpoint to exit. " +
		"This is the normal terminator for non-trivial ReAct tasks; the only narrow host exception is a classifier-approved simple_query with no effective todo_delta or open TODO. " +
		"Use it when evidence/results are already present in the timeline and no evidence-backed, in-scope, immediately executable next action would materially improve confidence, risk coverage, or impact assessment " +
		"(tool outputs are captured automatically and the system will synthesize a summary). " +
		"Do NOT precede this action with bash echo/cat/tee/printf calls that only restate facts " +
		"already produced by earlier tool calls — that wastes iterations. " +
		"CRITICAL: finish is a claim that the requested work is exhausted, not a TODO-reset command. If the current task still owns active TODO items, finish will be rejected until each is actually resolved, discriminatively dismissed, or externally blocked and deferred with evidence. " +
		"Never bulk-close, downgrade, or fabricate outcomes merely to pass finish. A deferred item remains unfinished history; if its continuation condition is now satisfied, create a new open continuation TODO with a new ID and execute it before trying finish again. " +
		"If the user needs a structured Markdown answer emitted to the chat, use 'directly_answer' first " +
		"(it delivers the answer but does NOT end the task), then call 'finish'. " +
		"Add 'human_readable_thought' only if a brief closing note is needed.",
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
		if loop.ShouldBlockFinishAtIteration(loop.GetCurrentIterationIndex()) {
			msg := buildFinishBlockedByGoalModeMessage()
			loop.invoker.AddToTimeline("[GOAL_MODE_FINISH_BLOCKED]", msg)
			operator.Feedback(msg)
			operator.Continue()
			return
		}
		// Known open work is a concrete blocker and should be reported before the
		// general completion audit. Showing the soft checkpoint first makes the
		// model audit an impossible finish, then retry finish instead of advancing
		// or applying an evidence-backed todo_delta.
		if items := aicommon.GetBlockingVerificationTodoItems(loop.GetConfig(), loop.GetCurrentTask()); len(items) > 0 {
			msg := buildExitBlockedByTodoMessage("finish", items)
			loop.invoker.AddToTimeline("[FINISH_BLOCKED_BY_TODO]", msg)
			operator.Feedback(msg)
			operator.Continue()
			return
		}
		if !loop.requestSoftTodoCheckpoint() {
			msg := "finish requested; a soft TODO checkpoint will be shown in the next context before termination can be confirmed"
			if loop.invoker != nil {
				loop.invoker.AddToTimeline("SOFT_TODO_CHECKPOINT_REQUESTED", msg)
			}
			operator.Feedback(msg)
			operator.Continue()
			return
		}
		if loop.invoker != nil {
			loop.invoker.AddToTimeline("finish", "AI confirmed finish after the soft TODO checkpoint")
		}
		operator.Exit()
	},
}

var loopAction_DirectlyAnswer = &LoopAction{
	ActionType: "directly_answer",
	Description: "Emit a direct answer to the user via 'answer_payload' or FINAL_ANSWER tag. For simple direct answers, omit 'human_readable_thought'. " +
		"For ordinary tasks directly_answer ONLY delivers the answer; use 'finish' when the latest user input is fully answered and no open TODO remains. " +
		"A classifier-approved simple_query with no effective todo_delta and no open TODO is closed by the host immediately after delivery. " +
		"Do not call directly_answer twice without an effective todo_delta in the same CURRENT-TASK; repeated or rephrased answers are rejected. " +
		"Carry a non-empty 'todo_delta' alongside a progress answer whenever it changes or schedules follow-up TODO state.",
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
		return FinishDirectlyAnswerVerification(loop, action, payload)
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

		// 答复交付后由共享收口决定续跑、simple_query 自动结束，
		// 或提醒下一轮通过 todo_delta / finish 收敛，避免重复回答。
		invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
		invoker.EmitResultAfterStream(payload)
		invoker.AddToTimeline(TimelineEntryAssistantOutput, fmt.Sprintf("user input: \n"+
			"%s\n"+
			"%s\n"+
			"%v",
			utils.PrefixLines(loop.GetCurrentTask().GetUserInput(), "  > "),
			TimelineAssistantOutputLabel,
			utils.PrefixLines(payload, "  | "),
		))
		DirectlyAnswerContinue(loop, action, operator)
	},
}
