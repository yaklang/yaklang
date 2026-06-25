package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// endpointSourcePriority returns higher for preferred catalog sources.
func endpointSourcePriority(source string) int {
	s := strings.ToLower(strings.TrimSpace(source))
	switch s {
	case SourceAICodeRead, SourceExtractSpring:
		return 3
	case "ai", "ai_probe":
		return 2
	case SourceStaticHint, sourceStaticHintY:
		return 1
	default:
		return 2
	}
}

// BuildCodeReadingPlanFromDB builds discovered_apis from http_endpoints (extract + AI sources first).
func BuildCodeReadingPlanFromDB(rt *Runtime) (*CodeReadingPlan, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	eps, err := rt.Repo.ListHttpEndpoints(rt.Session.ID)
	if err != nil {
		return nil, err
	}
	seen := map[string]store.HttpEndpoint{}
	for _, e := range eps {
		key := strings.ToUpper(strings.TrimSpace(e.Method)) + " " + strings.TrimSpace(e.PathPattern)
		ex, ok := seen[key]
		if !ok || endpointSourcePriority(e.Source) > endpointSourcePriority(ex.Source) {
			seen[key] = e
		}
	}
	var apis []DiscoveredAPI
	var staticFallback []DiscoveredAPI
	for _, e := range seen {
		api := DiscoveredAPI{
			Method:        e.Method,
			PathPattern:   e.PathPattern,
			HandlerClass:  e.HandlerClass,
			HandlerSymbol: e.HandlerMethod,
			CodeEvidence:  e.Source + ":" + e.Status,
		}
		if endpointSourcePriority(e.Source) < 2 && len(seen) > 20 {
			staticFallback = append(staticFallback, api)
			continue
		}
		apis = append(apis, api)
	}
	if len(apis) == 0 && len(staticFallback) > 0 {
		apis = staticFallback
	}
	if len(apis) == 0 {
		return nil, utils.Error("no http_endpoints in DB for code reading plan")
	}
	profile, _ := loadProjectProfile(rt.WorkDir)
	ctx := "/"
	if profile != nil && profile.ContextPath != "" && profile.ContextPath != "unknown" {
		ctx = normURLPath(profile.ContextPath)
	}
	return &CodeReadingPlan{
		DiscoveredAPIs: apis,
		HintDiff:       "built from DB http_endpoints (extract_spring + ai_code_read)",
		URLSpaces:      map[string]any{"default": map[string]any{"base_path": ctx}},
		EffectiveBases: []string{ctx},
	}, nil
}

// AssembleApiCatalogFromDB builds api_catalog.json primarily from http_endpoints in SQLite.
func AssembleApiCatalogFromDB(rt *Runtime) (*ApiCatalogV1, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	plan, err := BuildCodeReadingPlanFromDB(rt)
	if err != nil {
		return AssembleApiCatalogFromStages(rt)
	}
	profile, _ := loadProjectProfile(rt.WorkDir)
	rp, _ := RoutingProfileFromSession(rt.Session)
	if rp == nil {
		if b, rerr := os.ReadFile(store.RoutingProfilePath(rt.WorkDir)); rerr == nil {
			_ = json.Unmarshal(b, &rp)
		}
	}
	ctxPath := "/"
	ctxSource := "default"
	if profile != nil && profile.ContextPath != "" && profile.ContextPath != "unknown" {
		ctxPath = normURLPath(profile.ContextPath)
		ctxSource = "project_profile"
	} else if rp != nil && strings.TrimSpace(rp.Target.ContextPath) != "" && rp.Target.ContextPath != "unknown" {
		ctxPath = normURLPath(rp.Target.ContextPath)
		ctxSource = "routing_profile"
	}
	assemblyBasis := "db_endpoints"
	pathConf := 0.85
	if ctxSource == "default" {
		assemblyBasis = "inferred"
		pathConf = 0.55
	}
	authRequiredDefault := hasSecurityConfigEntry(profile) || authVerifiedFromRuntime(rt)
	catalog := &ApiCatalogV1{
		SchemaVersion: apiCatalogSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		ContextPath:   ctxPath,
		AssemblyBasis: assemblyBasis,
		Stats:         map[string]int{},
	}
	baseURL := EffectiveTargetBaseURL(rt.Session)
	for _, api := range plan.DiscoveredAPIs {
		method := strings.ToUpper(strings.TrimSpace(api.Method))
		pathPat := normURLPath(api.PathPattern)
		if method == "" || pathPat == "" {
			continue
		}
		mount := mountPrefixFromEvidence(api, rp)
		fullPath := joinCatalogPath(ctxPath, mount, pathPat)
		fullURL := fullPath
		if baseURL != "" {
			fullURL = strings.TrimRight(baseURL, "/") + fullPath
		}
		entry := ApiCatalogEntry{
			Method:         method,
			PathPattern:    pathPat,
			FullURL:        fullURL,
			EffectiveBase:  ctxPath,
			MountPrefix:    mount,
			HandlerFile:    api.HandlerFile,
			HandlerSymbol:  api.HandlerSymbol,
			HandlerClass:   api.HandlerClass,
			AuthRequired:   authRequiredDefault,
			AuthHint:       authHintFromProfile(profile),
			CodeEvidence:   api.CodeEvidence,
			AssemblyBasis:  assemblyBasis,
			PathConfidence: pathConf,
		}
		catalog.Entries = append(catalog.Entries, entry)
		catalog.Stats["total"]++
	}
	if len(catalog.Entries) == 0 {
		return catalog, utils.Error("api catalog empty from DB")
	}
	path := store.ApiCatalogPath(rt.WorkDir)
	b, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return catalog, err
	}
	if err := writeJSONFile(path, b); err != nil {
		return catalog, err
	}
	if rt.Repo != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactApiCatalog, string(b))
	}
	log.Infof("ssa_api_discovery: api_catalog from DB entries=%d", len(catalog.Entries))
	return catalog, nil
}
