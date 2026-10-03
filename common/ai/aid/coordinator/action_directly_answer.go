package coordinator

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func configureCoordinatorAnswer(loop *reactloops.ReActLoop) error {
	answer, err := loop.GetActionHandler("directly_answer")
	if err != nil {
		return err
	}
	answerCopy := *answer
	answerCopy.Description = "向用户说明进展或答复，之后继续协调。短答复使用 answer_payload；长答复使用主循环声明的 FINAL_ANSWER AITAG，两者不可同时输出。"
	answerCopy.NativeDescription = "向用户说明进展或答复，之后继续协调；完整正文写入 arguments.answer_payload，不使用外置 AITAG。"
	answerCopy.Options = []aitool.ToolOption{aitool.WithStringParam("answer_payload", aitool.WithParam_Description("短答复正文；长答复使用主循环声明的 FINAL_ANSWER AITAG，两者不可同时输出。"))}
	answerCopy.NativeOptions = []aitool.ToolOption{aitool.WithStringParam("answer_payload", aitool.WithParam_Description("向用户交付的完整答复，支持 Markdown 或代码；不使用外置 AITAG。"), aitool.WithParam_Required())}
	// A coordinator communicates repeatedly in one long-lived task. New inbox
	// batches permit a new progress answer; the mainloop's one-answer task guard
	// would incorrectly demand finish. Replays of the same batch remain blocked.
	answerCopy.ActionVerifier = func(l *reactloops.ReActLoop, a *aicommon.Action) error {
		payload := a.GetString("answer_payload")
		if payload == "" {
			payload = a.GetInvokeParams("next_action").GetString("answer_payload")
		}
		if payload == "" {
			payload = a.GetString("tag_final_answer")
		}
		if payload == "" {
			payload = l.Get("tag_final_answer")
		}
		if strings.TrimSpace(payload) == "" {
			return reactloops.WrapDirectlyAnswerError(l, fmt.Errorf("answer_payload or FINAL_ANSWER is required"))
		}
		cursor, _ := l.GetVariable("coordinator_message_cursor").(uint64)
		previous, answered := l.GetVariable("coordinator_answer_cursor").(uint64)
		if answered && previous == cursor {
			return fmt.Errorf("同一消息批次已经答复；继续使用工具或 wait_messages")
		}
		// Native batch execution also applies the shared duplicate guard. Reset
		// its task-level latch only for this verified new inbox boundary.
		l.Set("directly_answer_delivered_without_todo_delta", false)
		l.Set("directly_answer_payload", payload)
		return nil
	}
	answerCopy.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		l.Set("coordinator_last_action", "directly_answer")
		l.Set("coordinator_answer_cursor", l.GetVariable("coordinator_message_cursor"))
		if !l.FunctionCallModeEnabled() {
			gate := reactloops.NewActionHandlerOperator(op.GetTask())
			answer.ActionHandler(l, a, gate)
			if done, err := gate.IsTerminated(); done && err != nil {
				op.Fail(err)
				return
			}
			op.Continue()
			return
		}
		l.GetEmitter().EmitTextMarkdownStreamEvent("re-act-loop-answer-payload", strings.NewReader(a.GetString("answer_payload")), "")
		op.Continue()
	}
	answerCopy.FunctionCallAction = nil
	reactloops.WithOverrideLoopAction(&answerCopy)(loop)
	return nil
}
