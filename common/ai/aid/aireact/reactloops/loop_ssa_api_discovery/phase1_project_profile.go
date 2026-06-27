package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const projectProfileSchemaVersion = 1

// ProjectProfileV1 Stage0 output: directory tree, file classification, framework detection, entry points.
type ProjectProfileV1 struct {
	SchemaVersion   int                    `json:"schema_version"`
	GeneratedAt     string                 `json:"generated_at"`
	CodeRoot        string                 `json:"code_root"`
	Language        string                 `json:"language"`
	ContextPath     string                 `json:"context_path"`
	ContextPathSrc  string                 `json:"context_path_source,omitempty"`
	ServerPort      string                 `json:"server_port,omitempty"`
	Frameworks      []FrameworkDetection   `json:"frameworks"`
	Files           []ProjectFileEntry     `json:"files"`
	EntryPoints     []ProjectEntryPoint    `json:"entry_points"`
	WorklistSeed    []WorklistSeedItem     `json:"worklist_seed"`
	Stats           map[string]int         `json:"stats"`
	Warnings        []string               `json:"warnings,omitempty"`
}

type ProjectFileEntry struct {
	RelPath  string `json:"rel_path"`
	Size     int64  `json:"size"`
	Ext      string `json:"ext"`
	Category string `json:"category"`
}

type FrameworkDetection struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Confidence string `json:"confidence"`
	Evidence   string `json:"evidence"`
}

type ProjectEntryPoint struct {
	Kind    string `json:"kind"`
	RelPath string `json:"rel_path"`
	Hint    string `json:"hint"`
}

