package loop_http_fuzztest

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const (
	loopHTTPFuzzCompressionThreshold = 40 * 1024
	loopHTTPFuzzCompressionTarget    = 20 * 1024
	loopHTTPFuzzTimelinePreviewSize  = 8 * 1024
	modifiedPacketContentField       = "modified_packet_content"
)

func newLoopFuzzRequest(taskCtx context.Context, runtime aicommon.AIInvokeRuntime, rawPacket []byte, isHTTPS bool) (*mutate.FuzzHTTPRequest, error) {
	opts := []mutate.BuildFuzzHTTPRequestOption{
		mutate.OptHTTPS(isHTTPS),
		mutate.OptSource(loopHTTPFuzztestHTTPSource),
	}
	if runtime != nil {
		if cfg := runtime.GetConfig(); cfg != nil {
			if runtimeID := cfg.GetRuntimeId(); runtimeID != "" {
				opts = append(opts, mutate.OptRuntimeId(runtimeID))
			}
		}
	}
	if taskCtx != nil {
		opts = append(opts, mutate.OptContext(taskCtx))
	}
	return mutate.NewFuzzHTTPRequest(rawPacket, opts...)
}

func storeLoopFuzzRequestState(loop *reactloops.ReActLoop, fuzzReq *mutate.FuzzHTTPRequest, requestRaw []byte, isHTTPS bool) {
	_, originalSummary := buildHTTPRequestStreamSummary(string(requestRaw), isHTTPS)
	loop.Set("fuzz_request", fuzzReq)
	loop.Set("original_request", string(requestRaw))
	loop.Set("original_request_summary", originalSummary)
	loop.Set("current_request", string(requestRaw))
	loop.Set("current_request_summary", originalSummary)
	loop.Set("previous_request", "")
	loop.Set("previous_request_summary", "")
	loop.Set("request_change_summary", "")
	loop.Set("request_modification_reason", "")
	loop.Set("request_review_decision", "")
	loop.Set("is_https", utils.InterfaceToString(isHTTPS))
	loop.Set("bootstrap_source", "")
	loop.Set("last_request", "")
	loop.Set("last_request_summary", "")
	loop.Set("last_response", "")
	loop.Set("last_response_summary", "")
	loop.Set("last_httpflow_hidden_index", "")
	loop.Set("representative_request", "")
	loop.Set("representative_response", "")
	loop.Set("representative_httpflow_hidden_index", "")
	loop.Set("diff_result", "")
	loop.Set("diff_result_full", "")
	loop.Set("diff_result_compressed", "")
	loop.Set("verification_result", "")
}

func getCurrentRequestRaw(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	currentRequest := strings.TrimSpace(loop.Get("current_request"))
	if currentRequest != "" {
		return currentRequest
	}
	return strings.TrimSpace(loop.Get("original_request"))
}

func getCurrentRequestSummary(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	summary := strings.TrimSpace(loop.Get("current_request_summary"))
	if summary != "" {
		return summary
	}
	return strings.TrimSpace(loop.Get("original_request_summary"))
}

func setLoopCurrentRequestState(loop *reactloops.ReActLoop, fuzzReq *mutate.FuzzHTTPRequest, requestRaw []byte, isHTTPS bool) {
	if loop == nil {
		return
	}
	_, summary := buildHTTPRequestStreamSummary(string(requestRaw), isHTTPS)
	loop.Set("fuzz_request", fuzzReq)
	loop.Set("current_request", string(requestRaw))
	loop.Set("current_request_summary", summary)
	loop.Set("is_https", utils.InterfaceToString(isHTTPS))
}

