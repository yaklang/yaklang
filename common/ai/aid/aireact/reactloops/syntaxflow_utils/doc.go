// Package syntaxflow_utils holds small shared pieces for IRify / SyntaxFlow / SSA Risk ReAct loops
// that must stay import-light: attachment wire constants, loop variable key names, IRify attachment
// sync helpers, SSA overview filter building, and reflection-output appendix text.
//
// SyntaxFlow **scan** orchestration, stage markdown, and LoopAction factories such as
// [github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_syntaxflow_scan] live in that
// package, not here.
//
// # Attachment contract (Yakit / wire strings are stable)
//
//	Go name in this package	Wire Type / Key / Value
//	IrifyTypeSyntaxFlow	"irify_syntaxflow"
//	IrifyTypeSSARisk	"irify_ssa_risk"
//	IrifyTypeSSARisksFilter	"irify_ssa_risks_filter"
//	IrifyTypeSyntaxFlowRule	"irify_syntaxflow_rule"
//	IrifyKeyTaskID … IrifyKeyFullQuality	keys: task_id, session_mode, programs, risk_id, filter_json, runtime_id, program_name, full_quality
//	SessionModeAttach / SessionModeStart	values: attach, start
//
// Intake 使用 [SyncSyntaxFlowLoopVarsFromIrifyTask] 将底座附件写入下面「标准 loop 键」；session_mode=start 为**新扫**并忽略
// irify 随附的 task_id；session_mode=attach 为**附着**。
//
// # Standard loop keys (const LoopVar* in loop_vars.go)
//
//	Constant string value	Role
//	syntaxflow_task_id	附着模式下的 scan task / runtime
//	syntaxflow_scan_session_mode	attach | start
//	sf_rule_full_quality	"true" 时规则全文质量
//	ssa_risk_id	SSA 风险行主键（review 等）
//	ssa_risks_filter_json	ypb.SSARisksFilter 的 protojson
//	ssa_overview_filter_json	overview reload 时持久化的有效 filter
//	sf_scan_config_json	与 yak code-scan --config 同族的完整 JSON
//	project_path	可选，用于派生最小同进程 code-scan JSON
//	sf_scan_pipeline_summary / sf_scan_scan_end_summary / …	管线与用户向阶段文（多阶段解释子环）
//
// # Sync helpers and filter build (read attachments once → loop)
//
//   - [SyncSyntaxFlowLoopVarsFromIrifyTask] — P1 intake 入口：session_mode、full_quality、task_id（非 start）
//   - [SyncSSARiskIDFromIrifyToLoop] — 仅当 loop 上尚无 ssa_risk_id
//   - [SyncSSARisksFilterFromIrifyToLoop] — 仅当 loop 上尚无 ssa_risks_filter_json
//   - [BuildSSARisksFilterFromLoop] — 消费路径只读 loop 上的 ssa_risks_filter_json + 短 user query，不再读附件
package syntaxflow_utils
