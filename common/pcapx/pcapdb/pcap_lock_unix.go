//go:build unix

package pcapdb

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryFileLock(f *os.File) error {
	return tryFlock(f, unix.LOCK_EX)
}
func tryReadFileLock(f *os.File) error { return tryFlock(f, unix.LOCK_SH) }
func tryFlock(f *os.File, mode int) error {
	err := unix.Flock(int(f.Fd()), mode|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrBusy
	}
	return err
}
