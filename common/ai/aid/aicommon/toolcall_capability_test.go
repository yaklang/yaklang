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
		require.Equal(t, "auto", spec.ToolChoice)
		resp := NewAIResponse(c)
		switch reply {
		case "tool":
			spec.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "probe-id", Type: "function", Function: aispec.FuncReturn{Name: toolCallProbeName, Arguments: `{"message":"yak_`}}})
			spec.ToolCallCallback([]*aispec.ToolCall{{Index: 0, Function: aispec.FuncReturn{Arguments: `tool_call_probe_ok"}`}}})
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
	cfg := NewConfig(context.Background(), WithAICallback(toolCallProbeTestCallback(t, &count, "tool")))
	for range 2 {
		state, err := cfg.CheckToolCallCapability(context.Background(), false)
		require.NoError(t, err)
		require.Equal(t, ToolCallSupported, state)
	}
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
	cfg := NewConfig(context.Background(), WithAICallback(adapter))
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
		{name: "explicit tool rejection", reply: "unsupported", want: ToolCallUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var count atomic.Int32
			cfg := NewConfig(context.Background(), WithAICallback(toolCallProbeTestCallback(t, &count, tc.reply)))
			state, err := cfg.CheckToolCallCapability(context.Background(), false)
			require.Equal(t, tc.want, state)
			require.Error(t, err)
			state, _ = cfg.CheckToolCallCapability(context.Background(), false)
			require.Equal(t, tc.want, state)
			require.EqualValues(t, 1, count.Load())
		})
	}
}

func TestCheckToolCallCapabilityFirstUseSingleFlight(t *testing.T) {
	var count atomic.Int32
	callback := toolCallProbeTestCallback(t, &count, "tool")
	cfg := NewConfig(context.Background(), WithAICallback(func(c AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		time.Sleep(25 * time.Millisecond)
		return callback(c, req)
	}))
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
