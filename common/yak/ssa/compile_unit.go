package ssa

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// CompileUnit is a single translation unit (package/directory) of a project,
// produced by a language Builder's PartitionCompileUnits. The compile engine
// walks units in dependency order so each unit can release its AST after build.
type CompileUnit struct {
	Key      string
	Path     string
	Files    []string
	Bytes    int64
	Language ssaconfig.Language
}

// UnitRef is a directed dependency edge between two compile units.
type UnitRef struct {
	From string
	To   string
	Kind string
	Raw  string
}

// UnitPartitioner lets each language Builder decide how to slice a file set
// into compile units and how to extract dependency edges between them. The
// engine (ssaapi) drives partition + dependency order; the language owns the
// extraction (regex/AST-specific). Default implementations in PreHandlerBase
// fall back to directory-based partitioning with no dependency edges.
type UnitPartitioner interface {
	// PartitionCompileUnits groups files into compile units; the returned
	// slice order is not significant (the engine topologically sorts it).
	PartitionCompileUnits(fs filesys_interface.FileSystem, files []string) []*CompileUnit
	// CompileUnitDependencies extracts dependency edges among the given units.
	// Return nil when the language cannot provide a reliable dependency graph
	// (e.g. PHP/JS dynamic imports); the engine then falls back to a
	// deterministic directory order.
	CompileUnitDependencies(fs filesys_interface.FileSystem, units []*CompileUnit) []UnitRef
}

// --- shared compile-unit path helpers (used by language builders) ---

// ReadUnitSource reads a file's content via fs, returning "" on error.
func ReadUnitSource(fs filesys_interface.FileSystem, file string) string {
	data, err := fs.ReadFile(file)
	if err != nil {
		return ""
	}
	return string(data)
}

// NormalizeUnitPath converts a path to forward-slash, trimmed, "."-for-empty form.
func NormalizeUnitPath(fs filesys_interface.FileSystem, p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.ReplaceAll(p, string(fs.GetSeparators()), "/")
	p = strings.Trim(p, "/")
	if p == "" {
		return "."
	}
	return p
}

// UnitDir returns the directory part of a unit path ("." for empty).
func UnitDir(fs filesys_interface.FileSystem, file string) string {
	file = NormalizeUnitPath(fs, file)
	if file == "." {
		return "."
	}
	idx := strings.LastIndex(file, "/")
	if idx < 0 {
		return "."
	}
	dir := strings.Trim(file[:idx], "/")
	if dir == "" {
		return "."
	}
	return dir
}

// UnitBase returns the last path element.
func UnitBase(fs filesys_interface.FileSystem, p string) string {
	p = NormalizeUnitPath(fs, p)
	if p == "." {
		return "."
	}
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		return p[idx+1:]
	}
	return p
}

// CleanUnitPath resolves "."/".." segments in a forward-slash path.
func CleanUnitPath(fs filesys_interface.FileSystem, p string) string {
	parts := strings.Split(NormalizeUnitPath(fs, p), "/")
	stack := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, part)
		}
	}
	if len(stack) == 0 {
		return "."
	}
	return strings.Join(stack, "/")
}

// RelativeUnitPath returns target relative to root, or (ok=false) if target is
// not under root.
func RelativeUnitPath(root, target string) (string, bool) {
	root = strings.Trim(strings.ReplaceAll(root, "\\", "/"), "/")
	target = strings.Trim(strings.ReplaceAll(target, "\\", "/"), "/")
	if root == "" {
		root = "."
	}
	if target == "" {
		target = "."
	}
	if root == "." {
		if target == "." {
			return ".", true
		}
		return target, true
	}
	if target == root {
		return ".", true
	}
	prefix := root + "/"
	if strings.HasPrefix(target, prefix) {
		return strings.TrimPrefix(target, prefix), true
	}
	return "", false
}

// PackagePath converts a dotted package name to a slash path.
func PackagePath(pkg string) string {
	pkg = strings.Trim(strings.ReplaceAll(pkg, ".", "/"), "/")
	if pkg == "" {
		return "."
	}
	return pkg
}

// ResolvePathImport resolves a raw import path against a path->unitKey index,
// preferring an exact match, otherwise the longest suffix match.
func ResolvePathImport(pathToKey map[string]string, raw string) string {
	raw = strings.Trim(strings.ReplaceAll(raw, "\\", "/"), "/")
	if raw == "" {
		return ""
	}
	if key := pathToKey[raw]; key != "" {
		return key
	}
	bestPath := ""
	for unitPath := range pathToKey {
		if strings.HasSuffix(raw, unitPath) && len(unitPath) > len(bestPath) {
			bestPath = unitPath
		}
	}
	if bestPath == "" {
		return ""
	}
	return pathToKey[bestPath]
}

// UnitPathIndex maps each unit's normalized Path to its Key (skipping ".").
func UnitPathIndex(units []*CompileUnit) map[string]string {
	ret := make(map[string]string)
	for _, unit := range units {
		if unit == nil {
			continue
		}
		p := strings.Trim(strings.ReplaceAll(unit.Path, "\\", "/"), "/")
		if p == "." || p == "" {
			continue
		}
		ret[p] = unit.Key
	}
	return ret
}

