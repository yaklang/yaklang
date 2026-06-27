package loop_ssa_api_discovery

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func usesGranularFeaturePipeline(workDir string) bool {
	if workDir == "" {
		return false
	}
	_, err := loadFeatureInventory(workDir)
	return err == nil
}

// EnsureBusinessFunctionMapFromFeatureInventory backfills legacy contract artifact
// when runPhase1Redesigned produced feature_inventory but skipped business_function ReAct.
func EnsureBusinessFunctionMapFromFeatureInventory(rt *Runtime) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	if _, err := loadBusinessFunctionMap(rt.WorkDir); err == nil {
		return nil
	}
	inv, err := loadFeatureInventory(rt.WorkDir)
	if err != nil {
		return err
	}
	javaInv, _ := loadJavaBusinessScopeInventory(rt.WorkDir)
	m := &BusinessFunctionMap{
		SchemaVersion:          businessFunctionMapSchemaVersion,
		GeneratedAt:            time.Now().UTC().Format(time.RFC3339),
		Language:               rt.Session.Language,
		ClassificationStrategy: "feature_inventory_backfill",
		Functions:              map[string]BusinessFunctionEntry{},
	}
	for _, f := range inv.Features {
		entry := BusinessFunctionEntry{
			Description: f.Description,
			ScopePaths:  append([]string{}, f.PackagePatterns...),
		}
		if entry.Description == "" {
			entry.Description = f.Label
		}
		m.Functions[f.FeatureID] = entry
	}
	if inv.Coverage.Complete {
		m.Coverage = BusinessCoverageResult{
			Policy:        inv.Coverage.Policy,
			TotalRequired: inv.Coverage.TotalRequired,
			Covered:       inv.Coverage.Covered,
			Complete:      true,
		}
	} else if javaInv != nil {
		var allScope []string
		for _, f := range inv.Features {
			allScope = append(allScope, f.PackagePatterns...)
		}
		report := evaluateJavaBusinessCoverage(javaInv, allScope)
		m.Coverage = BusinessCoverageResult{
			Policy:        "controller_java_packages",
			TotalRequired: report.TotalRequired,
			Covered:       report.Covered,
			Complete:      report.Complete,
		}
	} else {
		m.Coverage = BusinessCoverageResult{
			Policy:        "feature_inventory",
			TotalRequired: len(inv.Features),
			Covered:       len(inv.Features),
			Complete:      true,
		}
	}
	if err := persistBusinessFunctionMap(rt, m); err != nil {
		return err
	}
	log.Infof("ssa_api_discovery: backfilled business_function_map from feature_inventory functions=%d", len(m.Functions))
	return nil
}

// EnsureForwardingProfileForContract writes forwarding_profile.json from routing_profile when missing.
func EnsureForwardingProfileForContract(rt *Runtime) error {
	if rt == nil {
		return utils.Error("nil runtime")
	}
	if _, err := os.Stat(store.ForwardingProfilePath(rt.WorkDir)); err == nil {
		return nil
	}
	if _, err := loadRoutingProfileFromWorkDir(rt.WorkDir); err != nil {
		if rt.Session != nil && rt.Session.RoutingProfileJSON != "" {
			_, err = writeForwardingProfileFromSession(rt)
			return err
		}
		return err
	}
	_, err := writeForwardingProfileFromSession(rt)
	return err
}

func collectFeatureApiMapRoutes(rt *Runtime) []FeatureApiEntry {
	if rt == nil {
		return nil
	}
	apiMap, err := loadFeatureApiMap(rt.WorkDir)
	if err != nil || apiMap == nil {
		return nil
	}
	var out []FeatureApiEntry
	for _, f := range apiMap.Features {
		if !f.Processed {
			continue
		}
		out = append(out, f.Apis...)
	}
	return out
}

