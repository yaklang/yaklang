//go:build windows

package pcapdb

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func tryFileLock(f *os.File) error {
	return tryWindowsLock(f, windows.LOCKFILE_EXCLUSIVE_LOCK)
}
func tryReadFileLock(f *os.File) error { return tryWindowsLock(f, 0) }
func tryWindowsLock(f *os.File, flags uint32) error {
	err := windows.LockFileEx(windows.Handle(f.Fd()), flags|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrBusy
	}
	return err
}
