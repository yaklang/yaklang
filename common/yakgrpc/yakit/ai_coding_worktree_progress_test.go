package yakit

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCodingWorktreeStartupProgress(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	db, root := codingTestDB(t), codingTestRepo(t)
	workspace, err := CreateAICodingWorkspace(context.Background(), db, "project", root)
	require.NoError(t, err)
	params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID, CodingCreateWorktree: true}
	var stages []string
	observe := func(p CodingWorktreeProgress) {
		stages = append(stages, p.Stage)
		if p.Stage == "checkout" {
			_, err := os.Stat(p.Path)
			require.True(t, os.IsNotExist(err), "checkout must be announced before git runs")
			require.NotEmpty(t, p.BaseCommit)
		}
		if p.Stage == "ready" {
			require.DirExists(t, p.Path)
			tree, err := GetAICodingWorktree(db, workspace.WorkspaceID, p.WorktreeID, "")
			require.NoError(t, err)
			require.Equal(t, "ready", tree.State)
		}
	}
	require.NoError(t, ResolveCodingSessionBinding(context.Background(), db, db, "progress", params, observe))
	require.Equal(t, []string{"workspace", "checkout", "ready"}, stages)

	// A retry reuses the durable checkout and reports its resolved metadata.
	retry := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID, CodingCreateWorktree: true}
	var retryProgress []CodingWorktreeProgress
	require.NoError(t, ResolveCodingSessionBinding(context.Background(), db, db, "progress", retry, func(p CodingWorktreeProgress) { retryProgress = append(retryProgress, p) }))
	require.Equal(t, params.CodingWorktreeID, retryProgress[len(retryProgress)-1].WorktreeID)
	require.Equal(t, "ready", retryProgress[len(retryProgress)-1].Stage)

	bad := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID, CodingCreateWorktree: true, CodingWorktreeBaseRef: "does-not-exist"}
	var failed []CodingWorktreeProgress
	require.Error(t, ResolveCodingSessionBinding(context.Background(), db, db, "failed", bad, func(p CodingWorktreeProgress) { failed = append(failed, p) }))
	require.Len(t, failed, 2)
	require.Equal(t, "failed", failed[1].Stage)
	require.NotEmpty(t, failed[1].Error)

	// Local coding and the default security agent have no worktree progress card.
	for _, local := range []*ypb.AIStartParams{{}, {AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID}} {
		called := false
		require.NoError(t, ResolveCodingSessionBinding(context.Background(), db, db, "local-"+local.AgentMode, local, func(CodingWorktreeProgress) { called = true }))
		require.False(t, called)
	}
}
