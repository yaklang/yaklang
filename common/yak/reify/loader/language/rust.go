package language

import (
	"bytes"
	"strings"

	"github.com/ianlancetaylor/demangle"
	"github.com/yaklang/yaklang/common/yak/reify/core"
)

type rustDetector struct{}

func (rustDetector) Name() string { return "rust" }

func (rustDetector) Detect(a *core.Artifact) *core.LanguageReport {
	r := &core.LanguageReport{Language: core.LanguageRust, Confidence: core.ConfidenceLikely,
		Notes: []string{
			"Rust trait-object dispatch, panic/unwind edges and Rust type layouts are not reconstructed",
			"Rust calling conventions are not stable; scalar examples use the architecture ABI as an approximation",
			"without retained symbol names this layer cannot recover Rust function names",
		}}
	for _, s := range a.Image.Symbols {
		name := s.Name
		if s.Mangled != "" {
			name = s.Mangled
		}
		if _, ok := DemangleRust(name); ok && !s.Imported {
			r.Confidence = core.ConfidenceCertain
			r.Evidence = append(r.Evidence, core.LanguageEvidence{Source: "symbol", Detail: name})
			break
		}
	}
	if a.Reader != nil {
		for _, s := range a.Image.Sections {
			if s.Name != ".comment" || s.FileSize == 0 || s.FileSize > 64<<10 {
				continue
			}
			data, err := a.Reader.Bytes(s.FileOffset, s.FileSize)
			if err != nil {
				continue
			}
			for _, line := range bytes.Split(data, []byte{0}) {
				if bytes.HasPrefix(line, []byte("rustc version ")) {
					r.Runtime = string(line)
					r.Evidence = append(r.Evidence, core.LanguageEvidence{Source: "comment", Detail: string(line)})
				}
			}
		}
	}
	if len(r.Evidence) == 0 {
		return nil
	}
	return r
}

// IsRustMangled is a candidate filter, not a detector. v0 names must also parse
// successfully. Legacy names share the C++ prefix, so require the Rust hash
// component rather than treating every _ZN symbol as Rust.
func IsRustMangled(name string) bool {
	name = rustLinkName(name)
	if strings.HasPrefix(name, "_R") {
		return true
	}
	if !strings.HasPrefix(name, "_ZN") {
		return false
	}
	if end := strings.LastIndex(name, "E."); end >= 0 {
		name = name[:end+1]
	}
	if len(name) < 23 || !strings.HasSuffix(name, "E") || name[len(name)-20:len(name)-17] != "17h" {
		return false
	}
	for _, c := range name[len(name)-17 : len(name)-1] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Mach-O can add a leading underscore to the linker name. Preserve the exact
// original in Symbol.Mangled; normalize only the string passed to demangle.
func rustLinkName(name string) string {
	if strings.HasPrefix(name, "__R") || strings.HasPrefix(name, "__ZN") {
		return name[1:]
	}
	return name
}

// DemangleRust accepts only Rust candidates; a C++ name must not turn into Rust
// detection evidence. This is a pure Go decoder, with no external executable.
func DemangleRust(name string) (string, bool) {
	if len(name) > 64<<10 || !IsRustMangled(name) {
		return "", false
	}
	out, err := demangle.ToString(rustLinkName(name), demangle.MaxLength(16))
	if err != nil || out == "" || len(out) >= 1<<16 {
		return "", false
	}
	return out, true
}
