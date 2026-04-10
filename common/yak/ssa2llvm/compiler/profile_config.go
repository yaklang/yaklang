package compiler

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation"
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
	ref := strings.TrimSpace(cfg.ProfileName)
	if ref == "" {
		return nil
	}

	p, err := profile.LoadRef(ref)
	if err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}

	extraObfs, err := expandCompileObfuscatorPatterns(cfg.Obfuscators)
	if err != nil {
		return err
	}
	mergeProfileExtraObfuscators(p, extraObfs)

	if len(p.LLVMPacks) > 0 && strings.TrimSpace(cfg.LLVMPack) == "" {
		cfg.LLVMPack = p.LLVMPacks[0]
	}

	switch p.NormalizedSeedPolicy() {
	case profile.SeedPerBuild:
		if len(cfg.BuildSeed) == 0 {
			seed := make([]byte, 32)
			if _, err := rand.Read(seed); err != nil {
				return fmt.Errorf("generate build seed: %w", err)
			}
			cfg.BuildSeed = seed
		}
	case profile.SeedFixed:
		if len(cfg.BuildSeed) == 0 {
			seed, err := p.FixedBuildSeed()
			if err != nil {
				return err
			}
			cfg.BuildSeed = seed
		}
	}

	if p.SelectionSeed == 0 && len(cfg.BuildSeed) >= 8 {
		p.SelectionSeed = int64(binary.LittleEndian.Uint64(cfg.BuildSeed[:8]))
	}

	cfg.resolvedProfile = p
	cfg.Obfuscators = appendObfuscatorNames(cfg.Obfuscators, p.ObfuscatorNames()...)
	return nil
}

func expandCompileObfuscatorPatterns(patterns []string) ([]string, error) {
	normalized := obfuscation.NormalizeNames(patterns)
	if len(normalized) == 0 {
		return nil, nil
	}

	availableInfos := obfuscation.List()
	available := make([]string, 0, len(availableInfos))
	for _, info := range availableInfos {
		available = append(available, info.Name)
	}

	seen := make(map[string]struct{}, len(available))
	out := make([]string, 0, len(normalized))
	for _, patternText := range normalized {
		matched := false
		for _, candidate := range available {
			ok, err := path.Match(patternText, candidate)
			if err != nil {
				return nil, fmt.Errorf("invalid obfuscator pattern %q: %w", patternText, err)
			}
			if !ok {
				continue
			}
			matched = true
			if _, exists := seen[candidate]; exists {
				continue
			}
			seen[candidate] = struct{}{}
			out = append(out, candidate)
		}
		if !matched {
			return nil, fmt.Errorf("unknown obfuscator/pattern %q", patternText)
		}
	}
	return out, nil
}

func mergeProfileExtraObfuscators(p *profile.Profile, names []string) {
	if p == nil || len(names) == 0 {
		return
	}
	existing := make(map[string]struct{}, len(p.Obfuscators))
	for _, entry := range p.Obfuscators {
		existing[strings.ToLower(strings.TrimSpace(entry.Name))] = struct{}{}
	}
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		if _, ok := existing[key]; ok {
			continue
		}
		p.Obfuscators = append(p.Obfuscators, profile.ObfEntry{
			Name:     name,
			Category: profile.DefaultCategoryForObfuscator(name),
			Selector: profile.Selector{AllowEntry: true},
		})
		existing[key] = struct{}{}
	}
}
