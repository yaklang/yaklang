package ssagitworkdir

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

const ownerScopeLockHelperEnv = "YAK_TEST_SSA_GIT_OWNER_SCOPE_LOCK_HELPER"
const ownerScopeActivityHelperEnv = "YAK_TEST_SSA_GIT_OWNER_SCOPE_ACTIVITY_HELPER"

func TestOwnerScopeLockIsExclusiveAcrossProcesses(t *testing.T) {
	if os.Getenv(ownerScopeLockHelperEnv) == "1" {
		lock, err := AcquireOwnerScopeLock("node-shared")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Println("ready")
		_, _ = bufio.NewReader(os.Stdin).ReadByte()
		if err := lock.Release(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
		return
	}

	root := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestOwnerScopeLockIsExclusiveAcrossProcesses$")
	command.Env = append(os.Environ(),
		ownerScopeLockHelperEnv+"=1",
		WorkDirEnv+"="+root,
		MinFreeBytesEnv+"=0",
	)
	stdin, err := command.StdinPipe()
	require.NoError(t, err)
	stdout, err := command.StdoutPipe()
	require.NoError(t, err)
	command.Stderr = os.Stderr
	require.NoError(t, command.Start())
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = command.Wait()
	})

	scanner := bufio.NewScanner(stdout)
	require.True(t, scanner.Scan())
	require.Equal(t, "ready", scanner.Text())
	t.Setenv(WorkDirEnv, root)

	_, err = AcquireOwnerScopeLock("node-shared")
	require.ErrorContains(t, err, "already active")

	require.NoError(t, stdin.Close())
	require.NoError(t, command.Wait())
	lock, err := AcquireOwnerScopeLock("node-shared")
	require.NoError(t, err)
	require.NoError(t, lock.Release())
}

func TestOwnerScopeRecoveryWaitsForChildWorkspaceAcrossProcesses(t *testing.T) {
	if os.Getenv(ownerScopeActivityHelperEnv) == "1" {
		_, cleanup, err := Prepare(context.Background(), os.Getpid())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Println("ready")
		_, _ = bufio.NewReader(os.Stdin).ReadByte()
		if err := cleanup(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
		return
	}

	root := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestOwnerScopeRecoveryWaitsForChildWorkspaceAcrossProcesses$")
	command.Env = append(os.Environ(),
		ownerScopeActivityHelperEnv+"=1",
		WorkDirEnv+"="+root,
		MinFreeBytesEnv+"=0",
		OwnerEnv+"=node-shared-task-child",
	)
	stdin, err := command.StdinPipe()
	require.NoError(t, err)
	stdout, err := command.StdoutPipe()
	require.NoError(t, err)
	command.Stderr = os.Stderr
	require.NoError(t, command.Start())
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = command.Wait()
	})

	scanner := bufio.NewScanner(stdout)
	require.True(t, scanner.Scan())
	require.Equal(t, "ready", scanner.Text())
	t.Setenv(WorkDirEnv, root)

	_, err = AcquireOwnerScopeRecoveryLock("node-shared")
	require.ErrorContains(t, err, "already active")
	require.NoError(t, stdin.Close())
	require.NoError(t, command.Wait())

	recoveryLock, err := AcquireOwnerScopeRecoveryLock("node-shared")
	require.NoError(t, err)
	require.NoError(t, recoveryLock.Release())
}

func TestPrepareUsesConfiguredRootAndIsolatesProcesses(t *testing.T) {
	root := filepath.Join(t.TempDir(), "custom-root")
	t.Setenv(WorkDirEnv, root)
	t.Setenv(MinFreeBytesEnv, "0")

	t.Setenv(OwnerEnv, "task-101")
	first, cleanupFirst, err := Prepare(context.Background(), 101)
	require.NoError(t, err)
	t.Setenv(OwnerEnv, "task-202")
	second, cleanupSecond, err := Prepare(context.Background(), 202)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanupFirst()) })
	t.Cleanup(func() { require.NoError(t, cleanupSecond()) })

	require.NotEqual(t, first, second)
	require.Equal(t, root, filepath.Dir(first))
	require.Equal(t, root, filepath.Dir(second))
	require.True(t, strings.HasPrefix(filepath.Base(first), "yakgit-task-101-"))
	require.True(t, strings.HasPrefix(filepath.Base(second), "yakgit-task-202-"))

	require.NoError(t, CleanupForOwner("task-101"))
	_, err = os.Stat(first)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(second)
	require.NoError(t, err)
}

