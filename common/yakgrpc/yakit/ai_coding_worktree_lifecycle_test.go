package yakit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func managedCodingTestTree(t *testing.T) (*gorm.DB, *schema.AICodingWorktree) {
	t.Helper()
	t.Setenv("YAKIT_HOME", t.TempDir())
	db, root := codingTestDB(t), codingTestRepo(t)
	workspace, err := CreateAICodingWorkspace(context.Background(), db, "project", root)
	require.NoError(t, err)
	params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID, CodingCreateWorktree: true}
	require.NoError(t, ResolveCodingSessionBinding(context.Background(), db, db, "owner", params))
	tree, err := GetAICodingWorktree(db, workspace.WorkspaceID, params.CodingWorktreeID, "")
	require.NoError(t, err)
	return db, tree
}

func TestCodingWorktreeDefaultMatchesBranchPicker(t *testing.T) {
	for _, selected := range []string{"upstream", "remote-head", "explicit-head", "local-only"} {
		t.Run(selected, func(t *testing.T) {
			t.Setenv("YAKIT_HOME", t.TempDir())
			ctx, db, root := context.Background(), codingTestDB(t), codingTestRepo(t)
			base := codingTestGit(t, root, "rev-parse", "HEAD")
			if selected != "local-only" {
				codingTestGit(t, root, "remote", "add", "origin", "https://example.invalid/repo.git")
				codingTestGit(t, root, "update-ref", "refs/remotes/origin/main", base)
				codingTestGit(t, root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
			}
			codingTestGit(t, root, "checkout", "-b", "temporary")
			codingTestGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "local-only")
			localHead := codingTestGit(t, root, "rev-parse", "HEAD")
			ref := "refs/remotes/origin/main"
			if selected == "upstream" {
				ref = "refs/remotes/origin/release"
				codingTestGit(t, root, "update-ref", ref, base)
				codingTestGit(t, root, "branch", "--set-upstream-to=origin/release")
			}
			workspace, err := CreateAICodingWorkspace(ctx, db, "project", root)
			require.NoError(t, err)
			params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID, CodingCreateWorktree: true}
			if selected == "explicit-head" {
				params.CodingWorktreeBaseRef = "HEAD"
			}
			if selected == "explicit-head" || selected == "local-only" {
				ref, base = "HEAD", localHead
			}
			require.NoError(t, ResolveCodingSessionBinding(ctx, db, db, "owner", params))
			tree, err := GetAICodingWorktree(db, workspace.WorkspaceID, params.CodingWorktreeID, "")
			require.NoError(t, err)
			require.Equal(t, ref, tree.BaseRef)
			require.Equal(t, base, tree.BaseCommit)
			require.Equal(t, base, codingTestGit(t, tree.Path, "rev-parse", "HEAD"))
			require.Equal(t, localHead, codingTestGit(t, root, "rev-parse", "HEAD"))
		})
	}
}

func TestCodingWorktreeRecoversInterruptedCreation(t *testing.T) {
	for _, state := range []string{"empty-directory", "partial-directory", "missing-checkout", "missing-backlink", "missing-administration", "no-checkout", "complete"} {
		t.Run(state, func(t *testing.T) {
			db, tree := managedCodingTestTree(t)
			require.NoError(t, db.Model(tree).Update("state", "creating").Error)
			switch state {
			case "empty-directory", "partial-directory", "no-checkout":
				codingTestGit(t, tree.RepositoryRoot, "worktree", "remove", tree.Path)
				if state == "no-checkout" {
					codingTestGit(t, tree.RepositoryRoot, "worktree", "add", "--detach", "--no-checkout", tree.Path, tree.BaseCommit)
				} else {
					require.NoError(t, os.MkdirAll(tree.Path, 0700))
					if state == "partial-directory" {
						require.NoError(t, os.WriteFile(filepath.Join(tree.Path, "partial"), []byte("interrupted"), 0600))
					}
				}
			case "missing-checkout":
				require.NoError(t, os.RemoveAll(tree.Path))
			case "missing-backlink":
				require.NoError(t, os.Remove(filepath.Join(tree.Path, ".git")))
			case "missing-administration":
				admin := codingTestGit(t, tree.Path, "rev-parse", "--path-format=absolute", "--git-dir")
				require.NoError(t, os.RemoveAll(admin))
			case "complete":
				require.NoError(t, os.WriteFile(filepath.Join(tree.Path, "app", "source.txt"), []byte("preserve changes"), 0600))
			}
			codingTestGit(t, tree.RepositoryRoot, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "moved HEAD")
			retry := &ypb.AIStartParams{}
			require.NoError(t, ResolveCodingSessionBinding(context.Background(), db, db, "owner", retry))
			require.Equal(t, tree.WorktreeID, retry.CodingWorktreeID)
			require.Equal(t, tree.BaseCommit, codingTestGit(t, tree.Path, "rev-parse", "HEAD"))
			content, err := os.ReadFile(filepath.Join(tree.Path, "app", "source.txt"))
			require.NoError(t, err)
			if state == "complete" {
				require.Equal(t, "preserve changes", string(content))
			} else {
				require.Equal(t, "first", string(content))
			}
			updated, err := GetAICodingWorktree(db, tree.WorkspaceID, tree.WorktreeID, "")
			require.NoError(t, err)
			require.Equal(t, "ready", updated.State)
			require.Empty(t, updated.LastError)
		})
	}
}

