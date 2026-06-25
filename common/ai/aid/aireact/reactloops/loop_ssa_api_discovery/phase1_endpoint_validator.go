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

// EndpointValidator is the AI agent that validates endpoints and finds security issues.
type EndpointValidator struct {
	Endpoints   []APIEndpoint
	Findings    []SecurityFinding
	SessionID   string
	CodeRoot    string
}

// NewEndpointValidator creates a new endpoint validator.
func NewEndpointValidator(endpoints []APIEndpoint, sessionID, codeRoot string) *EndpointValidator {
	return &EndpointValidator{
		Endpoints:  endpoints,
		Findings:   []SecurityFinding{},
		SessionID: sessionID,
		CodeRoot:  codeRoot,
	}
}

// Validate runs validation checks on all endpoints.
func (v *EndpointValidator) Validate() *ValidationReport {
	report := NewValidationReport(v.SessionID)
	report.EndpointsTotal = len(v.Endpoints)
	report.EndpointsValidated = len(v.Endpoints)

	// Run all validation checks
	v.checkMissingAuthentication()
	v.checkInsecureDirectObjectReference()
	v.checkMissingInputValidation()
	v.checkSensitiveDataExposure()
	v.checkCSRFProtection()
	v.checkMassAssignment()
	v.checkidor()

	report.Findings = v.Findings
	report.ComputeStats()

	return report
}

// checkMissingAuthentication checks for endpoints without authentication.
func (v *EndpointValidator) checkMissingAuthentication() {
	for _, ep := range v.Endpoints {
		// Skip if auth is explicitly required
		if ep.AuthRequired {
			continue
		}

		// Check if endpoint handles sensitive operations
		sensitivePaths := []string{
			"/admin", "/user", "/profile", "/password", "/account",
			"/settings", "/api", "/dashboard", "/config",
		}

		for _, sp := range sensitivePaths {
			if strings.Contains(strings.ToLower(ep.HTTPPath), sp) {
				finding := SecurityFinding{
					ID:          generateFindingID("AUTH", ep.ID),
					EndpointID:  ep.ID,
					Severity:    "MEDIUM",
					Category:    "MISSING_AUTH",
					Title:       "Missing Authentication on Sensitive Endpoint",
					Description: "Endpoint " + ep.HTTPPath + " does not require authentication but handles sensitive operations",
					Evidence:    "Path: " + ep.HTTPPath + " in " + ep.ClassName + "." + ep.MethodName,
					Recommendation: "Add @PreAuthorize, @Secured, or configure security for this endpoint",
					Confidence:  0.7,
					Validated:   true,
				}
				v.Findings = append(v.Findings, finding)
				break
			}
		}
	}
}

// checkInsecureDirectObjectReference checks for IDOR vulnerabilities.
func (v *EndpointValidator) checkInsecureDirectObjectReference() {
	for _, ep := range v.Endpoints {
		// Check for endpoints with path variables that modify data
		if len(ep.PathVariables) == 0 {
			continue
		}

		isModificationMethod := false
		for _, m := range ep.HTTPMethods {
			if m == "PUT" || m == "DELETE" || m == "PATCH" || m == "POST" {
				isModificationMethod = true
				break
			}
		}

		if !isModificationMethod {
			continue
		}

		// Check if there's authorization logic
		if len(ep.AuthRequirements) == 0 {
			finding := SecurityFinding{
				ID:          generateFindingID("IDOR", ep.ID),
				EndpointID:  ep.ID,
				Severity:    "HIGH",
				Category:    "IDOR",
				Title:       "Potential Insecure Direct Object Reference (IDOR)",
				Description: "Endpoint modifies resources using path parameter without explicit authorization check",
				Evidence:    "Path: " + ep.HTTPPath + ", Variables: " + pathVarsToString(ep.PathVariables),
				Recommendation: "Ensure proper authorization checks to verify the user has permission to modify the resource",
				Confidence:  0.6,
				Validated:   true,
			}
			v.Findings = append(v.Findings, finding)
		}
	}
}

