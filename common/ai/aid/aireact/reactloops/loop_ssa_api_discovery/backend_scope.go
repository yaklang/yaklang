package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// BackendScopeReport is written to workdir/ssa_discovery/backend_scope.json.
type BackendScopeReport struct {
	Version               int       `json:"version"`
	GeneratedAt           time.Time `json:"generated_at"`
	CodeRoot              string    `json:"code_root"`
	Language              string    `json:"language"`
	BackendRoots          []string  `json:"backend_roots"`
	FrontendRoots         []string  `json:"frontend_roots,omitempty"`
	RuleBackendRoots      []string  `json:"rule_backend_roots,omitempty"`
	LLMUsed               bool      `json:"llm_used,omitempty"`
	Reason                string    `json:"reason,omitempty"`
	ScanExcludedStats     struct {
		SkippedByDir int `json:"skipped_by_dir"`
		SkippedByExt int `json:"skipped_by_ext"`
	} `json:"scan_excluded_stats"`
	ControllerFileCandidates []struct {
		RelPath string `json:"rel_path"`
		Reason  string `json:"reason"`
	} `json:"controller_file_candidates"`
	ApiRouteFiles []string `json:"api_route_files"`
}

var routeFileExtBlacklist = map[string]struct{}{
	".js": {}, ".css": {}, ".scss": {}, ".less": {}, ".map": {},
	".png": {}, ".svg": {}, ".woff": {}, ".woff2": {},
	".html": {}, ".htm": {}, ".ftl": {}, ".vm": {}, ".jsp": {},
}

var harvestDirBlacklistExtra = map[string]struct{}{
	"webapp": {}, "static": {}, "assets": {}, "public": {},
	"templates": {}, "template": {}, "test": {},
}

func skipDirForRouteScan(name string) bool {
	if skipDirForHarvest(name) {
		return true
	}
	_, ok := harvestDirBlacklistExtra[strings.ToLower(name)]
	return ok
}

func skipRouteFileByExt(rel string) bool {
	ext := strings.ToLower(filepath.Ext(rel))
	_, ok := routeFileExtBlacklist[ext]
	return ok
}

func isHandlerPathMisreport(rel string) bool {
	l := strings.ToLower(filepath.ToSlash(strings.TrimSpace(rel)))
	if l == "" {
		return false
	}
	base := filepath.Base(l)
	if !strings.HasSuffix(base, "handler.java") {
		return false
	}
	if strings.Contains(l, "/controller/") {
		return false
	}
	return strings.Contains(l, "/handler/")
}

func isNarrowControllerCandidate(rel, lang string, staticFiles map[string]struct{}) (bool, string) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || skipRouteFileByExt(rel) {
		return false, ""
	}
	if strings.Contains(strings.ToLower(rel), "/src/test/") || strings.Contains(strings.ToLower(rel), "/resources/generator/") {
		return false, "test_or_generator"
	}
	if _, ok := staticFiles[rel]; ok {
		return true, "static_hint_ref"
	}
	if isHandlerPathMisreport(rel) {
		return false, "handler_path_exclude"
	}
	l := strings.ToLower(rel)
	base := filepath.Base(l)
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "java", "":
		if !strings.HasSuffix(l, ".java") {
			return false, ""
		}
	case "golang", "go":
		if !strings.HasSuffix(l, ".go") {
			return false, ""
		}
	case "php":
		if !strings.HasSuffix(l, ".php") {
			return false, ""
		}
	default:
		if !(strings.HasSuffix(l, ".java") || strings.HasSuffix(l, ".go") || strings.HasSuffix(l, ".php")) {
			return false, ""
		}
	}
	if strings.Contains(l, "/controller/") || strings.HasSuffix(base, "controller.java") {
		return true, "controller_path_or_name"
	}
	return false, ""
}

