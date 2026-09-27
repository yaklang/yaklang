package aireact

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestReAct_PersistentSession_Evidence verifies that session-level evidence persists
// across two conversations within the same persistent session:
//  1. Session 1 applies evidence operations (WAF findings, parameter discovery, test results)
//  2. Evidence is persisted to DB via ApplySessionEvidenceOps
//  3. Session 2 with the same persistent session ID restores evidence
//  4. GetSessionEvidenceRendered returns the same content
//  5. A session without persistent ID has no evidence
func TestReAct_PersistentSession_Evidence(t *testing.T) {
	pid := uuid.New().String()

	newMinimalReAct := func() (*ReAct, error) {
		in := make(chan *ypb.AIInputEvent, 1)
		return NewTestReAct(
			aicommon.WithEventInputChan(in),
			aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				return nil, nil
			}),
			aicommon.WithPersistentSessionId(pid),
		)
	}

	// === Session 1: Generate evidence ===
	ins1, err := newMinimalReAct()
	require.NoError(t, err)
	require.NotNil(t, ins1)

	// Before applying ops, evidence should be empty
	require.Empty(t, ins1.config.GetSessionEvidenceRendered(),
		"session 1 should start with empty evidence")

	// Apply evidence operations simulating real tool execution findings
	ops1 := []aicommon.EvidenceOperation{
		{ID: "waf-regex", Op: "add", Content: "WAF regex filter: /cat|more|less|head|tac|tail|nl|od|vi|vim|sort|flag| |;|[0-9]|\\*|`|%|>|<|'|\"/i"},
		{ID: "param-channel", Op: "add", Content: "Vulnerable parameter: rce via GET method ($_GET['rce']). POST body is NOT read by the application."},
		{ID: "bypass-test-1", Op: "add", Content: "Tested: rce=ls${IFS}/ via GET -> 200 OK, body contains directory listing. Command injection confirmed."},
	}
	ins1.config.ApplySessionEvidenceOps(ops1)

	// Verify evidence is rendered
	rendered1 := ins1.config.GetSessionEvidenceRendered()
	require.NotEmpty(t, rendered1, "evidence should be rendered after applying ops")
	assert.Contains(t, rendered1, "waf-regex")
	assert.Contains(t, rendered1, "WAF regex filter")
	assert.Contains(t, rendered1, "param-channel")
	assert.Contains(t, rendered1, "GET method")
	assert.Contains(t, rendered1, "bypass-test-1")
	assert.Contains(t, rendered1, "Command injection confirmed")
	log.Infof("session 1 evidence rendered (length=%d): %s", len(rendered1), rendered1[:min(200, len(rendered1))])

	// Apply more evidence (update existing + add new)
	ops2 := []aicommon.EvidenceOperation{
		{ID: "bypass-test-2", Op: "add", Content: "Tested: rce=/???/??t${IFS}/fla? via GET -> 200 OK, body=60KB binary. Wildcard bypass successful, target file found."},
		{ID: "bypass-test-1", Op: "update", Content: "Tested: rce=ls${IFS}/ via GET -> 200 OK, directory listing shows /flag file exists. Command injection confirmed."},
	}
	ins1.config.ApplySessionEvidenceOps(ops2)

	rendered1After := ins1.config.GetSessionEvidenceRendered()
	assert.Contains(t, rendered1After, "bypass-test-2")
	assert.Contains(t, rendered1After, "Wildcard bypass successful")
	assert.Contains(t, rendered1After, "/flag file exists")

	// Verify evidence is persisted in DB
	runtime1, err := yakit.GetLatestAIAgentRuntimeByPersistentSession(consts.GetGormProjectDatabase(), pid)
	require.NoError(t, err)
	require.NotNil(t, runtime1)
	dbEvidence := runtime1.GetEvidence()
	require.NotEmpty(t, dbEvidence, "evidence should be persisted in DB")
	assert.Contains(t, dbEvidence, "waf-regex")
	assert.Contains(t, dbEvidence, "bypass-test-2")
	log.Infof("session 1 DB evidence length: %d", len(dbEvidence))

	// === Session 2: Restore evidence from persistent session ===
	ins2, err := newMinimalReAct()
	require.NoError(t, err)
	require.NotNil(t, ins2)

	rendered2 := ins2.config.GetSessionEvidenceRendered()
	require.NotEmpty(t, rendered2, "session 2 should have restored evidence from persistent session")
	assert.Contains(t, rendered2, "waf-regex")
	assert.Contains(t, rendered2, "WAF regex filter")
	assert.Contains(t, rendered2, "param-channel")
	assert.Contains(t, rendered2, "bypass-test-1")
	assert.Contains(t, rendered2, "/flag file exists")
	assert.Contains(t, rendered2, "bypass-test-2")
	assert.Contains(t, rendered2, "Wildcard bypass successful")
	log.Infof("session 2 restored evidence (length=%d)", len(rendered2))

	// Session 2 can also add more evidence
	ops3 := []aicommon.EvidenceOperation{
		{ID: "final-payload", Op: "add", Content: "Final working payload: rce=c\\at${IFS}/fla? via GET -> 200 OK, flag content retrieved."},
	}
	ins2.config.ApplySessionEvidenceOps(ops3)

	rendered2After := ins2.config.GetSessionEvidenceRendered()
	assert.Contains(t, rendered2After, "final-payload")
	assert.Contains(t, rendered2After, "flag content retrieved")

	// === Session 3: Verify evidence accumulates across sessions ===
	ins3, err := newMinimalReAct()
	require.NoError(t, err)
	require.NotNil(t, ins3)

	rendered3 := ins3.config.GetSessionEvidenceRendered()
	require.NotEmpty(t, rendered3, "session 3 should restore all accumulated evidence")
	assert.Contains(t, rendered3, "waf-regex")
	assert.Contains(t, rendered3, "bypass-test-2")
	assert.Contains(t, rendered3, "final-payload")
	assert.Contains(t, rendered3, "flag content retrieved")
	log.Infof("session 3 accumulated evidence (length=%d)", len(rendered3))

	// === Session 4: Without persistent session ID - should have no evidence ===
	in4 := make(chan *ypb.AIInputEvent, 1)
	ins4, err := NewTestReAct(
		aicommon.WithEventInputChan(in4),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, nil
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, ins4)
	assert.Empty(t, ins4.config.GetSessionEvidenceRendered(),
		"session without persistent ID should have no evidence")

	// === Test evidence delete operation ===
	ins5, err := newMinimalReAct()
	require.NoError(t, err)
	require.NotNil(t, ins5)

	deleteOps := []aicommon.EvidenceOperation{
		{ID: "bypass-test-1", Op: "delete"},
	}
	ins5.config.ApplySessionEvidenceOps(deleteOps)

	rendered5 := ins5.config.GetSessionEvidenceRendered()
	assert.NotContains(t, rendered5, "bypass-test-1")
	assert.Contains(t, rendered5, "waf-regex")
	assert.Contains(t, rendered5, "final-payload")

	// Verify deletion persisted
	ins6, err := newMinimalReAct()
	require.NoError(t, err)
	rendered6 := ins6.config.GetSessionEvidenceRendered()
	assert.NotContains(t, rendered6, "bypass-test-1",
		"deleted evidence should not appear in subsequent sessions")
	assert.Contains(t, rendered6, "waf-regex",
		"non-deleted evidence should persist")

	log.Infof("persistent session evidence test passed")
}

