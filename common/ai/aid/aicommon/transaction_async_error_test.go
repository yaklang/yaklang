package aicommon

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergePostHandlerAndCallbackError_InfrastructurePriority(t *testing.T) {
	parseErr := fmt.Errorf("action type is empty (available_actions=[finish])")
	apiErr := fmt.Errorf("request post to https://api.example.com: context deadline exceeded")

	merged := mergePostHandlerAndCallbackError(parseErr, apiErr)
	require.Error(t, merged)
	msg := merged.Error()
	assert.True(t, strings.Contains(msg, "context deadline exceeded"),
		"infrastructure error should be primary, got: %s", msg)
	assert.True(t, strings.Contains(msg, "action type is empty"),
		"parse error should be attached as context, got: %s", msg)
	assert.False(t, strings.HasPrefix(msg, "post handler:"),
		"should not use legacy merge order, got: %s", msg)
}

func TestMergePostHandlerAndCallbackError_ValidationOnly(t *testing.T) {
	parseErr := fmt.Errorf("action type is empty")
	merged := mergePostHandlerAndCallbackError(parseErr, nil)
	assert.Equal(t, parseErr, merged)
}

func TestCallAITransaction_AsyncCallbackErrorSurfacesOverParseFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 1

	callAi := func(req *AIRequest) (*AIResponse, error) {
		rsp := NewAIResponse(nil)
		go func() {
			defer rsp.markCallbackDone()
			// Simulate Tee finishing before SetError without WaitForCallbackDone.
			time.Sleep(50 * time.Millisecond)
			rsp.SetError(fmt.Errorf("request post to https://api.example.com: context deadline exceeded"))
		}()
		return rsp, nil
	}

	postHandler := func(rsp *AIResponse) error {
		return fmt.Errorf("action type is empty (available_actions=[directly_answer finish])")
	}

	err := CallAITransaction(cfg, "timeout-prompt", callAi, postHandler)
	require.Error(t, err)
	errMsg := err.Error()
	t.Logf("error: %s", errMsg)

	assert.True(t, strings.Contains(errMsg, "context deadline exceeded"),
		"expected timeout error to surface, got: %s", errMsg)
	assert.True(t, strings.Contains(errMsg, "action type is empty"),
		"parse error should remain as context, got: %s", errMsg)
}

func TestAIResponse_WaitForCallbackDone(t *testing.T) {
	rsp := NewAIResponse(nil)
	doneCh := make(chan struct{})
	go func() {
		time.Sleep(30 * time.Millisecond)
		rsp.markCallbackDone()
		close(doneCh)
	}()

	ok := rsp.WaitForCallbackDone(context.Background())
	assert.True(t, ok)
	<-doneCh
}

func TestAIResponse_WaitForCallbackDone_AlreadyDone(t *testing.T) {
	rsp := NewUnboundAIResponse()
	ok := rsp.WaitForCallbackDone(context.Background())
	assert.True(t, ok)
}
