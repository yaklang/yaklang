package ssaapi

import (
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type CompileUnit struct {
	Key      string
	Path     string
	Files    []string
	Bytes    int64
	Language ssaconfig.Language
}

type UnitRef struct {
	From string
	To   string
	Kind string
	Raw  string
}

type UnitPlan struct {
	Units map[string]*CompileUnit
	Edges []UnitRef
	Order [][]*CompileUnit
}

var (
	javaPackageRe = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z_][A-Za-z0-9_.]*)\s*;`)
	javaImportRe  = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([A-Za-z_][A-Za-z0-9_.]*)(?:\.\*)?\s*;`)
	javaStringRe  = regexp.MustCompile(`"((?:\\.|[^"\\])*)"`)
	goImportRe    = regexp.MustCompile(`(?m)^\s*(?:import\s+)?(?:[A-Za-z_][A-Za-z0-9_]*\s+)?\"([^\"]+)\"`)
	goModuleRe    = regexp.MustCompile(`(?m)^\s*module\s+(\S+)`)
	pyImportRe    = regexp.MustCompile(`(?m)^\s*(?:from\s+([A-Za-z_][A-Za-z0-9_.]*)\s+import|import\s+([A-Za-z_][A-Za-z0-9_.]*))`)
	cIncludeRe    = regexp.MustCompile(`(?m)^\s*#\s*include\s+[<"]([^>"]+)[>"]`)
	jsImportRe    = regexp.MustCompile(`(?m)^\s*(?:import(?:\s+type)?(?:\s+[^'";]+?\s+from)?|export(?:\s+type)?\s+[^'";]+?\s+from)\s*['"]([^'"]+)['"]`)
	jsRequireRe   = regexp.MustCompile(`\brequire\s*\(\s*['"]([^'"]+)['"]\s*\)`)
)

var javaTemplateExtensions = []string{".jsp", ".jspx", ".ftl", ".ftlh", ".ftlx", ".vm", ".vtl", ".html", ".htm"}

func buildCompileUnitPlan(language ssaconfig.Language, fs filesys_interface.FileSystem, files []string) *UnitPlan {
	files = append([]string(nil), files...)
	sort.Strings(files)
	units := partitionCompileUnits(language, fs, files)
	edges := unitDependencies(language, fs, units)
	return topoCompileUnits(units, edges)
}

func partitionCompileUnits(language ssaconfig.Language, fs filesys_interface.FileSystem, files []string) map[string]*CompileUnit {
	units := make(map[string]*CompileUnit)
	add := func(key, unitPath, file string) {
		if key == "" {
			key = "unit:" + normalizeUnitPath(fs, unitPath)
		}
		unit := units[key]
		if unit == nil {
			unit = &CompileUnit{Key: key, Path: unitPath, Language: language}
			units[key] = unit
		}
		unit.Files = append(unit.Files, file)
		if info, err := fs.Stat(file); err == nil && info != nil {
			unit.Bytes += info.Size()
		}
	}
	for _, file := range files {
		unitPath := unitDir(fs, file)
		key := "dir:" + normalizeUnitPath(fs, unitPath)
		switch language {
		case ssaconfig.JAVA:
			if strings.EqualFold(fs.Ext(file), ".java") {
				if pkg := scanJavaPackage(fs, file); pkg != "" {
					key = "java:" + pkg
					unitPath = packagePath(pkg)
				}
			} else {
				key = "resource:" + normalizeUnitPath(fs, unitPath)
			}
		case ssaconfig.GO:
			if strings.EqualFold(fs.Base(file), "go.mod") {
				key = "resource:go.mod"
				unitPath = unitDir(fs, file)
			}
		}
		add(key, unitPath, file)
	}
	for _, unit := range units {
		sort.Strings(unit.Files)
	}
	return units
}

func unitDependencies(language ssaconfig.Language, fs filesys_interface.FileSystem, units map[string]*CompileUnit) []UnitRef {
	switch language {
	case ssaconfig.JAVA:
		return javaUnitDependencies(fs, units)
	case ssaconfig.GO:
		return goUnitDependencies(fs, units)
	case ssaconfig.PYTHON:
		return pythonUnitDependencies(fs, units)
	case ssaconfig.C:
		return cUnitDependencies(fs, units)
	case ssaconfig.JS, ssaconfig.TS:
		return jsUnitDependencies(fs, units)
	default:
		// TODO: PHP autoload/include dynamic dependency graphs.
		return nil
	}
}

