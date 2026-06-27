package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

const unifiedEndpointSchemaVersion = 1

// JavaFrameworkType represents supported Java web frameworks.
type JavaFrameworkType string

const (
	FrameworkSpringBoot JavaFrameworkType = "spring-boot"
	FrameworkJAXRS      JavaFrameworkType = "jax-rs"
	FrameworkStruts2    JavaFrameworkType = "struts2"
	FrameworkServlet    JavaFrameworkType = "servlet"
	FrameworkUnknown    JavaFrameworkType = "unknown"
)

// AuthRule represents an authorization requirement for an endpoint.
type AuthRule struct {
	Type       string   `json:"type"`        // pre_authz, post_authz, permit_all, deny_all, required
	Expression string   `json:"expression"`   // SpEL expression like "hasRole('ADMIN')"
	Roles      []string `json:"roles"`       // Extracted roles like ["ADMIN", "USER"]
}

// PathVariable represents a URL path variable extraction.
type PathVariable struct {
	Name string `json:"name"` // e.g., "id", "userId"
	Type string `json:"type"` // e.g., "Long", "String"
}

// QueryParam represents a query parameter.
type QueryParam struct {
	Name        string `json:"name"`         // parameter name
	Type        string `json:"type"`         // parameter type
	Required    bool   `json:"required"`     // is required
	DefaultVal  string `json:"default_val"`  // default value if any
	Description string `json:"description"`   // from @RequestParam description
}

// RequestBody represents the expected request body structure.
type RequestBody struct {
	ContentType string `json:"content_type"` // e.g., "application/json"
	TypeName    string `json:"type_name"`    // e.g., "CreateUserRequest"
}

// APIEndpoint is the unified API endpoint model for all Java frameworks.
type APIEndpoint struct {
	ID              string          `json:"id"`               // unique identifier
	Framework       JavaFrameworkType `json:"framework"`        // spring-boot, jax-rs, struts2, servlet
	ClassName       string          `json:"class_name"`       // fully qualified class name
	SimpleClassName string          `json:"simple_class_name"` // just the class name
	MethodName      string          `json:"method_name"`       // method name
	PackageName     string          `json:"package_name"`      // package name

	// HTTP routing
	HTTPPath    string   `json:"http_path"`    // complete path like /admin/user/list
	HTTPMethods []string `json:"http_methods"` // GET, POST, PUT, DELETE, PATCH

	// Parameters
	PathVariables []PathVariable `json:"path_variables"` // {id}, {userId}
	QueryParams   []QueryParam   `json:"query_params"`   // ?page=1&size=10
	RequestBody   *RequestBody   `json:"request_body"`   // JSON body type

	// Authorization
	AuthRequirements []AuthRule `json:"auth_requirements"`
	AuthRequired    bool       `json:"auth_required"`     // derived from auth annotations

	// Source location
	FilePath   string `json:"file_path"`   // relative file path
	LineNumber int    `json:"line_number"` // method line number

	// Confidence and metadata
	Confidence   float64 `json:"confidence"`   // 0.0-1.0
	Provenance   string  `json:"provenance"`   // "annotation", "xml_config", "ai_inference"
	RawAnnotations []string `json:"raw_annotations"` // original annotation text

	// Additional metadata
	Deprecated              bool   `json:"deprecated"`                         // marked as @Deprecated
	Documentation           string `json:"documentation"`                      // method javadoc if available
	SessionAttributeMethod  bool   `json:"session_attribute_method,omitempty"` // handler takes @SessionAttribute (typical POST form)
}

// UnifiedEndpointsInventory is the root output of the unified endpoint extraction.
type UnifiedEndpointsInventory struct {
	SchemaVersion int          `json:"schema_version"`
	GeneratedAt   string       `json:"generated_at"`
	CodeRoot      string       `json:"code_root"`
	Frameworks    []FrameworkInfo `json:"frameworks"`

	// Framework-specific endpoint counts
	SpringEndpoints int `json:"spring_endpoints"`
	JAXRSEndpoints  int `json:"jaxrs_endpoints"`
	StrutsEndpoints int `json:"struts_endpoints"`
	ServletEndpoints int `json:"servlet_endpoints"`

	// All extracted endpoints
	Endpoints []APIEndpoint `json:"endpoints"`

	// Statistics
	Stats EndpointStats `json:"stats"`

	// Warnings for potential issues
	Warnings []string `json:"warnings,omitempty"`
}

