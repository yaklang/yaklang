package syntaxflow_scan

import (
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

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

// ProductStageRank is the product order: collect < inspect < review < analyze.
// Compile sits with review. Unknown stages do not move the live cursor.
func ProductStageRank(stage ProductStage) int {
	switch stage {
	case StageCollect:
		return 1
	case StageInspect:
		return 2
	case StageReview, StageCompile:
		return 3
	case StageAnalyze:
		return 4
	default:
		return 0
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

// StageOutcome is the terminal result of one executed product stage. Only
// stages that actually ran are reported, so product surfaces can render
// "successful stages only" without re-deriving what ran.
type StageOutcome struct {
	Stage  ProductStage `json:"stage"`
	Status StageStatus  `json:"status"`
	Error  string       `json:"error,omitempty"`
	// DurationMs is the wall time this stage occupied. Absent when unknown.
	DurationMs int64 `json:"duration_ms,omitempty"`
	// RuleCount / RiskCount come from the stage's own process and result
	// callbacks, so a reader gets one authoritative per-stage summary.
	// Zero is still a real count for a detection stage that ran.
	RuleCount          int64                   `json:"rule_count"`
	RiskCount          int64                   `json:"risk_count"`
	CompileDiagnostics *ssa.CompileDiagnostics `json:"compile_diagnostics,omitempty"`
	FailedRules        int64                   `json:"failed_rules,omitempty"`
}

func (o StageOutcome) Succeeded() bool { return o.Status == StageStatusSucceeded }

// stageOutcomeRecorder collects terminal stage results for one ScanProject run
// and decides the aggregate result: any successful detection stage makes the
// whole run successful even when a sibling stage failed.
//
// A stage streams its per-rule progress from several goroutines at once, so
// every mutation and read of the counters is guarded by mu. Without it the
// concurrent map writes aborted the whole process with a fatal error and the
// report file was left empty.
type stageOutcomeRecorder struct {
	mu sync.Mutex

	outcomes           []StageOutcome
	started            map[ProductStage]time.Time
	metrics            map[ProductStage]stageMetrics
	ruleNames          map[ProductStage]map[string]struct{}
	scale              compileScale
	sourceStatistics   any
	compileDiagnostics map[ProductStage]ssa.CompileDiagnostics
}

// stageMetrics accumulates what one stage actually did while it ran.
type stageMetrics struct {
	ruleCount   int64
	riskCount   int64
	failedRules int64
}

// compileScale is the filesystem size signal emitted after clone/extract.
type compileScale struct {
	TotalFiles      int64
	HandlerFiles    int64
	PrehandlerFiles int64
	TotalBytes      int64
	TotalLines      int64
}

func newStageOutcomeRecorder() *stageOutcomeRecorder {
	return &stageOutcomeRecorder{
		started:   map[ProductStage]time.Time{},
		metrics:   map[ProductStage]stageMetrics{},
		ruleNames: map[ProductStage]map[string]struct{}{},
	}
}

// enter marks a stage as running so its wall time can be reported once it
// reaches a terminal state. Repeated calls keep the first timestamp, matching
// the first progress callback the stage emits.
func (r *stageOutcomeRecorder) enter(stage ProductStage) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, seen := r.started[stage]; !seen {
		r.started[stage] = time.Now()
	}
}

// observe folds one process callback into the running stage's counters.
func (r *stageOutcomeRecorder) observe(stage ProductStage, info *RuleProcessInfoList) {
	if r == nil || info == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	metrics := r.metrics[stage]
	if info.FailedQuery > metrics.failedRules {
		metrics.failedRules = info.FailedQuery
	}
	if info.TotalQuery > metrics.ruleCount {
		metrics.ruleCount = info.TotalQuery
	} else if info.FinishedQuery > metrics.ruleCount {
		metrics.ruleCount = info.FinishedQuery
	}
	if info.RiskCount > metrics.riskCount {
		metrics.riskCount = info.RiskCount
	}
	r.metrics[stage] = metrics
	r.observeScaleLocked(info)
}

// addRisk counts risks a stage streamed through the result callback.
func (r *stageOutcomeRecorder) addRisk(stage ProductStage, count int64) {
	if r == nil || count <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	metrics := r.metrics[stage]
	metrics.riskCount += count
	r.metrics[stage] = metrics
}

func (r *stageOutcomeRecorder) addRule(stage ProductStage, name string) {
	name = strings.TrimSpace(name)
	if r == nil || name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ruleNames == nil {
		r.ruleNames = map[ProductStage]map[string]struct{}{}
	}
	set := r.ruleNames[stage]
	if set == nil {
		set = map[string]struct{}{}
		r.ruleNames[stage] = set
	}
	set[name] = struct{}{}
	metrics := r.metrics[stage]
	if n := int64(len(set)); n > metrics.ruleCount {
		metrics.ruleCount = n
		r.metrics[stage] = metrics
	}
}

// observeScale keeps the last compile-scale / source-size snapshot so the
// platform can render 收集代码 代码行 / 文件 / 体积 after the run.
func (r *stageOutcomeRecorder) observeScale(info *RuleProcessInfoList) {
	if r == nil || info == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observeScaleLocked(info)
}

func (r *stageOutcomeRecorder) observeScaleLocked(info *RuleProcessInfoList) {
	if info.TotalFiles > 0 {
		r.scale.TotalFiles = info.TotalFiles
	}
	if info.HandlerFiles > 0 {
		r.scale.HandlerFiles = info.HandlerFiles
	}
	if info.PrehandlerFiles > 0 {
		r.scale.PrehandlerFiles = info.PrehandlerFiles
	}
	if info.TotalBytes > 0 {
		r.scale.TotalBytes = info.TotalBytes
	}
	if info.TotalLines > 0 {
		r.scale.TotalLines = info.TotalLines
	}
}

func (r *stageOutcomeRecorder) observeStruct(prog interface{ StructScanCounts() (int, int) }) {
	if r == nil || prog == nil {
		return
	}
	rules, results := prog.StructScanCounts()
	r.mu.Lock()
	defer r.mu.Unlock()
	metrics := r.metrics[StageReview]
	if int64(rules) > metrics.ruleCount {
		metrics.ruleCount = int64(rules)
	}
	if int64(results) > metrics.riskCount {
		metrics.riskCount = int64(results)
	}
	r.metrics[StageReview] = metrics
}

// observeAnalyzedSource records the analyzed source size reported by a compiled
// program, filling in the scale the process callbacks may not have produced.
func (r *stageOutcomeRecorder) observeAnalyzedSource(stats *ssaapi.SourceStatistics) {
	if r == nil || stats == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sourceStatistics = stats
	if stats.AnalyzedLineCount > 0 {
		r.scale.TotalLines = stats.AnalyzedLineCount
	}
	if stats.AnalyzedFileCount > 0 && r.scale.TotalFiles == 0 {
		r.scale.TotalFiles = stats.AnalyzedFileCount
	}
}

// Scale returns the compile-scale snapshot recorded so far.
func (r *stageOutcomeRecorder) Scale() compileScale {
	if r == nil {
		return compileScale{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.scale
}

// SourceStatistics returns the recorded source-size statistics.
func (r *stageOutcomeRecorder) SourceStatistics() any {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sourceStatistics
}

func (r *stageOutcomeRecorder) record(stage ProductStage, err error) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	outcome := StageOutcome{Stage: stage, Status: StageStatusSucceeded}
	if err != nil {
		outcome.Status = StageStatusFailed
		outcome.Error = err.Error()
	}
	if startedAt, ok := r.started[stage]; ok {
		if elapsed := time.Since(startedAt).Milliseconds(); elapsed > 0 {
			outcome.DurationMs = elapsed
		}
	}
	if metrics, ok := r.metrics[stage]; ok {
		outcome.RuleCount = metrics.ruleCount
		outcome.RiskCount = metrics.riskCount
		outcome.FailedRules = metrics.failedRules
	}
	if diagnostic, ok := r.compileDiagnostics[stage]; ok && diagnostic.Incomplete() {
		outcome.CompileDiagnostics = &diagnostic
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
	r.mu.Lock()
	defer r.mu.Unlock()
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
	r.mu.Lock()
	defer r.mu.Unlock()
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
	// IncompleteStages is true when the caller asked for detection stages
	// that never started. The run can still Succeeded when an earlier
	// detection stage completed.
	IncompleteStages bool     `json:"incomplete_stages,omitempty"`
	SkippedStages    []string `json:"skipped_stages,omitempty"`
	// Source size for 收集代码. Platforms persist these with the artifact so
	// diagnostics can show 代码行 / 文件 / 体积 after the process events expire.
	TotalFiles       int64 `json:"total_files,omitempty"`
	HandlerFiles     int64 `json:"handler_files,omitempty"`
	PrehandlerFiles  int64 `json:"prehandler_files,omitempty"`
	TotalBytes       int64 `json:"total_bytes,omitempty"`
	TotalLines       int64 `json:"total_lines,omitempty"`
	SourceStatistics any   `json:"source_statistics,omitempty"`
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
