package aireact

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
)

func TestAsyncDirectAnswerCrossesTaskLoopBoundary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	react, err := NewTestReAct(aicommon.WithContext(ctx))
	require.NoError(t, err)

	first := reactloops.NewMinimalReActLoop(react.config, react)
	first.SetCurrentTask(aicommon.NewStatefulTaskBase("review-turn", "start review", ctx, react.config.GetEmitter(), true))
	first.RequestAsyncDirectAnswer()
	first.Release()

	second := reactloops.NewMinimalReActLoop(react.config, react)
	second.SetCurrentTask(aicommon.NewStatefulTaskBase("answer-turn", "answer now", ctx, react.config.GetEmitter(), true))
	t.Cleanup(second.Release)
	select {
	case <-second.AsyncDirectAnswerSignal():
	case <-time.After(time.Second):
		t.Fatal("released loop request did not wake the next task loop")
	}
	_, err = react.GetAsyncExecutionManager()
	require.ErrorIs(t, err, asyncexecmanager.ErrClosed)
	_, err = second.GetAsyncExecutionManager()
	require.ErrorIs(t, err, asyncexecmanager.ErrClosed)
	require.True(t, react.ConsumeAsyncDirectAnswer())
	require.False(t, react.ConsumeAsyncDirectAnswer(), "one review request must be consumed once")
	_, err = second.GetAsyncExecutionManager()
	require.NoError(t, err, "acknowledging direct answer must reopen session admission")

	first.RequestAsyncDirectAnswer()
	select {
	case <-second.AsyncDirectAnswerSignal():
	case <-time.After(time.Second):
		t.Fatal("a later review request was not reusable after acknowledgement")
	}
}
