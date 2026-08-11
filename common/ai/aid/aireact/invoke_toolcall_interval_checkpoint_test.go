package aireact

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestIntervalReviewUsesDetachedCheckpointSequence(t *testing.T) {
	var calls int32
	tool, err := aitool.New(
		"detached_interval_review_tool",
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			return "unused", nil
		}),
	)
	require.NoError(t, err)

	react, err := NewTestReAct(
		aicommon.WithSequence(12400),
		aicommon.WithTools(tool),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			atomic.AddInt32(&calls, 1)
			require.True(t, request.IsDetachedCheckpoint())
			require.Zero(t, request.GetSeqId())
			rsp := config.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"interval-toolcall-review","decision":"continue","reason":"healthy","progress_summary":"running","estimated_remaining_time":"1s"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase(
		"detached-review-task",
		"keep the owning query stable",
		context.Background(),
		react.Emitter,
		true,
	)

	before := react.config.SeqIdProvider.CurrentID()
	shouldContinue, reviewErr := react._invokeToolCall_IntervalReviewWithContextForTask(
		context.Background(),
		task,
		tool,
		aitool.InvokeParams{},
		nil,
		nil,
		time.Now(),
		1,
		"continue",
	)
	require.NoError(t, reviewErr)
	require.True(t, shouldContinue)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
	require.Equal(t, before, react.config.SeqIdProvider.CurrentID(),
		"timing-dependent interval reviews must not shift coordinator replay IDs")
}
