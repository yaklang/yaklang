//go:build !windows

package minirehs

import "os"

func transientImageLockError(err error) bool { return false }

func readImageFile(path string) ([]byte, error) { return os.ReadFile(path) }

func replaceImageFile(from, to string) error { return os.Rename(from, to) }
