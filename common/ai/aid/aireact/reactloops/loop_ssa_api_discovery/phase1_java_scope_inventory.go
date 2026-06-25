package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	javaScopeInventorySchemaVersion = 1
	scopeUnitKindMavenModule        = "maven_module"
	scopeUnitKindJavaPackage        = "java_package"
	scopeUnitKindResourceBundle     = "resource_bundle"
	scopeUnitKindWebappBundle       = "webapp_bundle"
)

var (
	rePomModules        = regexp.MustCompile(`(?s)<modules>\s*(.*?)\s*</modules>`)
	rePomModule         = regexp.MustCompile(`<module>\s*([^<]+)\s*</module>`)
	rePomPackaging      = regexp.MustCompile(`<packaging>\s*([^<]+)\s*</packaging>`)
	reSpringBootAppFile = regexp.MustCompile(`@SpringBootApplication`)
)

// JavaScopeUnit is one assignable business audit unit under a Java module.
type JavaScopeUnit struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Path          string   `json:"path"`
	DomainSegment string   `json:"domain_segment,omitempty"`
	JavaFileCount int      `json:"java_file_count,omitempty"`
	Hints         []string `json:"hints,omitempty"`
}

// JavaModuleScope describes one Maven/Gradle module within the repo.
type JavaModuleScope struct {
	ModuleRoot  string          `json:"module_root"`
	BuildFile   string          `json:"build_file,omitempty"`
	Packaging   string          `json:"packaging,omitempty"`
	BasePackage string          `json:"base_package,omitempty"`
	ScopeUnits  []JavaScopeUnit `json:"scope_units"`
}

// JavaCoveragePolicy defines which unit kinds must be fully assigned.
type JavaCoveragePolicy struct {
	RequiredKinds            []string `json:"required_kinds"`
	OptionalKinds            []string `json:"optional_kinds"`
	ModuleRootCoversChildren bool     `json:"module_root_covers_children"`
}

// JavaScopeInventoryStats summarizes generated units.
type JavaScopeInventoryStats struct {
	TotalUnits          int `json:"total_units"`
	JavaPackageUnits    int `json:"java_package_units"`
	ResourceBundleUnits int `json:"resource_bundle_units,omitempty"`
	WebappBundleUnits   int `json:"webapp_bundle_units,omitempty"`
}

// JavaBusinessScopeInventory is persisted to java_business_scope_inventory.json.
type JavaBusinessScopeInventory struct {
	SchemaVersion  int                     `json:"schema_version"`
	GeneratedAt    string                  `json:"generated_at"`
	Language       string                  `json:"language"`
	Layout         string                  `json:"layout"`
	CodeRoot       string                  `json:"code_root"`
	Modules        []JavaModuleScope       `json:"modules"`
	CoveragePolicy JavaCoveragePolicy      `json:"coverage_policy"`
	Stats          JavaScopeInventoryStats `json:"stats"`
}

type javaPackageScan struct {
	relPath       string
	javaFileCount int
	classNames    []string
	packageName   string
}

// BuildJavaBusinessScopeInventory scans a Java repo and writes java_business_scope_inventory.json.
func BuildJavaBusinessScopeInventory(rt *Runtime) (*JavaBusinessScopeInventory, error) {
	if rt == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	if !rt.Session.CodePathOK {
		return nil, utils.Error("code_path not ok")
	}
	codeRoot := rt.Session.CodeRootPath
	if !isJavaProjectRoot(codeRoot) {
		return buildCodeScopeInventoryFallback(rt)
	}

	moduleRoots, layout := detectJavaModuleRoots(codeRoot)
	inv := &JavaBusinessScopeInventory{
		SchemaVersion: javaScopeInventorySchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Language:      "java",
		Layout:        layout,
		CodeRoot:      codeRoot,
		CoveragePolicy: JavaCoveragePolicy{
			RequiredKinds:            []string{scopeUnitKindJavaPackage},
			OptionalKinds:            []string{scopeUnitKindResourceBundle, scopeUnitKindWebappBundle},
			ModuleRootCoversChildren: true,
		},
	}

	for _, modRoot := range moduleRoots {
		mod := scanJavaModuleScope(codeRoot, modRoot)
		if len(mod.ScopeUnits) > 0 {
			inv.Modules = append(inv.Modules, mod)
		}
	}
	inv.Stats = computeJavaScopeStats(inv)
	if err := persistJavaBusinessScopeInventory(rt, inv); err != nil {
		return inv, err
	}
	log.Infof("ssa_api_discovery: java_business_scope_inventory modules=%d java_package_units=%d",
		len(inv.Modules), inv.Stats.JavaPackageUnits)
	return inv, nil
}

