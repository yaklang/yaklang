package python2ssa

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var pyImportRe = regexp.MustCompile(`(?m)^\s*(?:from\s+([A-Za-z_][A-Za-z0-9_.]*)\s+import\b|import[ \t]+([^#\n]+))`)

var (
	pyModuleNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
	pyImportAsRe   = regexp.MustCompile(`\s+as\s+[A-Za-z_][A-Za-z0-9_]*$`)
)

// CompileUnitDependencies extracts Python import edges by resolving dotted
// module names against a path index built from unit paths and file stems.
func (*SSABuilder) CompileUnitDependencies(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []ssa.UnitRef {
	pathToKey := pythonImportPathIndex(fs, units)
	var edges []ssa.UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".py") {
				continue
			}
			src := ssa.ReadUnitSource(fs, file)
			for _, match := range pyImportRe.FindAllStringSubmatch(src, -1) {
				if mod := match[1]; mod != "" {
					// from X import ...: the dependency is on module X.
					if to := resolvePyModule(pathToKey, mod, unit.Key); to != "" {
						edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "import", Raw: mod})
					}
					continue
				}
				// import a, b, c / import a as b, c as d: one edge per module.
				for _, mod := range splitPyImportNames(match[2]) {
					if to := resolvePyModule(pathToKey, mod, unit.Key); to != "" {
						edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "import", Raw: mod})
					}
				}
			}
		}
	}
	// TODO: handle relative imports and runtime importlib precisely.
	return ssa.DedupeUnitRefs(edges)
}

func resolvePyModule(pathToKey map[string]string, mod, fromKey string) string {
	to := ssa.ResolvePathImport(pathToKey, strings.ReplaceAll(mod, ".", "/"))
	if to == "" || to == fromKey {
		return ""
	}
	return to
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
