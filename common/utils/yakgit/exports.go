package yakgit

var Exports = map[string]any{
	"SetProxy": SetProxy,

	"RevParse":        RevParse,
	"HeadHash":        GetHeadHash,
	"HeadBranch":      GetHeadBranch,
	"HeadBranchRange": GetBranchRange,
	"ParentHash":      GetParentCommitHash,
	"Glance":          Glance,
	"Branch":          GetAllBranches,
	"Blame":           Blame,
	"BlameCommit":     BlameWithCommit,

	// githack
	"GitHack":       GitHack,
	"Clone":         Clone,
	"Pull":          pull,
	"Fetch":         fetch,
	"Checkout":      checkout,
	"IterateCommit": EveryCommit,

	"FileSystemFromCommit":          FromCommit,
	"FileSystemFromCommits":         FromCommits,
	"FileSystemFromCommitRange":     FromCommitRange,
	"FileSystemFromCommitDateRange": FileSystemFromCommitDateRange,
	"FileSystemCurrentWeek":         FileSystemCurrentWeek,
	"FileSystemLastSevenDay":        FileSystemLastSevenDay,
	"FileSystemCurrentDay":          FileSystemCurrentDay,
	"FileSystemCurrentMonth":        FileSystemCurrentMonth,
	"FileSystemFromDate":            FileSystemFromDate,
	"FileSystemFromMonth":           FileSystemFromMonth,

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
