package loopinfra

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

type saveEvidenceTestInvoker struct {
	*testInvoker
	verifyQuery      string
	verifyPayload    string
	verifyIsToolCall bool
	verifyCalls      int
}

func (t *saveEvidenceTestInvoker) VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (*aicommon.VerifySatisfactionResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.verifyCalls++
	t.verifyQuery = query
	t.verifyPayload = payload
	t.verifyIsToolCall = isToolCall
	return t.verifySatisfactionResult, nil
}

func buildSaveEvidenceAction(payload string) *aicommon.Action {
	action, err := aicommon.ExtractAction(payload, schema.AI_REACT_LOOP_ACTION_SAVE_EVIDENCE)
	if err != nil {
		panic(err)
	}
	return action
}

func TestSaveEvidence_Handler_UsesExplicitPayloadAndForcesVerification(t *testing.T) {
	ctx := context.Background()
	invoker := &saveEvidenceTestInvoker{testInvoker: newTestInvoker(ctx)}
	invoker.verifySatisfactionResult = aicommon.NewVerifySatisfactionResult(true, "done", "")

	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.SetCurrentTask(task)

	action := buildSaveEvidenceAction(`{
		"@action": "save_evidence",
		"verification_payload": "implemented the current change and want explicit acceptance now"
	}`)

	require.NoError(t, loopAction_SaveEvidence.ActionVerifier(loop, action))
	op := reactloops.NewActionHandlerOperator(task)
	loopAction_SaveEvidence.ActionHandler(loop, action, op)

	assert.Equal(t, 1, invoker.verifyCalls)
	assert.Equal(t, task.GetUserInput(), invoker.verifyQuery)
	assert.Equal(t, "implemented the current change and want explicit acceptance now", invoker.verifyPayload)
	assert.False(t, invoker.verifyIsToolCall)
	// verification 收缩为纯观测角色后, satisfied=true 不再触发 operator.Exit
	// (退出职责迁移到 AI 主动 finish). satisfied 仅作为观测信号沉淀, operator
	// 不应被终止.
	terminated, err := op.IsTerminated()
	require.NoError(t, err)
	assert.False(t, terminated, "verification satisfied must NOT terminate the operator anymore")
}

func TestSaveEvidence_Handler_BuildsDefaultPayloadWhenEmpty(t *testing.T) {
	ctx := context.Background()
	invoker := &saveEvidenceTestInvoker{testInvoker: newTestInvoker(ctx)}
	invoker.verifySatisfactionResult = aicommon.NewVerifySatisfactionResult(false, "need one more step", "")

	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.SetCurrentTask(task)

	action := buildSaveEvidenceAction(`{"@action": "save_evidence"}`)
	require.NoError(t, loopAction_SaveEvidence.ActionVerifier(loop, action))
	op := reactloops.NewActionHandlerOperator(task)
	loopAction_SaveEvidence.ActionHandler(loop, action, op)

	assert.Equal(t, 1, invoker.verifyCalls)
	assert.Contains(t, invoker.verifyPayload, "Agent explicitly requested verification")
	assert.Contains(t, invoker.verifyPayload, "Current iteration:")
	assert.Contains(t, invoker.verifyPayload, "Use the full timeline, TODO snapshot, and shared context as the primary evidence for acceptance.")
	assert.True(t, op.IsContinued())
	assert.Contains(t, op.GetFeedback().String(), "need one more step")
}
