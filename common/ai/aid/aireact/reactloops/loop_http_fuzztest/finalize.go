package loop_http_fuzztest

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	loopHTTPFuzzFinalizeReferenceMaxBytes = 12 * 1024
	loopHTTPFuzzFinalizeTimelineMaxBytes  = 6 * 1024
	loopHTTPFuzzFinalizeDiffPreviewBytes  = 4 * 1024
)

// BuildOnPostIterationHook 在 ReActLoop 退出时投递 HTTP Fuzz Test 阶段总结。
// 设计原则：优先让 AI 基于 timeline + 测试参考资料生成对话式总结，
// 失败时再回退到本地 lite 模板，保证 UI 上不会出现空白。
// 关键词: post_iteration, finalize, http_fuzztest, AI summary, fallback
func BuildOnPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		if !isDone {
			return
		}
		persistLoopHTTPFuzzSessionContext(loop, "post_iteration")
		if hasLoopHTTPFuzzFinalAnswerDelivered(loop) || hasLoopHTTPFuzzDirectlyAnswered(loop) || getLoopHTTPFuzzLastAction(loop) == "directly_answer" {
			if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
				operator.IgnoreError()
			}
			return
		}

		// 关键词: AI summary, DirectlyAnswer, reference material
		if tryDeliverLoopHTTPFuzzFinalizeViaAI(loop, invoker, reason) {
			if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
				operator.IgnoreError()
			}
			return
		}

		// 关键词: lite fallback, finalize summary
		finalContent := generateLoopHTTPFuzzFinalizeSummary(loop, reason)
		deliverLoopHTTPFuzzFinalizeSummary(loop, invoker, finalContent)
		if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
			operator.IgnoreError()
		}
	})
}

// tryDeliverLoopHTTPFuzzFinalizeViaAI 尝试让 AI 基于 timeline 与参考资料
// 生成对话式总结。返回 true 表示 AI 已经成功投递，外层不再走 lite 回退。
// 关键词: DirectlyAnswer, AddToTimeline, reference material, http_fuzztest finalize
func tryDeliverLoopHTTPFuzzFinalizeViaAI(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, reason any) bool {
	if loop == nil || invoker == nil {
		return false
	}

	referenceMaterial := buildLoopHTTPFuzzReferenceMaterial(loop, reason)
	if strings.TrimSpace(referenceMaterial) == "" {
		return false
	}

	ctx := getLoopHTTPFuzzFinalizeContext(loop)

	// 关键词: timeline_injection, http_fuzztest_finalize_context, finalize summary
	timelineMaterial := utils.ShrinkTextBlock(referenceMaterial, loopHTTPFuzzFinalizeTimelineMaxBytes)
	invoker.AddToTimeline("http_fuzztest_finalize_context", timelineMaterial)

	query := buildLoopHTTPFuzzSummarizationQuery(loop, reason)

	answer, err := invoker.DirectlyAnswer(
		ctx,
		query,
		nil,
		aicommon.WithDirectlyAnswerReferenceMaterial(referenceMaterial, 0),
	)
	if err != nil {
		log.Warnf("http_fuzztest finalize: DirectlyAnswer failed, falling back to lite summary: %v", err)
		return false
	}
	if strings.TrimSpace(answer) == "" {
		log.Warnf("http_fuzztest finalize: DirectlyAnswer returned empty answer, falling back to lite summary")
		return false
	}

	markLoopHTTPFuzzFinalAnswerDelivered(loop)
	recordLoopHTTPFuzzMetaAction(loop, "finalize_summary", "专注模式退出时由 AI 生成阶段总结", utils.ShrinkTextBlock(answer, 240))
	persistLoopHTTPFuzzSessionContext(loop, "finalize_summary")
	invoker.AddToTimeline("http_fuzztest_finalize", "Delivered AI conversational summary for loop_http_fuzztest")
	return true
}

