//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package yakcmds

import "os/exec"

func configureMemfitChildProcess(_ *exec.Cmd) {}

func killMemfitChildProcess(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
