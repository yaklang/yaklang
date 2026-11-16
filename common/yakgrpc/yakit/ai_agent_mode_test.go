package yakit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestAgentModeSessionMigration(t *testing.T) {
	for _, tc := range []struct{ raw, mode string }{
		{`{}`, ypb.AgentModeSecurity},
		{`{"CodingMode":true}`, ypb.AgentModeCoding},
		{`{"CodingMode":false}`, ypb.AgentModeSecurity},
		{`{"AgentMode":"security","CodingMode":true}`, ypb.AgentModeSecurity},
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
	workspace, err := CreateAICodingWorkspace(ctx, db, "legacy", t.TempDir())
	require.NoError(t, err)
	// Historical metadata predating AgentMode and without a coding binding row.
	_, err = EnsureAISessionMeta(db, "legacy-mode")
	require.NoError(t, err)
	legacy := &ypb.AIStartParams{CodingMode: true, CodingWorkspaceID: workspace.WorkspaceID}
	// Store raw legacy JSON so the normal serializer cannot upgrade it first.
	raw := `{"CodingMode":true,"CodingWorkspaceID":"` + workspace.WorkspaceID + `"}`
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", "legacy-mode").UpdateColumn("start_params", raw).Error)
	request := &ypb.AIStartParams{AgentMode: ypb.AgentModeSecurity}
	require.NoError(t, ResolveCodingSessionBinding(ctx, db, db, "legacy-mode", request))
	require.Equal(t, ypb.AgentModeCoding, request.AgentMode)
	require.Equal(t, workspace.RootPath, request.CodingWorkingDirectory)
	_, err = marshalAISessionStartParams(legacy)
	require.NoError(t, err)
	require.Empty(t, legacy.AgentMode, "serialization must not mutate the caller")

	security := &ypb.AIStartParams{AgentMode: ypb.AgentModeSecurity, CodingMode: true, CodingWorkspaceID: workspace.WorkspaceID, CodingCreateWorktree: true}
	require.NoError(t, ResolveCodingSessionBinding(ctx, db, db, "explicit-security", security))
	require.Empty(t, security.CodingWorkspaceID)
	require.False(t, security.CodingCreateWorktree)
	require.ErrorContains(t, ResolveCodingSessionBinding(ctx, db, db, "unknown-mode", &ypb.AIStartParams{AgentMode: "unknown"}), "unsupported agent mode")
}