// buildLoopHTTPFuzzReferenceMaterial 把动作清单 / 已测试 payload / 验证结论 /
// 代表性 HTTPFlow / diff_result 等结构化信息聚合成 markdown 风格的参考资料，
// 供 DirectlyAnswer 作为 reference material 使用。
// 关键词: reference_material, finalize, action_records, diff_result, verification
func buildLoopHTTPFuzzReferenceMaterial(loop *reactloops.ReActLoop, reason any) string {
	if loop == nil {
		return ""
	}

	var out strings.Builder

	if task := loop.GetCurrentTask(); task != nil {
		if userInput := strings.TrimSpace(task.GetUserInput()); userInput != "" {
			out.WriteString("## 用户原始需求\n\n")
			out.WriteString(userInput)
			out.WriteString("\n\n")
		}
	}

	if currentSummary := strings.TrimSpace(getCurrentRequestSummary(loop)); currentSummary != "" {
		out.WriteString("## 当前生效请求摘要\n\n")
		out.WriteString(currentSummary)
		out.WriteString("\n\n")
	}

	records := getLoopHTTPFuzzRecentActions(loop)
	if len(records) > 0 {
		out.WriteString(fmt.Sprintf("## 已执行动作 (共 %d 个)\n\n", len(records)))
		for idx, record := range records {
			out.WriteString(fmt.Sprintf("### 动作 #%d - %s\n", idx+1, record.ActionName))
			if record.ParamSummary != "" {
				out.WriteString(fmt.Sprintf("- 参数: %s\n", record.ParamSummary))
			}
			if record.ResultSummary != "" {
				out.WriteString(fmt.Sprintf("- 结果: %s\n", record.ResultSummary))
			}
			if record.VerificationSummary != "" {
				out.WriteString(fmt.Sprintf("- 验证: %s\n", record.VerificationSummary))
			}
			if record.RepresentativeHTTPFlow != "" {
				out.WriteString(fmt.Sprintf("- 代表性 HTTPFlow: %s\n", record.RepresentativeHTTPFlow))
			}
			if len(record.Payloads) > 0 {
				out.WriteString(fmt.Sprintf("- 本次 payload (%d 个): %s\n", len(record.Payloads), shrinkLoopHTTPFuzzList(record.Payloads, 8, 320)))
			}
			out.WriteString("\n")
		}
	}

	tested := getLoopHTTPFuzzTestedPayloads(loop)
	if len(tested) > 0 {
		out.WriteString("## 累计已测试 Payload\n\n")
		actionNames := make([]string, 0, len(tested))
		for actionName, payloads := range tested {
			if len(payloads) == 0 {
				continue
			}
			actionNames = append(actionNames, actionName)
		}
		sort.Strings(actionNames)
		for _, actionName := range actionNames {
			payloads := tested[actionName]
			out.WriteString(fmt.Sprintf("- %s (%d 个): %s\n", actionName, len(payloads), shrinkLoopHTTPFuzzList(payloads, 8, 320)))
		}
		out.WriteString("\n")
	}

	if hiddenIndex := strings.TrimSpace(loop.Get("representative_httpflow_hidden_index")); hiddenIndex != "" {
		out.WriteString("## 代表性 HTTPFlow\n\n")
		out.WriteString(fmt.Sprintf("- HTTPFlow ID: %s\n\n", hiddenIndex))
	}

	if diff := firstNonEmptyString(loop.Get("diff_result_compressed"), loop.Get("diff_result_analysis")); diff != "" {
		out.WriteString("## 关键响应差异 / 分析\n\n")
		out.WriteString(utils.ShrinkTextBlock(diff, loopHTTPFuzzFinalizeDiffPreviewBytes))
		out.WriteString("\n\n")
	}

	if verification := strings.TrimSpace(loop.Get("verification_result")); verification != "" {
		out.WriteString("## 验证结论原文\n\n")
		out.WriteString(verification)
		out.WriteString("\n\n")
	}

	if reasonErr, ok := reason.(error); ok && reasonErr != nil {
		out.WriteString("## 退出原因\n\n")
		out.WriteString(strings.TrimSpace(reasonErr.Error()))
		out.WriteString("\n")
	}

	return strings.TrimSpace(utils.ShrinkTextBlock(out.String(), loopHTTPFuzzFinalizeReferenceMaxBytes))
}

// generateLoopHTTPFuzzFinalizeSummary 输出 lite 兜底总结。
// 设计原则：仅在 AI 不可用时使用，所以追求"一行可读"的口语化短句，
// 不输出 markdown 标题、列表项或冗余引导语。
// 关键词: finalize_summary, lite_summary, fallback, concise
func generateLoopHTTPFuzzFinalizeSummary(loop *reactloops.ReActLoop, reason any) string {
	var parts []string

	// 已执行动作清单，浓缩到一句"已执行 N 个动作 (a / b)"
	// 关键词: action_count, action_names, lite_summary
	records := getLoopHTTPFuzzRecentActions(loop)
	if len(records) > 0 {
		actionNames := summarizeLoopHTTPFuzzActionNames(records)
		if len(actionNames) > 0 {
			parts = append(parts, fmt.Sprintf("已执行 %d 个动作 (%s)", len(records), strings.Join(actionNames, " / ")))
		} else {
			parts = append(parts, fmt.Sprintf("已执行 %d 个动作", len(records)))
		}
	}

	// 一句话验证结论
	// 关键词: verification_verdict, lite_summary
	if verdict := extractLoopHTTPFuzzVerificationVerdict(loop.Get("verification_result")); verdict != "" {
		parts = append(parts, verdict)
	}

	// 代表性 HTTPFlow，让用户可以直接定位关键请求
	// 关键词: representative_httpflow, lite_summary
	if hiddenIndex := strings.TrimSpace(loop.Get("representative_httpflow_hidden_index")); hiddenIndex != "" {
		parts = append(parts, fmt.Sprintf("代表性 HTTPFlow: %s", hiddenIndex))
	}

	summary := strings.TrimSpace(strings.Join(parts, "; "))

	// 异常退出原因仅在是 error 时追加，正常 reason 不展示
	// 关键词: exit_reason, lite_summary
	if reasonErr, ok := reason.(error); ok && reasonErr != nil {
		exitReason := fmt.Sprintf("(退出原因: %s)", strings.TrimSpace(reasonErr.Error()))
		if summary == "" {
			summary = "本轮 HTTP Fuzz Test 异常退出 " + exitReason
		} else {
			summary = summary + " " + exitReason
		}
	}

	return strings.TrimSpace(summary)
}

