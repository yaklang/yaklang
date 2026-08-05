package schema

// SyntaxFlow package/group source kinds (catalog metadata on SyntaxFlowGroup.Source).
const (
	SyntaxFlowPackageSourceEmbed  = "embed"
	SyntaxFlowPackageSourceOnline = "online"
	SyntaxFlowPackageSourceLocal  = "local"
	SyntaxFlowPackageSourceUser   = "user"
)

// Well-known rule-group / package-bucket names (stored in SyntaxFlowRule.RuleGroup).
const (
	SyntaxFlowPackageBuiltin = "builtin"
	SyntaxFlowPackageAgent   = "agent"
	SyntaxFlowPackageCustom  = "custom"
)

// Note: SyntaxFlowPackage table was dropped from the model. Package capabilities
// use SyntaxFlowGroup catalog + Rule.RuleGroup. Old syntax_flow_packages rows
// (if any) are left unused for soft DB compatibility.
