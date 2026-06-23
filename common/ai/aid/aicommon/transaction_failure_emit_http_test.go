package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// httpTxnConfig overrides CallAI so CallAITransaction can hit a real HTTP mock server.
type httpTxnConfig struct {
	*transactionTestConfig
	callAI AICallbackType
}

func (h *httpTxnConfig) CallAI(req *AIRequest) (*AIResponse, error) {
	return h.callAI(h.transactionTestConfig, req)
}

func (h *httpTxnConfig) CallSpeedPriorityAI(req *AIRequest) (*AIResponse, error) {
	return h.CallAI(req)
}

func (h *httpTxnConfig) CallQualityPriorityAI(req *AIRequest) (*AIResponse, error) {
	return h.CallAI(req)
}

func newHTTPFailureEmitHarness(t *testing.T, callAI AICallbackType) (*httpTxnConfig, *[]*schema.AiOutputEvent) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	var mu sync.Mutex
	var captured []*schema.AiOutputEvent
	base := newTransactionTestConfig(ctx)
	base.retryMax = 1
	base.emitter = NewEmitter("failure-emit-http-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		mu.Lock()
		captured = append(captured, e)
		mu.Unlock()
		return e, nil
	})

	cfg := &httpTxnConfig{
		transactionTestConfig: base,
		callAI:                callAI,
	}
	return cfg, &captured
}

func newChatBaseHTTPCallAI(host string, port int, useStream bool, extraPoc func() []poc.PocConfigOption, extraChat ...aispec.ChatBaseOption) AICallbackType {
	return func(aicf AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		resp := NewAIResponse(aicf)
		go func() {
			defer resp.Close()
			isStream := false
			var handlerErr error
			chatOpts := []aispec.ChatBaseOption{
				aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
					opts := []poc.PocConfigOption{
						poc.WithHost(host),
						poc.WithPort(port),
						poc.WithForceHTTPS(false),
						poc.WithSave(false),
					}
					if extraPoc != nil {
						opts = append(opts, extraPoc()...)
					}
					return opts, nil
				}),
				aispec.WithChatBase_ErrHandler(func(err error) {
					handlerErr = err
				}),
				aispec.WithChatBase_RawHTTPResponseHeaderCallback(func(headerBytes []byte) {
					resp.SetRawHTTPResponseHeader(headerBytes)
				}),
				aispec.WithChatBase_RawHTTPResponseCallback(func(headerBytes, bodyPreview []byte) {
					resp.SetRawHTTPResponseData(headerBytes, bodyPreview)
				}),
			}
			if useStream {
				chatOpts = append(chatOpts, aispec.WithChatBase_StreamHandler(func(reader io.Reader) {
					isStream = true
					resp.EmitOutputStream(reader)
				}))
			}
			chatOpts = append(chatOpts, extraChat...)
			_, err := aispec.ChatBase(
				"http://example.com/v1/chat/completions",
				"test-model",
				req.GetPrompt(),
				chatOpts...,
			)
			finalErr := err
			if finalErr == nil && handlerErr != nil {
				finalErr = handlerErr
			}
			if finalErr != nil {
				resp.SetError(finalErr)
			}
			if !isStream {
				resp.EmitOutputStream(strings.NewReader(""))
			}
		}()
		return resp, nil
	}
}

func failingActionPostHandler(*AIResponse) error {
	return utils.Errorf("action type is empty (available_actions=[finish])")
}

func collectAICallFailureEvents(events []*schema.AiOutputEvent) []*schema.AiOutputEvent {
	out := make([]*schema.AiOutputEvent, 0)
	for _, e := range events {
		if e == nil || e.NodeId != NodeAICallFailure || e.Type != schema.EVENT_TYPE_API_REQUEST_FAILED {
			continue
		}
		out = append(out, e)
	}
	return out
}

func hasAIErrorStreamEvent(events []*schema.AiOutputEvent) bool {
	for _, e := range events {
		if e == nil || e.Type != schema.EVENT_TYPE_STREAM {
			continue
		}
		if e.NodeId == "ai-error" {
			return true
		}
	}
	return false
}

func parseFailurePayload(t *testing.T, e *schema.AiOutputEvent) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(e.Content), &payload))
	return payload
}

func failureText(payload map[string]any) string {
	var b strings.Builder
	if cause := strings.TrimSpace(fmt.Sprint(payload["cause"])); cause != "" && cause != "<nil>" {
		b.WriteString(cause)
	}
	if raw := strings.TrimSpace(fmt.Sprint(payload["raw_http_response_dump"])); raw != "" && raw != "<nil>" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(raw)
	}
	return b.String()
}

