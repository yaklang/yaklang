//go:build !unix && !windows

package pcapdb

import (
	"fmt"
	"os"
)

func tryFileLock(*os.File) error {
	return fmt.Errorf("pcapdb: file locking is unsupported on this platform")
}

func tryReadFileLock(f *os.File) error { return tryFileLock(f) }
