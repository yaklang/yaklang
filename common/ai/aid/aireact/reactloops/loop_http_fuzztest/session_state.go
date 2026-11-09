package loop_http_fuzztest

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

const loopHTTPFuzzTimelineContextMarker = "[HTTP_FUZZ_TIMELINE_CONTEXT]"

type loopHTTPFuzzSessionContext struct {
	Version                   int                        `json:"version"`
	OriginalRequest           string                     `json:"original_request"`
	OriginalRequestSummary    string                     `json:"original_request_summary,omitempty"`
	CurrentRequest            string                     `json:"current_request,omitempty"`
	CurrentRequestSummary     string                     `json:"current_request_summary,omitempty"`
	PreviousRequest           string                     `json:"previous_request,omitempty"`
	PreviousRequestSummary    string                     `json:"previous_request_summary,omitempty"`
	RequestChangeSummary      string                     `json:"request_change_summary,omitempty"`
	RequestModificationReason string                     `json:"request_modification_reason,omitempty"`
	RequestReviewDecision     string                     `json:"request_review_decision,omitempty"`
	IsHTTPS                   bool                       `json:"is_https"`
	BootstrapSource           string                     `json:"bootstrap_source,omitempty"`
	RepresentativeRequest     string                     `json:"representative_request,omitempty"`
	RepresentativeResponse    string                     `json:"representative_response,omitempty"`
	RepresentativeHiddenIndex string                     `json:"representative_hidden_index,omitempty"`
	AnalysisSummary           string                     `json:"analysis_summary,omitempty"`
	VerificationResult        string                     `json:"verification_result,omitempty"`
	RecentActions             []loopHTTPFuzzActionRecord `json:"recent_actions,omitempty"`
	TestedPayloadsByAction    map[string][]string        `json:"tested_payloads_by_action,omitempty"`
	LastActionType            string                     `json:"last_action_type,omitempty"`
	DirectlyAnswered          bool                       `json:"directly_answered,omitempty"`
	FinalAnswerDelivered      bool                       `json:"final_answer_delivered,omitempty"`
	UpdatedAt                 time.Time                  `json:"updated_at"`
}

func getLoopPersistentConfig(loop *reactloops.ReActLoop) *aicommon.Config {
	if loop == nil || loop.GetInvoker() == nil {
		return nil
	}
	cfg, _ := loop.GetInvoker().GetConfig().(*aicommon.Config)
	return cfg
}

func captureLoopHTTPFuzzSessionContext(loop *reactloops.ReActLoop, source string) *loopHTTPFuzzSessionContext {
	if loop == nil {
		return nil
	}
	originalRequest := strings.TrimSpace(loop.Get("original_request"))
	if originalRequest == "" {
		return nil
	}

	analysisSummary := strings.TrimSpace(loop.Get("diff_result_analysis"))
	if analysisSummary == "" {
		analysisSummary = strings.TrimSpace(loop.Get("diff_result_compressed"))
	}
	if analysisSummary == "" {
		analysisSummary = strings.TrimSpace(loop.Get("diff_result"))
	}

	ctx := &loopHTTPFuzzSessionContext{
		Version:                   4,
		OriginalRequest:           originalRequest,
		OriginalRequestSummary:    strings.TrimSpace(loop.Get("original_request_summary")),
		CurrentRequest:            strings.TrimSpace(loop.Get("current_request")),
		CurrentRequestSummary:     strings.TrimSpace(loop.Get("current_request_summary")),
		PreviousRequest:           strings.TrimSpace(loop.Get("previous_request")),
		PreviousRequestSummary:    strings.TrimSpace(loop.Get("previous_request_summary")),
		RequestChangeSummary:      strings.TrimSpace(loop.Get("request_change_summary")),
		RequestModificationReason: strings.TrimSpace(loop.Get("request_modification_reason")),
		RequestReviewDecision:     strings.TrimSpace(loop.Get("request_review_decision")),
		IsHTTPS:                   strings.EqualFold(loop.Get("is_https"), "false"),
		BootstrapSource:           strings.TrimSpace(source),
		RepresentativeRequest:     strings.TrimSpace(loop.Get("representative_request")),
		RepresentativeResponse:    strings.TrimSpace(loop.Get("representative_response")),
		RepresentativeHiddenIndex: strings.TrimSpace(loop.Get("representative_httpflow_hidden_index")),
		AnalysisSummary:           analysisSummary,
		VerificationResult:        strings.TrimSpace(loop.Get("verification_result")),
		RecentActions:             getLoopHTTPFuzzRecentActions(loop),
		TestedPayloadsByAction:    getLoopHTTPFuzzTestedPayloads(loop),
		LastActionType:            getLoopHTTPFuzzLastAction(loop),
		DirectlyAnswered:          hasLoopHTTPFuzzDirectlyAnswered(loop),
		FinalAnswerDelivered:      hasLoopHTTPFuzzFinalAnswerDelivered(loop),
		UpdatedAt:                 time.Now(),
	}

	if ctx.OriginalRequestSummary == "" {
		_, summary := buildHTTPRequestStreamSummary(ctx.OriginalRequest, ctx.IsHTTPS)
		ctx.OriginalRequestSummary = summary
	}
	if ctx.CurrentRequest == "" {
		ctx.CurrentRequest = ctx.OriginalRequest
	}
	if ctx.CurrentRequestSummary == "" {
		_, summary := buildHTTPRequestStreamSummary(ctx.CurrentRequest, ctx.IsHTTPS)
		ctx.CurrentRequestSummary = summary
	}
	if ctx.PreviousRequest != "" && ctx.PreviousRequestSummary == "" {
		_, summary := buildHTTPRequestStreamSummary(ctx.PreviousRequest, ctx.IsHTTPS)
		ctx.PreviousRequestSummary = summary
	}
	return ctx
}