func buildRequestModificationFeedback(previousRequest, modifiedRequest []byte, isHTTPS bool, reason, reviewDecision string) string {
	previousSummary := "(none)"
	modifiedSummary := "(none)"
	if len(previousRequest) > 0 {
		_, previousSummary = buildHTTPRequestStreamSummary(string(previousRequest), isHTTPS)
	}
	if len(modifiedRequest) > 0 {
		_, modifiedSummary = buildHTTPRequestStreamSummary(string(modifiedRequest), isHTTPS)
	}

	var out strings.Builder
	out.WriteString("HTTP 数据包修改完成。\n\n")
	if strings.TrimSpace(reason) != "" {
		out.WriteString("=== 修改原因 ===\n")
		out.WriteString(strings.TrimSpace(reason))
		out.WriteString("\n\n")
	}
	out.WriteString("=== 审核结果 ===\n")
	if strings.TrimSpace(reviewDecision) == "" {
		reviewDecision = "auto_applied"
	}
	out.WriteString(reviewDecision)
	out.WriteString("\n\n")
	out.WriteString("=== 修改前摘要 ===\n")
	out.WriteString(previousSummary)
	out.WriteString("\n\n")
	out.WriteString("=== 修改后摘要 ===\n")
	out.WriteString(modifiedSummary)
	out.WriteString("\n\n")
	out.WriteString("=== Merge 变化 ===\n")
	out.WriteString(compareRequests(string(previousRequest), string(modifiedRequest)))
	out.WriteString("\n")
	out.WriteString("\n=== 当前生效数据包 ===\n")
	out.WriteString(string(modifiedRequest))
	out.WriteString("\n")
	return out.String()
}

func buildReviewDecisionLabel(decision string) string {
	switch strings.TrimSpace(strings.ToLower(decision)) {
	case "approved_by_user":
		return "已人工确认并应用新数据包"
	case "rejected_by_user":
		return "人工审核拒绝，保留旧数据包"
	default:
		return "无需人工审核，已直接应用新数据包"
	}
}

func reviewSuggestionApproved(suggestion string) bool {
	s := strings.TrimSpace(strings.ToLower(suggestion))
	if s == "" {
		return false
	}
	if strings.Contains(s, "reject") || strings.Contains(s, "拒绝") || strings.Contains(s, "保留旧") {
		return false
	}
	return strings.Contains(s, "accept") || strings.Contains(s, "approve") || strings.Contains(s, "同意") || strings.Contains(s, "确认") || strings.Contains(s, "使用新") || strings.Contains(s, "应用新")
}

func getLoopTaskContext(loop *reactloops.ReActLoop) context.Context {
	if loop == nil {
		return nil
	}
	task := loop.GetCurrentTask()
	if task == nil {
		return nil
	}
	return task.GetContext()
}

// getFuzzRequest retrieves the FuzzHTTPRequest from loop context
func getFuzzRequest(loop *reactloops.ReActLoop) (*mutate.FuzzHTTPRequest, error) {
	fuzzReqAny := loop.GetVariable("fuzz_request")
	if fuzzReqAny == nil {
		return nil, utils.Error("fuzz_request not found in loop context. Auto bootstrap from user input may have failed; provide a URL/raw HTTP packet or call set_http_request first")
	}
	fuzzReq, ok := fuzzReqAny.(*mutate.FuzzHTTPRequest)
	if !ok {
		return nil, utils.Error("fuzz_request is not a valid FuzzHTTPRequest")
	}
	return fuzzReq, nil
}

