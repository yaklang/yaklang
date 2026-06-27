package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

// APIPreanalysisReport 程序化预分析全量结果（写入 workdir/ssa_discovery/api_preanalysis.json）。
type APIPreanalysisReport struct {
	Version      int       `json:"version"`
	GeneratedAt  time.Time `json:"generated_at"`
	CodeRoot     string    `json:"code_root"`
	Language     string    `json:"language"`
	FullReport   string    `json:"full_report_path"`
	Frameworks   []string  `json:"framework_hints"`
	BuildMarkers []string  `json:"build_markers"`
	Modules      []struct {
		Name   string `json:"name"`
		RelDir string `json:"rel_dir"`
		Kind   string `json:"kind,omitempty"`
	} `json:"modules"`
	ConfigBaseCandidates []struct {
		Source  string `json:"source"`
		Key     string `json:"key"`
		Value   string `json:"value"`
		RelPath string `json:"rel_path"`
	} `json:"config_base_candidates"`
	RouteFileCandidates []struct {
		RelPath string `json:"rel_path"`
		Reason  string `json:"reason"`
	} `json:"route_file_candidates"`
	ControllerFileCandidates []struct {
		RelPath string `json:"rel_path"`
		Reason  string `json:"reason"`
	} `json:"controller_file_candidates,omitempty"`
	ApiRouteFiles []string `json:"api_route_files,omitempty"`
	BackendRoots  []string `json:"backend_roots,omitempty"`
	ScanExcludedStats struct {
		SkippedByDir int `json:"skipped_by_dir"`
		SkippedByExt int `json:"skipped_by_ext"`
	} `json:"scan_excluded_stats,omitempty"`
	OpenAPIFileCandidates []string `json:"openapi_file_candidates"`
	GatewayConfigFiles    []string `json:"gateway_config_files"`
	Warnings              []string `json:"warnings,omitempty"`
}

var (
	reSpringCtxPath = regexp.MustCompile(`(?i)context[-_]path\s*[:=]\s*['"]?(/[^'"\s#;]+)`)
	reServerPort    = regexp.MustCompile(`(?i)(?:server\.port|port)\s*[:=]\s*(\d+)`)
	reServletCtx    = regexp.MustCompile(`(?i)servlet\.context[-_]path\s*[:=]\s*['"]?(/[^'"\s#;]+)`)
)

