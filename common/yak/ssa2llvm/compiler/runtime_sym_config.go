package compiler

import (
	crand "crypto/rand"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/linkprep"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation"
)

func effectiveRandomizeRuntimeSymbols(cfg *CompileConfig) bool {
	if cfg == nil {
		return false
	}
	p := cfg.resolvedProfile
	if p == nil {
		// No --profile: keep stable runtime symbols (tests, quick compiles).
		return false
	}
	// Runtime symbol randomization (link_prep) rewrites the embedded runtime
	// archive's symbols using the host binutils (ar/objcopy/nm). That is a
	// host-toolchain dependency incompatible with the zero-dependency
	// self-contained build, which is now the only build mode. It is therefore
	// disabled: obfuscation transforms (callret/virtualize/mba/...) from a
	// profile still apply, but the runtime symbols are not randomized. The
	// linkprep package and the profile LinkPrep config are retained for a future
	// build mode that can assume host binutils.
	_ = p.LinkPrep
	return false
}

func finalizeRuntimeSymManifest(cfg *CompileConfig) error {
	if cfg == nil {
		return fmt.Errorf("compile failed: nil config")
	}
	if !effectiveRandomizeRuntimeSymbols(cfg) {
		cfg.RuntimeSymManifest = nil
		return nil
	}
	seed := cfg.BuildSeed
	if len(seed) < 16 {
		seed = make([]byte, 32)
		if _, err := crand.Read(seed); err != nil {
			return fmt.Errorf("linkprep: generate seed: %w", err)
		}
	}
	m, err := linkprep.BuildManifest(seed)
	if err != nil {
		return err
	}
	cfg.RuntimeSymManifest = m
	return nil
}

func patchObfuscationRuntimeSymbols(ctx *obfuscation.Context, m map[string]string) {
	if ctx == nil || len(m) == 0 {
		return
	}
	for _, w := range ctx.FunctionWrappers {
		if w == nil {
			continue
		}
		if sym, ok := m[w.RuntimeSymbol]; ok && sym != "" {
			w.RuntimeSymbol = sym
		}
	}
}
