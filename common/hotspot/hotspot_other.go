//go:build !darwin
// +build !darwin

package hotspot

// getEnableScript returns empty for non-darwin platforms
func getEnableScript() string {
	return ""
}

// getDisableScript returns empty for non-darwin platforms
func getDisableScript() string {
	return ""
}