// RunApiPreanalysisCollector 扫描代码根，生成 api_preanalysis.json 并将摘要写入 session.ApiPreanalysisMetaJSON。
func RunApiPreanalysisCollector(rt *Runtime) (*APIPreanalysisReport, error) {
	if rt == nil || rt.Session == nil || rt.Repo == nil {
		return nil, utils.Error("nil runtime")
	}
	sess := rt.Session
	if !sess.CodePathOK || strings.TrimSpace(sess.CodeRootPath) == "" {
		return nil, utils.Errorf("invalid code root")
	}
	root := filepath.Clean(sess.CodeRootPath)
	dir := filepath.Join(rt.WorkDir, store.SubDirName())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	rep := &APIPreanalysisReport{
		Version:     1,
		GeneratedAt: time.Now().UTC(),
		CodeRoot:    root,
		Language:    strings.TrimSpace(sess.Language),
		FullReport:  store.ApiPreanalysisReportPath(rt.WorkDir),
	}

	buildSeen := make(map[string]struct{})
	addBuild := func(rel string) {
		if _, ok := buildSeen[rel]; !ok {
			buildSeen[rel] = struct{}{}
			rep.BuildMarkers = append(rep.BuildMarkers, rel)
		}
	}

	routePath := routeFileCandidateFromPath

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if skipDirForRouteScan(info.Name()) {
				rep.ScanExcludedStats.SkippedByDir++
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if skipRouteFileByExt(rel) {
			rep.ScanExcludedStats.SkippedByExt++
			return nil
		}
		base := filepath.Base(rel)
		lb := strings.ToLower(base)

		switch lb {
		case "pom.xml", "build.gradle", "build.gradle.kts", "go.mod", "package.json",
			"composer.json", "pyproject.toml", "requirements.txt":
			addBuild(rel)
		case "application.yml", "application.yaml", "application.properties",
			"application-dev.yml", "bootstrap.yml", "bootstrap.yaml", "nginx.conf":
			scanConfigHints(rep, path, rel, 256*1024)
		}
		if strings.HasPrefix(lb, "application") && (strings.HasSuffix(lb, ".yml") || strings.HasSuffix(lb, ".yaml") || strings.HasSuffix(lb, ".properties")) {
			scanConfigHints(rep, path, rel, 256*1024)
		}
		if strings.Contains(strings.ToLower(rel), "gateway") && (strings.HasSuffix(lb, ".yml") || strings.HasSuffix(lb, ".yaml")) {
			rep.GatewayConfigFiles = append(rep.GatewayConfigFiles, rel)
		}
		if r, reason := routePath(rel); r != "" {
			appendRouteFileCandidate(rep, r, reason)
		} else if data, rerr := readFileHead(path, 256*1024); rerr == nil {
			if r, reason := routeFileCandidateFromContent(rel, data); r != "" {
				appendRouteFileCandidate(rep, r, reason)
			}
		}
		if looksOpenAPIFilename(lb) {
			rep.OpenAPIFileCandidates = append(rep.OpenAPIFileCandidates, rel)
		}
		return nil
	})

	enrichPreanalysisFromGoMod(rep, root)
	frameworkGuess(rep)
	enrichPreanalysisNarrowFields(rep, rt.WorkDir)

	rec, recErr := ReconcileLanguage(root, strings.TrimSpace(sess.Language))
	if recErr == nil {
		rep.Language = string(rec.Language)
		rep.Warnings = append(rep.Warnings, rec.Warnings...)
		if !strings.EqualFold(string(rec.Language), strings.TrimSpace(sess.Language)) {
			sess.Language = string(rec.Language)
		}
	}

	if len(rep.RouteFileCandidates) > 5000 {
		rep.Warnings = append(rep.Warnings, "route_file_candidates truncated from large tree")
		rep.RouteFileCandidates = rep.RouteFileCandidates[:5000]
	}

	fullPath := store.ApiPreanalysisReportPath(rt.WorkDir)
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(fullPath, b, 0o644); err != nil {
		return nil, err
	}
	_ = rt.Repo.UpsertPhaseArtifact(sess.ID, store.ArtifactApiPreanalysisFull, string(b))

	summary := map[string]any{
		"full_report_path":          fullPath,
		"framework_hints":           rep.Frameworks,
		"build_markers_count":       len(rep.BuildMarkers),
		"config_base_count":         len(rep.ConfigBaseCandidates),
		"route_candidates_count":       len(rep.RouteFileCandidates),
		"controller_candidates_count":  len(rep.ControllerFileCandidates),
		"api_route_files_count":        len(rep.ApiRouteFiles),
		"openapi_candidates_count":  len(rep.OpenAPIFileCandidates),
		"gateway_config_files_count": len(rep.GatewayConfigFiles),
		"language":                  rep.Language,
		"warnings":                  rep.Warnings,
	}
	if recErr == nil {
		summary["detected_language"] = string(rec.Detected)
		summary["language_source"] = rec.Source
	}
	sumB, _ := json.Marshal(summary)
	sess.ApiPreanalysisMetaJSON = string(sumB)
	if err := rt.Repo.UpdateSession(sess); err != nil {
		return rep, err
	}
	rt.Session = sess
	return rep, nil
}

// readPreanalysisHintsForReactive returns a short block for Phase4 reactive context.
func readPreanalysisHintsForReactive(rt *Runtime) string {
	if rt == nil {
		return ""
	}
	payload, err := GetPhaseArtifactPayload(rt, store.ArtifactApiPreanalysisFull, store.ApiPreanalysisReportPath(rt.WorkDir))
	if err != nil {
		return ""
	}
	var rep struct {
		Language     string   `json:"language"`
		Frameworks   []string `json:"framework_hints"`
		BuildMarkers []string `json:"build_markers"`
	}
	if json.Unmarshal([]byte(payload), &rep) != nil {
		return ""
	}
	return fmt.Sprintf("framework_hints: %v\nbuild_markers: %v\ndetected_language: %s",
		rep.Frameworks, rep.BuildMarkers, rep.Language)
}

func loadCodeReadingAuthNotesForRuntime(rt *Runtime) string {
	if rt == nil {
		return ""
	}
	payload, err := GetPhaseArtifactPayload(rt, store.ArtifactCodeReadingPlan, store.CodeReadingPlanPath(rt.WorkDir))
	if err != nil {
		return ""
	}
	var plan struct {
		AuthNotes string `json:"auth_notes"`
	}
	if json.Unmarshal([]byte(payload), &plan) != nil {
		return ""
	}
	return strings.TrimSpace(plan.AuthNotes)
}

// loadCodeReadingAuthNotes reads auth_notes from code_reading_plan (file fallback for legacy callers).
func loadCodeReadingAuthNotes(workDir string) string {
	b, err := os.ReadFile(store.CodeReadingPlanPath(workDir))
	if err != nil {
		return ""
	}
	var plan struct {
		AuthNotes string `json:"auth_notes"`
	}
	if json.Unmarshal(b, &plan) != nil {
		return ""
	}
	return strings.TrimSpace(plan.AuthNotes)
}

