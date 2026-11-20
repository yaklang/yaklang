package coding

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodingBranches(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		out, err := git(ctx, root, args...)
		require.NoError(t, err)
		return string(out)
	}
	run("init", "-b", "main")
	unborn, err := ListBranches(ctx, root)
	require.NoError(t, err)
	require.Equal(t, "main", unborn.CurrentBranch)
	require.Empty(t, unborn.CurrentCommit)
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial")
	run("branch", "feature/中文")
	run("remote", "add", "origin", "https://example.invalid/repo.git")
	run("update-ref", "refs/remotes/origin/main", "HEAD")
	run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	run("branch", "--set-upstream-to=origin/main", "main")
	require.NoError(t, os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("preserve"), 0600))
	before := run("status", "--porcelain=v1", "-z")
	branches, err := ListBranches(ctx, root)
	require.NoError(t, err)
	require.Len(t, branches.Data, 3, "symbolic origin/HEAD must not be a duplicate option")
	require.Equal(t, "refs/remotes/origin/main", branches.DefaultRef)
	require.Equal(t, "main", branches.CurrentBranch)
	require.NotEmpty(t, branches.CurrentCommit)
	require.True(t, branches.HasLocalChanges)
	require.Contains(t, branches.Data, Branch{Name: "origin/main", Ref: "refs/remotes/origin/main", Commit: branches.CurrentCommit, Remote: true})
	require.Equal(t, before, run("status", "--porcelain=v1", "-z"))
	run("checkout", "--detach", "HEAD")
	detached, err := ListBranches(ctx, root)
	require.NoError(t, err)
	require.Empty(t, detached.CurrentBranch)
	require.Equal(t, branches.CurrentCommit, detached.CurrentCommit)
	require.Equal(t, "refs/remotes/origin/main", detached.DefaultRef)
	run("update-ref", "-d", "refs/remotes/origin/HEAD")
	fallback, err := ListBranches(ctx, root)
	require.NoError(t, err)
	require.Equal(t, "HEAD", fallback.DefaultRef)
}
