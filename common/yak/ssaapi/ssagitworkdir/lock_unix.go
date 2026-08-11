//go:build unix

package ssagitworkdir

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockFileExclusive(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func lockFileShared(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_SH|unix.LOCK_NB)
}

func unlockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
