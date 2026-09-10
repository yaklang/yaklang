//go:build windows

package engineendpoint

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func configureIPCSmokeChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func runIPCSmokePlatform(t *testing.T) {
	t.Run("pipe-collision", TestNamedPipeWindowsCollision)
	t.Run("concurrent-starts", TestNamedPipeConcurrentStarts)
	t.Run("unicode-name", TestNamedPipeUnicodeName)
	t.Run("private-acl", TestNamedPipeACLContainsOnlyCurrentUserAndSystem)
	t.Run("working-directory", TestNamedPipeDifferentWorkingDirectory)
	t.Run("ordinary-token-server", func(t *testing.T) {
		token := ordinaryTestToken(t)
		defer token.Close()
		if token.IsElevated() {
			t.Fatal("smoke token must not be elevated")
		}
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		testPipeChildServer(t, binary, t.TempDir(), token)
	})
}
