//go:build !linux && !darwin && !freebsd && !dragonfly && !netbsd && !openbsd && !solaris && !windows
// +build !linux,!darwin,!freebsd,!dragonfly,!netbsd,!openbsd,!solaris,!windows

package pty

func (*Cmd) start() error {
	return ErrUnsupported
}

func (*Cmd) wait() error {
	return ErrUnsupported
}
