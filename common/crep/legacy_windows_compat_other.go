//go:build !windows

package crep

// isLegacyWindowsWithoutSHA256RootSupport is always false on non-Windows
// platforms; the SHA-256 root fallback is a Windows-only concern.
func isLegacyWindowsWithoutSHA256RootSupport() bool {
	return false
}
