package core

// Language names compiler evidence in a native artifact. The container and
// instruction set remain independent dimensions. A report does not imply that
// every function in a mixed-language executable uses the same language or ABI.
//
// LanguageC is the default for a native artifact without compiler metadata. It
// does not add an ABI: C, assembly and every unrecognized image keep the
// platform calling convention and the platform string rules. LanguageRust and
// LanguageGo override those per function, never for a whole artifact.
type Language string

const (
	LanguageUnknown Language = "unknown"
	LanguageC       Language = "c"
	LanguageGo      Language = "go"
	LanguageRust    Language = "rust"
)

// Language confidence levels. Certain means a compiler-owned structure was
// found, such as a Go pclntab or a Rust v0 symbol. Likely means only indirect
// evidence was found, such as a runtime comment.
const (
	ConfidenceCertain = "certain"
	ConfidenceLikely  = "likely"
	ConfidenceNone    = "none"
)

// LanguageEvidence is one observed fact that supports a detection. Source names
// where the fact came from: "section", "symbol", "buildinfo", "comment" or
// "pclntab".
type LanguageEvidence struct {
	Source string `json:"source"`
	Detail string `json:"detail"`
}

// ABI is a language-level calling convention for functions the language owns.
// It is an override, not a complete model: the artifact keeps the platform
// calling convention (registers, stack direction, callee-save set) as its
// default, and a consumer applies this override only where ConventionLanguage
// is recorded.
//
// Parameters belong to the pair (platform, language), so a language that does
// not override them must leave the platform model untouched.
type ABI struct {
	// Registers replaces the platform argument and return registers. The frame
	// and stack fields come from the platform profile.
	Registers RegisterModel `json:"registers"`
	// NoCalleeSave reports that the language ABI has no ordinary callee-saved
	// registers. The frame pointer is still restored by its own convention.
	NoCalleeSave bool `json:"no_callee_save,omitempty"`
}

// LanguageReport is the language layer's result for one artifact. It records
// what was found, what overrides the platform defaults, and what could not be
// recovered. A nil value means the language layer did not run, which happens for
// bytecode and source artifacts.
type LanguageReport struct {
	Language   Language `json:"language"`
	Confidence string   `json:"confidence"`
	// Runtime is the compiler or runtime version the image reports, for example
	// "go1.27.4". It is empty when the image carries no readable version.
	Runtime  string             `json:"runtime,omitempty"`
	Evidence []LanguageEvidence `json:"evidence,omitempty"`
	// ABI is the language override of the platform calling convention. It
	// applies only to functions recorded as ConventionLanguage, excluding
	// platform-convention functions such as Go .abi0 wrappers. A nil value means
	// every function keeps the platform convention.
	ABI *ABI `json:"abi,omitempty"`
	// Strings is the string representation the language registered. Decoding a
	// value uses Strings.Decode; no consumer switches on the language name.
	Strings *Strings `json:"strings,omitempty"`
	// Notes lists what the layer could not recover. A consumer must not read a
	// missing capability as a working one.
	Notes []string `json:"notes,omitempty"`
}

// EvidenceOf returns the details recorded for one evidence source.
func (r *LanguageReport) EvidenceOf(source string) []string {
	if r == nil {
		return nil
	}
	result := make([]string, 0, len(r.Evidence))
	for _, item := range r.Evidence {
		if item.Source == source {
			result = append(result, item.Detail)
		}
	}
	return result
}

// Known reports whether a report names a language the layer recognizes.
func (r *LanguageReport) Known() bool {
	return r != nil && r.Language != "" && r.Language != LanguageUnknown
}
