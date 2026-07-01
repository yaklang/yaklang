package reactloops

import (
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// LoopFinishEmission 描述一次 loop 收尾时, 全局兜底应当如何对客户端表态.
type LoopFinishEmission int

const (
	// LoopFinishSilent: 保持静默 (loop 通过 IgnoreError 自管收尾, 常见于隐藏/内部 loop).
	LoopFinishSilent LoopFinishEmission = iota
	// LoopFinishSuccess: 普通成功收尾, EmitReActSuccess.
	LoopFinishSuccess
	// LoopFinishFail: 硬错误收尾, EmitReActFail (携带错误信息).
	LoopFinishFail
	// LoopFinishSuccessWithInterruptSummary: 到达迭代上限的软性中断. 客户端表现为
	// "自然结束"(EmitReActSuccess), 并额外补发一段框架层 AI 生成的中断说明.
	LoopFinishSuccessWithInterruptSummary
)

// ClassifyLoopFinishEmission 是全局收尾兜底的纯决策逻辑, 抽成独立函数以便单测覆盖:
//   - IgnoreError 优先 -> 静默 (隐藏/内部 loop 自管收尾);
//   - 到达迭代上限的软性中断 -> 自然结束(success) + 补发中断说明 (对比硬中断: 不报错);
//   - 其余"已结束且带 reason(错误)" -> 硬失败(fail, 携带错误信息);
//   - 其余 -> 成功.
//
// 关键词: 全局收尾决策, max iteration 软中断 自然结束, 对比硬中断报错
func ClassifyLoopFinishEmission(isDone bool, reason any, ignoreError bool, maxIterationInterrupted bool) LoopFinishEmission {
	if ignoreError {
		return LoopFinishSilent
	}
	if isDone && maxIterationInterrupted {
		return LoopFinishSuccessWithInterruptSummary
	}
	if isDone && reason != nil {
		return LoopFinishFail
	}
	return LoopFinishSuccess
}

// DeliverMaxIterationInterruptSummary 是"到达迭代上限软性中断"的框架层统一收尾,
// 与具体 loop / 专注模式完全解耦. 它让 AI 依据当前会话的真实上下文 (用户诉求 /
// 未完成 TODO / 最近动作) 因地制宜地生成一段简短说明:
//   - 明确本轮是因为"达到最大迭代次数"而自然结束的, 这不是错误;
//   - 列出还没来得及做完 (已被 SKIP) 的 TODO;
//   - 启发用户下一步可以做什么;
//   - 告知用户可以直接回复 "继续" 续跑未完成的部分, 或直接开启一个新话题.
//
// AI 不可用 (调用失败或返回空) 时退回极简兜底文案. 内容不写死, 交由 AI 生成.
//
// 该方法由 re-act 主循环的全局收尾兜底在"软性中断 且 未被 IgnoreError"时调用且仅
// 调用一次. 隐藏 / 内部 loop (在自己的 finalize 里 IgnoreError 自管收尾) 不会触发
// 它, 因此不会被打扰.
//
// 关键词: 迭代上限软中断框架收尾, 自然结束, 未完成 TODO 交接, 询问下一步(继续)
func (r *ReActLoop) DeliverMaxIterationInterruptSummary() {
	if r == nil {
		return
	}
	invoker := r.GetInvoker()
	if utils.IsNil(invoker) {
		return
	}

	task := r.GetCurrentTask()
	userQuery := ""
	if !utils.IsNil(task) {
		userQuery = strings.TrimSpace(task.GetUserInput())
	}
	contextMaterials := r.buildMaxIterationInterruptContext(task)

	taskID := ""
	if !utils.IsNil(task) {
		taskID = task.GetId()
	}

	nonce := utils.RandStringBytes(8)
	prompt := utils.MustRenderTemplate(`
<|INSTRUCTION_{{ .Nonce }}|>
The assistant loop just STOPPED because it reached its maximum iteration/step limit. This is a NATURAL, EXPECTED stop, NOT an error and NOT a failure.

Write a SHORT closing note for the user (do NOT redo the whole task). Requirements:
1. Calmly state that work stopped because the step/iteration limit was reached (make clear this is NOT an error, the run simply ran out of allowed steps this round).
2. Based on the SPECIFIC context below (the user's request, the unfinished TODOs, and recent actions), point out concretely what was NOT finished in time (these unfinished items were auto-marked as SKIP), and give 2-4 tailored, inspiring suggestions for what to do next about THIS particular task. Do NOT give generic filler; refer to the actual work seen so far.
3. End by telling the user they can simply reply "继续" (continue) to resume the unfinished work, or start a brand new topic / give a new direction.

Keep it concise: a few short sentences or a short bullet list. Use clear Markdown.

CRITICAL LANGUAGE RULE: Write the ENTIRE note in the SAME language as the user's request below. If the user wrote in Chinese, write in Chinese; if in English, write in English. Do NOT mix languages.

User's original request: {{ .UserQuery }}
<|INSTRUCTION_END_{{ .Nonce }}|>

<|CONTEXT_{{ .Nonce }}|>
{{ .ContextMaterials }}
<|CONTEXT_END_{{ .Nonce }}|>
`, map[string]any{
		"Nonce":            nonce,
		"UserQuery":        userQuery,
		"ContextMaterials": contextMaterials,
	})

	r.loadingStatus("生成中断说明 / Generating interruption summary...")

	action, err := invoker.InvokeSpeedPriorityLiteForge(
		r.GetConfig().GetContext(),
		"max_iteration_interrupt_summary",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("continuation_note",
				aitool.WithParam_Description("Short, context-aware closing note in Markdown that explains the iteration-limit stop and inspires the user on what to do next"),
				aitool.WithParam_Required(true),
			),
		},
		aicommon.WithGeneralConfigStreamableFieldEmitterCallback([]string{
			"continuation_note",
		}, func(key string, rd io.Reader, emitter *aicommon.Emitter) {
			if emitter == nil {
				io.Copy(io.Discard, rd)
				return
			}
			if event, _ := emitter.EmitStreamEventWithContentType(
				"re-act-loop-answer-payload",
				utils.JSONStringReader(rd),
				taskID,
				aicommon.TypeTextMarkdown,
				func() {},
			); event != nil && contextMaterials != "" {
				emitter.EmitTextReferenceMaterial(event.GetStreamEventWriterId(), contextMaterials)
			}
		}),
	)
	if err != nil {
		log.Errorf("react loop[%s] max-iteration interrupt summary generation failed: %v", r.loopNameOrDefault(), err)
		r.deliverMaxIterationInterruptSummaryFallback(invoker, taskID)
		return
	}

	note := strings.TrimSpace(action.GetString("continuation_note"))
	if note == "" {
		log.Warnf("react loop[%s] max-iteration interrupt summary is empty, using fallback", r.loopNameOrDefault())
		r.deliverMaxIterationInterruptSummaryFallback(invoker, taskID)
		return
	}

	invoker.EmitResultAfterStream(note)
	invoker.AddToTimeline("iteration_limit_interrupt_summary",
		fmt.Sprintf("[%s] delivered AI continuation guidance after iteration-limit soft interrupt at iteration %d",
			r.loopNameOrDefault(), r.GetCurrentIterationIndex()))
}