func assertSingleStructuredFailure(t *testing.T, events []*schema.AiOutputEvent) map[string]any {
	t.Helper()
	failures := collectAICallFailureEvents(events)
	require.Len(t, failures, 1, "expected exactly one structured ai_call_failure event")
	assert.False(t, hasAIErrorStreamEvent(events), "structured failure path should not emit ai-error stream")
	payload := parseFailurePayload(t, failures[0])
	assert.Equal(t, ErrorCodeAICallFailed, payload["error_code"])
	return payload
}

func TestCallAITransaction_FailureEmit_ConnectionRefused(t *testing.T) {
	invalidPort := utils.GetRandomAvailableTCPPort() + 10000
	callAI := newChatBaseHTTPCallAI("127.0.0.1", invalidPort, true, func() []poc.PocConfigOption {
		return []poc.PocConfigOption{
			poc.WithConnectTimeout(1),
			poc.WithTimeout(2),
		}
	})

	cfg, captured := newHTTPFailureEmitHarness(t, callAI)
	err := CallAITransaction(cfg, "connect-refused-prompt", cfg.CallAI, failingActionPostHandler)
	require.Error(t, err)

	payload := assertSingleStructuredFailure(t, *captured)
	text := strings.ToLower(failureText(payload))
	assert.True(t,
		strings.Contains(text, "connection refused") ||
			strings.Contains(text, "connect") ||
			strings.Contains(text, "dial") ||
			strings.Contains(text, "refused") ||
			strings.Contains(text, "refuse retry"),
		"expected connection failure keywords, got: %s", text,
	)
}

func TestCallAITransaction_FailureEmit_StreamTimeoutWithPartialBody(t *testing.T) {
	const (
		partialMarker  = "PARTIAL_STREAM_CHUNK"
		timeoutSeconds = 2.0
		streamHangAfter = 30 * time.Second
	)

	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		chunk := fmt.Sprintf("42\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\r\n\r\n", partialMarker)
		responseHeader := "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n"
		_, _ = conn.Write([]byte(responseHeader + chunk))
		time.Sleep(streamHangAfter)
	})

	callAI := newChatBaseHTTPCallAI(host, port, true, func() []poc.PocConfigOption {
		return []poc.PocConfigOption{
			poc.WithTimeout(timeoutSeconds),
			poc.WithConnectTimeout(1),
			poc.WithConnPool(false),
			poc.WithNoBodyBuffer(true),
		}
	})

	cfg, captured := newHTTPFailureEmitHarness(t, callAI)
	start := time.Now()
	err := CallAITransaction(cfg, "stream-timeout-prompt", cfg.CallAI, failingActionPostHandler)
	elapsed := time.Since(start)
	require.Error(t, err)

	payload := assertSingleStructuredFailure(t, *captured)
	text := strings.ToLower(failureText(payload))
	assert.Contains(t, text, strings.ToLower(partialMarker), "expected partial response body in failure payload")
	assert.True(t,
		strings.Contains(text, "timeout") ||
			strings.Contains(text, "deadline exceeded") ||
			strings.Contains(text, "i/o timeout") ||
			strings.Contains(text, "unexpected eof") ||
			strings.Contains(text, "ai stream read failed"),
		"expected timeout/stream read error keywords, got: %s", text,
	)
	errMsg := strings.ToLower(err.Error())
	assert.True(t,
		strings.Contains(errMsg, "timeout") ||
			strings.Contains(errMsg, "deadline exceeded") ||
			strings.Contains(errMsg, "i/o timeout") ||
			strings.Contains(errMsg, "unexpected eof") ||
			strings.Contains(errMsg, "ai stream read failed"),
		"expected timeout/stream read error in transaction error, got: %s", errMsg,
	)
	assert.GreaterOrEqual(t, elapsed, time.Duration(timeoutSeconds*float64(time.Second))-200*time.Millisecond,
		"expected elapsed time to reflect configured read timeout, elapsed=%v", elapsed)
}

func TestCallAITransaction_FailureEmit_APIRequestTooLarge(t *testing.T) {
	const apiErrMsg = "request too large: token limit exceeded"
	mock413 := fmt.Sprintf(`HTTP/1.1 413 Request Entity Too Large
Content-Type: application/json

{"error":{"message":%q,"type":"invalid_request_error","code":"request_too_large"}}
`, apiErrMsg)

	host, port := utils.DebugMockHTTP([]byte(mock413))
	callAI := newChatBaseHTTPCallAI(host, port, true, func() []poc.PocConfigOption {
		return []poc.PocConfigOption{
			poc.WithTimeout(3),
			poc.WithConnectTimeout(1),
		}
	})

	cfg, captured := newHTTPFailureEmitHarness(t, callAI)
	err := CallAITransaction(cfg, "request-too-large-prompt", cfg.CallAI, failingActionPostHandler)
	require.Error(t, err)

	payload := assertSingleStructuredFailure(t, *captured)
	text := failureText(payload)
	assert.Contains(t, text, apiErrMsg, "expected upstream API error message in failure payload")
	assert.Contains(t, text, "413", "expected HTTP status in raw response dump")
}
