package loopinfra

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func buildSaveEvidenceAction(payload string) *aicommon.Action {
	action, err := aicommon.ExtractAction(payload, schema.AI_REACT_LOOP_ACTION_SAVE_EVIDENCE)
	if err != nil {
		panic(err)
	}
	return action
}

func newSaveEvidenceLoop(t *testing.T) (*reactloops.ReActLoop, *mock.MockInvoker, *mock.MockStatefulTask) {
	t.Helper()
	ctx := context.Background()
	invoker := mock.NewMockInvoker(ctx)
	loop, err := reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, invoker)
	require.NoError(t, err)
	task := mock.NewMockStatefulTask(ctx, "save-evidence-test", "collect reusable evidence")
	invoker.SetCurrentTask(task)
	loop.SetCurrentTask(task)
	return loop, invoker, task
}

func executeSaveEvidence(t *testing.T, loop *reactloops.ReActLoop, task *mock.MockStatefulTask, raw string) *reactloops.LoopActionHandlerOperator {
	t.Helper()
	handler, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_SAVE_EVIDENCE)
	require.NoError(t, err)
	require.NotNil(t, handler)
	action := buildSaveEvidenceAction(raw)
	require.NoError(t, handler.ActionVerifier(loop, action))
	op := reactloops.NewActionHandlerOperator(task)
	handler.ActionHandler(loop, action, op)
	return op
}

func TestSaveEvidence_IsCoreActionAndWritesSessionStore(t *testing.T) {
	loop, invoker, task := newSaveEvidenceLoop(t)
	op := executeSaveEvidence(t, loop, task, `{
		"@action": "save_evidence",
		"evidence_id": "refresh-token-replay",
		"verification_payload": "POST /token/refresh accepted the same refresh token twice and returned two valid access tokens."
	}`)

	assert.True(t, op.IsContinued())
	terminated, err := op.IsTerminated()
	require.NoError(t, err)
	assert.False(t, terminated)
	rendered := invoker.GetConfig().GetSessionEvidenceRendered()
	assert.Contains(t, rendered, "[id: refresh-token-replay]")
	assert.Contains(t, rendered, "accepted the same refresh token twice")
}

func TestSaveEvidence_RetryAndUpdateAreIdempotent(t *testing.T) {
	loop, invoker, task := newSaveEvidenceLoop(t)
	first := `{"@action":"save_evidence","evidence_id":"api-auth","verification_payload":"Unauthenticated GET /api/users returned 401."}`
	executeSaveEvidence(t, loop, task, first)
	executeSaveEvidence(t, loop, task, first)
	executeSaveEvidence(t, loop, task, `{"@action":"save_evidence","evidence_id":"api-auth","verification_payload":"Unauthenticated GET /api/users returned 401 with no response body."}`)

	rendered := invoker.GetConfig().GetSessionEvidenceRendered()
	assert.Len(t, regexp.MustCompile(`\[id: api-auth\]`).FindAllStringIndex(rendered, -1), 1)
	assert.NotContains(t, rendered, "returned 401.\n")
	assert.Contains(t, rendered, "returned 401 with no response body")
}

func TestSaveEvidence_DerivesStableIDAndRejectsEmptyContent(t *testing.T) {
	loop, invoker, task := newSaveEvidenceLoop(t)
	raw := `{"@action":"save_evidence","verification_payload":"Controlled comparison ruled out anonymous access to /admin/."}`
	executeSaveEvidence(t, loop, task, raw)
	executeSaveEvidence(t, loop, task, raw)

	rendered := invoker.GetConfig().GetSessionEvidenceRendered()
	assert.Len(t, regexp.MustCompile(`\[id: saved_[0-9a-f]{16}\]`).FindAllStringIndex(rendered, -1), 1)

	handler, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_SAVE_EVIDENCE)
	require.NoError(t, err)
	err = handler.ActionVerifier(loop, buildSaveEvidenceAction(`{"@action":"save_evidence"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session evidence content is required")
}
