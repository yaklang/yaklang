package reactloops

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

// DirectlyAnswerContinue 是 directly_answer 收尾的单一决策点, 供内置
// directly_answer 与各 loop 专用 directly_answer 复用, 调用方把答复 emit 完
// 之后调它代替裸 operator.Exit(), 让 "改 directly_answer 很简单".
//
// 核心约定 (与 high_static_section.txt 的 "## 任务状态机制: TODO" 的 "对任务终结的影响"子节
// 以及 "统一入口与终结" 对齐): directly_answer 只交付答复, 不应被当成
// 通用终结器. 普通任务由 finish 收口; 但 intent classifier 已明确标记为
// simple_query、本轮无有效 todo_delta 且无开放 TODO 时, host 在答复交付后
// 直接 Exit, 避免为了补一个 finish 再次调模型.
//
// 语义分支:
//   - 携带 todo_delta 增量: timeline 标注循环将继续推进这些 TODO 更新.
//   - 未携带增量: timeline 标注答复已交付, 需要时用 finish 收尾. 若当前任务
//     仍有未关闭 (pending/doing) TODO, 额外 Feedback 提醒 AI 先把 TODO 关掉
//     再 finish (finish 会被 blocked-by-todo 闸门拦住, 提前告知更顺滑).
//
// 注意: todo_delta 增量的 store apply 由主循环 (exec.go 的
// applyTodoDeltaBottomLine) 在 ActionHandler 之前完成, 所以这里
// GetBlockingVerificationTodoItems 读到的就是 apply 之后的状态.
//
// Timeline 里记录用户可见答复时，不使用 directly_answer 等 action 名，避免 agent
// 从 timeline 反推出「可随时调用 directly_answer」。
const (
	TimelineEntryAssistantOutput     = "assistant_output"
	TimelineEntryAssistantOutputNote = "assistant_output_note"
	TimelineAssistantOutputLabel     = "assistant output:"
	loopIntentHintSimpleQuery        = "simple_query"

	loopVarDirectlyAnswerDeliveredWithoutTodoDelta = "directly_answer_delivered_without_todo_delta"

	// TimelineEntryModelThinking is the timeline entry type for the pure AI
	// reasoning/thinking stream captured during an iteration. It is display-only
	// and excluded from prompt projection (see timeline_prompt_projection.go).
	TimelineEntryModelThinking = "model_thinking"
)

const errDuplicateDirectlyAnswerWithoutTodoDelta = "assistant output was already delivered for this CURRENT-TASK without an effective todo_delta; " +
	"do not emit another answer or rephrase the previous one. Use 'finish' if the latest user input is fully answered, " +
	"or choose a tool action and maintain todo_delta if more work remains."

func directlyAnswerHasTodoDelta(action *aicommon.Action) bool {
	delta, err := aicommon.NormalizeTodoDelta(action)
	return err == nil && delta != nil
}

func directlyAnswerDeliveredWithoutTodoDelta(loop *ReActLoop) bool {
	if loop == nil {
		return false
	}
	value, _ := loop.GetVariable(loopVarDirectlyAnswerDeliveredWithoutTodoDelta).(bool)
	return value
}

// RejectDuplicateDirectlyAnswerWithoutTodoDelta blocks a second user-visible
// answer in the same CURRENT-TASK when the first answer did not schedule any
// state change. A directly_answer with an effective todo_delta remains valid:
// it is a progress handoff rather than an accidental replay.
func RejectDuplicateDirectlyAnswerWithoutTodoDelta(loop *ReActLoop, action *aicommon.Action) error {
	if loop == nil || action == nil || directlyAnswerHasTodoDelta(action) || !directlyAnswerDeliveredWithoutTodoDelta(loop) {
		return nil
	}
	return utils.Error(errDuplicateDirectlyAnswerWithoutTodoDelta)
}

// FinishDirectlyAnswerVerification applies the duplicate guard shared by the
// built-in and specialized directly_answer actions, then stores their payload.
func FinishDirectlyAnswerVerification(loop *ReActLoop, action *aicommon.Action, payload string) error {
	if err := RejectDuplicateDirectlyAnswerWithoutTodoDelta(loop, action); err != nil {
		return err
	}
	loop.Set("directly_answer_payload", payload)
	return nil
}

func noteDirectlyAnswerDeliveredWithoutTodoDelta(loop *ReActLoop, action *aicommon.Action) {
	if loop == nil || action == nil || directlyAnswerHasTodoDelta(action) {
		return
	}
	loop.Set(loopVarDirectlyAnswerDeliveredWithoutTodoDelta, true)
}

