package damenggorm

import (
	"context"
	"database/sql/driver"
	"errors"
	"net"
)

// ErrNativeUnavailable is returned before connecting when the application was
// built without the optional DM protocol driver. Never fall back to local IR.
var ErrNativeUnavailable = errors.New("DM IR requires the native driver: rebuild with -tags dameng")

// Driver-specific APIs stay behind this boundary. The dialect, SQL adaptation,
// cancellation wrapper and SSA callers depend only on database/sql interfaces.
// Hooks are installed during package initialization and are immutable afterward.
var nativeDriver struct {
	matches      func(driver.Driver) bool
	registerDial func(string, func(context.Context, string) (net.Conn, error))
}

// NativeEnabled reports whether the application includes the DM protocol driver.
func NativeEnabled() bool { return nativeDriver.registerDial != nil }
