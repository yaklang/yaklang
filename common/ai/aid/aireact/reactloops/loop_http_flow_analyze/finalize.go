package loop_http_flow_analyze

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// BuildOnPostIterationHook 创建迭代后的钩子函数，用于处理循环结束时的逻辑
func BuildOnPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		if isDone {
			log.Infof("http_flow_analyze loop done at iteration %d", iteration)

			// 检查是否因为超出迭代次数而结束
			if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
				log.Infof("http_flow_analyze loop ended due to max iterations, generating final summary with AI")
				// 生成 AI 总结并直接回答
				generateFinalSummaryAndAnswer(loop, invoker, operator)
				// 忽略错误，不让专注模式报错退出
				operator.IgnoreError()
			} else {
				generateFinalSummaryAndAnswer(loop, invoker, operator)
			}
		}
	})
}

// generateFinalSummaryAndAnswer 生成最终总结并调用 directly_answer
func generateFinalSummaryAndAnswer(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, operator *reactloops.OnPostIterationOperator) {
	userQuery := loop.GetCurrentTask().GetUserInput()
	lastQuerySummary := loop.Get("last_query_summary")
	lastMatchSummary := loop.Get("last_match_summary")
	currentFlow := loop.Get("current_flow")

	// 构建上下文信息
	var contextBuilder strings.Builder
	contextBuilder.WriteString("# HTTP 流量分析上下文\n\n")

	contextBuilder.WriteString("## 用户问题\n\n")
	contextBuilder.WriteString(userQuery)
	contextBuilder.WriteString("\n\n")

	if lastQuerySummary != "" {
		contextBuilder.WriteString("## 查询结果摘要\n\n")
		contextBuilder.WriteString(lastQuerySummary)
		contextBuilder.WriteString("\n\n")
	}

	if lastMatchSummary != "" {
		contextBuilder.WriteString("## 匹配结果摘要\n\n")
		contextBuilder.WriteString(lastMatchSummary)
		contextBuilder.WriteString("\n\n")
	}

	if currentFlow != "" {
		contextBuilder.WriteString("## 当前流量详情\n\n")
		contextBuilder.WriteString(currentFlow)
		contextBuilder.WriteString("\n\n")
	}

	contextInfo := contextBuilder.String()

	// 使用 InvokeLiteForge 让 AI 生成总结
	nonce := utils.RandStringBytes(8)
	summaryPrompt := utils.MustRenderTemplate(`
<|INSTRUCTION_{{ .Nonce }}|>
你是一个 HTTP 流量分析专家。现在需要根据以下信息生成一个完整的分析报告。

要求：
1. 总结已经收集到的流量信息
2. 回答用户的问题（基于已有信息）
3. 如果信息不足，说明已经尝试的分析步骤和可能的原因
4. 给出具体的发现和建议

请用清晰、专业的方式组织报告，使用 Markdown 格式。
<|INSTRUCTION_END_{{ .Nonce }}|>

<|CONTEXT_{{ .Nonce }}|>
{{ .ContextInfo }}
<|CONTEXT_END_{{ .Nonce }}|>
`, map[string]any{
		"Nonce":       nonce,
		"ContextInfo": contextInfo,
	})

	log.Infof("generating final summary with AI, prompt length: %d", len(summaryPrompt))

	// 调用 InvokeLiteForge 生成总结
	action, err := invoker.InvokeLiteForge(
		loop.GetConfig().GetContext(),
		"generate_http_flow_analysis_summary",
		summaryPrompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("summary",
				aitool.WithParam_Description("完整的 HTTP 流量分析报告，使用 Markdown 格式"),
				aitool.WithParam_Required(true),
			),
		},
	)

	if err != nil {
		log.Errorf("failed to generate summary with AI: %v", err)
		// 如果 AI 生成失败，使用默认的总结
		generateDefaultSummary(loop, invoker, contextInfo)
		return
	}

	summary := action.GetString("summary")
	if summary == "" {
		log.Warnf("AI generated empty summary, using default summary")
		generateDefaultSummary(loop, invoker, contextInfo)
		return
	}

	log.Infof("AI generated summary length: %d", len(summary))

	loop.GetEmitter().EmitStreamEventWithContentType(
		"re-act-loop-answer-payload",
		strings.NewReader(summary),
		loop.GetCurrentTask().GetId(),
		aicommon.TypeTextMarkdown,
		func() {
		},
	)

	// 将 summary 设置到 loop context 中（directly_answer handler 会从这里读取）
	loop.Set("directly_answer_payload", summary)
	loop.Set("tag_final_answer", summary)

	log.Infof("successfully called directly_answer handler with AI generated summary")

	// 记录到时间线
	invoker.AddToTimeline("http_flow_analysis_completed",
		fmt.Sprintf("HTTP flow analysis completed after %d iterations with AI generated summary",
			loop.GetCurrentIterationIndex()))
}

// generateDefaultSummary 生成默认的总结（当 AI 生成失败时使用）
func generateDefaultSummary(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, contextInfo string) {
	var defaultSummary strings.Builder
	defaultSummary.WriteString("# HTTP 流量分析报告\n\n")
	defaultSummary.WriteString("## 分析状态\n\n")
	defaultSummary.WriteString("⚠️ 已达到最大迭代次数限制。以下是已收集的信息：\n\n")
	defaultSummary.WriteString(contextInfo)
	defaultSummary.WriteString("\n\n## 建议\n\n")
	defaultSummary.WriteString("1. 可以尝试使用更精确的过滤条件重新分析\n")
	defaultSummary.WriteString("2. 检查是否需要查看更多流量详情\n")
	defaultSummary.WriteString("3. 考虑使用不同的匹配器规则\n")

	summary := defaultSummary.String()

	loop.GetEmitter().EmitStreamEventWithContentType(
		"re-act-loop-answer-payload",
		strings.NewReader(summary),
		loop.GetCurrentTask().GetId(),
		aicommon.TypeTextMarkdown,
		func() {
		},
	)
	// 将 summary 设置到 loop context 中
	loop.Set("directly_answer_payload", summary)
	loop.Set("tag_final_answer", summary)

	log.Infof("used default summary as fallback")
	invoker.AddToTimeline("http_flow_analysis_completed_with_default",
		fmt.Sprintf("HTTP flow analysis completed with default summary after %d iterations",
			loop.GetCurrentIterationIndex()))
}
