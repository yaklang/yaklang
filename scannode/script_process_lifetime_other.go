//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package scannode

import "os/exec"

// ExtraFiles and Unix process groups are unavailable on these platforms.
// Keep their existing CommandContext cancellation behavior.
func configureScriptProcessLifetime(_ *exec.Cmd) (func(), error) {
	return func() {}, nil
}

func watchScriptParentLifetime() (func(), error) {
	return func() {}, nil
}
