package yakgit

var Exports = map[string]any{
	"SetProxy": SetProxy,

	// githack
	"GitHack":       GitHack,
	"Clone":         Clone,
	"Pull":          pull,
	"Fetch":         fetch,
	"Checkout":      checkout,
	"IterateCommit": EveryCommit,

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

	// inspect
	"handleCommit":    WithHandleGitCommit,
	"filterCommit":    WithFilterGitCommit,
	"handleReference": WithHandleGitReference,
	"filterReference": WithFilterGitReference,

	"threads":           WithThreads,
	"useLocalGitBinary": WithUseLocalGitBinary,
	"httpOpts":          WithHTTPOptions,
}
