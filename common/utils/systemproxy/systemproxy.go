package systemproxy

import (
	"errors"
	"fmt"
	"runtime"
)

// Settings represents systemwide proxy settings.
type Settings struct {
	// Enabled is true if static (i.e. non-PAC) proxy is enabled
	Enabled bool

	// DefaultServer is the server used for all protocols.
	DefaultServer string
}

// ErrNotImpl error is returned when the current platform isn't supported yet.
var ErrNotImpl = errors.New(fmt.Sprintf("systemproxy not implemented on this platform: %v", runtime.GOOS))
