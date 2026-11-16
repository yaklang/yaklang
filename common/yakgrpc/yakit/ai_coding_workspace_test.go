package yakit

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/coding"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodingWorkspaceSessionBinding(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AICodingWorkspace{}, &schema.AICodingSession{}, &schema.AICodingWorktree{}, &schema.AISession{}).Error)
	ctx := context.Background()
	workspace, err := CreateAICodingWorkspace(ctx, db, "Example", t.TempDir())
	require.NoError(t, err)
	same, err := CreateAICodingWorkspace(ctx, db, "Duplicate", workspace.RootPath)
	require.NoError(t, err)
	require.NotEqual(t, workspace.WorkspaceID, same.WorkspaceID)
	params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID}
	require.NoError(t, ResolveCodingSessionBinding(ctx, db, db, "coding", params))
	require.Equal(t, workspace.RootPath, params.CodingWorkingDirectory)
	_, err = CreateOrUpdateAISessionMetaOnStart(db, "coding", params, time.Now())
	require.NoError(t, err)
	reconnect := &ypb.AIStartParams{CodingWorkingDirectory: t.TempDir()}
	require.NoError(t, ResolveCodingSessionBinding(ctx, db, db, "coding", reconnect))
	require.Equal(t, ypb.AgentModeCoding, reconnect.AgentMode)
	require.Equal(t, workspace.RootPath, reconnect.CodingWorkingDirectory)
	invalid := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID, CodingWorkingDirectory: t.TempDir()}
	require.Error(t, ResolveCodingSessionBinding(ctx, db, db, "new", invalid))
	ordinary := &ypb.AIStartParams{CodingWorkspaceID: workspace.WorkspaceID, CodingWorkingDirectory: workspace.RootPath}
	require.NoError(t, ResolveCodingSessionBinding(ctx, db, db, "ordinary", ordinary))
	require.Empty(t, ordinary.CodingWorkspaceID)
	require.Empty(t, ordinary.CodingWorkingDirectory)
}

func codingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AICodingWorkspace{}, &schema.AICodingSession{}, &schema.AICodingWorktree{}, &schema.AISession{}).Error)
	return db
}

func codingTestGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "GIT_") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	return strings.TrimSpace(string(output))
}

func codingTestRepo(t *testing.T) string {
	t.Helper()
	root, err := coding.Directory(t.TempDir())
	require.NoError(t, err)
	codingTestGit(t, root, "init")
	require.NoError(t, os.Mkdir(filepath.Join(root, "app"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "app", "source.txt"), []byte("first"), 0600))
	codingTestGit(t, root, "add", ".")
	codingTestGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	return root
}

func TestCodingMultiDirectorySessionLifecycle(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	ctx := context.Background()
	profile, project := codingTestDB(t), codingTestDB(t)
	front, back := codingTestRepo(t), codingTestRepo(t)
	primary := filepath.Join(front, "app")
	workspace, err := CreateAICodingWorkspace(ctx, profile, "full stack", primary, back)
	require.NoError(t, err)
	require.Equal(t, primary, workspace.RootPath, "primary subdirectory must not become repo root")
	require.Equal(t, schema.StringSlice{primary, back}, workspace.RootPaths)
	plain := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID}
	require.NoError(t, ResolveCodingSessionBinding(ctx, profile, project, "plain", plain))
	require.Equal(t, []string{primary, back}, plain.CodingWorkspaceRoots)
	require.Empty(t, plain.CodingWorktreeID)

	// A user-created worktree in this repository is not a managed workspace item.
	external := filepath.Join(t.TempDir(), "external")
	codingTestGit(t, front, "worktree", "add", "--detach", external)
	base := codingTestGit(t, front, "rev-parse", "HEAD")
	params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID, CodingCreateWorktree: true,
		CodingWorktreeBaseRef: base, CodingWorkspaceRoots: []string{"/untrusted"}}
	require.NoError(t, ResolveCodingSessionBinding(ctx, profile, project, "isolated", params))
	require.Equal(t, []string{primary, back}, params.CodingSourceRoots)
	require.Equal(t, back, params.CodingWorkspaceRoots[1])
	require.NotEqual(t, primary, params.CodingWorkingDirectory)
	require.Equal(t, "app", filepath.Base(params.CodingWorkingDirectory))
	require.False(t, params.CodingCreateWorktree)
	trees, err := ListAICodingWorktrees(profile, workspace.WorkspaceID)
	require.NoError(t, err)
	require.Len(t, trees, 1)
	tree := trees[0]
	require.Equal(t, "isolated", tree.OwnerSessionID)
	require.Equal(t, "ready", tree.State)
	require.Equal(t, base, tree.BaseCommit)
	require.Equal(t, base, codingTestGit(t, tree.Path, "rev-parse", "HEAD"))
	backTrees, err := coding.ListWorktrees(ctx, back, true)
	require.NoError(t, err)
	require.Len(t, backTrees, 1, "secondary repository must not get a worktree")
	require.NoError(t, os.WriteFile(filepath.Join(params.CodingWorkingDirectory, "source.txt"), []byte("isolated"), 0600))
	original, err := os.ReadFile(filepath.Join(primary, "source.txt"))
	require.NoError(t, err)
	require.Equal(t, "first", string(original))
	lookup, err := GetAICodingWorktree(profile, "", params.CodingWorkingDirectory)
	require.NoError(t, err)
	require.Equal(t, workspace.WorkspaceID, lookup.WorkspaceID)
	require.Equal(t, front, lookup.RepositoryRoot)

	// Reusing a checkout records another session, not another checkout.
	reuse := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID, CodingWorktreeID: tree.WorktreeID}
	require.NoError(t, ResolveCodingSessionBinding(ctx, profile, project, "reuse", reuse))
	require.Equal(t, params.CodingWorkingDirectory, reuse.CodingWorkingDirectory)
	other, err := CreateAICodingWorkspace(ctx, profile, "other", front)
	require.NoError(t, err)
	require.Error(t, ResolveCodingSessionBinding(ctx, profile, project, "foreign", &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding,
		CodingWorkspaceID: other.WorkspaceID, CodingWorktreeID: tree.WorktreeID}))

	_, err = UpdateAICodingWorkspace(ctx, profile, workspace.WorkspaceID, "reordered", []string{back, primary})
	require.NoError(t, err)
	reconnect := &ypb.AIStartParams{AgentMode: ypb.AgentModeSecurity, CodingWorkspaceID: other.WorkspaceID, CodingCreateWorktree: true}
	require.NoError(t, ResolveCodingSessionBinding(ctx, profile, project, "isolated", reconnect))
	require.Equal(t, params.CodingWorkspaceRoots, reconnect.CodingWorkspaceRoots)
	require.Equal(t, tree.WorktreeID, reconnect.CodingWorktreeID)
	require.NoError(t, ResolveCodingSessionBinding(ctx, profile, project, "plain", plain))
	require.Equal(t, []string{primary, back}, plain.CodingWorkspaceRoots)
	trees, err = ListAICodingWorktrees(profile, workspace.WorkspaceID)
	require.NoError(t, err)
	require.Len(t, trees, 1)
	sessions, err := ListAICodingSessions(profile, workspace.WorkspaceID)
	require.NoError(t, err)
	require.Len(t, sessions, 3)

	// A missing ready checkout must not silently resurrect an empty replacement.
	codingTestGit(t, front, "worktree", "remove", "--force", tree.Path)
	require.ErrorContains(t, ResolveCodingSessionBinding(ctx, profile, project, "isolated", reconnect), "unavailable")
	require.NoDirExists(t, tree.Path)
}

