package scannode

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssagitworkdir"
)

func TestSSAGitWorkspaceOwnerScopeIsStableAndOpaque(t *testing.T) {
	const installationID = "installation-secret-value"
	first := ssaGitWorkspaceOwnerScope(installationID)
	second := ssaGitWorkspaceOwnerScope("  " + installationID + "  ")

	require.Equal(t, first, second)
	require.True(t, strings.HasPrefix(first, "node-"))
	require.NotContains(t, first, installationID)
}

func TestResilienceDelayedTaskDrainEventuallyReleasesSSAGitScopeLock(t *testing.T) {
	root := t.TempDir()
	t.Setenv(ssagitworkdir.WorkDirEnv, root)
	const scope = "node-delayed-drain"
	ownerLock, err := ssagitworkdir.AcquireOwnerScopeLock(scope)
	require.NoError(t, err)

	manager := newTaskManager()
	taskCtx, cancelTask := context.WithCancel(context.Background())
	task := newScriptTask(taskCtx, cancelTask, "task", "job", "subtask", "attempt")
	require.True(t, manager.Add(task.TaskId, task))
	node := &ScanNode{manager: manager, ssaGitScopeLock: ownerLock}

	originalTimeout := scanNodeTaskDrainTimeout
	scanNodeTaskDrainTimeout = time.Millisecond
	t.Cleanup(func() { scanNodeTaskDrainTimeout = originalTimeout })
	node.releaseSSAGitScopeLockAfterTasks()

	_, err = ssagitworkdir.AcquireOwnerScopeLock(scope)
	require.ErrorContains(t, err, "already active")
	manager.Remove(task.TaskId)
	cancelTask()

	var replacementLock *ssagitworkdir.OwnerScopeLock
	require.Eventually(t, func() bool {
		var acquireErr error
		replacementLock, acquireErr = ssagitworkdir.AcquireOwnerScopeLock(scope)
		return acquireErr == nil
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, replacementLock.Release())
}

func TestNextSSAGitWorkspaceOwnerUsesNodeScopeAndUniqueTaskID(t *testing.T) {
	node := &ScanNode{ssaGitOwnerScope: "node-scope"}
	first := node.nextSSAGitWorkspaceOwner()
	second := node.nextSSAGitWorkspaceOwner()

	require.True(t, strings.HasPrefix(first, "node-scope-task-"))
	require.True(t, strings.HasPrefix(second, "node-scope-task-"))
	require.NotEqual(t, first, second)
}

func TestResilienceScanNodeRestartRecoversOnlyItsSSAGitWorkspaces(t *testing.T) {
	root := t.TempDir()
	t.Setenv(ssagitworkdir.WorkDirEnv, root)

	staleScope := ssaGitWorkspaceOwnerScope("installation-a")
	activeScope := ssaGitWorkspaceOwnerScope("installation-b")
	stale, err := os.MkdirTemp(root, "yakgit-"+staleScope+"-task-stale-")
	require.NoError(t, err)
	active, err := os.MkdirTemp(root, "yakgit-"+activeScope+"-task-active-")
	require.NoError(t, err)

	scanNode, err := NewScanNode(node.BaseConfig{
		AgentInstallationID: "installation-a",
		BaseDir:             t.TempDir(),
		EnrollmentToken:     "enrollment-token",
		PlatformAPIBaseURL:  "http://platform.test",
		TransportClient:     &bootstrapSessionTransport{},
	})
	require.NoError(t, err)
	t.Cleanup(scanNode.Shutdown)
	require.Equal(t, staleScope, scanNode.ssaGitOwnerScope)
	_, err = os.Stat(stale)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(active)
	require.NoError(t, err)

	// A different NodeBase directory must not bypass the lock protecting the
	// same managed SSA Git root and installation scope.
	peer, err := NewScanNode(node.BaseConfig{
		AgentInstallationID: "installation-a",
		BaseDir:             t.TempDir(),
		EnrollmentToken:     "enrollment-token",
		PlatformAPIBaseURL:  "http://platform.test",
		TransportClient:     &bootstrapSessionTransport{},
	})
	require.Nil(t, peer)
	require.ErrorContains(t, err, "already active")
}