// FrameworkInfo contains framework detection results.
type FrameworkInfo struct {
	Type       JavaFrameworkType `json:"type"`
	Confidence float64           `json:"confidence"` // 0.0-1.0
	Evidence   string            `json:"evidence"`   // pom.xml or annotation that detected it
	Version    string            `json:"version"`    // detected version if available
}

// EndpointStats contains extraction statistics.
type EndpointStats struct {
	TotalEndpoints       int     `json:"total_endpoints"`
	AuthenticatedOnly    int     `json:"authenticated_only"`
	PublicEndpoints      int     `json:"public_endpoints"`
	WithPathVariables    int     `json:"with_path_variables"`
	WithRequestBody      int     `json:"with_request_body"`
	AvgConfidence        float64 `json:"avg_confidence"`
	LowConfidenceCount   int     `json:"low_confidence_count"` // confidence < 0.7
}

// SecurityFinding represents a security issue found by the agent validator.
type SecurityFinding struct {
	ID             string   `json:"id"`              // unique ID
	EndpointID     string   `json:"endpoint_id"`     // reference to APIEndpoint.ID
	Severity       string   `json:"severity"`        // CRITICAL, HIGH, MEDIUM, LOW, INFO
	Category       string   `json:"category"`        // SQL_INJECTION, XSS, AUTH_BYPASS, etc.
	Title          string   `json:"title"`           // short title
	Description    string   `json:"description"`     // detailed description
	Evidence       string   `json:"evidence"`        // code snippet or URL
	Recommendation string   `json:"recommendation"` // fix suggestion
	CVSSScore      float64  `json:"cvss_score,omitempty"`
	CWE            string   `json:"cwe,omitempty"`   // CWE ID if applicable
	OWASP          string   `json:"owasp,omitempty"` // OWASP category
	Validated      bool     `json:"validated"`       // confirmed by agent
	Confidence     float64  `json:"confidence"`      // 0.0-1.0
}

// ValidationReport is the output of the agent validation phase.
type ValidationReport struct {
	SchemaVersion int               `json:"schema_version"`
	GeneratedAt   string           `json:"generated_at"`
	SessionID     string           `json:"session_id"`
	EndpointsTotal int             `json:"endpoints_total"`
	EndpointsValidated int         `json:"endpoints_validated"`
	Findings       []SecurityFinding `json:"findings"`
	Stats          ValidationStats  `json:"stats"`
	Warnings       []string         `json:"warnings,omitempty"`
}

// ValidationStats contains validation statistics.
type ValidationStats struct {
	TotalFindings    int            `json:"total_findings"`
	BySeverity       map[string]int `json:"by_severity"`
	ByCategory       map[string]int `json:"by_category"`
	HighConfidence   int            `json:"high_confidence"`
	LowConfidence    int            `json:"low_confidence"`
}

// Framework detection functions.
func DetectJavaFrameworks(codeRoot string, javaFiles []string, pomContent, buildContent string) []FrameworkInfo {
	var results []FrameworkInfo
	lower := strings.ToLower(pomContent + buildContent)

	// Check for Spring Boot
	springScore := 0.0
	if strings.Contains(lower, "spring-boot") {
		springScore += 0.6
	}
	if strings.Contains(lower, "spring-web") || strings.Contains(lower, "springframework") {
		springScore += 0.3
	}
	if reSpringBootApp.MatchString(pomContent) || reSpringBootApp.MatchString(buildContent) {
		springScore += 0.2
	}
	// Check Java files for Spring annotations
	for _, f := range javaFiles {
		content, _ := osReadFile(f)
		if reSpringWebAnnotation.Match(content) {
			springScore += 0.1
			if springScore > 1.0 {
				springScore = 1.0
			}
			break
		}
	}
	if springScore >= 0.3 {
		results = append(results, FrameworkInfo{
			Type:       FrameworkSpringBoot,
			Confidence: minFloat(springScore, 1.0),
			Evidence:   "pom.xml contains spring-boot dependency or @SpringBootApplication found",
		})
	}

	// Check for JAX-RS
	jaxrsScore := 0.0
	if strings.Contains(lower, "jakarta.ws.rs") || strings.Contains(lower, "javax.ws.rs") {
		jaxrsScore += 0.7
	}
	if strings.Contains(lower, "jersey") || strings.Contains(lower, "resteasy") {
		jaxrsScore += 0.3
	}
	for _, f := range javaFiles {
		content, _ := osReadFile(f)
		if reJAXRSImport.Match(content) {
			jaxrsScore += 0.2
			break
		}
	}
	if jaxrsScore >= 0.3 {
		results = append(results, FrameworkInfo{
			Type:       FrameworkJAXRS,
			Confidence: minFloat(jaxrsScore, 1.0),
			Evidence:   "pom.xml contains JAX-RS dependency",
		})
	}

	// Check for Struts 2
	if strings.Contains(lower, "struts") || strings.Contains(lower, "struts2") {
		results = append(results, FrameworkInfo{
			Type:       FrameworkStruts2,
			Confidence: 0.7,
			Evidence:   "pom.xml contains Struts dependency",
		})
	}

	// Check for Servlet (always possible for Java web apps)
	servletScore := 0.0
	if strings.Contains(lower, "servlet") {
		servletScore += 0.4
	}
	if strings.Contains(lower, "web.xml") || strings.Contains(lower, "webapp") {
		servletScore += 0.3
	}
	for _, f := range javaFiles {
		content, _ := osReadFile(f)
		if reServletImport.Match(content) {
			servletScore += 0.3
			break
		}
	}
	if servletScore >= 0.3 {
		results = append(results, FrameworkInfo{
			Type:       FrameworkServlet,
			Confidence: minFloat(servletScore, 0.6), // Servlet is often a fallback
			Evidence:   "Java Servlet API detected",
		})
	}

	return results
}

