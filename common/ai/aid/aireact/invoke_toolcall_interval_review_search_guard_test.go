package aireact

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestIntervalReviewOutputGuard_SkipsOnlyWhenSnapshotChanges(t *testing.T) {
	var g intervalReviewOutputGuard
	require.False(t, g.outputChanged(nil, nil))
	require.True(t, g.outputChanged([]byte("running"), nil))
	require.False(t, g.outputChanged([]byte("running"), nil))
	require.True(t, g.outputChanged([]byte("running"), []byte("warn")))
	require.True(t, g.outputChanged([]byte("done"), []byte("warn")))
}

func TestIntervalReviewHandler_SkipsAIWhileOutputIsLiveThenReviewsWhenFrozen(t *testing.T) {
	cancelJSON := `{"@action":"interval-toolcall-review","decision":"cancel","reason":"no new output","progress_summary":"idle","estimated_remaining_time":"unknown"}`
	var calls int32
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			atomic.AddInt32(&calls, 1)
			rsp := config.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(cancelJSON))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	tool := aitool.NewWithoutCallback("sleep_test", aitool.WithNumberParam("seconds"))
	task := aicommon.NewStatefulTaskBase(
		"output-guard-task",
		"keep running",
		context.Background(),
		react.Emitter,
		true,
	)
	handler := react.CreateIntervalReviewHandlerForTaskAndEmitter(task, react.Emitter)
	require.NotNil(t, handler)

	shouldContinue, reviewErr := handler(
		context.Background(),
		tool,
		aitool.InvokeParams{"seconds": 1},
		[]byte("still sleeping"),
		nil,
		"",
	)
	require.NoError(t, reviewErr)
	require.True(t, shouldContinue, "new output must skip AI")
	require.Equal(t, int32(0), atomic.LoadInt32(&calls))

	shouldContinue, reviewErr = handler(
		context.Background(),
		tool,
		aitool.InvokeParams{"seconds": 1},
		[]byte("still sleeping"),
		nil,
		"",
	)
	require.NoError(t, reviewErr)
	require.False(t, shouldContinue, "unchanged output is reviewed and may cancel")
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
}
