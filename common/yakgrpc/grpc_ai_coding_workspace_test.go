package yakgrpc

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCodingWorkspaceRPC(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.AICodingWorkspace{}, &schema.AICodingSession{}, &schema.AICodingWorktree{}).Error)
	server := &Server{profileDatabase: db}
	ctx := context.Background()
	workspace, err := server.CreateAICodingWorkspace(ctx, &ypb.CreateAICodingWorkspaceRequest{Name: "project", RootPaths: []string{t.TempDir()}})
	require.NoError(t, err)
	list, err := server.QueryAICodingWorkspaces(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.Len(t, list.Data, 1)
	require.Equal(t, workspace.ID, list.Data[0].ID)
	trees, err := server.ListAICodingWorktrees(ctx, &ypb.AICodingWorkspaceRequest{WorkspaceID: workspace.ID})
	require.NoError(t, err)
	require.Empty(t, trees.Data)
	_, err = server.ListAICodingWorktrees(ctx, &ypb.AICodingWorkspaceRequest{WorkspaceID: "missing"})
	require.Error(t, err)
}

func TestCodingManagedWorktreeRPCOwnershipAndAvailability(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.AICodingWorkspace{}, &schema.AICodingSession{}, &schema.AICodingWorktree{}, &schema.AISession{}).Error)
	server, ctx := &Server{profileDatabase: db}, context.Background()
	root := t.TempDir()
	for _, args := range [][]string{{"init"}, {"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"}} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	workspace, err := server.CreateAICodingWorkspace(ctx, &ypb.CreateAICodingWorkspaceRequest{Name: "two roots", RootPaths: []string{root, t.TempDir()}})
	require.NoError(t, err)
	require.Len(t, workspace.RootPaths, 2)

	branches, err := server.ListAICodingBranches(ctx, &ypb.AICodingWorkspaceRequest{WorkspaceID: workspace.ID})
	require.NoError(t, err)
	require.Len(t, branches.Data, 1)
	require.NotEmpty(t, branches.CurrentCommit)
	_, err = server.ListAICodingBranches(ctx, &ypb.AICodingWorkspaceRequest{WorkspaceID: "missing"})
	require.Error(t, err)
	params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.ID, CodingCreateWorktree: true}
	require.NoError(t, yakit.ResolveCodingSessionBinding(ctx, db, db, "owner", params))
	reuse := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.ID, CodingWorktreeID: params.CodingWorktreeID}
	require.NoError(t, yakit.ResolveCodingSessionBinding(ctx, db, db, "linked", reuse))
	list, err := server.ListAICodingWorktrees(ctx, &ypb.AICodingWorkspaceRequest{WorkspaceID: workspace.ID})
	require.NoError(t, err)
	require.Len(t, list.Data, 1)
	tree := list.Data[0]
	require.Equal(t, "ready", tree.State)
	require.Equal(t, "owner", tree.OwnerSessionID)
	require.Equal(t, []string{"owner", "linked"}, tree.SessionIDs)
	require.Equal(t, workspace.ID, tree.WorkspaceID)
	require.True(t, tree.Detached)
	require.NotEmpty(t, tree.Head)
	lookup, err := server.GetAICodingWorktree(ctx, &ypb.GetAICodingWorktreeRequest{Path: filepath.Join(tree.Path, "file-not-yet-created")})
	require.NoError(t, err)
	require.Equal(t, tree.ID, lookup.ID)
	_, err = server.GetAICodingWorktree(ctx, &ypb.GetAICodingWorktreeRequest{WorkspaceID: "another-workspace", WorktreeID: tree.ID})
	require.Error(t, err)
	_, err = server.GetAICodingWorktree(ctx, &ypb.GetAICodingWorktreeRequest{WorkspaceID: "another-workspace", Path: tree.Path})
	require.Error(t, err)
	_, err = server.DeleteAICodingWorktree(ctx, &ypb.DeleteAICodingWorktreeRequest{WorkspaceID: workspace.ID, WorktreeID: tree.ID})
	require.ErrorContains(t, err, "associated sessions first")
	sessions, err := server.ListAICodingSessions(ctx, &ypb.AICodingWorkspaceRequest{WorkspaceID: workspace.ID})
	require.NoError(t, err)
	require.Len(t, sessions.Data, 2)
	_, err = server.UpdateAICodingWorkspace(ctx, &ypb.UpdateAICodingWorkspaceRequest{WorkspaceID: workspace.ID, Name: "updated", RootPaths: []string{workspace.RootPaths[1], workspace.RootPaths[0]}})
	require.NoError(t, err)
	updated, err := server.GetAICodingWorkspace(ctx, &ypb.AICodingWorkspaceRequest{WorkspaceID: workspace.ID})
	require.NoError(t, err)
	require.Equal(t, workspace.RootPaths[1], updated.RootPath)
	out, err := exec.Command("git", "-C", root, "worktree", "remove", tree.Path).CombinedOutput()
	require.NoError(t, err, string(out))
	lookup, err = server.GetAICodingWorktree(ctx, &ypb.GetAICodingWorktreeRequest{WorktreeID: tree.ID})
	require.NoError(t, err)
	require.Equal(t, "unavailable", lookup.State)
	require.NotEmpty(t, lookup.LastError)
	require.Equal(t, workspace.ID, lookup.WorkspaceID, "ownership survives a missing checkout")
	// An orphan with only a stale Git registration can be explicitly cleaned up.
	require.NoError(t, db.Unscoped().Where("worktree_id = ?", tree.ID).Delete(&schema.AICodingSession{}).Error)
	_, err = server.DeleteAICodingWorktree(ctx, &ypb.DeleteAICodingWorktreeRequest{WorkspaceID: workspace.ID, WorktreeID: tree.ID})
	require.NoError(t, err)
	_, err = server.GetAICodingWorktree(ctx, &ypb.GetAICodingWorktreeRequest{WorktreeID: tree.ID})
	require.Error(t, err)
}