func persistLoopHTTPFuzzSessionContext(loop *reactloops.ReActLoop, source string) {
	if loop == nil || loop.GetInvoker() == nil {
		return
	}
	ctx := captureLoopHTTPFuzzSessionContext(loop, source)
	if ctx == nil {
		return
	}

	payloadBytes, err := json.Marshal(ctx)
	if err != nil {
		log.Warnf("http_fuzztest: marshal session context failed: %v", err)
		return
	}
	payload := string(payloadBytes)
	previousCtx, hadPreviousCtx := loadLatestLoopHTTPFuzzSessionContext(loop)
	if !hadPreviousCtx || !sameLoopHTTPFuzzSessionContext(previousCtx, ctx) {
		loop.GetInvoker().AddToTimeline(
			"http_fuzztest_session_context",
			loopHTTPFuzzTimelineContextMarker+" "+payload,
		)
	}
}

func restoreLoopHTTPFuzzSessionContext(loop *reactloops.ReActLoop, runtime aicommon.AIInvokeRuntime) bool {
	ctx, ok := loadLatestLoopHTTPFuzzSessionContext(loop)
	if !ok || ctx == nil || strings.TrimSpace(ctx.OriginalRequest) == "" {
		return false
	}
	return applyLoopHTTPFuzzSessionContext(loop, runtime, ctx)
}

