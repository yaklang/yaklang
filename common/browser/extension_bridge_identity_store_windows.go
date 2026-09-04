//go:build windows

package browser

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func replaceExtensionBridgeIdentityFile(temporaryPath, targetPath string) error {
	temporaryPathPointer, err := windows.UTF16PtrFromString(temporaryPath)
	if err != nil {
		return fmt.Errorf("encode temporary identity path: %w", err)
	}
	targetPathPointer, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return fmt.Errorf("encode target identity path: %w", err)
	}
	if err := windows.MoveFileEx(
		temporaryPathPointer,
		targetPathPointer,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	); err != nil {
		return fmt.Errorf("atomically replace identity file: %w", err)
	}
	return nil
}
