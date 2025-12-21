//go:build !windows
// +build !windows

package yaklib

import (
	"os/exec"
	"syscall"
)

// setupProcessGroup configures the command to run in a new process group
// and sets up proper cleanup when context is cancelled.
// On Unix systems, this ensures the entire process tree is killed.
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Override the Cancel function to kill the process group instead of just the process
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			// Kill the entire process group (negative PID)
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
}

