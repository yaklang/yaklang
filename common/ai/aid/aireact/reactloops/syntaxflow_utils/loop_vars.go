package syntaxflow_utils

// Loop variable keys (reactloops.WithVar) used by IRify-related resolvers.
const (
	LoopVarSyntaxFlowTaskID          = "syntaxflow_task_id"
	LoopVarSyntaxFlowScanSessionMode = "syntaxflow_scan_session_mode"
	LoopVarSFRuleFullQuality         = "sf_rule_full_quality"
	LoopVarSSARiskID                 = "ssa_risk_id"
	LoopVarSSARisksFilterJSON        = "ssa_risks_filter_json"
	// LoopVarSSAOverviewFilterJSON stores the last effective SSARisksFilter (protojson) for reload_ssa_risk_overview without parameters.
	LoopVarSSAOverviewFilterJSON = "ssa_overview_filter_json"
)
