package aicommon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

const transactionPromptOverflowBody = `{"error":{"code":"context_length_exceeded","message":"This model's maximum context length is 8192 tokens. However, your messages resulted in 12000 tokens."}}`

func promptOverflowTestResponse(status int, body string) *AIResponse {
	rsp := NewUnboundAIResponse()
	rsp.SetModelInfo("test-provider", "test-model")
	rsp.SetRawHTTPResponseData([]byte(fmt.Sprintf("HTTP/1.1 %d Status\r\n\r\n", status)), []byte(body))
	rsp.Close()
	return rsp
}

func TestClassifyPromptTooLong(t *testing.T) {
	original := errors.New("provider request rejected")
	rsp := promptOverflowTestResponse(400, transactionPromptOverflowBody)
	err := ClassifyPromptTooLong(rsp, original)
	require.ErrorIs(t, err, ErrPromptTooLong)
	require.ErrorIs(t, err, aispec.ErrPromptOverflow)
	require.ErrorIs(t, err, original)
	ev, ok := PromptOverflowEvidenceFrom(err)
	require.True(t, ok)
	require.Equal(t, "test-provider", ev.Provider)
	require.Equal(t, "test-model", ev.Model)
	require.EqualValues(t, 8192, ev.MaxContextTokens)
	require.EqualValues(t, 12000, ev.InputTokens)

	// Evidence consumers must not mutate the shared error.
	ev.Model = "changed"
	again, ok := PromptOverflowEvidenceFrom(err)
	require.True(t, ok)
	require.Equal(t, "test-model", again.Model)
	require.Nil(t, ClassifyPromptTooLong(nil, nil))
	require.Nil(t, ClassifyPromptTooLong(promptOverflowTestResponse(429, transactionPromptOverflowBody), original))
	require.Nil(t, ClassifyPromptTooLong(promptOverflowTestResponse(200, `{"choices":[{"message":{"content":"prompt is too long"}}]}`), nil))
	require.False(t, IsPromptTooLong(original))
	_, ok = PromptOverflowEvidenceFrom(original)
	require.False(t, ok)

	callbackRsp := NewAIResponse(nil)
	callbackCause := errors.New("prompt is too long")
	callbackRsp.SetError(callbackCause)
	callbackRsp.Close()
	combined := ClassifyPromptTooLong(callbackRsp, original)
	require.ErrorIs(t, combined, ErrPromptTooLong)
	require.ErrorIs(t, combined, callbackCause)
	require.ErrorIs(t, combined, original)
}

func TestClassifyPromptTooLongDoesNotMutateSharedError(t *testing.T) {
	overflow := &aispec.PromptOverflowError{
		PromptOverflowEvidence: aispec.PromptOverflowEvidence{Message: "prompt is too long"},
	}
	rsp := promptOverflowTestResponse(200, "")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := ClassifyPromptTooLong(rsp, overflow)
			ev, ok := PromptOverflowEvidenceFrom(err)
			if !ok || ev.Provider != "test-provider" {
				t.Error("classification lost response identity")
			}
		}()
	}
	wg.Wait()
	require.Empty(t, overflow.Provider)
	require.Empty(t, overflow.Model)
	require.Zero(t, overflow.HTTPStatus)
}

func TestTransactionPromptOverflowIsTerminal(t *testing.T) {
	for _, status := range []int{0, 200, 400, 413, 422} {
		for _, directError := range []bool{false, true} {
			t.Run(fmt.Sprintf("status=%d/direct=%t", status, directError), func(t *testing.T) {
				cfg := newTransactionTestConfig(context.Background())
				cfg.retryMax = 5
				calls, parsed := 0, 0
				var original error
				err := CallAITransaction(cfg, "large prompt", func(*AIRequest) (*AIResponse, error) {
					calls++
					rsp := promptOverflowTestResponse(status, transactionPromptOverflowBody)
					if directError {
						original = errors.New("provider rejected request")
						return rsp, original
					}
					return rsp, nil
				}, func(*AIResponse) error { parsed++; return nil })
				require.ErrorIs(t, err, ErrPromptTooLong)
				require.Equal(t, 1, calls)
				require.Zero(t, parsed, "overflow must not be passed to action parsing")
				require.NotContains(t, err.Error(), "max retry count")
				if original != nil {
					require.ErrorIs(t, err, original)
				}
				ev, ok := PromptOverflowEvidenceFrom(err)
				require.True(t, ok)
				require.Equal(t, "test-provider", ev.Provider)
			})
		}
	}
}

func TestTransactionPromptOverflowWaitsForAsyncCallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := newTransactionTestConfig(ctx)
	calls := 0
	original := errors.New("prompt is too long")
	err := CallAITransaction(cfg, "large prompt", func(*AIRequest) (*AIResponse, error) {
		calls++
		rsp := NewAIResponse(nil)
		rsp.SetRawHTTPResponseHeader([]byte("HTTP/1.1 200 OK\r\n\r\n"))
		go func() {
			rsp.SetModelInfo("async-provider", "async-model")
			rsp.SetError(original)
			rsp.Close()
		}()
		return rsp, nil
	}, func(*AIResponse) error { t.Error("must not parse failed provider output"); return nil })
	require.ErrorIs(t, err, ErrPromptTooLong)
	require.ErrorIs(t, err, original)
	require.Equal(t, 1, calls)
	ev, ok := PromptOverflowEvidenceFrom(err)
	require.True(t, ok)
	require.Equal(t, "async-model", ev.Model)
}

