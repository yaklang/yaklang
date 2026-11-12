package aicommon

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAICallTraceHookCapturesPromptAndError(t *testing.T) {
	var records []AICallTraceRecord
	cfg := NewConfig(
		context.Background(),
		WithDisableAutoSkills(true),
		WithAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
			return nil, errors.New("provider failed")
		}),
		WithAIAutoRetry(1),
		WithAIRetryWaitFunc(func(context.Context, time.Duration) error { return nil }),
		WithAICallTraceHook(func(record AICallTraceRecord) {
			records = append(records, record)
		}),
	)

	request := NewAIRequest("trace prompt", WithAIRequest_CallerLabel("trace-test"))
	_, err := cfg.GetQualityPriorityAICallback()(cfg, request)
	require.ErrorContains(t, err, "provider failed")
	require.Len(t, records, 1)
	require.Equal(t, "trace prompt", records[0].Prompt)
	require.Equal(t, "trace-test", records[0].CallerLabel)
	require.Contains(t, records[0].Error, "provider failed")
}

func TestAICallTraceHookPanicDoesNotFailAICall(t *testing.T) {
	cfg := NewConfig(
		context.Background(),
		WithDisableAutoSkills(true),
		WithAICallback(func(config AICallerConfigIf, _ *AIRequest) (*AIResponse, error) {
			response := config.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString("ok"))
			response.Close()
			return response, nil
		}),
		WithAIAutoRetry(1),
		WithAIRetryWaitFunc(func(context.Context, time.Duration) error { return nil }),
		WithAICallTraceHook(func(AICallTraceRecord) {
			panic("trace failure")
		}),
	)
	_, err := cfg.GetQualityPriorityAICallback()(cfg, NewAIRequest("prompt"))
	require.NoError(t, err)
}
