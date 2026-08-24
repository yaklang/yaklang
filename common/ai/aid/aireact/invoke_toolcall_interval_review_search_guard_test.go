package aireact

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestLongSearchStdoutGuard_SkipsUntilSilenceAfterOutputOrStart(t *testing.T) {
	var g longSearchStdoutGuard
	started := time.Unix(1_700_000_000, 0)

	g.observe(nil, nil, started)
	require.True(t, g.shouldSkipAI(started.Add(time.Minute), started, 15*time.Minute), "no output yet must skip AI")
	require.False(t, g.shouldSkipAI(started.Add(16*time.Minute), started, 15*time.Minute), "no output past 15m from start may review")

	g.observe([]byte("=== Start Grep ==="), nil, started.Add(time.Minute))
	require.True(t, g.shouldSkipAI(started.Add(2*time.Minute), started, 15*time.Minute), "start banner is still live output")

	g.observe([]byte("=== Start Grep ==="), nil, started.Add(10*time.Minute))
	require.True(t, g.shouldSkipAI(started.Add(10*time.Minute), started, 15*time.Minute), "unchanged banner for 9m is under the window")
	require.False(t, g.shouldSkipAI(started.Add(17*time.Minute), started, 15*time.Minute), "unchanged banner past 15m may review")

	g.observe([]byte("=== Start Grep ==="), []byte("scanning..."), started.Add(17*time.Minute))
	require.True(t, g.shouldSkipAI(started.Add(17*time.Minute), started, 15*time.Minute), "new stderr resets the silence window")

	g.observe([]byte("=== Start Grep ===\nmatch.go:1"), []byte("scanning..."), started.Add(18*time.Minute))
	require.True(t, g.shouldSkipAI(started.Add(18*time.Minute), started, 15*time.Minute), "new stdout resets the silence window")
}

func TestIntervalReviewHandler_SkipsAIForGrepWhileStdoutIsLive(t *testing.T) {
	cancelJSON := `{"@action":"interval-toolcall-review","decision":"cancel","reason":"no new output / LOOP_STALL_DETECTED","progress_summary":"only start banner","estimated_remaining_time":"unknown"}`
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

	grep := aitool.NewWithoutCallback("grep", aitool.WithStringParam("pattern"))
	other := aitool.NewWithoutCallback("sleep_test", aitool.WithNumberParam("seconds"))
	task := aicommon.NewStatefulTaskBase(
		"search-guard-task",
		"keep searching",
		context.Background(),
		react.Emitter,
		true,
	)
	handler := react.CreateIntervalReviewHandlerForTaskAndEmitter(task, react.Emitter)
	require.NotNil(t, handler)

	banner := []byte("=== Start Grep ===")
	shouldContinue, reviewErr := handler(
		context.Background(),
		grep,
		aitool.InvokeParams{"pattern": "Runtime.exec"},
		banner,
		nil,
		"",
	)
	require.NoError(t, reviewErr)
	require.True(t, shouldContinue, "grep must continue without asking interval-review AI")
	require.Equal(t, int32(0), atomic.LoadInt32(&calls), "live grep must not spend an AI review turn")

	shouldContinue, reviewErr = handler(
		context.Background(),
		other,
		aitool.InvokeParams{"seconds": 1},
		[]byte("still sleeping"),
		nil,
		"",
	)
	require.NoError(t, reviewErr)
	require.False(t, shouldContinue, "non-search tools still honor interval-review cancel")
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestIntervalReviewHandler_HonorsGrepCancelAfterStdoutSilence(t *testing.T) {
	prev := intervalReviewLongSearchSilence
	intervalReviewLongSearchSilence = time.Millisecond
	t.Cleanup(func() { intervalReviewLongSearchSilence = prev })

	cancelJSON := `{"@action":"interval-toolcall-review","decision":"cancel","reason":"silent too long","progress_summary":"banner only","estimated_remaining_time":"unknown"}`
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

	grep := aitool.NewWithoutCallback("grep", aitool.WithStringParam("pattern"))
	task := aicommon.NewStatefulTaskBase(
		"search-silence-task",
		"keep searching",
		context.Background(),
		react.Emitter,
		true,
	)
	handler := react.CreateIntervalReviewHandlerForTaskAndEmitter(task, react.Emitter)
	require.NotNil(t, handler)

	banner := []byte("=== Start Grep ===")
	shouldContinue, reviewErr := handler(
		context.Background(),
		grep,
		aitool.InvokeParams{"pattern": "foo"},
		banner,
		nil,
		"",
	)
	require.NoError(t, reviewErr)
	require.True(t, shouldContinue, "first observation of stdout must still skip AI")
	require.Equal(t, int32(0), atomic.LoadInt32(&calls))

	time.Sleep(3 * time.Millisecond)
	shouldContinue, reviewErr = handler(
		context.Background(),
		grep,
		aitool.InvokeParams{"pattern": "foo"},
		banner,
		nil,
		"",
	)
	require.NoError(t, reviewErr)
	require.False(t, shouldContinue, "unchanged stdout past the silence window may cancel")
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestIntervalReviewHandler_FindFileAndTreeUseTheSameVeto(t *testing.T) {
	cancelJSON := `{"@action":"interval-toolcall-review","decision":"cancel","reason":"slow","progress_summary":"listing","estimated_remaining_time":"unknown"}`
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(cancelJSON))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase(
		"search-tools-task",
		"enumerate files",
		context.Background(),
		react.Emitter,
		true,
	)

	for _, name := range []string{"find_file", "tree"} {
		t.Run(name, func(t *testing.T) {
			handler := react.CreateIntervalReviewHandlerForTaskAndEmitter(task, react.Emitter)
			require.NotNil(t, handler)
			tool := aitool.NewWithoutCallback(name)
			shouldContinue, reviewErr := handler(
				context.Background(),
				tool,
				aitool.InvokeParams{},
				[]byte("start"),
				nil,
				"",
			)
			require.NoError(t, reviewErr)
			require.True(t, shouldContinue, "%s cancel must be vetoed", name)
		})
	}
}
