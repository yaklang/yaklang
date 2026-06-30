package c2ssa

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var cIncludeRe = regexp.MustCompile(`(?m)^\s*#\s*include\s+[<"]([^>"]+)[>"]`)

// cIncludeExtCandidates are extensions tried when an include path has no
// extension (e.g. `#include "util"`).
var cIncludeExtCandidates = []string{".h", ".hpp", ".hxx", ".c", ".cc", ".cpp", ".cxx"}

// CompileUnitDependencies extracts C/C++ #include edges by resolving include
// paths against a file/path index. An include resolves to the unit that owns
// the referenced file; quoted includes are also tried relative to the
// including file's directory.
func (*SSABuilder) CompileUnitDependencies(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []ssa.UnitRef {
	fileToKey := ssa.UnitFileIndex(fs, units)
	pathToKey := ssa.UnitPathIndex(units)
	var edges []ssa.UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".c") && !strings.EqualFold(fs.Ext(file), ".h") {
				continue
			}
			src := ssa.ReadUnitSource(fs, file)
			for _, match := range cIncludeRe.FindAllStringSubmatch(src, -1) {
				raw := match[1]
				if to := resolveCInclude(fs, fileToKey, pathToKey, file, raw); to != "" && to != unit.Key {
					edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "include", Raw: raw})
				}
			}
		}
	}
	// TODO: macro-generated include paths need language-specific expansion.
	return ssa.DedupeUnitRefs(edges)
}

// resolveCInclude maps a #include path to its owning compile unit. It tries the
// path as a project-relative file, then relative to the including file's
// directory, with extension candidates for extensionless includes, and finally
// falls back to the unit-path index for includes that name a directory.
func resolveCInclude(fs filesys_interface.FileSystem, fileToKey, pathToKey map[string]string, importerFile, raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return ""
	}
	importerDir := ssa.UnitDir(fs, importerFile)
	candidates := []string{
		raw,
		ssa.CleanUnitPath(fs, fs.Join(importerDir, raw)),
	}
	lookup := func(p string) string {
		if key := fileToKey[ssa.NormalizeUnitPath(fs, p)]; key != "" {
			return key
		}
		if fs.Ext(p) == "" {
			for _, e := range cIncludeExtCandidates {
				if key := fileToKey[ssa.NormalizeUnitPath(fs, p+e)]; key != "" {
					return key
				}
			}
		}
		return ""
	}
	for _, c := range candidates {
		if key := lookup(c); key != "" {
			return key
		}
	}
	// Fallback: the include path (or its directory) is itself a unit directory.
	if key := pathToKey[ssa.CleanUnitPath(fs, raw)]; key != "" {
		return key
	}
	if key := pathToKey[ssa.UnitDir(fs, raw)]; key != "" {
		return key
	}
	return ""
}
