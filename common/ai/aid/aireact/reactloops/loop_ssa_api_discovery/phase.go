package loop_ssa_api_discovery

const (
	PhaseInitialized  = "initialized"
	PhaseSSADone      = "ssa_done"
	PhaseArchAnalyzed = "arch_analyzed"
	PhaseCoreAnalyzed = "core_analyzed"

	// 编排流水线后续阶段（会话 phase 字段）
	PhaseApiVerified    = "api_verified"
	PhaseVulnScanned    = "vuln_scanned"
	PhaseVulnVerified   = "vuln_verified"
	PhasePipelineReport = "pipeline_report_done"

	// Phase5 sub-step phases
	PhaseStep0ChecklistDone = "phase5_step0_done"
	PhaseStep1AuthDone      = "phase5_step1_done"
	PhaseStep2StaticDone    = "phase5_step2_done"
)

const runtimeKey = "ssa_discovery_runtime"
