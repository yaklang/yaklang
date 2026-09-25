package aicommon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func toolCallProbeTestCallback(t *testing.T, count *atomic.Int32, reply string) AICallbackType {
	t.Helper()
	return func(c AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		count.Add(1)
		require.Equal(t, "toolcall-capability-probe", req.GetCallerLabel())
		require.True(t, req.IsDetachedCheckpoint())
		spec := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		require.Len(t, spec.Tools, 1)
		require.Equal(t, toolCallProbeName, spec.Tools[0].Function.Name)
		require.Empty(t, spec.ToolChoice, "provider default avoids rejecting an optional tool_choice parameter")
		resp := NewAIResponse(c)
		switch reply {
		case "tool":
			spec.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "probe-id", Type: "function", Function: aispec.FuncReturn{Name: toolCallProbeName, Arguments: `{"message":"yak_`}}})
			spec.ToolCallCallback([]*aispec.ToolCall{{Index: 0, Function: aispec.FuncReturn{Arguments: `tool_call_probe_ok"}`}}})
		case "tool-reused-index":
			for _, id := range []string{"first-id", "second-id"} {
				spec.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: id, Type: "function", Function: aispec.FuncReturn{Name: toolCallProbeName, Arguments: `{"message":"yak_tool_call_probe_ok"}`}}})
			}
		case "text":
			resp.EmitOutputStream(strings.NewReader("ordinary answer"))
		case "unsupported":
			resp.SetRawHTTPResponseData([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"), []byte(`{"error":"tools unsupported"}`))
			resp.SetError(errors.New("tools unsupported"))
		}
		resp.Close()
		return resp, nil
	}
}

func TestCheckToolCallCapabilityEchoAndCache(t *testing.T) {
	var count atomic.Int32
	cfg := NewConfig(context.Background(), WithAICallback(toolCallProbeTestCallback(t, &count, "tool")), WithCheckToolCall(true))
	for range 2 {
		state, err := cfg.CheckToolCallCapability(context.Background(), false)
		require.NoError(t, err)
		require.Equal(t, ToolCallSupported, state)
	}
	require.EqualValues(t, 1, count.Load())
}

func TestCheckToolCallCapabilityReusedIndex(t *testing.T) {
	var count atomic.Int32
	cfg := NewConfig(context.Background(), WithAICallback(toolCallProbeTestCallback(t, &count, "tool-reused-index")), WithCheckToolCall(true))
	state, err := cfg.CheckToolCallCapability(context.Background(), false)
	require.NoError(t, err)
	require.Equal(t, ToolCallSupported, state)
	require.EqualValues(t, 1, count.Load())
}

func TestCheckToolCallCapabilityThroughAIChatAdapter(t *testing.T) {
	var calls atomic.Int32
	adapter := AIChatToAICallbackType(func(prompt string, options ...aispec.AIConfigOption) (string, error) {
		calls.Add(1)
		require.Contains(t, prompt, toolCallProbeName)
		config := aispec.NewDefaultAIConfig(options...)
		require.Len(t, config.Tools, 1)
		config.ToolCallCallback([]*aispec.ToolCall{{
			ID: "adapter-probe", Type: "function",
			Function: aispec.FuncReturn{Name: toolCallProbeName, Arguments: `{"message":"yak_tool_call_probe_ok"}`},
		}})
		return "", nil
	})
	cfg := NewConfig(context.Background(), WithAICallback(adapter), WithCheckToolCall(true))
	state, err := cfg.CheckToolCallCapability(context.Background(), false)
	require.NoError(t, err)
	require.Equal(t, ToolCallSupported, state)
	require.EqualValues(t, 1, calls.Load())
}

func TestCheckToolCallCapabilityConservativeFallback(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		want        ToolCallCapability
	}{
		{name: "plain text is inconclusive", reply: "text", want: ToolCallUnknown},
		{name: "tool-related 400 is inconclusive", reply: "unsupported", want: ToolCallUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var count atomic.Int32
			cfg := NewConfig(context.Background(), WithAICallback(toolCallProbeTestCallback(t, &count, tc.reply)), WithCheckToolCall(true))
			state, err := cfg.CheckToolCallCapability(context.Background(), false)
			require.Equal(t, tc.want, state)
			require.Error(t, err)
			state, _ = cfg.CheckToolCallCapability(context.Background(), false)
			require.Equal(t, tc.want, state)
			require.EqualValues(t, 1, count.Load())
		})
	}
}

func TestCheckToolCallCapabilityUnknownExpiresAndRecovers(t *testing.T) {
	var count atomic.Int32
	textCallback := toolCallProbeTestCallback(t, &count, "text")
	toolCallback := toolCallProbeTestCallback(t, &count, "tool")
	cfg := NewConfig(context.Background(),
		WithToolCallProbeCacheTTL(20*time.Millisecond),
		WithAICallback(func(c AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			if count.Load() == 0 {
				return textCallback(c, req)
			}
			return toolCallback(c, req)
		}),
		WithCheckToolCall(true),
	)
	state, err := cfg.CheckToolCallCapability(context.Background(), false)
	require.Error(t, err)
	require.Equal(t, ToolCallUnknown, state)
	state, _ = cfg.CheckToolCallCapability(context.Background(), false)
	require.Equal(t, ToolCallUnknown, state)
	require.EqualValues(t, 1, count.Load())

	time.Sleep(30 * time.Millisecond)
	state, err = cfg.CheckToolCallCapability(context.Background(), false)
	require.NoError(t, err)
	require.Equal(t, ToolCallSupported, state)
	require.EqualValues(t, 2, count.Load())
}

func TestCheckToolCallCapabilityFirstUseSingleFlight(t *testing.T) {
	var count atomic.Int32
	callback := toolCallProbeTestCallback(t, &count, "tool")
	cfg := NewConfig(context.Background(), WithAICallback(func(c AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		time.Sleep(25 * time.Millisecond)
		return callback(c, req)
	}), WithCheckToolCall(true))
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state, err := cfg.CheckToolCallCapability(context.Background(), false)
			require.NoError(t, err)
			require.Equal(t, ToolCallSupported, state)
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, count.Load())
}

func TestCheckToolCallCapabilityExplicitDisable(t *testing.T) {
	var count atomic.Int32
	callback := toolCallProbeTestCallback(t, &count, "tool")
	require.False(t, NewConfig(context.Background(), WithAICallback(callback)).CheckToolCall,
		"custom callbacks keep their scripted first response unless probing is explicitly requested")
	require.True(t, NewConfig(context.Background(), WithCheckToolCall(true), WithAICallback(callback)).CheckToolCall)
	cfg := NewConfig(context.Background(), WithAICallback(callback), WithEnableFunctionCallMode(false))
	state, err := cfg.CheckToolCallCapability(context.Background(), false)
	require.NoError(t, err)
	require.Equal(t, ToolCallUnsupported, state)
	require.Zero(t, count.Load())

	cfg = NewConfig(context.Background(), WithAICallback(callback), WithCheckToolCall(false))
	state, err = cfg.CheckToolCallCapability(context.Background(), false)
	require.NoError(t, err)
	require.Equal(t, ToolCallSupported, state)
	require.Zero(t, count.Load())
}
