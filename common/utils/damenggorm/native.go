package damenggorm

import "github.com/yaklang/gorm/dialects/dameng"

// ErrNativeUnavailable preserves the generic driver's error identity.
var ErrNativeUnavailable = dameng.ErrNativeUnavailable

// NativeEnabled reports whether the binary was built with -tags dameng.
func NativeEnabled() bool { return dameng.NativeEnabled() }
