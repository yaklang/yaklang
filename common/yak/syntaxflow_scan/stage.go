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
	// StageCompile: compile-only run. Persists IR and reports program_name
	// without running rule sets; product surfaces render it as the compile
	// step instead of a rule-detection stage.
	StageCompile ProductStage = "compile"
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
	case StageCompile:
		return "编译源码"
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
	case StageReview, StageCompile:
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
	case StageCompile:
		return 0.10, 1.00
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

// StageStatus is the terminal state of one product stage in a ScanProject run.
// Only stages that actually ran are reported; unselected stages stay absent so
// product surfaces can render "successful stages only".
type StageStatus string

const (
	StageStatusSucceeded StageStatus = "succeeded"
	StageStatusFailed    StageStatus = "failed"
)

// StageOutcome is the terminal result of one executed product stage.
type StageOutcome struct {
	Stage  ProductStage `json:"stage"`
	Status StageStatus  `json:"status"`
	Error  string       `json:"error,omitempty"`
}

func (o StageOutcome) Succeeded() bool { return o.Status == StageStatusSucceeded }

// stageOutcomeRecorder collects terminal stage results for one ScanProject run
// and decides the aggregate result: any successful detection stage makes the
// whole run successful even when a sibling stage failed.
type stageOutcomeRecorder struct {
	outcomes []StageOutcome
}

func newStageOutcomeRecorder() *stageOutcomeRecorder { return &stageOutcomeRecorder{} }

func (r *stageOutcomeRecorder) record(stage ProductStage, err error) {
	if r == nil {
		return
	}
	outcome := StageOutcome{Stage: stage, Status: StageStatusSucceeded}
	if err != nil {
		outcome.Status = StageStatusFailed
		outcome.Error = err.Error()
	}
	r.outcomes = append(r.outcomes, outcome)
}

// Succeeded applies the product rule: the run succeeds when the source was
// collected and at least one detection stage succeeded. A compile-only run
// succeeds through its review (compile) stage.
func (r *stageOutcomeRecorder) Succeeded() bool {
	if r == nil {
		return false
	}
	collected := false
	detected := false
	for _, outcome := range r.outcomes {
		if !outcome.Succeeded() {
			continue
		}
		switch outcome.Stage {
		case StageCollect:
			collected = true
		case StageInspect, StageReview, StageAnalyze, StageCompile:
			detected = true
		}
	}
	return collected && detected
}

// Outcomes returns the recorded stage results in run order.
func (r *stageOutcomeRecorder) Outcomes() []StageOutcome {
	if r == nil {
		return nil
	}
	return append([]StageOutcome(nil), r.outcomes...)
}

// ProjectResult is the terminal summary of one ScanProject run. Platforms parse
// exactly this shape instead of inferring stage state from job chains: Stages
// carries only the stages that actually executed, Succeeded applies the
// "any successful detection stage wins" product rule, and ProgramName lets a
// compile-only run publish the IR it persisted for later reuse.
type ProjectResult struct {
	Stages      []StageOutcome `json:"stages"`
	ProgramName string         `json:"program_name,omitempty"`
	Succeeded   bool           `json:"succeeded"`
	Error       string         `json:"error,omitempty"`
}

// ProjectResultCallback receives the terminal ProjectResult of one ScanProject
// run, including runs that ended in an error.
type ProjectResultCallback func(ProjectResult)

const projectResultCallbackKey = "syntaxflow-scan/projectResultCallback"

var withProjectResultCallbackOption = ssaconfig.SetOption(projectResultCallbackKey, func(c *Config, callback ProjectResultCallback) {
	if c.ScanTaskCallback == nil {
		c.ScanTaskCallback = &ScanTaskCallback{}
	}
	c.projectResultCallback = callback
})

// WithProjectResultCallback reports the terminal ProjectResult of a ScanProject
// run (export: syntaxflow.withProjectResultCallback). It fires once, after the
// last executed stage, and also fires for failed runs.
func WithProjectResultCallback(callback ProjectResultCallback) ssaconfig.Option {
	return withProjectResultCallbackOption(callback)
}
