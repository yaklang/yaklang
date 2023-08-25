package yakgit

var Exports = map[string]any{
	"SetProxy": SetProxy,
	"Clone":    clone,
	"Pull":     pull,
	"Fetch":    fetch,
	"Checkout": checkout,

	"auth":           WithUsernamePassword,
	"context":        WithContext,
	"depth":          WithDepth,
	"recursive":      WithRecuriveSubmodule,
	"remote":         WithRemote,
	"force":          WithForce,
	"verify":         WithVerifyTLS,
	"checkoutCreate": WithCheckoutCreate,
	"checkoutForce":  WithCheckoutForce,
	"checkoutKeep":   WithCheckoutKeep,
	"noFetchTags":    WithNoFetchTags,
	"fetchAllTags":   WithFetchAllTags,
}
