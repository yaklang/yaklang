// Package tiers defines the pre-built runtime archive tiers.
//
// Link-time pruning (see cmd/elfsplit) removes module *code* from one archive
// that already contains everything, but the archive's Go metadata — pclntab,
// type descriptors, itabs — describes every module whether or not its code
// survives. Only the Go linker can drop those, and it only runs when libyak.a
// is built. A tier is therefore just an archive built with a smaller
// SSA2LLVM_EMBED_MODULES set: the Go linker's dead-code elimination deletes the
// unused modules' code and metadata together, before ssa2llvm ever sees it.
//
// Tiers and link-time pruning are orthogonal and compose: the tier decides how
// much metadata exists at all, pruning decides how much of the remaining code
// reaches the binary. Measured on one hello-world script:
//
//	staticanalyze tier + pruning    84.62 MiB
//	net  tier + pruning    28.73 MiB
//	core tier + pruning     7.87 MiB
//
// Which modules can live in a small tier is not a free choice. Modules whose
// yaklib entry points are backed by the lightweight AOT export tables (os,
// codec, yakit) keep the monolithic common/yak/yaklib package out of the
// archive. A module without such a shim imports the monolith, which transitively
// imports the SSA frontends, the database drivers and the protobuf runtime — so
// adding it to the core tier inflates that tier back to the full one. Growing
// the ladder means writing shims, not editing this list.
package tiers

import (
	"fmt"
	"slices"
	"strings"
)

// Tier is one pre-built archive: a name and the yaklib modules registered into
// it. Modules are sorted so the set reads the same everywhere it is printed.
type Tier struct {
	Name    string
	Modules []string
}

// All lists the tiers from smallest to largest. Each tier's module set must be
// a superset of the previous one's, so Select can return the first covering
// tier and callers can fall back to any later tier when an archive is missing.
var All = []Tier{
	{
		Name:    "core",
		Modules: []string{"codec", "os", "yakit"},
	},
	{
		Name:    "net",
		Modules: []string{"cli", "codec", "http", "os", "poc", "yakit"},
	},
	{
		Name:    "staticanalyze",
		Modules: []string{"cli", "codec", "file", "filesys", "http", "json", "os", "poc", "ssa", "str", "sync", "yakit"},
	},
}

// Largest is the tier that covers every module the ladder knows about.
func Largest() Tier { return All[len(All)-1] }

// Lookup returns the tier with the given name.
func Lookup(name string) (Tier, bool) {
	for _, t := range All {
		if t.Name == name {
			return t, true
		}
	}
	return Tier{}, false
}

// Covers reports whether the tier's archive can register every named module.
func (t Tier) Covers(modules []string) bool {
	for _, m := range modules {
		if !slices.Contains(t.Modules, m) {
			return false
		}
	}
	return true
}

// ModuleList renders the tier's modules the way SSA2LLVM_EMBED_MODULES wants
// them.
func (t Tier) ModuleList() string { return strings.Join(t.Modules, ",") }

// Select returns the smallest tier that can register every module the script
// uses. It fails for a module no tier carries — that module needs an AOT shim
// and a place in the ladder before a tier can serve it, and picking the largest
// tier anyway would only move the failure to the linker.
func Select(modules []string) (Tier, error) {
	for _, t := range All {
		if t.Covers(modules) {
			return t, nil
		}
	}
	unknown := make([]string, 0, len(modules))
	for _, m := range modules {
		if !Largest().Covers([]string{m}) {
			unknown = append(unknown, m)
		}
	}
	slices.Sort(unknown)
	return Tier{}, fmt.Errorf("no runtime tier provides yaklib module(s) %s; tiers are %s",
		strings.Join(unknown, ", "), Names())
}

// Names lists the tier names smallest-first, for error messages.
func Names() string {
	names := make([]string, 0, len(All))
	for _, t := range All {
		names = append(names, t.Name)
	}
	return strings.Join(names, " < ")
}

// AtLeast returns the tiers from the named one upwards, largest last. A build
// that cannot find the selected tier's archive can use any of these instead:
// they are supersets, so the link still succeeds and only the binary is bigger.
func AtLeast(name string) []Tier {
	for i, t := range All {
		if t.Name == name {
			return slices.Clone(All[i:])
		}
	}
	return nil
}
