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
				// 已经给过答复, 但因为迭代上限被中断: 补一条简短中断说明, 告诉
				// 用户哪些没做完 + 询问下一步 (回复 "继续"). 收尾 IgnoreError.
				deliverMaxIterationInterruptNotice(loop, invoker)
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
// 简短的中断说明气泡: 明确任务被中断、列出未完成的 TODO、并询问用户下一步
// (回复 "继续" 续跑, 或给新方向). 不覆盖已给出的答复, 只做增量提示.
//
// 关键词: 迭代上限软中断补充说明, 未完成 TODO 交接, 询问下一步
func deliverMaxIterationInterruptNotice(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime) {
	if loop == nil || invoker == nil {
		return
	}

	var notice strings.Builder
	notice.WriteString("> 任务因达到迭代次数上限被自动中断（这不是错误，只是本轮已用尽可执行的步数）。\n\n")

	if unfinished := strings.TrimSpace(loop.GetMaxIterationInterruptSummary()); unfinished != "" {
		notice.WriteString("**尚未完成、已标记为 SKIP 的事项：**\n\n")
		notice.WriteString(unfinished)
		notice.WriteString("\n\n")
	} else {
		notice.WriteString("当前没有明确记录在案的未完成待办，但分析流程被提前中断，可能仍有可以深入的方向。\n\n")
	}

	notice.WriteString("**接下来你可以：**\n")
	notice.WriteString("- 直接回复 “继续”，我会接着之前的分析继续做未完成的部分；\n")
	notice.WriteString("- 或者告诉我新的方向/侧重点，我按你的调整来分析。\n")

	payload := notice.String()

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
			log.Warnf("http_flow_analyze finalize: failed to emit max-iteration interrupt notice: %v", err)
		}
	}
	invoker.EmitResultAfterStream(payload)
	invoker.AddToTimeline("http_flow_analysis_interrupted",
		fmt.Sprintf("HTTP flow analysis interrupted by iteration limit after %d iterations; asked user how to proceed",
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
