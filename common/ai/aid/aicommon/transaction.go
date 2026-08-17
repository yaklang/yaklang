package aicommon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

func is429Response(ctx context.Context, rsp *AIResponse) bool {
	if rsp == nil {
		return false
	}
	rsp.WaitForHTTPHeaders(ctx)
	return rsp.GetHTTPStatusCode() == 429
}

func normalizeTransactionPostHandlerError(rsp *AIResponse, err error) error {
	if err == nil || rsp == nil {
		return err
	}
	lower := strings.ToLower(err.Error())
	if rsp.GetTotalOutputBytes() != 0 {
		return err
	}
	if strings.Contains(lower, "action type is empty") || strings.Contains(lower, "action @action not found or invalid") {
		provider := strings.TrimSpace(rsp.GetProviderName())
		model := strings.TrimSpace(rsp.GetModelName())
		if provider != "" || model != "" {
			return utils.Wrapf(err, "ai model returned empty response (provider=%s model=%s)", provider, model)
		}
		return utils.Wrapf(err, "ai model returned empty response")
	}
	return err
}

func CallAITransaction(
	c AICallerConfigIf,
	prompt string,
	callAi func(*AIRequest) (*AIResponse, error),
	postHandler func(rsp *AIResponse) error,
	requestOpts ...AIRequestOption,
) error {
	return callAITransaction(c, prompt, callAi, postHandler, nil, requestOpts...)
}

func CallAITransactionWithFailureExtra(
	c AICallerConfigIf,
	prompt string,
	callAi func(*AIRequest) (*AIResponse, error),
	postHandler func(rsp *AIResponse) error,
	failureExtra map[string]any,
	requestOpts ...AIRequestOption,
) error {
	return callAITransaction(c, prompt, callAi, postHandler, failureExtra, requestOpts...)
}