func looksOpenAPIFilename(lb string) bool {
	if strings.Contains(lb, "openapi") || strings.Contains(lb, "swagger") {
		return strings.HasSuffix(lb, ".json") || strings.HasSuffix(lb, ".yaml") || strings.HasSuffix(lb, ".yml")
	}
	return lb == "api-docs.json" || lb == "swagger.json"
}

func scanConfigHints(rep *APIPreanalysisReport, abs, rel string, maxBytes int64) {
	data, err := readFileHead(abs, maxBytes)
	if err != nil || len(data) == 0 {
		return
	}
	s := string(data)
	add := func(key, val string) {
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		rep.ConfigBaseCandidates = append(rep.ConfigBaseCandidates, struct {
			Source  string `json:"source"`
			Key     string `json:"key"`
			Value   string `json:"value"`
			RelPath string `json:"rel_path"`
		}{Source: "config_scan", Key: key, Value: val, RelPath: rel})
	}
	for _, m := range reSpringCtxPath.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 {
			add("context-path", m[1])
		}
	}
	for _, m := range reServletCtx.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 {
			add("servlet.context-path", m[1])
		}
	}
	for _, m := range reServerPort.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 {
			add("server_PORT", m[1])
		}
	}
}

func readFileHead(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, max)
	n, err := f.Read(buf)
	if n >= 0 {
		return buf[:n], err
	}
	return nil, err
}

func frameworkGuess(rep *APIPreanalysisReport) {
	set := make(map[string]struct{})
	for _, m := range rep.BuildMarkers {
		lm := strings.ToLower(m)
		switch {
		case strings.HasSuffix(lm, "pom.xml"):
			set["java_maven"] = struct{}{}
		case strings.HasSuffix(lm, "go.mod"):
			set["golang"] = struct{}{}
		case strings.HasSuffix(lm, "composer.json"):
			set["php"] = struct{}{}
		case strings.HasSuffix(lm, "package.json"):
			set["nodejs"] = struct{}{}
		case strings.HasSuffix(lm, "pyproject.toml") || strings.HasSuffix(lm, "requirements.txt"):
			set["python"] = struct{}{}
		}
	}
	for k := range set {
		rep.Frameworks = append(rep.Frameworks, k)
	}
}

func routeFileCandidateFromPath(rel string) (string, string) {
	l := strings.ToLower(filepath.ToSlash(rel))
	base := filepath.Base(l)
	switch {
	case base == "main.go":
		return rel, "go_entry_main"
	case strings.HasSuffix(l, "/main.go") && strings.HasPrefix(l, "cmd/"):
		return rel, "go_cmd_main"
	case strings.Contains(l, "controller"), strings.Contains(l, "/routes/"),
		strings.Contains(l, "router"), strings.HasSuffix(l, "routes.php"):
		return rel, "path_heuristic"
	case strings.Contains(l, "/handler/") || strings.Contains(l, "\\handler\\"):
		if strings.Contains(l, "controller") || strings.Contains(l, "router") || strings.Contains(l, "routes") {
			return rel, "path_heuristic"
		}
	case strings.Contains(base, "route"), strings.Contains(base, "router"):
		return rel, "path_heuristic"
	case strings.Contains(base, "handler"):
		if strings.Contains(l, "controller") || strings.Contains(l, "router") || strings.Contains(l, "routes") {
			return rel, "path_heuristic"
		}
	}
	return "", ""
}

func routeFileCandidateFromContent(rel string, data []byte) (string, string) {
	if len(data) == 0 {
		return "", ""
	}
	s := string(data)
	if len(s) > 262144 {
		s = s[:262144]
	}
	for _, needle := range []string{
		"http.HandleFunc", "http.Handle(", "gin.New(", "echo.New(", "chi.NewRouter(",
	} {
		if strings.Contains(s, needle) {
			return rel, "content_http_router"
		}
	}
	return "", ""
}

func appendRouteFileCandidate(rep *APIPreanalysisReport, rel, reason string) {
	if rep == nil || rel == "" {
		return
	}
	for _, existing := range rep.RouteFileCandidates {
		if existing.RelPath == rel {
			return
		}
	}
	rep.RouteFileCandidates = append(rep.RouteFileCandidates, struct {
		RelPath string `json:"rel_path"`
		Reason  string `json:"reason"`
	}{RelPath: rel, Reason: reason})
}