// osReadFile is a simple wrapper for reading files in extraction functions.
func osReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// ToJSON serializes the inventory to JSON.
func (u *UnifiedEndpointsInventory) ToJSON() ([]byte, error) {
	return json.MarshalIndent(u, "", "  ")
}

// ToSecurityFindingsJSON serializes findings to JSON.
func (v *ValidationReport) ToJSON() ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// NewUnifiedEndpointsInventory creates a new inventory with defaults.
func NewUnifiedEndpointsInventory(codeRoot string) *UnifiedEndpointsInventory {
	return &UnifiedEndpointsInventory{
		SchemaVersion: unifiedEndpointSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		CodeRoot:      codeRoot,
		Endpoints:     []APIEndpoint{},
		Warnings:      []string{},
	}
}

// NewValidationReport creates a new validation report.
func NewValidationReport(sessionID string) *ValidationReport {
	return &ValidationReport{
		SchemaVersion: unifiedEndpointSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		SessionID:     sessionID,
		Findings:      []SecurityFinding{},
		Stats: ValidationStats{
			BySeverity: make(map[string]int),
			ByCategory: make(map[string]int),
		},
		Warnings: []string{},
	}
}

// ComputeStats calculates statistics for the inventory.
func (u *UnifiedEndpointsInventory) ComputeStats() {
	u.Stats = EndpointStats{}
	u.Stats.TotalEndpoints = len(u.Endpoints)

	var confidenceSum float64
	lowConfCount := 0

	for _, ep := range u.Endpoints {
		switch ep.Framework {
		case FrameworkSpringBoot:
			u.SpringEndpoints++
		case FrameworkJAXRS:
			u.JAXRSEndpoints++
		case FrameworkStruts2:
			u.StrutsEndpoints++
		case FrameworkServlet:
			u.ServletEndpoints++
		}

		if ep.AuthRequired {
			u.Stats.AuthenticatedOnly++
		} else {
			u.Stats.PublicEndpoints++
		}

		if len(ep.PathVariables) > 0 {
			u.Stats.WithPathVariables++
		}

		if ep.RequestBody != nil {
			u.Stats.WithRequestBody++
		}

		confidenceSum += ep.Confidence
		if ep.Confidence < 0.7 {
			lowConfCount++
		}
	}

	if len(u.Endpoints) > 0 {
		u.Stats.AvgConfidence = confidenceSum / float64(len(u.Endpoints))
	}
	u.Stats.LowConfidenceCount = lowConfCount
}

// ComputeValidationStats calculates statistics for the validation report.
func (v *ValidationReport) ComputeStats() {
	v.Stats = ValidationStats{
		BySeverity: make(map[string]int),
		ByCategory: make(map[string]int),
	}
	v.Stats.TotalFindings = len(v.Findings)

	for _, f := range v.Findings {
		v.Stats.BySeverity[f.Severity]++
		v.Stats.ByCategory[f.Category]++

		if f.Confidence >= 0.7 {
			v.Stats.HighConfidence++
		} else {
			v.Stats.LowConfidence++
		}
	}
}
