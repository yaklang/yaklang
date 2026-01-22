package aiconfig

import "github.com/yaklang/yaklang/common/utils"

// Common errors for aiconfig package
var (
	// ErrNoConfigAvailable indicates no AI configuration is available for the requested tier
	ErrNoConfigAvailable = utils.Error("no AI configuration available for the requested tier")

	// ErrTieredConfigDisabled indicates tiered AI config is not enabled
	ErrTieredConfigDisabled = utils.Error("tiered AI configuration is not enabled")

	// ErrInvalidPolicy indicates an invalid routing policy
	ErrInvalidPolicy = utils.Error("invalid routing policy")
)
