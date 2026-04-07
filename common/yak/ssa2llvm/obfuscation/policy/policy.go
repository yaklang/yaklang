// Package policy defines the structured obfuscation policy that drives the
// unified obf system. A policy file (JSON) lets
// users specify per-obfuscator function selectors, ratios, seeds, and
// conflict-resolution rules in a single declarative document.
package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ObfCategory classifies an obfuscator by how it transforms functions.
type ObfCategory string

const (
	// CategoryBodyReplace replaces the original function body entirely
	// (e.g. virtualize). At most one body-replace obf may own a function.
	CategoryBodyReplace ObfCategory = "body-replace"

	// CategoryCallflow rewrites call/return sequences across functions
	// (e.g. callret). May co-exist with body-replace wrappers.
	CategoryCallflow ObfCategory = "callflow"

	// CategoryLocal applies local IR-level transformations
	// (e.g. mba, opaque, addsub, xor). May stack freely.
	CategoryLocal ObfCategory = "llvm-local"
)

// Selector describes how to choose target functions for one obfuscator.
type Selector struct {
	// Include lists function name patterns to include (glob).
	// Empty means "all eligible functions" (subject to Ratio/Count).
	Include []string `json:"include,omitempty"`

	// Exclude lists function name patterns to exclude (glob).
	Exclude []string `json:"exclude,omitempty"`

	// Ratio is the fraction of eligible functions to select (0.0–1.0).
	// Mutually exclusive with Count.
	Ratio *float64 `json:"ratio,omitempty"`

	// Count is the exact number of functions to select.
	// Mutually exclusive with Ratio.
	Count *int `json:"count,omitempty"`

	// AllowEntry permits selecting the program entry function.
	AllowEntry bool `json:"allow_entry,omitempty"`

	// MinBlocks is the minimum number of basic blocks a function must have.
	MinBlocks int `json:"min_blocks,omitempty"`

	// MinInsts is the minimum number of instructions a function must have.
	MinInsts int `json:"min_insts,omitempty"`

	// Seed is a per-obfuscator seed override. When non-zero, it takes
	// precedence over the policy-level Seed for this obfuscator's selection.
	Seed int64 `json:"seed,omitempty"`
}

// ObfEntry configures a single obfuscator within the policy.
type ObfEntry struct {
	// Name is the registered obfuscator name (e.g. "virtualize", "callret").
	Name string `json:"name"`

	// Category classifies the obfuscator. Used for conflict detection.
	// If empty, defaults are inferred from the obfuscator registry.
	Category ObfCategory `json:"category,omitempty"`

	// Selector controls which functions this obfuscator targets.
	Selector Selector `json:"selector,omitempty"`
}

// Policy is the top-level obfuscation policy document.
type Policy struct {
	// Seed is the global random seed for deterministic selection.
	// When zero, a random seed is generated per build.
	Seed int64 `json:"seed,omitempty"`

	// Obfuscators lists per-obfuscator entries.
	Obfuscators []ObfEntry `json:"obfuscators,omitempty"`
}

// LoadFile reads and parses a policy JSON file.
func LoadFile(path string) (*Policy, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("empty policy file path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy file %q: %w", path, err)
	}
	return Parse(data)
}

// Parse parses policy JSON bytes.
func Parse(data []byte) (*Policy, error) {
	var p Policy
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse obf policy: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate checks the policy for internal consistency.
func (p *Policy) Validate() error {
	if p == nil {
		return fmt.Errorf("nil policy")
	}
	bodyReplaceNames := make(map[string]bool)
	for i, entry := range p.Obfuscators {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			return fmt.Errorf("obf policy entry %d has empty name", i)
		}
		if entry.Category == CategoryBodyReplace {
			bodyReplaceNames[name] = true
		}
		sel := entry.Selector
		if sel.Ratio != nil && sel.Count != nil {
			return fmt.Errorf("obf policy entry %q: ratio and count are mutually exclusive", name)
		}
		if sel.Ratio != nil && (*sel.Ratio < 0 || *sel.Ratio > 1) {
			return fmt.Errorf("obf policy entry %q: ratio must be between 0.0 and 1.0", name)
		}
		if sel.Count != nil && *sel.Count < 0 {
			return fmt.Errorf("obf policy entry %q: count must be non-negative", name)
		}
	}
	return nil
}

// ObfuscatorNames returns the list of obfuscator names from the policy.
func (p *Policy) ObfuscatorNames() []string {
	if p == nil {
		return nil
	}
	names := make([]string, 0, len(p.Obfuscators))
	for _, entry := range p.Obfuscators {
		name := strings.TrimSpace(entry.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}
