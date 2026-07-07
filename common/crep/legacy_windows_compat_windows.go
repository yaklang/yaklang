//go:build windows

package crep

import "golang.org/x/sys/windows"

// isLegacyWindowsWithoutSHA256RootSupport reports whether the current Windows
// version cannot reliably validate SHA-256 signed root certificates in the
// system trust store.
//
// Native SHA-256 root certificate support landed in Windows 8 (NT 6.2).
// Windows 7 / Server 2008 R2 (NT 6.1) and earlier require KB3033929, which is
// frequently absent on long-unpatched installs (exactly the old machines this
// tool tends to run on). On such systems a SHA-256 MITM root installs without
// error but is silently rejected by browsers/crypto, making MITM unusable. We
// detect these versions so callers can fall back to a SHA-1 signed root.
//
// RtlGetVersion is used instead of GetVersion/GetVersionEx because the latter
// is manifest-gated and lies on modern Windows; RtlGetVersion reports the true
// OS version regardless of the calling process's manifest.
func isLegacyWindowsWithoutSHA256RootSupport() bool {
	info := windows.RtlGetVersion()
	if info == nil {
		// Cannot determine version; assume modern to avoid degrading security.
		return false
	}
	// Anything older than Windows 8 (NT 6.2) lacks native SHA-256 root support.
	if info.MajorVersion < 6 {
		return true
	}
	if info.MajorVersion == 6 && info.MinorVersion < 2 {
		return true
	}
	return false
}