// UnitFileIndex maps each unit file's normalized path to its unit Key.
func UnitFileIndex(fs filesys_interface.FileSystem, units []*CompileUnit) map[string]string {
	ret := make(map[string]string)
	for _, unit := range units {
		if unit == nil {
			continue
		}
		for _, file := range unit.Files {
			ret[NormalizeUnitPath(fs, file)] = unit.Key
		}
	}
	return ret
}

// UniqueStringIndex maps raw strings to a single unit key, dropping any raw
// string that maps to more than one key (collision).
type UniqueStringIndex struct {
	values     map[string]string
	collisions map[string]struct{}
}

// NewUniqueStringIndex creates an empty UniqueStringIndex.
func NewUniqueStringIndex() *UniqueStringIndex {
	return &UniqueStringIndex{
		values:     make(map[string]string),
		collisions: make(map[string]struct{}),
	}
}

// Add records raw -> key; a raw that ever maps to two different keys is dropped.
func (idx *UniqueStringIndex) Add(raw string, key string) {
	raw = strings.Trim(strings.ReplaceAll(raw, "\\", "/"), "/")
	if raw == "" || raw == "." || key == "" {
		return
	}
	if _, collided := idx.collisions[raw]; collided {
		return
	}
	if old, ok := idx.values[raw]; ok && old != key {
		delete(idx.values, raw)
		idx.collisions[raw] = struct{}{}
		return
	}
	idx.values[raw] = key
}

// Values returns the non-colliding raw -> key mapping.
func (idx *UniqueStringIndex) Values() map[string]string {
	ret := make(map[string]string, len(idx.values))
	for raw, key := range idx.values {
		if _, collided := idx.collisions[raw]; collided {
			continue
		}
		ret[raw] = key
	}
	return ret
}

// ResolveRelativeImportUnit resolves a relative JS/TS import against file/path indexes.
func ResolveRelativeImportUnit(
	fs filesys_interface.FileSystem,
	pathToKey map[string]string,
	fileToKey map[string]string,
	importerFile string,
	raw string,
	extensions []string,
) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" || !strings.HasPrefix(raw, ".") {
		return ""
	}
	importerDir := UnitDir(fs, importerFile)
	base := CleanUnitPath(fs, fs.Join(importerDir, raw))
	candidates := []string{base}
	if fs.Ext(base) == "" {
		for _, ext := range extensions {
			candidates = append(candidates, base+ext)
		}
		for _, ext := range extensions {
			candidates = append(candidates, CleanUnitPath(fs, fs.Join(base, "index"+ext)))
		}
	}
	for _, candidate := range candidates {
		if key := fileToKey[NormalizeUnitPath(fs, candidate)]; key != "" {
			return key
		}
	}
	if key := pathToKey[base]; key != "" {
		return key
	}
	if key := pathToKey[UnitDir(fs, base)]; key != "" {
		return key
	}
	return ""
}

// DedupeUnitRefs removes duplicate edges and sorts them by (From, To, Raw).
func DedupeUnitRefs(edges []UnitRef) []UnitRef {
	seen := make(map[string]struct{})
	ret := make([]UnitRef, 0, len(edges))
	for _, edge := range edges {
		key := edge.From + "\x00" + edge.To + "\x00" + edge.Kind + "\x00" + edge.Raw
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ret = append(ret, edge)
	}
	sort.Slice(ret, func(i, j int) bool {
		if ret[i].From != ret[j].From {
			return ret[i].From < ret[j].From
		}
		if ret[i].To != ret[j].To {
			return ret[i].To < ret[j].To
		}
		return ret[i].Raw < ret[j].Raw
	})
	return ret
}

// --- default UnitPartitioner implementation on PreHandlerBase ---

// defaultPartitionCompileUnits groups files by directory. Languages with a
// stronger notion of a translation unit (e.g. Java package, Go module) override
// PartitionCompileUnits on their Builder.
func defaultPartitionCompileUnits(language ssaconfig.Language, fs filesys_interface.FileSystem, files []string) []*CompileUnit {
	sort.Strings(files)
	units := make(map[string]*CompileUnit)
	order := make([]*CompileUnit, 0)
	add := func(key, unitPath, file string) {
		if key == "" {
			key = "unit:" + NormalizeUnitPath(fs, unitPath)
		}
		unit := units[key]
		if unit == nil {
			unit = &CompileUnit{Key: key, Path: unitPath, Language: language}
			units[key] = unit
			order = append(order, unit)
		}
		unit.Files = append(unit.Files, file)
		if info, err := fs.Stat(file); err == nil && info != nil {
			unit.Bytes += info.Size()
		}
	}
	for _, file := range files {
		unitPath := UnitDir(fs, file)
		key := "dir:" + NormalizeUnitPath(fs, unitPath)
		add(key, unitPath, file)
	}
	for _, unit := range order {
		sort.Strings(unit.Files)
	}
	return order
}
