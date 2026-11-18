// Package language is the compiler-metadata layer above container loaders.
// It recovers symbols without decoding instructions or registering new formats.
package language

import (
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// Detector recognizes compiler evidence in an already loaded image. A nil
// report means no evidence, not that the image is a C program.
type Detector interface {
	Name() string
	Detect(*core.Artifact) *core.LanguageReport
}

// SymbolRecoverer reads named function ranges from compiler metadata. It never
// modifies the container's address mapping or reads beyond Artifact.Reader.
type SymbolRecoverer interface {
	Recover(*core.Artifact) ([]symbol.Symbol, error)
}

var detectors = []Detector{goDetector{}, rustDetector{}}
var recoverers = map[core.Language]SymbolRecoverer{core.LanguageGo: goSymbols{}}

// Inspect runs before an artifact is exposed to consumers, including before
// Image.SymbolsAt builds its index. Unknown inputs keep the existing C-oriented
// architecture defaults. Evidence describes compiler presence, not proof that
// every function in a mixed-language executable has the same language or ABI.
func Inspect(a *core.Artifact) *core.LanguageReport {
	if a == nil || a.Image == nil || a.Kind != core.KindNative {
		return nil
	}
	if a.Language != nil {
		return a.Language
	}
	report := &core.LanguageReport{Language: core.LanguageUnknown, Confidence: core.ConfidenceNone}
	for _, detector := range detectors {
		candidate := detector.Detect(a)
		if candidate != nil && (!report.Known() || candidate.Confidence == core.ConfidenceCertain && report.Confidence != core.ConfidenceCertain) {
			report = candidate
		}
	}
	if recoverer, ok := recoverers[report.Language]; ok {
		recovered, err := recoverer.Recover(a)
		if err != nil {
			report.Notes = append(report.Notes, fmt.Sprintf("symbol recovery failed: %v", err))
			// A section name alone is not evidence of a valid Go table.
			if report.Runtime == "" {
				report.Language, report.Confidence = core.LanguageUnknown, core.ConfidenceNone
			}
		} else {
			report.Confidence = core.ConfidenceCertain
			report.Evidence = append(report.Evidence, core.LanguageEvidence{Source: symbol.SourcePCLNTab, Detail: fmt.Sprintf("%d functions", len(recovered))})
			mergeSymbols(a, recovered)
		}
	}
	// Demangle individual Rust symbols even in a Go image containing Rust code.
	// Do not apply the detected language's ABI to foreign functions.
	normalizeRustSymbols(a, report)
	if report.Language == core.LanguageGo && report.Registers == nil {
		report.Notes = append(report.Notes, "Go ABIInternal is not modelled for this architecture or compiler version")
	}
	a.Image.SetSymbols(a.Image.Symbols)
	a.Language = report
	return report
}

func Language(a *core.Artifact) core.Language {
	if a == nil || a.Language == nil {
		return core.LanguageUnknown
	}
	return a.Language.Language
}

func mergeSymbols(a *core.Artifact, recovered []symbol.Symbol) {
	type key struct {
		name string
		addr uint64
	}
	known := make(map[key]int, len(a.Image.Symbols))
	for i, s := range a.Image.Symbols {
		known[key{s.Name, s.Address}] = i
	}
	for _, s := range recovered {
		k := key{s.Name, s.Address}
		if i, ok := known[k]; ok {
			// Record provenance even when .symtab already supplied the name.
			// Keep an exact ELF size instead of pclntab's alignment padding.
			a.Image.Symbols[i].Source = symbol.SourcePCLNTab
			a.Image.Symbols[i].Kind = symbol.SymbolFunction
			if a.Image.Symbols[i].Size == 0 {
				a.Image.Symbols[i].Size = s.Size
			}
			continue
		}
		known[k] = len(a.Image.Symbols)
		a.Image.Symbols = append(a.Image.Symbols, s)
	}
	sort.SliceStable(a.Image.Symbols, func(i, j int) bool { return a.Image.Symbols[i].Address < a.Image.Symbols[j].Address })
}

func normalizeRustSymbols(a *core.Artifact, report *core.LanguageReport) {
	failed := 0
	addresses := make(map[string]uint64)
	ambiguous := make(map[string]bool)
	for i := range a.Image.Symbols {
		s := &a.Image.Symbols[i]
		if s.Mangled != "" {
			continue
		}
		readable, ok := DemangleRust(s.Name)
		if !ok {
			if IsRustMangled(s.Name) {
				failed++
			}
			continue
		}
		s.Mangled, s.Name, s.Source = s.Name, readable, symbol.SourceRustDemangle
		if address, exists := addresses[readable]; exists && address != s.Address {
			ambiguous[readable] = true
		}
		addresses[readable] = s.Address
	}
	// The demangler omits crate hashes. Disambiguate colliding display names
	// rather than merging distinct monomorphizations into one SSA function.
	for i := range a.Image.Symbols {
		s := &a.Image.Symbols[i]
		if s.Mangled != "" && ambiguous[s.Name] {
			s.Name = fmt.Sprintf("%s@0x%x", s.Name, s.Address)
		}
	}
	if failed > 0 {
		report.Notes = append(report.Notes, fmt.Sprintf("%d Rust symbols retained their encoded name because demangling failed", failed))
	}
}