func TestCodingWorktreeCleanupPreservesSharedCheckout(t *testing.T) {
	db, tree := managedCodingTestTree(t)
	ctx := context.Background()
	params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: tree.WorkspaceID, CodingWorktreeID: tree.WorktreeID}
	require.NoError(t, ResolveCodingSessionBinding(ctx, db, db, "other-project-session", params))
	// Existing profile databases can have NULL in a later-added tombstone field.
	require.NoError(t, db.Model(&schema.AICodingSession{}).Where("session_id = ?", "other-project-session").Update("session_deleted_at", nil).Error)
	marker := filepath.Join(tree.Path, "uncommitted.txt")
	require.NoError(t, os.WriteFile(marker, []byte("work"), 0600))
	deleteHistory := func() error { return nil }
	first, err := DeleteCodingSessions(ctx, db, []string{"owner"}, deleteHistory)
	require.NoError(t, err)
	require.EqualValues(t, 1, first.DeletedSessions)
	require.EqualValues(t, 0, first.DeletedWorktrees)
	require.EqualValues(t, 1, first.RetainedWorktrees)
	require.FileExists(t, marker)
	retained, err := GetAICodingWorktree(db, tree.WorkspaceID, tree.WorktreeID, "")
	require.NoError(t, err)
	require.Equal(t, "other-project-session", retained.OwnerSessionID)
	second, err := DeleteCodingSessions(ctx, db, []string{"other-project-session"}, deleteHistory)
	require.NoError(t, err)
	require.EqualValues(t, 1, second.DeletedWorktrees)
	require.NoDirExists(t, tree.Path)
	require.NotContains(t, codingTestGit(t, tree.RepositoryRoot, "worktree", "list", "--porcelain"), tree.Path)
}

func TestCodingWorktreeExplicitCleanup(t *testing.T) {
	ctx := context.Background()
	t.Run("live session is protected", func(t *testing.T) {
		db, tree := managedCodingTestTree(t)
		require.ErrorContains(t, DeleteAICodingWorktree(ctx, db, tree.WorkspaceID, tree.WorktreeID), "associated sessions first")
		require.DirExists(t, tree.Path)
	})
	t.Run("failed reservation without session history", func(t *testing.T) {
		db, tree := managedCodingTestTree(t)
		codingTestGit(t, tree.RepositoryRoot, "worktree", "remove", tree.Path)
		require.NoError(t, os.Mkdir(tree.Path, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(tree.Path, "partial"), []byte("incomplete"), 0600))
		require.NoError(t, db.Model(tree).Update("state", "failed").Error)
		require.NoError(t, DeleteAICodingWorktree(ctx, db, tree.WorkspaceID, tree.WorktreeID))
		require.NoDirExists(t, tree.Path)
		_, err := GetAICodingSession(db, "owner")
		require.True(t, gorm.IsRecordNotFoundError(err))
		require.NoError(t, DeleteAICodingWorktree(ctx, db, tree.WorkspaceID, tree.WorktreeID))
	})
	t.Run("orphaned ready checkout", func(t *testing.T) {
		db, tree := managedCodingTestTree(t)
		require.NoError(t, db.Unscoped().Where("session_id = ?", "owner").Delete(&schema.AICodingSession{}).Error)
		codingTestGit(t, tree.RepositoryRoot, "worktree", "lock", tree.Path)
		require.NoError(t, os.WriteFile(filepath.Join(tree.Path, "untracked"), []byte("delete explicitly"), 0600))
		require.NoError(t, DeleteAICodingWorktree(ctx, db, "other-workspace", tree.WorktreeID))
		require.DirExists(t, tree.Path)
		require.NoError(t, DeleteAICodingWorktree(ctx, db, tree.WorkspaceID, tree.WorktreeID))
		require.NoDirExists(t, tree.Path)
	})
	t.Run("finish interrupted deletion", func(t *testing.T) {
		db, tree := managedCodingTestTree(t)
		require.NoError(t, db.Model(tree).Update("state", "deleting").Error)
		require.NoError(t, os.RemoveAll(tree.Path))
		require.NoError(t, DeleteAICodingWorktree(ctx, db, tree.WorkspaceID, tree.WorktreeID))
		_, err := GetAICodingSession(db, "owner")
		require.True(t, gorm.IsRecordNotFoundError(err))
		require.NotContains(t, codingTestGit(t, tree.RepositoryRoot, "worktree", "list", "--porcelain"), tree.Path)
	})
}