// executeFuzzAndCompare executes the fuzz request, keeps the full request/response archive,
// emits compact user-visible summaries, and asks AI whether fuzzing should continue.
func executeFuzzAndCompare(loop *reactloops.ReActLoop, fuzzResult mutate.FuzzHTTPRequestIf, actionName string) (string, *aicommon.VerifySatisfactionResult, error) {
	isHttpsStr := loop.Get("is_https")
	isHttps := isHttpsStr == "true"
	task := loop.GetCurrentTask()
	taskID := ""
	streamTaskID := ""
	var taskCtx context.Context
	if task != nil {
		taskID = task.GetId()
		streamTaskID = taskID
		if streamTaskID == "" {
			streamTaskID = task.GetIndex()
		}
		taskCtx = task.GetContext()
	}
	runtimeID := ""
	invoker := loop.GetInvoker()
	if invoker != nil {
		if cfg := invoker.GetConfig(); cfg != nil {
			runtimeID = cfg.GetRuntimeId()
		}
	}
	if taskCtx == nil {
		taskCtx = context.Background()
	}

	emitFuzzStage(loop, streamTaskID, fmt.Sprintf("开始执行 %s，HTTPFlow 会落库并保留完整请求/响应。", actionName))

	// Execute the fuzz request
	execOpts := []mutate.HttpPoolConfigOption{
		mutate.WithPoolOpt_Https(isHttps),
		mutate.WithPoolOpt_RuntimeId(runtimeID),
		mutate.WithPoolOpt_Source(loopHTTPFuzztestHTTPSource),
		mutate.WithPoolOpt_SaveHTTPFlow(true),
	}
	if taskCtx != nil {
		execOpts = append(execOpts, mutate.WithPoolOpt_Context(taskCtx))
	}
	resultCh, err := fuzzResult.Exec(execOpts...)
	if err != nil {
		return "", nil, utils.Errorf("failed to execute fuzz request: %v", err)
	}

	var diffSummary strings.Builder
	diffSummary.WriteString(fmt.Sprintf("=== Fuzz Results for %s ===\n", actionName))
	var analysisSummary strings.Builder
	analysisSummary.WriteString(fmt.Sprintf("=== 漏洞测试分析：%s ===\n", actionName))

	originalRequest := getCurrentRequestRaw(loop)
	resultCount := 0
	savedFlowCount := 0
	representativeRequest := ""
	representativeResponse := ""
	representativeHiddenIndex := ""

	for result := range resultCh {
		resultCount++
		if result.Error != nil {
			errMsg := fmt.Sprintf("\n--- Result %d ---\nError: %v\n", resultCount, result.Error)
			diffSummary.WriteString(errMsg)
			analysisSummary.WriteString(errMsg)
			emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 第 %d 个测试请求执行失败：%v", actionName, resultCount, result.Error))
			continue
		}

		// Get request and response
		requestRaw := string(result.RequestRaw)
		responseRaw := string(result.ResponseRaw)
		requestURL, requestStreamSummary := buildHTTPRequestStreamSummary(requestRaw, isHttps)
		responseStreamSummary := buildHTTPResponseStreamSummary(responseRaw, requestURL)

		if streamTaskID != "" {
			emitPacketSummary(loop, streamTaskID, actionName, resultCount, "request", requestStreamSummary)
			emitPacketSummary(loop, streamTaskID, actionName, resultCount, "response", responseStreamSummary)
		}

		hiddenIndex := ""
		if result.LowhttpResponse != nil {
			hiddenIndex = strings.TrimSpace(result.LowhttpResponse.HiddenIndex)
			if hiddenIndex != "" {
				savedFlowCount++
				loop.Set("last_httpflow_hidden_index", hiddenIndex)
				if runtimeID != "" {
					loop.GetEmitter().EmitYakitHTTPFlow(runtimeID, hiddenIndex)
				}
			}
		}

		// Compare request differences
		requestDiff := compareRequests(originalRequest, requestRaw)
		responseSummary := summarizeResponse(responseRaw)
		loop.Set("last_request", requestRaw)
		loop.Set("last_request_summary", requestStreamSummary)
		loop.Set("last_response", responseRaw)
		loop.Set("last_response_summary", responseStreamSummary)

		if representativeRequest == "" {
			representativeRequest = requestRaw
			representativeResponse = responseRaw
			representativeHiddenIndex = hiddenIndex
			loop.Set("representative_request", representativeRequest)
			loop.Set("representative_response", representativeResponse)
			loop.Set("representative_httpflow_hidden_index", representativeHiddenIndex)
		}

		diffSummary.WriteString(fmt.Sprintf("\n--- Result %d ---\n", resultCount))
		analysisSummary.WriteString(fmt.Sprintf("\n--- Result %d ---\n", resultCount))
		diffSummary.WriteString(fmt.Sprintf("Payload: %v\n", result.Payloads))
		analysisSummary.WriteString(fmt.Sprintf("Payload: %v\n", result.Payloads))
		diffSummary.WriteString(fmt.Sprintf("Duration: %d ms\n", result.DurationMs))
		analysisSummary.WriteString(fmt.Sprintf("Duration: %d ms\n", result.DurationMs))
		diffSummary.WriteString(fmt.Sprintf("Status: %s\n", getStatusFromResponse(responseRaw)))
		analysisSummary.WriteString(fmt.Sprintf("Status: %s\n", getStatusFromResponse(responseRaw)))
		diffSummary.WriteString(fmt.Sprintf("Request Summary: %s\n", requestStreamSummary))
		analysisSummary.WriteString(fmt.Sprintf("Request Summary: %s\n", requestStreamSummary))
		diffSummary.WriteString(fmt.Sprintf("Response Summary: %s\n", responseStreamSummary))
		analysisSummary.WriteString(fmt.Sprintf("Response Summary: %s\n", responseStreamSummary))
		if hiddenIndex != "" {
			diffSummary.WriteString(fmt.Sprintf("Saved HTTPFlow: %s\n", hiddenIndex))
			analysisSummary.WriteString(fmt.Sprintf("Saved HTTPFlow: %s\n", hiddenIndex))
		}
		diffSummary.WriteString(fmt.Sprintf("Request Changes:\n%s\n", requestDiff))
		analysisSummary.WriteString(fmt.Sprintf("Request Changes:\n%s\n", requestDiff))
		diffSummary.WriteString(fmt.Sprintf("Response Summary:\n%s\n", responseSummary))
		analysisSummary.WriteString(fmt.Sprintf("Response Summary:\n%s\n", responseSummary))
		diffSummary.WriteString("Request Packet:\n")
		diffSummary.WriteString(requestRaw)
		diffSummary.WriteString("\nResponse Packet:\n")
		diffSummary.WriteString(responseRaw)
		diffSummary.WriteRune('\n')
	}

	if resultCount == 0 {
		diffSummary.WriteString("\n(no results returned by the fuzz execution)\n")
		analysisSummary.WriteString("\n(no results returned by the fuzz execution)\n")
	}
	if savedFlowCount > 0 {
		diffSummary.WriteString(fmt.Sprintf("\nSaved %d HTTP flow records to database and linked them to the current AI runtime/task event context.\n", savedFlowCount))
		analysisSummary.WriteString(fmt.Sprintf("\nSaved %d HTTP flow records to database and linked them to the current AI runtime/task event context.\n", savedFlowCount))
	}

	fullDiffResult := diffSummary.String()
	analysisResult := analysisSummary.String()
	loop.Set("diff_result_full", fullDiffResult)
	loop.Set("diff_result_compressed", analysisResult)

	feedbackResult := analysisResult
	compressedResult := ""
	if len(fullDiffResult) > loopHTTPFuzzCompressionThreshold && invoker != nil {
		emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 结果超过 40KB，开始生成压缩报告并检查所有数据包。", actionName))
		compressionTarget := buildFuzzCompressionTarget(loop, actionName)
		compressed, compressErr := invoker.CompressLongTextWithDestination(taskCtx, fullDiffResult, compressionTarget, loopHTTPFuzzCompressionTarget)
		if compressErr != nil {
			emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 压缩报告失败，回退到原始测试结果。", actionName))
		} else {
			compressedResult = compressed
			feedbackResult = buildCompressedFeedbackReport(compressed, representativeRequest, representativeResponse, representativeHiddenIndex)
			loop.Set("diff_result_compressed", compressed)
			emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 压缩报告完成，准备验证是否达到安全测试目标。", actionName))
		}
	}

	verifyResult, verificationText, err := verifyFuzzCompletion(loop, taskCtx, streamTaskID, actionName, feedbackResult, compressedResult, representativeRequest, representativeResponse, representativeHiddenIndex)
	if err != nil {
		return "", nil, err
	}
	if verificationText != "" {
		feedbackResult += "\n\n" + verificationText
		loop.Set("verification_result", verificationText)
	}

	loop.Set("diff_result", feedbackResult)
	persistLoopHTTPFuzzSessionContext(loop, actionName)

	return feedbackResult, verifyResult, nil
}