// checkMissingInputValidation checks for endpoints without input validation.
func (v *EndpointValidator) checkMissingInputValidation() {
	for _, ep := range v.Endpoints {
		// Check for endpoints that accept request bodies
		if ep.RequestBody == nil && len(ep.QueryParams) == 0 && len(ep.PathVariables) == 0 {
			continue
		}

		// Check if endpoint has parameters but no validation
		hasValidation := false
		for _, anno := range ep.RawAnnotations {
			if strings.Contains(anno, "@NotNull") || strings.Contains(anno, "@NotBlank") ||
				strings.Contains(anno, "@Size") || strings.Contains(anno, "@Min") ||
				strings.Contains(anno, "@Max") || strings.Contains(anno, "@Pattern") {
				hasValidation = true
				break
			}
		}

		if !hasValidation && (ep.RequestBody != nil || len(ep.QueryParams) > 0) {
			finding := SecurityFinding{
				ID:          generateFindingID("VAL", ep.ID),
				EndpointID:  ep.ID,
				Severity:    "MEDIUM",
				Category:    "MISSING_VALIDATION",
				Title:       "Missing Input Validation",
				Description: "Endpoint accepts parameters without visible validation annotations",
				Evidence:    "Path: " + ep.HTTPPath + " in " + ep.ClassName + "." + ep.MethodName,
				Recommendation: "Add Bean Validation annotations (@NotNull, @Size, @Min, @Max) to parameters",
				Confidence:  0.5,
				Validated:   true,
			}
			v.Findings = append(v.Findings, finding)
		}
	}
}

// checkSensitiveDataExposure checks for potential sensitive data exposure.
func (v *EndpointValidator) checkSensitiveDataExposure() {
	sensitivePatterns := []struct {
		Pattern string
		Fields  []string
	}{
		{"password", []string{"password", "pwd", "passwd", "secret"}},
		{"token", []string{"token", "jwt", "bearer"}},
		{"secret", []string{"secret", "apikey", "api_key"}},
		{"credential", []string{"credential", "auth"}},
	}

	for _, ep := range v.Endpoints {
		for _, sensitive := range sensitivePatterns {
			if strings.Contains(strings.ToLower(ep.HTTPPath), sensitive.Pattern) {
				// Check if it's a write operation (less concerning)
				if ep.HTTPMethods[0] == "POST" || ep.HTTPMethods[0] == "PUT" {
					continue
				}

				finding := SecurityFinding{
					ID:          generateFindingID("SDE", ep.ID),
					EndpointID:  ep.ID,
					Severity:    "LOW",
					Category:    "SENSITIVE_DATA_EXPOSURE",
					Title:       "Endpoint Accesses Sensitive Data",
					Description: "Endpoint path suggests access to sensitive data (e.g., tokens, credentials)",
					Evidence:    "Path: " + ep.HTTPPath,
					Recommendation: "Ensure proper authentication and authorization; audit response data for sensitive fields",
					Confidence:  0.6,
					Validated:   true,
				}
				v.Findings = append(v.Findings, finding)
				break
			}
		}
	}
}

// checkCSRFProtection checks for missing CSRF protection.
func (v *EndpointValidator) checkCSRFProtection() {
	for _, ep := range v.Endpoints {
		// Only check state-changing operations
		isStateChanging := false
		for _, m := range ep.HTTPMethods {
			if m == "POST" || m == "PUT" || m == "DELETE" || m == "PATCH" {
				isStateChanging = true
				break
			}
		}

		if !isStateChanging {
			continue
		}

		// Check if auth is required (CSRF is less critical for APIs without session)
		if ep.AuthRequired && len(ep.AuthRequirements) > 0 && ep.AuthRequirements[0].Type != "permit_all" {
			// Authenticated API - may need CSRF protection
			finding := SecurityFinding{
				ID:          generateFindingID("CSRF", ep.ID),
				EndpointID:  ep.ID,
				Severity:    "INFO",
				Category:    "CSRF",
				Title:       "Review CSRF Protection for State-Changing Operations",
				Description: "State-changing operation requires CSRF protection review",
				Evidence:    "Path: " + ep.HTTPPath + ", Methods: " + strings.Join(ep.HTTPMethods, ","),
				Recommendation: "For session-based APIs, ensure CSRF tokens are validated; for token-based APIs, verify token validation",
				Confidence:  0.4,
				Validated:   true,
			}
			v.Findings = append(v.Findings, finding)
		}
	}
}

// checkMassAssignment checks for potential mass assignment vulnerabilities.
func (v *EndpointValidator) checkMassAssignment() {
	for _, ep := range v.Endpoints {
		// Check for endpoints that accept request bodies
		if ep.RequestBody == nil {
			continue
		}

		// High-risk patterns: update, save, create, set operations
		highRisk := false
		riskMethods := []string{"update", "save", "create", "set", "add", "edit", "modify"}
		for _, rm := range riskMethods {
			if strings.Contains(strings.ToLower(ep.MethodName), rm) {
				highRisk = true
				break
			}
		}

		if highRisk {
			finding := SecurityFinding{
				ID:          generateFindingID("MA", ep.ID),
				EndpointID:  ep.ID,
				Severity:    "MEDIUM",
				Category:    "MASS_ASSIGNMENT",
				Title:       "Potential Mass Assignment Vulnerability",
				Description: "Endpoint accepts request body that may be subject to mass assignment",
				Evidence:    "Path: " + ep.HTTPPath + ", Body Type: " + ep.RequestBody.TypeName,
				Recommendation: "Use @JsonIgnore, @JsonProperty, or DTO pattern to control which fields can be set",
				Confidence:  0.5,
				Validated:   true,
			}
			v.Findings = append(v.Findings, finding)
		}
	}
}

