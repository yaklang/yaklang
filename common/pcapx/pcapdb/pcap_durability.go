package pcapdb

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Sync every newly created directory and its first existing parent, from leaf
// to root. A synced file is not durable if its parent directory disappears.
func makeDurableDirectory(path string) error {
	var directories []string
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		directories = append(directories, current)
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("pcapdb: not a directory: %s", current)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(current) == current {
			return err
		}
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	for _, directory := range directories {
		if err := syncDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		// Go cannot portably flush an opened directory handle on Windows.
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
