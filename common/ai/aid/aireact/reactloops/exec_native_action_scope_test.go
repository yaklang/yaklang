package reactloops

import (
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestNativeActionRetryUsesAdvertisedActions(t *testing.T) {
	var attempts int
	loop := newCallAILoopTransactionTestLoop(t, true, func(req *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		attempts++
		name := "read_file" // removed when planning enters its handoff phase
		if attempts > 1 {
			require.Contains(t, req.GetPrompt(), "available_actions=[finish_exploration]")
			name = "finish_exploration"
		}
		cfg.ToolCallCallback([]*aispec.ToolCall{{ID: "call_plan", Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: `{}`}}})
		cfg.FinishReasonCallback("tool_calls", nil)
		resp := aicommon.NewAIResponse(nil)
		resp.Close()
		return resp, nil
	})
	loop.config.(*fcTestConfig).SetConfig("AiTransactionAutoRetry", 2)
	loop.actions.Set("finish_exploration", &LoopAction{ActionType: "finish_exploration"})
	loop.actions.Set("hidden_action", &LoopAction{ActionType: "hidden_action"})
	loop.initActionMustUse = []string{"finish_exploration"}
	_, prompt, err := loop.prepareLoopActionSchemas(nil)
	require.NoError(t, err)
	require.True(t, loop.initActionApplied, "recomputing filters now would widen the advertised set")
	require.NotContains(t, prompt, "hidden_action")
	calls, stop, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, prompt, "n", nil,
		func(io.Reader, io.Reader) {}, func(string, string, io.Reader, io.Reader) {})
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.Equal(t, LoopStopToolCalls, stop)
	require.Len(t, calls, 1)
	require.Equal(t, "finish_exploration", calls[0].Action.Name())
}

func TestNativeActionCannotSelectHiddenRegisteredHandler(t *testing.T) {
	var verified bool
	loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		cfg.ToolCallCallback([]*aispec.ToolCall{{ID: "call_hidden", Type: "function", Function: aispec.FuncReturn{Name: "hidden_action", Arguments: `{}`}}})
		cfg.FinishReasonCallback("tool_calls", nil)
		resp := aicommon.NewAIResponse(nil)
		resp.Close()
		return resp, nil
	})
	loop.actions.Set("finish_exploration", &LoopAction{ActionType: "finish_exploration"})
	loop.actions.Set("hidden_action", &LoopAction{ActionType: "hidden_action", ActionVerifier: func(*ReActLoop, *aicommon.Action) error { verified = true; return nil }})
	loop.actionFilters = []func(*LoopAction) bool{func(a *LoopAction) bool { return a.ActionType == "finish_exploration" }}
	_, prompt, err := loop.prepareLoopActionSchemas(nil)
	require.NoError(t, err)
	calls, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, prompt, "n", nil,
		func(io.Reader, io.Reader) {}, func(string, string, io.Reader, io.Reader) {})
	require.ErrorContains(t, err, "available_actions=[finish_exploration]")
	require.Empty(t, calls)
	require.False(t, verified)

	// An explicitly empty tool set must not fall back to the global registry.
	loop.actionFilters = []func(*LoopAction) bool{func(*LoopAction) bool { return false }}
	_, prompt, err = loop.prepareLoopActionSchemas(nil)
	require.NoError(t, err)
	_, _, _, err = loop.callAILoopTransaction(&sync.WaitGroup{}, prompt, "n", nil,
		func(io.Reader, io.Reader) {}, func(string, string, io.Reader, io.Reader) {})
	require.ErrorContains(t, err, "available_actions=[]")
	require.False(t, verified)

	// A new prompt takes a new snapshot; exclusions are not permanent.
	loop.actionFilters = []func(*LoopAction) bool{func(a *LoopAction) bool { return a.ActionType == "hidden_action" }}
	_, prompt, err = loop.prepareLoopActionSchemas(nil)
	require.NoError(t, err)
	calls, _, _, err = loop.callAILoopTransaction(&sync.WaitGroup{}, prompt, "n", nil,
		func(io.Reader, io.Reader) {}, func(string, string, io.Reader, io.Reader) {})
	require.NoError(t, err)
	require.Len(t, calls, 1)
	require.True(t, verified)
}
