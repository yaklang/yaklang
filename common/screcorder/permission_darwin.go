//go:build darwin
// +build darwin

package screcorder

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CGDisplayConfiguration.h>

// CGRequestScreenCaptureAccess is a C function that wraps the Objective-C call.
int RequestScreenCaptureAccess() {
    if (CGRequestScreenCaptureAccess()) {
        return 1; // true, access granted or already has access
    }
    return 0; // false, access denied
}
*/
import "C"

// RequestScreenRecordingPermission triggers the macOS screen recording permission prompt.
// It returns true if permission is already granted or was just granted by the user.
// It returns false if the user denies the permission.
func RequestScreenRecordingPermission() bool {
	return C.RequestScreenCaptureAccess() == 1
}
