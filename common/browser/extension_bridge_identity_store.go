package browser

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeExtensionBridgeIdentityFileAtomically(targetPath string, data []byte) error {
	directory := filepath.Dir(targetPath)
	temporaryFile, err := os.CreateTemp(directory, "."+filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary identity file: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporaryFile.Close()
		}
		_ = os.Remove(temporaryPath)
	}()

	if err := temporaryFile.Chmod(0o600); err != nil {
		return fmt.Errorf("set temporary identity permissions: %w", err)
	}
	if _, err := temporaryFile.Write(data); err != nil {
		return fmt.Errorf("write temporary identity file: %w", err)
	}
	if err := temporaryFile.Sync(); err != nil {
		return fmt.Errorf("sync temporary identity file: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close temporary identity file: %w", err)
	}
	closed = true
	if err := replaceExtensionBridgeIdentityFile(temporaryPath, targetPath); err != nil {
		return err
	}
	return nil
}
