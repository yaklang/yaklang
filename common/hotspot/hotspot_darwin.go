//go:build darwin
// +build darwin

package hotspot

import (
	_ "embed"
)

//go:embed scripts/enable_hotspot.applescript
var enableScript string

//go:embed scripts/disable_hotspot.applescript
var disableScript string

// getEnableScript returns the embedded enable script
func getEnableScript() string {
	return enableScript
}

// getDisableScript returns the embedded disable script
func getDisableScript() string {
	return disableScript
}