func TestPrepareCreatesUniqueWorkspacesConcurrently(t *testing.T) {
	root := t.TempDir()
	t.Setenv(WorkDirEnv, root)
	t.Setenv(MinFreeBytesEnv, "0")
	t.Setenv(OwnerEnv, "")

	const workers = 8
	paths := make(chan string, workers)
	errs := make(chan error, workers)
	cleanups := make(chan func() error, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func(pid int) {
			defer group.Done()
			workspace, cleanup, err := Prepare(context.Background(), pid)
			if err != nil {
				errs <- err
				return
			}
			paths <- workspace
			cleanups <- cleanup
		}(1000 + index)
	}
	group.Wait()
	close(paths)
	close(errs)
	close(cleanups)

	for err := range errs {
		require.NoError(t, err)
	}
	unique := make(map[string]struct{}, workers)
	for workspace := range paths {
		unique[workspace] = struct{}{}
	}
	require.Len(t, unique, workers)
	for cleanup := range cleanups {
		require.NoError(t, cleanup())
	}
}

func TestCleanupForOwnerScopeRemovesOnlyThatScanNodeInstallation(t *testing.T) {
	root := t.TempDir()
	t.Setenv(WorkDirEnv, root)
	first, err := os.MkdirTemp(root, "yakgit-node-a-task-first-")
	require.NoError(t, err)
	second, err := os.MkdirTemp(root, "yakgit-node-a-task-second-")
	require.NoError(t, err)
	other, err := os.MkdirTemp(root, "yakgit-node-b-task-active-")
	require.NoError(t, err)

	require.NoError(t, CleanupForOwnerScope("node-a"))
	_, err = os.Stat(first)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(second)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(other)
	require.NoError(t, err)
}

func TestCleanupForOwnerScopeRejectsUnsafePrefix(t *testing.T) {
	t.Setenv(WorkDirEnv, t.TempDir())
	require.ErrorContains(t, CleanupForOwnerScope("../node"), "invalid SSA Git workspace owner")
}

func TestResolveRootDefaultsUnderYakitHome(t *testing.T) {
	yakitHome := t.TempDir()
	t.Setenv(WorkDirEnv, "")
	t.Setenv("YAKIT_HOME", yakitHome)

	root, err := ResolveRoot()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(yakitHome, "temp", "ssa-git"), root)
}

func TestPrepareRejectsUnsafeOwner(t *testing.T) {
	t.Setenv(WorkDirEnv, t.TempDir())
	t.Setenv(MinFreeBytesEnv, "0")
	t.Setenv(OwnerEnv, "../other-task")

	_, _, err := Prepare(context.Background(), 505)
	require.ErrorContains(t, err, "invalid SSA Git workspace owner")
}

func TestPrepareRejectsLowSpaceWithActionableEvidence(t *testing.T) {
	root := t.TempDir()
	t.Setenv(WorkDirEnv, root)
	t.Setenv(MinFreeBytesEnv, "4096")
	originalDiskUsage := diskUsage
	diskUsage = func(context.Context, string) (*diskUsageStat, error) {
		return &diskUsageStat{Free: 1024, InodesTotal: 10, InodesFree: 9}, nil
	}
	t.Cleanup(func() { diskUsage = originalDiskUsage })

	_, _, err := Prepare(context.Background(), 303)
	require.Error(t, err)
	require.ErrorContains(t, err, "directory=\"")
	require.ErrorContains(t, err, root)
	require.ErrorContains(t, err, "available_bytes=1024")
	require.ErrorContains(t, err, "required_minimum_bytes=4096")
	require.ErrorContains(t, err, WorkDirEnv)
}

