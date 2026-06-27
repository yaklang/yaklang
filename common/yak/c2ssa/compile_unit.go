package c2ssa

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var cIncludeRe = regexp.MustCompile(`(?m)^\s*#\s*include\s+[<"]([^>"]+)[>"]`)

// CompileUnitDependencies extracts C/C++ #include edges by resolving include
// paths against a unit-path index.
func (*SSABuilder) CompileUnitDependencies(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []ssa.UnitRef {
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
				if to := ssa.ResolvePathImport(pathToKey, raw); to != "" && to != unit.Key {
					edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "include", Raw: raw})
				}
			}
		}
	}
	// TODO: macro-generated include paths need language-specific expansion.
	return ssa.DedupeUnitRefs(edges)
}
