package loop_ssa_api_discovery

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	reFrontendBasePath = regexp.MustCompile(`(?i)(?:basePath|baseUrl|adminPath|contextPath|API_BASE)\s*[=:]\s*['"]([^'"]+)['"]`)
	reJQueryAjaxURL    = regexp.MustCompile(`(?i)(?:\$\.(?:ajax|post|get)|jQuery\.(?:ajax|post|get))\s*\(\s*\{[^}]{0,800}?(?:url|href)\s*:\s*['"]([^'"]+)['"]`)
	reJQuerySimpleURL  = regexp.MustCompile(`(?i)\$\.(?:post|get)\s*\(\s*['"]([^'"]+)['"]`)
	reAxiosCall        = regexp.MustCompile(`(?i)axios\.(get|post|put|delete|patch)\s*\(\s*['"]([^'"]+)['"]`)
	reFetchURL         = regexp.MustCompile(`(?i)fetch\s*\(\s*['"]([^'"]+)['"]`)
	reFormAction       = regexp.MustCompile(`(?i)<form[^>]{0,400}?action\s*=\s*['"]([^'"]+)['"]`)
	reFormMethod       = regexp.MustCompile(`(?i)<form[^>]{0,400}?method\s*=\s*['"]?(get|post)['"]?`)
	reFTLUrlCall       = regexp.MustCompile(`(?i)url\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	reInputName        = regexp.MustCompile(`(?i)<input[^>]+name\s*=\s*['"]([^'"]+)['"]`)
	reDataFieldName    = regexp.MustCompile(`(?i)(?:data|params)\s*:\s*\{[^}]{0,600}?['"]?([A-Za-z_][\w]*)['"]?\s*:`)
)

var frontendScanExtensions = map[string]struct{}{
	".js": {}, ".mjs": {}, ".cjs": {}, ".ts": {}, ".tsx": {}, ".jsx": {}, ".vue": {},
	".ftl": {}, ".jsp": {}, ".html": {}, ".htm": {},
}

var frontendSkipDirTokens = []string{
	"node_modules", "vendor", "plugins", "pdfjs", "dist", "build", "target",
	".git", ".idea", ".gradle", "coverage", "test", "tests", "__tests__",
}

func shouldRunFrontendAPIAnalysis(rt *Runtime) (bool, string) {
	if strings.TrimSpace(os.Getenv("YAK_SSA_SKIP_FRONTEND_API")) == "1" {
		return false, "YAK_SSA_SKIP_FRONTEND_API=1"
	}
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return false, "no code path"
	}
	if scope, err := loadBackendScope(rt.WorkDir); err == nil && scope != nil && len(scope.FrontendRoots) > 0 {
		return true, "frontend_roots"
	}
	if roots := discoverFrontendScanRoots(rt); len(roots) > 0 {
		return true, "discovered_frontend_roots"
	}
	return false, "no frontend signals"
}

func discoverFrontendScanRoots(rt *Runtime) []string {
	if rt == nil || rt.Session == nil {
		return nil
	}
	codeRoot := rt.Session.CodeRootPath
	var roots []string
	seen := map[string]struct{}{}
	add := func(p string) {
		p = filepath.ToSlash(strings.TrimSpace(p))
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		roots = append(roots, p)
	}
	if scope, err := loadBackendScope(rt.WorkDir); err == nil && scope != nil {
		for _, r := range scope.FrontendRoots {
			add(r)
		}
	}
	_ = filepath.Walk(codeRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			name := strings.ToLower(info.Name())
			for _, tok := range frontendSkipDirTokens {
				if name == tok {
					return filepath.SkipDir
				}
			}
			rel, _ := filepath.Rel(codeRoot, path)
			rel = strings.ToLower(filepath.ToSlash(rel))
			switch {
			case strings.Contains(rel, "src/main/resources/templates"),
				strings.Contains(rel, "src/main/webapp"),
				strings.Contains(rel, "/webapp/"):
				add(filepath.ToSlash(rel))
			}
			if info.Name() == "package.json" {
				if dirRel, rerr := filepath.Rel(codeRoot, path); rerr == nil {
					add(filepath.ToSlash(filepath.Dir(dirRel)))
				}
			}
		}
		return nil
	})
	return roots
}

