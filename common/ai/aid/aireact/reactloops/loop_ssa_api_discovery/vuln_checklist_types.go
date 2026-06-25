package loop_ssa_api_discovery

// VulnChecklistItem represents one pending verification target linking a static finding to an endpoint.
type VulnChecklistItem struct {
	FindingID         uint   `json:"finding_id"`
	RuleName          string `json:"rule_name"`
	Severity          string `json:"severity"`
	Title             string `json:"title"`
	MatchedFile       string `json:"matched_file,omitempty"`
	DataFlowHint      string `json:"data_flow_hint,omitempty"`
	EndpointID        uint   `json:"endpoint_id,omitempty"`
	VerifiedHttpApiID uint   `json:"verified_http_api_id,omitempty"`
	Method            string `json:"method,omitempty"`
	PathPattern       string `json:"path_pattern,omitempty"`
	FullSampleURL     string `json:"full_sample_url,omitempty"`
	HandlerClass      string `json:"handler_class,omitempty"`
	Priority          int    `json:"priority"`
	AssocConfidence   string `json:"assoc_confidence"`
}
