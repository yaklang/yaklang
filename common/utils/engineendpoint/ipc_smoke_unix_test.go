//go:build !windows

package engineendpoint

import (
	"os/exec"
	"testing"
)

func configureIPCSmokeChild(cmd *exec.Cmd) {}

func runIPCSmokePlatform(t *testing.T) {
	t.Run("private-modes-and-replacement", TestPrivateModesAndReplacementPreservedOnClose)
	t.Run("shared-directory", TestExistingSharedDirectoryPermissionsArePreserved)
	t.Run("symlink-parent", TestSymlinkParentAndAliasCannotTakeOver)
	t.Run("stale-recovery", TestReclaimsStaleSocketAfterReadOnlyPreflight)
	t.Run("concurrent-recovery", TestConcurrentStaleRecoveryHasExactlyOneListener)
	t.Run("foreign-endpoint", TestRefusesForeignLiveSocketAndDatagramSocket)
	t.Run("preserve-non-socket", TestRefusesFilesDirectoriesAndLinks)
	t.Run("startup-lock", TestStaleSocketIsNotRemovedWhileStartupOwnsLock)
	t.Run("lock-symlink", TestLockSymlinkIsPreserved)
	t.Run("short-alias", TestShortSymlinkAddressDoesNotInheritRealPathLength)
}
