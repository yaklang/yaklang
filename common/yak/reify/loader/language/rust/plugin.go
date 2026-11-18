package rust

import (
	"bytes"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// Rust is the Rust language layer. Detection and recovery share one owner so a
// report never labels an image whose symbols this package cannot handle.
type Rust struct{}

func (Rust) Kind() core.Language { return core.LanguageRust }

func (Rust) Detect(a *core.Artifact) *core.LanguageReport {
	report := &core.LanguageReport{Language: core.LanguageRust, Confidence: core.ConfidenceLikely,
		Notes: []string{
			"Rust trait-object dispatch, panic/unwind edges and Rust type layouts are not reconstructed",
			"Rust calling conventions are not stable; scalar examples use the platform ABI as an approximation",
			"without retained symbol names this layer cannot recover Rust function names",
		}}
	for _, s := range a.Image.Symbols {
		name := s.Name
		if s.Mangled != "" {
			name = s.Mangled
		}
		if _, ok := Demangle(name); ok && !s.Imported {
			report.Confidence = core.ConfidenceCertain
			report.Evidence = append(report.Evidence, core.LanguageEvidence{Source: "symbol", Detail: name})
			break
		}
	}
	if a.Reader != nil {
		report.Runtime, report.Evidence = appendRustcVersion(a, report.Evidence)
	}
	if len(report.Evidence) == 0 {
		return nil
	}
	return report
}

// appendRustcVersion reads the compiler version from .comment. The section is
// plain text, so a missing or oversized one is not an error.
func appendRustcVersion(a *core.Artifact, evidence []core.LanguageEvidence) (string, []core.LanguageEvidence) {
	version := ""
	for _, section := range a.Image.Sections {
		if section.Name != ".comment" || section.FileSize == 0 || section.FileSize > 64<<10 {
			continue
		}
		data, err := a.Reader.Bytes(section.FileOffset, section.FileSize)
		if err != nil {
			continue
		}
		for _, line := range bytes.Split(data, []byte{0}) {
			if bytes.HasPrefix(line, []byte("rustc version ")) {
				version = string(line)
				evidence = append(evidence, core.LanguageEvidence{Source: "comment", Detail: version})
			}
		}
	}
	return version, evidence
}

// Recover turns the container's encoded Rust names into display names. It adds
// no address and does not guess: a name the decoder rejects keeps its linker
// form and is counted as a limit by the caller.
func (Rust) Recover(a *core.Artifact) ([]symbol.Symbol, error) {
	var recovered []symbol.Symbol
	for _, s := range a.Image.Symbols {
		if s.Imported || s.Address == 0 || s.Source == symbol.SourceRustDemangle {
			continue
		}
		encoded := s.Name
		if s.Mangled != "" {
			encoded = s.Mangled
		}
		readable, ok := Demangle(encoded)
		if !ok {
			continue
		}
		kind := s.Kind
		if kind == symbol.SymbolUnknown {
			kind = symbol.SymbolFunction
		}
		recovered = append(recovered, symbol.Symbol{Name: readable, Mangled: encoded, Address: s.Address,
			Size: s.Size, Kind: kind, Source: symbol.SourceRustDemangle})
	}
	return recovered, nil
}

// Strings reports the &str layout: a pointer and a length. A String adds a
// capacity word to the same pointer and length; both are UTF-8.
func (Rust) Strings() core.Strings {
	return core.Strings{Layout: core.StringLayoutPointerLength, Encoding: core.StringEncodingUTF8, ValidateUTF8: true}
}
