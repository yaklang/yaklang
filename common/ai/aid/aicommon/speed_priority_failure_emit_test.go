package aicommon

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEmitAICallFailureIfApplicable_EmitsWithModelInfo(t *testing.T) {
	var mu sync.Mutex
	var events []*schema.AiOutputEvent

	cfg := NewConfig(
		t.Context(),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, _ *AIRequest) (*AIResponse, error) {
			return nil, fmt.Errorf("speed down")
		}),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		}),
	)

	rsp := NewAIResponse(cfg)
	rsp.SetModelInfo("openai", "gpt-4o")
	emitErr := fmt.Errorf("connection refused")
	EmitAICallFailureIfApplicable(cfg, consts.TierIntelligent, rsp, emitErr, map[string]any{
		"liteforge_action": "test_action",
	})

	mu.Lock()
	list := append([]*schema.AiOutputEvent(nil), events...)
	mu.Unlock()

	var found bool
	for _, e := range list {
		if e == nil || e.NodeId != NodeAICallFailure {
			continue
		}
		if !e.IsSystem || !e.IsJson || e.Type != schema.EVENT_TYPE_API_REQUEST_FAILED {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(e.Content), &m))
		if m["error_code"] != ErrorCodeAICallFailed {
			continue
		}
		require.Equal(t, "intelligent", m["model_tier"])
		require.Equal(t, "openai", m["provider_name"])
		require.Equal(t, "gpt-4o", m["model_name"])
		require.Contains(t, fmt.Sprint(m["cause"]), "connection refused")
		require.Equal(t, "test_action", m["liteforge_action"])
		found = true
		break
	}
	require.True(t, found)
}

func TestEmitAICallFailureIfApplicable_EmitsWithoutModelInfo(t *testing.T) {
	var mu sync.Mutex
	var events []*schema.AiOutputEvent

	cfg := NewConfig(
		t.Context(),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, _ *AIRequest) (*AIResponse, error) {
			return nil, fmt.Errorf("speed down")
		}),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		}),
	)

	emitErr := fmt.Errorf("network timeout")
	EmitAICallFailureIfApplicable(cfg, consts.TierLightweight, nil, emitErr, map[string]any{
		"react_loop_name": "loop_test",
	})

	mu.Lock()
	list := append([]*schema.AiOutputEvent(nil), events...)
	mu.Unlock()

	var found bool
	for _, e := range list {
		if e == nil || e.NodeId != NodeAICallFailure {
			continue
		}
		if !e.IsSystem || !e.IsJson || e.Type != schema.EVENT_TYPE_API_REQUEST_FAILED {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(e.Content), &m))
		if m["error_code"] != ErrorCodeAICallFailed {
			continue
		}
		require.Equal(t, "lightweight", m["model_tier"])
		require.Equal(t, "", m["provider_name"])
		require.Equal(t, "", m["model_name"])
		require.Contains(t, fmt.Sprint(m["cause"]), "network timeout")
		require.Equal(t, "loop_test", m["react_loop_name"])
		found = true
		break
	}
	require.True(t, found)
}

func TestEmitAICallFailureIfApplicable_NoEmitOnNilError(t *testing.T) {
	var mu sync.Mutex
	var n int
	cfg := NewConfig(
		t.Context(),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, _ *AIRequest) (*AIResponse, error) {
			return nil, nil
		}),
		WithEventHandler(func(_ *schema.AiOutputEvent) {
			mu.Lock()
			n++
			mu.Unlock()
		}),
	)

	EmitAICallFailureIfApplicable(cfg, consts.TierIntelligent, nil, nil, nil)
	mu.Lock()
	got := n
	mu.Unlock()
	require.Equal(t, 0, got)
}


