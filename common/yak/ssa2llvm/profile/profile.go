// Package profile provides the single user-facing obfuscation configuration
// model for ssa2llvm. A profile can be a built-in preset or a JSON file.
package profile

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
)

// SeedPolicy governs how the build seed is provisioned.
type SeedPolicy string

const (
	SeedNone     SeedPolicy = "none"
	SeedPerBuild SeedPolicy = "per-build"
	SeedFixed    SeedPolicy = "fixed"
)

// ObfCategory classifies how an obfuscator transforms functions.
type ObfCategory string

const (
	CategoryBodyReplace ObfCategory = "body-replace"
	CategoryCallflow    ObfCategory = "callflow"
	CategoryLocal       ObfCategory = "llvm-local"
)

// Selector describes how to choose target functions for one obfuscator.
type Selector struct {
	Include    []string `json:"include,omitempty"`
	Exclude    []string `json:"exclude,omitempty"`
	Ratio      *float64 `json:"ratio,omitempty"`
	Count      *int     `json:"count,omitempty"`
	AllowEntry bool     `json:"allow_entry,omitempty"`
	MinBlocks  int      `json:"min_blocks,omitempty"`
	MinInsts   int      `json:"min_insts,omitempty"`
	Seed       int64    `json:"seed,omitempty"`
}

// ObfEntry configures one obfuscator inside a profile.
type ObfEntry struct {
	Name     string      `json:"name"`
	Category ObfCategory `json:"category,omitempty"`
	Selector Selector    `json:"selector,omitempty"`
}

// EffectiveCategory returns the configured category or the default category for
// known built-in obfuscators.
func (e ObfEntry) EffectiveCategory() ObfCategory {
	if e.Category != "" {
		return e.Category
	}
	return DefaultCategoryForObfuscator(e.Name)
}

// Profile is the single configuration document used by compiler, CLI, and
// profile-driven function selection. Built-in presets and JSON files share the
// same schema.
type Profile struct {
	Name          string     `json:"name,omitempty"`
	Description   string     `json:"description,omitempty"`
	SeedPolicy    SeedPolicy `json:"seed_policy,omitempty"`
	SelectionSeed int64      `json:"selection_seed,omitempty"`
	BuildSeedHex  string     `json:"build_seed_hex,omitempty"`
	LLVMPacks     []string   `json:"llvm_packs,omitempty"`
	Obfuscators   []ObfEntry `json:"obfuscators,omitempty"`
}

// Clone returns a deep copy so callers can safely derive a profile without
// mutating the registry instance.
func (p *Profile) Clone() *Profile {
	if p == nil {
		return nil
	}
	clone := *p
	if len(p.LLVMPacks) > 0 {
		clone.LLVMPacks = append([]string{}, p.LLVMPacks...)
	}
	if len(p.Obfuscators) > 0 {
		clone.Obfuscators = make([]ObfEntry, len(p.Obfuscators))
		for i, entry := range p.Obfuscators {
			entryClone := entry
			entryClone.Selector.Include = append([]string{}, entry.Selector.Include...)
			entryClone.Selector.Exclude = append([]string{}, entry.Selector.Exclude...)
			if entry.Selector.Ratio != nil {
				r := *entry.Selector.Ratio
				entryClone.Selector.Ratio = &r
			}
			if entry.Selector.Count != nil {
				c := *entry.Selector.Count
				entryClone.Selector.Count = &c
			}
			clone.Obfuscators[i] = entryClone
		}
	}
	return &clone
}

// NormalizedSeedPolicy returns the seed policy, treating an empty value as
// SeedNone.
func (p *Profile) NormalizedSeedPolicy() SeedPolicy {
	if p == nil || strings.TrimSpace(string(p.SeedPolicy)) == "" {
		return SeedNone
	}
	return p.SeedPolicy
}