func TestTransactionPromptOverflowSupersedesEarlierTransientFailure(t *testing.T) {
	cfg := newTransactionTestConfig(context.Background())
	calls := 0
	err := CallAITransaction(cfg, "large prompt", func(*AIRequest) (*AIResponse, error) {
		calls++
		if calls == 1 {
			return promptOverflowTestResponse(502, `{"error":{"message":"bad gateway"}}`), errors.New("bad gateway")
		}
		return promptOverflowTestResponse(400, transactionPromptOverflowBody), nil
	}, func(*AIResponse) error { t.Error("must not parse provider errors"); return nil })
	require.ErrorIs(t, err, ErrPromptTooLong)
	require.Equal(t, 2, calls)
}

func TestMergePromptOverflowErrorPreservesCause(t *testing.T) {
	original := errors.New("provider rejected context")
	overflow := &aispec.PromptOverflowError{
		PromptOverflowEvidence: aispec.PromptOverflowEvidence{Message: "prompt is too long"},
		Cause:                  original,
	}
	merged := mergePostHandlerAndCallbackError(errors.New("missing action"), utils.Wrap(overflow, "callback"))
	require.ErrorIs(t, merged, ErrPromptTooLong)
	require.ErrorIs(t, merged, original)
	require.ErrorContains(t, merged, "missing action")

	parseErr, callbackErr := errors.New("parse"), errors.New("callback")
	merged = mergePostHandlerAndCallbackError(parseErr, callbackErr)
	require.ErrorIs(t, merged, parseErr)
	require.ErrorIs(t, merged, callbackErr)
	require.Equal(t, "post handler: parse; ai callback: callback", merged.Error())
}

func TestRequestWrapperPromptOverflowIsTerminal(t *testing.T) {
	for _, detached := range []bool{false, true} {
		for _, hasResponse := range []bool{false, true} {
			t.Run(fmt.Sprintf("detached=%t/response=%t", detached, hasResponse), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				cfg := NewTestConfig(ctx)
				cfg.AiAutoRetry = 5
				calls := 0
				original := errors.New("prompt is too long")
				callback := cfg.wrapper(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
					calls++
					if hasResponse {
						return promptOverflowTestResponse(400, transactionPromptOverflowBody), nil
					}
					return nil, original
				}, consts.TierIntelligent)
				req := NewAIRequest("large prompt")
				req.SetDetachCheckpoint(detached)
				_, err := callback(cfg, req)
				require.ErrorIs(t, err, ErrPromptTooLong)
				require.Equal(t, 1, calls)
				if !hasResponse {
					require.ErrorIs(t, err, original)
				}
			})
		}
	}
}

func TestPromptOverflowHTTPToTransaction(t *testing.T) {
	for _, protocol := range []aispec.ChatBaseInterfaceType{aispec.ChatBaseInterfaceTypeChatCompletions, aispec.ChatBaseInterfaceTypeResponses} {
		for _, status := range []int{200, 400} {
			for _, detached := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/status=%d/detached=%t", protocol, status, detached), func(t *testing.T) {
					var requests atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests.Add(1)
						_, _ = io.Copy(io.Discard, r.Body)
						body, contentType := transactionPromptOverflowBody, "application/json"
						if status == 200 {
							contentType = "text/event-stream"
							body = "data: " + body + "\n\n"
						}
						w.Header().Set("Content-Type", contentType)
						w.WriteHeader(status)
						_, _ = io.WriteString(w, body)
					}))
					defer server.Close()
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					cfg := NewTestConfig(ctx)
					cfg.AiAutoRetry, cfg.AiTransactionAutoRetry = 5, 5
					adapter := AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
						ac := aispec.NewDefaultAIConfig(opts...)
						ac.ModelInfoCallback("local-provider", "local-model")
						return aispec.ChatBase(server.URL, "local-model", prompt,
							aispec.WithChatBase_InterfaceType(protocol),
							aispec.WithChatBase_StreamHandler(ac.StreamHandler),
							aispec.WithChatBase_ReasonStreamHandler(ac.ReasonStreamHandler),
							aispec.WithChatBase_RawHTTPResponseHeaderCallback(ac.RawHTTPResponseHeaderCallback),
							aispec.WithChatBase_RawHTTPResponseCallback(ac.RawHTTPResponseCallback),
							aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }))
					})
					callback := cfg.wrapper(adapter, consts.TierIntelligent)
					err := CallAITransaction(cfg, "large prompt", func(req *AIRequest) (*AIResponse, error) {
						req.SetDetachCheckpoint(detached)
						return callback(cfg, req)
					}, func(*AIResponse) error { t.Error("must not parse overflow response"); return nil })
					require.ErrorIs(t, err, ErrPromptTooLong)
					var overflow *aispec.PromptOverflowError
					require.ErrorAs(t, err, &overflow)
					require.Equal(t, "local-provider", overflow.Provider)
					require.Equal(t, "local-model", overflow.Model)
					require.EqualValues(t, 1, requests.Load(), "neither retry layer may replay an overflowing prompt")
				})
			}
		}
	}
}
