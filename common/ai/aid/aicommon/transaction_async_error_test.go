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

func TestNormalizeTransactionPostHandlerError_ActionResolution(t *testing.T) {
	t.Run("missing action with no model output is reported as empty response", func(t *testing.T) {
		rsp := NewUnboundAIResponse()
		err := fmt.Errorf(`action resolution failed: requested="<missing>"; matcher=exact registered action or alias; available_actions=[finish]; reason=no non-empty @action was emitted`)

		normalized := normalizeTransactionPostHandlerError(rsp, err)
		require.Error(t, normalized)
		assert.Contains(t, normalized.Error(), "ai model returned empty response")
		assert.Contains(t, normalized.Error(), `requested="<missing>"`)
	})

	t.Run("unsupported non-empty action is not mislabeled as empty response", func(t *testing.T) {
		rsp := NewUnboundAIResponse()
		err := fmt.Errorf(`action resolution failed: requested="save_evidence"; matcher=exact registered action or alias; available_actions=[finish]; reason=no registered action or alias matched`)

		normalized := normalizeTransactionPostHandlerError(rsp, err)
		assert.Equal(t, err, normalized)
		assert.NotContains(t, normalized.Error(), "ai model returned empty response")
	})
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

func TestTransactionHTTP400StopsBeforeActionParsing(t *testing.T) {
	cfg := newTransactionTestConfig(context.Background())
	cfg.retryMax = 5
	calls, parsed := 0, false
	err := CallAITransaction(cfg, "invalid tool schema", func(*AIRequest) (*AIResponse, error) {
		calls++
		rsp := NewUnboundAIResponse()
		rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"), []byte(`{"error":{"message":"Invalid schema: required:null"}}`))
		return rsp, nil
	}, func(*AIResponse) error {
		parsed = true
		return fmt.Errorf("action @action not found or invalid")
	})
	require.ErrorContains(t, err, "400")
	require.ErrorContains(t, err, "Invalid schema")
	require.NotContains(t, err.Error(), "empty response")
	require.NotContains(t, err.Error(), "max retry count")
	require.Equal(t, 1, calls)
	require.False(t, parsed)
}

func TestTransactionHTTPRetryClassification(t *testing.T) {
	for _, status := range []int{0, 200, 400, 401, 403, 404, 408, 409, 413, 422, 425, 429, 500, 502, 503} {
		rsp := NewUnboundAIResponse()
		rsp.SetRawHTTPResponseData([]byte(fmt.Sprintf("HTTP/1.1 %d Status\r\n\r\n", status)), nil)
		want := status == 400 || status == 401 || status == 403 || status == 404 || status == 413 || status == 422
		require.Equal(t, want, isNonRetryableAIHTTPResponse(rsp), "status %d", status)
	}
}