// ObfuscatorNames returns a deduplicated, normalized copy of obfuscator names.
func (p *Profile) ObfuscatorNames() []string {
	if p == nil {
		return nil
	}
	names := make([]string, 0, len(p.Obfuscators))
	for _, entry := range p.Obfuscators {
		names = append(names, entry.Name)
	}
	return core.NormalizeNames(names)
}

// FixedBuildSeed decodes the configured fixed build seed.
func (p *Profile) FixedBuildSeed() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("nil profile")
	}
	if p.NormalizedSeedPolicy() != SeedFixed {
		return nil, nil
	}
	raw := strings.TrimSpace(p.BuildSeedHex)
	if raw == "" {
		return nil, fmt.Errorf("profile %q requires build_seed_hex when seed_policy=fixed", p.Name)
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("profile %q has invalid build_seed_hex: %w", p.Name, err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("profile %q build_seed_hex must decode to 32 bytes", p.Name)
	}
	return decoded, nil
}

// Validate checks the profile for internal consistency and unknown obfuscators.
func (p *Profile) Validate() error {
	if p == nil {
		return fmt.Errorf("nil profile")
	}

	switch p.NormalizedSeedPolicy() {
	case SeedNone, SeedPerBuild, SeedFixed:
	default:
		return fmt.Errorf("profile %q has unsupported seed_policy %q", p.Name, p.SeedPolicy)
	}

	if p.NormalizedSeedPolicy() == SeedFixed {
		if _, err := p.FixedBuildSeed(); err != nil {
			return err
		}
	} else if strings.TrimSpace(p.BuildSeedHex) != "" {
		return fmt.Errorf("profile %q build_seed_hex requires seed_policy=fixed", p.Name)
	}

	registered := core.List()
	registry := make(map[string]struct{}, len(registered))
	for _, info := range registered {
		registry[info.Name] = struct{}{}
	}

	for i, entry := range p.Obfuscators {
		name := normalize(entry.Name)
		if name == "" {
			return fmt.Errorf("profile %q obfuscator entry %d has empty name", p.Name, i)
		}
		if _, ok := registry[name]; !ok {
			return fmt.Errorf("profile %q references unregistered obfuscator %q", p.Name, entry.Name)
		}

		switch entry.EffectiveCategory() {
		case CategoryBodyReplace, CategoryCallflow, CategoryLocal:
		default:
			return fmt.Errorf("profile %q obfuscator %q has invalid category %q", p.Name, entry.Name, entry.Category)
		}

		sel := entry.Selector
		if sel.Ratio != nil && sel.Count != nil {
			return fmt.Errorf("profile %q obfuscator %q: ratio and count are mutually exclusive", p.Name, entry.Name)
		}
		if sel.Ratio != nil && (*sel.Ratio < 0 || *sel.Ratio > 1) {
			return fmt.Errorf("profile %q obfuscator %q: ratio must be between 0.0 and 1.0", p.Name, entry.Name)
		}
		if sel.Count != nil && *sel.Count < 0 {
			return fmt.Errorf("profile %q obfuscator %q: count must be non-negative", p.Name, entry.Name)
		}
		if sel.MinBlocks < 0 {
			return fmt.Errorf("profile %q obfuscator %q: min_blocks must be non-negative", p.Name, entry.Name)
		}
		if sel.MinInsts < 0 {
			return fmt.Errorf("profile %q obfuscator %q: min_insts must be non-negative", p.Name, entry.Name)
		}
	}

	return nil
}

// Parse parses profile JSON bytes.
func Parse(data []byte) (*Profile, error) {
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// LoadFile reads and parses a profile JSON file.
func LoadFile(path string) (*Profile, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("empty profile file path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile file %q: %w", path, err)
	}
	return Parse(data)
}

