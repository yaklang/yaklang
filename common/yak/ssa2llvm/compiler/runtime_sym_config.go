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
		// No --profile: keep legacy stable runtime symbols (tests, quick compiles).
		return false
	}
	if p.LinkPrep == nil {
		// Profile loaded but link_prep omitted: default on for fingerprinting.
		return true
	}
	if p.LinkPrep.RandomizeRuntimeSymbols == nil {
		return true
	}
	return *p.LinkPrep.RandomizeRuntimeSymbols
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
