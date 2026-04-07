package loop_http_fuzztest

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const loopHTTPFuzzSessionContextMarker = "[HTTP_FUZZ_SESSION_CONTEXT]"

type loopHTTPFuzzSessionContext struct {
	Version                   int       `json:"version"`
	OriginalRequest           string    `json:"original_request"`
	OriginalRequestSummary    string    `json:"original_request_summary,omitempty"`
	IsHTTPS                   bool      `json:"is_https"`
	BootstrapSource           string    `json:"bootstrap_source,omitempty"`
	RepresentativeRequest     string    `json:"representative_request,omitempty"`
	RepresentativeResponse    string    `json:"representative_response,omitempty"`
	RepresentativeHiddenIndex string    `json:"representative_hidden_index,omitempty"`
	AnalysisSummary           string    `json:"analysis_summary,omitempty"`
	VerificationResult        string    `json:"verification_result,omitempty"`
	UpdatedAt                 time.Time `json:"updated_at"`
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

	analysisSummary := strings.TrimSpace(loop.Get("diff_result_compressed"))
	if analysisSummary == "" {
		analysisSummary = strings.TrimSpace(loop.Get("diff_result"))
	}

	ctx := &loopHTTPFuzzSessionContext{
		Version:                   1,
		OriginalRequest:           originalRequest,
		OriginalRequestSummary:    strings.TrimSpace(loop.Get("original_request_summary")),
		IsHTTPS:                   strings.EqualFold(loop.Get("is_https"), "true"),
		BootstrapSource:           strings.TrimSpace(source),
		RepresentativeRequest:     strings.TrimSpace(loop.Get("representative_request")),
		RepresentativeResponse:    strings.TrimSpace(loop.Get("representative_response")),
		RepresentativeHiddenIndex: strings.TrimSpace(loop.Get("representative_httpflow_hidden_index")),
		AnalysisSummary:           analysisSummary,
		VerificationResult:        strings.TrimSpace(loop.Get("verification_result")),
		UpdatedAt:                 time.Now(),
	}

	if ctx.OriginalRequestSummary == "" {
		_, summary := buildHTTPRequestStreamSummary(ctx.OriginalRequest, ctx.IsHTTPS)
		ctx.OriginalRequestSummary = summary
	}
	return ctx
}

func persistLoopHTTPFuzzSessionContext(loop *reactloops.ReActLoop, source string) {
	cfg := getLoopPersistentConfig(loop)
	if cfg == nil || cfg.GetDB() == nil || cfg.PersistentSessionId == "" {
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

	if lastCtx, ok := extractLatestLoopHTTPFuzzSessionContext(cfg.GetUserInputHistory()); ok {
		lastBytes, _ := json.Marshal(lastCtx)
		if string(lastBytes) == payload {
			return
		}
	}

	record := loopHTTPFuzzSessionContextMarker + " " + payload
	quotedHistory, err := cfg.AppendUserInputHistory(record, time.Now())
	if err != nil {
		log.Warnf("http_fuzztest: append session context history failed: %v", err)
		return
	}
	if err := yakit.UpdateAIAgentRuntimeUserInput(cfg.GetDB(), cfg.GetRuntimeId(), quotedHistory); err != nil {
		log.Warnf("http_fuzztest: persist session context history failed: %v", err)
	}

	if loop != nil && loop.GetInvoker() != nil {
		loop.GetInvoker().AddToTimeline(
			"http_fuzztest_session_context",
			fmt.Sprintf("Persisted HTTP fuzz session context (%s): %s", source, utils.ShrinkTextBlock(ctx.OriginalRequestSummary, 200)),
		)
	}
	persistLoopHTTPFuzzTimeline(cfg)
}

func persistLoopHTTPFuzzTimeline(cfg *aicommon.Config) {
	if cfg == nil || cfg.GetDB() == nil || cfg.PersistentSessionId == "" || cfg.Timeline == nil {
		return
	}
	timelineRaw, err := aicommon.MarshalTimeline(cfg.Timeline)
	if err != nil {
		log.Warnf("http_fuzztest: marshal timeline failed: %v", err)
		return
	}
	if err := yakit.UpdateAIAgentRuntimeTimeline(cfg.GetDB(), cfg.GetRuntimeId(), strconv.Quote(timelineRaw)); err != nil {
		log.Warnf("http_fuzztest: persist timeline failed: %v", err)
	}
}

func restoreLoopHTTPFuzzSessionContext(loop *reactloops.ReActLoop, runtime aicommon.AIInvokeRuntime) bool {
	ctx, ok := loadLatestLoopHTTPFuzzSessionContext(loop)
	if !ok || ctx == nil || strings.TrimSpace(ctx.OriginalRequest) == "" {
		return false
	}

	originalRaw := []byte(ctx.OriginalRequest)
	fuzzReq, err := newLoopFuzzRequest(getLoopTaskContext(loop), runtime, originalRaw, ctx.IsHTTPS)
	if err != nil {
		log.Warnf("http_fuzztest: restore session fuzz request failed: %v", err)
		return false
	}

	storeLoopFuzzRequestState(loop, fuzzReq, originalRaw, ctx.IsHTTPS)
	loop.Set("original_request_summary", ctx.OriginalRequestSummary)
	loop.Set("bootstrap_source", ctx.BootstrapSource)
	loop.Set("representative_request", ctx.RepresentativeRequest)
	loop.Set("representative_response", ctx.RepresentativeResponse)
	loop.Set("representative_httpflow_hidden_index", ctx.RepresentativeHiddenIndex)
	loop.Set("diff_result", ctx.AnalysisSummary)
	loop.Set("diff_result_compressed", ctx.AnalysisSummary)
	loop.Set("verification_result", ctx.VerificationResult)
	if ctx.RepresentativeHiddenIndex != "" {
		loop.Set("last_httpflow_hidden_index", ctx.RepresentativeHiddenIndex)
	}
	loop.Set("restored_session_context", "true")
	return true
}

func loadLatestLoopHTTPFuzzSessionContext(loop *reactloops.ReActLoop) (*loopHTTPFuzzSessionContext, bool) {
	cfg := getLoopPersistentConfig(loop)
	if cfg == nil {
		return nil, false
	}
	if ctx, ok := extractLatestLoopHTTPFuzzSessionContext(cfg.GetUserInputHistory()); ok {
		return ctx, true
	}
	if cfg.GetDB() == nil || cfg.PersistentSessionId == "" {
		return nil, false
	}
	runtime, err := yakit.GetLatestAIAgentRuntimeByPersistentSession(cfg.GetDB(), cfg.PersistentSessionId)
	if err != nil || runtime == nil {
		return nil, false
	}
	return extractLatestLoopHTTPFuzzSessionContext(runtime.GetUserInputHistory())
}

func extractLatestLoopHTTPFuzzSessionContext(history []schema.AIAgentUserInputRecord) (*loopHTTPFuzzSessionContext, bool) {
	for i := len(history) - 1; i >= 0; i-- {
		input := strings.TrimSpace(history[i].UserInput)
		if !strings.HasPrefix(input, loopHTTPFuzzSessionContextMarker) {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(input, loopHTTPFuzzSessionContextMarker))
		if payload == "" {
			continue
		}
		var ctx loopHTTPFuzzSessionContext
		if err := json.Unmarshal([]byte(payload), &ctx); err != nil {
			log.Warnf("http_fuzztest: unmarshal session context failed: %v", err)
			continue
		}
		return &ctx, true
	}
	return nil, false
}
