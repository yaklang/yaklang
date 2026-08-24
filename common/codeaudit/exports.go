package codeaudit

// Exports is the map registered as the yak built-in library "codeaudit".
var Exports = map[string]any{
	// Core functions
	"ProbeProject":      ProbeProject,      // project probing
	"ScanSecrets":       ScanSecrets,       // secret scanning
	"AuditConfig":       AuditConfig,       // config auditing
	"ScanDependencies":  ScanDependencies,   // dependency SCA
	"RunFrameworkAudit": RunFrameworkAudit, // framework architecture baseline
	"AuditCmsProduct":   AuditCmsProduct,   // CMS product auditing

	// Options (lowercase for yak convention)
	"withLanguage":         WithLanguage,
	"withDetectionMode":    WithDetectionMode,
	"withScopeModules":     WithScopeModules,
	"withScopeExclude":     WithScopeExclude,
	"withCmsProducts":      WithCmsProducts,
	"withDedupeFindings":   WithDedupeFindings,
	"withDedupeFindingsRaw": WithDedupeFindingsRaw,
	"withRiskyMode":        WithRiskyMode,
	"withConfigScope":      WithConfigScope,
	"withIncludeFrameworks": WithIncludeFrameworks,
	"withExcludeFrameworks": WithExcludeFrameworks,
}
