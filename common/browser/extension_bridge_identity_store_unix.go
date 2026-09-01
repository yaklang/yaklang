//go:build !windows

package browser

import (
	"fmt"
	"os"
	"path/filepath"
)

func replaceExtensionBridgeIdentityFile(temporaryPath, targetPath string) error {
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return fmt.Errorf("atomically replace identity file: %w", err)
	}
	directory, err := os.Open(filepath.Dir(targetPath))
	if err != nil {
		return fmt.Errorf("open identity directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync identity directory: %w", err)
	}
	return nil
}
