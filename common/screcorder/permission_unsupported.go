//go:build !darwin
// +build !darwin

package screcorder

// RequestScreenRecordingPermission is a stub for non-Darwin platforms.
// It always returns true as this permission model is specific to macOS.
func RequestScreenRecordingPermission() bool {
	return true
}