func resolveFrontendScanRoots(rt *Runtime) []string {
	if rt == nil || rt.Session == nil {
		return nil
	}
	codeRoot := rt.Session.CodeRootPath
	roots := discoverFrontendScanRoots(rt)
	if len(roots) == 0 {
		return []string{"."}
	}
	out := make([]string, 0, len(roots))
	seen := map[string]struct{}{}
	for _, r := range roots {
		abs := r
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(codeRoot, r)
		}
		rel, err := filepath.Rel(codeRoot, abs)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

func skipFrontendScanPath(rel string) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	if strings.Contains(rel, ".min.") {
		return true
	}
	for _, tok := range frontendSkipDirTokens {
		if strings.Contains(rel, "/"+tok+"/") || strings.HasPrefix(rel, tok+"/") {
			return true
		}
	}
	return false
}

func RunFrontendAPIHarvest(rt *Runtime) (*FrontendAPIHarvestReport, error) {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return nil, utils.Error("invalid runtime")
	}
	codeRoot := rt.Session.CodeRootPath
	roots := resolveFrontendScanRoots(rt)
	rep := &FrontendAPIHarvestReport{
		SchemaVersion: frontendAPISchemaVersion,
		FrontendRoots: roots,
		Calls:         []FrontendAPICall{},
	}
	fileHits := map[string]int{}
	callSeq := 0
	filesScanned := 0

	scanRoot := func(scanRel string) {
		abs := filepath.Join(codeRoot, scanRel)
		_ = filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				if info != nil && info.IsDir() && skipDirForHarvest(info.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(codeRoot, path)
			rel = filepath.ToSlash(rel)
			if skipFrontendScanPath(rel) {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(rel))
			if _, ok := frontendScanExtensions[ext]; !ok {
				return nil
			}
			data, rerr := os.ReadFile(path)
			if rerr != nil || len(data) == 0 {
				return nil
			}
			filesScanned++
			calls := harvestFrontendCallsFromSource(rt, data, rel)
			for _, c := range calls {
				callSeq++
				c.CallID = fmt.Sprintf("fe_%04d", callSeq)
				rep.Calls = append(rep.Calls, c)
				fileHits[rel]++
			}
			for _, bp := range reFrontendBasePath.FindAllStringSubmatch(string(data), -1) {
				if len(bp) > 1 {
					rep.BaseURLPatterns = appendUniqueString(rep.BaseURLPatterns, normURLPath(bp[1]))
				}
			}
			return nil
		})
	}
	for _, root := range roots {
		scanRoot(root)
	}
	if len(rep.Calls) == 0 && !(len(roots) == 1 && roots[0] == ".") {
		scanRoot(".")
	}

	rep.Stats.FilesScanned = filesScanned
	rep.Stats.Calls = len(rep.Calls)
	rep.Candidates = buildFrontendHarvestCandidates(fileHits)
	if err := persistFrontendAPIHarvest(rt, rep); err != nil {
		return rep, err
	}
	if err := mergeFrontendHarvestIntoStaticHints(rt.WorkDir, rep.Calls); err != nil {
		rep.Warnings = append(rep.Warnings, "merge static_route_hints: "+err.Error())
	}
	log.Infof("ssa_api_discovery: frontend_api_harvest calls=%d files=%d roots=%v", rep.Stats.Calls, rep.Stats.FilesScanned, roots)
	return rep, nil
}

func harvestFrontendCallsFromSource(rt *Runtime, data []byte, fileRel string) []FrontendAPICall {
	var out []FrontendAPICall
	s := string(data)

	addCall := func(method, pathRaw, clientLib string, lineHint int, params []FrontendAPIParam) {
		pathRaw = cleanFrontendPathRaw(pathRaw)
		if pathRaw == "" || isNoiseFrontendPath(pathRaw) {
			return
		}
		resolved, conf := resolveFrontendAPIPath(rt, pathRaw, fileRel)
		method = normalizeHTTPMethod(method)
		out = append(out, FrontendAPICall{
			Method:            method,
			PathRaw:           pathRaw,
			PathResolved:      resolved,
			SourceFile:        fileRel,
			LineHint:          lineHint,
			ClientLib:         clientLib,
			AuthRealmHint:     inferAuthRealmFromFrontendPath(resolved, fileRel),
			Params:            params,
			LinkedHandlerHint: linkedHandlerHintFromPath(resolved),
			Confidence:        conf,
		})
	}

	for _, m := range reJQueryAjaxURL.FindAllStringSubmatch(s, -1) {
		if len(m) < 2 {
			continue
		}
		addCall(inferMethodFromJQueryBlock(m[0]), m[1], "jquery.ajax", lineNumberOf(data, m[0]), extractParamsNearBlock(s, m[0]))
	}
	for _, m := range reJQuerySimpleURL.FindAllStringSubmatch(s, -1) {
		if len(m) < 2 {
			continue
		}
		method := "GET"
		if strings.Contains(strings.ToLower(m[0]), ".post") {
			method = "POST"
		}
		addCall(method, m[1], "jquery", lineNumberOf(data, m[0]), nil)
	}
	for _, m := range reAxiosCall.FindAllStringSubmatch(s, -1) {
		if len(m) < 3 {
			continue
		}
		addCall(m[1], m[2], "axios", lineNumberOf(data, m[0]), extractParamsNearBlock(s, m[0]))
	}
	for _, m := range reFetchURL.FindAllStringSubmatch(s, -1) {
		if len(m) < 2 {
			continue
		}
		addCall("GET", m[1], "fetch", lineNumberOf(data, m[0]), nil)
	}
	for _, m := range reFTLUrlCall.FindAllStringSubmatch(s, -1) {
		if len(m) < 2 {
			continue
		}
		addCall("POST", m[1], "ftl.url", lineNumberOf(data, m[0]), extractFormInputNamesNear(s, m[0]))
	}
	for _, m := range reFormAction.FindAllStringSubmatch(s, -1) {
		if len(m) < 2 {
			continue
		}
		method := "POST"
		if fm := reFormMethod.FindStringSubmatch(m[0]); len(fm) > 1 {
			method = strings.ToUpper(fm[1])
		}
		addCall(method, m[1], "html.form", lineNumberOf(data, m[0]), extractFormInputNamesNear(s, m[0]))
	}
	return dedupeFrontendCalls(out)
}

