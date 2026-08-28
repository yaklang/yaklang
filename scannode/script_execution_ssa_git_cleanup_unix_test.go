//go:build linux || darwin

package scannode

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssagitworkdir"
)

func TestExecuteScriptCleansChildSSAGitWorkspaceAfterSuccess(t *testing.T) {
	root := t.TempDir()
	helper := writeSSAGitWorkspaceHelper(t, false)
	t.Setenv(ssagitworkdir.WorkDirEnv, root)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")

	var node *ScanNode
	err := node.executeScript(context.Background(), helper, "fixture.yak", nil, "runtime", nil, nil)
	require.NoError(t, err)
	require.Empty(t, mustReadDir(t, root))
}

func TestExecuteScriptCleansChildSSAGitWorkspaceAfterCancellation(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ready")
	helper := writeSSAGitWorkspaceHelper(t, true)
	t.Setenv(ssagitworkdir.WorkDirEnv, root)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")
	t.Setenv("YAK_SSA_GIT_TEST_MARKER", marker)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		var node *ScanNode
		errCh <- node.executeScript(ctx, helper, "fixture.yak", nil, "runtime", nil, nil)
	}()
	require.Eventually(t, func() bool {
		_, err := os.Stat(marker)
		return err == nil
	}, 5*time.Second, 10*time.Millisecond)
	cancel()

	require.Error(t, <-errCh)
	require.Empty(t, mustReadDir(t, root))
}

func TestExecuteScriptExtraEnvironmentOverridesParentYakitHome(t *testing.T) {
	parentHome := t.TempDir()
	taskHome := t.TempDir()
	resultPath := filepath.Join(t.TempDir(), "yakit-home.txt")
	helper := filepath.Join(t.TempDir(), "helper.sh")
	require.NoError(t, os.WriteFile(
		helper,
		[]byte("#!/bin/sh\nset -eu\nprintf '%s' \"$YAKIT_HOME\" > \"$YAKIT_HOME_RESULT\"\n"),
		0o700,
	))
	t.Setenv("YAKIT_HOME", parentHome)

	var node *ScanNode
	err := node.executeScript(
		context.Background(),
		helper,
		"fixture.yak",
		nil,
		"runtime",
		[]string{"YAKIT_HOME=" + taskHome, "YAKIT_HOME_RESULT=" + resultPath},
		nil,
	)
	require.NoError(t, err)
	raw, err := os.ReadFile(resultPath)
	require.NoError(t, err)
	require.Equal(t, taskHome, string(raw))
}

func writeSSAGitWorkspaceHelper(t *testing.T, block bool) string {
	t.Helper()
	body := "#!/bin/sh\nset -eu\nmkdir -p \"$YAK_SSA_GIT_WORKDIR/yakgit-$YAK_SSA_GIT_WORKSPACE_OWNER-fixture\"\n"
	if block {
		body += ": > \"$YAK_SSA_GIT_TEST_MARKER\"\nwhile :; do :; done\n"
	}
	path := filepath.Join(t.TempDir(), "helper.sh")
	require.NoError(t, os.WriteFile(path, []byte(body), 0700))
	return path
}

func mustReadDir(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	return entries
}