func TestEvidencePersistenceIdleRestartsPreserveFreezeAndDeletion(t *testing.T) {
	pid := uuid.New().String()
	create := func() *ReAct {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		react, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithPersistentSessionId(pid), aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			t.Error("evidence persistence must not call a model")
			return nil, nil
		}))
		require.NoError(t, err)
		react.config.GetTimeline().SetTimelineBucketByteSize(-1)
		return react
	}
	first := create()
	first.config.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "stable", Content: "FROZEN_EVIDENCE"}})
	first.config.GetTimeline().FreezeAll()
	first.config.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "pending", Content: "PENDING_EVIDENCE"}})
	expected := aicommon.RenderTimelineFrozenOpen(first.config.GetTimeline())
	require.Contains(t, expected.EvidenceSemiDynamic, "FROZEN_EVIDENCE")
	require.Contains(t, expected.Open, "PENDING_EVIDENCE")
	// No write between these restores: each new runtime must flush the journal too.
	assertRestored := func(react *ReAct) {
		t.Helper()
		actual := aicommon.RenderTimelineFrozenOpen(react.config.GetTimeline())
		require.Equal(t, expected.EvidenceSemiDynamic, actual.EvidenceSemiDynamic)
		require.Contains(t, actual.Open, "PENDING_EVIDENCE")
		require.NotContains(t, actual.Open, "FROZEN_EVIDENCE")
		require.Equal(t, 1, strings.Count(actual.Open, "[UPSERT]"))
		require.Equal(t, expected.Frozen, actual.Frozen)
		snapshot := react.config.GetTimeline().FreezeSnapshot()
		require.Len(t, snapshot.Batches, 1)
		require.Len(t, snapshot.Batches[0].IDs, 1)
	}
	// Restart deliberately reassigns item IDs. Validate placement and contents,
	// not the Open tag's ID-derived nonce.
	second := create()
	assertRestored(second)
	third := create()
	assertRestored(third)
	third.config.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "delete", ID: "stable"}, {Op: "delete", ID: "pending"}})
	third.config.GetTimeline().FreezeAll()
	third.config.FlushRestoredSessionEvidence()
	fourth := create()
	require.Empty(t, fourth.config.GetSessionEvidenceRendered())
	require.Empty(t, aicommon.RenderTimelineFrozenOpen(fourth.config.GetTimeline()).EvidenceSemiDynamic)
	fifth := create()
	require.Empty(t, fifth.config.GetSessionEvidenceRendered(), "old mirrors cannot resurrect deleted evidence")
}