func TestPrepareReportsFreeSpaceWhenWriteProbeHitsENOSPC(t *testing.T) {
	root := t.TempDir()
	t.Setenv(WorkDirEnv, root)
	t.Setenv(MinFreeBytesEnv, "0")
	originalCreateTemp := createTemp
	originalDiskUsage := diskUsage
	createTemp = func(string, string) (*os.File, error) {
		return nil, &os.PathError{Op: "open", Path: root, Err: syscall.ENOSPC}
	}
	diskUsage = func(context.Context, string) (*diskUsageStat, error) {
		return &diskUsageStat{Free: 0, InodesTotal: 10, InodesFree: 9}, nil
	}
	t.Cleanup(func() {
		createTemp = originalCreateTemp
		diskUsage = originalDiskUsage
	})

	_, _, err := Prepare(context.Background(), 606)
	require.ErrorIs(t, err, syscall.ENOSPC)
	require.ErrorContains(t, err, root)
	require.ErrorContains(t, err, "available_bytes=0")
	require.ErrorContains(t, err, "free disk space")
	require.ErrorContains(t, err, WorkDirEnv)
}

func TestPrepareReportsFreeSpaceWhenWorkspaceCreationHitsENOSPC(t *testing.T) {
	root := t.TempDir()
	t.Setenv(WorkDirEnv, root)
	t.Setenv(MinFreeBytesEnv, "0")
	originalMkdirTemp := mkdirTemp
	originalDiskUsage := diskUsage
	mkdirTemp = func(string, string) (string, error) {
		return "", &os.PathError{Op: "mkdir", Path: root, Err: syscall.ENOSPC}
	}
	diskUsage = func(context.Context, string) (*diskUsageStat, error) {
		return &diskUsageStat{Free: 512, InodesTotal: 10, InodesFree: 1}, nil
	}
	t.Cleanup(func() {
		mkdirTemp = originalMkdirTemp
		diskUsage = originalDiskUsage
	})

	_, _, err := Prepare(context.Background(), 607)
	require.ErrorIs(t, err, syscall.ENOSPC)
	require.ErrorContains(t, err, root)
	require.ErrorContains(t, err, "available_bytes=512")
	require.ErrorContains(t, err, "create isolated workspace")
	require.ErrorContains(t, err, "free disk space")
	require.ErrorContains(t, err, WorkDirEnv)
}

func TestPrepareRejectsInvalidMinimum(t *testing.T) {
	t.Setenv(WorkDirEnv, t.TempDir())
	t.Setenv(MinFreeBytesEnv, "1GiB")

	_, _, err := Prepare(context.Background(), 404)
	require.ErrorContains(t, err, MinFreeBytesEnv)
	require.ErrorContains(t, err, "non-negative byte count")
}

func TestWrapCloneErrorIncludesWorkspaceFreeSpaceAndAdvice(t *testing.T) {
	root := t.TempDir()
	originalDiskUsage := diskUsage
	diskUsage = func(context.Context, string) (*diskUsageStat, error) {
		return &diskUsageStat{Free: 2048}, nil
	}
	t.Cleanup(func() { diskUsage = originalDiskUsage })

	err := WrapCloneError(context.Background(), root, &os.PathError{Op: "write", Path: root, Err: syscall.ENOSPC})
	require.ErrorContains(t, err, root)
	require.ErrorContains(t, err, "available_bytes=2048")
	require.ErrorContains(t, err, WorkDirEnv)
	require.ErrorIs(t, err, syscall.ENOSPC)
}

func TestWrapCloneErrorDoesNotMisclassifyOtherFailuresAsDiskPressure(t *testing.T) {
	root := t.TempDir()
	originalDiskUsage := diskUsage
	diskUsage = func(context.Context, string) (*diskUsageStat, error) {
		return &diskUsageStat{Free: 2048}, nil
	}
	t.Cleanup(func() { diskUsage = originalDiskUsage })

	err := WrapCloneError(context.Background(), root, os.ErrPermission)
	require.ErrorContains(t, err, "available_bytes=2048")
	require.NotContains(t, err.Error(), "free disk space")
	require.ErrorIs(t, err, os.ErrPermission)
}

func TestCleanupReportsRemovalFailure(t *testing.T) {
	root := t.TempDir()
	t.Setenv(WorkDirEnv, root)
	t.Setenv(MinFreeBytesEnv, "0")
	workspace, cleanup, err := Prepare(context.Background(), 707)
	require.NoError(t, err)
	originalRemoveAll := removeAll
	removeAll = func(path string) error {
		return &os.PathError{Op: "remove", Path: path, Err: os.ErrPermission}
	}
	t.Cleanup(func() {
		removeAll = originalRemoveAll
		_ = os.RemoveAll(workspace)
	})

	err = cleanup()
	require.ErrorIs(t, err, os.ErrPermission)
	require.ErrorContains(t, err, workspace)
}
