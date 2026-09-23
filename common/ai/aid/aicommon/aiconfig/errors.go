package aiconfig

import "github.com/yaklang/yaklang/common/utils"

// Common errors for aiconfig package
var (
	// ErrNoConfigAvailable indicates no AI configuration is available for the requested tier
	ErrNoConfigAvailable = utils.Error("no AI configuration available for the requested tier")

	// ErrTieredConfigDisabled is retained for compatibility and indicates that
	// no tiered AI configuration is available.
	ErrTieredConfigDisabled = utils.Error("tiered AI configuration is not available")

	// ErrInvalidPolicy indicates an invalid routing policy
	ErrInvalidPolicy = utils.Error("invalid routing policy")
)