// checkidor checks for additional IDOR patterns.
func (v *EndpointValidator) checkidor() {
	// Additional IDOR checks for common patterns
	idorPatterns := []struct {
		PathPattern string
		Risk       string
	}{
		{"/{id}/delete", "DELETE operation with ID parameter"},
		{"/{id}/reset", "Password reset with ID parameter"},
		{"/{id}/transfer", "Data transfer with ID parameter"},
		{"/{id}/export", "Data export with ID parameter"},
	}

	for _, ep := range v.Endpoints {
		for _, pattern := range idorPatterns {
			if strings.Contains(ep.HTTPPath, "{") && strings.Contains(strings.ToLower(ep.HTTPPath), pattern.PathPattern[2:]) {
				finding := SecurityFinding{
					ID:          generateFindingID("IDOR2", ep.ID),
					EndpointID:  ep.ID,
					Severity:    "HIGH",
					Category:    "IDOR",
					Title:       "Potential IDOR: " + pattern.Risk,
					Description: pattern.Risk + " without explicit authorization",
					Evidence:    "Path: " + ep.HTTPPath,
					Recommendation: "Verify user authorization before performing operation",
					Confidence:  0.65,
					Validated:   true,
				}
				v.Findings = append(v.Findings, finding)
			}
		}
	}
}

// Helper functions.

func pathVarsToString(pvs []PathVariable) string {
	var parts []string
	for _, pv := range pvs {
		parts = append(parts, pv.Name)
	}
	return strings.Join(parts, ", ")
}

func generateFindingID(prefix, endpointID string) string {
	return prefix + "_" + endpointID[:8]
}

// PersistValidationReport saves the validation report.
func (v *EndpointValidator) PersistValidationReport(rt *Runtime, report *ValidationReport) error {
	if rt == nil || report == nil {
		return utils.Error("nil runtime or report")
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	path := store.ValidationReportPath(rt.WorkDir)
	if err := writeJSONFile(path, b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactValidationReport, string(b))
	}
	return nil
}

