//go:build !windows && !plan9
// +build !windows,!plan9

package routewrapper

import (
	"os/exec"
)

func onBeforeCommandRun(cmd *exec.Cmd) (interface{}, error) {
	cmd.Env = []string{"LANG=C", "LC_CTYPE=C", "LC_MESSAGES=C"}
	return nil, nil
}

func onAfterCommandRun(ctx interface{}, cmd *exec.Cmd) error {
	return nil
}
