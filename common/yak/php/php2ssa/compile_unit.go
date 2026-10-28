package php2ssa

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var phpRequireRe = regexp.MustCompile(`(?i)\b(?:require|include)(?:_once)?\s*\(?\s*['"]([^'"]+)['"]`)

// SupportsConcurrentUnitPreHandler allows independent directory owners to
// register declarations in parallel. The PHP deferred body drain remains
// serial because include execution and the global container are shared.
func (*SSABuilder) SupportsConcurrentUnitPreHandler() bool { return true }

// CompileUnitDependencies extracts require/include file edges. The graph
// orders directories that include each other even with a serial body drain.
func (*SSABuilder) CompileUnitDependencies(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []ssa.UnitRef {
	fileToKey := ssa.UnitFileIndex(fs, units)
	pathToKey := ssa.UnitPathIndex(units)
	var edges []ssa.UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".php") {
				continue
			}
			src := ssa.ReadUnitSource(fs, file)
			for _, match := range phpRequireRe.FindAllStringSubmatch(src, -1) {
				raw := match[1]
				if to := ssa.ResolveRelativeImportUnit(fs, pathToKey, fileToKey, file, raw, []string{".php"}); to != "" && to != unit.Key {
					edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "require", Raw: raw})
				}
			}
		}
	}
	return ssa.DedupeUnitRefs(edges)
}
