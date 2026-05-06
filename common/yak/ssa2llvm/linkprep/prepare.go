// Package linkprep performs link-time transformations on static archives before the
// system linker runs. It is separate from the SSA/LLVM obfuscation subsystem: it does
// not call obfuscation.Apply and does not register KindLLVM obfuscators.
//
// Compiler integration should use PrepareForLink as the single entry point.
package linkprep

import (
	"path/filepath"
	"strings"
)

// PrepareInput is the single compiler-facing contract for the pre-link stage.
// It does not run SSA/LLVM obfuscation; see package comment and docs/link-prep.md.
type PrepareInput struct {
	// Archives lists static library paths: first is libyak.a, then optional libyakobf_*.a.
	Archives []string
	// Manifest maps canonical runtime symbol names to per-build link names; empty means no-op.
	Manifest map[string]string
	WorkDir  string
	Trace    bool
}

// PrepareForLink rewrites archive members when Manifest is non-empty, otherwise returns
// Archives unchanged with a no-op cleanup. This is the only entry the compiler should use
// for link-prep (no obfuscation.Apply, no KindLLVM registration).
func PrepareForLink(in PrepareInput) (archives []string, cleanup func(), err error) {
	if len(in.Manifest) == 0 {
		return append([]string{}, in.Archives...), func() {}, nil
	}
	inPath := make([]string, 0, len(in.Archives))
	for _, p := range in.Archives {
		p = strings.TrimSpace(p)
		if p != "" {
			inPath = append(inPath, filepath.Clean(p))
		}
	}
	return RewriteArchives(inPath, in.Manifest, in.WorkDir, in.Trace)
}
