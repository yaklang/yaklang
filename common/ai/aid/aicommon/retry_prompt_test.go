package aicommon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
)

type retryPromptTestConfig struct{ *transactionTestConfig }

func (c *retryPromptTestConfig) RetryPromptBuilder(prompt string, err error) string {
	return (&Config{}).RetryPromptBuilder(prompt, err)
}

func retryDetail(t *testing.T, prompt string) map[string]any {
	t.Helper()
	_, tail, ok := strings.Cut(prompt, "最近一次失败（错误、响应）：\n")
	require.True(t, ok)
	data, _, ok := strings.Cut(tail, "\n# 纠正结束")
	require.True(t, ok)
	var detail map[string]any
	require.NoError(t, json.Unmarshal([]byte(data), &detail))
	return detail
}

func TestTransactionNativeRetryCorrectsActualDSMLArguments(t *testing.T) {
	const prompt = "STATIC PREFIX\nFrozen history\nCurrent task\n"
	bad := []string{
		`{"identifier": "await_risk_settlement_continue", "timeout_seconds">60</｜｜DSML｜｜: "await_risk_settlement_continue", "timeout_seconds": 60}`,
		`{"identifier">await_risk_worker_settle_7</｜DSML｜:`,
	}
	cfg := &retryPromptTestConfig{newTransactionTestConfig(context.Background())}
	cfg.retryMax = 3
	var requests []string
	var current string
	err := CallAITransaction(cfg, prompt, func(req *AIRequest) (*AIResponse, error) {
		requests = append(requests, req.GetPrompt())
		if len(requests) > 1 {
			require.True(t, strings.HasPrefix(req.GetPrompt(), prompt))
			require.Equal(t, 1, strings.Count(req.GetPrompt(), "# 重试纠正"))
			detail := retryDetail(t, req.GetPrompt())
			require.Equal(t, "function_call", detail["protocol"])
			require.Equal(t, "tool_calls", detail["finish_reason"])
			require.NotZero(t, detail["json_error_offset"])
			call := detail["tool_calls"].([]any)[0].(map[string]any)
			require.Equal(t, "wait_messages", call["name"])
			require.Equal(t, bad[len(requests)-2], call["arguments"])
			if len(requests) == 3 {
				require.NotContains(t, req.GetPrompt(), "await_risk_settlement_continue")
			}
		}
		// Call-site options are deliberately installed after the transaction
		// creates its trace, as in tool parameter generation and risk review.
		WithAIRequest_ExtraSpecOpts(aispec.WithToolCallCallback(func([]*aispec.ToolCall) {}))(req)
		wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		current = `{"identifier":"settle","timeout_seconds":60,"extension":true}`
		if len(requests) <= len(bad) {
			current = bad[len(requests)-1]
		}
		wire.ToolCallCallback([]*aispec.ToolCall{{ID: "call", Function: aispec.FuncReturn{Name: "wait_messages", Arguments: current[:5]}}})
		wire.ToolCallCallback([]*aispec.ToolCall{{Function: aispec.FuncReturn{Arguments: current[5:]}}})
		wire.FinishReasonCallback("tool_calls", nil)
		resp := NewUnboundAIResponse()
		resp.Close()
		return resp, nil
	}, func(resp *AIResponse) error {
		var result map[string]any
		return json.Unmarshal([]byte(current), &result)
	})
	require.NoError(t, err)
	require.Len(t, requests, 3)
}

func TestTransactionTextRetryCapturesUnboundContent(t *testing.T) {
	const original = "Read input; keep <|REPORT_nonce|> and output result JSON."
	const bad = `{"@action":"result","value":"bad","extension":"<|PROMPT_SECTION_END_dynamic|>"}`
	cfg := &retryPromptTestConfig{newTransactionTestConfig(context.Background())}
	cfg.retryMax = 2
	var attempt int
	err := CallAITransaction(cfg, original, func(req *AIRequest) (*AIResponse, error) {
		attempt++
		if attempt == 2 {
			require.True(t, strings.HasPrefix(req.GetPrompt(), original))
			detail := retryDetail(t, req.GetPrompt())
			require.Equal(t, bad, detail["content"])
			require.Equal(t, "text_stream", detail["protocol"])
			require.NotContains(t, req.GetPrompt()[len(original):], "<|PROMPT_SECTION_END_dynamic|>")
			require.NotContains(t, req.GetPrompt(), "PRIVATE_THINKING")
		}
		wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		require.Nil(t, wire.ToolCallCallback, "text protocol must not acquire native interception")
		wire.FinishReasonCallback("stop", nil)
		resp := NewUnboundAIResponse()
		resp.EmitReasonStream(strings.NewReader("PRIVATE_THINKING"))
		data := bad
		if attempt == 2 {
			data = `{"@action":"result","value":1,"extension":true}`
		}
		resp.EmitOutputStream(strings.NewReader(data))
		resp.Close()
		return resp, nil
	}, func(resp *AIResponse) error {
		reason, output := resp.GetUnboundStreamReaderEx(nil, nil, nil)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, reason) }()
		data, err := io.ReadAll(output)
		wg.Wait()
		if err != nil {
			return err
		}
		var value struct{ Value int }
		return json.Unmarshal(data, &value)
	})
	require.NoError(t, err)
	require.Equal(t, 2, attempt)
}

