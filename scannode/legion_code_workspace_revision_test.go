package scannode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssagitworkdir"
)

func TestLegionCodeWorkspaceGitRestoresPinnedRevisionAfterBranchMoves(t *testing.T) {
	managedRoot := t.TempDir()
	t.Setenv(ssagitworkdir.WorkDirEnv, managedRoot)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")
	source, originalRevision := createLocalGitRepository(t)
	repository, err := git.PlainOpen(source)
	if err != nil {
		t.Fatal(err)
	}
	config, err := repository.Config()
	if err != nil {
		t.Fatal(err)
	}
	config.Raw.Section("uploadpack").SetOption("allowReachableSHA1InWant", "true")
	if err := repository.SetConfig(config); err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "main.go"), []byte("package main\n// changed after the original run\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("main.go"); err != nil {
		t.Fatal(err)
	}
	latestRevision, err := worktree.Commit("move branch", &git.CommitOptions{Author: &object.Signature{
		Name: "Test", Email: "test@example.invalid", When: time.Unix(2, 0),
	}})
	if err != nil {
		t.Fatal(err)
	}
	originalClone := cloneLegionCodeGitWorkspace
	cloneLegionCodeGitWorkspace = func(_ string, local string, _ ...yakgit.Option) error {
		cloned, err := git.PlainClone(local, false, &git.CloneOptions{URL: source, Depth: 1})
		if err != nil {
			return err
		}
		head, err := cloned.Head()
		if err != nil || head.Hash() != latestRevision {
			t.Fatalf("fixture did not clone the moved branch: %v", err)
		}
		if _, err := cloned.CommitObject(plumbing.NewHash(originalRevision)); !errors.Is(err, plumbing.ErrObjectNotFound) {
			t.Fatalf("fixture must require fetching the historical commit, got %v", err)
		}
		return nil
	}
	t.Cleanup(func() { cloneLegionCodeGitWorkspace = originalClone })
	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	spec.ExpectedRevision = originalRevision
	workspace, err := materializeLegionCodeGitWorkspace(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.Cleanup() })
	content, err := os.ReadFile(filepath.Join(workspace.root, "main.go"))
	if err != nil || string(content) != "package main\n" || workspace.lockedRevision != originalRevision {
		t.Fatalf("retry did not recover the original input: revision=%s content=%q err=%v", workspace.lockedRevision, content, err)
	}
	if err := workspace.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace.root); !os.IsNotExist(err) {
		t.Fatalf("pinned workspace was not cleaned: %v", err)
	}
}

func TestLegionCodeWorkspaceGitRevisionRejectsSymbolicInvalidAndCancelledPins(t *testing.T) {
	for _, revision := range []string{"HEAD", "main", "HEAD~1", strings.Repeat("0", 40), strings.Repeat("z", 40), "1234567"} {
		if err := restoreLegionCodeGitRevision(context.Background(), t.TempDir(), revision); err == nil || !strings.Contains(err.Error(), "revision mismatch") {
			t.Fatalf("accepted invalid revision %q: %v", revision, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := restoreLegionCodeGitRevision(ctx, t.TempDir(), strings.Repeat("a", 40)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled restoration lost cancellation: %v", err)
	}
}
