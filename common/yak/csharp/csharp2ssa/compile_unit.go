package csharp2ssa

import (
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var (
	csharpNamespaceRe = regexp.MustCompile(`(?m)^\s*namespace\s+([A-Za-z_][A-Za-z0-9_.]*)`)
	csharpUsingRe     = regexp.MustCompile(`(?m)^\s*using\s+(?:static\s+)?(?:[A-Za-z_][A-Za-z0-9_]*\s*=\s*)?([A-Za-z_][A-Za-z0-9_.]*)\s*;`)
)

func (*SSABuilder) PartitionCompileUnits(fs filesys_interface.FileSystem, files []string) []*ssa.CompileUnit {
	sort.Strings(files)
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
			if ns := scanCSharpNamespace(fs, file); ns != "" {
				key = "csharp:" + ns
				unitPath = ssa.PackagePath(ns)
			}
		}
		add(key, unitPath, file)
	}
	for _, unit := range order {
		sort.Strings(unit.Files)
	}
	return order
}

func (*SSABuilder) CompileUnitDependencies(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []ssa.UnitRef {
	nsToKey := make(map[string]string)
	for _, unit := range units {
		if unit != nil && strings.HasPrefix(unit.Key, "csharp:") {
			nsToKey[strings.TrimPrefix(unit.Key, "csharp:")] = unit.Key
		}
	}
	var edges []ssa.UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".cs") {
				continue
			}
			src := ssa.ReadUnitSource(fs, file)
			for _, match := range csharpUsingRe.FindAllStringSubmatch(src, -1) {
				raw := match[1]
				if to := resolveCSharpUsing(nsToKey, raw); to != "" && to != unit.Key {
					edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "using", Raw: raw})
				}
			}
		}
	}
	return ssa.DedupeUnitRefs(edges)
}

func scanCSharpNamespace(fs filesys_interface.FileSystem, file string) string {
	src := ssa.ReadUnitSource(fs, file)
	match := csharpNamespaceRe.FindStringSubmatch(src)
	if len(match) < 2 {
		return ""
	}
	return match[1]
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
