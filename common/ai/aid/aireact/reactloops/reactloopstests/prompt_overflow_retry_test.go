package reactloopstests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

// A confirmed prompt overflow must not replay the same prompt. The loop records
// a tighter timeline limit and stops with the original typed error, so the
// caller can decide what to compress.
func TestReActLoop_PromptOverflowStopsReplayAndRecordsLimit(t *testing.T) {
	callCount := 0
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			rsp := i.NewAIResponse()
			rsp.SetRawHTTPResponseData(
				[]byte("HTTP/1.1 400 Bad Request\r\nContent-Type: application/json\r\n\r\n"),
				[]byte(`{"error":{"code":"context_length_exceeded","message":"This model's maximum context length is 8192 tokens. However, your messages resulted in 12000 tokens."}}`),
			)
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	cfg, ok := reactIns.GetConfig().(*aicommon.Config)
	require.True(t, ok)
	base := cfg.GetTimelineContentSizeLimit()
	require.NotNil(t, cfg.GetTimeline())

	loop, err := reactloops.NewReActLoop("prompt-overflow-loop", reactIns)
	require.NoError(t, err)
	err = loop.Execute("prompt-overflow-task", context.Background(), "test input")

	require.Error(t, err)
	require.True(t, aicommon.IsPromptTooLong(err), "the typed overflow must reach the caller: %v", err)
	require.Equal(t, 1, callCount, "the same prompt must not be replayed")
	require.EqualValues(t, 8192*8/10, cfg.GetTimeline().GetTimelineContentLimit(),
		"the loop records a tighter limit for the model that rejected the prompt")
	require.Less(t, cfg.GetTimeline().GetTimelineContentLimit(), base)
}

// Non-overflow provider failures keep their existing handling: no dynamic limit
// is recorded for them.
func TestReActLoop_PlainRequestFailureDoesNotTouchTheLimit(t *testing.T) {
	callCount := 0
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			rsp := i.NewAIResponse()
			rsp.SetRawHTTPResponseData(
				[]byte("HTTP/1.1 400 Bad Request\r\nContent-Type: application/json\r\n\r\n"),
				[]byte(`{"error":{"message":"invalid schema for function"}}`),
			)
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	cfg, ok := reactIns.GetConfig().(*aicommon.Config)
	require.True(t, ok)
	base := cfg.GetTimelineContentSizeLimit()

	loop, err := reactloops.NewReActLoop("plain-failure-loop", reactIns)
	require.NoError(t, err)
	err = loop.Execute("plain-failure-task", context.Background(), "test input")

	require.Error(t, err)
	require.False(t, aicommon.IsPromptTooLong(err))
	require.Equal(t, base, cfg.GetTimeline().GetTimelineContentLimit())
}
