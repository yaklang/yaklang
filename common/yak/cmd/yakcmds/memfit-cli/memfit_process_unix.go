//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package memfitcli

import (
	"os/exec"
	"syscall"
)

func configureMemfitChildProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killMemfitChildProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
