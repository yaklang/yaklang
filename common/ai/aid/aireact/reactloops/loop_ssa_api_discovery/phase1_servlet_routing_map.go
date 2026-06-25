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

const servletRoutingMapSchemaVersion = 1

// ServletDispatcherEntry is one DispatcherServlet (or equivalent) mount discovered from code.
type ServletDispatcherEntry struct {
	ID              string   `json:"id"`
	ServletPattern  string   `json:"servlet_pattern"`
	URLPrefix       string   `json:"url_prefix"`
	PackagePatterns []string `json:"package_patterns,omitempty"`
	ComponentScan   string   `json:"component_scan,omitempty"`
	ConfigClass     string   `json:"config_class,omitempty"`
	SourceRef       string   `json:"source_ref,omitempty"`
	Confidence      float64  `json:"confidence"`
}

// ServletRoutingMapV1 maps Java package/controller layers to servlet-level URL prefixes.
type ServletRoutingMapV1 struct {
	SchemaVersion int                      `json:"schema_version"`
	GeneratedAt   string                   `json:"generated_at"`
	ContextPath   string                   `json:"context_path"`
	Dispatchers   []ServletDispatcherEntry `json:"dispatchers"`
}

var (
	reServletMappingsReturn = regexp.MustCompile(`(?is)getServletMappings\s*\(\s*\)\s*\{[^}]*return\s+new\s+String\s*\[\s*\]\s*\{([^}]+)\}`)
	rePathConstantAssign    = regexp.MustCompile(`(?m)(?:public\s+)?static\s+final\s+String\s+(\w*(?:CONTEXT|ADMIN|API|WEB)\w*(?:PATH|PREFIX|URI)?)\s*=\s*"([^"]+)"`)
	reComponentScanPkg      = regexp.MustCompile(`(?i)@ComponentScan\s*\([^)]*basePackages\s*=\s*"([^"]+)"`)
	reServletConfigClasses  = regexp.MustCompile(`(?is)getServletConfigClasses\s*\(\s*\)\s*\{[^}]*\{([^}]+)\}`)
	reConfigClassRef        = regexp.MustCompile(`(\w+Config)\.class`)
	reContextPathProperty   = regexp.MustCompile(`(?i)(?:cms\.contextPath|server\.servlet\.context-path)\s*[=:]\s*"?([^"\s]+)"?`)
)