func inferRuleBackendRoots(rep *APIPreanalysisReport, codeRoot string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = filepath.ToSlash(strings.TrimSpace(p))
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, m := range rep.Modules {
		rd := strings.ToLower(filepath.ToSlash(m.RelDir))
		switch {
		case strings.Contains(rd, "-core"), strings.Contains(rd, "-api"),
			strings.Contains(rd, "-web"), strings.Contains(rd, "-server"),
			strings.HasPrefix(rd, "cmd/"), strings.HasPrefix(rd, "internal/"):
			add(m.RelDir)
		}
	}
	_ = filepath.Walk(codeRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || !info.IsDir() {
			return nil
		}
		if skipDirForRouteScan(info.Name()) {
			return filepath.SkipDir
		}
		rel, rerr := filepath.Rel(codeRoot, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		l := strings.ToLower(rel)
		if strings.Contains(l, "src/main/java") && strings.Count(l, "/") <= 6 {
			add(rel)
		}
		return nil
	})
	return out
}

func staticHintFileSet(workDir string) map[string]struct{} {
	out := map[string]struct{}{}
	b, err := os.ReadFile(store.StaticRouteHintsPath(workDir))
	if err != nil {
		return out
	}
	var rep StaticRouteHintsReport
	if json.Unmarshal(b, &rep) != nil {
		return out
	}
	for _, h := range rep.Hints {
		if p := filepath.ToSlash(strings.TrimSpace(h.FileRelPath)); p != "" {
			out[p] = struct{}{}
		}
	}
	return out
}

func enrichPreanalysisNarrowFields(rep *APIPreanalysisReport, workDir string) {
	if rep == nil {
		return
	}
	staticFiles := staticHintFileSet(workDir)
	rep.BackendRoots = inferRuleBackendRoots(rep, rep.CodeRoot)
	rep.ControllerFileCandidates = nil
	seenCtrl := map[string]struct{}{}
	for _, c := range rep.RouteFileCandidates {
		ok, reason := isNarrowControllerCandidate(c.RelPath, rep.Language, staticFiles)
		if !ok {
			continue
		}
		if _, dup := seenCtrl[c.RelPath]; dup {
			continue
		}
		seenCtrl[c.RelPath] = struct{}{}
		rep.ControllerFileCandidates = append(rep.ControllerFileCandidates, struct {
			RelPath string `json:"rel_path"`
			Reason  string `json:"reason"`
		}{RelPath: c.RelPath, Reason: reason})
	}
	for p := range staticFiles {
		if _, ok := seenCtrl[p]; ok {
			continue
		}
		seenCtrl[p] = struct{}{}
		rep.ControllerFileCandidates = append(rep.ControllerFileCandidates, struct {
			RelPath string `json:"rel_path"`
			Reason  string `json:"reason"`
		}{RelPath: p, Reason: "static_hint_ref"})
	}
	rep.ApiRouteFiles = dedupeStrings(collectAPIRouteFiles(rep, staticFiles))
}

func collectAPIRouteFiles(rep *APIPreanalysisReport, staticFiles map[string]struct{}) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = filepath.ToSlash(strings.TrimSpace(p))
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for p := range staticFiles {
		add(p)
	}
	for _, c := range rep.ControllerFileCandidates {
		add(c.RelPath)
	}
	return out
}

