package aireact

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestReAct_DirectlyAnswer_DelegatesToActiveMainLoop(t *testing.T) {
	var aiCalls atomic.Int32
	ins, err := NewTestReAct(
		aicommon.WithDirectlyAnswerViaMainLoop(true),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			aiCalls.Add(1)
			return nil, nil
		}),
	)
	require.NoError(t, err)

	loop := reactloops.NewMinimalReActLoop(ins.config, ins)
	task := aicommon.NewStatefulTaskBase("delegated-answer-task", "检查主机", context.Background(), ins.Emitter, true)
	task.SetStatus(aicommon.AITaskState_Processing)
	loop.SetCurrentTask(task)
	loop.SetDirectlyAnswerDelegationAllowed(true)

	answer, err := ins.DirectlyAnswer(
		context.Background(),
		"总结当前证据并标注已完成任务",
		nil,
		aicommon.WithDirectlyAnswerReferenceMaterial("evidence: host is reachable", 0),
	)
	require.Empty(t, answer)
	require.ErrorIs(t, err, aicommon.ErrDirectlyAnswerDelegatedToMainLoop)
	require.Zero(t, aiCalls.Load(), "delegation must not issue a standalone AI request")
	require.True(t, loop.HasPendingStageSummaryRequest())
}

func TestReAct_DirectlyAnswer_AsyncTaskKeepsStandaloneFallback(t *testing.T) {
	var aiCalls atomic.Int32
	ins, err := NewTestReAct(
		aicommon.WithDirectlyAnswerViaMainLoop(true),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			aiCalls.Add(1)
			return mockedLoopDirectlyAnswerOutput(i, `{"@action":"directly_answer","answer_payload":"async fallback"}`)
		}),
	)
	require.NoError(t, err)

	loop := reactloops.NewMinimalReActLoop(ins.config, ins)
	task := aicommon.NewStatefulTaskBase("async-answer-task", "异步检查", context.Background(), ins.Emitter, true)
	task.SetStatus(aicommon.AITaskState_Processing)
	task.SetAsyncMode(true)
	loop.SetCurrentTask(task)
	loop.SetDirectlyAnswerDelegationAllowed(true)

	answer, err := ins.DirectlyAnswer(context.Background(), "总结异步结果", nil)
	require.NoError(t, err)
	require.Equal(t, "async fallback", answer)
	require.EqualValues(t, 1, aiCalls.Load())
	require.False(t, loop.HasPendingStageSummaryRequest())
}
