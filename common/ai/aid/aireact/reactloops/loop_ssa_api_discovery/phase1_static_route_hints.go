package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const (
	SourceAICodeRead  = "ai_code_read"
	SourceStaticHint  = "static_hint"
	sourceStaticHintY = "static_hint_yak"
)

// StaticRouteHint is a non-authoritative route candidate from static/Yak harvest.
type StaticRouteHint struct {
	Method        string `json:"method"`
	PathPattern   string `json:"path_pattern"`
	HandlerClass  string `json:"handler_class,omitempty"`
	HandlerMethod string `json:"handler_method,omitempty"`
	FileRelPath   string `json:"file_rel_path,omitempty"`
	Source        string `json:"source"`
}

// StaticRouteHintsReport is written to static_route_hints.json during Phase1A.
type StaticRouteHintsReport struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Language    string            `json:"language"`
	Hints       []StaticRouteHint `json:"hints"`
	Count       int               `json:"count"`
	SourcesRun  []string          `json:"sources_run,omitempty"`
	Warnings    []string          `json:"warnings,omitempty"`
	FullPath    string            `json:"full_report_path"`
}

// IsAIPrimaryEndpointSource returns true when the endpoint source is AI-owned and must not be overwritten.
func IsAIPrimaryEndpointSource(source string) bool {
	s := strings.TrimSpace(strings.ToLower(source))
	return s == SourceAICodeRead || s == "ai" || strings.HasPrefix(s, "ai_")
}

// CollectStaticRouteHints runs Go/Yak static harvesters and writes hints only (no http_endpoints DB writes).
func CollectStaticRouteHints(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime) (*StaticRouteHintsReport, error) {
	if rt == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	sess := rt.Session
	rep := &StaticRouteHintsReport{
		GeneratedAt: time.Now().UTC(),
		Language:    sess.Language,
		Hints:       []StaticRouteHint{},
		FullPath:    store.StaticRouteHintsPath(rt.WorkDir),
	}
	if !sess.CodePathOK || strings.TrimSpace(sess.CodeRootPath) == "" {
		rep.Warnings = append(rep.Warnings, "code path invalid; no static hints")
		return rep, writeStaticRouteHintsReport(rt.WorkDir, rep)
	}

	langEnum, lerr := ssaconfig.ValidateLanguage(sess.Language)
	frameworks := detectedFrameworkIDs(loadProfileOrNilFromWorkDir(rt.WorkDir))
	if lerr == nil && langEnum != "" {
		harvesters := staticHarvestersForFrameworks(langEnum, frameworks)
		for _, h := range harvesters {
			rows, herr := h.fn(sess.CodeRootPath)
			if herr != nil {
				return nil, herr
			}
			rep.SourcesRun = append(rep.SourcesRun, h.sourceKey)
			for _, row := range rows {
				rep.Hints = append(rep.Hints, harvestedToStaticHint(row, h.sourceKey))
			}
		}
	}

	if invoker != nil {
		extra := map[string]any{
			"hints-only":               1,
			"ai-mode":                  "function_call",
			"file-concurrent":          4,
			"ai-route-sniff-max-files": 4,
		}
		if pre := store.ApiPreanalysisReportPath(rt.WorkDir); pre != "" {
			extra["preanalysis"] = pre
		}
		if _, yakErr := executeYakTool(invoker, ctx, ToolRouteCoreHarvest, rt, extra); yakErr != nil {
			rep.Warnings = append(rep.Warnings, "yak hint harvest: "+utils.ShrinkString(yakErr.Error(), 300))
		} else {
			yakHints, rerr := readYakRouteHarvestAsHints(rt.WorkDir)
			if rerr != nil {
				rep.Warnings = append(rep.Warnings, "read yak harvest: "+rerr.Error())
			} else {
				rep.SourcesRun = append(rep.SourcesRun, "yak_api_route_harvest")
				rep.Hints = append(rep.Hints, yakHints...)
			}
		}
	}

	rep.Hints = dedupeStaticHints(rep.Hints)
	rep.Hints = enrichAllStaticHintsWithServletMap(rt, rep.Hints)
	rep.Count = len(rep.Hints)
	if err := writeStaticRouteHintsReport(rt.WorkDir, rep); err != nil {
		return rep, err
	}
	if rt.Repo != nil {
		if b, merr := json.MarshalIndent(rep, "", "  "); merr == nil {
			_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactStaticRouteHints, string(b))
		}
	}
	log.Infof("ssa_api_discovery: static_route_hints count=%d sources=%v", rep.Count, rep.SourcesRun)
	return rep, nil
}

