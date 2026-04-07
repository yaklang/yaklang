// Package profile provides resilience profiles that combine obfuscation passes
// and build-seed strategies into named presets.
//
// Users pick a single profile (e.g. "resilience-lite") instead of manually
// assembling low-level obfuscation flags.
package profile

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/policy"
)

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

// Level is a human-friendly resilience tier.
type Level string

const (
	LevelLite   Level = "resilience-lite"
	LevelHybrid Level = "resilience-hybrid"
	LevelMax    Level = "resilience-max"
)

// SeedPolicy governs how the build seed is provisioned.
type SeedPolicy string

const (
	SeedNone     SeedPolicy = "none"      // no diversification
	SeedPerBuild SeedPolicy = "per-build" // unique seed per build invocation
	SeedFixed    SeedPolicy = "fixed"     // reproducible seed from user-supplied key
)

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

// Profile is a self-contained description of an obfuscation strategy.
type Profile struct {
	Name        string     // e.g. "resilience-lite"
	Level       Level      // tier for UI/logging
	Obfuscators []string   // pass names registered in obfuscation/core
	LLVMPacks   []string   // optional LLVM interop pack names
	SeedPolicy  SeedPolicy // how to generate build seeds
	Description string     // human-readable summary

	// DefaultEntries provides per-obfuscator selection rules that take
	// effect when no explicit --obf-policy file is supplied.  These
	// entries are used to construct an inline Policy → Resolver flow
	// so profiles can drive ratio/count/seed selection without requiring
	// the user to write a policy file.
	DefaultEntries []policy.ObfEntry
}

// ObfuscatorNames returns a deduplicated, normalised copy of Obfuscators.
func (p *Profile) ObfuscatorNames() []string {
	return core.NormalizeNames(p.Obfuscators)
}

// DefaultPolicy returns a *policy.Policy constructed from DefaultEntries,
// or nil if no entries are configured.
func (p *Profile) DefaultPolicy() *policy.Policy {
	if p == nil || len(p.DefaultEntries) == 0 {
		return nil
	}
	return &policy.Policy{
		Obfuscators: p.DefaultEntries,
	}
}

// Validate checks that every requested obfuscator is registered.
func (p *Profile) Validate() error {
	if p == nil {
		return fmt.Errorf("nil profile")
	}
	names := p.ObfuscatorNames()
	registered := core.List()
	regSet := make(map[string]struct{}, len(registered))
	for _, info := range registered {
		regSet[info.Name] = struct{}{}
	}
	for _, n := range names {
		matched := false
		for candidate := range regSet {
			ok, err := path.Match(n, candidate)
			if err != nil {
				return fmt.Errorf("profile %q has invalid obfuscator pattern %q: %w", p.Name, n, err)
			}
			if ok {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("profile %q references unregistered obfuscator %q", p.Name, n)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Built-in profiles
// ---------------------------------------------------------------------------

func ratioPtr(r float64) *float64 { return &r }

var (
	// ResilienceLite applies only the lightweight native obfuscation passes.
	// No build diversification. Good for development builds where compile
	// speed matters more than tamper resistance.
	ResilienceLite = &Profile{
		Name:        "resilience-lite",
		Level:       LevelLite,
		Obfuscators: []string{"addsub", "xor", "callret"},
		SeedPolicy:  SeedNone,
		Description: "Lightweight native-only obfuscation (addsub + xor + callret)",
	}

	// ResilienceHybrid adds MBA, opaque predicates, and partial VM
	// virtualization on top of the lite passes.  Per-build seed
	// diversification ensures no two builds share the same obfuscation layout.
	ResilienceHybrid = &Profile{
		Name:        "resilience-hybrid",
		Level:       LevelHybrid,
		Obfuscators: []string{"addsub", "xor", "callret", "mba", "opaque", "virtualize"},
		SeedPolicy:  SeedPerBuild,
		Description: "Native passes + MBA + opaque + partial virtualize + per-build diversification",
		DefaultEntries: []policy.ObfEntry{
			{
				Name:     "virtualize",
				Category: policy.CategoryBodyReplace,
				Selector: policy.Selector{
					Ratio:    ratioPtr(0.3),
					MinInsts: 4,
				},
			},
		},
	}

	// ResilienceMax turns on all available passes with full virtualization.
	// Build seed diversification is per-build.
	ResilienceMax = &Profile{
		Name:        "resilience-max",
		Level:       LevelMax,
		Obfuscators: []string{"*"}, // glob: all registered passes
		SeedPolicy:  SeedPerBuild,
		Description: "All passes + full virtualize + per-build diversification",
		DefaultEntries: []policy.ObfEntry{
			{
				Name:     "virtualize",
				Category: policy.CategoryBodyReplace,
				Selector: policy.Selector{
					Ratio:    ratioPtr(1.0),
					MinInsts: 2,
				},
			},
		},
	}
)

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

var registry = map[string]*Profile{}

func init() {
	Register(ResilienceLite)
	Register(ResilienceHybrid)
	Register(ResilienceMax)
}

// Register adds a profile to the global registry.
func Register(p *Profile) {
	if p == nil || p.Name == "" {
		return
	}
	registry[normalize(p.Name)] = p
}

// Get looks up a profile by name (case-insensitive, trimmed).
func Get(name string) (*Profile, bool) {
	p, ok := registry[normalize(name)]
	return p, ok
}

// List returns all registered profiles sorted by name.
func List() []*Profile {
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*Profile, 0, len(keys))
	for _, k := range keys {
		out = append(out, registry[k])
	}
	return out
}

// Names returns sorted profile names.
func Names() []string {
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
