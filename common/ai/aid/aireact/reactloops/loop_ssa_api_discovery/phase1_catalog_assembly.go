package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const apiCatalogSchemaVersion = 1

// ApiCatalogV1 full assembled API catalog after staged code reading.
type ApiCatalogV1 struct {
	SchemaVersion  int              `json:"schema_version"`
	GeneratedAt    string           `json:"generated_at"`
	ContextPath    string           `json:"context_path"`
	AssemblyBasis  string           `json:"assembly_basis"`
	Entries        []ApiCatalogEntry `json:"entries"`
	Stats          map[string]int   `json:"stats"`
}

// ApiCatalogEntry one API with assembled URL and evidence.
type ApiCatalogEntry struct {
	Method          string  `json:"method"`
	PathPattern     string  `json:"path_pattern"`
	FullURL         string  `json:"full_url"`
	EffectiveBase   string  `json:"effective_base,omitempty"`
	MountPrefix     string  `json:"mount_prefix,omitempty"`
	HandlerFile     string  `json:"handler_file,omitempty"`
	HandlerSymbol   string  `json:"handler_symbol,omitempty"`
	HandlerClass    string  `json:"handler_class,omitempty"`
	AuthRequired    bool    `json:"auth_required"`
	AuthHint        string  `json:"auth_hint,omitempty"`
	CodeEvidence    string  `json:"code_evidence"`
	AssemblyBasis   string  `json:"assembly_basis"`
	PathConfidence  float64 `json:"path_confidence"`
	QueryParamsJSON string  `json:"query_params_json,omitempty"`
	BodyHintJSON    string  `json:"body_hint_json,omitempty"`
}

// AssembleApiCatalogFromStages merges code_reading_plan + routing profile into api_catalog.json.
func AssembleApiCatalogFromStages(rt *Runtime) (*ApiCatalogV1, error) {
	if rt == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	plan, err := LoadCodeReadingPlan(rt.WorkDir)
	if err != nil {
		return nil, err
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
		ctxSource = "project_profile:" + profile.ContextPathSrc
	} else if rp != nil && strings.TrimSpace(rp.Target.ContextPath) != "" && rp.Target.ContextPath != "unknown" {
		ctxPath = normURLPath(rp.Target.ContextPath)
		ctxSource = "routing_profile"
	} else {
		ctxPath = ""
	}

	assemblyBasis := "evidence"
	pathConf := 0.85
	if ctxSource == "default" || ctxPath == "" {
		assemblyBasis = "inferred"
		pathConf = 0.4
	}

	authRequiredDefault := hasSecurityConfigEntry(profile)
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
			QueryParamsJSON: api.QueryParamsJSON,
			BodyHintJSON:   api.BodyHintJSON,
		}
		catalog.Entries = append(catalog.Entries, entry)
		catalog.Stats["total"]++
	}

	if len(catalog.Entries) == 0 {
		return catalog, utils.Error("api catalog empty: no discovered_apis")
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
	log.Infof("ssa_api_discovery: api_catalog assembled entries=%d basis=%s", len(catalog.Entries), assemblyBasis)
	return catalog, nil
}

func mountPrefixFromEvidence(api DiscoveredAPI, rp *RoutingProfileV1) string {
	if bp := strings.TrimSpace(api.ClassBasePath); bp != "" {
		return normURLPath(bp)
	}
	if rp != nil {
		for _, sp := range rp.URLSpaces {
			if mp := strings.TrimSpace(sp.MountPrefix); mp != "" {
				return normURLPath(mp)
			}
		}
	}
	return ""
}

func joinCatalogPath(ctxPath, mount, pathPat string) string {
	parts := []string{}
	for _, p := range []string{ctxPath, mount, pathPat} {
		p = strings.TrimSpace(p)
		if p == "" || p == "/" || strings.EqualFold(p, "unknown") {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		parts = append(parts, strings.TrimSuffix(p, "/"))
	}
	if len(parts) == 0 {
		return normURLPath(pathPat)
	}
	out := strings.Join(parts, "")
	out = collapseDuplicatePathSegments(out)
	return out
}

// catalogReadyForProbe returns false when catalog URLs are likely invalid (unknown context, no entries).
func catalogReadyForProbe(catalog *ApiCatalogV1) (bool, string) {
	if catalog == nil || len(catalog.Entries) == 0 {
		return false, "empty catalog"
	}
	bad := 0
	for _, e := range catalog.Entries {
		if strings.Contains(e.FullURL, "/unknown/") || strings.Contains(e.FullURL, "/unknown") {
			bad++
		}
	}
	if bad > len(catalog.Entries)/2 {
		return false, fmt.Sprintf("%d/%d catalog URLs contain unknown context path", bad, len(catalog.Entries))
	}
	if strings.EqualFold(strings.TrimSpace(catalog.ContextPath), "unknown") && catalog.AssemblyBasis == "inferred" {
		return false, "context_path unknown and assembly inferred"
	}
	return true, ""
}

func hasSecurityConfigEntry(p *ProjectProfileV1) bool {
	if p == nil {
		return false
	}
	for _, ep := range p.EntryPoints {
		if ep.Kind == "security_config" {
			return true
		}
	}
	return false
}

func authHintFromProfile(p *ProjectProfileV1) string {
	if p == nil {
		return ""
	}
	for _, ep := range p.EntryPoints {
		if ep.Kind == "security_config" {
			return fmt.Sprintf("security config at %s", ep.RelPath)
		}
	}
	return ""
}

func loadApiCatalog(workDir string) (*ApiCatalogV1, error) {
	b, err := os.ReadFile(store.ApiCatalogPath(workDir))
	if err != nil {
		return nil, err
	}
	var c ApiCatalogV1
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
