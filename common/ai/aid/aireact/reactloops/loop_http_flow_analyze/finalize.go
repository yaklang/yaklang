package loop_http_flow_analyze

import (
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func buildPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		if !isDone {
			collectIterationFindings(loop)
			return
		}

		log.Infof("http_flow_analyze loop done at iteration %d", iteration)
		collectIterationFindings(loop)

		// 是否为"到达迭代上限"的软性中断. exec.go 已把标记置位并把未完成 TODO
		// 标记 SKIP, 这里只负责把"任务中断 + 未完成清单 + 询问下一步"融进直接回答.
		// 关键词: max iteration 软性中断, http_flow_analyze finalize
		maxIterInterrupt := isMaxIterationInterrupt(loop, reason)

		if hasFinalAnswerDelivered(loop) || hasDirectlyAnswered(loop) || getLastAction(loop) == "directly_answer" {
			log.Infof("http_flow_analyze: answer already delivered before finalize")
			if maxIterInterrupt {
				// 已经给过答复, 但因为迭代上限被中断: 让 AI 依据本次分析的真实
				// 上下文因地制宜地补一条"续做指引", 告诉用户哪些没做完 + 启发下
				// 一步 (回复 "继续" 续跑, 或给新方向). 收尾 IgnoreError.
				deliverMaxIterationInterruptNotice(loop, invoker, collectFinalizeContextMaterials(loop, reason))
				operator.IgnoreError()
			}
			return
		}

		contextMaterials := collectFinalizeContextMaterials(loop, reason)
		deliverFinalAnswerFallback(loop, invoker, contextMaterials, maxIterInterrupt)

		if maxIterInterrupt {
			operator.IgnoreError()
		}
	})
}

// isMaxIterationInterrupt 判断本次 loop 结束是否因为"到达迭代上限"被软性中断.
// 以 exec.go 置位的 loop 标记为准, 同时兼容 reason 里携带 "max iterations" 文案.
func isMaxIterationInterrupt(loop *reactloops.ReActLoop, reason any) bool {
	if loop != nil && loop.IsMaxIterationInterrupted() {
		return true
	}
	if reasonErr, ok := reason.(error); ok && reasonErr != nil {
		return strings.Contains(reasonErr.Error(), "max iterations")
	}
	if reasonStr, ok := reason.(string); ok {
		return strings.Contains(reasonStr, "max iterations")
	}
	return false
}