func TestCodingSourceSessionDeletionNeverRemovesSource(t *testing.T) {
	db, root, ctx := codingTestDB(t), codingTestRepo(t), context.Background()
	workspace, err := CreateAICodingWorkspace(ctx, db, "source", root)
	require.NoError(t, err)
	params := &ypb.AIStartParams{AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID}
	require.NoError(t, ResolveCodingSessionBinding(ctx, db, db, "source-session", params))
	result, err := DeleteCodingSessions(ctx, db, []string{"source-session"}, func() error { return nil })
	require.NoError(t, err)
	require.EqualValues(t, 1, result.DeletedSessions)
	require.Zero(t, result.DeletedWorktrees)
	require.FileExists(t, filepath.Join(root, "app", "source.txt"))
}

func TestCodingWorktreeDeleteRetryAfterHistoryFailure(t *testing.T) {
	db, tree := managedCodingTestTree(t)
	ctx := context.Background()
	_, err := DeleteCodingSessions(ctx, db, []string{"owner"}, func() error { return errors.New("history failure") })
	require.ErrorContains(t, err, "history failure")
	require.NoDirExists(t, tree.Path)
	require.ErrorContains(t, ResolveCodingSessionBinding(ctx, db, db, "owner", &ypb.AIStartParams{}), "deletion is in progress")
	result, err := DeleteCodingSessions(ctx, db, []string{"owner"}, func() error { return nil })
	require.NoError(t, err)
	require.EqualValues(t, 1, result.DeletedWorktrees)
	_, err = GetAICodingSession(db, "owner")
	require.True(t, gorm.IsRecordNotFoundError(err))
}

func TestCodingWorktreeLookupScopeAndReverseLookup(t *testing.T) {
	db, tree := managedCodingTestTree(t)
	for _, path := range []string{tree.Path, filepath.Join(tree.Path, "app", "not-yet-created.txt")} {
		for _, scope := range []string{"", tree.WorkspaceID} {
			found, err := GetAICodingWorktree(db, scope, "", path)
			require.NoError(t, err)
			require.Equal(t, tree.WorktreeID, found.WorktreeID)
		}
		_, err := GetAICodingWorktree(db, "another-workspace", "", path)
		require.True(t, gorm.IsRecordNotFoundError(err))
	}
	_, err := GetAICodingWorktree(db, "another-workspace", tree.WorktreeID, "")
	require.True(t, gorm.IsRecordNotFoundError(err))
	_, err = GetAICodingWorktree(db, "", "", tree.Path+"-sibling")
	require.True(t, gorm.IsRecordNotFoundError(err))
}

func TestCodingWorktreeCleanupRejectsReplacedPaths(t *testing.T) {
	for _, replacement := range []string{"foreign-repository", "symlink-bucket"} {
		t.Run(replacement, func(t *testing.T) {
			db, tree := managedCodingTestTree(t)
			codingTestGit(t, tree.RepositoryRoot, "worktree", "remove", tree.Path)
			foreign := codingTestRepo(t)
			if replacement == "foreign-repository" {
				require.NoError(t, os.Rename(foreign, tree.Path))
				foreign = tree.Path
			} else {
				require.NoError(t, os.Remove(filepath.Dir(tree.Path)))
				require.NoError(t, os.Symlink(filepath.Dir(foreign), filepath.Dir(tree.Path)))
			}
			called := false
			_, err := DeleteCodingSessions(context.Background(), db, []string{"owner"}, func() error { called = true; return nil })
			require.Error(t, err)
			require.False(t, called)
			require.FileExists(t, filepath.Join(foreign, "app", "source.txt"))
			_, err = GetAICodingSession(db, "owner")
			require.NoError(t, err)
		})
	}
}