// deliverMaxIterationInterruptSummaryFallback 只在 AI 不可用时使用: 尽量少写死内容,
// 仅陈述"因迭代上限自然结束(非错误)" + 罗列未完成 TODO + 提示可回复"继续"或换话题.
func (r *ReActLoop) deliverMaxIterationInterruptSummaryFallback(invoker aicommon.AIInvokeRuntime, taskID string) {
	if r == nil || utils.IsNil(invoker) {
		return
	}

	var b strings.Builder
	b.WriteString("> 本轮已到达最大迭代次数上限，任务在此自然结束（这不是错误，只是本轮可执行的步数已经用尽）。\n\n")
	if unfinished := strings.TrimSpace(r.GetMaxIterationInterruptSummary()); unfinished != "" {
		b.WriteString("尚未完成、已标记为 SKIP 的事项：\n\n")
		b.WriteString(unfinished)
		b.WriteString("\n\n")
	}
	b.WriteString("接下来你可以直接回复 “继续”，我会接着把没做完的部分做完；或者告诉我新的方向，也可以直接开启一个新话题。\n")
	payload := b.String()

	if emitter := r.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent(
			"re-act-loop-answer-payload",
			strings.NewReader(payload),
			taskID,
			func() {},
		); err != nil {
			log.Warnf("react loop[%s] failed to emit max-iteration interrupt fallback: %v", r.loopNameOrDefault(), err)
		}
	}
	invoker.EmitResultAfterStream(payload)
	invoker.AddToTimeline("iteration_limit_interrupt_summary",
		fmt.Sprintf("[%s] delivered fallback continuation guidance after iteration-limit soft interrupt at iteration %d",
			r.loopNameOrDefault(), r.GetCurrentIterationIndex()))
}

