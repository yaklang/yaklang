package syntaxflow_scan

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// A failing detection stage after a successful sibling keeps the run
// successful: the product rule is "any successful detection stage wins".
func TestStageOutcomeRecorder_AnyDetectionStageSucceeds(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.record(StageCollect, nil)
	recorder.record(StageInspect, nil)
	recorder.record(StageAnalyze, fmt.Errorf("ssa rule set failed"))

	require.True(t, recorder.Succeeded(), "one successful detection stage must win")

	var failed []StageOutcome
	for _, outcome := range recorder.Outcomes() {
		if !outcome.Succeeded() {
			failed = append(failed, outcome)
		}
	}
	require.Len(t, failed, 1)
	require.Equal(t, StageAnalyze, failed[0].Stage)
	require.Equal(t, StageStatusFailed, failed[0].Status)
	require.NotEmpty(t, failed[0].Error)
}

// Collect alone is not a successful scan.
func TestStageOutcomeRecorder_CollectAloneIsNotSuccess(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.record(StageCollect, nil)
	recorder.record(StageInspect, fmt.Errorf("source rule set failed"))

	require.False(t, recorder.Succeeded())
}

// Compile-only success still requires collect plus the compile stage.
func TestProductStageRankDoesNotRegress(t *testing.T) {
	require.Greater(t, ProductStageRank(StageReview), ProductStageRank(StageInspect))
	require.Greater(t, ProductStageRank(StageAnalyze), ProductStageRank(StageReview))
	require.Equal(t, ProductStageRank(StageReview), ProductStageRank(StageCompile))
}

func TestStageOutcomeJSONKeepsZeroDetectionCounts(t *testing.T) {
	raw, err := json.Marshal(StageOutcome{Stage: StageInspect, Status: StageStatusSucceeded})
	require.NoError(t, err)
	require.Contains(t, string(raw), `"rule_count":0`)
	require.Contains(t, string(raw), `"risk_count":0`)
}

func TestStageOutcomeRecorder_CompileOnlySuccess(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.record(StageCollect, nil)
	recorder.record(StageCompile, nil)

	require.True(t, recorder.Succeeded())
	require.Equal(t, "编译源码", StageCompile.DisplayName())
	require.Equal(t, "compile", StageCompile.LegacyPhase())
}

// No mode at all resolves to compile-only; explicit modes stay as selected.
func TestResolveProductModes(t *testing.T) {
	compileOnly := resolveProductModes(&Config{})
	require.True(t, compileOnly.compileOnly)
	require.False(t, compileOnly.source)
	require.False(t, compileOnly.review)
	require.False(t, compileOnly.analyze)

	noMode := resolveProductModes(&Config{ScanTaskCallback: &ScanTaskCallback{}})
	require.True(t, noMode.compileOnly)

	cfg := &Config{ScanTaskCallback: &ScanTaskCallback{}}
	cfg.scanModes = []string{SourceMode, SSAMode}
	selected := resolveProductModes(cfg)
	require.False(t, selected.compileOnly)
	require.True(t, selected.source)
	require.False(t, selected.review)
	require.True(t, selected.analyze)
}

// JSON rule_filter_mode is the product intent when withMode was not applied.
func TestResolveProductModes_RuleFilterModeWithoutWithMode(t *testing.T) {
	cfg := &Config{
		Config: &ssaconfig.Config{
			Mode: ssaconfig.ModeAll,
			SyntaxFlowRule: &ssaconfig.SyntaxFlowRuleConfig{
				RuleFilterMode: []string{SourceMode},
			},
		},
		ScanTaskCallback: &ScanTaskCallback{},
	}
	selected := resolveProductModes(cfg)
	require.False(t, selected.compileOnly, "inspect-only JSON must not become compile-only")
	require.True(t, selected.source)
	require.False(t, selected.review)
	require.False(t, selected.analyze)
}

// Unknown mode values must not silently degrade into compile-only.
func TestResolveProductModes_UnknownValueFallsBackToAll(t *testing.T) {
	cfg := &Config{ScanTaskCallback: &ScanTaskCallback{}}
	cfg.scanModes = []string{"not-a-mode"}
	selected := resolveProductModes(cfg)
	require.False(t, selected.compileOnly)
	require.True(t, selected.source)
	require.True(t, selected.review)
	require.True(t, selected.analyze)
}

// A stage outcome carries the evidence a platform renders: status, error,
// wall time, and the rule/risk counters observed while the stage ran.
func TestStageOutcomeRecorder_CarriesStageMetrics(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.enter(StageInspect)
	recorder.observe(StageInspect, &RuleProcessInfoList{TotalQuery: 12, RiskCount: 3})
	recorder.record(StageInspect, fmt.Errorf("rule set aborted"))

	require.Len(t, recorder.Outcomes(), 1)
	outcome := recorder.Outcomes()[0]
	require.Equal(t, StageInspect, outcome.Stage)
	require.Equal(t, StageStatusFailed, outcome.Status)
	require.Equal(t, "rule set aborted", outcome.Error)
	require.EqualValues(t, 12, outcome.RuleCount)
	require.EqualValues(t, 3, outcome.RiskCount)
}