// deliverMaxIterationInterruptNotice 在"答复已下发但仍撞到迭代上限"时补发一条
// 续做指引: 内容不写死, 而是让 AI 依据本次分析的真实上下文 (已积累证据 / 最近
// 动作 / 未完成 TODO) 因地制宜地生成 —— 冷静说明这是"被中断"而非失败, 结合具体
// 调查进展点出还有哪些没做、给出针对性的下一步方向来启发用户, 并告知可回复
// "继续" 续跑或给新方向. 不覆盖已给出的答复, 只做增量提示. AI 不可用时才退回到
// 极简兜底文案 (deliverMaxIterationInterruptNoticeFallback).
//
// 关键词: 迭代上限软中断续做指引, AI 因地制宜生成, 未完成 TODO 交接, 询问下一步
func deliverMaxIterationInterruptNotice(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, contextMaterials string) {
	if loop == nil || invoker == nil {
		return
	}

	contextMaterials = strings.TrimSpace(contextMaterials)

	userQuery := ""
	if task := loop.GetCurrentTask(); task != nil {
		userQuery = strings.TrimSpace(task.GetUserInput())
	}

	nonce := utils.RandStringBytes(8)
	noticePrompt := utils.MustRenderTemplate(`
<|INSTRUCTION_{{ .Nonce }}|>
You are an HTTP traffic analysis expert. An analysis answer has ALREADY been delivered to the user, but the analysis loop was then INTERRUPTED because it reached the iteration/step limit. This is NOT an error or a failure.

Write a SHORT continuation note (do NOT repeat the full report). Requirements:
1. Calmly state that the analysis was interrupted because the step limit was reached, and make clear this is NOT a failure.
2. Ground everything in the SPECIFIC context below (accumulated evidence, recent actions, unfinished TODOs). Point out concretely what was left undone, then give 2-4 tailored, inspiring next-step directions for THIS particular investigation. Do NOT give generic advice; refer to the actual endpoints/params/findings seen so far.
3. End by telling the user they can simply reply "继续" (continue) to resume the unfinished work, or give a new direction/focus to adjust.

Keep it concise: a few short paragraphs or a short bullet list. Use clear Markdown.

CRITICAL LANGUAGE RULE: Write the ENTIRE note in the SAME language as the user's query below. If the user wrote in Chinese, write in Chinese; if in English, write in English. Do NOT mix languages.

User's original query: {{ .UserQuery }}
<|INSTRUCTION_END_{{ .Nonce }}|>

<|CONTEXT_{{ .Nonce }}|>
{{ .ContextMaterials }}
<|CONTEXT_END_{{ .Nonce }}|>
`, map[string]any{
		"Nonce":            nonce,
		"UserQuery":        userQuery,
		"ContextMaterials": contextMaterials,
	})

	reactloops.EmitStatus(loop, "生成续做指引 / Generating Continuation Guidance...")

	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}

	action, err := invoker.InvokeSpeedPriorityLiteForge(
		loop.GetConfig().GetContext(),
		"http_flow_analyze_interrupt_notice",
		noticePrompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("continuation_note",
				aitool.WithParam_Description("Short, context-aware continuation note in Markdown that explains the interruption and inspires the user on what to do next"),
				aitool.WithParam_Required(true),
			),
		},
		aicommon.WithGeneralConfigStreamableFieldEmitterCallback([]string{
			"continuation_note",
		}, func(key string, r io.Reader, emitter *aicommon.Emitter) {
			if emitter == nil {
				io.Copy(io.Discard, r)
				return
			}
			if event, _ := emitter.EmitStreamEventWithContentType(
				"re-act-loop-answer-payload",
				utils.JSONStringReader(r),
				taskID,
				aicommon.TypeTextMarkdown,
				func() {},
			); event != nil && contextMaterials != "" {
				emitter.EmitTextReferenceMaterial(event.GetStreamEventWriterId(), contextMaterials)
			}
		}),
	)

	if err != nil {
		log.Errorf("http_flow_analyze finalize: AI interrupt notice generation failed: %v", err)
		deliverMaxIterationInterruptNoticeFallback(loop, invoker)
		return
	}

	note := strings.TrimSpace(action.GetString("continuation_note"))
	if note == "" {
		log.Warnf("http_flow_analyze finalize: AI generated empty interrupt notice, using fallback")
		deliverMaxIterationInterruptNoticeFallback(loop, invoker)
		return
	}

	invoker.EmitResultAfterStream(note)
	recordMetaAction(loop, "interrupt_notice",
		"iteration limit continuation guidance",
		utils.ShrinkTextBlock(note, 240))
	invoker.AddToTimeline("http_flow_analysis_interrupted",
		fmt.Sprintf("HTTP flow analysis interrupted by iteration limit after %d iterations; AI generated continuation guidance",
			loop.GetCurrentIterationIndex()))
}

// deliverMaxIterationInterruptNoticeFallback 是 AI 生成续做指引失败时的极简兜底:
// 只在无法调用 AI 时使用, 尽量少写死内容, 仅陈述被中断 + 罗列未完成 TODO + 提示
// 用户可回复 "继续" 或给新方向. 与 deliverRawContextFallback 的降级定位一致.
func deliverMaxIterationInterruptNoticeFallback(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime) {
	if loop == nil || invoker == nil {
		return
	}

	var b strings.Builder
	b.WriteString("> 分析因达到迭代次数上限被中断（这不是错误，只是本轮已用尽可执行的步数）。\n\n")
	if unfinished := strings.TrimSpace(loop.GetMaxIterationInterruptSummary()); unfinished != "" {
		b.WriteString("尚未完成、已标记为 SKIP 的事项：\n\n")
		b.WriteString(unfinished)
		b.WriteString("\n\n")
	}
	b.WriteString("你可以回复 “继续” 让我接着做，或告诉我新的方向/侧重点。\n")
	payload := b.String()

	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent(
			"re-act-loop-answer-payload",
			strings.NewReader(payload),
			taskID,
			func() {},
		); err != nil {
			log.Warnf("http_flow_analyze finalize: failed to emit interrupt notice fallback: %v", err)
		}
	}
	invoker.EmitResultAfterStream(payload)
	invoker.AddToTimeline("http_flow_analysis_interrupted",
		fmt.Sprintf("HTTP flow analysis interrupted by iteration limit after %d iterations (fallback notice)",
			loop.GetCurrentIterationIndex()))
}

