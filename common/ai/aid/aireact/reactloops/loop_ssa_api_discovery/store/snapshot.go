package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DiscoverySnapshotPath returns workDir/ssa_discovery/discovery_snapshot.json
func DiscoverySnapshotPath(workDir string) string {
	return filepath.Join(workDir, subDir, "discovery_snapshot.json")
}

// RoutingProfilePath returns workDir/ssa_discovery/routing_profile.json
func RoutingProfilePath(workDir string) string {
	return filepath.Join(workDir, subDir, "routing_profile.json")
}

// ApiPreanalysisReportPath returns workDir/ssa_discovery/api_preanalysis.json
func ApiPreanalysisReportPath(workDir string) string {
	return filepath.Join(workDir, subDir, "api_preanalysis.json")
}

// ApiBaseCalibrationReportPath returns workDir/ssa_discovery/api_base_calibration.json
func ApiBaseCalibrationReportPath(workDir string) string {
	return filepath.Join(workDir, subDir, "api_base_calibration.json")
}

// ApiRouteHarvestReportPath returns workDir/ssa_discovery/api_route_harvest.json
func ApiRouteHarvestReportPath(workDir string) string {
	return filepath.Join(workDir, subDir, "api_route_harvest.json")
}

// ApiSpecImportReportPath returns workDir/ssa_discovery/api_spec_import.json
func ApiSpecImportReportPath(workDir string) string {
	return filepath.Join(workDir, subDir, "api_spec_import.json")
}

// Phase1PrepBundlePath returns workDir/ssa_discovery/phase1_prep_bundle.json
func Phase1PrepBundlePath(workDir string) string {
	return filepath.Join(workDir, subDir, "phase1_prep_bundle.json")
}

// RouteCandidatesPath returns workDir/ssa_discovery/route_candidates.json
func RouteCandidatesPath(workDir string) string {
	return filepath.Join(workDir, subDir, "route_candidates.json")
}

// AuthSurfacePath returns workDir/ssa_discovery/auth_surface.json
func AuthSurfacePath(workDir string) string {
	return filepath.Join(workDir, subDir, "auth_surface.json")
}

// DependenciesInventoryPath returns workDir/ssa_discovery/dependencies.json
func DependenciesInventoryPath(workDir string) string {
	return filepath.Join(workDir, subDir, "dependencies.json")
}

// ForwardingProfilePath returns workDir/ssa_discovery/forwarding_profile.json
func ForwardingProfilePath(workDir string) string {
	return filepath.Join(workDir, subDir, "forwarding_profile.json")
}

// CodeReadingPlanPath returns workDir/ssa_discovery/code_reading_plan.json
func CodeReadingPlanPath(workDir string) string {
	return filepath.Join(workDir, subDir, "code_reading_plan.json")
}

// StaticRouteHintsPath returns workDir/ssa_discovery/static_route_hints.json
func StaticRouteHintsPath(workDir string) string {
	return filepath.Join(workDir, subDir, "static_route_hints.json")
}

// BackendScopePath returns workDir/ssa_discovery/backend_scope.json
func BackendScopePath(workDir string) string {
	return filepath.Join(workDir, subDir, "backend_scope.json")
}

// CodeReadingPlanBuildMetaPath returns workDir/ssa_discovery/code_reading_plan_build_meta.json
func CodeReadingPlanBuildMetaPath(workDir string) string {
	return filepath.Join(workDir, subDir, "code_reading_plan_build_meta.json")
}

// ApiArchMapPath returns workDir/ssa_discovery/api_arch_map.json
func ApiArchMapPath(workDir string) string {
	return filepath.Join(workDir, subDir, "api_arch_map.json")
}

// ApiArchTestEvalPath returns workDir/ssa_discovery/api_arch_test_eval.json
func ApiArchTestEvalPath(workDir string) string {
	return filepath.Join(workDir, subDir, "api_arch_test_eval.json")
}

// SyntaxflowSummaryPath returns workDir/ssa_discovery/syntaxflow_summary.json
func SyntaxflowSummaryPath(workDir string) string {
	return filepath.Join(workDir, subDir, "syntaxflow_summary.json")
}

// VulnChecklistPath returns workDir/ssa_discovery/vuln_checklist.json
func VulnChecklistPath(workDir string) string {
	return filepath.Join(workDir, subDir, "vuln_checklist.json")
}

// ProjectProfilePath returns workDir/ssa_discovery/project_profile.json
func ProjectProfilePath(workDir string) string {
	return filepath.Join(workDir, subDir, "project_profile.json")
}

// CodeReadingStagePath returns workDir/ssa_discovery/code_reading_stage_<n>.json
func CodeReadingStagePath(workDir string, stage int) string {
	return filepath.Join(workDir, subDir, fmt.Sprintf("code_reading_stage_%d.json", stage))
}

// ApiCatalogPath returns workDir/ssa_discovery/api_catalog.json
func ApiCatalogPath(workDir string) string {
	return filepath.Join(workDir, subDir, "api_catalog.json")
}

