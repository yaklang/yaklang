package compiler

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/profile"
)

func prepareCompileConfig(cfg *CompileConfig) error {
	if cfg == nil {
		return fmt.Errorf("compile failed: nil config")
	}
	if err := applyCompileProfile(cfg); err != nil {
		return err
	}
	return nil
}

func applyCompileProfile(cfg *CompileConfig) error {
	name := strings.TrimSpace(cfg.ProfileName)
	if name == "" {
		return nil
	}

	p, ok := profile.Get(name)
	if !ok {
		return fmt.Errorf("unknown compile profile %q", name)
	}
	if err := p.Validate(); err != nil {
		return err
	}

	cfg.Obfuscators = appendObfuscatorNames(cfg.Obfuscators, p.ObfuscatorNames()...)

	if len(p.LLVMPacks) > 0 && strings.TrimSpace(cfg.LLVMPack) == "" {
		cfg.LLVMPack = p.LLVMPacks[0]
	}

	// Generate build seed from the profile's SeedPolicy.
	switch p.SeedPolicy {
	case profile.SeedPerBuild:
		seed := make([]byte, 32)
		if _, err := rand.Read(seed); err != nil {
			return fmt.Errorf("generate build seed: %w", err)
		}
		cfg.BuildSeed = seed
	case profile.SeedFixed:
		// When SeedFixed and the caller hasn't set a seed, use a
		// deterministic zero seed.  The user can override via CLI.
		if len(cfg.BuildSeed) == 0 {
			cfg.BuildSeed = make([]byte, 32)
		}
	}

	// If no explicit --obf-policy file is set, use the profile's default
	// policy entries (if any) to drive function selection.
	if strings.TrimSpace(cfg.ObfPolicyFile) == "" {
		cfg.profilePolicy = p.DefaultPolicy()
	}

	return nil
}
