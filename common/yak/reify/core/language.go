package core

// Language names compiler evidence in a native artifact. The container and
// instruction set remain independent dimensions. A report does not imply that
// every function in a mixed-language executable uses the same language or ABI.
type Language string

const (
	LanguageUnknown Language = "unknown"
	LanguageC       Language = "c"
	LanguageGo      Language = "go"
	LanguageRust    Language = "rust"
)

// Language confidence levels. Certain means a compiler-owned structure was
// found, such as a Go pclntab or a Rust v0 symbol. Likely means only indirect
// evidence was found, such as runtime symbols or a section name.
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

// LanguageReport is the language layer's result for one artifact. It records
// what was found, what overrides the container defaults, and what could not be
// recovered. A nil value means the language layer did not run, which happens for
// bytecode and source artifacts.
type LanguageReport struct {
	Language   Language `json:"language"`
	Confidence string   `json:"confidence"`
	// Runtime is the compiler or runtime version the image reports, for example
	// "go1.27.4". It is empty when the image carries no readable version.
	Runtime  string             `json:"runtime,omitempty"`
	Evidence []LanguageEvidence `json:"evidence,omitempty"`
	// Registers is the scalar integer ABIInternal model for a known Go version.
	// It applies only to functions recovered from pclntab, excluding ABI0. It
	// must never replace the architecture defaults for the whole artifact.
	Registers *RegisterModel `json:"registers,omitempty"`
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