// SyncFeatureApiMapToVerifiedHttpApis mirrors feature verify results into verified_http_apis
// so Phase1 gate/contract counts rejected routes as covered.
func SyncFeatureApiMapToVerifiedHttpApis(rt *Runtime) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	routes := collectFeatureApiMapRoutes(rt)
	if len(routes) == 0 {
		return nil
	}
	for _, a := range routes {
		if a.Method == "" || a.PathPattern == "" {
			continue
		}
		rejectReason := a.RejectReason
		if !a.Verified && rejectReason == "" {
			rejectReason = "feature_verify: not verified"
		}
		row := &store.VerifiedHttpApi{
			SessionID:     rt.Session.ID,
			Method:        a.Method,
			PathPattern:   a.PathPattern,
			HandlerFile:   a.HandlerFile,
			HandlerSymbol: a.HandlerSymbol,
			FullSampleURL: a.FullSampleURL,
			Verified:      a.Verified,
			RejectReason:  rejectReason,
			VerdictReason: a.VerdictReason,
			Source:        "feature_verify",
		}
		if row.VerdictReason == "" {
			if a.Verified {
				row.VerdictReason = "feature_verify: verified"
			} else {
				row.VerdictReason = rejectReason
			}
		}
		if err := rt.Repo.UpsertVerifiedHttpApi(row); err != nil {
			return err
		}
	}
	log.Infof("ssa_api_discovery: synced feature_api_map routes to verified_http_apis count=%d", len(routes))
	return nil
}

// EnsureCanonicalApiRoutePlan merges all http_endpoints into code_reading_plan so the Phase1 gate
// requires full API coverage, not only feature_api_map subsets.
func EnsureCanonicalApiRoutePlan(rt *Runtime) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	eps, err := rt.Repo.ListHttpEndpoints(rt.Session.ID)
	if err != nil {
		return err
	}
	if len(eps) == 0 {
		return nil
	}
	plan, _ := LoadCodeReadingPlan(rt.WorkDir)
	if plan == nil {
		plan = &CodeReadingPlan{}
	}
	seen := map[string]struct{}{}
	for _, a := range plan.DiscoveredAPIs {
		seen[routeKey(a.Method, a.PathPattern)] = struct{}{}
	}
	changed := false
	for _, e := range eps {
		if strings.TrimSpace(e.Method) == "" || strings.TrimSpace(e.PathPattern) == "" {
			continue
		}
		key := routeKey(e.Method, e.PathPattern)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		plan.DiscoveredAPIs = append(plan.DiscoveredAPIs, DiscoveredAPI{
			Method:        e.Method,
			PathPattern:   e.PathPattern,
			HandlerClass:  e.HandlerClass,
			HandlerSymbol: e.HandlerMethod,
			CodeEvidence:  e.Source + ":" + e.Status,
		})
		changed = true
	}
	if !changed && len(plan.DiscoveredAPIs) >= len(eps) {
		return nil
	}
	if strings.TrimSpace(plan.HintDiff) == "" {
		plan.HintDiff = "merged all http_endpoints for full Phase1 verification coverage"
	}
	if err := PersistCodeReadingPlan(rt, plan); err != nil {
		return err
	}
	log.Infof("ssa_api_discovery: canonical api route plan apis=%d http_endpoints=%d", len(plan.DiscoveredAPIs), len(eps))
	return nil
}

// RunPhase1FullApiVerificationGate checks unit artifacts and HTTP probe evidence for http_api routes.
func RunPhase1FullApiVerificationGate(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime) error {
	_ = ctx
	if rt == nil || rt.Session == nil {
		return nil
	}
	ensureGranularPhase1ContractArtifacts(rt)
	if err := verifyPhase1GranularGate(rt); err != nil {
		if invoker != nil {
			invoker.AddToTimeline("[ssa_pipeline]", "phase1_full_verify: gate_fail "+err.Error())
		}
		return &Phase1VerificationGateError{Reason: err.Error()}
	}
	return nil
}

// ensureGranularPhase1ContractArtifacts backfills artifacts required by legacy enforcePhase1Contract.
func ensureGranularPhase1ContractArtifacts(rt *Runtime) {
	if rt == nil || !usesGranularFeaturePipeline(rt.WorkDir) {
		return
	}
	if err := EnsureBusinessFunctionMapFromFeatureInventory(rt); err != nil {
		log.Warnf("ssa_api_discovery: ensure business_function_map: %v", err)
	}
	if err := EnsureForwardingProfileForContract(rt); err != nil {
		log.Warnf("ssa_api_discovery: ensure forwarding_profile: %v", err)
	}
	if err := SyncFeatureApiMapToVerifiedHttpApis(rt); err != nil {
		log.Warnf("ssa_api_discovery: sync feature_api_map to verified_http_apis: %v", err)
	}
}
