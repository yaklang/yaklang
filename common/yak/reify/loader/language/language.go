// Package language is the compiler-metadata layer above container loaders.
// It recovers symbols without decoding instructions or registering new formats.
package language

import (
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
	"github.com/yaklang/yaklang/common/yak/reify/loader/language/c"
	"github.com/yaklang/yaklang/common/yak/reify/loader/language/golang"
	"github.com/yaklang/yaklang/common/yak/reify/loader/language/rust"
)

// Language describes one language layer: how to recognize its artifacts, how to
// recover the names and ranges of the functions it owns, and which conventions
// replace the platform defaults for those functions.
//
// Detection and recovery belong to the same owner on purpose. A detector that
// cannot recover would otherwise become a label without behavior, and a
// recoverer without a detector would have no way to prove its input matches.
type Language interface {
	Kind() core.Language
	// Strings is the representation this language registers. Decoding uses the
	// model, never the language name.
	Strings() core.Strings
	// Detect returns nil when the image holds no evidence for this language.
	Detect(*core.Artifact) *core.LanguageReport
	// Recover returns the functions this language can name. It must not decode
	// instructions or read outside Artifact.Reader.
	Recover(*core.Artifact) ([]symbol.Symbol, error)
}

// recoveryEvidence lets a language describe what its own recovery found. A
// language that does not implement it adds no recovery line to the report.
type recoveryEvidence interface {
	RecoveryEvidence(count int) core.LanguageEvidence
}

// languages lists every registered layer. The C layer is first and always
// detects with the lowest confidence, so Inspect always has a result.
var languages = []Language{c.C{}, golang.Golang{}, rust.Rust{}}

// languageFor returns the layer for a kind. An unknown kind resolves to the
// default C layer: a native artifact always has a language.
func languageFor(kind core.Language) Language {
	for _, candidate := range languages {
		if candidate.Kind() == kind {
			return candidate
		}
	}
	return c.C{}
}

func confidenceRank(confidence string) int {
	switch confidence {
	case core.ConfidenceCertain:
		return 3
	case core.ConfidenceLikely:
		return 2
	case core.ConfidenceNone:
		return 1
	}
	return 0
}

// Inspect runs before an artifact is exposed to consumers, including before
// Image.SymbolsAt builds its index. Every native artifact gets a report: the C
// layer is the fallback, with no ABI override and a NUL-terminated string
// model. Evidence
// describes compiler presence, not proof that every function in a mixed-language
// executable has the same language or ABI.
func Inspect(a *core.Artifact) *core.LanguageReport {
	if a == nil || a.Image == nil || a.Kind != core.KindNative {
		return nil
	}
	if a.Language != nil {
		return a.Language
	}
	chosen := Language(c.C{})
	report := chosen.Detect(a)
	for _, candidate := range languages {
		detected := candidate.Detect(a)
		if detected == nil || detected.Language != candidate.Kind() {
			continue
		}
		if confidenceRank(detected.Confidence) > confidenceRank(report.Confidence) {
			report, chosen = detected, candidate
		}
	}
	recovered, err := chosen.Recover(a)
	switch {
	case err != nil:
		report.Notes = append(report.Notes, fmt.Sprintf("symbol recovery failed: %v", err))
		// A section name alone is not evidence of a valid table. Without a
		// compiler version, fall back to the default layer and keep the note.
		if report.Runtime == "" {
			failure := report.Notes
			chosen = c.C{}
			report = chosen.Detect(a)
			report.Notes = append(report.Notes, failure...)
		}
	default:
		if len(recovered) > 0 {
			report.Confidence = core.ConfidenceCertain
			if reporter, ok := chosen.(recoveryEvidence); ok {
				report.Evidence = append(report.Evidence, reporter.RecoveryEvidence(len(recovered)))
			}
			mergeSymbols(a, recovered)
		}
	}
	// The model comes from the layer that won, including the C fallback.
	model := chosen.Strings()
	report.Strings = &model
	// A Go image can link Rust code and a Rust image can export Go code. Naming
	// those functions does not change the detected language or its ABI.
	if report.Language != core.LanguageRust {
		if foreign, err := (rust.Rust{}).Recover(a); err == nil && len(foreign) > 0 {
			mergeSymbols(a, foreign)
		}
	}
	disambiguateRustSymbols(a)
	if failed := undecodedRustSymbols(a); failed > 0 {
		report.Notes = append(report.Notes, fmt.Sprintf("%d Rust symbols retained their encoded name because demangling failed", failed))
	}
	if report.Language == core.LanguageGo && report.ABI == nil {
		report.Notes = append(report.Notes, "Go ABIInternal is not modelled for this architecture or compiler version")
	}
	a.Image.SetSymbols(a.Image.Symbols)
	a.Language = report
	return report
}