// Phase1DiscoveryReportPath returns workDir/ssa_discovery/phase1_discovery_report.md
func Phase1DiscoveryReportPath(workDir string) string {
	return filepath.Join(workDir, subDir, "phase1_discovery_report.md")
}

// AuthStatePath returns workDir/ssa_discovery/auth_state.json
func AuthStatePath(workDir string) string {
	return filepath.Join(workDir, subDir, "auth_state.json")
}

// Phase1ReconPath returns workDir/ssa_discovery/phase1_recon.json
func Phase1ReconPath(workDir string) string {
	return filepath.Join(workDir, subDir, "phase1_recon.json")
}

// AuthEvidencePath returns workDir/ssa_discovery/auth_evidence.json
func AuthEvidencePath(workDir string) string {
	return filepath.Join(workDir, subDir, "auth_evidence.json")
}

// Phase1AuthFailureReportPath returns workDir/ssa_discovery/phase1_auth_failure_report.json
func Phase1AuthFailureReportPath(workDir string) string {
	return filepath.Join(workDir, subDir, "phase1_auth_failure_report.json")
}

// Phase1AuthFailureReportMDPath returns workDir/ssa_discovery/phase1_auth_failure_report.md
func Phase1AuthFailureReportMDPath(workDir string) string {
	return filepath.Join(workDir, subDir, "phase1_auth_failure_report.md")
}

// JavaBusinessScopeInventoryPath returns workDir/ssa_discovery/java_business_scope_inventory.json
func JavaBusinessScopeInventoryPath(workDir string) string {
	return filepath.Join(workDir, subDir, "java_business_scope_inventory.json")
}

// CodeScopeInventoryPath returns workDir/ssa_discovery/code_scope_inventory.json (non-Java fallback)
func CodeScopeInventoryPath(workDir string) string {
	return filepath.Join(workDir, subDir, "code_scope_inventory.json")
}

// TechArchitecturePath returns workDir/ssa_discovery/tech_architecture.json
func TechArchitecturePath(workDir string) string {
	return filepath.Join(workDir, subDir, "tech_architecture.json")
}

// BusinessFunctionMapPath returns workDir/ssa_discovery/business_function_map.json
func BusinessFunctionMapPath(workDir string) string {
	return filepath.Join(workDir, subDir, "business_function_map.json")
}

// FeatureInventoryPath returns workDir/ssa_discovery/feature_inventory.json
func FeatureInventoryPath(workDir string) string {
	return filepath.Join(workDir, subDir, "feature_inventory.json")
}

// AuthSurfaceMapPath returns workDir/ssa_discovery/auth_surface_map.json
func AuthSurfaceMapPath(workDir string) string {
	return filepath.Join(workDir, subDir, "auth_surface_map.json")
}

// AuthRealmInventoryPath returns workDir/ssa_discovery/auth_realm_inventory.json
func AuthRealmInventoryPath(workDir string) string {
	return filepath.Join(workDir, subDir, "auth_realm_inventory.json")
}

// AuthMechanismMapPath returns workDir/ssa_discovery/auth_mechanism_map.json
func AuthMechanismMapPath(workDir string) string {
	return filepath.Join(workDir, subDir, "auth_mechanism_map.json")
}

// FailureSemanticsPath returns workDir/ssa_discovery/failure_semantics.json
func FailureSemanticsPath(workDir string) string {
	return filepath.Join(workDir, subDir, "failure_semantics.json")
}

// AuthCsrfTokensPath returns workDir/ssa_discovery/auth_csrf_tokens.json
func AuthCsrfTokensPath(workDir string) string {
	return filepath.Join(workDir, subDir, "auth_csrf_tokens.json")
}


// FrontendAPIHarvestPath returns workDir/ssa_discovery/frontend_api_harvest.json
func FrontendAPIHarvestPath(workDir string) string {
	return filepath.Join(workDir, subDir, "frontend_api_harvest.json")
}

// FrontendAPIInventoryPath returns workDir/ssa_discovery/frontend_api_inventory.json
func FrontendAPIInventoryPath(workDir string) string {
	return filepath.Join(workDir, subDir, "frontend_api_inventory.json")
}

// CombinedAPICatalogPath returns workDir/ssa_discovery/combined_api_catalog.json
func CombinedAPICatalogPath(workDir string) string {
	return filepath.Join(workDir, subDir, "combined_api_catalog.json")
}

// FrameworkToolkitSelectionPath returns workDir/ssa_discovery/framework_toolkit_selection.json
func FrameworkToolkitSelectionPath(workDir string) string {
	return filepath.Join(workDir, subDir, "framework_toolkit_selection.json")
}

// ServletRoutingMapPath returns workDir/ssa_discovery/servlet_routing_map.json
func ServletRoutingMapPath(workDir string) string {
	return filepath.Join(workDir, subDir, "servlet_routing_map.json")
}

