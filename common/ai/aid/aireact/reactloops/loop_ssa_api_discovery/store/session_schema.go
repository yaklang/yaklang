package store

// SessionReadEntity names supported by discovery_read_session_data (read-only).
const (
	SessionEntityStatus              = "status"
	SessionEntitySnapshot            = "snapshot"
	SessionEntitySchema              = "schema"
	SessionEntityHTTPEndpoints       = "http_endpoints"
	SessionEntityVerifiedEndpoints   = "verified_endpoints"
	SessionEntitySyntaxflowFindings  = "syntaxflow_findings"
	SessionEntityVulnVerifications  = "vuln_verifications"
	SessionEntityDynamicVulnFindings = "dynamic_vuln_findings"
	SessionEntityAuthCredentials     = "auth_credentials"
	SessionEntityComponents          = "components"
	SessionEntityConfigArtifacts     = "config_artifacts"
	SessionEntityDependencies        = "dependencies"
	SessionEntitySecurityMechanisms  = "security_mechanisms"
	SessionEntityBusinessCapabilities = "business_capabilities"
	SessionEntityVerifiedHTTPApis    = "verified_http_apis"
	SessionEntityVulnChecklistItems  = "vuln_checklist_items"
	SessionEntityPhaseArtifacts      = "phase_artifacts"
	SessionEntityCoverageWorkItems   = "coverage_work_items"
	SessionEntityDiscoveryEvents     = "discovery_events"
	SessionEntityEndpointValidationAttempts = "endpoint_validation_attempts"
	SessionEntityFileOperations             = "file_operations"
)

// DocumentedSessionTableColumns is the authoritative column list for report/SQL hints.
// Keys are SQLite table names; values are column names in storage order.
var DocumentedSessionTableColumns = map[string][]string{
	"vuln_verifications": {
		"id", "created_at", "updated_at", "session_id",
		"syntax_flow_finding_id", "dynamic_finding_id", "source", "status", "confidence",
		"exploit_payload", "exploit_response", "ai_analysis", "fix",
	},
	"verified_endpoints": {
		"id", "created_at", "updated_at", "session_id",
		"http_endpoint_id", "method", "path_pattern", "request_url",
		"status_code", "verified", "response_hint", "notes",
		"request_headers", "response_body", "content_type", "response_size",
	},
	"http_endpoints": {
		"id", "created_at", "updated_at", "session_id",
		"method", "path_pattern", "handler_class", "handler_method",
		"authz_hint", "source", "status", "last_probed_at",
		"probe_status_code", "probe_evidence", "reject_reason", "function_score",
	},
	"discovery_syntaxflow_findings": {
		"id", "created_at", "updated_at", "session_id",
		"risk_hash", "rule_name", "severity", "title", "description",
		"matched_file", "matched_line", "data_flow_hint", "confidence",
	},
	"dynamic_vuln_findings": {
		"id", "created_at", "updated_at", "session_id",
		"http_endpoint_id", "vuln_type", "severity", "confidence",
		"payload", "request_url", "request_raw", "response_raw",
		"evidence", "status", "ai_analysis", "code_context",
	},
	"verified_http_apis": {
		"id", "created_at", "updated_at", "session_id",
		"method", "path_pattern", "full_sample_url", "effective_base", "url_space",
		"query_params_json", "body_hint_json", "auth_required", "auth_headers_json",
		"handler_file", "handler_symbol", "code_snippet", "business_capability_id",
		"forward_chain_json", "probe_status_code", "content_type", "response_excerpt",
		"probe_attempts_json", "verdict_reason", "verified", "confidence", "source", "notes", "reject_reason",
	},
	"auth_credentials": {
		"id", "created_at", "updated_at", "session_id",
		"auth_type", "username", "token_value", "header_name", "header_value",
		"url_space", "auth_realm", "credential_group_id", "mount_prefix", "login_path", "login_evidence_json",
		"valid_until", "verified", "verify_url", "notes",
		"headers_json", "headers_text", "acquire_recipe_id",
		"last_acquired_at", "last_verified_at", "expires_hint",
		"refresh_state", "reacquire_count",
	},
	"vuln_checklist_items": {
		"id", "created_at", "updated_at", "session_id",
		"finding_id", "endpoint_id", "verified_http_api_id",
		"rule_name", "severity", "title", "matched_file", "data_flow_hint",
		"method", "path_pattern", "full_sample_url", "handler_class",
		"priority", "assoc_confidence", "status",
	},
	"phase_artifacts": {
		"id", "created_at", "updated_at", "session_id",
		"kind", "version", "payload_json",
	},
	"coverage_work_items": {
		"id", "created_at", "updated_at", "session_id",
		"kind", "ref_id", "ref_label", "priority", "status", "evidence", "blocked_reason",
	},
	"discovery_events": {
		"id", "created_at", "session_id", "level", "message", "payload_json",
	},
	"endpoint_validation_attempts": {
		"id", "created_at", "session_id", "http_endpoint_id",
		"attempt_no", "url", "method", "request_headers", "status_code",
		"response_snippet", "verdict", "reason", "ai_analysis",
	},
	"discovery_file_operations": {
		"id", "created_at", "session_id", "pipeline_stage", "operation",
		"rel_path", "rule_id", "tool_name", "outcome", "summary", "detail_json", "duration_ms",
	},
}

// AllSessionReadEntities lists entity values for action param help text.
func AllSessionReadEntities() []string {
	return []string{
		SessionEntityStatus,
		SessionEntitySnapshot,
		SessionEntitySchema,
		SessionEntityHTTPEndpoints,
		SessionEntityVerifiedEndpoints,
		SessionEntitySyntaxflowFindings,
		SessionEntityVulnVerifications,
		SessionEntityDynamicVulnFindings,
		SessionEntityAuthCredentials,
		SessionEntityComponents,
		SessionEntityConfigArtifacts,
		SessionEntityDependencies,
		SessionEntitySecurityMechanisms,
		SessionEntityBusinessCapabilities,
		SessionEntityVerifiedHTTPApis,
		SessionEntityVulnChecklistItems,
		SessionEntityPhaseArtifacts,
		SessionEntityCoverageWorkItems,
		SessionEntityDiscoveryEvents,
		SessionEntityEndpointValidationAttempts,
		SessionEntityFileOperations,
	}
}
