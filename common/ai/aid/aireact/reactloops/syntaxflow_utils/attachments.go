package syntaxflow_utils

// Attached resource types for IRify / SyntaxFlow / SSA Risk focus loops.
// Wire values stay stable for Yakit (e.g. still "irify_syntaxflow").
// Values are passed via AIInputEvent.AttachedResourceInfo → task.SetAttachedDatas.
const (
	AttachedTypeSyntaxFlow      = "irify_syntaxflow"
	AttachedTypeSSARisk         = "irify_ssa_risk"
	AttachedTypeSSARisksFilter  = "irify_ssa_risks_filter"
	AttachedTypeSyntaxFlowRule  = "irify_syntaxflow_rule"
)

// Standard keys for AttachedType* (Value format depends on key).
const (
	AttachedKeyTaskID       = "task_id"
	AttachedKeySessionMode  = "session_mode"
	AttachedKeyPrograms     = "programs"
	AttachedKeyRiskID       = "risk_id"
	AttachedKeyFilterJSON   = "filter_json"
	AttachedKeyRuntimeID    = "runtime_id"
	AttachedKeyProgramName  = "program_name"
	AttachedKeyFullQuality  = "full_quality"
)

// SessionMode values (AttachedKeySessionMode or loop var LoopVarSyntaxFlowScanSessionMode).
const (
	SessionModeAttach = "attach"
	SessionModeStart  = "start"
)
