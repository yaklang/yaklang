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
	var seq int64
	var saver CheckpointCommitHandler
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
			seq = i
		}),
		WithAIRequest_SaveCheckpointCallback(func(handler CheckpointCommitHandler) {
			saver = handler
		}),
	)

	for i := int64(0); i < trcRetry; {
		if c.IsCtxDone() {
			return utils.Errorf("context is done, cannot continue transaction")
		}
		finalPrompt := c.RetryPromptBuilder(prompt, postHandlerErr)

		utils.Debug(func() {
			if i == 0 {
				emitter.EmitInfo("[DEBUG] AI Transaction Prompt (seq=%d, attempt=%d):\n%s", seq, i+1, finalPrompt)
			} else {
				emitter.EmitInfo("[DEBUG] AI Transaction Prompt Retry (seq=%d, attempt=%d):\n%s", seq, i+1, utils.ShrinkString(finalPrompt, 512))
			}
		})

		aiReq := NewAIRequest(
			finalPrompt,
			append(requestOpts, WithAIRequest_SeqId(seq))...,
		)
		lastReq = aiReq
		rsp, err := callAi(aiReq)
		if err != nil {
			lastErr = err
			lastCallAiErr = err
			lastRsp = rsp
			rspEmitter := bindEmitter(rsp)

			if is429Response(c.GetContext(), rsp) {
				rspEmitter.EmitWarning("429 rate limit detected in transaction layer (seq=%d), will retry without counting attempt", seq)
				attemptHistory = append(attemptHistory, buildAttemptRecord(i+1, finalPrompt, err, rsp))
				select {
				case <-c.GetContext().Done():
					return err
				case <-time.After(5 * time.Second):
					continue
				}
			}

			i++
			attemptHistory = append(attemptHistory, buildAttemptRecord(i, finalPrompt, err, rsp))
			rspEmitter.EmitError("call ai api error (attempt %d/%d): %v", i, trcRetry, err)
			select {
			case <-c.GetContext().Done():
				return err
			case <-time.After(100 * time.Millisecond):
				rspEmitter.EmitWarning("call ai transaction retry (attempt %d/%d)", i, trcRetry)
				continue
			}
		}
		if c.IsCtxDone() {
			return utils.Errorf("context is done, cannot continue transaction")
		}
		lastRsp = rsp
		// Inject capture hooks so that, after postHandler consumes the
		// stream, we can record the *plain* AI output / reason text for
		// this attempt. The hooks fire asynchronously when the stream
		// finishes. We synchronise on the output-capture completion below
		// (best-effort, bounded by the caller context) before reading the
		// captured text so that the retry record contains the actual AI
		// output even when the stream finishes slightly after postHandler
		// returns.
		var capturedOutput, capturedReason string
		var capturedMu sync.Mutex
		outputDone := make(chan struct{})
		var outputDoneOnce sync.Once
		rsp.SetOutputCapture(func(text string) {
			capturedMu.Lock()
			capturedOutput = text
			capturedMu.Unlock()
			outputDoneOnce.Do(func() { close(outputDone) })
		})
		rsp.SetReasonCapture(func(text string) {
			capturedMu.Lock()
			capturedReason = text
			capturedMu.Unlock()
			outputDoneOnce.Do(func() { close(outputDone) })
		})
		if !rsp.WaitForCallbackDone(c.GetContext()) {
			return c.GetContext().Err()
		}
		postHandlerErr = postHandler(rsp)
		// 检查 rsp 的 error（由 AIChatToAICallbackType 等设置），合并错误
		postHandlerErr = mergePostHandlerAndCallbackError(postHandlerErr, rsp.GetError())
		if postHandlerErr != nil {
			lastErr = postHandlerErr
			i++
			// Best-effort wait for the output/reason capture to complete so
			// the record carries the plain AI text. Falls back to raw HTTP
			// dump when nothing was captured (e.g. stream not consumed).
			select {
			case <-outputDone:
			case <-c.GetContext().Done():
			case <-time.After(2 * time.Second):
			}
			rec := buildAttemptRecord(i, finalPrompt, nil, rsp)
			rec.PostHandlerErr = postHandlerErr
			capturedMu.Lock()
			rec.PlainOutput = capturedOutput
			rec.PlainReason = capturedReason
			capturedMu.Unlock()
			attemptHistory = append(attemptHistory, rec)
			rspEmitter := bindEmitter(rsp)
			rspEmitter.EmitError("ai transaction postHandler error (attempt %d/%d): %v", i, trcRetry, postHandlerErr)
			select {
			case <-c.GetContext().Done():
				return postHandlerErr
			case <-time.After(100 * time.Millisecond):
				rspEmitter.EmitWarning("call ai transaction retry (attempt %d/%d)", i, trcRetry)
				continue
			}
		}
		if saver != nil {
			cp, err := saver()
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
		if lastRsp != nil {
			rawDump := lastRsp.GetRawHTTPResponseDump()
			if rawDump != "" {
				finalErrMsg += "\n\n--- Last Raw HTTP Response ---\n" + utils.ShrinkString(rawDump, 4096)
			}
		}
		finalErrMsg += formatAttemptHistory(attemptHistory)
		bindEmitter(lastRsp).EmitDefaultStreamEvent("ai-error", strings.NewReader(finalErrMsg), "")
	}

	// Build the final returned error. Append the full attempt history so the
	// caller can inspect every retry's error / AI response directly from the
	// returned error, regardless of whether a structured failure event was
	// emitted.
	historyStr := formatAttemptHistory(attemptHistory)
	wrapMsg := fmt.Sprintf("max retry count[%v] reached in transaction", trcRetry)
	if historyStr != "" {
		wrapMsg += historyStr
	}
	if finalErr != nil {
		return utils.Wrap(finalErr, wrapMsg)
	}
	return utils.Errorf("%s", wrapMsg)
}
