package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	_ "github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
)

// TestInvokeSpeedPriorityLiteForge_EmitsStructuredErrorOnFailure verifies that when
// SpeedPriorityAICallback is configured and LiteForge execution fails, a fixed system
// JSON event is emitted so clients can prompt users to fix lightweight model config.
func TestInvokeSpeedPriorityLiteForge_EmitsStructuredErrorOnFailure(t *testing.T) {
	var mu sync.Mutex
	var captured []*schema.AiOutputEvent

	speedErr := fmt.Errorf("simulated speed-tier model failure")

	r, err := NewTestReAct(
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithSpeedPriorityAICallback(func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := aicommon.NewAIResponse(nil)
			rsp.SetModelInfo("mock-provider", "mock-model")
			rsp.SetHeaderReady()
			return rsp, speedErr
		}),
		aicommon.WithQualityPriorityAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			_ = i
			_ = req
			return nil, fmt.Errorf("quality should not be used in this test")
		}),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			mu.Lock()
			captured = append(captured, e)
			mu.Unlock()
		}),
	)
	require.NoError(t, err)

	const actionName = "test_speed_liteforge_action"
	_, invokeErr := r.InvokeSpeedPriorityLiteForge(
		context.Background(),
		actionName,
		"minimal prompt for liteforge",
		[]aitool.ToolOption{
			aitool.WithStringParam("out", aitool.WithParam_Required(true), aitool.WithParam_Description("output")),
		},
	)
	require.Error(t, invokeErr)

	mu.Lock()
	events := append([]*schema.AiOutputEvent(nil), captured...)
	mu.Unlock()

	var found bool
	var failureEventCount int
	for _, e := range events {
		if e == nil || e.NodeId != aicommon.NodeAICallFailure {
			continue
		}
		failureEventCount++
		if e.Type != schema.EVENT_TYPE_API_REQUEST_FAILED || !e.IsJson || !e.IsSystem {
			continue
		}
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(e.Content), &payload))
		if payload["error_code"] != aicommon.ErrorCodeAICallFailed {
			continue
		}
		require.Equal(t, "lightweight", payload["model_tier"])
		require.Equal(t, "mock-provider", payload["provider_name"])
		require.Equal(t, "mock-model", payload["model_name"])
		require.Equal(t, actionName, payload["liteforge_action"])
		cause := fmt.Sprint(payload["cause"])
		require.NotEmpty(t, cause)
		require.True(t,
			strings.Contains(cause, speedErr.Error()) || strings.Contains(cause, "max retry count"),
			"cause should include root error or transaction retry summary: %q", cause,
		)
		found = true
		break
	}
	require.Equal(t, 1, failureEventCount, "expected only one ai_call_failure event")
	require.True(t, found, "expected system structured event for ai call failure")
}
