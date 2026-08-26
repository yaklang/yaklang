package ssaapi

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssagitworkdir"
)

func TestIsRetryableGitCloneError(t *testing.T) {
	t.Parallel()
	require.True(t, isRetryableGitCloneError(io.EOF))
	require.True(t, isRetryableGitCloneError(io.ErrUnexpectedEOF))
	require.True(t, isRetryableGitCloneError(errors.New(`Get "https://github.com/dotCMS/core/info/refs?service=git-upload-pack": EOF`)))
	require.True(t, isRetryableGitCloneError(errors.New("TLS handshake timeout")))
	require.False(t, isRetryableGitCloneError(transport.ErrAuthenticationRequired))
	require.False(t, isRetryableGitCloneError(transport.ErrRepositoryNotFound))
	require.False(t, isRetryableGitCloneError(errors.New("no space left on device")))
	require.False(t, isRetryableGitCloneError(errors.New("authentication required")))
}

func TestCloneSSAGitRepositoryWithRetrySucceedsAfterTransientEOF(t *testing.T) {
	originalClone := cloneSSAGitRepository
	originalSleep := ssaGitCloneSleep
	t.Cleanup(func() {
		cloneSSAGitRepository = originalClone
		ssaGitCloneSleep = originalSleep
	})
	ssaGitCloneSleep = func(time.Duration) {}

	var attempts atomic.Int32
	cloneSSAGitRepository = func(_, local string, _ ...yakgit.Option) error {
		n := attempts.Add(1)
		if n < 3 {
			_ = os.WriteFile(filepath.Join(local, "partial"), []byte("debris"), 0o600)
			return errors.New(`git clone: https://example.invalid/repo.git to ` + local + ` failed: EOF`)
		}
		require.NoFileExists(t, filepath.Join(local, "partial"))
		return os.WriteFile(filepath.Join(local, "main.yak"), []byte("println(1)"), 0o600)
	}

	dir := t.TempDir()
	err := cloneSSAGitRepositoryWithRetry("https://example.invalid/repo.git", dir, nil, nil)
	require.NoError(t, err)
	require.Equal(t, int32(3), attempts.Load())
	require.FileExists(t, filepath.Join(dir, "main.yak"))
}

func TestCloneSSAGitRepositoryWithRetryDoesNotRetryAuthFailure(t *testing.T) {
	originalClone := cloneSSAGitRepository
	originalSleep := ssaGitCloneSleep
	t.Cleanup(func() {
		cloneSSAGitRepository = originalClone
		ssaGitCloneSleep = originalSleep
	})
	ssaGitCloneSleep = func(time.Duration) {}

	var attempts atomic.Int32
	cloneSSAGitRepository = func(_, _ string, _ ...yakgit.Option) error {
		attempts.Add(1)
		return transport.ErrAuthenticationRequired
	}

	err := cloneSSAGitRepositoryWithRetry("https://example.invalid/repo.git", t.TempDir(), nil, nil)
	require.ErrorIs(t, err, transport.ErrAuthenticationRequired)
	require.Equal(t, int32(1), attempts.Load())
}

func TestGitFSRetriesTransientCloneFailures(t *testing.T) {
	root := t.TempDir()
	t.Setenv(ssagitworkdir.WorkDirEnv, root)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")

	originalClone := cloneSSAGitRepository
	originalSleep := ssaGitCloneSleep
	t.Cleanup(func() {
		cloneSSAGitRepository = originalClone
		ssaGitCloneSleep = originalSleep
	})
	ssaGitCloneSleep = func(time.Duration) {}

	var attempts atomic.Int32
	cloneSSAGitRepository = func(_, local string, _ ...yakgit.Option) error {
		n := attempts.Add(1)
		if n == 1 {
			return errors.New(`Get "https://github.com/dotCMS/core/info/refs?service=git-upload-pack": EOF`)
		}
		return os.WriteFile(filepath.Join(local, "main.yak"), []byte("println(1)"), 0o600)
	}

	config := newGitSourceConfigForTest(t)
	_, err := gitFs(config)
	require.NoError(t, err)
	require.Equal(t, int32(2), attempts.Load())
}

func TestGitFSWrapsExhaustedCloneRetriesWithoutCompilePrefix(t *testing.T) {
	root := t.TempDir()
	t.Setenv(ssagitworkdir.WorkDirEnv, root)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")

	originalClone := cloneSSAGitRepository
	originalSleep := ssaGitCloneSleep
	t.Cleanup(func() {
		cloneSSAGitRepository = originalClone
		ssaGitCloneSleep = originalSleep
	})
	ssaGitCloneSleep = func(time.Duration) {}

	cloneSSAGitRepository = func(_, _ string, _ ...yakgit.Option) error {
		return errors.New(`Get "https://github.com/dotCMS/core/info/refs?service=git-upload-pack": EOF`)
	}

	config := newGitSourceConfigForTest(t)
	_, err := gitFs(config)
	require.Error(t, err)
	require.ErrorContains(t, err, "SSA Git clone failed")
	require.False(t, strings.Contains(err.Error(), "编译失败"))
}

func TestCloneSSAGitRepositoryPreferShallowFallsBackToFullClone(t *testing.T) {
	originalClone := cloneSSAGitRepository
	originalSleep := ssaGitCloneSleep
	t.Cleanup(func() {
		cloneSSAGitRepository = originalClone
		ssaGitCloneSleep = originalSleep
	})
	ssaGitCloneSleep = func(time.Duration) {}

	var depths []int
	cloneSSAGitRepository = func(_, local string, opts ...yakgit.Option) error {
		settings := yakgit.InspectCloneSettings(opts...)
		depths = append(depths, settings.Depth)
		if settings.Depth != 0 {
			return errors.New(`unexpected client error: unexpected requesting "http://127.0.0.1/repo.git/git-upload-pack" status code: 500`)
		}
		return os.WriteFile(filepath.Join(local, "ok"), []byte("1"), 0o600)
	}

	dir := t.TempDir()
	err := cloneSSAGitRepositoryPreferShallow(
		"https://example.invalid/repo.git",
		dir,
		[]yakgit.Option{yakgit.WithDepth(1), yakgit.WithSingleBranch(true)},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, []int{1, 0}, depths)
}

func TestCloneSSAGitRepositoryPreferShallowKeepsPermanentErrors(t *testing.T) {
	originalClone := cloneSSAGitRepository
	t.Cleanup(func() { cloneSSAGitRepository = originalClone })

	var attempts int
	cloneSSAGitRepository = func(_, _ string, _ ...yakgit.Option) error {
		attempts++
		return transport.ErrAuthenticationRequired
	}

	err := cloneSSAGitRepositoryPreferShallow(
		"https://example.invalid/repo.git",
		t.TempDir(),
		[]yakgit.Option{yakgit.WithDepth(1)},
		nil,
	)
	require.ErrorIs(t, err, transport.ErrAuthenticationRequired)
	require.Equal(t, 1, attempts)
}