func buildFuzzCompressionTarget(loop *reactloops.ReActLoop, actionName string) string {
	task := loop.GetCurrentTask()
	userInput := ""
	if task != nil {
		userInput = strings.TrimSpace(task.GetUserInput())
	}
	if userInput == "" {
		userInput = "HTTP 安全模糊测试"
	}
	return fmt.Sprintf("用户正在执行 HTTP 安全模糊测试，当前步骤是 %s。你的核心目标是分析漏洞，而不是复述数据包。请覆盖所有请求/响应对，重点归纳疑似漏洞类型、触发依据、差异模式、可复现代表性数据包、以及下一步验证动作。原始目标：%s", actionName, userInput)
}

func buildCompressedFeedbackReport(compressed, representativeRequest, representativeResponse, representativeHiddenIndex string) string {
	var out strings.Builder
	out.WriteString("=== Compressed Fuzz Report ===\n")
	out.WriteString(compressed)
	if representativeRequest != "" || representativeResponse != "" {
		out.WriteString("\n\n=== Representative Packet For Follow-Up Testing ===\n")
		if representativeHiddenIndex != "" {
			out.WriteString(fmt.Sprintf("HTTPFlow: %s\n", representativeHiddenIndex))
		}
		if representativeRequest != "" {
			out.WriteString("Request:\n")
			out.WriteString(representativeRequest)
			out.WriteRune('\n')
		}
		if representativeResponse != "" {
			out.WriteString("Response:\n")
			out.WriteString(representativeResponse)
		}
	}
	return out.String()
}

