//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package subprocess

import (
	"os/exec"
	"syscall"
)

// ConfigureProcessGroup sets up a new process group so the entire
// tree can be killed together. Called before cmd.Start().
func ConfigureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillProcessGroup sends SIGKILL to the process group (including
// grandchildren that stayed in that group), falling back to killing
// the direct child.
func KillProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