// Streamed risks are attributed to the stage that produced them, and counted
// cumulatively when a stage streams several batches.
func TestParseCompileScale(t *testing.T) {
	info := parseCompileScale(`ssa-compile-scale:{"total_files":12,"handler_files":8,"prehandler_files":12,"total_bytes":4096}`)
	require.NotNil(t, info)
	require.EqualValues(t, 12, info.TotalFiles)
	require.EqualValues(t, 8, info.HandlerFiles)
	require.EqualValues(t, 4096, info.TotalBytes)
	require.Nil(t, parseCompileScale("compiling php"))
}

func TestStageOutcomeRecorder_ObserveScale(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.observeScale(&RuleProcessInfoList{TotalFiles: 12, TotalBytes: 4096, TotalLines: 80})
	scale := recorder.Scale()
	require.EqualValues(t, 12, scale.TotalFiles)
	require.EqualValues(t, 4096, scale.TotalBytes)
	require.EqualValues(t, 80, scale.TotalLines)
}

func TestStageOutcomeRecorder_AccumulatesStreamedRisks(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.enter(StageAnalyze)
	recorder.addRisk(StageAnalyze, 2)
	recorder.addRisk(StageAnalyze, 5)
	recorder.record(StageAnalyze, nil)

	outcome := recorder.Outcomes()[0]
	require.True(t, outcome.Succeeded())
	require.EqualValues(t, 7, outcome.RiskCount)
}

func TestStageOutcomeRecorder_ConcurrentResults(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	const workers = 32
	const resultsPerWorker = 32
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for result := 0; result < resultsPerWorker; result++ {
				recorder.addRule(StageInspect, fmt.Sprintf("rule-%d-%d", worker, result))
				recorder.addRisk(StageInspect, 1)
			}
		}(worker)
	}
	wg.Wait()
	recorder.record(StageInspect, nil)

	outcome := recorder.Outcomes()[0]
	require.EqualValues(t, workers*resultsPerWorker, outcome.RuleCount)
	require.EqualValues(t, workers*resultsPerWorker, outcome.RiskCount)
}

// Source rules call addRule from parallel result callbacks. Concurrent map
// writes here used to abort the whole scan with "fatal error: concurrent map writes".
func TestStageOutcomeRecorder_ParallelSourceResults(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	var wg sync.WaitGroup
	const n = 64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("rule-%d", i)
			recorder.enter(StageInspect)
			recorder.addRule(StageInspect, name)
			recorder.addRisk(StageInspect, 1)
			recorder.observe(StageInspect, &RuleProcessInfoList{TotalQuery: 1, TotalFiles: int64(i + 1)})
			recorder.observeScale(&RuleProcessInfoList{TotalLines: int64(i + 1)})
			_ = recorder.Succeeded()
			_ = recorder.Outcomes()
			_ = recorder.Scale()
		}(i)
	}
	wg.Wait()
	recorder.record(StageInspect, nil)
	outcomes := recorder.Outcomes()
	require.Len(t, outcomes, 1)
	require.EqualValues(t, n, outcomes[0].RuleCount)
	require.EqualValues(t, n, outcomes[0].RiskCount)
}

func TestStageOutcomeRecorder_IgnoresBlankRuleNames(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.addRule(StageInspect, " ")
	recorder.addRule(StageInspect, "rule-a")
	recorder.record(StageInspect, nil)

	require.EqualValues(t, 1, recorder.Outcomes()[0].RuleCount)
}

func TestStageOutcomeRecorder_AnalyzedSourceKeepsExistingFileCount(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.observeScale(&RuleProcessInfoList{TotalFiles: 12, TotalLines: 3})
	recorder.observeAnalyzedSource(&ssaapi.SourceStatistics{AnalyzedLineCount: 80, AnalyzedFileCount: 4})

	scale := recorder.Scale()
	require.EqualValues(t, 12, scale.TotalFiles)
	require.EqualValues(t, 80, scale.TotalLines)
}

func TestStageOutcomeRecorder_ParallelStagesStayIndependent(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			recorder.addRule(StageInspect, fmt.Sprintf("source-%d", i))
			recorder.addRisk(StageInspect, 1)
		}(i)
		go func(i int) {
			defer wg.Done()
			recorder.addRule(StageReview, fmt.Sprintf("review-%d", i))
			recorder.observe(StageReview, &RuleProcessInfoList{TotalQuery: int64(i + 1)})
		}(i)
	}
	wg.Wait()
	recorder.record(StageInspect, nil)
	recorder.record(StageReview, nil)

	outcomes := recorder.Outcomes()
	require.Len(t, outcomes, 2)
	require.Equal(t, StageInspect, outcomes[0].Stage)
	require.EqualValues(t, 32, outcomes[0].RuleCount)
	require.EqualValues(t, 32, outcomes[0].RiskCount)
	require.Equal(t, StageReview, outcomes[1].Stage)
	require.EqualValues(t, 32, outcomes[1].RuleCount)
	require.Zero(t, outcomes[1].RiskCount)
}

