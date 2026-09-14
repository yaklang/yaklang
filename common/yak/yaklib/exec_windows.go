//go:build windows
// +build windows

package yaklib

import (
	"os/exec"

	"github.com/yaklang/yaklang/common/subprocess"
)

// setupProcessGroup configures the command for proper cleanup when context is cancelled.
// On Windows, the default behavior is sufficient as exec.CommandContext
// will terminate the process when context is cancelled.
func setupProcessGroup(cmd *exec.Cmd) {
	subprocess.ConfigureProcessGroup(cmd)
}
