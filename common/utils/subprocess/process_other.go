//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package subprocess

import "os/exec"

// ConfigureProcessGroup is a no-op on platforms without process groups.
func ConfigureProcessGroup(_ *exec.Cmd) {}

// KillProcessGroup kills the direct child process.
func KillProcessGroup(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