func TestTransactionTransportRetryDoesNotReuseValidationCorrection(t *testing.T) {
	cfg := &retryPromptTestConfig{newTransactionTestConfig(context.Background())}
	cfg.retryMax = 3
	var attempt int
	transportErr := errors.New("connection reset")
	err := CallAITransaction(cfg, "original", func(req *AIRequest) (*AIResponse, error) {
		attempt++
		if attempt == 2 {
			require.Contains(t, req.GetPrompt(), "broken JSON")
			return nil, transportErr
		}
		if attempt == 3 {
			require.Equal(t, "original", req.GetPrompt())
		}
		resp := NewUnboundAIResponse()
		resp.Close()
		return resp, nil
	}, func(*AIResponse) error {
		if attempt == 1 {
			return errors.New("broken JSON")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, attempt)
}

func TestTransactionFailureDiagnosticsKeepFinalProviderCalls(t *testing.T) {
	cfg := &retryPromptTestConfig{newTransactionTestConfig(context.Background())}
	cfg.retryMax = 1
	var events []*schema.AiOutputEvent
	cfg.emitter = NewEmitter("retry-diagnostics", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		events = append(events, e)
		return e, nil
	})
	const bad = `{"path":"C:\\tmp", "seconds">60</parameter>`
	var observed, headers, finishes, argumentStreams int
	err := CallAITransaction(cfg, "original", func(req *AIRequest) (*AIResponse, error) {
		WithAIRequest_ExtraSpecOpts(
			aispec.WithRawHTTPResponseHeaderCallback(func([]byte) { headers++ }),
			aispec.WithToolCallCallback(func([]*aispec.ToolCall) { observed++ }),
			aispec.WithFinishReasonCallback(func(string, []byte) { finishes++ }),
			aispec.WithToolCallArgumentsStreamHandler(func(reader io.Reader) { argumentStreams++; _, _ = io.Copy(io.Discard, reader) }),
		)(req)
		wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		wire.RawHTTPResponseHeaderCallback(nil)
		wire.ToolCallCallback([]*aispec.ToolCall{{ID: "old", Function: aispec.FuncReturn{Name: "old", Arguments: "superseded"}}})
		wire.RawHTTPResponseHeaderCallback(nil)
		wire.ToolCallCallback([]*aispec.ToolCall{{ID: "new", Function: aispec.FuncReturn{Name: "wait_messages", Arguments: bad}}})
		wire.ToolCallArgumentsStreamHandler(strings.NewReader(bad))
		wire.FinishReasonCallback("tool_calls", nil)
		resp := NewUnboundAIResponse()
		resp.EmitReasonStream(strings.NewReader("reason diagnostic"))
		resp.EmitOutputStream(strings.NewReader("content diagnostic"))
		resp.Close()
		return resp, nil
	}, func(resp *AIResponse) error {
		_, _ = io.Copy(io.Discard, resp.GetUnboundStreamReader(false))
		return errors.New("rejected arguments")
	})
	require.ErrorContains(t, err, "rejected arguments")
	require.Equal(t, 2, observed)
	require.Equal(t, 2, headers)
	require.Equal(t, 1, finishes)
	require.Equal(t, 1, argumentStreams, "tracing must not duplicate argument streaming")
	failures := collectAICallFailureEvents(events)
	require.Len(t, failures, 1)
	detail := parseFailurePayload(t, failures[0])["attempts"].([]any)[0].(map[string]any)
	require.Equal(t, "tool_calls", detail["finish_reason"])
	require.Equal(t, "content diagnostic", detail["output"])
	require.Equal(t, "reason diagnostic", detail["reason"])
	calls := detail["tool_calls"].([]any)
	require.Len(t, calls, 1)
	require.Equal(t, bad, calls[0].(map[string]any)["arguments"])
	require.Equal(t, "new", calls[0].(map[string]any)["id"])
}

func TestTransactionEmptyAsyncTransportErrorKeepsOriginalPrompt(t *testing.T) {
	cfg := &retryPromptTestConfig{newTransactionTestConfig(context.Background())}
	cfg.retryMax = 2
	var attempt int
	err := CallAITransaction(cfg, "original", func(req *AIRequest) (*AIResponse, error) {
		attempt++
		require.Equal(t, "original", req.GetPrompt())
		resp := NewAIResponse(nil)
		if attempt == 1 {
			resp.SetError(errors.New("connection reset before model output"))
		}
		resp.Close()
		return resp, nil
	}, func(*AIResponse) error {
		if attempt == 1 {
			return errors.New("missing action")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, attempt)
}

func TestRetryCorrectionBoundsResponseAndPreservesErrorChain(t *testing.T) {
	trace := &retryResponseTrace{}
	wire := aispec.NewDefaultAIConfig(aispec.WithToolCallCallback(func([]*aispec.ToolCall) {}), trace.option())
	for i := 0; i < 20; i++ {
		wire.ToolCallCallback([]*aispec.ToolCall{{Index: i, ID: fmt.Sprint(i), Function: aispec.FuncReturn{Name: "action", Arguments: strings.Repeat("中文", 20000)}}})
	}
	rec := transactionAttemptRecord{PlainOutput: strings.Repeat("text", 20000)}
	trace.fillRecord(&rec)
	require.Len(t, rec.ToolCalls, retryToolCallLimit)
	require.True(t, rec.ToolCallsTruncated)
	cause := errors.New("schema validation rejected")
	wrapped := &retryCorrectionError{cause: WithRetryInstruction(cause, "按原参数 schema 补全必填字段。"), attempt: rec}
	require.ErrorIs(t, wrapped, cause)
	require.Equal(t, cause.Error(), wrapped.Error())
	prompt := buildRetryPrompt("PREFIX", wrapped)
	require.Less(t, len(prompt), 24000)
	detail := retryDetail(t, prompt)
	require.Equal(t, "按原参数 schema 补全必填字段。", detail["correction"])
	require.Equal(t, true, detail["content_truncated"])
	require.Equal(t, true, detail["tool_calls_truncated"])
	for _, call := range detail["tool_calls"].([]any) {
		require.Equal(t, true, call.(map[string]any)["arguments_truncated"])
	}
}