// buildMaxIterationInterruptContext 汇总一份与 loop 无关的通用上下文, 供 AI 生成
// 中断说明: 专注模式名 / 迭代上限 / 用户诉求 / 未完成 TODO / 最近动作.
func (r *ReActLoop) buildMaxIterationInterruptContext(task aicommon.AIStatefulTask) string {
	var ctx strings.Builder
	ctx.WriteString("# Iteration-Limit Soft Interrupt Context\n\n")
	ctx.WriteString(fmt.Sprintf("- Focus mode / loop: %s\n", r.loopNameOrDefault()))
	ctx.WriteString(fmt.Sprintf("- Stopped at iteration: %d\n\n", r.GetCurrentIterationIndex()))

	if !utils.IsNil(task) {
		if userInput := strings.TrimSpace(task.GetUserInput()); userInput != "" {
			ctx.WriteString("## User Request\n\n")
			ctx.WriteString(userInput)
			ctx.WriteString("\n\n")
		}
	}

	if unfinished := strings.TrimSpace(r.GetMaxIterationInterruptSummary()); unfinished != "" {
		ctx.WriteString("## Unfinished TODOs (auto-marked SKIP)\n\n")
		ctx.WriteString(unfinished)
		ctx.WriteString("\n\n")
	}

	if recent := strings.TrimSpace(r.buildRecentActionsForInterrupt()); recent != "" {
		ctx.WriteString("## Recent Actions\n\n")
		ctx.WriteString(recent)
		ctx.WriteString("\n\n")
	}

	return strings.TrimSpace(ctx.String())
}

// buildRecentActionsForInterrupt 从 actionHistory 里取最近若干条动作, 拼成通用摘要.
func (r *ReActLoop) buildRecentActionsForInterrupt() string {
	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()

	if len(r.actionHistory) == 0 {
		return ""
	}

	const maxShow = 8
	start := 0
	if len(r.actionHistory) > maxShow {
		start = len(r.actionHistory) - maxShow
	}

	var b strings.Builder
	for _, rec := range r.actionHistory[start:] {
		if rec == nil {
			continue
		}
		name := strings.TrimSpace(rec.ActionName)
		if name == "" {
			name = strings.TrimSpace(rec.ActionType)
		}
		if name == "" {
			continue
		}
		line := fmt.Sprintf("- iter %d: %s", rec.IterationIndex, name)
		if tool := strings.TrimSpace(rec.ToolName); tool != "" {
			line += fmt.Sprintf(" (tool: %s)", tool)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// loopNameOrDefault 返回 loop 名, 空时给一个稳定占位, 便于日志 / timeline 阅读.
func (r *ReActLoop) loopNameOrDefault() string {
	if r == nil {
		return "general-purpose"
	}
	if name := strings.TrimSpace(r.loopName); name != "" {
		return name
	}
	return "general-purpose"
}
