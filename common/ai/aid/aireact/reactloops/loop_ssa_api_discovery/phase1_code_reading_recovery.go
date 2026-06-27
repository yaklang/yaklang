package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const codeReadingPlanRecoveryNote = "auto-recovered: AI finalize_code_reading_plan did not commit; built from static_route_hints.json"

// EnsureCodeReadingPlanFile guarantees workdir/ssa_discovery/code_reading_plan.json exists when CodePathOK.
// Order: valid existing plan → stages merge → DB → static_route_hints fallback.
func EnsureCodeReadingPlanFile(rt *Runtime) error {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return nil
	}
	path := store.CodeReadingPlanPath(rt.WorkDir)
	if plan, err := LoadCodeReadingPlan(rt.WorkDir); err == nil && len(plan.DiscoveredAPIs) > 0 {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		log.Warnf("ssa_api_discovery: code_reading_plan exists but discovered_apis empty; rebuilding")
	}
	if rt.Repo != nil {
		if payload, err := GetPhaseArtifactPayload(rt, store.ArtifactCodeReadingPlan, ""); err == nil && strings.TrimSpace(payload) != "" {
			var plan CodeReadingPlan
			if json.Unmarshal([]byte(payload), &plan) == nil && len(plan.DiscoveredAPIs) > 0 {
				if err := writeJSONFile(path, []byte(payload)); err != nil {
					return err
				}
				log.Infof("ssa_api_discovery: restored code_reading_plan.json from phase_artifact")
				return nil
			}
		}
	}
	if plan, err := BuildCodeReadingPlanFromStageFiles(rt); err == nil && len(plan.DiscoveredAPIs) > 0 {
		if err := PersistCodeReadingPlan(rt, plan); err != nil {
			return err
		}
		log.Infof("ssa_api_discovery: rebuilt code_reading_plan.json from stages (discovered_apis=%d)", len(plan.DiscoveredAPIs))
		return nil
	}
	if plan, err := BuildCodeReadingPlanFromDB(rt); err == nil && plan != nil && len(plan.DiscoveredAPIs) > 0 {
		if err := PersistCodeReadingPlan(rt, plan); err != nil {
			return err
		}
		log.Infof("ssa_api_discovery: wrote code_reading_plan.json from DB http_endpoints (discovered_apis=%d)", len(plan.DiscoveredAPIs))
		return nil
	}
	plan, err := BuildFallbackCodeReadingPlanFromStaticHints(rt)
	if err != nil {
		return err
	}
	if err := PersistCodeReadingPlan(rt, plan); err != nil {
		return err
	}
	log.Warnf("ssa_api_discovery: wrote fallback code_reading_plan.json from static_route_hints (discovered_apis=%d)", len(plan.DiscoveredAPIs))
	return nil
}

// PersistCodeReadingPlan writes code_reading_plan.json and upserts the DB artifact.
func PersistCodeReadingPlan(rt *Runtime, plan *CodeReadingPlan) error {
	if rt == nil || plan == nil {
		return utils.Error("nil runtime or plan")
	}
	b, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	path := store.CodeReadingPlanPath(rt.WorkDir)
	if err := writeJSONFile(path, b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactCodeReadingPlan, string(b))
	}
	return nil
}

// BuildFallbackCodeReadingPlanFromStaticHints synthesizes a minimal plan from static hints.
func BuildFallbackCodeReadingPlanFromStaticHints(rt *Runtime) (*CodeReadingPlan, error) {
	if rt == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	b, err := os.ReadFile(store.StaticRouteHintsPath(rt.WorkDir))
	if err != nil {
		return nil, utils.Errorf("cannot build fallback plan: %v", err)
	}
	var rep StaticRouteHintsReport
	if err := json.Unmarshal(b, &rep); err != nil {
		return nil, err
	}
	if len(rep.Hints) == 0 {
		return nil, utils.Error("static_route_hints.hints is empty")
	}

	seenRoute := map[string]struct{}{}
	seenFile := map[string]struct{}{}
	urlSpaces := map[string]any{}
	var apis []DiscoveredAPI
	var completed []string
	var queue []string
	var bases []string

	for _, h := range rep.Hints {
		method := strings.ToUpper(strings.TrimSpace(h.Method))
		pathPat := normURLPath(h.PathPattern)
		if method == "" || pathPat == "" || isWildcardRoutePattern(pathPat) {
			continue
		}
		key := routeKey(method, pathPat)
		if _, ok := seenRoute[key]; ok {
			continue
		}
		seenRoute[key] = struct{}{}

		fileRel := normalizePlanFileRef(rt, h.FileRelPath)
		if fileRel == "" {
			fileRel = normalizePlanFileRef(rt, guessFileFromHandlerClass(h.HandlerClass))
		}
		apis = append(apis, DiscoveredAPI{
			Method:        method,
			PathPattern:   pathPat,
			HandlerFile:   fileRel,
			HandlerSymbol: strings.TrimSpace(h.HandlerMethod),
			HandlerClass:  strings.TrimSpace(h.HandlerClass),
			CodeEvidence:  codeReadingPlanRecoveryNote,
		})
		if fileRel != "" {
			if _, ok := seenFile[fileRel]; !ok {
				seenFile[fileRel] = struct{}{}
				completed = append(completed, fileRel)
				queue = append(queue, fileRel)
			}
			if cls := strings.TrimSpace(h.HandlerClass); cls != "" {
				base := inferBasePathFromHandlerClass(cls)
				if base != "" {
					urlSpaces[simpleNameFromHandlerClass(cls)] = map[string]any{"base_path": base}
					bases = append(bases, base)
				}
			}
		}
	}
	if len(apis) == 0 {
		return nil, utils.Error("no usable routes in static_route_hints after filtering wildcards")
	}
	if len(urlSpaces) == 0 {
		urlSpaces["default"] = map[string]any{"base_path": "/"}
		bases = append(bases, "/")
	}

	return &CodeReadingPlan{
		DiscoveredAPIs:     apis,
		ReadFilesCompleted: completed,
		ReadQueue:          queue,
		EffectiveBases:     dedupeStrings(bases),
		URLSpaces:          urlSpaces,
		HintDiff:           codeReadingPlanRecoveryNote,
		AuthNotes:          loadCodeReadingAuthNotes(rt.WorkDir),
	}, nil
}

