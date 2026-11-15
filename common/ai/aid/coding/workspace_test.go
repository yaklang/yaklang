package coding

import (
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestCodingWorkspaceWorktrees(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	_, err := git(ctx, root, "init")
	require.NoError(t, err)
	_, err = git(ctx, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial")
	require.NoError(t, err)
	canonical, isGit, err := Source(ctx, root)
	require.NoError(t, err)
	require.True(t, isGit)
	destination := filepath.Join(t.TempDir(), "中文 worktree")
	require.NoError(t, CreateWorktree(ctx, canonical, destination, "HEAD"))
	destination, err = Directory(destination)
	require.NoError(t, err)
	trees, err := ListWorktrees(ctx, canonical, true)
	require.NoError(t, err)
	require.Len(t, trees, 2)
	repo, err := InspectRepository(ctx, canonical)
	require.NoError(t, err)
	selected, err := InspectWorktree(ctx, destination, repo.CommonDir, destination)
	require.NoError(t, err)
	require.True(t, selected.Detached)
	_, err = InspectWorktree(ctx, destination, t.TempDir(), destination)
	require.ErrorContains(t, err, "identity changed")
	require.Error(t, CreateWorktree(ctx, canonical, destination, "HEAD"))
	require.NoError(t, os.WriteFile(filepath.Join(destination, "user-change.txt"), []byte("keep"), 0600))
	_, err = ListWorktrees(ctx, canonical, true)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(destination, "user-change.txt"))
	// Environment overrides must never substitute a different repository.
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), ".git"))
	trees, err = ListWorktrees(ctx, canonical, true)
	require.NoError(t, err)
	require.Len(t, trees, 2)
}

func TestCodingWorkspacePlainDirectoryAndInvalidPaths(t *testing.T) {
	root, isGit, err := Source(context.Background(), t.TempDir())
	require.NoError(t, err)
	require.False(t, isGit)
	require.NotEmpty(t, root)
	_, err = Directory("relative")
	require.Error(t, err)
	_, err = InspectRepository(context.Background(), root)
	require.ErrorContains(t, err, "not a Git")
}

func TestCodingWorktreePorcelain(t *testing.T) {
	trees := ParseWorktrees([]byte("worktree /a/path with\nnewline\x00HEAD abc\x00branch refs/heads/main\x00\x00worktree /b\x00HEAD def\x00detached\x00locked reason\x00prunable missing\x00\x00"))
	require.Len(t, trees, 2)
	require.Equal(t, "/a/path with\nnewline", trees[0].Path)
	require.Equal(t, "main", trees[0].Branch)
	require.True(t, trees[1].Detached)
	require.True(t, trees[1].Locked)
	require.True(t, trees[1].Prunable)
}

func TestCodingRejectsBareRepository(t *testing.T) {
	root := t.TempDir()
	_, err := git(context.Background(), root, "init", "--bare")
	require.NoError(t, err)
	_, _, err = Source(context.Background(), root)
	require.ErrorContains(t, err, "bare Git repository")
}
