package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// DiscoveredAPI is one AI code-reading route entry in code_reading_plan.json.
type DiscoveredAPI struct {
	Method         string `json:"method"`
	PathPattern    string `json:"path_pattern"`
	HandlerFile    string `json:"handler_file"`
	HandlerSymbol  string `json:"handler_symbol"`
	ClassBasePath  string `json:"class_base_path,omitempty"`
	CodeEvidence   string `json:"code_evidence,omitempty"`
	HandlerClass   string `json:"handler_class,omitempty"`
	QueryParamsJSON string `json:"query_params_json,omitempty"`
	BodyHintJSON   string `json:"body_hint_json,omitempty"`
}

// CodeReadingPlan is the structured output of Phase1B.
type CodeReadingPlan struct {
	DiscoveredAPIs     []DiscoveredAPI      `json:"discovered_apis"`
	ReadFilesCompleted []string             `json:"read_files_completed"`
	EffectiveBases     []string             `json:"effective_bases"`
	URLSpaces          map[string]any       `json:"url_spaces"`
	HintDiff           string               `json:"hint_diff,omitempty"`
	AuthNotes          string               `json:"auth_notes,omitempty"`
	AuthEvidence       *AuthEvidenceRecord  `json:"auth_evidence,omitempty"`
	ReadQueue          []string             `json:"read_queue,omitempty"`
	ForwardChains      []any                `json:"forward_chains,omitempty"`
	Conflicts          []any                `json:"conflicts,omitempty"`
}

// SyncAICodeReadingRoutesToEndpoints reads code_reading_plan.json and writes ai_code_read endpoints.
func SyncAICodeReadingRoutesToEndpoints(rt *Runtime) (int, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0, utils.Error("nil runtime")
	}
	plan, err := LoadCodeReadingPlan(rt.WorkDir)
	if err != nil {
		return 0, err
	}
	if len(plan.DiscoveredAPIs) == 0 {
		return 0, utils.Error("code_reading_plan.discovered_apis is empty; AI code reading must produce API catalog")
	}

	inserted := 0
	for _, api := range plan.DiscoveredAPIs {
		method := strings.ToUpper(strings.TrimSpace(api.Method))
		path := normURLPath(api.PathPattern)
		if method == "" || path == "" {
			continue
		}
		row := &store.HttpEndpoint{
			SessionID:     rt.Session.ID,
			Method:        method,
			PathPattern:   path,
			HandlerClass:  strings.TrimSpace(api.HandlerClass),
			HandlerMethod: strings.TrimSpace(api.HandlerSymbol),
			Source:        SourceAICodeRead,
			Status:        store.EndpointStatusPendingValidation,
		}
		if row.HandlerClass == "" && api.HandlerFile != "" {
			row.HandlerClass = api.HandlerFile
		}
		res, gerr := EndpointInsertionGateway(rt, row)
		if gerr != nil {
			return inserted, gerr
		}
		if res != nil && res.Reason == "created" {
			inserted++
		}
	}

	meta := map[string]any{
		"tool":              "ai_code_read_sync",
		"discovered_count":  len(plan.DiscoveredAPIs),
		"inserted_count":    inserted,
		"full_report_path":  store.CodeReadingPlanPath(rt.WorkDir),
	}
	b, _ := json.Marshal(meta)
	rt.Session.CodeReadingRoutesMetaJSON = string(b)
	if err := rt.Repo.UpdateSession(rt.Session); err != nil {
		log.Warnf("ssa_api_discovery: update code reading routes meta: %v", err)
	}
	if _, err := writeRouteCandidatesFromDB(rt); err != nil {
		return inserted, err
	}
	log.Infof("ssa_api_discovery: synced ai_code_read routes inserted=%d total=%d", inserted, len(plan.DiscoveredAPIs))
	return inserted, nil
}

// LoadCodeReadingPlan loads and parses code_reading_plan.json.
func LoadCodeReadingPlan(workDir string) (*CodeReadingPlan, error) {
	b, err := os.ReadFile(store.CodeReadingPlanPath(workDir))
	if err != nil {
		return nil, err
	}
	var plan CodeReadingPlan
	if err := json.Unmarshal(b, &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

// LookupDiscoveredAPI finds a discovered API by method+path.
func LookupDiscoveredAPI(plan *CodeReadingPlan, method, pathPattern string) *DiscoveredAPI {
	if plan == nil {
		return nil
	}
	key := routeKey(method, pathPattern)
	for i := range plan.DiscoveredAPIs {
		api := &plan.DiscoveredAPIs[i]
		if routeKey(api.Method, api.PathPattern) == key {
			return api
		}
	}
	return nil
}

// CountAICodeReadEndpoints returns endpoints with source ai_code_read.
func CountAICodeReadEndpoints(rt *Runtime) (int, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0, nil
	}
	eps, err := rt.Repo.ListHttpEndpoints(rt.Session.ID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range eps {
		if e.Source == SourceAICodeRead {
			n++
		}
	}
	return n, nil
}