func collectIterationFindings(loop *reactloops.ReActLoop) {
	lastAction := loop.GetLastAction()
	if lastAction == nil {
		return
	}
	if lastAction.ActionType == httpFlowEvidenceActionName || lastAction.ActionType == "" {
		return
	}
	if lastAction.ActionParams == nil {
		return
	}
	incoming := normalizeHTTPFlowEvidence(utils.InterfaceToString(lastAction.ActionParams[httpFlowEvidenceFieldName]))
	if incoming == "" {
		return
	}
	if _, changed := appendHTTPFlowEvidence(loop, incoming); changed {
		log.Infof("http_flow_analyze: post-iteration evidence hook merged evidence after action=%s", lastAction.ActionType)
	}
}

func collectFinalizeContextMaterials(loop *reactloops.ReActLoop, reason any) string {
	var ctx strings.Builder
	ctx.WriteString("# HTTP Flow Analysis Context\n\n")

	if task := loop.GetCurrentTask(); task != nil {
		userInput := strings.TrimSpace(task.GetUserInput())
		if userInput != "" {
			ctx.WriteString("## User Query\n\n")
			ctx.WriteString(userInput)
			ctx.WriteString("\n\n")
		}
	}

	if evidence := strings.TrimSpace(loop.Get(httpFlowEvidenceKey)); evidence != "" {
		ctx.WriteString("## Accumulated HTTP Flow Evidence\n\n")
		ctx.WriteString(evidence)
		ctx.WriteString("\n\n")
	}

	if actionsSummary := strings.TrimSpace(buildRecentActionsPrompt(loop)); actionsSummary != "" {
		ctx.WriteString("## Recent Actions\n\n")
		ctx.WriteString(actionsSummary)
		ctx.WriteString("\n\n")
	}

	if lastQuerySummary := strings.TrimSpace(loop.Get("last_query_summary")); lastQuerySummary != "" {
		ctx.WriteString("## Last Query Summary\n\n")
		ctx.WriteString(utils.ShrinkTextBlock(lastQuerySummary, 3000))
		ctx.WriteString("\n\n")
	}

	if lastMatchSummary := strings.TrimSpace(loop.Get("last_match_summary")); lastMatchSummary != "" {
		ctx.WriteString("## Last Match Summary\n\n")
		ctx.WriteString(utils.ShrinkTextBlock(lastMatchSummary, 3000))
		ctx.WriteString("\n\n")
	}

	if currentFlow := strings.TrimSpace(loop.Get("current_flow")); currentFlow != "" {
		ctx.WriteString("## Current Flow Detail\n\n")
		ctx.WriteString(utils.ShrinkTextBlock(currentFlow, 2000))
		ctx.WriteString("\n\n")
	}

	if unfinished := strings.TrimSpace(loop.GetMaxIterationInterruptSummary()); unfinished != "" {
		ctx.WriteString("## Unfinished TODOs (interrupted, marked as SKIP)\n\n")
		ctx.WriteString(unfinished)
		ctx.WriteString("\n\n")
	}

	ctx.WriteString("## Exit Reason\n\n")
	if reasonErr, ok := reason.(error); ok && reasonErr != nil {
		ctx.WriteString(reasonErr.Error())
	} else if reasonStr := strings.TrimSpace(utils.InterfaceToString(reason)); reasonStr != "" {
		ctx.WriteString(reasonStr)
	} else {
		ctx.WriteString("Analysis phase completed normally.")
	}

	return strings.TrimSpace(ctx.String())
}

