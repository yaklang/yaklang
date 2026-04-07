// Package pack defines the manifest format for curated LLVM pass packs.
// A pack bundles one or more plugins/tools with version constraints,
// textual pipeline descriptions, and known limitations.
package pack

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/llvminterop/plugin"
)

// Manifest describes a curated LLVM pass pack.
type Manifest struct {
	// Name is the pack identifier (e.g. "ollvm-classic").
	Name string `json:"name"`

	// Description is a short human-readable summary.
	Description string `json:"description,omitempty"`

	// LLVMVersionMin is the minimum supported LLVM major version (inclusive).
	LLVMVersionMin int `json:"llvm_version_min"`

	// LLVMVersionMax is the maximum supported LLVM major version (inclusive).
	// 0 means no upper bound.
	LLVMVersionMax int `json:"llvm_version_max,omitempty"`

	// Plugins lists the plugin descriptors in execution order.
	Plugins []plugin.Descriptor `json:"plugins"`

	// KnownLimitations documents things that don't work with this pack.
	KnownLimitations []string `json:"known_limitations,omitempty"`
}

// Validate checks that the manifest has the minimum required fields.
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("pack manifest: name must not be empty")
	}
	if m.LLVMVersionMin <= 0 {
		return fmt.Errorf("pack manifest: llvm_version_min must be positive")
	}
	if len(m.Plugins) == 0 {
		return fmt.Errorf("pack manifest: must have at least one plugin")
	}
	for i := range m.Plugins {
		if err := m.Plugins[i].Validate(); err != nil {
			return fmt.Errorf("pack manifest: plugin[%d]: %w", i, err)
		}
	}
	return nil
}

// Compatible checks if this pack is compatible with the given LLVM major version.
func (m *Manifest) Compatible(llvmMajor int) bool {
	if llvmMajor < m.LLVMVersionMin {
		return false
	}
	if m.LLVMVersionMax > 0 && llvmMajor > m.LLVMVersionMax {
		return false
	}
	return true
}

// LoadManifest reads a pack manifest from a JSON file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("pack: read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("pack: parse manifest %q: %w", path, err)
	}
	return &m, nil
}

// SaveManifest writes a pack manifest to a JSON file.
func SaveManifest(path string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("pack: marshal manifest: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// Registry holds a set of curated packs indexed by name.
type Registry struct {
	Packs map[string]*Manifest
}

// NewRegistry creates an empty pack registry.
func NewRegistry() *Registry {
	return &Registry{Packs: make(map[string]*Manifest)}
}

// Register adds a manifest to the registry.
func (r *Registry) Register(m *Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	r.Packs[m.Name] = m
	return nil
}

// Get retrieves a manifest by name.
func (r *Registry) Get(name string) (*Manifest, bool) {
	m, ok := r.Packs[name]
	return m, ok
}

// ListCompatible returns all packs compatible with the given LLVM version.
func (r *Registry) ListCompatible(llvmMajor int) []*Manifest {
	var result []*Manifest
	for _, m := range r.Packs {
		if m.Compatible(llvmMajor) {
			result = append(result, m)
		}
	}
	return result
}
