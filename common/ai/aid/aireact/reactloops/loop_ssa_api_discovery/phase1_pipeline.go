package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// runPhase1Redesigned executes Phase1 using the new unified endpoint extraction architecture.
// This replaces the old HarvestedEndpoint/http_endpoints model with APIEndpoint/unified_endpoints.
func runPhase1Redesigned(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime) error {
	start := time.Now()

	// Language reconciliation
	if _, _, recErr := ReconcileSessionLanguageFromMarkers(ctx, rt); recErr != nil {
		log.Warnf("ssa_api_discovery: phase1 language reconcile: %v", recErr)
	}

	// Stage 0: Project Profile (directory tree, file classification)
	{
		started := time.Now()
		rt.execStepStart("phase1.project_profile", "programmatic")
		if _, err := RunBuildProjectProfile(rt); err != nil {
			rt.execStepError("phase1.project_profile", "programmatic", started, err, nil)
			log.Warnf("ssa_api_discovery: stage0 project_profile: %v", err)
			r.AddToTimeline("[ssa_pipeline]", "stage0_project_profile: "+err.Error())
		} else {
			rt.execStepEnd("phase1.project_profile", "programmatic", started, []string{store.ProjectProfilePath(rt.WorkDir)})
		}
	}

	// Stage 1: Backend Scope
	{
		started := time.Now()
		rt.execStepStart("phase1.backend_scope", "programmatic")
		if _, err := RunBuildBackendScope(ctx, r, rt); err != nil {
			rt.execStepError("phase1.backend_scope", "programmatic", started, err, nil)
			log.Warnf("ssa_api_discovery: phase1 backend_scope: %v", err)
		} else {
			rt.execStepEnd("phase1.backend_scope", "programmatic", started, []string{store.BackendScopePath(rt.WorkDir)})
		}
	}

	// Stage 2: NEW - Unified Endpoint Extraction (replaces RunEndpointHarvestForSession)
	{
		started := time.Now()
		rt.execStepStart("phase1.unified_endpoint_extraction", "programmatic")
		if _, err := RunFullEndpointExtraction(rt); err != nil {
			rt.execStepError("phase1.unified_endpoint_extraction", "programmatic", started, err, nil)
			log.Warnf("ssa_api_discovery: unified endpoint extraction: %v", err)
			r.AddToTimeline("[ssa_pipeline]", "unified_endpoint_extraction: "+err.Error())
		} else {
			rt.execStepEnd("phase1.unified_endpoint_extraction", "programmatic", started, []string{store.UnifiedEndpointsPath(rt.WorkDir)})
			log.Infof("ssa_api_discovery: unified endpoint extraction completed")
		}
	}

	// Stage 2.6: Servlet routing map (multi-Dispatcher CMS / Java web apps)
	{
		started := time.Now()
		rt.execStepStart("phase1.servlet_routing_map", "programmatic")
		if _, err := RunBuildServletRoutingMap(rt); err != nil {
			rt.execStepError("phase1.servlet_routing_map", "programmatic", started, err, nil)
			log.Warnf("ssa_api_discovery: servlet routing map: %v", err)
		} else {
			rt.execStepEnd("phase1.servlet_routing_map", "programmatic", started, []string{store.ServletRoutingMapPath(rt.WorkDir)})
		}
	}

	// Stage 2.5: Filter code_unit_registry to only include endpoint-related files
	{
		started := time.Now()
		rt.execStepStart("phase1.filter_code_unit_registry", "programmatic")
		if _, err := FilterCodeUnitRegistryByEndpoints(rt); err != nil {
			rt.execStepError("phase1.filter_code_unit_registry", "programmatic", started, err, nil)
			log.Warnf("ssa_api_discovery: filter code unit registry: %v", err)
		} else {
			rt.execStepEnd("phase1.filter_code_unit_registry", "programmatic", started, []string{store.CodeUnitRegistryPath(rt.WorkDir)})
		}
	}

	// Stage 3: Build Java Business Scope (for Java projects)
	{
		started := time.Now()
		rt.execStepStart("phase1.java_business_scope", "programmatic")
		if _, err := BuildJavaBusinessScopeInventory(rt); err != nil {
			rt.execStepError("phase1.java_business_scope", "programmatic", started, err, nil)
			log.Warnf("ssa_api_discovery: java scope inventory: %v", err)
			r.AddToTimeline("[ssa_pipeline]", "java_scope_inventory: "+err.Error())
		} else {
			rt.execStepEnd("phase1.java_business_scope", "programmatic", started, []string{store.JavaBusinessScopeInventoryPath(rt.WorkDir)})
		}
	}

	// Stage 4: Build Code Unit Registry
	{
		started := time.Now()
		rt.execStepStart("phase1.code_unit_registry", "programmatic")
		if _, err := BuildCodeUnitRegistry(rt); err != nil {
			rt.execStepError("phase1.code_unit_registry", "programmatic", started, err, nil)
			log.Warnf("ssa_api_discovery: code unit registry: %v", err)
			r.AddToTimeline("[ssa_pipeline]", "code_unit_registry: "+err.Error())
		} else {
			rt.execStepEnd("phase1.code_unit_registry", "programmatic", started, []string{store.CodeUnitRegistryPath(rt.WorkDir)})
		}
	}

	// Write minimal prep bundle
	{
		started := time.Now()
		rt.execStepStart("phase1.prep_bundle", "programmatic")
		if err := writeMinimalPhase1PrepBundle(rt, "phase1_audit_prep"); err != nil {
			rt.execStepError("phase1.prep_bundle", "programmatic", started, err, nil)
			log.Warnf("ssa_api_discovery: phase1 prep bundle: %v", err)
		} else {
			rt.execStepEnd("phase1.prep_bundle", "programmatic", started, []string{store.Phase1PrepBundlePath(rt.WorkDir)})
		}
	}

	// ReAct Sub-loops (simplified for new architecture)

	// T1 — routing probe (using unified endpoints)
	{
		started := time.Now()
		rt.execStepStart("phase1.routing_probe", "ai")
		if err := runPhase1RoutingProbeReAct(ctx, r, task, rt); err != nil {
			rt.execStepError("phase1.routing_probe", "ai", started, err, nil)
			log.Warnf("ssa_api_discovery: T1 routing probe: %v", err)
		} else {
			rt.execStepEnd("phase1.routing_probe", "ai", started, []string{store.RoutingProfilePath(rt.WorkDir)})
		}
	}
	if err := verifyRoutingProbeGate(rt); err != nil {
		log.Warnf("ssa_api_discovery: routing gate: %v", err)
		r.AddToTimeline("[ssa_pipeline]", "routing_gate: "+err.Error())
	}

	// T2 — component/package map
	{
		started := time.Now()
		rt.execStepStart("phase1.component_map", "ai")
		if err := runPhase1ComponentMapReAct(ctx, r, task, rt); err != nil {
			rt.execStepError("phase1.component_map", "ai", started, err, nil)
			log.Warnf("ssa_api_discovery: T2 component map: %v", err)
		} else {
			rt.execStepEnd("phase1.component_map", "ai", started, []string{store.ComponentPackageMapPath(rt.WorkDir)})
		}
	}

	// P0 — project context summary (first-party vs third-party boundary before BFS)
	{
		started := time.Now()
		rt.execStepStart("phase1.project_context", "ai")
		if err := runPhase1ProjectContextReAct(ctx, r, task, rt); err != nil {
			rt.execStepError("phase1.project_context", "ai", started, err, nil)
			log.Warnf("ssa_api_discovery: P0 project context: %v", err)
			r.AddToTimeline("[ssa_pipeline]", "phase1_project_context: "+err.Error())
		} else {
			rt.execStepEnd("phase1.project_context", "ai", started, []string{store.ProjectContextSummaryPath(rt.WorkDir)})
		}
	}

	// F0/F1 — frontend/template API harvest + inventory
	{
		ok, skipReason := shouldRunFrontendAPIAnalysis(rt)
		if !ok {
			rt.execInfo("phase1.frontend_api", "programmatic", "skipped: "+skipReason)
			log.Infof("ssa_api_discovery: frontend_api skipped (%s)", skipReason)
		} else {
			hStart := time.Now()
			rt.execStepStart("phase1.frontend_api_harvest", "programmatic")
			if _, err := RunFrontendAPIHarvest(rt); err != nil {
				rt.execStepError("phase1.frontend_api_harvest", "programmatic", hStart, err, nil)
				log.Warnf("ssa_api_discovery: frontend_api_harvest: %v", err)
			} else {
				rt.execStepEnd("phase1.frontend_api_harvest", "programmatic", hStart, []string{store.FrontendAPIHarvestPath(rt.WorkDir)})
			}
			rStart := time.Now()
			rt.execStepStart("phase1.frontend_api_inventory", "ai")
			if err := runPhase1FrontendAPIReAct(ctx, r, task, rt); err != nil {
				rt.execStepError("phase1.frontend_api_inventory", "ai", rStart, err, nil)
				log.Warnf("ssa_api_discovery: frontend_api_inventory: %v", err)
			} else {
				rt.execStepEnd("phase1.frontend_api_inventory", "ai", rStart, []string{store.FrontendAPIInventoryPath(rt.WorkDir)})
			}
		}
	}


	// D — Directory Analysis (replaces F1: AI guesswork with programmatic BFS)
	skipDirAnalysis := shouldSkipDirectoryAnalysis(rt)
	if skipDirAnalysis {
		note := "skipped"
		if rt.SkipDirectoryAnalysis {
			note = "skipped by user input"
		} else {
			note = "skipped by YAK_SSA_SKIP_DIR_ANALYSIS=1"
		}
		rt.execInfo("phase1.directory_analysis", "programmatic", note)
		log.Warnf("ssa_api_discovery: D step skipped (%s); backfilling feature_inventory from code_unit_registry", note)
		r.AddToTimeline("[ssa_pipeline]", "directory_analysis: "+note)
		backfillFeatureInventoryAfterSkip(r, rt)
	} else {
		dirStart := time.Now()
		rt.execStepStart("phase1.directory_analysis", "ai+programmatic")
		tree, err := RunDirectoryAnalysis(ctx, r, task, rt)
		if err != nil {
			rt.execStepError("phase1.directory_analysis", "ai+programmatic", dirStart, err, nil)
			log.Errorf("ssa_api_discovery: D directory analysis failed: %v", err)
			r.AddToTimeline("[ssa_pipeline]", "directory_analysis: "+err.Error())
			return fmt.Errorf("D directory analysis failed: %w", err)
		}

		workStart := time.Now()
		rt.execStepStart("phase1.work_unit_estimation", "programmatic")
		workUnits, err := EstimateWorkUnits(tree)
		if err != nil {
			rt.execStepError("phase1.work_unit_estimation", "programmatic", workStart, err, nil)
			rt.execStepError("phase1.directory_analysis", "ai+programmatic", dirStart, err, nil)
			log.Errorf("ssa_api_discovery: W work unit estimation failed: %v", err)
			r.AddToTimeline("[ssa_pipeline]", "work_unit_estimation: "+err.Error())
			return fmt.Errorf("W work unit estimation failed: %w", err)
		}
		if len(workUnits) == 0 {
			err := fmt.Errorf("directory analysis produced 0 work units; ReAct BFS must identify business scope")
			rt.execStepError("phase1.work_unit_estimation", "programmatic", workStart, err, nil)
			rt.execStepError("phase1.directory_analysis", "ai+programmatic", dirStart, err, nil)
			log.Errorf("ssa_api_discovery: %v", err)
			r.AddToTimeline("[ssa_pipeline]", "directory_analysis: "+err.Error())
			return err
		}
		rt.execStepEnd("phase1.work_unit_estimation", "programmatic", workStart, nil)

		persistStart := time.Now()
		rt.execStepStart("phase1.persist_feature_inventory", "programmatic")
		if err := PersistFeatureInventory(rt, workUnits); err != nil {
			rt.execStepError("phase1.persist_feature_inventory", "programmatic", persistStart, err, nil)
			rt.execStepError("phase1.directory_analysis", "ai+programmatic", dirStart, err, nil)
			log.Errorf("ssa_api_discovery: persist feature inventory failed: %v", err)
			r.AddToTimeline("[ssa_pipeline]", "persist_feature_inventory: "+err.Error())
			return fmt.Errorf("persist feature inventory failed: %w", err)
		}
		rt.execStepEnd("phase1.persist_feature_inventory", "programmatic", persistStart, []string{store.FeatureInventoryPath(rt.WorkDir)})
		rt.execStepEnd("phase1.directory_analysis", "ai+programmatic", dirStart, []string{
			store.DirectoryAnalysisPath(rt.WorkDir),
			store.FeatureInventoryPath(rt.WorkDir),
		})

		log.Infof("ssa_api_discovery: feature_inventory persisted: %d work units", len(workUnits))
		r.AddToTimeline("[ssa_pipeline]", fmt.Sprintf("feature_inventory: %d work units", len(workUnits)))
	}

	// A1/A2/A3 — auth chain
	{
		started := time.Now()
		rt.execStepStart("phase1.auth_chain", "ai")
		if err := runPhase1AuthChain(ctx, r, task, rt); err != nil {
			rt.execStepError("phase1.auth_chain", "ai", started, err, nil)
			log.Warnf("ssa_api_discovery: auth chain: %v", err)
			r.AddToTimeline("[ssa_pipeline]", "phase1_auth_chain: "+err.Error())
		} else {
			rt.execStepEnd("phase1.auth_chain", "ai", started, []string{store.AuthSurfacePath(rt.WorkDir), store.AuthEvidencePath(rt.WorkDir)})
		}
	}

	// X1 — failure semantics
	{
		started := time.Now()
		rt.execStepStart("phase1.failure_semantics", "ai")
		if err := runPhase1FailureSemanticsReAct(ctx, r, task, rt); err != nil {
			rt.execStepError("phase1.failure_semantics", "ai", started, err, nil)
			log.Warnf("ssa_api_discovery: X1 failure semantics: %v", err)
		} else {
			rt.execStepEnd("phase1.failure_semantics", "ai", started, []string{store.FailureSemanticsPath(rt.WorkDir)})
		}
	}
	if err := verifyFailureSemanticsGate(rt); err != nil {
		log.Warnf("ssa_api_discovery: failure semantics gate: %v", err)
	}

	// C1 — auth calibration (hard gate when auth required + target reachable)
	if phase1AuthRequired(rt) && rt.Session.TargetReachable {
		calStart := time.Now()
		rt.execStepStart("phase1.auth_calibration", "ai")
		if err := runPhase1AuthCalibrationChain(ctx, r, task, rt); err != nil {
			if IsPhase1AuthFailed(err) {
				rt.execStepError("phase1.auth_calibration", "ai", calStart, err, []string{store.Phase1AuthFailureReportPath(rt.WorkDir)})
				_ = WritePhase1AuthFailureReport(rt, err.Error())
				_ = WritePhase1DiscoveryReport(rt)
				r.AddToTimeline("[ssa_pipeline]", "phase1_auth_calibration_failed: "+err.Error())
				return err
			}
			rt.execStepError("phase1.auth_calibration", "ai", calStart, err, nil)
			log.Warnf("ssa_api_discovery: C1 auth calibration: %v", err)
		}
		ready, reason := EvaluatePhase1AuthCalibrationReadiness(rt)
		if !ready {
			err := &Phase1AuthFailedError{Reason: reason}
			rt.execStepError("phase1.auth_calibration", "ai", calStart, err, []string{store.Phase1AuthFailureReportPath(rt.WorkDir)})
			_ = WritePhase1AuthFailureReport(rt, reason)
			_ = WritePhase1DiscoveryReport(rt)
			r.AddToTimeline("[ssa_pipeline]", "phase1_auth_calibration_failed: "+reason)
			return err
		}
		rt.execStepEnd("phase1.auth_calibration", "ai", calStart, []string{store.AuthCalibrationPath(rt.WorkDir)})
		r.AddToTimeline("[ssa_pipeline]", "phase1_auth_calibration: "+reason)
	} else if !phase1AuthRequired(rt) {
		_, _ = writeAuthState(rt, authStateNoAuthNeeded, "no auth surfaces required")
		rt.execInfo("phase1.auth_calibration", "programmatic", "skipped: no auth surfaces required")
	}

	// Write route candidates from unified endpoints
	{
		started := time.Now()
		rt.execStepStart("phase1.route_candidates", "programmatic")
		if _, err := writeRouteCandidatesFromUnifiedEndpoints(rt); err != nil {
			rt.execStepError("phase1.route_candidates", "programmatic", started, err, nil)
			log.Warnf("ssa_api_discovery: route_candidates: %v", err)
		} else {
			rt.execStepEnd("phase1.route_candidates", "programmatic", started, []string{store.RouteCandidatesPath(rt.WorkDir)})
		}
	}

	// Write final discovery report
	{
		started := time.Now()
		rt.execStepStart("phase1.discovery_report", "programmatic")
		if err := WritePhase1DiscoveryReport(rt); err != nil {
			rt.execStepError("phase1.discovery_report", "programmatic", started, err, nil)
			log.Warnf("ssa_api_discovery: discovery report: %v", err)
		} else {
			rt.execStepEnd("phase1.discovery_report", "programmatic", started, []string{store.Phase1DiscoveryReportPath(rt.WorkDir)})
		}
	}

	log.Infof("ssa_api_discovery: phase1 new architecture done in %s", time.Since(start))
	return nil
}