// LoadValidationReport loads a saved validation report.
func LoadValidationReport(workDir string) (*ValidationReport, error) {
	path := store.ValidationReportPath(workDir)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report ValidationReport
	if err := json.Unmarshal(b, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

// RunEndpointValidation runs the full validation pipeline.
func RunEndpointValidation(rt *Runtime, endpoints []APIEndpoint) (*ValidationReport, error) {
	if rt == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}

	sessionID := fmt.Sprintf("%d", rt.Session.ID)
	codeRoot := rt.Session.CodeRootPath

	validator := NewEndpointValidator(endpoints, sessionID, codeRoot)
	report := validator.Validate()

	// Persist the report
	if err := validator.PersistValidationReport(rt, report); err != nil {
		log.Warnf("ssa_api_discovery: failed to persist validation report: %v", err)
	}

	log.Infof("ssa_api_discovery: endpoint_validation endpoints=%d findings=%d",
		report.EndpointsValidated, report.Stats.TotalFindings)

	return report, nil
}

// ExportEndpointsToFile saves endpoints to a JSON file.
func ExportEndpointsToFile(endpoints []APIEndpoint, workDir string) error {
	path := filepath.Join(workDir, "endpoints.json")
	data, err := json.MarshalIndent(endpoints, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadEndpointsFromFile loads endpoints from a JSON file.
func LoadEndpointsFromFile(workDir string) ([]APIEndpoint, error) {
	path := filepath.Join(workDir, "endpoints.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var endpoints []APIEndpoint
	if err := json.Unmarshal(b, &endpoints); err != nil {
		return nil, err
	}
	return endpoints, nil
}

// RunFullEndpointExtraction runs the complete extraction pipeline.
func RunFullEndpointExtraction(rt *Runtime) (*UnifiedEndpointsInventory, error) {
	if rt == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}

	codeRoot := rt.Session.CodeRootPath
	inventory := NewUnifiedEndpointsInventory(codeRoot)

	// Step 1: Detect frameworks
	detector, err := RunJavaFrameworkDetection(rt)
	if err != nil {
		log.Warnf("ssa_api_discovery: framework detection failed: %v", err)
	}
	if detector != nil {
		inventory.Frameworks = detector.Frameworks
	}

	// Step 2: Extract endpoints based on detected frameworks
	var allEndpoints []APIEndpoint

	// Extract Spring endpoints if Spring is detected
	if detector == nil || detector.HasFramework(FrameworkSpringBoot) {
		springEndpoints, err := HarvestSpringEndpoints(codeRoot)
		if err != nil {
			log.Warnf("ssa_api_discovery: spring endpoint extraction failed: %v", err)
		}
		allEndpoints = append(allEndpoints, springEndpoints...)
		log.Infof("ssa_api_discovery: spring endpoints extracted: %d", len(springEndpoints))
	}

	// Extract JAX-RS endpoints if JAX-RS is detected
	if detector == nil || detector.HasFramework(FrameworkJAXRS) {
		jaxrsEndpoints, err := HarvestJAXRSEndpoints(codeRoot)
		if err != nil {
			log.Warnf("ssa_api_discovery: jaxrs endpoint extraction failed: %v", err)
		}
		allEndpoints = append(allEndpoints, jaxrsEndpoints...)
		log.Infof("ssa_api_discovery: jaxrs endpoints extracted: %d", len(jaxrsEndpoints))
	}

	// Extract Struts 2 endpoints if Struts is detected
	if detector != nil && detector.HasFramework(FrameworkStruts2) {
		strutsEndpoints, err := HarvestStruts2Endpoints(codeRoot)
		if err != nil {
			log.Warnf("ssa_api_discovery: struts endpoint extraction failed: %v", err)
		}
		allEndpoints = append(allEndpoints, strutsEndpoints...)
		log.Infof("ssa_api_discovery: struts endpoints extracted: %d", len(strutsEndpoints))
	}

	// Extract Servlet endpoints (always as baseline)
	servletEndpoints, err := HarvestServletEndpoints(codeRoot)
	if err != nil {
		log.Warnf("ssa_api_discovery: servlet endpoint extraction failed: %v", err)
	}
	allEndpoints = append(allEndpoints, servletEndpoints...)
	log.Infof("ssa_api_discovery: servlet endpoints extracted: %d", len(servletEndpoints))

	// Step 3: Deduplicate
	allEndpoints = deduplicateEndpoints(allEndpoints)

	// Step 4: Set in inventory
	inventory.Endpoints = allEndpoints
	inventory.ComputeStats()

	// Step 5: Persist
	if err := persistUnifiedEndpoints(rt, inventory); err != nil {
		log.Warnf("ssa_api_discovery: failed to persist unified endpoints: %v", err)
	}

	// Step 6: Run validation
	validationReport, err := RunEndpointValidation(rt, allEndpoints)
	if err != nil {
		log.Warnf("ssa_api_discovery: endpoint validation failed: %v", err)
	} else {
		log.Infof("ssa_api_discovery: validation completed findings=%d", validationReport.Stats.TotalFindings)
	}

	return inventory, nil
}

// persistUnifiedEndpoints saves the unified endpoints inventory.
func persistUnifiedEndpoints(rt *Runtime, inventory *UnifiedEndpointsInventory) error {
	if rt == nil || inventory == nil {
		return utils.Error("nil inventory")
	}
	b, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return err
	}
	path := store.UnifiedEndpointsPath(rt.WorkDir)
	if err := writeJSONFile(path, b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactUnifiedEndpoints, string(b))
	}
	return nil
}

// AgentSupplementEndpoints handles cases where programmatic extraction fails.
// The React Agent should call this to supplement with AI-discovered endpoints.
func AgentSupplementEndpoints(rt *Runtime, agentEndpoints []APIEndpoint) error {
	if rt == nil {
		return utils.Error("nil runtime")
	}

	// Load existing endpoints
	existing, err := LoadEndpointsFromFile(rt.WorkDir)
	if err != nil {
		existing = []APIEndpoint{}
	}

	// Merge agent endpoints
	allEndpoints := append(existing, agentEndpoints...)
	allEndpoints = deduplicateEndpoints(allEndpoints)

	// Persist updated endpoints
	return ExportEndpointsToFile(allEndpoints, rt.WorkDir)
}

// CreateAgentEndpoint creates an APIEndpoint from agent-discovered information.
// This ensures agent output matches the programmatic output format.
func CreateAgentEndpoint(className, methodName, path, httpMethod, framework string, authRequired bool) *APIEndpoint {
	return &APIEndpoint{
		ID:              generateEndpointID(className, methodName, httpMethod, path),
		Framework:       JavaFrameworkType(framework),
		ClassName:       className,
		SimpleClassName: extractSimpleClassName(className),
		MethodName:      methodName,
		HTTPPath:        path,
		HTTPMethods:     []string{httpMethod},
		AuthRequired:   authRequired,
		Confidence:      0.9, // AI-discovered endpoints get high confidence
		Provenance:      "ai_inference",
	}
}
