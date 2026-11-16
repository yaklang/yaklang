package yakit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestAgentModeSessionPersistence(t *testing.T) {
	for _, tc := range []struct{ raw, mode string }{
		{`{}`, ypb.AgentModeSecurity},
		{`{"AgentMode":"coding"}`, ypb.AgentModeCoding},
	} {
		params, err := UnmarshalAISessionStartParams(tc.raw)
		require.NoError(t, err)
		require.Equal(t, tc.mode, params.AgentMode)
		raw, err := marshalAISessionStartParams(params)
		require.NoError(t, err)
		restored, err := UnmarshalAISessionStartParams(raw)
		require.NoError(t, err)
		require.Equal(t, tc.mode, restored.AgentMode)
	}

	db := codingTestDB(t)
	ctx := context.Background()
	workspace, err := CreateAICodingWorkspace(ctx, db, "project", t.TempDir())
	require.NoError(t, err)
	// Persisted security sessions cannot be switched to coding by an attachment.
	_, err = EnsureAISessionMeta(db, "saved-security")
	require.NoError(t, err)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", "saved-security").UpdateColumn("start_params", `{"AgentMode":"security"}`).Error)
	request := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID}
	require.NoError(t, ResolveCodingSessionBinding(ctx, db, db, "saved-security", request))
	require.Equal(t, ypb.AgentModeSecurity, request.AgentMode)
	require.Empty(t, request.CodingWorkspaceID)
	security := &ypb.AIStartParams{AgentMode: ypb.AgentModeSecurity, CodingWorkspaceID: workspace.WorkspaceID, CodingCreateWorktree: true}
	require.NoError(t, ResolveCodingSessionBinding(ctx, db, db, "explicit-security", security))
	require.Empty(t, security.CodingWorkspaceID)
	require.False(t, security.CodingCreateWorktree)
	require.ErrorContains(t, ResolveCodingSessionBinding(ctx, db, db, "unknown-mode", &ypb.AIStartParams{AgentMode: "unknown"}), "unsupported agent mode")
}