type WorklistSeedItem struct {
	RelPath  string `json:"rel_path"`
	Reason   string `json:"reason"`
	Category string `json:"category,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

var (
	reJavaMainClass     = regexp.MustCompile(`(?m)^\s*public\s+class\s+(\w+)`)
	reSpringBootApp     = regexp.MustCompile(`@SpringBootApplication`)
	reController        = regexp.MustCompile(`@(?:Rest)?Controller\b`)
	reSecurityConfig    = regexp.MustCompile(`@EnableWebSecurity|extends\s+WebSecurityConfigurerAdapter|SecurityFilterChain`)
	reWebMvcConfigurer  = regexp.MustCompile(`implements\s+WebMvcConfigurer|extends\s+WebMvcConfigurationSupport`)
)

// RunBuildProjectProfile scans code root and writes project_profile.json (Stage0).
func RunBuildProjectProfile(rt *Runtime) (*ProjectProfileV1, error) {
	if rt == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	sess := rt.Session
	profile := &ProjectProfileV1{
		SchemaVersion: projectProfileSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		CodeRoot:      sess.CodeRootPath,
		Language:      strings.TrimSpace(sess.Language),
		ContextPath:   "unknown",
		Stats:         map[string]int{},
	}
	if !sess.CodePathOK || strings.TrimSpace(sess.CodeRootPath) == "" {
		profile.Warnings = append(profile.Warnings, "code path invalid; empty profile")
		return profile, writeProjectProfile(rt, profile)
	}

	root := filepath.Clean(sess.CodeRootPath)
	frameworkSeen := map[string]struct{}{}
	addFramework := func(id, label, conf, evidence string) {
		if _, ok := frameworkSeen[id]; ok {
			return
		}
		frameworkSeen[id] = struct{}{}
		profile.Frameworks = append(profile.Frameworks, FrameworkDetection{
			ID: id, Label: label, Confidence: conf, Evidence: evidence,
		})
	}

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if shouldSkipPathForCodeHarvest(rel) && classifyFileCategory(rel, nil) == fileCategoryResource {
			// still catalog resource files but don't walk into huge dirs above
		}

		var head []byte
		if info.Size() > 0 && info.Size() <= 512*1024 {
			head, _ = readFileHead(path, 8192)
		}
		cat := classifyFileCategory(rel, head)
		profile.Files = append(profile.Files, ProjectFileEntry{
			RelPath: rel, Size: info.Size(), Ext: strings.ToLower(filepath.Ext(rel)), Category: cat,
		})
		profile.Stats[cat]++

		base := strings.ToLower(filepath.Base(rel))
		switch {
		case base == "pom.xml" || base == "build.gradle" || base == "build.gradle.kts":
			scanBuildFileForFrameworks(path, rel, addFramework)
		case cat == fileCategoryConfig:
			scanConfigFileForProfile(profile, path, rel, head)
		case cat == fileCategoryCode && strings.HasSuffix(strings.ToLower(rel), ".java"):
			scanJavaFileForEntryPoints(profile, rel, string(head))
		}
		return nil
	})

	if len(profile.Frameworks) == 0 {
		profile.Warnings = append(profile.Warnings, "no framework detected")
	}
	if profile.ContextPath == "" {
		profile.ContextPath = "unknown"
	}
	profile.WorklistSeed = nil // worklist comes from phase1_recon ReAct output

	if err := validateProjectProfileContract(profile); err != nil {
		profile.Warnings = append(profile.Warnings, "contract: "+err.Error())
	}
	if werr := writeProjectProfile(rt, profile); werr != nil {
		return profile, werr
	}
	log.Infof("ssa_api_discovery: project_profile files=%d frameworks=%d seeds=%d",
		len(profile.Files), len(profile.Frameworks), len(profile.WorklistSeed))
	return profile, nil
}

func writeProjectProfile(rt *Runtime, profile *ProjectProfileV1) error {
	path := store.ProjectProfilePath(rt.WorkDir)
	b, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	if err := writeJSONFile(path, b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactProjectProfile, string(b))
	}
	return nil
}

func validateProjectProfileContract(p *ProjectProfileV1) error {
	if p == nil {
		return utils.Error("nil profile")
	}
	if len(p.Files) == 0 {
		return utils.Error("files list empty")
	}
	if len(p.Frameworks) == 0 {
		return utils.Error("no framework detection")
	}
	if strings.TrimSpace(p.ContextPath) == "" {
		return utils.Error("context_path missing")
	}
	return nil
}

func scanBuildFileForFrameworks(abs, rel string, add func(id, label, conf, evidence string)) {
	data, err := os.ReadFile(abs)
	if err != nil {
		return
	}
	s := string(data)
	lower := strings.ToLower(s)
	if strings.Contains(lower, "spring-boot") || strings.Contains(lower, "springframework") {
		add("spring", "Spring / Spring Boot", "high", rel)
	}
	if strings.Contains(lower, "javax.ws.rs") || strings.Contains(lower, "jakarta.ws.rs") {
		add("jaxrs", "JAX-RS", "medium", rel)
	}
	if strings.Contains(lower, "struts") {
		add("struts", "Apache Struts", "medium", rel)
	}
	if strings.Contains(lower, "servlet") || strings.Contains(lower, "web.xml") {
		add("servlet", "Java Servlet", "low", rel)
	}
}

func scanConfigFileForProfile(p *ProjectProfileV1, abs, rel string, head []byte) {
	if len(head) == 0 {
		var err error
		head, err = readFileHead(abs, 256*1024)
		if err != nil {
			return
		}
	}
	s := string(head)
	for _, m := range reSpringCtxPath.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 {
			p.ContextPath = normURLPath(m[1])
			p.ContextPathSrc = rel + ":context-path"
			break
		}
	}
	for _, m := range reServletCtx.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 && (p.ContextPath == "" || p.ContextPath == "unknown") {
			p.ContextPath = normURLPath(m[1])
			p.ContextPathSrc = rel + ":servlet.context-path"
		}
	}
	for _, m := range reServerPort.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 && p.ServerPort == "" {
			p.ServerPort = m[1]
		}
	}
	if strings.Contains(strings.ToLower(filepath.Base(rel)), "web.xml") {
		p.EntryPoints = append(p.EntryPoints, ProjectEntryPoint{Kind: "servlet_config", RelPath: rel, Hint: "web.xml deployment descriptors"})
	}
}

func scanJavaFileForEntryPoints(p *ProjectProfileV1, rel, content string) {
	if reSpringBootApp.MatchString(content) {
		p.EntryPoints = append(p.EntryPoints, ProjectEntryPoint{Kind: "spring_boot_main", RelPath: rel, Hint: "@SpringBootApplication"})
	}
	if reController.MatchString(content) {
		kind := "controller"
		if isAuthEntryPath(rel) {
			kind = "auth_entry"
		}
		p.EntryPoints = append(p.EntryPoints, ProjectEntryPoint{Kind: kind, RelPath: rel, Hint: "@Controller/@RestController"})
	}
	if reSecurityConfig.MatchString(content) {
		p.EntryPoints = append(p.EntryPoints, ProjectEntryPoint{Kind: "security_config", RelPath: rel, Hint: "security configuration"})
	}
	if reWebMvcConfigurer.MatchString(content) {
		p.EntryPoints = append(p.EntryPoints, ProjectEntryPoint{Kind: "web_mvc_config", RelPath: rel, Hint: "WebMvcConfigurer"})
	}
}

func authEntryContentHint(content string) bool {
	// Deprecated: auth_entry classification uses isAuthEntryPath only (narrow path heuristics).
	_ = content
	return false
}

func buildWorklistSeedFromProfile(p *ProjectProfileV1) []WorklistSeedItem {
	seen := map[string]struct{}{}
	var seed []WorklistSeedItem
	add := func(rel, reason, category string, priority int) {
		rel = filepath.ToSlash(strings.TrimSpace(rel))
		if rel == "" || !isBackendCodeRelPath(rel) {
			return
		}
		if _, ok := seen[rel]; ok {
			return
		}
		seen[rel] = struct{}{}
		seed = append(seed, WorklistSeedItem{RelPath: rel, Reason: reason, Category: category, Priority: priority})
	}
	addAuthTemplate := func(rel, reason string) {
		rel = filepath.ToSlash(strings.TrimSpace(rel))
		if rel == "" || !isLoginTemplateRelPath(rel) {
			return
		}
		if _, ok := seen[rel]; ok {
			return
		}
		seen[rel] = struct{}{}
		seed = append(seed, WorklistSeedItem{RelPath: rel, Reason: reason, Category: worklistCategoryAuthEntry, Priority: 2})
	}

	// Tier 1 — routing / security / deployment config (must yield mount prefixes before controllers)
	for _, ep := range p.EntryPoints {
		switch ep.Kind {
		case "web_mvc_config", "security_config", "servlet_config", "spring_boot_main":
			add(ep.RelPath, ep.Hint, worklistCategoryRoutingConfig, 1)
		}
	}
	for _, f := range p.Files {
		if f.Category != fileCategoryConfig {
			continue
		}
		b := strings.ToLower(filepath.Base(f.RelPath))
		if strings.HasPrefix(b, "application") || b == "web.xml" {
			add(f.RelPath, "routing/security config", worklistCategoryBuildConfig, 1)
		}
	}

	// Tier 2 — authentication entry handlers (by path/content category, not hardcoded class names)
	for _, ep := range p.EntryPoints {
		if ep.Kind == "auth_entry" {
			add(ep.RelPath, ep.Hint, worklistCategoryAuthEntry, 2)
		}
	}
	for _, f := range p.Files {
		if isLoginTemplateRelPath(f.RelPath) {
			addAuthTemplate(f.RelPath, "login UI template")
		}
	}

	// Tier 3 — API controllers grouped by package (backend Java only; skip templates/resources)
	packageSeen := map[string]int{}
	for _, ep := range p.EntryPoints {
		if ep.Kind != "controller" {
			continue
		}
		pkg := filepath.ToSlash(filepath.Dir(ep.RelPath))
		packageSeen[pkg]++
		if packageSeen[pkg] > 8 {
			continue
		}
		add(ep.RelPath, ep.Hint, worklistCategoryAPIHandler, 3)
	}

	sortWorklistSeedByPriority(seed)
	return seed
}

func sortWorklistSeedByPriority(seed []WorklistSeedItem) {
	if len(seed) < 2 {
		return
	}
	for i := 0; i < len(seed)-1; i++ {
		for j := i + 1; j < len(seed); j++ {
			pi, pj := seed[i].Priority, seed[j].Priority
			if pi == 0 {
				pi = 99
			}
			if pj == 0 {
				pj = 99
			}
			if pj < pi || (pj == pi && seed[j].RelPath < seed[i].RelPath) {
				seed[i], seed[j] = seed[j], seed[i]
			}
		}
	}
}

func loadProjectProfile(workDir string) (*ProjectProfileV1, error) {
	b, err := os.ReadFile(store.ProjectProfilePath(workDir))
	if err != nil {
		return nil, err
	}
	var p ProjectProfileV1
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func loadProfileOrNilFromWorkDir(workDir string) *ProjectProfileV1 {
	p, _ := loadProjectProfile(workDir)
	return p
}

func detectedFrameworkIDs(p *ProjectProfileV1) []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.Frameworks))
	for _, f := range p.Frameworks {
		if id := strings.TrimSpace(f.ID); id != "" {
			out = append(out, id)
		}
	}
	return out
}