// AuthCalibrationPath returns workDir/ssa_discovery/auth_calibration.json
func AuthCalibrationPath(workDir string) string {
	return filepath.Join(workDir, subDir, "auth_calibration.json")
}

// FeatureApiMapPath returns workDir/ssa_discovery/feature_api_map.json
func FeatureApiMapPath(workDir string) string {
	return filepath.Join(workDir, subDir, "feature_api_map.json")
}

// CodeUnitRegistryPath returns workDir/ssa_discovery/code_unit_registry.json
func CodeUnitRegistryPath(workDir string) string {
	return filepath.Join(workDir, subDir, "code_unit_registry.json")
}

// FeatureCodeAnalysisMapPath returns workDir/ssa_discovery/feature_code_analysis_map.json
func FeatureCodeAnalysisMapPath(workDir string) string {
	return filepath.Join(workDir, subDir, "feature_code_analysis_map.json")
}

// FeatureWorkProgressPath returns workDir/ssa_discovery/feature_work_progress.json
func FeatureWorkProgressPath(workDir string) string {
	return filepath.Join(workDir, subDir, "feature_work_progress.json")
}

// CoverageSignalPath returns workDir/ssa_discovery/coverage_signal.json
func CoverageSignalPath(workDir string) string {
	return filepath.Join(workDir, subDir, "coverage_signal.json")
}

// ComponentPackageMapPath returns workDir/ssa_discovery/component_package_map.json
func ComponentPackageMapPath(workDir string) string {
	return filepath.Join(workDir, subDir, "component_package_map.json")
}

// ProjectContextSummaryPath returns workDir/ssa_discovery/project_context_summary.json
func ProjectContextSummaryPath(workDir string) string {
	return filepath.Join(workDir, subDir, "project_context_summary.json")
}

// JavaFrameworkDetectorPath returns workDir/ssa_discovery/java_framework_detector.json
func JavaFrameworkDetectorPath(workDir string) string {
	return filepath.Join(workDir, subDir, "java_framework_detector.json")
}

// DirectoryAnalysisPath returns workDir/ssa_discovery/directory_analysis.json
func DirectoryAnalysisPath(workDir string) string {
	return filepath.Join(workDir, subDir, "directory_analysis.json")
}

// DirectoryAnalysisNodePath returns workDir/ssa_discovery/directory_analysis/{nodeID}.json
func DirectoryAnalysisNodePath(workDir string, nodeID string) string {
	return filepath.Join(workDir, subDir, "directory_analysis", nodeID+".json")
}

// UnifiedEndpointsPath returns workDir/ssa_discovery/unified_endpoints.json
func UnifiedEndpointsPath(workDir string) string {
	return filepath.Join(workDir, subDir, "unified_endpoints.json")
}

// ValidationReportPath returns workDir/ssa_discovery/validation_report.json
func ValidationReportPath(workDir string) string {
	return filepath.Join(workDir, subDir, "validation_report.json")
}

// WriteDiscoverySnapshot writes a JSON bundle for report phases.
func WriteDiscoverySnapshot(
	path string,
	session *DiscoverySession,
	endpoints []HttpEndpoint,
	components []ArchitectureComponent,
	security []SecurityMechanism,
	verified []VerifiedEndpoint,
	verifiedHTTP []VerifiedHttpApi,
	sf []DiscoverySyntaxFlowFinding,
	vuln []VulnVerification,
	creds []AuthCredential,
	dynFindings []DynamicVulnFinding,
	checklist []VulnChecklistItem,
	coverage []CoverageWorkItem,
	events []DiscoveryEvent,
	valAttempts []EndpointValidationAttempt,
	recipes []AuthAcquisitionRecipe,
	configArts []ConfigArtifact,
	deps []DependencyRef,
	bizCaps []BusinessCapability,
	artifacts []PhaseArtifact,
) error {
	payload := map[string]any{
		"session":                      session,
		"http_endpoints":               endpoints,
		"components":                   components,
		"security_mechanisms":          security,
		"verified_endpoints":           verified,
		"verified_http_apis":           verifiedHTTP,
		"syntaxflow_findings":          sf,
		"vuln_verifications":           vuln,
		"auth_credentials":             creds,
		"dynamic_vuln_findings":        dynFindings,
		"vuln_checklist_items":         checklist,
		"coverage_work_items":          coverage,
		"discovery_events":             events,
		"endpoint_validation_attempts": valAttempts,
		"auth_acquisition_recipes":     recipes,
		"config_artifacts":             configArts,
		"dependency_refs":              deps,
		"business_capabilities":        bizCaps,
		"phase_artifacts":              artifactSummaries(artifacts),
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return os.WriteFile(path, b, 0o644)
}

func artifactSummaries(artifacts []PhaseArtifact) []map[string]any {
	out := make([]map[string]any, 0, len(artifacts))
	for _, a := range artifacts {
		out = append(out, map[string]any{
			"id":           a.ID,
			"kind":         a.Kind,
			"version":      a.Version,
			"payload_size": len(a.PayloadJSON),
		})
	}
	return out
}
