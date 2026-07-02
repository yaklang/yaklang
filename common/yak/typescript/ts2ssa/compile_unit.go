package ts2ssa

import (
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var (
	jsImportRe        = regexp.MustCompile(`(?m)^\s*(?:import(?:\s+type)?(?:\s+[^'";]+?\s+from)?|export(?:\s+type)?\s+[^'";]+?\s+from)\s*['"]([^'"]+)['"]`)
	jsRequireRe       = regexp.MustCompile(`\brequire\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	jsDynamicImportRe = regexp.MustCompile(`\bimport\s*\(\s*['"]([^'"]+)['"]\s*\)`)
)

var jsModuleExtensions = []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs", ".json"}

// CompileUnitDependencies extracts JS/TS import edges for relative imports
// (those starting with "."), resolved against file/path indexes. Bare module
// specifiers are ignored (no reliable on-disk target).
func (*SSABuilder) CompileUnitDependencies(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []ssa.UnitRef {
	pathToKey := ssa.UnitPathIndex(units)
	fileToKey := ssa.UnitFileIndex(fs, units)
	var edges []ssa.UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			ext := strings.ToLower(fs.Ext(file))
			if ext != ".js" && ext != ".ts" && ext != ".tsx" && ext != ".jsx" {
				continue
			}
			src := ssa.ReadUnitSource(fs, file)
			for _, raw := range scanJSImportSpecs(src) {
				if !strings.HasPrefix(raw, ".") {
					continue
				}
				if to := ssa.ResolveRelativeImportUnit(fs, pathToKey, fileToKey, file, raw, jsModuleExtensions); to != "" && to != unit.Key {
					edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "import", Raw: raw})
				}
			}
		}
	}
	return ssa.DedupeUnitRefs(edges)
}

func scanJSImportSpecs(src string) []string {
	seen := make(map[string]struct{})
	var ret []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		ret = append(ret, raw)
	}
	for _, match := range jsImportRe.FindAllStringSubmatch(src, -1) {
		add(match[1])
	}
	for _, match := range jsRequireRe.FindAllStringSubmatch(src, -1) {
		add(match[1])
	}
	for _, match := range jsDynamicImportRe.FindAllStringSubmatch(src, -1) {
		add(match[1])
	}
	sort.Strings(ret)
	return ret
}