// ShouldAutoFinishAfterSimpleQueryDirectlyAnswer identifies the narrow host
// guard for greetings, status checks, and other classifier-approved trivial
// inquiries. No extra model round is useful when the answer was delivered and
// neither todo_delta nor the persistent TODO store contains remaining work.
func ShouldAutoFinishAfterSimpleQueryDirectlyAnswer(loop *ReActLoop, action *aicommon.Action) bool {
	if loop == nil || action == nil || strings.TrimSpace(loop.Get("intent_hint")) != loopIntentHintSimpleQuery {
		return false
	}
	if directlyAnswerHasTodoDelta(action) {
		return false
	}
	if items := aicommon.GetBlockingVerificationTodoItems(loop.GetConfig(), loop.GetCurrentTask()); len(items) > 0 {
		return false
	}
	return true
}

func DirectlyAnswerContinue(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
	if operator == nil {
		return
	}
	if loop == nil {
		operator.Continue()
		return
	}
	noteDirectlyAnswerDeliveredWithoutTodoDelta(loop, action)
	invoker := loop.GetInvoker()
	if directlyAnswerHasTodoDelta(action) {
		if !utils.IsNil(invoker) {
			invoker.AddToTimeline(TimelineEntryAssistantOutputNote,
				"assistant output delivered; the loop continues to honor the scheduled todo_delta. "+
					"Use the 'finish' action to end the task once all work is done.")
		}
		operator.Continue()
		return
	}
	if ShouldAutoFinishAfterSimpleQueryDirectlyAnswer(loop, action) {
		if !utils.IsNil(invoker) {
			invoker.AddToTimeline(TimelineEntryAssistantOutputNote,
				"simple_query answer delivered; CURRENT-TASK has no effective todo_delta or open TODO. "+
					"The host is closing this trivial exchange without another model iteration.")
		}
		operator.Exit()
		return
	}
	if !utils.IsNil(invoker) {
		invoker.AddToTimeline(TimelineEntryAssistantOutputNote,
			"assistant output delivered. Do not repeat or rephrase the same answer. "+
				"Re-evaluate CURRENT-TASK: if the latest user input is fully answered and no open TODO remains, use 'finish' now; "+
				"otherwise continue the existing Current with tools and maintain todo_delta. "+
				"The user does not need to reply 'continue'.")
	}
	if items := aicommon.GetBlockingVerificationTodoItems(loop.GetConfig(), loop.GetCurrentTask()); len(items) > 0 {
		operator.Feedback(buildExitBlockedByTodoMessage("finish", items))
	}
	operator.Continue()
}

// WrapDirectlyAnswerError 给 React Loop 内置 directly_answer ActionVerifier 的
// 报错统一附加 nonce 化的 AITAG retry hint, 让 RetryPromptBuilder 把它注入下一轮
// 提示, 引导 AI 用 FINAL_ANSWER tag 重发结构化答案, 而不是再次空 answer_payload.
//
// 背景: 上轮 hostscan 长跑暴露 directly_answer 5 次重试黑洞 - ActionVerifier
// 只抛纯文字 "answer_payload is required for ActionDirectlyAnswer but empty",
// AI 拿不到 AITAG 示例或 nonce, 5 次重试都同样错下去, 最终 fatal abort 浪费
// 14% 时间 (~2 分钟) 与 ~1.2MB 的 token. r.DirectlyAnswer() 独立路径
// (invoke_directly_answer.go:errorWarp) 早就有同款 hint 注入但 React Loop 内
// 4 个内置 directly_answer 都漏了, 本 helper 把同款修复挪过来共用.
//
// nonce 取自 loop.Get("last_ai_decision_nonce") - 由 reactloops/exec.go 在
// ExtractActionFromStream 之后立即写入, ActionVerifier 调用前一定已就位.
// 缺 nonce (异常路径) 不阻塞, 退化成最小 hint, 至少不让原错信息丢失.
//
// 关键词: WrapDirectlyAnswerError AITAG retry hint, directly_answer 5 次重试黑洞修复,
// last_ai_decision_nonce, FINAL_ANSWER tag 自纠正
func WrapDirectlyAnswerError(loop *ReActLoop, err error) error {
	if err == nil {
		return nil
	}
	if loop == nil {
		// 极端兜底: loop 引用都没了, 仍按最小 hint 包一层, 维持错误链.
		return utils.Wrap(err, "AITAG retry hint: missing loop context, fallback minimal hint")
	}
	nonce := strings.TrimSpace(loop.Get("last_ai_decision_nonce"))
	if nonce == "" {
		return utils.Wrap(err, "AITAG retry hint: missing nonce, fallback minimal hint")
	}
	return utils.Wrapf(err,
		"AITAG retry hint: previous response missed answer_payload AND FINAL_ANSWER tag. "+
			"For long/multi-line/markdown answers, you MUST emit AITAG block instead of "+
			"answer_payload. Example:\n"+
			`{"@action":"directly_answer"}`+"\n"+
			"<|FINAL_ANSWER_%s|>\n"+
			"# your markdown answer here\n"+
			"<|FINAL_ANSWER_END_%s|>",
		nonce, nonce,
	)
}