// endpointCountLow checks if the number of unified endpoints is too low.
// This replaces the old endpointCountLow which checked http_endpoints table.
func endpointCountLow(rt *Runtime) bool {
	if rt == nil {
		return true
	}

	// Load unified endpoints inventory
	inventory, err := loadUnifiedEndpointsInventory(rt.WorkDir)
	if err != nil {
		return true
	}

	return len(inventory.Endpoints) < 5
}

// loadUnifiedEndpointsInventory loads the unified endpoints from disk.
func loadUnifiedEndpointsInventory(workDir string) (*UnifiedEndpointsInventory, error) {
	path := store.UnifiedEndpointsPath(workDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var inventory UnifiedEndpointsInventory
	if err := json.Unmarshal(data, &inventory); err != nil {
		return nil, err
	}
	return &inventory, nil
}

// writeRouteCandidatesFromUnifiedEndpoints writes route candidates from unified endpoints.
// This replaces writeRouteCandidatesFromDB which used the old http_endpoints table.
func writeRouteCandidatesFromUnifiedEndpoints(rt *Runtime) (int, error) {
	if rt == nil {
		return 0, utils.Error("nil runtime")
	}

	inventory, err := loadUnifiedEndpointsInventory(rt.WorkDir)
	if err != nil {
		return 0, err
	}

	// Convert unified endpoints to route candidates format
	var candidates []RouteCandidate
	for _, ep := range inventory.Endpoints {
		candidate := RouteCandidate{
			Method:      strings.Join(ep.HTTPMethods, ","),
			Path:        ep.HTTPPath,
			ClassName:   ep.ClassName,
			MethodName:  ep.MethodName,
			Source:      string(ep.Provenance),
			Framework:   string(ep.Framework),
			AuthHint:    authHintFromRules(ep.AuthRequirements),
			Confidence:  ep.Confidence,
		}
		candidates = append(candidates, candidate)
	}

	// Write to file
	path := store.RouteCandidatesPath(rt.WorkDir)
	data, err := json.MarshalIndent(candidates, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := writeJSONFile(path, data); err != nil {
		return 0, err
	}

	return len(candidates), nil
}

// authHintFromRules converts auth rules to a hint string.
func authHintFromRules(rules []AuthRule) string {
	if len(rules) == 0 {
		return ""
	}
	var parts []string
	for _, r := range rules {
		if r.Type != "" {
			parts = append(parts, r.Type)
		}
		if len(r.Roles) > 0 {
			parts = append(parts, "roles:"+strings.Join(r.Roles, ","))
		}
	}
	return strings.Join(parts, "; ")
}

// RouteCandidate represents a discovered route candidate.
type RouteCandidate struct {
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	ClassName  string  `json:"class_name"`
	MethodName string  `json:"method_name"`
	Source     string  `json:"source"`
	Framework  string  `json:"framework"`
	AuthHint   string  `json:"auth_hint"`
	Confidence float64 `json:"confidence"`
}
