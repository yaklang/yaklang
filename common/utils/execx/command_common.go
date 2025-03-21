//go:build !windows
// +build !windows

package execx

import (
	"context"
	"os/exec"
)

func Command(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}

func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}