func isJavaProjectRoot(codeRoot string) bool {
	if utils.FileExists(filepath.Join(codeRoot, "pom.xml")) {
		return true
	}
	if utils.FileExists(filepath.Join(codeRoot, "build.gradle")) || utils.FileExists(filepath.Join(codeRoot, "build.gradle.kts")) {
		return true
	}
	return false
}

func detectJavaModuleRoots(codeRoot string) (moduleRoots []string, layout string) {
	rootPom := filepath.Join(codeRoot, "pom.xml")
	if b, err := os.ReadFile(rootPom); err == nil {
		if mods := parseMavenModules(string(b)); len(mods) > 0 {
			for _, m := range mods {
				m = strings.TrimSpace(filepath.ToSlash(m))
				if m != "" {
					moduleRoots = append(moduleRoots, m)
				}
			}
			if len(moduleRoots) > 0 {
				return moduleRoots, "multi_module"
			}
		}
	}
	return []string{"."}, "single_module"
}

func parseMavenModules(pomContent string) []string {
	m := rePomModules.FindStringSubmatch(pomContent)
	if len(m) < 2 {
		return nil
	}
	var out []string
	for _, sm := range rePomModule.FindAllStringSubmatch(m[1], -1) {
		if len(sm) > 1 {
			out = append(out, strings.TrimSpace(sm[1]))
		}
	}
	return out
}

func scanJavaModuleScope(codeRoot, moduleRel string) JavaModuleScope {
	moduleRel = normScopePath(moduleRel)
	if moduleRel == "." {
		moduleRel = ""
	}
	moduleAbs := codeRoot
	buildFile := "pom.xml"
	if moduleRel != "" {
		moduleAbs = filepath.Join(codeRoot, moduleRel)
		buildFile = filepath.ToSlash(filepath.Join(moduleRel, "pom.xml"))
	}
	packaging := "jar"
	if b, err := os.ReadFile(filepath.Join(moduleAbs, "pom.xml")); err == nil {
		if m := rePomPackaging.FindStringSubmatch(string(b)); len(m) > 1 {
			packaging = strings.TrimSpace(m[1])
		}
	}

	basePackage := inferModuleBasePackage(moduleAbs)
	packages := scanJavaPackages(moduleAbs, moduleRel)
	resourceBundles := scanResourceBundles(moduleAbs, moduleRel)
	webappBundles := scanWebappBundles(moduleAbs, moduleRel)

	var units []JavaScopeUnit
	modRootPath := moduleRel
	if modRootPath == "" {
		modRootPath = "."
	}
	units = append(units, JavaScopeUnit{
		ID:   scopeUnitID(modRootPath, scopeUnitKindMavenModule, modRootPath),
		Kind: scopeUnitKindMavenModule,
		Path: modRootPath,
	})

	for _, pkg := range packages {
		domain := domainSegmentFromPackage(basePackage, pkg.packageName)
		hints := dedupeStrings(append([]string{}, pkg.classNames...))
		sort.Strings(hints)
		if len(hints) > 8 {
			hints = hints[:8]
		}
		units = append(units, JavaScopeUnit{
			ID:            scopeUnitID(modRootPath, scopeUnitKindJavaPackage, pkg.relPath),
			Kind:          scopeUnitKindJavaPackage,
			Path:          pkg.relPath,
			DomainSegment: domain,
			JavaFileCount: pkg.javaFileCount,
			Hints:         hints,
		})
	}
	for _, rb := range resourceBundles {
		units = append(units, JavaScopeUnit{
			ID:   scopeUnitID(modRootPath, scopeUnitKindResourceBundle, rb),
			Kind: scopeUnitKindResourceBundle,
			Path: rb,
		})
	}
	for _, wb := range webappBundles {
		units = append(units, JavaScopeUnit{
			ID:   scopeUnitID(modRootPath, scopeUnitKindWebappBundle, wb),
			Kind: scopeUnitKindWebappBundle,
			Path: wb,
		})
	}

	return JavaModuleScope{
		ModuleRoot:  modRootPath,
		BuildFile:   buildFile,
		Packaging:   packaging,
		BasePackage: basePackage,
		ScopeUnits:  units,
	}
}

func inferModuleBasePackage(moduleAbs string) string {
	javaRoot := filepath.Join(moduleAbs, "src", "main", "java")
	var bootPkg string
	var packages []string
	_ = filepath.Walk(javaRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".java") {
			return nil
		}
		rel, _ := filepath.Rel(javaRoot, path)
		if skipJavaSourceRelPath(filepath.ToSlash(rel)) {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(b)
		if m := reJavaPackage.FindStringSubmatch(content); len(m) > 1 {
			pkg := strings.TrimSpace(m[1])
			if pkg != "" {
				packages = append(packages, pkg)
			}
			if bootPkg == "" && reSpringBootAppFile.MatchString(content) {
				bootPkg = pkg
			}
		}
		return nil
	})
	if bootPkg != "" {
		return bootPkg
	}
	return longestCommonPackagePrefix(packages, 2)
}

