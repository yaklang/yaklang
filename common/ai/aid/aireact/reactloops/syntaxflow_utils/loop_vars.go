package syntaxflow_utils

// Standard ReAct loop keys (WithVar / loop.Set) for IRify + SyntaxFlow + SSA risk overview flows.
// Intake 应在进入各阶段前用 Sync*FromIrify* 将底座附件一次写入本表所列键，后续只读 loop.Get。
// 与 Irify 线串、Sync 方法名的对应关系见本包顶部的包文档（doc.go）。
const (
	LoopVarSyntaxFlowTaskID          = "syntaxflow_task_id"
	LoopVarSyntaxFlowScanSessionMode = "syntaxflow_scan_session_mode"
	LoopVarSFRuleFullQuality         = "sf_rule_full_quality"
	LoopVarSSARiskID                 = "ssa_risk_id"
	LoopVarSSARisksFilterJSON        = "ssa_risks_filter_json"
	// LoopVarSSAOverviewFilterJSON stores the last effective SSARisksFilter (protojson) for reload_ssa_risk_overview without parameters.
	LoopVarSSAOverviewFilterJSON = "ssa_overview_filter_json"
	// LoopVarSFScanConfigJSON: full code-scan JSON (same as yak `code-scan --config`).
	LoopVarSFScanConfigJSON = "sf_scan_config_json"
	// LoopVarProjectPath: optional absolute directory for minimal in-process code-scan JSON (orchestrator / WithVar).
	LoopVarProjectPath = "project_path"

	// Multi-stage pipeline + final report (interpret sub-loop; engine-filled).
	LoopVarSFPipelineSummary  = "sf_scan_pipeline_summary"  // 各阶段一行行累积摘要
	LoopVarSFScanEndSummary   = "sf_scan_scan_end_summary"  // 扫描任务行终态时一次性写入
	LoopVarSFFinalReportDue   = "sf_scan_final_report_due"  // "1" 时模型必须输出大总结
	LoopVarSFCompileMeta      = "sf_scan_compile_meta"      // 编译起止与项目名等
	LoopVarSFUserStageLog     = "sf_scan_user_stage_log"    // 阶段化用户向正文（# / ##）
	LoopVarSFRiskConverged    = "sf_scan_risk_converged"     // 风险侧收敛
)
