package ssaapi

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssagitworkdir"
)

func TestGitFSRemovesWorkspaceWhenCloneFails(t *testing.T) {
	root := t.TempDir()
	t.Setenv(ssagitworkdir.WorkDirEnv, root)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")
	originalClone := cloneSSAGitRepository
	cloneSSAGitRepository = func(_, _ string, _ ...yakgit.Option) error {
		return errors.New("no space left on device")
	}
	t.Cleanup(func() { cloneSSAGitRepository = originalClone })

	config := newGitSourceConfigForTest(t)
	_, err := gitFs(config)
	require.ErrorContains(t, err, "no space left on device")
	require.ErrorContains(t, err, "workspace=")
	require.ErrorContains(t, err, "available_bytes=")
	require.ErrorContains(t, err, ssagitworkdir.WorkDirEnv)
	require.Empty(t, readDirectoryNames(t, root))
}

func TestGitFSCleanupRemovesSuccessfulCloneWorkspace(t *testing.T) {
	root := t.TempDir()
	t.Setenv(ssagitworkdir.WorkDirEnv, root)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")
	originalClone := cloneSSAGitRepository
	cloneSSAGitRepository = func(_, local string, _ ...yakgit.Option) error {
		return os.WriteFile(filepath.Join(local, "main.yak"), []byte("println(1)"), 0600)
	}
	t.Cleanup(func() { cloneSSAGitRepository = originalClone })

	config := newGitSourceConfigForTest(t)
	_, err := gitFs(config)
	require.NoError(t, err)
	require.Len(t, readDirectoryNames(t, root), 1)

	config.Cleanup()
	require.Empty(t, readDirectoryNames(t, root))
}

func TestGitSourceLocalRepositoryEndToEnd(t *testing.T) {
	repositoryRoot := t.TempDir()
	repository, err := git.PlainInit(repositoryRoot, false)
	require.NoError(t, err)
	worktree, err := repository.Worktree()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(repositoryRoot, "main.yak"),
		[]byte("value = 40 + 2\nprintln(value)\n"),
		0600,
	))
	_, err = worktree.Add("main.yak")
	require.NoError(t, err)
	_, err = worktree.Commit("add Yak fixture", &git.CommitOptions{Author: &object.Signature{
		Name:  "INT-8 test",
		Email: "int-8@example.invalid",
		When:  time.Unix(1, 0),
	}})
	require.NoError(t, err)

	managedRoot := filepath.Join(t.TempDir(), "managed-git-workspaces")
	t.Setenv(ssagitworkdir.WorkDirEnv, managedRoot)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")

	programs, err := ParseProject(
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceGit),
		ssaconfig.WithCodeSourceURL(repositoryRoot),
		WithLanguage(ssaconfig.Yak),
	)
	require.NoError(t, err)
	require.NotEmpty(t, programs)
	require.Empty(t, readDirectoryNames(t, managedRoot))
}

func newGitSourceConfigForTest(t *testing.T) *Config {
	t.Helper()
	config, err := ssaconfig.New(
		ssaconfig.ModeSSACompile,
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceGit),
		ssaconfig.WithCodeSourceURL("https://example.invalid/repository.git"),
		ssaconfig.WithCodeSourceBranch("main"),
	)
	require.NoError(t, err)
	return &Config{Config: config}
}

func readDirectoryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