// Of returns the language recorded on an artifact.
func Of(a *core.Artifact) core.Language {
	if a == nil || a.Language == nil {
		return core.LanguageUnknown
	}
	return a.Language.Language
}

// mergeSymbols applies recovered names to the container symbol table. A
// recovered symbol replaces the name and provenance of the symbol with the same
// encoded name or the same display name, and is appended otherwise. The
// container's exact size wins when it has one: a compiler table often records
// the next entry, padding included.
func mergeSymbols(a *core.Artifact, recovered []symbol.Symbol) {
	type key struct {
		name    string
		address uint64
	}
	known := make(map[key]int, len(a.Image.Symbols))
	for i, s := range a.Image.Symbols {
		known[key{s.Name, s.Address}] = i
		if s.Mangled != "" {
			known[key{s.Mangled, s.Address}] = i
		}
	}
	for _, s := range recovered {
		index, ok := known[key{s.Mangled, s.Address}]
		if !ok || s.Mangled == "" {
			index, ok = known[key{s.Name, s.Address}]
		}
		if ok {
			existing := &a.Image.Symbols[index]
			if s.Name != "" && s.Name != existing.Name {
				if existing.Mangled == "" {
					existing.Mangled = existing.Name
				}
				existing.Name = s.Name
			} else if existing.Mangled == "" && s.Mangled != "" {
				existing.Mangled = s.Mangled
			}
			existing.Source = s.Source
			existing.Convention = s.Convention
			if existing.Size == 0 {
				existing.Size = s.Size
			}
			if existing.Kind == symbol.SymbolUnknown {
				existing.Kind = s.Kind
			}
			continue
		}
		known[key{s.Name, s.Address}] = len(a.Image.Symbols)
		if s.Mangled != "" {
			known[key{s.Mangled, s.Address}] = len(a.Image.Symbols)
		}
		a.Image.Symbols = append(a.Image.Symbols, s)
	}
	sort.SliceStable(a.Image.Symbols, func(i, j int) bool { return a.Image.Symbols[i].Address < a.Image.Symbols[j].Address })
}

// disambiguateRustSymbols separates decoded names that collide. The decoder
// omits crate hashes, so two monomorphizations can share a display name; merging
// them would create one SSA function with two bodies.
func disambiguateRustSymbols(a *core.Artifact) {
	addresses := make(map[string]uint64)
	ambiguous := make(map[string]bool)
	for i := range a.Image.Symbols {
		s := &a.Image.Symbols[i]
		if s.Source != symbol.SourceRustDemangle {
			continue
		}
		if address, exists := addresses[s.Name]; exists && address != s.Address {
			ambiguous[s.Name] = true
		}
		addresses[s.Name] = s.Address
	}
	for i := range a.Image.Symbols {
		s := &a.Image.Symbols[i]
		if s.Source == symbol.SourceRustDemangle && ambiguous[s.Name] {
			s.Name = fmt.Sprintf("%s@0x%x", s.Name, s.Address)
		}
	}
}

// undecodedRustSymbols counts encoded Rust names the decoder did not handle.
// They keep their linker name so a caller still sees the address and size.
func undecodedRustSymbols(a *core.Artifact) int {
	count := 0
	for i := range a.Image.Symbols {
		s := &a.Image.Symbols[i]
		if s.Source == symbol.SourceRustDemangle || s.Mangled != "" {
			continue
		}
		if rust.IsMangled(s.Name) {
			count++
		}
	}
	return count
}
