package syntaxflow_scan

import "github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"

// ProductStage is a product-facing scan phase. IDs stay stable for StatusCard
// / CLI / gRPC; DisplayName is what operators see.
type ProductStage string

const (
	// StageCollect: clone, extract, or open the project tree.
	StageCollect ProductStage = "collect"
	// StageInspect: source-mode regex over files (no IR).
	StageInspect ProductStage = "inspect"
	// StageReview: compile-unit semantic checks (struct mode).
	StageReview ProductStage = "review"
	// StageAnalyze: SSA dataflow / taint.
	StageAnalyze ProductStage = "analyze"
)

// DisplayName is the operator-facing label. Hierarchy: 收集 → 检测 → 检测(语义) → 分析(深度).
func (s ProductStage) DisplayName() string {
	switch s {
	case StageCollect:
		return "收集代码"
	case StageInspect:
		return "代码检测"
	case StageReview:
		return "语义检测"
	case StageAnalyze:
		return "深度分析"
	default:
		return string(s)
	}
}

// LegacyPhase is the older script StatusCard value (clone/source-scan/compile/scan)
// so existing monitors keep working until they switch to DisplayName.
func (s ProductStage) LegacyPhase() string {
	switch s {
	case StageCollect:
		return "clone"
	case StageInspect:
		return "source-scan"
	case StageReview:
		return "compile"
	case StageAnalyze:
		return "scan"
	default:
		return string(s)
	}
}

func (s ProductStage) overallRange() (start, end float64) {
	switch s {
	case StageCollect:
		return 0.00, 0.10
	case StageInspect:
		return 0.10, 0.30
	case StageReview:
		return 0.30, 0.60
	case StageAnalyze:
		return 0.60, 1.00
	default:
		return 0, 1
	}
}

// OverallProgress maps a 0-1 stage fraction onto the full product bar.
func (s ProductStage) OverallProgress(stageProgress float64) float64 {
	if stageProgress < 0 {
		stageProgress = 0
	}
	if stageProgress > 1 {
		stageProgress = 1
	}
	start, end := s.overallRange()
	return start + (end-start)*stageProgress
}

// StageCallback is invoked whenever a product stage starts, moves, or finishes.
// overall is 0-1 across the whole ScanProject run; progress is 0-1 inside the stage.
type StageCallback func(stage ProductStage, overall, progress float64, info *RuleProcessInfoList)

const stageCallbackKey = "syntaxflow-scan/stageCallback"

var withStageCallbackOption = ssaconfig.SetOption(stageCallbackKey, func(c *Config, callback StageCallback) {
	if c.ScanTaskCallback == nil {
		c.ScanTaskCallback = &ScanTaskCallback{}
	}
	c.stageCallback = callback
})

// WithStageCallback reports collect / inspect / review / analyze progress
// (export: syntaxflow.withStageCallback).
func WithStageCallback(callback StageCallback) ssaconfig.Option {
	return withStageCallbackOption(callback)
}