func harvestedToStaticHint(h HarvestedEndpoint, sourceKey string) StaticRouteHint {
	_ = sourceKey
	return StaticRouteHint{
		Method:        strings.ToUpper(strings.TrimSpace(h.Method)),
		PathPattern:   normURLPath(h.PathPattern),
		HandlerClass:  h.HandlerClass,
		HandlerMethod: h.HandlerMethod,
		FileRelPath:   h.FileRelPath,
		Source:        SourceStaticHint,
	}
}

func enrichAllStaticHintsWithServletMap(rt *Runtime, hints []StaticRouteHint) []StaticRouteHint {
	if rt == nil || len(hints) == 0 {
		return hints
	}
	out := make([]StaticRouteHint, len(hints))
	for i, h := range hints {
		job := FeatureWorkJob{
			EntryFile:  h.FileRelPath,
			StaticHints: []StaticRouteHint{h},
		}
		if h.HandlerClass != "" {
			job.PackagePatterns = []string{handlerClassToPackagePattern(h.HandlerClass)}
		}
		out[i] = enrichStaticRouteHintPath(rt, h, job)
	}
	return out
}

func handlerClassToPackagePattern(fqClass string) string {
	fqClass = strings.TrimSpace(fqClass)
	if fqClass == "" {
		return ""
	}
	if i := strings.LastIndex(fqClass, "."); i > 0 {
		return fqClass[:i] + ".*"
	}
	return fqClass + ".*"
}

func dedupeStaticHints(in []StaticRouteHint) []StaticRouteHint {
	seen := make(map[string]struct{})
	var out []StaticRouteHint
	for _, h := range in {
		k := routeKey(h.Method, h.PathPattern)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, h)
	}
	return out
}

func writeStaticRouteHintsReport(workDir string, rep *StaticRouteHintsReport) error {
	if rep == nil {
		return utils.Error("nil report")
	}
	rep.FullPath = store.StaticRouteHintsPath(workDir)
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONFile(rep.FullPath, b)
}

func readStaticRouteHintsReport(workDir string) (*StaticRouteHintsReport, error) {
	path := store.StaticRouteHintsPath(workDir)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rep StaticRouteHintsReport
	if err := json.Unmarshal(b, &rep); err != nil {
		return nil, err
	}
	return &rep, nil
}

func readYakRouteHarvestAsHints(workDir string) ([]StaticRouteHint, error) {
	b, err := os.ReadFile(store.ApiRouteHarvestReportPath(workDir))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Endpoints []struct {
			Method      string `json:"method"`
			PathPattern string `json:"path_pattern"`
			FileRelPath string `json:"file_rel_path"`
			Source      string `json:"source"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, err
	}
	out := make([]StaticRouteHint, 0, len(payload.Endpoints))
	for _, ep := range payload.Endpoints {
		if ep.Method == "" || ep.PathPattern == "" {
			continue
		}
		src := sourceStaticHintY
		if ep.Source != "" {
			src = ep.Source
		}
		out = append(out, StaticRouteHint{
			Method:      strings.ToUpper(ep.Method),
			PathPattern: normURLPath(ep.PathPattern),
			FileRelPath: ep.FileRelPath,
			Source:      src,
		})
	}
	return out, nil
}