// BuildServletRoutingMap scans Initializer/Config sources for servlet-level URL prefixes.
func BuildServletRoutingMap(rt *Runtime) (*ServletRoutingMapV1, error) {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return nil, utils.Error("code path unavailable")
	}
	root := strings.TrimSpace(rt.Session.CodeRootPath)
	if root == "" {
		return nil, utils.Error("empty code root")
	}
	out := &ServletRoutingMapV1{
		SchemaVersion: servletRoutingMapSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		ContextPath:   detectContextPath(root),
	}
	seen := map[string]struct{}{}

	addDispatcher := func(d ServletDispatcherEntry) {
		d.URLPrefix = servletPatternToURLPrefix(d.ServletPattern)
		if d.URLPrefix == "" && d.ServletPattern != "" {
			d.URLPrefix = normURLPath(strings.TrimSuffix(d.ServletPattern, "/*"))
		}
		if d.URLPrefix == "" {
			return
		}
		key := d.URLPrefix + "|" + strings.Join(d.PackagePatterns, ",")
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		if d.ID == "" {
			d.ID = strings.TrimPrefix(d.URLPrefix, "/")
			if d.ID == "" {
				d.ID = "root"
			}
		}
		if d.Confidence == 0 {
			d.Confidence = 0.9
		}
		out.Dispatchers = append(out.Dispatchers, d)
	}

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		lower := strings.ToLower(path)
		if !strings.HasSuffix(lower, ".java") && !strings.HasSuffix(lower, ".properties") && !strings.HasSuffix(lower, ".yml") && !strings.HasSuffix(lower, ".yaml") {
			return nil
		}
		if skipDirForHarvest(filepath.Base(path)) {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		content := string(data)
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		if strings.HasSuffix(lower, ".java") && strings.Contains(lower, "initializer") {
			for _, pat := range extractServletMappingPatterns(content, root) {
				d := ServletDispatcherEntry{
					ServletPattern: pat,
					SourceRef:      rel,
				}
				if cfg := extractServletConfigClass(content); cfg != "" {
					d.ConfigClass = cfg
					if cfgPath := findConfigFile(root, cfg); cfgPath != "" {
						if cfgData, err := os.ReadFile(cfgPath); err == nil {
							cfgContent := string(cfgData)
							if pkg := reComponentScanPkg.FindStringSubmatch(cfgContent); len(pkg) > 1 {
								d.ComponentScan = strings.TrimSpace(pkg[1])
								d.PackagePatterns = componentScanToPackagePatterns(pkg[1])
							}
							if strings.Contains(content, "joinString") {
								reJoin := regexp.MustCompile(`(?i)joinString\s*\(\s*(?:[\w.]+\.)?(\w+)\s*,\s*"([^"]+)"\s*\)`)
								if jm := reJoin.FindStringSubmatch(content); len(jm) > 2 {
									if resolved := resolveConstantPlusSuffix(cfgContent, jm[1], jm[2]); resolved != "" {
										d.ServletPattern = resolved
									}
								}
							}
						}
					}
				}
				addDispatcher(d)
			}
		}

		if strings.Contains(lower, "config") && strings.HasSuffix(lower, ".java") {
			for _, m := range rePathConstantAssign.FindAllStringSubmatch(content, -1) {
				if len(m) < 3 {
					continue
				}
				prefix := normURLPath(m[2])
				if prefix == "" || prefix == "/" {
					continue
				}
				addDispatcher(ServletDispatcherEntry{
					ID:             strings.ToLower(m[1]),
					ServletPattern: prefix + "/*",
					URLPrefix:      prefix,
					SourceRef:      rel,
					Confidence:     0.85,
				})
			}
		}

		if strings.HasSuffix(lower, ".properties") || strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml") {
			if cp := extractContextPathFromContent(content); cp != "" {
				out.ContextPath = normURLPath(cp)
			}
		}
		return nil
	})

	// Package heuristics fallback when no Initializer found (single-dispatcher apps).
	if len(out.Dispatchers) == 0 {
		addDispatcher(ServletDispatcherEntry{
			ID:              "admin",
			ServletPattern:  "/admin/*",
			URLPrefix:       "/admin",
			PackagePatterns: []string{"*.controller.admin.*"},
			SourceRef:       "heuristic",
			Confidence:      0.5,
		})
		addDispatcher(ServletDispatcherEntry{
			ID:              "api",
			ServletPattern:  "/api/*",
			URLPrefix:       "/api",
			PackagePatterns: []string{"*.controller.api.*"},
			SourceRef:       "heuristic",
			Confidence:      0.5,
		})
		addDispatcher(ServletDispatcherEntry{
			ID:              "web",
			ServletPattern:  "/*",
			URLPrefix:       "/",
			PackagePatterns: []string{"*.controller.web.*"},
			SourceRef:       "heuristic",
			Confidence:      0.5,
		})
	}
	return out, nil
}

func extractServletMappingPatterns(content, codeRoot string) []string {
	var out []string
	inner := ""
	if m := reServletMappingsReturn.FindStringSubmatch(content); len(m) > 1 {
		inner = m[1]
	}
	if inner == "" {
		if m := regexp.MustCompile(`(?is)getServletMappings\s*\(\s*\)\s*\{([^}]+)\}`).FindStringSubmatch(content); len(m) > 1 {
			inner = m[1]
		}
	}
	if inner != "" {
		for _, lit := range extractQuotedStrings(inner) {
			if resolved := resolveServletMappingLiteral(content, lit); resolved != "" {
				out = append(out, resolved)
			}
		}
		reJoin := regexp.MustCompile(`(?i)joinString\s*\(\s*(?:[\w.]+\.)?(\w+)\s*,\s*"([^"]+)"\s*\)`)
		if jm := reJoin.FindStringSubmatch(inner); len(jm) > 2 {
			if resolved := resolveConstantPlusSuffix(content, jm[1], jm[2]); resolved != "" {
				out = append(out, resolved)
			}
			if cfg := extractServletConfigClass(content); cfg != "" && codeRoot != "" {
				if cfgPath := findConfigFile(codeRoot, cfg); cfgPath != "" {
					if cfgData, err := os.ReadFile(cfgPath); err == nil {
						if resolved := resolveConstantPlusSuffix(string(cfgData), jm[1], jm[2]); resolved != "" {
							out = append(out, resolved)
						}
					}
				}
			}
		}
	}
	return uniqueStrings(out)
}