func javaUnitDependencies(fs filesys_interface.FileSystem, units map[string]*CompileUnit) []UnitRef {
	packageToKey := make(map[string]string)
	for key := range units {
		if strings.HasPrefix(key, "java:") {
			packageToKey[strings.TrimPrefix(key, "java:")] = key
		}
	}
	templateFileToKey, templateBaseToKey := javaTemplateResourceIndexes(fs, units)
	var edges []UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".java") {
				continue
			}
			src := readUnitSource(fs, file)
			for _, match := range javaImportRe.FindAllStringSubmatch(src, -1) {
				raw := match[1]
				if to := resolveJavaImport(packageToKey, raw); to != "" && to != unit.Key {
					edges = append(edges, UnitRef{From: unit.Key, To: to, Kind: "import", Raw: raw})
				}
			}
			for _, match := range javaStringRe.FindAllStringSubmatch(src, -1) {
				raw := javaUnquoteLight(match[1])
				if to := resolveJavaTemplateResource(fs, templateFileToKey, templateBaseToKey, raw); to != "" && to != unit.Key {
					edges = append(edges,
						UnitRef{From: unit.Key, To: to, Kind: "template", Raw: raw},
						UnitRef{From: to, To: unit.Key, Kind: "template-owner", Raw: raw},
					)
				}
			}
		}
	}
	return dedupeEdges(edges)
}

func javaTemplateResourceIndexes(fs filesys_interface.FileSystem, units map[string]*CompileUnit) (map[string]string, map[string]string) {
	fileIndex := newUniqueStringIndex()
	baseIndex := newUniqueStringIndex()
	for _, unit := range units {
		for _, file := range unit.Files {
			if !isJavaTemplatePath(file) {
				continue
			}
			normalized := normalizeUnitPath(fs, file)
			fileIndex.Add(normalized, unit.Key)
			if stem := stripJavaTemplateExtension(normalized); stem != normalized {
				fileIndex.Add(stem, unit.Key)
			}
			base := unitBase(fs, normalized)
			baseIndex.Add(base, unit.Key)
			if stem := stripJavaTemplateExtension(base); stem != base {
				baseIndex.Add(stem, unit.Key)
			}
		}
	}
	return fileIndex.Values(), baseIndex.Values()
}

func resolveJavaTemplateResource(fs filesys_interface.FileSystem, fileToKey map[string]string, baseToKey map[string]string, raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return ""
	}
	raw = stripResourceSuffix(raw)
	trimmed := strings.TrimLeft(raw, "/")
	candidates := javaTemplateResourceCandidates(fs, trimmed)
	for _, candidate := range candidates {
		if key := fileToKey[normalizeUnitPath(fs, candidate)]; key != "" {
			return key
		}
	}
	if key := baseToKey[unitBase(fs, trimmed)]; key != "" {
		return key
	}
	return ""
}

func isJavaTemplatePath(path string) bool {
	return javaTemplateExtension(path) != ""
}