func scanJavaPackages(moduleAbs, moduleRel string) []javaPackageScan {
	javaRoot := filepath.Join(moduleAbs, "src", "main", "java")
	if _, err := os.Stat(javaRoot); err != nil {
		return nil
	}
	type agg struct {
		count   int
		classes []string
		pkg     string
	}
	byDir := map[string]*agg{}
	_ = filepath.Walk(javaRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".java") {
			return nil
		}
		relFromJava, _ := filepath.Rel(javaRoot, path)
		relFromJava = filepath.ToSlash(relFromJava)
		if skipJavaSourceRelPath(relFromJava) {
			return nil
		}
		dir := filepath.ToSlash(filepath.Dir(relFromJava))
		if dir == "." {
			dir = ""
		}
		var fullRel string
		if moduleRel != "" {
			fullRel = filepath.ToSlash(filepath.Join(moduleRel, "src", "main", "java", dir))
		} else {
			fullRel = filepath.ToSlash(filepath.Join("src", "main", "java", dir))
		}
		a := byDir[fullRel]
		if a == nil {
			a = &agg{}
			byDir[fullRel] = a
		}
		a.count++
		base := strings.TrimSuffix(filepath.Base(path), ".java")
		if strings.HasSuffix(base, "Controller") || strings.HasSuffix(base, "Service") || strings.HasSuffix(base, "Repository") {
			a.classes = append(a.classes, base)
		}
		b, _ := os.ReadFile(path)
		if m := reJavaPackage.FindStringSubmatch(string(b)); len(m) > 1 {
			a.pkg = strings.TrimSpace(m[1])
		}
		return nil
	})
	var out []javaPackageScan
	for rel, a := range byDir {
		if a.count == 0 {
			continue
		}
		out = append(out, javaPackageScan{
			relPath:       rel,
			javaFileCount: a.count,
			classNames:    a.classes,
			packageName:   a.pkg,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].relPath < out[j].relPath })
	return out
}

func scanResourceBundles(moduleAbs, moduleRel string) []string {
	resRoot := filepath.Join(moduleAbs, "src", "main", "resources")
	return scanBundleDirs(resRoot, moduleRel)
}

func scanWebappBundles(moduleAbs, moduleRel string) []string {
	webRoot := filepath.Join(moduleAbs, "src", "main", "webapp")
	if _, err := os.Stat(webRoot); err != nil {
		return nil
	}
	return []string{joinModuleRel(moduleRel, "src/main/webapp")}
}

func scanBundleDirs(root, moduleRel string) []string {
	if _, err := os.Stat(root); err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".xml" && ext != ".yml" && ext != ".yaml" && ext != ".properties" && ext != ".sql" {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}
		var full string
		if moduleRel != "" {
			full = filepath.ToSlash(filepath.Join(moduleRel, "src", "main", "resources", rel))
		} else {
			full = filepath.ToSlash(filepath.Join("src", "main", "resources", rel))
		}
		full = strings.TrimSuffix(full, "/")
		if full == "" {
			return nil
		}
		if _, ok := seen[full]; ok {
			return nil
		}
		seen[full] = struct{}{}
		out = append(out, full)
		return nil
	})
	sort.Strings(out)
	return out
}

func joinModuleRel(moduleRel, suffix string) string {
	if moduleRel == "" {
		return filepath.ToSlash(suffix)
	}
	return filepath.ToSlash(filepath.Join(moduleRel, suffix))
}

func skipJavaSourceRelPath(rel string) bool {
	lower := strings.ToLower(filepath.ToSlash(rel))
	if strings.Contains(lower, "/test/") || strings.HasPrefix(lower, "test/") {
		return true
	}
	if strings.Contains(lower, "generated-sources") || strings.Contains(lower, "generated-test-sources") {
		return true
	}
	return false
}

func domainSegmentFromPackage(basePackage, fullPackage string) string {
	basePackage = strings.TrimSpace(basePackage)
	fullPackage = strings.TrimSpace(fullPackage)
	if basePackage == "" || fullPackage == "" {
		return ""
	}
	if fullPackage == basePackage {
		parts := strings.Split(basePackage, ".")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return ""
	}
	prefix := basePackage + "."
	if !strings.HasPrefix(fullPackage, prefix) {
		parts := strings.Split(fullPackage, ".")
		if len(parts) >= 3 {
			return parts[2]
		}
		return ""
	}
	rest := strings.TrimPrefix(fullPackage, prefix)
	return strings.SplitN(rest, ".", 2)[0]
}

