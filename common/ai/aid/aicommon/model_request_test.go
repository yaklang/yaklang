package aicommon

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
)

func TestModelRequestRecoveryDiscardsPartialResponseAndPreservesPrompt(t *testing.T) {
	old := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(&consts.TieredAIConfig{RetryForever: true})
	t.Cleanup(func() { consts.SetTieredAIConfig(old) })
	cfg := NewTestConfig(context.Background())
	cfg.AiAutoRetry = 1
	cfg.aiRetryWaitFunc = func(context.Context, time.Duration) error { return nil }
	calls := 0
	var scopes []httptrace.Scope
	rsp, err := cfg.callModelRequest(context.Background(), func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		calls++
		require.Equal(t, "original prompt", req.GetPrompt())
		ctx := modelNetworkDebugContext(req.GetContext(), cfg, 42)
		scopes = append(scopes, httptrace.GetScope(ctx))
		response := NewAIResponse(nil)
		response.SetRawHTTPResponseHeader([]byte("HTTP/1.1 200 OK\r\n\r\n"))
		if calls < 3 {
			response.EmitReasonStream(strings.NewReader("discard reasoning"))
			response.EmitOutputStream(strings.NewReader("discard content"))
			response.SetError(&aispec.ModelCallFailure{Kind: "protocol", Cause: io.ErrUnexpectedEOF})
		} else {
			response.EmitReasonStream(strings.NewReader("reasoning"))
			response.EmitOutputStream(strings.NewReader("complete"))
		}
		response.Close()
		return response, nil
	}, cfg, NewAIRequest("original prompt"))
	require.NoError(t, err)
	require.Equal(t, 3, calls, "request retry must exceed the finite budget when enabled")
	reason, output := rsp.GetUnboundStreamReaderEx(nil, nil, nil)
	reasonData, err := io.ReadAll(reason)
	require.NoError(t, err)
	outputData, err := io.ReadAll(output)
	require.NoError(t, err)
	require.Equal(t, "reasoning", string(reasonData))
	require.Equal(t, "complete", string(outputData))
	for i, scope := range scopes {
		require.Equal(t, scopes[0].CallID, scope.CallID)
		require.Equal(t, i+1, scope.Attempt)
		require.EqualValues(t, 42, scope.Sequence)
		require.Equal(t, cfg.GetRuntimeId(), scope.RuntimeID)
	}
}

func TestModelRequestCancellationWhileWaitingForResponse(t *testing.T) {
	old := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(&consts.TieredAIConfig{RetryForever: true})
	t.Cleanup(func() { consts.SetTieredAIConfig(old) })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	cfg := NewTestConfig(ctx)
	calls := 0
	_, err := cfg.callModelRequest(ctx, func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
		calls++
		response := NewAIResponse(nil)
		response.SetRawHTTPResponseHeader([]byte("HTTP/1.1 200 OK\r\n\r\n"))
		return response, nil
	}, cfg, NewAIRequest("wait"))
	require.True(t, errors.Is(err, context.DeadlineExceeded), "%v", err)
	require.Equal(t, 1, calls)
}
