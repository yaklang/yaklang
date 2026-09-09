//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package stdio

import "os/exec"

func configureProcess(_ *exec.Cmd) {}

func killProcess(cmd *exec.Cmd) { _ = cmd.Process.Kill() }