func TestSkippedRequestedStages_DetectsUnstartedDetection(t *testing.T) {
	mode := productModeSelection{source: true, review: true, analyze: true}
	skipped := skippedRequestedStages(mode, []StageOutcome{
		{Stage: StageCollect, Status: StageStatusSucceeded},
		{Stage: StageInspect, Status: StageStatusSucceeded},
		{Stage: StageReview, Status: StageStatusSucceeded},
	})
	require.Equal(t, []string{string(StageAnalyze)}, skipped)

	none := skippedRequestedStages(productModeSelection{source: true}, []StageOutcome{
		{Stage: StageInspect, Status: StageStatusFailed, Error: "rule failed"},
	})
	require.Empty(t, none)
}

func TestMergeStageInfoKeepsCompileScaleAndStructRules(t *testing.T) {
	scale := &RuleProcessInfoList{TotalFiles: 12, TotalBytes: 4096, TotalLines: 80}
	rules := &RuleProcessInfoList{
		TotalQuery:    2,
		FinishedQuery: 2,
		SuccessQuery:  2,
		RiskCount:     3,
		Rules:         []*RuleProcessInfo{{RuleName: "php-struct", Finished: true, RiskCount: 3}},
	}
	got := mergeStageInfo(scale, rules)
	require.EqualValues(t, 12, got.TotalFiles)
	require.EqualValues(t, 80, got.TotalLines)
	require.EqualValues(t, 2, got.TotalQuery)
	require.EqualValues(t, 3, got.RiskCount)
	require.Len(t, got.Rules, 1)
	require.Equal(t, "php-struct", got.Rules[0].RuleName)
}

// A stage streams its rule/risk callbacks from several goroutines at once, so
// the recorder has to be safe to touch concurrently. Unsynchronized map writes
// used to abort the whole process with "fatal error: concurrent map writes"
// and left the report file empty.
func TestStageOutcomeRecorder_ConcurrentUpdates(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	recorder.enter(StageInspect)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			recorder.addRule(StageInspect, fmt.Sprintf("rule-%d", i))
			recorder.addRisk(StageInspect, 1)
			recorder.observe(StageInspect, &RuleProcessInfoList{
				TotalQuery: int64(i),
				RiskCount:  int64(i),
			})
			recorder.observeScale(&RuleProcessInfoList{TotalFiles: int64(i + 1)})
		}(i)
	}
	wg.Wait()

	recorder.record(StageInspect, nil)

	outcome := recorder.Outcomes()[0]
	require.EqualValues(t, 32, outcome.RuleCount)
	// addRisk accumulates while observe keeps the maximum, so the risk total
	// depends on the interleaving; only the lower bound is deterministic.
	require.GreaterOrEqual(t, outcome.RiskCount, int64(32))
	require.Positive(t, recorder.Scale().TotalFiles)
}

// structScanCountsStub reports fixed struct-stage counts and records how often
// it was queried, so the caller can be checked without a compiled program.
type structScanCountsStub struct {
	rules   int
	results int
	calls   atomic.Int64
}

func (s *structScanCountsStub) StructScanCounts() (int, int) {
	s.calls.Add(1)
	return s.rules, s.results
}

// observeStruct must take the recorder lock exactly once. Taking it twice
// self-deadlocks on the non-reentrant sync.Mutex, which hangs the whole
// ScanProject call: that is how this was caught on CI.
func TestStageOutcomeRecorder_ObserveStructDoesNotSelfDeadlock(t *testing.T) {
	recorder := newStageOutcomeRecorder()
	prog := &structScanCountsStub{rules: 3, results: 7}

	done := make(chan struct{})
	go func() {
		defer close(done)
		recorder.observeStruct(prog)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("observeStruct deadlocked: it must not lock the recorder mutex twice")
	}

	require.EqualValues(t, 1, prog.calls.Load(), "counts must be read once")
	metrics := recorder.metrics[StageReview]
	require.EqualValues(t, 3, metrics.ruleCount)
	require.EqualValues(t, 7, metrics.riskCount)

	// A later call with smaller counts keeps the maximum; observeStruct records
	// the high-water mark rather than the latest sample.
	recorder.observeStruct(&structScanCountsStub{rules: 1, results: 2})
	metrics = recorder.metrics[StageReview]
	require.EqualValues(t, 3, metrics.ruleCount)
	require.EqualValues(t, 7, metrics.riskCount)
}