func longestCommonPackagePrefix(packages []string, minSegments int) string {
	if len(packages) == 0 {
		return ""
	}
	split := func(p string) []string { return strings.Split(p, ".") }
	base := split(packages[0])
	for _, p := range packages[1:] {
		parts := split(p)
		i := 0
		for i < len(base) && i < len(parts) && base[i] == parts[i] {
			i++
		}
		base = base[:i]
		if len(base) == 0 {
			break
		}
	}
	if len(base) < minSegments {
		return ""
	}
	return strings.Join(base, ".")
}

func scopeUnitID(moduleRoot, kind, path string) string {
	path = normScopePath(path)
	moduleRoot = normScopePath(moduleRoot)
	if moduleRoot == "." {
		moduleRoot = "root"
	}
	return moduleRoot + ":" + kind + ":" + path
}

func computeJavaScopeStats(inv *JavaBusinessScopeInventory) JavaScopeInventoryStats {
	var stats JavaScopeInventoryStats
	for _, mod := range inv.Modules {
		for _, u := range mod.ScopeUnits {
			stats.TotalUnits++
			switch u.Kind {
			case scopeUnitKindJavaPackage:
				stats.JavaPackageUnits++
			case scopeUnitKindResourceBundle:
				stats.ResourceBundleUnits++
			case scopeUnitKindWebappBundle:
				stats.WebappBundleUnits++
			}
		}
	}
	return stats
}

func persistJavaBusinessScopeInventory(rt *Runtime, inv *JavaBusinessScopeInventory) error {
	if rt == nil || inv == nil {
		return utils.Error("nil inventory")
	}
	b, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return err
	}
	if err := writeJSONFile(store.JavaBusinessScopeInventoryPath(rt.WorkDir), b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactJavaBusinessScopeInventory, string(b))
	}
	return nil
}

func loadJavaBusinessScopeInventory(workDir string) (*JavaBusinessScopeInventory, error) {
	b, err := os.ReadFile(store.JavaBusinessScopeInventoryPath(workDir))
	if err != nil {
		return nil, err
	}
	var inv JavaBusinessScopeInventory
	if err := json.Unmarshal(b, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

func buildCodeScopeInventoryFallback(rt *Runtime) (*JavaBusinessScopeInventory, error) {
	codeRoot := rt.Session.CodeRootPath
	inv := &JavaBusinessScopeInventory{
		SchemaVersion: javaScopeInventorySchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Language:      strings.TrimSpace(rt.Session.Language),
		Layout:        "generic",
		CodeRoot:      codeRoot,
		CoveragePolicy: JavaCoveragePolicy{
			RequiredKinds:            []string{scopeUnitKindJavaPackage},
			OptionalKinds:            []string{},
			ModuleRootCoversChildren: true,
		},
	}
	var units []JavaScopeUnit
	_ = filepath.Walk(codeRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || !info.IsDir() {
			return nil
		}
		if info.Name() != "." && skipDirForHarvest(info.Name()) {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(codeRoot, path)
		if rel == "." {
			return nil
		}
		rel = normScopePath(rel)
		if hasSourceFileUnder(path) {
			units = append(units, JavaScopeUnit{
				ID:   scopeUnitID(".", scopeUnitKindJavaPackage, rel),
				Kind: scopeUnitKindJavaPackage,
				Path: rel,
			})
		}
		return nil
	})
	inv.Modules = []JavaModuleScope{{
		ModuleRoot: ".",
		ScopeUnits: units,
	}}
	inv.Stats = computeJavaScopeStats(inv)
	path := store.CodeScopeInventoryPath(rt.WorkDir)
	b, _ := json.MarshalIndent(inv, "", "  ")
	_ = writeJSONFile(path, b)
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactCodeScopeInventory, string(b))
	}
	return inv, persistJavaBusinessScopeInventory(rt, inv)
}

func hasSourceFileUnder(dir string) bool {
	found := false
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".java", ".go", ".py", ".php", ".js", ".ts", ".tsx", ".vue":
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func normScopePath(p string) string {
	p = filepath.ToSlash(strings.TrimSpace(p))
	p = strings.TrimPrefix(p, "./")
	return strings.TrimSuffix(p, "/")
}

func isScopePathDescendantOrEqual(child, ancestor string) bool {
	child = normScopePath(child)
	ancestor = normScopePath(ancestor)
	if child == "" || ancestor == "" {
		return false
	}
	if child == ancestor {
		return true
	}
	return strings.HasPrefix(child+"/", ancestor+"/") || strings.HasPrefix(ancestor+"/", child+"/")
}