// LoadRef resolves a profile reference. Built-in names win over filesystem
// paths with the same text.
func LoadRef(ref string) (*Profile, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("empty profile ref")
	}
	if builtin, ok := Get(ref); ok {
		return builtin.Clone(), nil
	}
	p, err := LoadFile(ref)
	if err != nil {
		return nil, fmt.Errorf("unknown compile profile %q: %w", ref, err)
	}
	return p, nil
}

func ratioPtr(v float64) *float64 { return &v }

// DefaultCategoryForObfuscator returns the default profile category for a
// registered built-in obfuscator.
func DefaultCategoryForObfuscator(name string) ObfCategory {
	switch normalize(name) {
	case "virtualize":
		return CategoryBodyReplace
	case "callret":
		return CategoryCallflow
	default:
		return CategoryLocal
	}
}

var (
	ResilienceLite = &Profile{
		Name:        "resilience-lite",
		Description: "Lightweight native-only obfuscation (addsub + xor + callret)",
		SeedPolicy:  SeedNone,
		Obfuscators: []ObfEntry{
			{Name: "addsub", Category: CategoryLocal, Selector: Selector{AllowEntry: true}},
			{Name: "xor", Category: CategoryLocal, Selector: Selector{AllowEntry: true}},
			{Name: "callret", Category: CategoryCallflow, Selector: Selector{AllowEntry: true}},
		},
	}

	ResilienceHybrid = &Profile{
		Name:        "resilience-hybrid",
		Description: "Native passes + MBA + opaque + partial virtualize + per-build diversification",
		SeedPolicy:  SeedPerBuild,
		Obfuscators: []ObfEntry{
			{Name: "addsub", Category: CategoryLocal, Selector: Selector{AllowEntry: true}},
			{Name: "xor", Category: CategoryLocal, Selector: Selector{AllowEntry: true}},
			{Name: "callret", Category: CategoryCallflow, Selector: Selector{AllowEntry: true}},
			{Name: "mba", Category: CategoryLocal, Selector: Selector{AllowEntry: true}},
			{Name: "opaque", Category: CategoryLocal, Selector: Selector{AllowEntry: true}},
			{
				Name:     "virtualize",
				Category: CategoryBodyReplace,
				Selector: Selector{
					Ratio:    ratioPtr(0.3),
					MinInsts: 4,
				},
			},
		},
	}

	ResilienceMax = &Profile{
		Name:        "resilience-max",
		Description: "All current passes + full virtualize + per-build diversification",
		SeedPolicy:  SeedPerBuild,
		Obfuscators: []ObfEntry{
			{Name: "addsub", Category: CategoryLocal, Selector: Selector{AllowEntry: true}},
			{Name: "xor", Category: CategoryLocal, Selector: Selector{AllowEntry: true}},
			{Name: "callret", Category: CategoryCallflow, Selector: Selector{AllowEntry: true}},
			{Name: "mba", Category: CategoryLocal, Selector: Selector{AllowEntry: true}},
			{Name: "opaque", Category: CategoryLocal, Selector: Selector{AllowEntry: true}},
			{
				Name:     "virtualize",
				Category: CategoryBodyReplace,
				Selector: Selector{
					Ratio:    ratioPtr(1.0),
					MinInsts: 2,
				},
			},
		},
	}
)

var registry = map[string]*Profile{}

func init() {
	Register(ResilienceLite)
	Register(ResilienceHybrid)
	Register(ResilienceMax)
}

// Register adds a built-in profile to the registry.
func Register(p *Profile) {
	if p == nil || strings.TrimSpace(p.Name) == "" {
		return
	}
	registry[normalize(p.Name)] = p.Clone()
}

// Get looks up a built-in profile by name.
func Get(name string) (*Profile, bool) {
	p, ok := registry[normalize(name)]
	return p, ok
}

// List returns all built-in profiles sorted by name.
func List() []*Profile {
	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*Profile, 0, len(keys))
	for _, key := range keys {
		out = append(out, registry[key].Clone())
	}
	return out
}

// Names returns the sorted built-in profile names.
func Names() []string {
	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