func resolveConstantPlusSuffix(content, constName, suffix string) string {
	for _, cm := range rePathConstantAssign.FindAllStringSubmatch(content, -1) {
		if len(cm) > 2 && strings.EqualFold(cm[1], constName) {
			return normURLPath(cm[2]) + suffix
		}
	}
	return ""
}

func resolveServletMappingLiteral(fullContent, lit string) string {
	lit = strings.TrimSpace(lit)
	if strings.HasPrefix(lit, "/") {
		return lit
	}
	// joinString(AdminConfig.ADMIN_CONTEXT_PATH, "/*")
	reJoin := regexp.MustCompile(`(?i)joinString\s*\(\s*(?:[\w.]+\.)?(\w+)\s*,\s*"([^"]+)"\s*\)`)
	if m := reJoin.FindStringSubmatch(lit); len(m) > 2 {
		constName := m[1]
		suffix := m[2]
		if cm := rePathConstantAssign.FindStringSubmatch(fullContent); len(cm) > 2 {
			_ = cm
		}
		for _, cm := range rePathConstantAssign.FindAllStringSubmatch(fullContent, -1) {
			if len(cm) > 2 && strings.EqualFold(cm[1], constName) {
				return normURLPath(cm[2]) + suffix
			}
		}
	}
	return lit
}

func extractServletConfigClass(content string) string {
	if m := reServletConfigClasses.FindStringSubmatch(content); len(m) > 1 {
		if c := reConfigClassRef.FindStringSubmatch(m[1]); len(c) > 1 {
			return c[1]
		}
	}
	return ""
}

func findConfigFile(root, simpleClass string) string {
	var found string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), strings.ToLower(simpleClass)+".java") {
			return nil
		}
		found = path
		return filepath.SkipAll
	})
	return found
}

func componentScanToPackagePatterns(scan string) []string {
	scan = strings.TrimSpace(scan)
	if scan == "" {
		return nil
	}
	if strings.Contains(scan, "*") {
		return []string{scan + ".*"}
	}
	return []string{scan + ".*"}
}

func servletPatternToURLPrefix(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	if pattern == "/*" || pattern == "/" {
		return "/"
	}
	pattern = strings.TrimSuffix(pattern, "/*")
	pattern = strings.TrimSuffix(pattern, "*")
	return normURLPath(pattern)
}

func detectContextPath(codeRoot string) string {
	var found string
	_ = filepath.Walk(codeRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		lower := strings.ToLower(path)
		if !strings.HasSuffix(lower, ".properties") && !strings.HasSuffix(lower, ".yml") && !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".java") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if cp := extractContextPathFromContent(string(data)); cp != "" {
			found = cp
			return filepath.SkipAll
		}
		return nil
	})
	if found == "" {
		return "/"
	}
	return normURLPath(found)
}

func extractContextPathFromContent(content string) string {
	if m := reContextPathProperty.FindStringSubmatch(content); len(m) > 1 {
		return normURLPath(m[1])
	}
	if strings.Contains(content, "setContextPath") {
		re := regexp.MustCompile(`setContextPath\s*\(\s*System\.getProperty\s*\(\s*"cms\.contextPath"\s*,\s*"([^"]*)"\s*\)`)
		if m := re.FindStringSubmatch(content); len(m) > 1 {
			cp := strings.TrimSpace(m[1])
			if cp == "" {
				return "/"
			}
			return normURLPath(cp)
		}
	}
	return ""
}

