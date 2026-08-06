package browser

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newAuthorizationLifecycleTestManager() *ExtensionBridgeManager {
	return &ExtensionBridgeManager{
		engineInstanceID:        "engine-current",
		authorization:           make(map[string]ExtensionAuthorizationWorkspace),
		authorizationTombstones: make(map[string]extensionAuthorizationWorkspaceTombstone),
	}
}

func authorizationLifecycleTestWorkspace(
	manager *ExtensionBridgeManager,
	index int,
	createdAt int64,
) ExtensionAuthorizationWorkspace {
	left := testAuthorizationSlot("left", "installation-left", fmt.Sprintf("left-%d", index), "authenticated")
	right := testAuthorizationSlot("right", "installation-right", fmt.Sprintf("right-%d", index), "authenticated")
	left.Target.TabID = 100 + index*2
	right.Target.TabID = 101 + index*2
	return ExtensionAuthorizationWorkspace{
		Version:          1,
		ID:               newExtensionAuthorizationWorkspaceID(manager.engineInstanceID),
		EngineInstanceID: manager.engineInstanceID,
		Mode:             "horizontal",
		Left:             left,
		Right:            right,
		CreatedAt:        createdAt,
		ExpiresAt:        time.Now().Add(time.Hour).UnixMilli(),
	}
}

func requireAuthorizationLifecycleReason(
	t *testing.T,
	err error,
	reason ExtensionAuthorizationWorkspaceLifecycleReason,
) *ExtensionAuthorizationWorkspaceLifecycleError {
	t.Helper()
	var lifecycle *ExtensionAuthorizationWorkspaceLifecycleError
	require.ErrorAs(t, err, &lifecycle)
	require.Equal(t, reason, lifecycle.Reason)
	return lifecycle
}

func TestAuthorizationWorkspaceReadDoesNotEvictAtCapacity(t *testing.T) {
	manager := newAuthorizationLifecycleTestManager()
	now := time.Now().UnixMilli()
	var oldest ExtensionAuthorizationWorkspace
	for index := 0; index < maxExtensionAuthorizationWorkspaces; index++ {
		workspace := authorizationLifecycleTestWorkspace(manager, index, now+int64(index))
		if index == 0 {
			oldest = workspace
		}
		manager.authorization[workspace.ID] = workspace
	}

	got, err := manager.GetExtensionAuthorizationWorkspace(context.Background(), oldest.ID, false)
	require.NoError(t, err)
	require.Equal(t, oldest.ID, got.ID)
	require.Len(t, manager.authorization, maxExtensionAuthorizationWorkspaces)
	require.Empty(t, manager.authorizationTombstones)
}

func TestAuthorizationWorkspaceThirtyThirdInsertEvictsOldest(t *testing.T) {
	manager := newAuthorizationLifecycleTestManager()
	now := time.Now().UnixMilli()
	var oldest ExtensionAuthorizationWorkspace
	for index := 0; index < maxExtensionAuthorizationWorkspaces; index++ {
		workspace := authorizationLifecycleTestWorkspace(manager, index, now+int64(index))
		if index == 0 {
			oldest = workspace
		}
		manager.authorization[workspace.ID] = workspace
	}
	newest := authorizationLifecycleTestWorkspace(manager, maxExtensionAuthorizationWorkspaces, now+100)
	manager.insertExtensionAuthorizationWorkspace(newest)

	require.Len(t, manager.authorization, maxExtensionAuthorizationWorkspaces)
	_, err := manager.GetExtensionAuthorizationWorkspace(context.Background(), oldest.ID, false)
	requireAuthorizationLifecycleReason(t, err, ExtensionAuthorizationWorkspaceEvicted)
	_, err = manager.GetExtensionAuthorizationWorkspace(context.Background(), newest.ID, false)
	require.NoError(t, err)
}