func verifyFuzzCompletion(loop *reactloops.ReActLoop, taskCtx context.Context, streamTaskID, actionName, feedbackResult, compressedResult, representativeRequest, representativeResponse, representativeHiddenIndex string) (*aicommon.VerifySatisfactionResult, string, error) {
	invoker := loop.GetInvoker()
	task := loop.GetCurrentTask()
	if invoker == nil || task == nil {
		return nil, "", nil
	}

	payload := feedbackResult
	if compressedResult != "" {
		payload = buildCompressedFeedbackReport(compressedResult, representativeRequest, representativeResponse, representativeHiddenIndex)
	}

	emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 测试结果已准备完成，开始验证是否达到当前安全测试目标。", actionName))
	verifyResult, err := invoker.VerifyUserSatisfaction(taskCtx, task.GetUserInput(), true, payload)
	if err != nil {
		return nil, "", utils.Wrap(err, "verify fuzz completion")
	}

	var verifySummary strings.Builder
	verifySummary.WriteString("=== Verification ===\n")
	verifySummary.WriteString(fmt.Sprintf("Satisfied: %v\n", verifyResult.Satisfied))
	verifySummary.WriteString(fmt.Sprintf("Reasoning: %s\n", verifyResult.Reasoning))
	if next := aicommon.FormatVerifyNextMovementsSummary(verifyResult.NextMovements); next != "" {
		verifySummary.WriteString(fmt.Sprintf("Next Steps: %s\n", next))
	}
	if representativeHiddenIndex != "" {
		verifySummary.WriteString(fmt.Sprintf("Representative HTTPFlow: %s\n", representativeHiddenIndex))
	}

	state := "未完成，需要继续测试。"
	if verifyResult.Satisfied {
		state = "已达到当前安全测试目标。"
	}
	emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 目标验证完成：%s", actionName, state))

	return verifyResult, verifySummary.String(), nil
}

func emitFuzzStage(loop *reactloops.ReActLoop, taskID, msg string) {
	if loop == nil || taskID == "" || loop.GetEmitter() == nil || strings.TrimSpace(msg) == "" {
		return
	}
	_, _ = loop.GetEmitter().EmitDefaultStreamEvent("thought", bytes.NewBufferString(msg), taskID)
}

func emitPacketSummary(loop *reactloops.ReActLoop, taskID, actionName string, index int, stage, summary string) {
	if loop == nil || taskID == "" || loop.GetEmitter() == nil || strings.TrimSpace(summary) == "" {
		return
	}
	message := fmt.Sprintf("[%s #%d][%s] %s", actionName, index, stage, summary)
	_, _ = loop.GetEmitter().EmitDefaultStreamEvent("http_flow", bytes.NewBufferString(message), taskID)
}

