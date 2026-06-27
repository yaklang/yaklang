package go2ssa

import (
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var (
	goImportRe = regexp.MustCompile(`(?m)^\s*(?:import\s+)?(?:[A-Za-z_][A-Za-z0-9_]*\s+)?\"([^\"]+)\"`)
	goModuleRe  = regexp.MustCompile(`(?m)^\s*module\s+(\S+)`)
)

type goModuleRoot struct {
	dir    string
	module string
}

// PartitionCompileUnits groups Go files by directory, assigning go.mod files to
// a shared "resource:go.mod" unit so they are excluded from the import-path index.
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
			unit = &ssa.CompileUnit{Key: key, Path: unitPath, Language: ssaconfig.GO}
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
		if strings.EqualFold(fs.Base(file), "go.mod") {
			key = "resource:go.mod"
			unitPath = ssa.UnitDir(fs, file)
		}
		add(key, unitPath, file)
	}
	for _, unit := range order {
		sort.Strings(unit.Files)
	}
	return order
}

// CompileUnitDependencies extracts Go import edges by resolving each import path
// against a module-rooted path index built from go.mod files.
func (*SSABuilder) CompileUnitDependencies(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []ssa.UnitRef {
	pathToKey := goImportPathIndex(fs, units)
	var edges []ssa.UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".go") {
				continue
			}
			src := ssa.ReadUnitSource(fs, file)
			for _, match := range goImportRe.FindAllStringSubmatch(src, -1) {
				raw := match[1]
				if to := ssa.ResolvePathImport(pathToKey, raw); to != "" && to != unit.Key {
					edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "import", Raw: raw})
				}
			}
		}
	}
	return ssa.DedupeUnitRefs(edges)
}

func goImportPathIndex(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) map[string]string {
	roots := goModuleRoots(fs, units)
	index := ssa.NewUniqueStringIndex()
	for _, unit := range units {
		if unit == nil {
			continue
		}
		unitPath := ssa.CleanUnitPath(fs, unit.Path)
		if unitPath == "." || strings.HasPrefix(unit.Key, "resource:") {
			continue
		}
		index.Add(unitPath, unit.Key)
		index.Add(ssa.UnitBase(fs, unitPath), unit.Key)
		for _, root := range roots {
			rel, ok := ssa.RelativeUnitPath(root.dir, unitPath)
			if !ok {
				continue
			}
			if rel != "." {
				index.Add(rel, unit.Key)
			}
			if root.module != "" {
				if rel == "." {
					index.Add(root.module, unit.Key)
				} else {
					index.Add(root.module+"/"+rel, unit.Key)
				}
			}
		}
	}
	return index.Values()
}

func goModuleRoots(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []goModuleRoot {
	seen := make(map[string]struct{})
	var roots []goModuleRoot
	for _, unit := range units {
		if unit == nil {
			continue
		}
		for _, file := range unit.Files {
			if !strings.EqualFold(ssa.UnitBase(fs, file), "go.mod") {
				continue
			}
			dir := ssa.CleanUnitPath(fs, ssa.UnitDir(fs, file))
			module := scanGoModulePath(ssa.ReadUnitSource(fs, file))
			key := dir + "\x00" + module
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			roots = append(roots, goModuleRoot{dir: dir, module: module})
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		if roots[i].dir != roots[j].dir {
			return roots[i].dir < roots[j].dir
		}
		return roots[i].module < roots[j].module
	})
	return roots
}

func scanGoModulePath(src string) string {
	match := goModuleRe.FindStringSubmatch(src)
	if len(match) < 2 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(match[1]), `"'`)
}