func TestCodingSessionDeletionRemovesExclusiveWorktrees(t *testing.T) {
	for _, deleteAll := range []bool{false, true} {
		t.Run(fmt.Sprint(deleteAll), func(t *testing.T) {
			t.Setenv("YAKIT_HOME", t.TempDir())
			profile, err := utils.CreateTempTestDatabaseInMemory()
			require.NoError(t, err)
			defer profile.Close()
			project, err := utils.CreateTempTestDatabaseInMemory()
			require.NoError(t, err)
			defer project.Close()
			require.NoError(t, profile.AutoMigrate(&schema.AICodingWorkspace{}, &schema.AICodingSession{}, &schema.AICodingWorktree{}).Error)
			require.NoError(t, project.AutoMigrate(&schema.AISession{}, &schema.AIAgentRuntime{}, &schema.AiCheckpoint{}, &schema.AiOutputEvent{}, &schema.AiProcessAndAiEvent{}).Error)
			root := t.TempDir()
			for _, args := range [][]string{{"init"}, {"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"}} {
				out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
				require.NoError(t, err, string(out))
			}
			server := &Server{profileDatabase: profile, projectDatabase: project}
			workspace, err := server.CreateAICodingWorkspace(context.Background(), &ypb.CreateAICodingWorkspaceRequest{Name: "project", RootPaths: []string{root}})
			require.NoError(t, err)
			params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.ID, CodingCreateWorktree: true}
			require.NoError(t, yakit.ResolveCodingSessionBinding(context.Background(), profile, project, "deleted-chat", params))
			_, err = yakit.CreateOrUpdateAISessionMeta(project, "deleted-chat", "Preserved chat title")
			require.NoError(t, err)
			require.NoError(t, profile.Create(&schema.AICodingSession{SessionID: "other-project-chat", WorkspaceID: workspace.ID}).Error)
			marker := filepath.Join(params.CodingWorkingDirectory, "uncommitted.txt")
			require.NoError(t, os.WriteFile(marker, []byte("keep this work"), 0600))
			deleted, err := server.DeleteAISession(context.Background(), &ypb.DeleteAISessionRequest{DeleteAll: deleteAll, Filter: &ypb.DeleteAISessionFilter{SessionID: []string{"deleted-chat"}}})
			require.NoError(t, err)
			sessions, err := server.ListAICodingSessions(context.Background(), &ypb.AICodingWorkspaceRequest{WorkspaceID: workspace.ID})
			require.NoError(t, err)
			require.Len(t, sessions.Data, 1)
			require.Equal(t, "other-project-chat", sessions.Data[0].SessionID)
			trees, err := server.ListAICodingWorktrees(context.Background(), &ypb.AICodingWorkspaceRequest{WorkspaceID: workspace.ID})
			require.NoError(t, err)
			require.Empty(t, trees.Data)
			require.NoDirExists(t, params.CodingWorkingDirectory)
			require.DirExists(t, root)
			require.Contains(t, deleted.ExtraMessage, "deleted_coding_sessions=1")
			require.Contains(t, deleted.ExtraMessage, "deleted_coding_worktrees=1")
			var count int
			require.NoError(t, profile.Unscoped().Model(&schema.AICodingSession{}).Where("session_id = ?", "deleted-chat").Count(&count).Error)
			require.Zero(t, count, "hard delete must not leave a soft-deleted binding")
			require.NoError(t, profile.Unscoped().Model(&schema.AICodingWorktree{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}