func buildHTTPRequestStreamSummary(requestRaw string, isHTTPS bool) (string, string) {
	requestURL := extractRequestURL(requestRaw, isHTTPS)
	_, body := lowhttp.SplitHTTPPacketFast([]byte(requestRaw))
	return requestURL, fmt.Sprintf("URL: %s BODY: [(%d) bytes]", requestURL, len(body))
}

func buildHTTPResponseStreamSummary(responseRaw, requestURL string) string {
	status := strings.TrimSpace(getStatusFromResponse(responseRaw))
	_, body := lowhttp.SplitHTTPPacketFast([]byte(responseRaw))
	if status == "" {
		return fmt.Sprintf("URL: %s BODY: [(%d) bytes]", requestURL, len(body))
	}
	return fmt.Sprintf("URL: %s STATUS: %s BODY: [(%d) bytes]", requestURL, status, len(body))
}

func extractRequestURL(requestRaw string, isHTTPS bool) string {
	urlObj, err := lowhttp.ExtractURLFromHTTPRequestRaw([]byte(requestRaw), isHTTPS)
	if err == nil && urlObj != nil && urlObj.String() != "" {
		return urlObj.String()
	}

	scheme := "http"
	if isHTTPS {
		scheme = "https"
	}
	if fallback := strings.TrimSpace(lowhttp.GetUrlFromHTTPRequest(scheme, []byte(requestRaw))); fallback != "" {
		return fallback
	}
	return "(unknown url)"
}

func buildFuzzTimelineSummary(summary string) string {
	if len(summary) <= loopHTTPFuzzTimelinePreviewSize {
		return summary
	}
	return utils.ShrinkTextBlock(summary, loopHTTPFuzzTimelinePreviewSize)
}

func applyFuzzVerificationOutcome(loop *reactloops.ReActLoop, operator *reactloops.LoopActionHandlerOperator, diffResult string, verifyResult *aicommon.VerifySatisfactionResult) {
	if verifyResult == nil {
		operator.Feedback(diffResult)
		return
	}

	loop.PushSatisfactionRecordWithCompletedTaskIndex(
		verifyResult.Satisfied,
		verifyResult.Reasoning,
		verifyResult.CompletedTaskIndex,
		verifyResult.NextMovements,
	)

	if verifyResult.Satisfied {
		operator.Exit()
		return
	}

	operator.Feedback(diffResult)
	operator.Continue()
}

// compareRequests compares two HTTP requests and returns the differences
func compareRequests(original, modified string) string {
	originalLines := strings.Split(strings.TrimSpace(original), "\n")
	modifiedLines := strings.Split(strings.TrimSpace(modified), "\n")

	var diff strings.Builder
	maxLines := max(len(originalLines), len(modifiedLines))

	for i := 0; i < maxLines; i++ {
		origLine := ""
		modLine := ""
		if i < len(originalLines) {
			origLine = strings.TrimSpace(originalLines[i])
		}
		if i < len(modifiedLines) {
			modLine = strings.TrimSpace(modifiedLines[i])
		}

		if origLine != modLine {
			if origLine != "" {
				diff.WriteString(fmt.Sprintf("  - %s\n", origLine))
			}
			if modLine != "" {
				diff.WriteString(fmt.Sprintf("  + %s\n", modLine))
			}
		}
	}

	if diff.Len() == 0 {
		return "  (no changes)"
	}
	return diff.String()
}

// summarizeResponse creates a summary of the HTTP response
func summarizeResponse(response string) string {
	if response == "" {
		return "  (empty response)"
	}

	statusLine, body := lowhttp.SplitHTTPPacketFast([]byte(response))

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("  Status: %s\n", statusLine))

	// Get content length
	contentLength := len(body)
	summary.WriteString(fmt.Sprintf("  Content-Length: %d bytes\n", contentLength))

	// Show first part of body if not too long
	if contentLength > 0 {
		bodyPreview := string(body)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		bodyPreview = strings.ReplaceAll(bodyPreview, "\n", " ")
		summary.WriteString(fmt.Sprintf("  Body Preview: %s\n", bodyPreview))
	}

	return summary.String()
}

// getStatusFromResponse extracts status code from response
func getStatusFromResponse(response string) string {
	statusLine, _ := lowhttp.SplitHTTPPacketFast([]byte(response))
	return statusLine
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