func javaTemplateResourceCandidates(fs filesys_interface.FileSystem, raw string) []string {
	raw = strings.Trim(strings.ReplaceAll(raw, "\\", "/"), "/")
	if raw == "" {
		return nil
	}
	prefixes := []string{
		raw,
		fs.Join("src/main/webapp", raw),
		fs.Join("src/main/resources/templates", raw),
		fs.Join("src/main/resources", raw),
		fs.Join("src/main/resource", raw),
	}
	seen := make(map[string]struct{})
	var candidates []string
	add := func(candidate string) {
		candidate = cleanUnitPath(fs, candidate)
		if candidate == "" || candidate == "." {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}
	for _, prefix := range prefixes {
		add(prefix)
	}
	if javaTemplateExtension(raw) == "" {
		for _, prefix := range prefixes {
			for _, ext := range javaTemplateExtensions {
				add(prefix + ext)
			}
		}
	}
	return candidates
}

func javaTemplateExtension(path string) string {
	path = strings.ToLower(stripResourceSuffix(path))
	for _, ext := range javaTemplateExtensions {
		if strings.HasSuffix(path, ext) {
			return ext
		}
	}
	return ""
}

func stripJavaTemplateExtension(path string) string {
	ext := javaTemplateExtension(path)
	if ext == "" {
		return path
	}
	return path[:len(path)-len(ext)]
}

func stripResourceSuffix(path string) string {
	if idx := strings.IndexAny(path, "?#"); idx >= 0 {
		return path[:idx]
	}
	return path
}

func javaUnquoteLight(raw string) string {
	raw = strings.ReplaceAll(raw, `\/`, `/`)
	raw = strings.ReplaceAll(raw, `\\`, `\`)
	return raw
}

func goUnitDependencies(fs filesys_interface.FileSystem, units map[string]*CompileUnit) []UnitRef {
	pathToKey := goImportPathIndex(fs, units)
	var edges []UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".go") {
				continue
			}
			src := readUnitSource(fs, file)
			for _, match := range goImportRe.FindAllStringSubmatch(src, -1) {
				raw := match[1]
				if to := resolvePathImport(pathToKey, raw); to != "" && to != unit.Key {
					edges = append(edges, UnitRef{From: unit.Key, To: to, Kind: "import", Raw: raw})
				}
			}
		}
	}
	return dedupeEdges(edges)
}

type goModuleRoot struct {
	dir    string
	module string
}

func goImportPathIndex(fs filesys_interface.FileSystem, units map[string]*CompileUnit) map[string]string {
	roots := goModuleRoots(fs, units)
	index := newUniqueStringIndex()
	for _, unit := range units {
		if unit == nil {
			continue
		}
		unitPath := cleanUnitPath(fs, unit.Path)
		if unitPath == "." || strings.HasPrefix(unit.Key, "resource:") {
			continue
		}
		index.Add(unitPath, unit.Key)
		index.Add(unitBase(fs, unitPath), unit.Key)
		for _, root := range roots {
			rel, ok := relativeUnitPath(root.dir, unitPath)
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

func goModuleRoots(fs filesys_interface.FileSystem, units map[string]*CompileUnit) []goModuleRoot {
	seen := make(map[string]struct{})
	var roots []goModuleRoot
	for _, unit := range units {
		if unit == nil {
			continue
		}
		for _, file := range unit.Files {
			if !strings.EqualFold(unitBase(fs, file), "go.mod") {
				continue
			}
			dir := cleanUnitPath(fs, unitDir(fs, file))
			module := scanGoModulePath(readUnitSource(fs, file))
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

func pythonUnitDependencies(fs filesys_interface.FileSystem, units map[string]*CompileUnit) []UnitRef {
	pathToKey := unitPathIndex(units)
	var edges []UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".py") {
				continue
			}
			src := readUnitSource(fs, file)
			for _, match := range pyImportRe.FindAllStringSubmatch(src, -1) {
				raw := match[1]
				if raw == "" {
					raw = match[2]
				}
				if to := resolvePathImport(pathToKey, strings.ReplaceAll(raw, ".", "/")); to != "" && to != unit.Key {
					edges = append(edges, UnitRef{From: unit.Key, To: to, Kind: "import", Raw: raw})
				}
			}
		}
	}
	// TODO: handle relative imports and runtime importlib precisely.
	return dedupeEdges(edges)
}

func cUnitDependencies(fs filesys_interface.FileSystem, units map[string]*CompileUnit) []UnitRef {
	pathToKey := unitPathIndex(units)
	var edges []UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".c") && !strings.EqualFold(fs.Ext(file), ".h") {
				continue
			}
			src := readUnitSource(fs, file)
			for _, match := range cIncludeRe.FindAllStringSubmatch(src, -1) {
				raw := match[1]
				if to := resolvePathImport(pathToKey, raw); to != "" && to != unit.Key {
					edges = append(edges, UnitRef{From: unit.Key, To: to, Kind: "include", Raw: raw})
				}
			}
		}
	}
	// TODO: macro-generated include paths need language-specific expansion.
	return dedupeEdges(edges)
}

func jsUnitDependencies(fs filesys_interface.FileSystem, units map[string]*CompileUnit) []UnitRef {
	pathToKey := unitPathIndex(units)
	fileToKey := unitFileIndex(fs, units)
	var edges []UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			ext := strings.ToLower(fs.Ext(file))
			if ext != ".js" && ext != ".ts" && ext != ".tsx" && ext != ".jsx" {
				continue
			}
			src := readUnitSource(fs, file)
			for _, raw := range scanJSImportSpecs(src) {
				if !strings.HasPrefix(raw, ".") {
					continue
				}
				if to := resolveRelativeImportUnit(fs, pathToKey, fileToKey, file, raw, jsModuleExtensions); to != "" && to != unit.Key {
					edges = append(edges, UnitRef{From: unit.Key, To: to, Kind: "import", Raw: raw})
				}
			}
		}
	}
	// TODO: path aliases, tsconfig baseUrl/paths, package exports, and dynamic expressions.
	return dedupeEdges(edges)
}

func topoCompileUnits(units map[string]*CompileUnit, edges []UnitRef) *UnitPlan {
	sccs := stronglyConnectedUnits(units, edges)
	sccIndex := make(map[string]int)
	for idx, scc := range sccs {
		for _, unit := range scc {
			sccIndex[unit.Key] = idx
		}
	}
	out := make(map[int]map[int]struct{})
	indegree := make(map[int]int)
	for i := range sccs {
		indegree[i] = 0
	}
	for _, edge := range edges {
		from, fromOK := sccIndex[edge.From]
		to, toOK := sccIndex[edge.To]
		if !fromOK || !toOK || from == to {
			continue
		}
		if out[to] == nil {
			out[to] = make(map[int]struct{})
		}
		if _, exists := out[to][from]; !exists {
			out[to][from] = struct{}{}
			indegree[from]++
		}
	}
	queue := make([]int, 0)
	for idx, degree := range indegree {
		if degree == 0 {
			queue = append(queue, idx)
		}
	}
	sort.Ints(queue)
	var order [][]*CompileUnit
	for len(queue) > 0 {
		idx := queue[0]
		queue = queue[1:]
		order = append(order, sccs[idx])
		next := make([]int, 0, len(out[idx]))
		for dep := range out[idx] {
			indegree[dep]--
			if indegree[dep] == 0 {
				next = append(next, dep)
			}
		}
		sort.Ints(next)
		queue = append(queue, next...)
	}
	if len(order) != len(sccs) {
		order = sccs
	}
	return &UnitPlan{Units: units, Edges: edges, Order: order}
}

func stronglyConnectedUnits(units map[string]*CompileUnit, edges []UnitRef) [][]*CompileUnit {
	keys := make([]string, 0, len(units))
	for key := range units {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	graph := make(map[string][]string)
	for _, edge := range edges {
		if units[edge.From] == nil || units[edge.To] == nil {
			continue
		}
		graph[edge.From] = append(graph[edge.From], edge.To)
	}
	for key := range graph {
		sort.Strings(graph[key])
	}
	index := 0
	stack := make([]string, 0)
	onStack := make(map[string]bool)
	indices := make(map[string]int)
	lowlink := make(map[string]int)
	var sccs [][]*CompileUnit
	var visit func(string)
	visit = func(v string) {
		indices[v] = index
		lowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range graph[v] {
			if _, seen := indices[w]; !seen {
				visit(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] && indices[w] < lowlink[v] {
				lowlink[v] = indices[w]
			}
		}
		if lowlink[v] != indices[v] {
			return
		}
		var scc []*CompileUnit
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			scc = append(scc, units[w])
			if w == v {
				break
			}
		}
		sort.Slice(scc, func(i, j int) bool { return scc[i].Key < scc[j].Key })
		sccs = append(sccs, scc)
	}
	for _, key := range keys {
		if _, seen := indices[key]; !seen {
			visit(key)
		}
	}
	sort.SliceStable(sccs, func(i, j int) bool { return sccs[i][0].Key < sccs[j][0].Key })
	return sccs
}

func scanJavaPackage(fs filesys_interface.FileSystem, file string) string {
	src := readUnitSource(fs, file)
	match := javaPackageRe.FindStringSubmatch(src)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func resolveJavaImport(packageToKey map[string]string, raw string) string {
	if key := packageToKey[raw]; key != "" {
		return key
	}
	bestPkg := ""
	for pkg := range packageToKey {
		if strings.HasPrefix(raw, pkg+".") && len(pkg) > len(bestPkg) {
			bestPkg = pkg
		}
	}
	if bestPkg == "" {
		return ""
	}
	return packageToKey[bestPkg]
}

func resolvePathImport(pathToKey map[string]string, raw string) string {
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

func unitPathIndex(units map[string]*CompileUnit) map[string]string {
	ret := make(map[string]string)
	for _, unit := range units {
		p := strings.Trim(strings.ReplaceAll(unit.Path, "\\", "/"), "/")
		if p == "." || p == "" {
			continue
		}
		ret[p] = unit.Key
	}
	return ret
}

type uniqueStringIndex struct {
	values     map[string]string
	collisions map[string]struct{}
}

func newUniqueStringIndex() *uniqueStringIndex {
	return &uniqueStringIndex{
		values:     make(map[string]string),
		collisions: make(map[string]struct{}),
	}
}

func (idx *uniqueStringIndex) Add(raw string, key string) {
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

func (idx *uniqueStringIndex) Values() map[string]string {
	ret := make(map[string]string, len(idx.values))
	for raw, key := range idx.values {
		if _, collided := idx.collisions[raw]; collided {
			continue
		}
		ret[raw] = key
	}
	return ret
}

func unitFileIndex(fs filesys_interface.FileSystem, units map[string]*CompileUnit) map[string]string {
	ret := make(map[string]string)
	for _, unit := range units {
		for _, file := range unit.Files {
			ret[normalizeUnitPath(fs, file)] = unit.Key
		}
	}
	return ret
}

var jsModuleExtensions = []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs", ".json"}

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
	sort.Strings(ret)
	return ret
}

func resolveRelativeImportUnit(
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
	importerDir := unitDir(fs, importerFile)
	base := cleanUnitPath(fs, fs.Join(importerDir, raw))
	candidates := []string{base}
	if fs.Ext(base) == "" {
		for _, ext := range extensions {
			candidates = append(candidates, base+ext)
		}
		for _, ext := range extensions {
			candidates = append(candidates, cleanUnitPath(fs, fs.Join(base, "index"+ext)))
		}
	}
	for _, candidate := range candidates {
		if key := fileToKey[normalizeUnitPath(fs, candidate)]; key != "" {
			return key
		}
	}
	if key := pathToKey[base]; key != "" {
		return key
	}
	if key := pathToKey[unitDir(fs, base)]; key != "" {
		return key
	}
	return ""
}

func cleanUnitPath(fs filesys_interface.FileSystem, p string) string {
	parts := strings.Split(normalizeUnitPath(fs, p), "/")
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

func relativeUnitPath(root, target string) (string, bool) {
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

func dedupeEdges(edges []UnitRef) []UnitRef {
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

func readUnitSource(fs filesys_interface.FileSystem, file string) string {
	data, err := fs.ReadFile(file)
	if err != nil {
		return ""
	}
	return string(data)
}

func unitDir(fs filesys_interface.FileSystem, file string) string {
	file = normalizeUnitPath(fs, file)
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

func normalizeUnitPath(fs filesys_interface.FileSystem, p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.ReplaceAll(p, string(fs.GetSeparators()), "/")
	p = strings.Trim(p, "/")
	if p == "" {
		return "."
	}
	return p
}

func unitBase(fs filesys_interface.FileSystem, p string) string {
	p = normalizeUnitPath(fs, p)
	if p == "." {
		return "."
	}
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		return p[idx+1:]
	}
	return p
}

func packagePath(pkg string) string {
	pkg = strings.Trim(strings.ReplaceAll(pkg, ".", "/"), "/")
	if pkg == "" {
		return "."
	}
	return pkg
}
