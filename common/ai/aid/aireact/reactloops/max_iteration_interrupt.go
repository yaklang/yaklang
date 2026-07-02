package reactloops

// LoopFinishEmission 描述一次 loop 收尾时, 全局兜底应当如何对客户端表态.
type LoopFinishEmission int

const (
	// LoopFinishSilent: 保持静默 (loop 通过 IgnoreError 自管收尾, 常见于隐藏/内部 loop).
	LoopFinishSilent LoopFinishEmission = iota
	// LoopFinishSuccess: 自然结束, EmitReActSuccess. 普通成功收尾与"到达迭代上限的
	// 软性中断"都归到这里 —— 迭代上限不是错误, 客户端一律表现为自然结束.
	LoopFinishSuccess
	// LoopFinishFail: 硬错误收尾, EmitReActFail (携带错误信息).
	LoopFinishFail
)

// ClassifyLoopFinishEmission 是全局收尾兜底的纯决策逻辑, 抽成独立函数以便单测覆盖:
//   - IgnoreError 优先 -> 静默 (隐藏/内部 loop 自管收尾);
//   - 到达迭代上限的软性中断 -> 自然结束(success). 迭代上限属于"步数用尽"而非错误,
//     虽然 reason 里带着 maxIterErr, 也不当作硬失败上报 (对比硬中断: 不报错). 中断
//     原因 / 未完成 TODO / 下一步建议由各 loop 已有的 finalize 收尾总结承载 (它会读
//     取框架落在 timeline / interrupt store 里的上下文), 框架层不再额外发一次 AI 请求;
//   - 其余"已结束且带 reason(错误)" -> 硬失败(fail, 携带错误信息);
//   - 其余 -> 成功.
//
// 关键词: 全局收尾决策, max iteration 软中断 自然结束, 对比硬中断报错
func ClassifyLoopFinishEmission(isDone bool, reason any, ignoreError bool, maxIterationInterrupted bool) LoopFinishEmission {
	if ignoreError {
		return LoopFinishSilent
	}
	if isDone && maxIterationInterrupted {
		return LoopFinishSuccess
	}
	if isDone && reason != nil {
		return LoopFinishFail
	}
	return LoopFinishSuccess
}