func callAITransaction(
	c AICallerConfigIf,
	prompt string,
	callAi func(*AIRequest) (*AIResponse, error),
	postHandler func(rsp *AIResponse) error,
	failureExtra map[string]any,
	requestOpts ...AIRequestOption,
) error {
	// Honour a caller-reserved sequence. Batch parameter generation allocates
	// sequences in model order before launching goroutines.
	seedRequest := NewAIRequest("", requestOpts...)
	transactionCtx, stopTransactionCtx := combineAIRequestAndConfigContext(c.GetContext(), seedRequest.GetContext())
	defer stopTransactionCtx()
	seq := seedRequest.GetSeqId()
	var saver CheckpointCommitHandler
	var transactionStateMu sync.Mutex
	getSeq := func() int64 {
		transactionStateMu.Lock()
		defer transactionStateMu.Unlock()
		return seq
	}
	var trcRetry int64 = 3
	if c != nil {
		trcRetry = c.GetAITransactionAutoRetryCount()
	}
	if trcRetry <= 0 {
		trcRetry = 3
	}
	var postHandlerErr error
	var lastErr error
	var lastCallAiErr error // 保留 API 调用错误，防止被 postHandler 错误覆盖
	var lastRsp *AIResponse
	var lastReq *AIRequest

	// attemptHistory records every attempt (including 429 rate-limit retries) so
	// that the final failure message can expose the full retry history to the
	// caller instead of only the last attempt.
	var attemptHistory []transactionAttemptRecord

	emitter := c.GetEmitter()
	bindEmitter := func(rsp *AIResponse) *Emitter {
		if rsp == nil {
			return emitter
		}
		return rsp.BindEmitter(emitter)
	}

	requestOpts = append(requestOpts,
		WithAIRequest_OnAcquireSeq(func(i int64) {
			transactionStateMu.Lock()
			defer transactionStateMu.Unlock()
			seq = i
		}),
		WithAIRequest_SaveCheckpointCallback(func(handler CheckpointCommitHandler) {
			transactionStateMu.Lock()
			defer transactionStateMu.Unlock()
			saver = handler
		}),
	)

	for i := int64(0); i < trcRetry; {
		if err := transactionCtx.Err(); err != nil {
			return err
		}
		if c.IsCtxDone() {
			return c.GetContext().Err()
		}
		finalPrompt := c.RetryPromptBuilder(prompt, postHandlerErr)

		utils.Debug(func() {
			if i == 0 {
				emitter.EmitInfo("[DEBUG] AI Transaction Prompt (seq=%d, attempt=%d):\n%s", getSeq(), i+1, finalPrompt)
			} else {
				emitter.EmitInfo("[DEBUG] AI Transaction Prompt Retry (seq=%d, attempt=%d):\n%s", getSeq(), i+1, utils.ShrinkString(finalPrompt, 512))
			}
		})

		aiReq := NewAIRequest(
			finalPrompt,
			append(requestOpts, WithAIRequest_SeqId(getSeq()))...,
		)
		lastReq = aiReq
		rsp, err := callAi(aiReq)
		// A request-scoped cancellation is a terminal control signal, not an AI
		// failure. Check it before classifying callAi errors so a task finishing
		// while the provider is in flight cannot emit a misleading model-error
		// event or enter the retry loop.
		if ctxErr := transactionCtx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			lastErr = err
			lastCallAiErr = err
			lastRsp = rsp
			rspEmitter := bindEmitter(rsp)

			if is429Response(transactionCtx, rsp) {
				if is429Retryable(transactionCtx, rsp) {
					// 频率限流/过载类：可重试，不消耗重试次数。
					rspEmitter.EmitWarning("429 rate limit detected in transaction layer (seq=%d), will retry without counting attempt", getSeq())
					attemptHistory = append(attemptHistory, buildAttemptRecord(i+1, finalPrompt, err, rsp))
					retryAfter := parseRetryAfterSeconds(rsp, 5)
					waitSec := capRetryAfterSeconds(jitterSeconds(retryAfter, 3), 1, 120)
					select {
					case <-transactionCtx.Done():
						return transactionCtx.Err()
					case <-time.After(time.Duration(waitSec) * time.Second):
						continue
					}
				}
				// 额度耗尽类 429：不可重试，消耗重试次数让上层暴露错误。
				rspEmitter.EmitWarning("429 quota exceeded in transaction layer (seq=%d), not retryable", getSeq())
			}

			i++
			attemptHistory = append(attemptHistory, buildAttemptRecord(i, finalPrompt, err, rsp))
			rspEmitter.EmitError("call ai api error (attempt %d/%d): %v", i, trcRetry, err)
			select {
			case <-transactionCtx.Done():
				return transactionCtx.Err()
			case <-time.After(100 * time.Millisecond):
				if len(attemptHistory) > 0 {
					if failedOutput := attemptHistory[len(attemptHistory)-1].FailedAIOutput(); failedOutput != "" {
						rspEmitter.EmitWarning("call ai transaction retry (attempt %d/%d, previous AI output: %s)", i, trcRetry, failedOutput)
					} else {
						rspEmitter.EmitWarning("call ai transaction retry (attempt %d/%d)", i, trcRetry)
					}
				} else {
					rspEmitter.EmitWarning("call ai transaction retry (attempt %d/%d)", i, trcRetry)
				}
				continue
			}
		}
		if err := transactionCtx.Err(); err != nil {
			return err
		}
		if c.IsCtxDone() {
			return c.GetContext().Err()
		}
		lastRsp = rsp
		// The plain AI output / reason text is captured automatically by
		// AIResponse as the stream is consumed (GetOutputStreamReader /
		// reason-stream goroutine). We can read it directly via
		// GetPlainOutput / GetPlainReason after postHandler finishes — no
		// custom capture hooks needed.
		if !rsp.WaitForCallbackDone(transactionCtx) {
			return transactionCtx.Err()
		}
		if ctxErr := transactionCtx.Err(); ctxErr != nil {
			return ctxErr
		}
		postHandlerErr = postHandler(rsp)
		// The post-handler may consume a stream with the same request context.
		// If that context was cancelled while parsing, do not reinterpret the
		// cancellation as malformed model output and retry it.
		if ctxErr := transactionCtx.Err(); ctxErr != nil {
			return ctxErr
		}
		// 归一化空响应错误，再与 rsp 的回调错误（由 AIChatToAICallbackType 等设置）合并
		postHandlerErr = normalizeTransactionPostHandlerError(rsp, postHandlerErr)
		postHandlerErr = mergePostHandlerAndCallbackError(postHandlerErr, rsp.GetError())
		if postHandlerErr != nil {
			lastErr = postHandlerErr
			i++
			rec := buildAttemptRecord(i, finalPrompt, nil, rsp)
			rec.PostHandlerErr = postHandlerErr
			attemptHistory = append(attemptHistory, rec)
			rspEmitter := bindEmitter(rsp)
			rspEmitter.EmitError("ai transaction postHandler error (attempt %d/%d): %v", i, trcRetry, postHandlerErr)
			select {
			case <-transactionCtx.Done():
				return transactionCtx.Err()
			case <-time.After(100 * time.Millisecond):
				if len(attemptHistory) > 0 {
					if failedOutput := attemptHistory[len(attemptHistory)-1].FailedAIOutput(); failedOutput != "" {
						rspEmitter.EmitWarning("call ai transaction retry (attempt %d/%d, previous AI output: %s)", i, trcRetry, failedOutput)
					} else {
						rspEmitter.EmitWarning("call ai transaction retry (attempt %d/%d)", i, trcRetry)
					}
				} else {
					rspEmitter.EmitWarning("call ai transaction retry (attempt %d/%d)", i, trcRetry)
				}
				continue
			}
		}
		transactionStateMu.Lock()
		checkpointSaver := saver
		transactionStateMu.Unlock()
		if checkpointSaver != nil {
			cp, err := checkpointSaver()
			if cp == nil {
				emitter.EmitError("cannot save checkpoint")
				return err
			} else {
				//emitter.EmitInfo("checkpoint cached in database: %v:%v", utils.ShrinkString(cp.CoordinatorUuid, 12), cp.Seq)
			}
		}
		return nil
	}

	// 确定最终错误：优先使用 API 调用错误，保留错误链
	finalErr := lastErr
	if lastCallAiErr != nil {
		finalErr = lastCallAiErr
	}

	var modelInfo string
	if lastRsp != nil {
		provider := lastRsp.GetProviderName()
		model := lastRsp.GetModelName()
		if provider != "" || model != "" {
			modelInfo = fmt.Sprintf(" (model: %s:%s)", provider, model)
		}
	}
	finalErrMsg := fmt.Sprintf(
		"[AI Transaction Failed] After %d attempts%s, the AI interaction could not complete.\n"+
			"Last error: %v\n\n"+
			"Suggested actions:\n"+
			"1. Check if the current AI model is working properly\n"+
			"2. Try switching to a different AI model\n"+
			"3. Simplify the task or reduce the prompt complexity\n"+
			"4. Check network connectivity and API rate limits",
		trcRetry, modelInfo, finalErr,
	)
	var tier consts.ModelTier
	if lastReq != nil {
		tier = consts.ModelTier(lastReq.GetModelTier())
	}
	// Attach the full attempt history to the structured failure payload so clients
	// can inspect every retry's error / response. Copy the map to avoid mutating
	// the caller-supplied failureExtra.
	structuredExtra := failureExtra
	if len(attemptHistory) > 0 {
		structuredExtra = make(map[string]any, len(failureExtra)+1)
		for k, v := range failureExtra {
			structuredExtra[k] = v
		}
		attempts := make([]map[string]any, 0, len(attemptHistory))
		for _, r := range attemptHistory {
			attempts = append(attempts, r.ToMap())
		}
		structuredExtra["attempts"] = attempts
	}
	emittedStructuredFailure := EmitAICallFailureIfApplicable(c, tier, lastRsp, finalErr, structuredExtra)
	if !emittedStructuredFailure {
		finalErrMsg += formatAttemptHistory(attemptHistory)
		bindEmitter(lastRsp).EmitDefaultStreamEvent("ai-error", strings.NewReader(finalErrMsg), "")
	}

	// The full attempt history is emitted via EmitAICallFailureIfApplicable
	// (structured event) or the fallback stream event above. The returned
	// error stays concise — callers that need the full retry history should
	// consume the emitted events rather than parsing the error string.
	if finalErr != nil {
		return utils.Wrap(finalErr, fmt.Sprintf("max retry count[%v] reached in transaction", trcRetry))
	}
	return utils.Errorf("max retry count[%v] reached in transaction", trcRetry)
}