func TestAuthorizationWorkspaceNaturalExpiryIsReported(t *testing.T) {
	manager := newAuthorizationLifecycleTestManager()
	workspace := authorizationLifecycleTestWorkspace(manager, 1, time.Now().Add(-time.Minute).UnixMilli())
	workspace.ExpiresAt = time.Now().Add(-time.Millisecond).UnixMilli()
	manager.authorization[workspace.ID] = workspace

	_, err := manager.GetExtensionAuthorizationWorkspace(context.Background(), workspace.ID, false)
	lifecycle := requireAuthorizationLifecycleReason(t, err, ExtensionAuthorizationWorkspaceExpired)
	require.Equal(t, workspace.ExpiresAt, lifecycle.ExpiresAt)
}

func TestAuthorizationWorkspaceReplacementIsReported(t *testing.T) {
	manager := newAuthorizationLifecycleTestManager()
	first := authorizationLifecycleTestWorkspace(manager, 1, time.Now().UnixMilli())
	second := first
	second.ID = newExtensionAuthorizationWorkspaceID(manager.engineInstanceID)
	second.CreatedAt++
	manager.insertExtensionAuthorizationWorkspace(first)
	manager.insertExtensionAuthorizationWorkspace(second)

	_, err := manager.GetExtensionAuthorizationWorkspace(context.Background(), first.ID, false)
	lifecycle := requireAuthorizationLifecycleReason(t, err, ExtensionAuthorizationWorkspaceReplaced)
	require.Equal(t, second.ID, lifecycle.ReplacementWorkspaceID)
	_, err = manager.GetExtensionAuthorizationWorkspace(context.Background(), second.ID, false)
	require.NoError(t, err)
}

func TestAuthorizationWorkspaceLateUpdateCannotResurrectReplacement(t *testing.T) {
	manager := newAuthorizationLifecycleTestManager()
	first := authorizationLifecycleTestWorkspace(manager, 1, time.Now().UnixMilli())
	replacement := first
	replacement.ID = newExtensionAuthorizationWorkspaceID(manager.engineInstanceID)
	replacement.CreatedAt++
	manager.insertExtensionAuthorizationWorkspace(first)
	manager.insertExtensionAuthorizationWorkspace(replacement)

	first.State = "stale"
	err := manager.updateExtensionAuthorizationWorkspace(first)
	requireAuthorizationLifecycleReason(t, err, ExtensionAuthorizationWorkspaceReplaced)
	require.NotContains(t, manager.authorization, first.ID)
	require.Contains(t, manager.authorization, replacement.ID)
}

func TestAuthorizationWorkspacePreviousEngineAndUnknownIDsAreDistinct(t *testing.T) {
	manager := newAuthorizationLifecycleTestManager()
	previousID := newExtensionAuthorizationWorkspaceID("engine-previous")

	_, err := manager.GetExtensionAuthorizationWorkspace(context.Background(), previousID, false)
	lifecycle := requireAuthorizationLifecycleReason(t, err, ExtensionAuthorizationWorkspaceEngineInstanceChanged)
	require.Equal(t, manager.engineInstanceID, lifecycle.EngineInstanceID)

	currentUnknownID := newExtensionAuthorizationWorkspaceID(manager.engineInstanceID)
	_, err = manager.GetExtensionAuthorizationWorkspace(context.Background(), currentUnknownID, false)
	requireAuthorizationLifecycleReason(t, err, ExtensionAuthorizationWorkspaceNotFound)

	var target *ExtensionAuthorizationWorkspaceLifecycleError
	require.True(t, errors.As(err, &target))
}

func TestExtensionAuthorizationClientErrorPreservesLifecycleReason(t *testing.T) {
	input := &ExtensionAuthorizationWorkspaceLifecycleError{
		Reason:           ExtensionAuthorizationWorkspaceExpired,
		WorkspaceID:      "workspace-1",
		EngineInstanceID: "engine-current",
	}
	result := extensionAuthorizationClientError(input)
	require.Equal(t, "authorization_workspace_expired", result.Code)
	require.JSONEq(t, `{
		"reason":"expired",
		"workspaceId":"workspace-1",
		"engineInstanceId":"engine-current"
	}`, string(result.Data))
}
