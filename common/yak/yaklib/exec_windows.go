//go:build windows
// +build windows

package yaklib

import (
	"os/exec"
)

// setupProcessGroup configures the command for proper cleanup when context is cancelled.
// On Windows, the default behavior is sufficient as exec.CommandContext
// will terminate the process when context is cancelled.
func setupProcessGroup(cmd *exec.Cmd) {
	// Windows handles process termination differently.
	// The default exec.CommandContext behavior is sufficient.
	// No additional setup needed.
}

