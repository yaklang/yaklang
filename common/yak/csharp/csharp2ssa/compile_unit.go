package csharp2ssa

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func (s *SSABuilder) PartitionCompileUnits(fs filesys_interface.FileSystem, files []string) []*ssa.CompileUnit {
	sort.Strings(files)
	s.constructors.reset()
	s.declaredTypes.reset()
	fileScans := make(map[string]csharpFileScan)
	globalFiles := make(map[string]bool)
	globalDirectives := make([]string, 0)
	for _, file := range files {
		if !strings.EqualFold(fs.Ext(file), ".cs") {
			continue
		}
		cacheKey := ssa.NormalizeUnitPath(fs, file)
		scan := scanCSharpFile(ssa.ReadUnitSource(fs, file))
		fileScans[cacheKey] = scan
		if len(scan.globalDirectives) == 0 {
			continue
		}
		globalFiles[file] = true
		globalDirectives = append(globalDirectives, scan.globalDirectives...)
	}
	// PartitionCompileUnits is called once at the start of each project. Replace,
	// rather than append, so reusing an SSABuilder cannot carry scanner results
	// or globals into the next project.
	s.fileScans.replace(fileScans)
	s.prepareGlobalUsings(globalDirectives)

	units := make(map[string]*ssa.CompileUnit)
	order := make([]*ssa.CompileUnit, 0)
	add := func(key, unitPath, file string) {
		if key == "" {
			key = "unit:" + ssa.NormalizeUnitPath(fs, unitPath)
		}
		unit := units[key]
		if unit == nil {
			unit = &ssa.CompileUnit{Key: key, Path: unitPath, Language: ssaconfig.CSHARP}
			units[key] = unit
			order = append(order, unit)
		}
		unit.Files = append(unit.Files, file)
		if info, err := fs.Stat(file); err == nil && info != nil {
			unit.Bytes += info.Size()
		}
	}
	for _, file := range files {
		unitPath := ssa.UnitDir(fs, file)
		key := "dir:" + ssa.NormalizeUnitPath(fs, unitPath)
		if strings.EqualFold(fs.Ext(file), ".cs") {
			if ns := fileScans[ssa.NormalizeUnitPath(fs, file)].namespaceName; ns != "" {
				key = "csharp:" + ns
				unitPath = ssa.PackagePath(ns)
			}
		}
		add(key, unitPath, file)
	}
	for _, unit := range order {
		sort.SliceStable(unit.Files, func(i, j int) bool {
			leftGlobal, rightGlobal := globalFiles[unit.Files[i]], globalFiles[unit.Files[j]]
			if leftGlobal != rightGlobal {
				return leftGlobal
			}
			return unit.Files[i] < unit.Files[j]
		})
	}
	return order
}

func (s *SSABuilder) CompileUnitDependencies(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []ssa.UnitRef {
	namespaceIndex := ssa.NewUniqueStringIndex()
	for _, unit := range units {
		if unit == nil {
			continue
		}
		if strings.HasPrefix(unit.Key, "csharp:") {
			namespaceIndex.Add(strings.TrimPrefix(unit.Key, "csharp:"), unit.Key)
		}
		// One C# file may legally contain sibling namespace declarations or
		// nested block namespaces. The file still belongs to one compile unit,
		// but every fully-qualified namespace it declares must resolve back to
		// that unit when another file imports it.
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".cs") {
				continue
			}
			cacheKey := ssa.NormalizeUnitPath(fs, file)
			scan := s.fileScans.loadOrStore(cacheKey, func() csharpFileScan {
				return scanCSharpFile(ssa.ReadUnitSource(fs, file))
			})
			for _, namespaceName := range scan.namespaceNames {
				namespaceIndex.Add(namespaceName, unit.Key)
			}
		}
	}
	nsToKey := namespaceIndex.Values()
	var edges []ssa.UnitRef
	for _, unit := range units {
		if unit == nil {
			continue
		}
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".cs") {
				continue
			}
			cacheKey := ssa.NormalizeUnitPath(fs, file)
			scan := s.fileScans.loadOrStore(cacheKey, func() csharpFileScan {
				return scanCSharpFile(ssa.ReadUnitSource(fs, file))
			})
			for _, using := range scan.usings {
				if using.global {
					continue
				}
				raw := strings.ReplaceAll(stripGenericSuffix(using.target), "global::", "")
				if to := resolveCSharpUsing(nsToKey, raw); to != "" && to != unit.Key {
					edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "using", Raw: raw})
				}
			}
		}
	}

	// A global namespace/alias/static import is also a dependency of every
	// consumer unit, not merely of the file where the directive is written. The
	// registry is prepared before unit execution, so consumers do not depend on
	// the source unit that happened to declare the directive; retaining only the
	// real target edge also avoids artificial global-unit SCCs.
	globals := s.globalUsings.snapshot()
	globalTargets := append([]string(nil), globals.namespaces...)
	globalTargets = append(globalTargets, globals.statics...)
	for _, target := range globals.aliases {
		globalTargets = append(globalTargets, target)
	}
	for _, raw := range globalTargets {
		target := strings.ReplaceAll(stripGenericSuffix(raw), "global::", "")
		to := resolveCSharpUsing(nsToKey, target)
		if to == "" {
			continue
		}
		for _, unit := range units {
			if unit != nil && unit.Key != to {
				edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "global-using-target", Raw: raw})
			}
		}
	}
	return ssa.DedupeUnitRefs(edges)
}

func resolveCSharpUsing(nsToKey map[string]string, raw string) string {
	if key := nsToKey[raw]; key != "" {
		return key
	}
	best := ""
	for ns := range nsToKey {
		if strings.HasPrefix(raw, ns+".") && len(ns) > len(best) {
			best = ns
		}
	}
	if best == "" {
		return ""
	}
	return nsToKey[best]
}
