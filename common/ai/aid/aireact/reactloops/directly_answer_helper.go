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
// 核心约定 (与 high_static_section.txt 的 "## 任务状态机制: next_movements"
// 以及 "统一入口与终结" 对齐): directly_answer 先交付答复, 随后根据 runtime
// 状态决定自动结束或继续. 无活跃 TODO / next_movements / 显式续跑请求时直接
// Exit, 避免额外消耗一轮只输出 finish; 阶段性答复则 Continue.
//
// 语义分支:
//   - 携带 next_movements 增量: timeline 标注循环将继续推进这些 TODO 更新.
//   - 未携带增量: timeline 标注答复已交付, 需要时用 finish 收尾. 若当前任务
//     仍有未关闭 (pending/doing) TODO, 额外 Feedback 提醒 AI 先把 TODO 关掉
//     再 finish (finish 会被 blocked-by-todo 闸门拦住, 提前告知更顺滑).
//
// 注意: next_movements 增量的 store apply 由主循环 (exec.go 的
// applyNextMovementsBottomLine) 在 ActionHandler 之前完成, 所以这里
// GetBlockingVerificationTodoItems 读到的就是 apply 之后的状态.
//
// 关键词: directly_answer 自动结束, continue_after_answer, finish-only round elimination,
//
//	directly_answer 改起来很简单
const loopIntentHintSimpleQuery = "simple_query"

// Timeline 里记录用户可见答复时，不使用 directly_answer 等 action 名，避免 agent
// 从 timeline 反推出「可随时调用 directly_answer」。
const (
	TimelineEntryAssistantOutput     = "assistant_output"
	TimelineEntryAssistantOutputNote = "assistant_output_note"
	TimelineAssistantOutputLabel     = "assistant output:"
)

// ShouldAutoFinishAfterDirectlyAnswer reports whether a delivered answer can end
// the loop without spending another model round on the mechanical finish action.
// Models can explicitly keep the loop alive with continue_after_answer=true.
func ShouldAutoFinishAfterDirectlyAnswer(loop *ReActLoop, action *aicommon.Action) bool {
	if loop == nil || action == nil {
		return false
	}
	if action.GetBool("continue_after_answer") || action.GetInvokeParams("next_action").GetBool("continue_after_answer") {
		return false
	}
	if len(aicommon.NormalizeVerifyNextMovements(action)) > 0 {
		return false
	}
	if loop.ShouldBlockFinishAtIteration(loop.GetCurrentIterationIndex()) {
		return false
	}
	cfg := loop.GetConfig()
	task := loop.GetCurrentTask()
	if cfg != nil && task != nil {
		if len(aicommon.GetBlockingVerificationTodoItems(cfg, task)) > 0 {
			return false
		}
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
	invoker := loop.GetInvoker()
	if len(aicommon.NormalizeVerifyNextMovements(action)) > 0 {
		if !utils.IsNil(invoker) {
			invoker.AddToTimeline(TimelineEntryAssistantOutputNote,
				"assistant output delivered; the loop continues to honor the scheduled next_movements. "+
					"Use the 'finish' action to end the task once all work is done.")
		}
		operator.Continue()
		return
	}
	if ShouldAutoFinishAfterDirectlyAnswer(loop, action) {
		if !utils.IsNil(invoker) {
			invoker.AddToTimeline(
				TimelineEntryAssistantOutputNote,
				"assistant output delivered and CURRENT-TASK has no active TODO or requested continuation; "+
					"terminating the ReAct loop without an extra finish-only model round.",
			)
		}
		operator.Exit()
		return
	}
	if !utils.IsNil(invoker) {
		invoker.AddToTimeline(TimelineEntryAssistantOutputNote,
			"assistant output delivered; the task is NOT complete yet. "+
				"The ReAct loop CONTINUES automatically — the user does NOT need to reply '继续'/'continue' for you to proceed. "+
				"If you already know the next step, EXECUTE it now in the next iteration via require_tool / directly_call_tool / request_plan; "+
				"do NOT merely announce it and wait for the user's permission. "+
				"When the entire CURRENT-TASK is complete, use the 'finish' action to terminate the ReAct loop. "+
				"Do not repeat the same answer; continue with tools, next_movements, or finish.")
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
