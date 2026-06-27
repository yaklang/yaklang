package python2ssa

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var pyImportRe = regexp.MustCompile(`(?m)^\s*(?:from\s+([A-Za-z_][A-Za-z0-9_.]*)\s+import|import\s+([A-Za-z_][A-Za-z0-9_.]*))`)

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
				raw := match[1]
				if raw == "" {
					raw = match[2]
				}
				if to := ssa.ResolvePathImport(pathToKey, strings.ReplaceAll(raw, ".", "/")); to != "" && to != unit.Key {
					edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "import", Raw: raw})
				}
			}
		}
	}
	// TODO: handle relative imports and runtime importlib precisely.
	return ssa.DedupeUnitRefs(edges)
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