func applyLoopHTTPFuzzSessionContext(loop *reactloops.ReActLoop, runtime aicommon.AIInvokeRuntime, ctx *loopHTTPFuzzSessionContext) bool {
	if ctx == nil || strings.TrimSpace(ctx.OriginalRequest) == "" {
		return false
	}

	restoreRequest := strings.TrimSpace(ctx.CurrentRequest)
	if restoreRequest == "" {
		restoreRequest = strings.TrimSpace(ctx.OriginalRequest)
	}
	originalRaw := []byte(restoreRequest)
	fuzzReq, err := newLoopFuzzRequest(getLoopTaskContext(loop), runtime, originalRaw, ctx.IsHTTPS)
	if err != nil {
		log.Warnf("http_fuzztest: restore session fuzz request failed: %v", err)
		return false
	}

	storeLoopFuzzRequestState(loop, fuzzReq, originalRaw, ctx.IsHTTPS)
	loop.Set("original_request", ctx.OriginalRequest)
	loop.Set("original_request_summary", ctx.OriginalRequestSummary)
	loop.Set("current_request", restoreRequest)
	loop.Set("current_request_summary", ctx.CurrentRequestSummary)
	loop.Set("previous_request", ctx.PreviousRequest)
	loop.Set("previous_request_summary", ctx.PreviousRequestSummary)
	loop.Set("request_change_summary", ctx.RequestChangeSummary)
	loop.Set("request_modification_reason", ctx.RequestModificationReason)
	loop.Set("request_review_decision", ctx.RequestReviewDecision)
	loop.Set("bootstrap_source", ctx.BootstrapSource)
	loop.Set("representative_request", ctx.RepresentativeRequest)
	loop.Set("representative_response", ctx.RepresentativeResponse)
	loop.Set("representative_httpflow_hidden_index", ctx.RepresentativeHiddenIndex)
	// 恢复会话时同样收敛代表性请求/响应的 prompt 安全版本，避免大原始包重新注入 LLM 上下文。
	loop.Set(loopHTTPUploadRepresentativeSafeKey, sanitizeRequestTextForPrompt(ctx.RepresentativeRequest))
	loop.Set(loopHTTPUploadRepresentativeResponseSafeKey, sanitizeResponseTextForPrompt(ctx.RepresentativeResponse))
	loop.Set("diff_result", ctx.AnalysisSummary)
	loop.Set("diff_result_analysis", ctx.AnalysisSummary)
	loop.Set("diff_result_compressed", "")
	loop.Set("verification_result", ctx.VerificationResult)
	setLoopHTTPFuzzRecentActions(loop, ctx.RecentActions)
	setLoopHTTPFuzzTestedPayloads(loop, ctx.TestedPayloadsByAction)
	loop.Set(loopHTTPFuzzLastActionTypeKey, strings.TrimSpace(ctx.LastActionType))
	loop.Set(loopHTTPFuzzDirectlyAnsweredKey, ctx.DirectlyAnswered)
	loop.Set(loopHTTPFuzzFinalAnswerDeliveredKey, ctx.FinalAnswerDelivered)
	if ctx.RepresentativeHiddenIndex != "" {
		loop.Set("last_httpflow_hidden_index", ctx.RepresentativeHiddenIndex)
	}
	loop.Set("restored_session_context", "true")
	syncLoopHTTPUploadContext(loop, originalRaw, ctx.IsHTTPS, false)
	return true
}

func loadLatestLoopHTTPFuzzSessionContext(loop *reactloops.ReActLoop) (*loopHTTPFuzzSessionContext, bool) {
	cfg := getLoopPersistentConfig(loop)
	if cfg == nil {
		return nil, false
	}
	return extractLatestLoopHTTPFuzzSessionContextFromTimeline(cfg.GetTimeline())
}

func extractLatestLoopHTTPFuzzSessionContextFromTimeline(timeline *aicommon.Timeline) (*loopHTTPFuzzSessionContext, bool) {
	if timeline == nil {
		return nil, false
	}
	ids := timeline.GetTimelineItemIDs()
	for i := len(ids) - 1; i >= 0; i-- {
		item, ok := timeline.GetIdToTimelineItem().Get(ids[i])
		if !ok || item == nil {
			continue
		}
		text, ok := item.GetValue().(*aicommon.TextTimelineItem)
		if !ok || text == nil {
			continue
		}
		for _, body := range []string{text.PromptText, text.Text} {
			idx := strings.Index(body, loopHTTPFuzzTimelineContextMarker)
			if idx < 0 {
				continue
			}
			payload := strings.TrimSpace(body[idx+len(loopHTTPFuzzTimelineContextMarker):])
			if ctx, ok := decodeLoopHTTPFuzzSessionContext(payload); ok {
				return ctx, true
			}
		}
	}
	return nil, false
}

func decodeLoopHTTPFuzzSessionContext(payload string) (*loopHTTPFuzzSessionContext, bool) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil, false
	}
	var ctx loopHTTPFuzzSessionContext
	if err := json.Unmarshal([]byte(payload), &ctx); err != nil {
		log.Warnf("http_fuzztest: unmarshal session context failed: %v", err)
		return nil, false
	}
	return &ctx, true
}

func sameLoopHTTPFuzzSessionContext(left, right *loopHTTPFuzzSessionContext) bool {
	if left == nil || right == nil {
		return left == right
	}
	leftCopy := *left
	rightCopy := *right
	leftCopy.UpdatedAt = time.Time{}
	rightCopy.UpdatedAt = time.Time{}
	leftBytes, _ := json.Marshal(leftCopy)
	rightBytes, _ := json.Marshal(rightCopy)
	return string(leftBytes) == string(rightBytes)
}