func cleanFrontendPathRaw(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'`)
	if raw == "" || strings.HasPrefix(raw, "javascript:") || strings.HasPrefix(raw, "mailto:") {
		return ""
	}
	if strings.Contains(raw, "${") || strings.Contains(raw, "{{") {
		return ""
	}
	return raw
}

func isNoiseFrontendPath(p string) bool {
	p = strings.ToLower(p)
	if strings.HasPrefix(p, "#") {
		return true
	}
	for _, ext := range []string{".css", ".png", ".jpg", ".svg", ".woff", ".ico", ".map"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func resolveFrontendAPIPath(rt *Runtime, pathRaw, sourceFile string) (resolved, confidence string) {
	pathRaw = strings.TrimSpace(pathRaw)
	if pathRaw == "" {
		return "", "low"
	}
	if strings.HasPrefix(pathRaw, "http://") || strings.HasPrefix(pathRaw, "https://") {
		u := pathRaw
		if i := strings.Index(u[8:], "/"); i >= 0 {
			off := 8
			if strings.HasPrefix(u, "https://") {
				off = 9
			}
			return normURLPath(u[off+i:]), "high"
		}
		return "", "low"
	}
	if strings.HasPrefix(pathRaw, "/") {
		return normURLPath(pathRaw), "high"
	}
	prefix := inferServletPrefixFromFrontendFile(rt, sourceFile)
	if prefix == "" {
		return normURLPath(pathRaw), "low"
	}
	return joinMountAndRelativePath(prefix, pathRaw), "medium"
}

func inferServletPrefixFromFrontendFile(rt *Runtime, sourceFile string) string {
	rel := strings.ToLower(filepath.ToSlash(sourceFile))
	switch {
	case strings.Contains(rel, "/admin/"), strings.Contains(rel, "templates/admin"), strings.Contains(rel, "/controller/admin/"):
		return "/admin"
	case strings.Contains(rel, "/api/"), strings.Contains(rel, "templates/api"):
		return "/api"
	}
	if rt != nil {
		if m, err := loadServletRoutingMap(rt.WorkDir); err == nil && m != nil {
			for _, d := range m.Dispatchers {
				mp := normURLPath(d.URLPrefix)
				if mp == "" || mp == "/" {
					continue
				}
				core := strings.Trim(strings.ToLower(strings.TrimSuffix(d.ComponentScan, ".*")), ".")
				if core != "" && strings.Contains(rel, strings.ReplaceAll(core, ".", "/")) {
					return mp
				}
				if strings.Contains(rel, strings.TrimPrefix(mp, "/")) {
					return mp
				}
			}
			for _, d := range m.Dispatchers {
				if normURLPath(d.URLPrefix) == "/admin" && strings.Contains(rel, "admin") {
					return "/admin"
				}
			}
		}
	}
	return ""
}

func inferAuthRealmFromFrontendPath(resolvedPath, sourceFile string) string {
	p := strings.ToLower(resolvedPath + " " + sourceFile)
	switch {
	case strings.Contains(p, "/admin"):
		return AuthRealmAdmin
	case strings.Contains(p, "/api"):
		return "api"
	default:
		return AuthRealmWeb
	}
}

func linkedHandlerHintFromPath(resolvedPath string) string {
	resolvedPath = strings.Trim(normURLPath(resolvedPath), "/")
	if resolvedPath == "" {
		return ""
	}
	parts := strings.Split(resolvedPath, "/")
	seg := parts[len(parts)-1]
	if seg == "" && len(parts) > 1 {
		seg = parts[len(parts)-2]
	}
	if seg == "" {
		return ""
	}
	runes := []rune(seg)
	return strings.ToUpper(string(runes[0])) + string(runes[1:]) + "AdminController"
}

func inferMethodFromJQueryBlock(block string) string {
	b := strings.ToLower(block)
	switch {
	case strings.Contains(b, "type:") && strings.Contains(b, "post"):
		return "POST"
	case strings.Contains(b, "type:") && strings.Contains(b, "put"):
		return "PUT"
	case strings.Contains(b, "type:") && strings.Contains(b, "delete"):
		return "DELETE"
	case strings.Contains(b, ".post"):
		return "POST"
	default:
		return "GET"
	}
}

func extractParamsNearBlock(src, block string) []FrontendAPIParam {
	idx := strings.Index(src, block)
	if idx < 0 {
		return nil
	}
	end := idx + len(block) + 400
	if end > len(src) {
		end = len(src)
	}
	window := src[idx:end]
	var params []FrontendAPIParam
	seen := map[string]struct{}{}
	for _, m := range reDataFieldName.FindAllStringSubmatch(window, -1) {
		if len(m) < 2 {
			continue
		}
		name := strings.TrimSpace(m[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		params = append(params, FrontendAPIParam{Name: name, Location: "post", Required: name == "_csrf"})
	}
	return params
}

func extractFormInputNamesNear(src, anchor string) []FrontendAPIParam {
	idx := strings.Index(src, anchor)
	if idx < 0 {
		return nil
	}
	start := idx - 1200
	if start < 0 {
		start = 0
	}
	end := idx + 1200
	if end > len(src) {
		end = len(src)
	}
	window := src[start:end]
	var params []FrontendAPIParam
	seen := map[string]struct{}{}
	for _, m := range reInputName.FindAllStringSubmatch(window, -1) {
		if len(m) < 2 {
			continue
		}
		name := strings.TrimSpace(m[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		params = append(params, FrontendAPIParam{Name: name, Location: "post", Required: name == "_csrf"})
	}
	return params
}

func dedupeFrontendCalls(in []FrontendAPICall) []FrontendAPICall {
	seen := map[string]struct{}{}
	out := make([]FrontendAPICall, 0, len(in))
	for _, c := range in {
		key := routeKey(c.Method, firstNonEmpty(c.PathResolved, c.PathRaw)) + "|" + c.SourceFile + "|" + c.ClientLib
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, c)
	}
	return out
}

func buildFrontendHarvestCandidates(fileHits map[string]int) []FrontendAPIHarvestCandidate {
	var cands []FrontendAPIHarvestCandidate
	for rel, n := range fileHits {
		priority := n
		l := strings.ToLower(rel)
		if strings.Contains(l, "api.js") || strings.Contains(l, "request.") || strings.Contains(l, "/common.") {
			priority += 5
		}
		cands = append(cands, FrontendAPIHarvestCandidate{FileRelPath: rel, HitCount: n, Priority: priority})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].Priority == cands[j].Priority {
			return cands[i].HitCount > cands[j].HitCount
		}
		return cands[i].Priority > cands[j].Priority
	})
	if len(cands) > 40 {
		cands = cands[:40]
	}
	return cands
}

func lineNumberOf(data []byte, substr string) int {
	idx := bytes.Index(data, []byte(substr))
	if idx < 0 {
		return 0
	}
	return bytes.Count(data[:idx], []byte("\n")) + 1
}

func appendUniqueString(in []string, v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return in
	}
	for _, x := range in {
		if x == v {
			return in
		}
	}
	return append(in, v)
}


func normalizeHTTPMethod(m string) string {
	m = strings.ToUpper(strings.TrimSpace(m))
	if utils.IsCommonHTTPRequestMethod(m) {
		return m
	}
	return "GET"
}

func bootstrapFrontendAPIInventoryFromHarvest(rt *Runtime, harvest *FrontendAPIHarvestReport) error {
	if rt == nil || harvest == nil {
		return utils.Error("nil bootstrap input")
	}
	inv := &FrontendAPIInventory{
		FrontendRoots: harvest.FrontendRoots,
		Calls:         harvest.Calls,
		Stats:         harvest.Stats,
		BootstrapNote: "auto-bootstrap from frontend_api_harvest.json",
	}
	return persistFrontendAPIInventory(rt, inv)
}