// buildLoopHTTPFuzzSummarizationQuery 构造发给 DirectlyAnswer 的对话提问。
// 设计原则：尽量短、不要 markdown 标题，让 AI 输出 1-3 句口语化结论，
// 覆盖测试位置 / 数量 / 关键差异 / 漏洞迹象 / 是否达成目标 / 末尾附 HTTPFlow id 即可。
// 关键词: summarization_query, finalize, DirectlyAnswer, prompt, concise
func buildLoopHTTPFuzzSummarizationQuery(loop *reactloops.ReActLoop, reason any) string {
	userInput := ""
	if loop != nil {
		if task := loop.GetCurrentTask(); task != nil {
			userInput = strings.TrimSpace(task.GetUserInput())
		}
	}

	var out strings.Builder
	out.WriteString("基于参考资料用 1-3 句中文口语，给出本轮 HTTP 安全模糊测试的精炼结论：")
	out.WriteString("测试位置、发送数量、关键差异、漏洞迹象、是否达成目标。")
	out.WriteString("不要标题、列表、编号或任何 markdown，纯短句；如有代表性 HTTPFlow id 直接附在末尾。")
	if reasonErr, ok := reason.(error); ok && reasonErr != nil {
		out.WriteString(fmt.Sprintf(" 本轮异常退出 (%s)，请提示用户。", strings.TrimSpace(reasonErr.Error())))
	}
	if userInput != "" {
		out.WriteString(" 用户原始需求：")
		out.WriteString(userInput)
	}
	return out.String()
}

// getLoopHTTPFuzzFinalizeContext 选择一个可用的 context，用于 DirectlyAnswer。
// 优先用当前任务的 context，其次用 loop 配置上下文，最后兜底 context.Background()。
// 关键词: finalize, context, DirectlyAnswer
func getLoopHTTPFuzzFinalizeContext(loop *reactloops.ReActLoop) context.Context {
	if loop == nil {
		return context.Background()
	}
	if ctx := getLoopTaskContext(loop); ctx != nil {
		return ctx
	}
	if cfg := loop.GetConfig(); cfg != nil {
		if ctx := cfg.GetContext(); ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

// summarizeLoopHTTPFuzzActionNames 从最近动作记录里提取去重的 action_name 列表，
// 保持原有出现顺序，便于在 lite summary 中按时间顺序展示。
// 关键词: action_names, lite_summary, dedupe
func summarizeLoopHTTPFuzzActionNames(records []loopHTTPFuzzActionRecord) []string {
	if len(records) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(records))
	names := make([]string, 0, len(records))
	for _, record := range records {
		name := strings.TrimSpace(record.ActionName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// extractLoopHTTPFuzzVerificationVerdict 解析 verification_result 文本中的 Satisfied 字段，
// 返回中文一句话的验证结论。无法识别则返回空字符串。
// 关键词: verification_verdict, Satisfied, lite_summary
func extractLoopHTTPFuzzVerificationVerdict(verification string) string {
	verification = strings.TrimSpace(verification)
	if verification == "" {
		return ""
	}
	for _, line := range strings.Split(verification, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), "satisfied:") {
			continue
		}
		value := strings.TrimSpace(trimmed[len("Satisfied:"):])
		switch strings.ToLower(value) {
		case "true":
			return "已达到当前安全测试目标"
		case "false":
			return "未达到当前安全测试目标"
		}
		return ""
	}
	return ""
}

func deliverLoopHTTPFuzzFinalizeSummary(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, finalContent string) {
	finalContent = strings.TrimSpace(finalContent)
	if finalContent == "" || hasLoopHTTPFuzzFinalAnswerDelivered(loop) || hasLoopHTTPFuzzDirectlyAnswered(loop) || getLoopHTTPFuzzLastAction(loop) == "directly_answer" {
		return
	}
	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent("re-act-loop-answer-payload", strings.NewReader(finalContent), taskID, func() {}); err != nil {
			log.Warnf("http_fuzztest finalize: failed to emit markdown stream: %v", err)
		}
	}
	invoker.EmitResultAfterStream(finalContent)
	markLoopHTTPFuzzFinalAnswerDelivered(loop)
	recordLoopHTTPFuzzMetaAction(loop, "finalize_summary", "专注模式退出时自动补充阶段总结", utils.ShrinkTextBlock(finalContent, 240))
	persistLoopHTTPFuzzSessionContext(loop, "finalize_summary")
	invoker.AddToTimeline("http_fuzztest_finalize", "Delivered fallback summary for loop_http_fuzztest")
}