func extractQuotedStrings(s string) []string {
	re := regexp.MustCompile(`"([^"]+)"`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 {
			out = append(out, m[1])
		}
	}
	return out
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func loadServletRoutingMap(workDir string) (*ServletRoutingMapV1, error) {
	path := store.ServletRoutingMapPath(workDir)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m ServletRoutingMapV1
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func persistServletRoutingMap(rt *Runtime, m *ServletRoutingMapV1) error {
	if rt == nil || m == nil {
		return utils.Error("nil persist target")
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = servletRoutingMapSchemaVersion
	}
	if m.GeneratedAt == "" {
		m.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := writeJSONFile(store.ServletRoutingMapPath(rt.WorkDir), b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactServletRoutingMap, string(b))
	}
	return nil
}

func RunBuildServletRoutingMap(rt *Runtime) (*ServletRoutingMapV1, error) {
	m, err := BuildServletRoutingMap(rt)
	if err != nil {
		return nil, err
	}
	if err := persistServletRoutingMap(rt, m); err != nil {
		return m, err
	}
	log.Infof("ssa_api_discovery: servlet_routing_map dispatchers=%d context_path=%s", len(m.Dispatchers), m.ContextPath)
	return m, nil
}

// resolveURLPrefixFromServletMap returns servlet-level prefix for a controller entry file.
func resolveURLPrefixFromServletMap(m *ServletRoutingMapV1, entryFile, handlerClass string, packagePatterns []string) string {
	if m == nil {
		return ""
	}
	entryFile = strings.ToLower(strings.ReplaceAll(entryFile, "\\", "/"))
	handlerClass = strings.ToLower(handlerClass)
	for _, d := range m.Dispatchers {
		for _, pat := range d.PackagePatterns {
			if packagePatternMatchesEntry(pat, entryFile, handlerClass, packagePatterns) {
				return normURLPath(d.URLPrefix)
			}
		}
		if d.ComponentScan != "" {
			core := strings.ToLower(strings.TrimSuffix(d.ComponentScan, ".*"))
			if core != "" && (strings.Contains(entryFile, strings.ReplaceAll(core, ".", "/")) || strings.Contains(handlerClass, core)) {
				return normURLPath(d.URLPrefix)
			}
		}
	}
	return ""
}

func packagePatternMatchesEntry(servletPat, entryFile, handlerClass string, jobPatterns []string) bool {
	servletPat = strings.ToLower(strings.TrimSpace(servletPat))
	for _, jp := range jobPatterns {
		if authPackagePatternMatches(jp, []string{servletPat}) {
			return true
		}
	}
	core := strings.TrimPrefix(strings.TrimSuffix(servletPat, ".*"), "*.")
	core = strings.TrimPrefix(core, "*")
	if core == "" {
		return false
	}
	return strings.Contains(entryFile, strings.ReplaceAll(core, ".", "/")) || strings.Contains(handlerClass, core)
}

func allowedServletMountPrefixes(m *ServletRoutingMapV1) map[string]struct{} {
	out := map[string]struct{}{"/": {}}
	if m == nil {
		out["/admin"] = struct{}{}
		out["/api"] = struct{}{}
		return out
	}
	for _, d := range m.Dispatchers {
		mp := normURLPath(d.URLPrefix)
		if mp != "" {
			out[mp] = struct{}{}
		}
	}
	return out
}

// SanitizeRoutingProfileURLSpaces removes method-level false mounts from routing_profile.
func SanitizeRoutingProfileURLSpaces(rp *RoutingProfileV1, servletMap *ServletRoutingMapV1) {
	if rp == nil {
		return
	}
	allowed := allowedServletMountPrefixes(servletMap)
	var kept []RoutingURLSpace
	for _, sp := range rp.URLSpaces {
		mp := normURLPath(sp.MountPrefix)
		if _, ok := allowed[mp]; ok {
			kept = append(kept, sp)
		}
	}
	if len(kept) == 0 {
		for mp := range allowed {
			id := strings.TrimPrefix(mp, "/")
			if id == "" {
				id = "root"
			}
			kept = append(kept, RoutingURLSpace{ID: id, MountPrefix: mp, Confidence: 0.7})
		}
	}
	rp.URLSpaces = kept
}

func isLikelyControllerMappingMount(mp string) bool {
	mp = normURLPath(mp)
	if mp == "" || mp == "/" || mp == "/admin" || mp == "/api" {
		return false
	}
	seg := strings.TrimPrefix(mp, "/")
	if seg == "" || strings.Contains(seg, "/") {
		return false
	}
	// camelCase single segment: cmsCategory, cmsDictionary, simpleAi
	if len(seg) >= 2 && seg[0] >= 'a' && seg[0] <= 'z' {
		for i := 1; i < len(seg); i++ {
			if seg[i] >= 'A' && seg[i] <= 'Z' {
				return true
			}
		}
	}
	return false
}

// SupplementAuthSurfaceFromServletMap adds missing admin/api/web surfaces from servlet map.
func SupplementAuthSurfaceFromServletMap(rt *Runtime) error {
	if rt == nil {
		return utils.Error("nil runtime")
	}
	m, err := loadServletRoutingMap(rt.WorkDir)
	if err != nil || m == nil {
		return err
	}
	surface, err := loadAuthSurfaceMap(rt.WorkDir)
	if err != nil || surface == nil {
		surface = &AuthSurfaceMapV1{SchemaVersion: artifactV2SchemaVersion, MultiAuth: len(m.Dispatchers) > 1}
	}
	mergeServletMapIntoAuthSurface(m, surface)
	if surface.GeneratedAt == "" {
		surface.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return persistAuthSurfaceMap(rt, surface)
}

func mergeServletMapIntoAuthSurface(m *ServletRoutingMapV1, surface *AuthSurfaceMapV1) {
	if m == nil || surface == nil {
		return
	}
	realmForPrefix := func(prefix string) string {
		switch normURLPath(prefix) {
		case "/admin":
			return AuthRealmAdmin
		case "/api":
			return AuthRealmAPI
		case "/":
			return AuthRealmWeb
		default:
			return ""
		}
	}
	existing := map[string]struct{}{}
	for _, s := range surface.Surfaces {
		existing[NormalizeAuthRealm(s.AuthRealm)] = struct{}{}
	}
	for _, d := range m.Dispatchers {
		realm := realmForPrefix(d.URLPrefix)
		if realm == "" {
			continue
		}
		if _, ok := existing[realm]; ok {
			continue
		}
		prefix := normURLPath(d.URLPrefix)
		surface.Surfaces = append(surface.Surfaces, AuthSurfaceEntry{
			AuthRealm:        realm,
			MountPrefix:      prefix,
			PackagePatterns:  d.PackagePatterns,
			PathPrefixes:     []string{prefix},
			SessionMechanism: "JSESSIONID",
			CodeEvidence:     []string{d.SourceRef},
		})
		existing[realm] = struct{}{}
	}
	surface.MultiAuth = len(surface.Surfaces) > 1
}

func buildServletRoutingMapBlock(rt *Runtime) string {
	m, err := loadServletRoutingMap(rt.WorkDir)
	if err != nil || m == nil || len(m.Dispatchers) == 0 {
		return ""
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return "## servlet_routing_map (engine — authoritative servlet prefixes)\n" +
		"Use this **before** routing_profile.url_spaces. Full URL = context_path + url_prefix + class @RequestMapping + method mapping.\n" +
		"404 on bare path → prepend url_prefix from matching dispatcher.\n" +
		"404 on @Csrf endpoints without _csrf → route exists; add _csrf query/body and retry.\n" +
		"```json\n" + string(b) + "\n```"
}

// ResolvedFeatureRoute is a programmatically resolved route for one controller job.
type ResolvedFeatureRoute struct {
	Method       string `json:"method"`
	PathPattern  string `json:"path_pattern"`
	URLPrefix    string `json:"url_prefix"`
	NeedsCSRF    bool   `json:"needs_csrf,omitempty"`
	HandlerClass string `json:"handler_class,omitempty"`
	Source       string `json:"source,omitempty"`
}

func resolveFeatureRoutesProgrammatic(rt *Runtime, job FeatureWorkJob) []ResolvedFeatureRoute {
	var out []ResolvedFeatureRoute
	hints := job.StaticHints
	if len(hints) == 0 {
		hints = enrichStaticRouteHintsForJob(rt, job, staticRouteHintsByFile(rt)[normalizePlanFileRef(rt, job.EntryFile)])
	}
	seen := map[string]struct{}{}
	for _, h := range hints {
		if isWildcardRoutePattern(h.PathPattern) {
			continue
		}
		key := routeKey(h.Method, h.PathPattern)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ResolvedFeatureRoute{
			Method:       h.Method,
			PathPattern:  h.PathPattern,
			HandlerClass: h.HandlerClass,
			Source:       h.Source,
		})
	}
	return out
}

func buildResolvedRoutesBlock(rt *Runtime, job FeatureWorkJob) string {
	routes := resolveFeatureRoutesProgrammatic(rt, job)
	if len(routes) == 0 {
		return ""
	}
	prefix := resolvePathPrefixForHandler(rt, "", job.EntryFile, job.PackagePatterns)
	realm := InferAuthRealmForFeatureJob(rt, job)
	b, _ := json.MarshalIndent(routes, "", "  ")
	return "## resolved_routes (engine — do not change url_prefix)\n" +
		"- auth_realm=" + realm + "\n" +
		"- mount_prefix=" + prefix + "\n" +
		"- Verify each route in S4; on 403 try POST with _csrf before reject.\n" +
		"```json\n" + string(b) + "\n```"
}
