package reactloops

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

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
