package aicommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
)

// callModelRequest adapts AICallback to the common request executor. Nested
// aispec clients reuse this attempt's admission and budget. Checkpoints and
// Agent output validation stay outside the transport recovery operation.
func (c *Config) callModelRequest(ctx context.Context, callback AICallbackType, caller AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
	policy := aispec.ModelRetryOptions{MaxAttempts: int(c.AiAutoRetry), Wait: c.aiRetryWaitFunc,
		Notify: func(message string) { c.EmitInfo("%s", message) }}
	return aispec.RunModelCall(ctx, policy, func(attemptCtx context.Context) (*AIResponse, error) {
		attemptRequest := *request
		attemptRequest.ctx = attemptCtx
		response, err := callback(caller, &attemptRequest)
		if err != nil {
			if overflow := ClassifyPromptTooLong(response, err); overflow != nil {
				return response, &aispec.ModelCallFailure{Kind: "configuration", Cause: overflow}
			}
			if response != nil {
				return response, aispec.ClassifyModelFailure(err, response.GetHTTPStatusCode(), response.GetHTTPHeader("Retry-After"))
			}
			return response, err
		}
		if response == nil {
			return nil, fmt.Errorf("model callback returned no response")
		}
		if overflow := ClassifyPromptTooLong(response, nil); overflow != nil {
			return response, &aispec.ModelCallFailure{Kind: "configuration", Cause: overflow}
		}
		// Before exposing a successful stream, headers can already reject HTTP
		// failures. A failed response is drained without exposing its body.
		response.WaitForHTTPHeaders(attemptCtx)
		status := response.GetHTTPStatusCode()
		if !consts.IsAIRetryForeverEnabled() && status < 400 {
			return response, nil
		}
		stopDrain := context.AfterFunc(attemptCtx, response.Close)
		defer stopDrain()
		reasonReader, outputReader := response.GetUnboundStreamReaderEx(nil, nil, nil)
		var reason, output []byte
		var reasonErr, outputErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); reason, reasonErr = io.ReadAll(reasonReader) }()
		go func() { defer wg.Done(); output, outputErr = io.ReadAll(outputReader) }()
		drained := make(chan struct{})
		go func() { wg.Wait(); close(drained) }()
		select {
		case <-drained:
		case <-attemptCtx.Done():
			if closer, ok := reasonReader.(io.Closer); ok {
				closer.Close()
			}
			if closer, ok := outputReader.(io.Closer); ok {
				closer.Close()
			}
			return response, attemptCtx.Err()
		}
		if !response.WaitForCallbackDone(attemptCtx) {
			return response, attemptCtx.Err()
		}
		err = response.GetError()
		if err == nil {
			err = reasonErr
		}
		if err == nil {
			err = outputErr
		}
		status = response.GetHTTPStatusCode()
		if err == nil && status >= 400 {
			err = aiHTTPResponseError(response)
		}
		if overflow := ClassifyPromptTooLong(response, err); overflow != nil {
			return response, &aispec.ModelCallFailure{Kind: "configuration", Cause: overflow}
		}
		if err != nil {
			return response, aispec.ClassifyModelFailure(err, status, response.GetHTTPHeader("Retry-After"))
		}
		result := NewAIResponse(caller)
		result.SetModelInfo(response.GetProviderName(), response.GetModelName())
		result.SetThinkingLevel(response.GetThinkingLevel())
		result.SetUsageInfo(response.GetUsageInfo())
		response.rawHTTPResponseHeaderMu.Lock()
		result.SetRawHTTPResponseData(bytes.Clone(response.rawHTTPResponseHeader), bytes.Clone(response.rawHTTPResponseBody))
		response.rawHTTPResponseHeaderMu.Unlock()
		result.EmitReasonStream(bytes.NewReader(reason))
		result.EmitOutputStream(bytes.NewReader(output))
		result.Close()
		return result, nil
	})
}
