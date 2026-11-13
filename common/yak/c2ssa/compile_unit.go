package c2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/c2ssa/preprocess"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// cIncludeExtCandidates are extensions tried when an include path has no
// extension (e.g. `#include "util"`).
var cIncludeExtCandidates = []string{".h", ".hpp", ".hxx", ".c", ".cc", ".cpp", ".cxx"}

// CompileUnitDependencies extracts C/C++ #include edges by resolving include
// paths against a file/path index. An include resolves to the unit that owns
// the referenced file; quoted includes are also tried relative to the
// including file's directory.
//
// The scan reads raw source instead of going through the C preprocessing hook:
// planning only needs the textual #include lines, while the hook expands every
// macro and inlines the whole include closure. On a macro-heavy tree that pass
// costs far more than the compile it is planning (nuttx: 10k+ TUs, tens of
// thousands of headers), and it is single-threaded. Files whose include operand
// is a macro (`#include FOO`) still fall back to the preprocessed text, because
// only expansion can resolve such a path.
func (*SSABuilder) CompileUnitDependencies(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []ssa.UnitRef {
	fileToKey := ssa.UnitFileIndex(fs, units)
	pathToKey := ssa.UnitPathIndex(units)
	var edges []ssa.UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".c") && !strings.EqualFold(fs.Ext(file), ".h") {
				continue
			}
			data, err := filesys.ReadFileRaw(fs, file)
			if err != nil {
				continue
			}
			src := string(data)
			if hasMacroInclude(src) {
				src = ssa.ReadUnitSource(fs, file)
			}
			for _, line := range preprocess.JoinLogicalLines(src) {
				raw, _, ok := preprocess.ParseIncludePath(line)
				if !ok {
					continue
				}
				if to := resolveCInclude(fs, fileToKey, pathToKey, file, raw); to != "" && to != unit.Key {
					edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "include", Raw: raw})
				}
			}
		}
	}
	return ssa.DedupeUnitRefs(edges)
}

// hasMacroInclude reports whether src contains an #include whose operand is not
// a literal "..." / <...> path, i.e. a path only macro expansion can resolve.
func hasMacroInclude(src string) bool {
	for _, line := range preprocess.JoinLogicalLines(src) {
		if preprocess.DirectiveName(line) != "include" {
			continue
		}
		if _, _, ok := preprocess.ParseIncludePath(line); ok {
			continue
		}
		rest := preprocess.DirectiveRest(line)
		if rest == "" || strings.HasPrefix(rest, "//") || strings.HasPrefix(rest, "/*") {
			continue
		}
		return true
	}
	return false
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
		norm := ssa.NormalizeUnitPath(fs, p)
		if key := fileToKey[norm]; key != "" {
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