// mergeStaticHintsIntoPlan appends routes from static_route_hints.json that are not already in plan.
func mergeStaticHintsIntoPlan(rt *Runtime, plan *CodeReadingPlan) *CodeReadingPlan {
	if plan == nil {
		plan = &CodeReadingPlan{}
	}
	fb, err := BuildFallbackCodeReadingPlanFromStaticHints(rt)
	if err != nil || fb == nil {
		return plan
	}
	seen := map[string]struct{}{}
	seenFile := map[string]struct{}{}
	for _, a := range plan.DiscoveredAPIs {
		seen[routeKey(a.Method, a.PathPattern)] = struct{}{}
	}
	for _, f := range plan.ReadFilesCompleted {
		seenFile[f] = struct{}{}
	}
	for _, a := range fb.DiscoveredAPIs {
		key := routeKey(a.Method, a.PathPattern)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		plan.DiscoveredAPIs = append(plan.DiscoveredAPIs, a)
	}
	for _, f := range fb.ReadFilesCompleted {
		if _, ok := seenFile[f]; ok {
			continue
		}
		seenFile[f] = struct{}{}
		plan.ReadFilesCompleted = append(plan.ReadFilesCompleted, f)
	}
	if len(plan.EffectiveBases) == 0 {
		plan.EffectiveBases = fb.EffectiveBases
	}
	if len(plan.URLSpaces) == 0 {
		plan.URLSpaces = fb.URLSpaces
	}
	return plan
}

func isWildcardRoutePattern(path string) bool {
	p := strings.TrimSpace(path)
	if p == "" || p == "/**" || p == "/**/**" {
		return true
	}
	return strings.Count(p, "**") >= 2
}

func inferBasePathFromHandlerClass(class string) string {
	// evidence-first: package-name heuristics removed; base paths come from code reading / routing facts.
	_ = class
	return ""
}

func simpleNameFromHandlerClass(class string) string {
	if i := strings.LastIndex(class, "."); i >= 0 && i+1 < len(class) {
		return class[i+1:]
	}
	return class
}

func guessFileFromHandlerClass(class string) string {
	class = strings.TrimSpace(class)
	if class == "" {
		return ""
	}
	pkg := class
	if i := strings.LastIndex(class, "."); i >= 0 {
		pkg = class[:i]
	}
	pkgPath := strings.ReplaceAll(pkg, ".", "/")
	simple := simpleNameFromHandlerClass(class)
	if simple == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Join(pkgPath, simple+".java"))
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
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

// normalizePlanFileRef converts absolute paths under code_root to repo-relative slash paths.
func normalizePlanFileRef(rt *Runtime, raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = filepath.ToSlash(s)
	if rt == nil || rt.Session == nil {
		return s
	}
	root := filepath.Clean(rt.Session.CodeRootPath)
	if root == "" || root == "." {
		return s
	}
	rootSlash := filepath.ToSlash(root)
	if strings.HasPrefix(s, rootSlash+"/") {
		return strings.TrimPrefix(s, rootSlash+"/")
	}
	if abs, err := filepath.Abs(s); err == nil {
		absSlash := filepath.ToSlash(abs)
		if strings.HasPrefix(absSlash, rootSlash+"/") {
			return strings.TrimPrefix(absSlash, rootSlash+"/")
		}
	}
	return s
}

func isControllerRouteCandidate(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(strings.TrimSpace(path)))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, ".ftl") || strings.Contains(lower, "/test/") || strings.Contains(lower, "/resources/generator/") {
		return false
	}
	return strings.Contains(lower, "controller")
}

// requiredReadFilesForCodeReadingPlan lists files that finalize must prove were read.
// Prefer handler_file from discovered_apis; do not require every api_preanalysis candidate (often 100+).
func requiredReadFilesForCodeReadingPlan(plan map[string]any, candidates []string, rt *Runtime) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = normalizePlanFileRef(rt, p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}

	if rawAPIs, ok := plan["discovered_apis"].([]any); ok {
		for _, item := range rawAPIs {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			add(fmt.Sprint(row["handler_file"]))
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, cand := range candidates {
		if !isControllerRouteCandidate(cand) {
			continue
		}
		add(cand)
		if len(out) >= 40 {
			break
		}
	}
	return out
}

func planPathInSet(set map[string]struct{}, rt *Runtime, path string) bool {
	norm := normalizePlanFileRef(rt, path)
	if norm == "" {
		return false
	}
	if _, ok := set[norm]; ok {
		return true
	}
	base := filepath.Base(norm)
	for k := range set {
		if k == norm || filepath.Base(k) == base {
			return true
		}
	}
	return false
}