func TestCodingWorktreeFailedSetupRetriesSameReservation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("YAKIT_HOME", home)
	ctx, db := context.Background(), codingTestDB(t)
	repo := codingTestRepo(t)
	workspace, err := CreateAICodingWorkspace(ctx, db, "retry", repo)
	require.NoError(t, err)
	// Prevent directory creation after the binding is persisted.
	block := filepath.Join(home, "coding-worktrees")
	require.NoError(t, os.WriteFile(block, []byte("blocked"), 0600))
	params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID, CodingCreateWorktree: true}
	require.Error(t, ResolveCodingSessionBinding(ctx, db, db, "retry", params))
	trees, err := ListAICodingWorktrees(db, workspace.WorkspaceID)
	require.NoError(t, err)
	require.Len(t, trees, 1)
	require.Equal(t, "failed", trees[0].State)
	require.NotEmpty(t, trees[0].LastError)
	originalCommit := trees[0].BaseCommit
	require.NoError(t, os.Remove(block))
	codingTestGit(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "moved HEAD")
	retry := &ypb.AIStartParams{}
	require.NoError(t, ResolveCodingSessionBinding(ctx, db, db, "retry", retry))
	require.Equal(t, trees[0].WorktreeID, retry.CodingWorktreeID)
	require.Equal(t, originalCommit, codingTestGit(t, retry.CodingWorkingDirectory, "rev-parse", "HEAD"))
	trees, err = ListAICodingWorktrees(db, workspace.WorkspaceID)
	require.NoError(t, err)
	require.Len(t, trees, 1)
	require.Equal(t, "ready", trees[0].State)
	require.Empty(t, trees[0].LastError)
}

func TestCodingWorkspaceValidation(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	ctx, db := context.Background(), codingTestDB(t)
	root, repo := t.TempDir(), codingTestRepo(t)
	_, err := CreateAICodingWorkspace(ctx, db, "none")
	require.Error(t, err)
	_, err = CreateAICodingWorkspace(ctx, db, "duplicate", root, root)
	require.ErrorContains(t, err, "duplicate")
	workspace, err := CreateAICodingWorkspace(ctx, db, "plain primary", root, repo)
	require.NoError(t, err)
	require.False(t, workspace.IsGit)
	require.ErrorContains(t, ResolveCodingSessionBinding(ctx, db, db, "no-fallback", &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding,
		CodingWorkspaceID: workspace.WorkspaceID, CodingCreateWorktree: true}), "primary directory is not a Git")
	workspace, err = CreateAICodingWorkspace(ctx, db, "git", repo)
	require.NoError(t, err)
	require.Error(t, ResolveCodingSessionBinding(ctx, db, db, "bad-ref", &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding,
		CodingWorkspaceID: workspace.WorkspaceID, CodingCreateWorktree: true, CodingWorktreeBaseRef: "--help"}))
	trees, err := ListAICodingWorktrees(db, workspace.WorkspaceID)
	require.NoError(t, err)
	require.Empty(t, trees)
}
