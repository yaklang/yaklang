package codeaudit

import "strings"

// ProbeOptions controls audit behavior across all tools.
type ProbeOptions struct {
	Language            string   // "java" (default)
	DetectionMode       string   // "permissive" | "balanced" | "strict"
	MinConfidence       float64  // 0 = use detection-mode default
	CmsMinConfidence    float64  // 0 = use detection-mode default
	ScopeModules        []string // sub-module directory names
	ScopeExclude        []string // excluded path fragments
	IncludeFrameworks   []string // force-include frameworks
	ExcludeFrameworks   []string // force-exclude frameworks
	ToolProfile         string   // "full" | "minimal" | "config-only" | "deps-secrets"
	ResolveMonorepoRoot bool
	CmsProducts         []string // forced CMS ids
	ConfigScope         string   // "framework" | "all"
	RiskyMode           string   // "name" | "off"
	DedupeFindings      bool
	MaxFindingsPerRule  int
}

// DefaultProbeOptions returns the default options.
func DefaultProbeOptions() *ProbeOptions {
	return &ProbeOptions{
		Language:           "java",
		DetectionMode:      "balanced",
		MinConfidence:      0,
		CmsMinConfidence:   0,
		ScopeModules:        []string{},
		ScopeExclude:       []string{},
		IncludeFrameworks:  []string{},
		ExcludeFrameworks:  []string{},
		ToolProfile:        "full",
		ConfigScope:        "framework",
		RiskyMode:           "name",
		DedupeFindings:      true,
		MaxFindingsPerRule:  5,
	}
}

// ProbeOption is a functional option for ProbeOptions.
type ProbeOption func(*ProbeOptions)

// applyOptions merges functional options into a default ProbeOptions.
func applyOptions(opts ...ProbeOption) *ProbeOptions {
	o := DefaultProbeOptions()
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// --- Functional options (exported for yak) ---

// WithLanguage sets the target language.
func WithLanguage(lang string) ProbeOption {
	return func(o *ProbeOptions) { o.Language = lang }
}

// WithDetectionMode sets the detection mode.
func WithDetectionMode(mode string) ProbeOption {
	return func(o *ProbeOptions) { o.DetectionMode = mode }
}

// WithScopeModules sets scope module filter.
func WithScopeModules(modules string) ProbeOption {
	return func(o *ProbeOptions) { o.ScopeModules = splitCSV(modules) }
}

// WithScopeExclude sets scope exclude filter.
func WithScopeExclude(exclude string) ProbeOption {
	return func(o *ProbeOptions) { o.ScopeExclude = splitCSV(exclude) }
}

// WithCmsProducts sets forced CMS product ids.
func WithCmsProducts(products string) ProbeOption {
	return func(o *ProbeOptions) { o.CmsProducts = splitCSV(products) }
}

// WithDedupeFindings toggles finding deduplication.
func WithDedupeFindings(dedupe bool) ProbeOption {
	return func(o *ProbeOptions) { o.DedupeFindings = dedupe }
}

// WithDedupeFindingsRaw accepts a string "true"/"false" for yak compatibility.
func WithDedupeFindingsRaw(s string) ProbeOption {
	return func(o *ProbeOptions) { o.DedupeFindings = s != "false" }
}

// WithRiskyMode sets the risky component matching mode.
func WithRiskyMode(mode string) ProbeOption {
	return func(o *ProbeOptions) { o.RiskyMode = mode }
}

// WithConfigScope sets config audit scope.
func WithConfigScope(scope string) ProbeOption {
	return func(o *ProbeOptions) { o.ConfigScope = scope }
}

// WithIncludeFrameworks forces inclusion of frameworks.
func WithIncludeFrameworks(frameworks string) ProbeOption {
	return func(o *ProbeOptions) { o.IncludeFrameworks = splitCSV(frameworks) }
}

// WithExcludeFrameworks forces exclusion of frameworks.
func WithExcludeFrameworks(frameworks string) ProbeOption {
	return func(o *ProbeOptions) { o.ExcludeFrameworks = splitCSV(frameworks) }
}

// splitCSV splits a comma-separated string into a slice, trimming whitespace.
func splitCSV(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
