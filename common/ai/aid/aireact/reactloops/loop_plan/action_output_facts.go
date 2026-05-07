package loop_plan

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

// resolveOutputFactsContent 把 output_facts 这个 action 内嵌的 facts 内容
// 按优先级解析出来:
//  1. action.params[facts] (来自 JSON 字段, 或 buildActionTagOption 双注册
//     ExtraNonces 命中后由 ForceSet 写入)
//  2. AI 原始响应里所有 <|FACTS_*|> AITag 块拼接 (兜底任意 nonce, 包括既非
//     turn nonce 也非 CURRENT_NONCE 的随机字符串场景)
//
// 这两层 fallback 只解决"AI 已经清楚地表达了 output_facts 意图但 nonce 写错"
// 的场景; 真正完全没提供任何内容的兜底由 handler 调用 autoGenerateFacts 完成.
//
// 关键词: resolveOutputFactsContent, FACTS 多层兜底, 尊重 JSON action 意图
func resolveOutputFactsContent(loop *reactloops.ReActLoop, action *aicommon.Action) string {
	if facts := normalizeFactsDocument(action.GetString(PlanFactsFieldName)); facts != "" {
		return facts
	}
	if loop == nil {
		return ""
	}
	rawResp := loop.Get("last_ai_decision_response")
	if rawResp == "" {
		return ""
	}
	return extractFactsAITagFromRawResponse(rawResp)
}

// verifyOutputFactsAction 是 output_facts 动作的 verifier. 拆成命名函数以便
// 单元测试直接调用 (避免反射/闭包逃逸).
//
// 行为契约: 只要 AI 已经声明了 {"@action":"output_facts"} 这个动作意图就
// 全部接受, 永远返回 nil. facts 字段缺失/空的情况由 handler 兜底, 不再升级
// 到 AI Transaction 重试层. 这是消除 [AI Transaction Failed] After 5 attempts
// 黑洞的关键: 旧 verifier 在 facts 为空时返回 "facts content is required",
// CallAITransaction 把它当重试触发条件, 同 prompt 同模型反复 5 次都同样空,
// 最后致命中断把整条任务规划链路搞挂.
//
// 关键词: verifyOutputFactsAction, output_facts verifier 容错, 尊重 JSON action,
//
//	避免 5 次重试黑洞
func verifyOutputFactsAction(_ *reactloops.ReActLoop, _ *aicommon.Action) error {
	return nil
}

// handleOutputFactsAction 是 output_facts 动作的 handler. 拆成命名函数以便
// 单元测试直接调用. 多层兜底链路:
//  1. action.params[facts] (JSON 字段或 AITag 双注册命中)
//  2. 从 last_ai_decision_response 原始响应里再扫一遍所有 FACTS AITag 块
//  3. 仍为空时调用 autoGenerateFacts 让系统基于上下文自动补一份
//
// 这里第 3 步必须显式触发, 因为 buildPlanPostIterationHook 中
// shouldAutoFactsForAction("output_facts") 返回 false, post-action hook
// 默认会跳过自动补 facts 的路径.
//
// 关键词: handleOutputFactsAction, output_facts handler 多层兜底,
//
//	shouldAutoFactsForAction 排除补偿
func handleOutputFactsAction(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
	facts := resolveOutputFactsContent(loop, action)
	if facts == "" && loop != nil {
		log.Warnf("plan loop: output_facts action received no facts via JSON, AITag, or raw response; falling back to auto-generation")
		facts = autoGenerateFacts(loop, loop.GetCurrentTask(), "incremental", loop.GetLastAction())
	}
	merged, changed := appendPlanFacts(loop, facts)
	if changed {
		log.Infof("plan loop: output_facts merged, length=%d", len(merged))
	} else {
		log.Infof("plan loop: output_facts received no new facts")
	}
	op.Continue()
}

var outputFactsAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	_ = r
	return reactloops.WithRegisterLoopActionWithStreamField(
		"output_facts",
		"Append newly observed concrete facts into the shared FACTS document. Prefer the FACTS AITag format: output {\"@action\":\"output_facts\"} and then emit <|FACTS_nonce|>...<|FACTS_END_nonce|>. Facts must be Markdown and contain only precise, verifiable values.",
		[]aitool.ToolOption{
			aitool.WithStringParam("facts",
				aitool.WithParam_Description("本轮新增 facts 的 Markdown 文本。系统会自动与历史 FACTS 合并。也可以不在 JSON 中传递该字段，而是使用 FACTS AITag 输出。"),
			),
		},
		[]*reactloops.LoopStreamField{},
		verifyOutputFactsAction,
		handleOutputFactsAction,
	)
}
