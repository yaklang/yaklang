package ssaapi

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
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

func TestGitFSRequestsShallowSingleBranchClone(t *testing.T) {
	root := t.TempDir()
	t.Setenv(ssagitworkdir.WorkDirEnv, root)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")

	var captured []yakgit.Option
	originalClone := cloneSSAGitRepository
	cloneSSAGitRepository = func(_, local string, opts ...yakgit.Option) error {
		captured = append([]yakgit.Option(nil), opts...)
		return os.WriteFile(filepath.Join(local, "main.yak"), []byte("println(1)"), 0600)
	}
	t.Cleanup(func() { cloneSSAGitRepository = originalClone })

	config := newGitSourceConfigForTest(t)
	_, err := gitFs(config)
	require.NoError(t, err)

	settings := yakgit.InspectCloneSettings(captured...)
	require.Equal(t, 1, settings.Depth)
	require.True(t, settings.SingleBranch)
	require.True(t, settings.NoFetchTags)
	require.Equal(t, "main", settings.Branch)
	require.False(t, settings.RecursiveSubmodule)
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
	repositoryRoot := createGitSourceRepository(t)

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

func TestParseFromReaderCleansGitWorkspace(t *testing.T) {
	repositoryRoot := createGitSourceRepository(t)
	managedRoot := filepath.Join(t.TempDir(), "managed-git-workspaces")
	t.Setenv(ssagitworkdir.WorkDirEnv, managedRoot)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")

	program, err := ParseFromReader(
		strings.NewReader("println(42)\n"),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceGit),
		ssaconfig.WithCodeSourceURL(repositoryRoot),
		WithLanguage(ssaconfig.Yak),
	)
	require.NoError(t, err)
	require.NotNil(t, program)
	require.Empty(t, readDirectoryNames(t, managedRoot))
}

func TestIncrementalGitFallbackUsesIndependentCleanup(t *testing.T) {
	repositoryRoot := createGitSourceRepository(t)
	managedRoot := filepath.Join(t.TempDir(), "managed-git-workspaces")
	t.Setenv(ssagitworkdir.WorkDirEnv, managedRoot)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")

	rawConfig, err := ssaconfig.New(
		ssaconfig.ModeSSACompile,
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceGit),
		ssaconfig.WithCodeSourceURL(repositoryRoot),
	)
	require.NoError(t, err)
	cachedConfig := &Config{Config: rawConfig}
	baseProgram := &Program{config: cachedConfig}

	type cloneResult struct {
		cleanup func()
		err     error
	}
	results := make(chan cloneResult, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, cleanup, rebuildErr := rebuildBaseFileSystemFromConfig(baseProgram)
			results <- cloneResult{cleanup: cleanup, err: rebuildErr}
		}()
	}
	group.Wait()
	close(results)

	cleanups := make([]func(), 0, 2)
	for result := range results {
		require.NoError(t, result.err)
		cleanups = append(cleanups, result.cleanup)
	}
	require.Len(t, cleanups, 2)
	require.Len(t, readDirectoryNames(t, managedRoot), 2)

	// The ProgramCache-owned config must never receive either clone's cleanup.
	cachedConfig.Cleanup()
	require.Len(t, readDirectoryNames(t, managedRoot), 2)
	cleanups[0]()
	require.Len(t, readDirectoryNames(t, managedRoot), 1)
	cleanups[1]()
	require.Empty(t, readDirectoryNames(t, managedRoot))
}

func TestIncrementalCompileEntryCleansGitFallbackWorkspace(t *testing.T) {
	repositoryRoot := createGitSourceRepository(t)
	managedRoot := filepath.Join(t.TempDir(), "managed-git-workspaces")
	t.Setenv(ssagitworkdir.WorkDirEnv, managedRoot)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")

	baseProgramName := "git-fallback-base-" + uuid.NewString()
	diffProgramName := "git-fallback-diff-" + uuid.NewString()
	t.Cleanup(func() {
		ProgramCache.Remove(baseProgramName)
		ProgramCache.Remove(diffProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), baseProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), diffProgramName)
	})
	baseProgram, err := ParseFromReader(
		strings.NewReader("println(40 + 2)\n"),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceGit),
		ssaconfig.WithCodeSourceURL(repositoryRoot),
		WithLanguage(ssaconfig.Yak),
		WithProgramName(baseProgramName),
	)
	require.NoError(t, err)
	require.NotNil(t, baseProgram)
	SetProgramCache(baseProgram)
	require.Empty(t, readDirectoryNames(t, managedRoot))

	originalBuild := buildFileSystemFromProgramNameForIncremental
	buildFileSystemFromProgramNameForIncremental = func(string) (fi.FileSystem, error) {
		return nil, errors.New("force Git config fallback")
	}
	t.Cleanup(func() { buildFileSystemFromProgramNameForIncremental = originalBuild })

	updatedFS := filesys.NewVirtualFs()
	updatedFS.AddFile("main.yak", "println(43)\n")
	programs, err := ParseProjectWithIncrementalCompile(
		updatedFS,
		baseProgramName,
		diffProgramName,
		ssaconfig.Yak,
		WithEnableIncrementalCompile(true),
	)
	require.NoError(t, err)
	require.NotEmpty(t, programs)
	require.Empty(t, readDirectoryNames(t, managedRoot))
}

func createGitSourceRepository(t *testing.T) string {
	t.Helper()
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
		Name:  "Git source test",
		Email: "git-source@example.invalid",
		When:  time.Unix(1, 0),
	}})
	require.NoError(t, err)
	return repositoryRoot
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
