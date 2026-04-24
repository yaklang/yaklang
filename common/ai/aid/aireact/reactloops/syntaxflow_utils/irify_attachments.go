package syntaxflow_utils

// Yakit IRify 附件 Type / Key 的线串值（与 AIInputEvent.AttachedResourceInfo 一致），Go 标识符仅作说明性命名。
// Wire 字面量须保持稳定，勿改 YAK/底座已有约定。

const (
	IrifyTypeSyntaxFlow     = "irify_syntaxflow"
	IrifyTypeSSARisk        = "irify_ssa_risk"
	IrifyTypeSSARisksFilter = "irify_ssa_risks_filter"
	IrifyTypeSyntaxFlowRule = "irify_syntaxflow_rule"
)

// Irify key names for the corresponding IrifyType* (value format depends on key).
const (
	IrifyKeyTaskID      = "task_id"
	IrifyKeySessionMode = "session_mode"
	IrifyKeyPrograms    = "programs"
	IrifyKeyRiskID      = "risk_id"
	IrifyKeyFilterJSON  = "filter_json"
	IrifyKeyRuntimeID   = "runtime_id"
	IrifyKeyProgramName = "program_name"
	IrifyKeyFullQuality = "full_quality"
)

// SessionMode values (IrifyKeySessionMode or loop var LoopVarSyntaxFlowScanSessionMode).
const (
	SessionModeAttach = "attach"
	SessionModeStart  = "start"
)
