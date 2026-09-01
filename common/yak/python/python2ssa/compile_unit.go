package python2ssa

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var pyImportRe = regexp.MustCompile(`(?m)^\s*(?:from\s+([A-Za-z_][A-Za-z0-9_.]*)\s+import\b|import[ \t]+([^#\n]+))`)

// pyRelativeImportRe matches Python relative imports: "from ." / "from .." /
// "from .mod" — a dot prefix (one or more) followed by an optional dotted module.
// Example: "from ..pkg import x" -> dots = "..", module = "pkg".
var pyRelativeImportRe = regexp.MustCompile(`(?m)^\s*from\s+(\.+)([A-Za-z_][A-Za-z0-9_.]*)?\s+import\b`)

// pyImportlibRe matches dynamic imports via the importlib module:
// importlib.import_module("a.b", package="pkg") / __import__("a.b") / a bare
// import_module("a.b") (from importlib import import_module).
// Group 1 is the module literal, group 2 the optional package= literal.
var pyImportlibRe = regexp.MustCompile(`(?:__import__|importlib\s*\.\s*import_module|import_module)\s*\(\s*["']([^"'\n]+)["'](?:\s*,[^)]*?package\s*=\s*["']([^"'\n]*)["'])?`)

var (
	pyModuleNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
	pyImportAsRe   = regexp.MustCompile(`\s+as\s+[A-Za-z_][A-Za-z0-9_]*$`)
)

// CompileUnitDependencies extracts Python import edges by resolving dotted
// module names against a path index built from unit paths and file stems.
// It handles absolute imports ("import a.b", "from a.b import c"), relative
// imports ("from . import x", "from ..pkg import y"), and dynamic imports
// (importlib.import_module("a.b") / __import__("a.b") with a literal argument).
func (*SSABuilder) CompileUnitDependencies(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []ssa.UnitRef {
	pathToKey := pythonImportPathIndex(fs, units)
	var edges []ssa.UnitRef
	addEdge := func(from, to, raw string) {
		if to == "" || to == from {
			return
		}
		edges = append(edges, ssa.UnitRef{From: from, To: to, Kind: "import", Raw: raw})
	}
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".py") {
				continue
			}
			src := ssa.ReadUnitSource(fs, file)
			for _, match := range pyImportRe.FindAllStringSubmatch(src, -1) {
				if mod := match[1]; mod != "" {
					// from X import ...: the dependency is on module X.
					addEdge(unit.Key, ssa.ResolvePathImport(pathToKey, strings.ReplaceAll(mod, ".", "/")), mod)
					continue
				}
				// import a, b, c / import a as b, c as d: one edge per module.
				for _, mod := range splitPyImportNames(match[2]) {
					addEdge(unit.Key, ssa.ResolvePathImport(pathToKey, strings.ReplaceAll(mod, ".", "/")), mod)
				}
			}
			// Relative imports: resolve against the importing file's directory.
			for _, match := range pyRelativeImportRe.FindAllStringSubmatch(src, -1) {
				dots, module := match[1], match[2]
				if to := resolvePyRelativeImport(fs, pathToKey, file, dots, module); to != "" {
					addEdge(unit.Key, to, strings.TrimSpace(dots+module))
				}
			}
			// Dynamic imports with literal module names (importlib / __import__).
			for _, match := range pyImportlibRe.FindAllStringSubmatch(src, -1) {
				mod, pkg := match[1], strings.TrimSpace(match[2])
				if to := resolvePyDynamicImport(fs, pathToKey, file, mod, pkg); to != "" {
					addEdge(unit.Key, to, mod)
				}
			}
		}
	}
	return ssa.DedupeUnitRefs(edges)
}

