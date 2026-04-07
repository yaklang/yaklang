package core

import (
	"path/filepath"
	"sort"
)

// RuntimeDep describes runtime symbols an obfuscator needs at link time.
type RuntimeDep struct {
	ObfName        string
	ArchiveName    string
	Symbols        []string
	FallbackToMain bool
}

func (d *RuntimeDep) ArchiveFileName() string {
	return "libyakobf_" + d.ArchiveName + ".a"
}

// RuntimeDepProvider is implemented by obfuscators that need runtime support.
type RuntimeDepProvider interface {
	RuntimeDeps() []RuntimeDep
}

func CollectRuntimeDeps(obfNames []string) []*RuntimeDep {
	seen := make(map[string]struct{})
	var deps []*RuntimeDep
	for _, name := range NormalizeNames(obfNames) {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		obf := Default[name]
		if obf == nil {
			continue
		}
		provider, ok := obf.(RuntimeDepProvider)
		if !ok {
			continue
		}
		for _, dep := range provider.RuntimeDeps() {
			depCopy := dep
			if depCopy.ObfName == "" {
				depCopy.ObfName = name
			}
			deps = append(deps, &depCopy)
		}
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].ObfName == deps[j].ObfName {
			return deps[i].ArchiveName < deps[j].ArchiveName
		}
		return deps[i].ObfName < deps[j].ObfName
	})
	return deps
}

func ExtraRuntimeArchivePaths(deps []*RuntimeDep, archiveDir string) []string {
	seen := make(map[string]struct{})
	var paths []string
	for _, dep := range deps {
		if dep == nil || dep.FallbackToMain {
			continue
		}
		name := dep.ArchiveFileName()
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		paths = append(paths, filepath.Join(archiveDir, name))
	}
	return paths
}

func AllRuntimeSymbols() []string {
	set := make(map[string]struct{})
	for _, obf := range Default {
		provider, ok := obf.(RuntimeDepProvider)
		if !ok {
			continue
		}
		for _, dep := range provider.RuntimeDeps() {
			for _, sym := range dep.Symbols {
				set[sym] = struct{}{}
			}
		}
	}
	syms := make([]string, 0, len(set))
	for sym := range set {
		syms = append(syms, sym)
	}
	sort.Strings(syms)
	return syms
}
