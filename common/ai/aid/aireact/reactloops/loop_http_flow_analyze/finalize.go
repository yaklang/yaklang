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

		if hasFinalAnswerDelivered(loop) || hasDirectlyAnswered(loop) || getLastAction(loop) == "directly_answer" {
			log.Infof("http_flow_analyze: skip finalize because answer was already delivered")
			return
		}

		contextMaterials := collectFinalizeContextMaterials(loop, reason)
		deliverFinalAnswerFallback(loop, invoker, contextMaterials)

		if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
			operator.IgnoreError()
		}
	})
}

func collectIterationFindings(loop *reactloops.ReActLoop) {
	lastAction := loop.GetLastAction()
	if lastAction == nil {
		return
	}
	if lastAction.ActionType == "output_findings" || lastAction.ActionType == "" {
		return
	}
	if lastAction.ActionParams == nil {
		return
	}
	incoming := normalizeFindings(utils.InterfaceToString(lastAction.ActionParams[findingsFieldName]))
	if incoming == "" {
		return
	}
	if _, changed := appendFindings(loop, incoming); changed {
		log.Infof("http_flow_analyze: post-iteration findings hook merged findings after action=%s", lastAction.ActionType)
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

	if findings := strings.TrimSpace(loop.Get(findingsKey)); findings != "" {
		ctx.WriteString("## Accumulated Findings\n\n")
		ctx.WriteString(findings)
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

func deliverFinalAnswerFallback(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, contextMaterials string) {
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

	nonce := utils.RandStringBytes(8)
	summaryPrompt := utils.MustRenderTemplate(`
<|INSTRUCTION_{{ .Nonce }}|>
You are an HTTP traffic analysis expert. Based on the analysis context below, generate a complete analysis report for the user.

Requirements:
1. Summarize all collected traffic information and findings
2. Answer the user's question based on available evidence
3. If information is insufficient, explain what was attempted and possible reasons
4. Provide concrete discoveries and actionable recommendations
5. Use clear, professional Markdown formatting

CRITICAL LANGUAGE RULE: You MUST write the ENTIRE report in the SAME language as the user's query below. If the user wrote in Chinese, your report MUST be in Chinese. If the user wrote in English, your report MUST be in English. Do NOT mix languages.

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

	log.Infof("http_flow_analyze finalize: generating forced AI answer, prompt length: %d", len(summaryPrompt))

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
		aicommon.WithGeneralConfigStreamableFieldCallback([]string{
			"summary",
		}, func(key string, r io.Reader) {
			if event, _ := loop.GetEmitter().EmitStreamEventWithContentType(
				"re-act-loop-answer-payload",
				utils.JSONStringReader(r),
				taskID,
				aicommon.TypeTextMarkdown,
				func() {},
			); event != nil {
				streamId := event.GetStreamEventWriterId()
				loop.GetEmitter().EmitTextReferenceMaterial(streamId, contextMaterials)
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