// resolvePyRelativeImport resolves a relative import ("from <dots><module> import ...")
// to a unit key. dots is one or more "." characters: each dot ascends one
// directory level from the importing file's package; the first dot refers to
// the file's own package (its directory), two dots the parent, and so on.
// module is the optional dotted suffix after the dots.
func resolvePyRelativeImport(
	fs filesys_interface.FileSystem,
	pathToKey map[string]string,
	importerFile string,
	dots, module string,
) string {
	dotCount := strings.Count(dots, ".")
	if dotCount <= 0 {
		return ""
	}
	// The importing file's package directory is the base for the first dot.
	dir := ssa.UnitDir(fs, importerFile)
	for i := 1; i < dotCount; i++ {
		dir = ssa.CleanUnitPath(fs, dir+"/..")
	}
	base := dir
	if module != "" {
		base = ssa.CleanUnitPath(fs, fs.Join(dir, strings.ReplaceAll(module, ".", "/")))
	}
	// Candidate forms, most specific first: the module file itself, then its
	// package directory (from .pkg import x -> pkg/__init__.py).
	if key := ssa.ResolvePathImport(pathToKey, base); key != "" {
		return key
	}
	return ssa.ResolvePathImport(pathToKey, ssa.UnitDir(fs, base))
}

// resolvePyDynamicImport resolves a literal dynamic import. Absolute module
// names go through the plain path index. A leading dot (relative dynamic import,
// e.g. importlib.import_module(".b", package="pkg")) resolves against the
// package= argument when present, otherwise against the importer's directory.
func resolvePyDynamicImport(
	fs filesys_interface.FileSystem,
	pathToKey map[string]string,
	importerFile string,
	mod, pkg string,
) string {
	mod = strings.TrimSpace(mod)
	if mod == "" {
		return ""
	}
	if !strings.HasPrefix(mod, ".") {
		return ssa.ResolvePathImport(pathToKey, strings.ReplaceAll(mod, ".", "/"))
	}
	// Split the leading dots from the module suffix.
	dots := mod[:strings.IndexFunc(mod, func(r rune) bool { return r != '.' })]
	if dots == "" {
		dots = mod
	}
	suffix := strings.TrimPrefix(mod, dots)
	base := ""
	if pkg != "" {
		// package= anchors the resolution: one dot = pkg, two = pkg's parent, ...
		base = strings.ReplaceAll(strings.Trim(pkg, "/"), ".", "/")
		up := strings.Count(dots, ".") - 1
		for i := 0; i < up; i++ {
			base = ssa.CleanUnitPath(fs, base+"/..")
		}
	} else {
		dir := ssa.UnitDir(fs, importerFile)
		up := strings.Count(dots, ".") - 1
		for i := 0; i < up; i++ {
			dir = ssa.CleanUnitPath(fs, dir+"/..")
		}
		base = dir
	}
	if suffix != "" {
		base = ssa.CleanUnitPath(fs, fs.Join(base, strings.ReplaceAll(suffix, ".", "/")))
	}
	if key := ssa.ResolvePathImport(pathToKey, base); key != "" {
		return key
	}
	return ssa.ResolvePathImport(pathToKey, ssa.UnitDir(fs, base))
}

// splitPyImportNames parses the tail of a plain `import` statement
// ("a, b, c" or "a as b, c as d", possibly with a trailing comment) into the
// list of imported module names, dropping aliases and anything that is not a
// valid dotted module name (e.g. stray parens from `import (a, b)`).
func splitPyImportNames(s string) []string {
	var names []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		part = pyImportAsRe.ReplaceAllString(part, "")
		part = strings.TrimSpace(part)
		if part == "" || !pyModuleNameRe.MatchString(part) {
			continue
		}
		names = append(names, part)
	}
	return names
}

func pythonImportPathIndex(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) map[string]string {
	index := ssa.NewUniqueStringIndex()
	for _, unit := range units {
		if unit == nil {
			continue
		}
		unitPath := ssa.CleanUnitPath(fs, unit.Path)
		if unitPath != "" && unitPath != "." {
			index.Add(unitPath, unit.Key)
		}
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".py") {
				continue
			}
			normalized := ssa.NormalizeUnitPath(fs, file)
			stem := strings.TrimSuffix(normalized, fs.Ext(normalized))
			if stem == "" {
				continue
			}
			index.Add(stem, unit.Key)
			if base := ssa.UnitBase(fs, stem); base != "" && base != "." && base != "__init__" {
				index.Add(base, unit.Key)
			}
		}
	}
	return index.Values()
}