func deliverFinalAnswerFallback(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, contextMaterials string, maxIterInterrupt bool) {
	contextMaterials = strings.TrimSpace(contextMaterials)
	if contextMaterials == "" {
		log.Infof("http_flow_analyze finalize: skip fallback because context materials are empty")
		return
	}

	if hasFinalAnswerDelivered(loop) || hasDirectlyAnswered(loop) {
		log.Infof("http_flow_analyze finalize: skip fallback because answer was already delivered")
		return
	}

	userQuery := ""
	if task := loop.GetCurrentTask(); task != nil {
		userQuery = strings.TrimSpace(task.GetUserInput())
	}

	// 到达迭代上限的软性中断: 让报告以"任务被中断"的口吻收尾, 明确列出没来得及
	// 做的事情, 并询问用户下一步 (回复"继续"续跑, 或给新方向). 非中断的正常收尾
	// 则沿用原来的完整报告口吻.
	// 关键词: 迭代上限软中断报告, 未完成清单, 询问下一步(继续)
	interruptGuidance := ""
	if maxIterInterrupt {
		interruptGuidance = `
IMPORTANT - This analysis was INTERRUPTED because it reached the iteration limit (this is NOT a failure):
6. Clearly state at the top that the analysis was interrupted because the step/iteration limit was reached, so it did not fully finish.
7. Explicitly list what was NOT completed in time (see the "Unfinished TODOs" section in the context if present); these were auto-marked as SKIP.
8. End by asking the user how to proceed: they can simply reply "继续" (continue) to resume the unfinished work, or provide a new direction/focus to adjust.`
	}

	nonce := utils.RandStringBytes(8)
	summaryPrompt := utils.MustRenderTemplate(`
<|INSTRUCTION_{{ .Nonce }}|>
You are an HTTP traffic analysis expert. Based on the analysis context below, generate a complete analysis report for the user.

Requirements:
1. Summarize all collected traffic information and HTTP flow evidence
2. Answer the user's question based on available evidence
3. If information is insufficient, explain what was attempted and possible reasons
4. Provide concrete discoveries and actionable recommendations
5. Use clear, professional Markdown formatting{{ .InterruptGuidance }}

CRITICAL LANGUAGE RULE: You MUST write the ENTIRE report in the SAME language as the user's query below. If the user wrote in Chinese, your report MUST be in Chinese. If the user wrote in English, your report MUST be in English. Do NOT mix languages.

User's original query: {{ .UserQuery }}
<|INSTRUCTION_END_{{ .Nonce }}|>

<|CONTEXT_{{ .Nonce }}|>
{{ .ContextMaterials }}
<|CONTEXT_END_{{ .Nonce }}|>
`, map[string]any{
		"Nonce":             nonce,
		"UserQuery":         userQuery,
		"ContextMaterials":  contextMaterials,
		"InterruptGuidance": interruptGuidance,
	})

	log.Infof("http_flow_analyze finalize: generating forced AI answer, prompt length: %d", len(summaryPrompt))

	reactloops.EmitStatus(loop, "生成分析报告 / Generating Report...")

	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}

	action, err := invoker.InvokeSpeedPriorityLiteForge(
		loop.GetConfig().GetContext(),
		"http_flow_analyze_finalize_summary",
		summaryPrompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("summary",
				aitool.WithParam_Description("Complete HTTP traffic analysis report in Markdown format"),
				aitool.WithParam_Required(true),
			),
		},
		aicommon.WithGeneralConfigStreamableFieldEmitterCallback([]string{
			"summary",
		}, func(key string, r io.Reader, emitter *aicommon.Emitter) {
			if emitter == nil {
				io.Copy(io.Discard, r)
				return
			}
			if event, _ := emitter.EmitStreamEventWithContentType(
				"re-act-loop-answer-payload",
				utils.JSONStringReader(r),
				taskID,
				aicommon.TypeTextMarkdown,
				func() {},
			); event != nil {
				streamId := event.GetStreamEventWriterId()
				emitter.EmitTextReferenceMaterial(streamId, contextMaterials)
			}
		}),
	)

	if err != nil {
		log.Errorf("http_flow_analyze finalize: AI summary generation failed: %v", err)
		deliverRawContextFallback(loop, invoker, contextMaterials)
		return
	}

	summary := strings.TrimSpace(action.GetString("summary"))
	if summary == "" {
		log.Warnf("http_flow_analyze finalize: AI generated empty summary, using raw context fallback")
		deliverRawContextFallback(loop, invoker, contextMaterials)
		return
	}

	invoker.EmitResultAfterStream(summary)
	markFinalAnswerDelivered(loop)
	recordMetaAction(loop, "finalize_summary",
		"forced AI answer at loop exit",
		utils.ShrinkTextBlock(summary, 240))
	invoker.AddToTimeline("http_flow_analysis_finalized",
		fmt.Sprintf("HTTP flow analysis finalized after %d iterations with AI generated summary",
			loop.GetCurrentIterationIndex()))
}

func deliverRawContextFallback(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, contextMaterials string) {
	if hasFinalAnswerDelivered(loop) {
		return
	}

	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}

	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent(
			"re-act-loop-answer-payload",
			strings.NewReader(contextMaterials),
			taskID,
			func() {},
		); err != nil {
			log.Warnf("http_flow_analyze finalize: failed to emit raw context fallback: %v", err)
		}
	}

	invoker.EmitResultAfterStream(contextMaterials)
	markFinalAnswerDelivered(loop)
	invoker.AddToTimeline("http_flow_analysis_finalized_raw",
		fmt.Sprintf("HTTP flow analysis finalized with raw context after %d iterations",
			loop.GetCurrentIterationIndex()))
}