// RunBuildBackendScope narrows api_route_files and writes backend_scope.json.
func RunBuildBackendScope(ctx context.Context, r aicommon.AIInvokeRuntime, rt *Runtime) (*BackendScopeReport, error) {
	if rt == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	_ = ctx
	_ = r

	rep, err := loadOrRunPreanalysis(rt)
	if err != nil {
		return nil, err
	}
	staticFiles := staticHintFileSet(rt.WorkDir)
	enrichPreanalysisNarrowFields(rep, rt.WorkDir)

	scope := &BackendScopeReport{
		Version:          1,
		GeneratedAt:      time.Now().UTC(),
		CodeRoot:         rep.CodeRoot,
		Language:         rep.Language,
		BackendRoots:     rep.BackendRoots,
		RuleBackendRoots: rep.BackendRoots,
		Reason:           "rule-based backend scope; static hints primary worklist",
	}
	scope.ScanExcludedStats.SkippedByDir = rep.ScanExcludedStats.SkippedByDir
	scope.ScanExcludedStats.SkippedByExt = rep.ScanExcludedStats.SkippedByExt
	scope.ControllerFileCandidates = rep.ControllerFileCandidates
	scope.ApiRouteFiles = rep.ApiRouteFiles

	var auditRows []FileOpInput
	for _, c := range rep.RouteFileCandidates {
		ok, reason := isNarrowControllerCandidate(c.RelPath, rep.Language, staticFiles)
		if ok {
			auditRows = append(auditRows, FileOpInput{
				Stage: store.FileOpStagePhase1BPre, Operation: store.FileOpFilterInclude,
				RelPath: c.RelPath, RuleID: reason, ToolName: "backend_scope",
				Outcome: store.FileOpOutcomeIncluded, Summary: "api_route_files candidate",
			})
			continue
		}
		if isHandlerPathMisreport(c.RelPath) {
			auditRows = append(auditRows, FileOpInput{
				Stage: store.FileOpStagePhase1BPre, Operation: store.FileOpFilterExclude,
				RelPath: c.RelPath, RuleID: "handler_path_exclude", ToolName: "backend_scope",
				Outcome: store.FileOpOutcomeExcluded, Summary: "handler 误报排除",
			})
		} else if skipRouteFileByExt(c.RelPath) {
			auditRows = append(auditRows, FileOpInput{
				Stage: store.FileOpStagePhase1BPre, Operation: store.FileOpFilterExclude,
				RelPath: c.RelPath, RuleID: "blacklist_ext", ToolName: "backend_scope",
				Outcome: store.FileOpOutcomeExcluded, Summary: "扩展名黑名单",
			})
		}
	}
	logFileOpsBatch(rt, auditRows)

	dir := filepath.Join(rt.WorkDir, store.SubDirName())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := store.BackendScopePath(rt.WorkDir)
	b, err := json.MarshalIndent(scope, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return nil, err
	}
	if rt.Repo != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactBackendScope, string(b))
	}

	// Refresh preanalysis with narrow fields
	repB, _ := json.MarshalIndent(rep, "", "  ")
	_ = os.WriteFile(store.ApiPreanalysisReportPath(rt.WorkDir), repB, 0o644)
	if rt.Repo != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactApiPreanalysisFull, string(repB))
	}

	log.Infof("ssa_api_discovery: backend_scope api_route_files=%d (from %d route candidates)",
		len(scope.ApiRouteFiles), len(rep.RouteFileCandidates))
	return scope, nil
}

func loadOrRunPreanalysis(rt *Runtime) (*APIPreanalysisReport, error) {
	path := store.ApiPreanalysisReportPath(rt.WorkDir)
	if b, err := os.ReadFile(path); err == nil && len(b) > 2 {
		var rep APIPreanalysisReport
		if json.Unmarshal(b, &rep) == nil {
			return &rep, nil
		}
	}
	return RunApiPreanalysisCollector(rt)
}

func loadBackendScope(workDir string) (*BackendScopeReport, error) {
	b, err := os.ReadFile(store.BackendScopePath(workDir))
	if err != nil {
		return nil, err
	}
	var scope BackendScopeReport
	if err := json.Unmarshal(b, &scope); err != nil {
		return nil, err
	}
	return &scope, nil
}

func apiRouteFilesForRuntime(rt *Runtime) []string {
	if rt == nil {
		return nil
	}
	if scope, err := loadBackendScope(rt.WorkDir); err == nil && len(scope.ApiRouteFiles) > 0 {
		return scope.ApiRouteFiles
	}
	if b, err := os.ReadFile(store.ApiPreanalysisReportPath(rt.WorkDir)); err == nil {
		var rep struct {
			ApiRouteFiles []string `json:"api_route_files"`
		}
		if json.Unmarshal(b, &rep) == nil && len(rep.ApiRouteFiles) > 0 {
			return rep.ApiRouteFiles
		}
	}
	return nil
}
