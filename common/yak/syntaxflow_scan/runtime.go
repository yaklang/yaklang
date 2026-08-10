package syntaxflow_scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/diagnostics"

	"github.com/yaklang/yaklang/common/schema"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const (
	// autoScaleWorkLimitLineThreshold is the source-line threshold at which the
	// CLI default per-rule work limit is auto-scaled. On Hadoop-scale repos
	// (2-3M lines) the 200k budget bails ~21 rules that would otherwise need a
	// larger fanout budget; a 5M-instruction graph is simply more work than the
	// small-project default.
	autoScaleWorkLimitLineThreshold = 500_000
	// maxAutoScaledRuleWorkLimit caps the auto-scale so a pathological rule
	// cannot grow the budget without bound. The per-rule wall-clock budget still
	// applies as the hard backstop.
	maxAutoScaledRuleWorkLimit = 1_000_000
)

// effectiveRuleWorkLimit returns the per-rule work budget for a program. The
// auto-scale applies only when the caller used the CLI default (200k): non-
// default explicit limits (including 0=disabled) are never modified.
func effectiveRuleWorkLimit(configured int64, totalLines int) int64 {
	if configured != ssaconfig.DefaultScanRuleWorkLimit || totalLines < autoScaleWorkLimitLineThreshold {
		return configured
	}
	scaled := configured * (int64(totalLines)/autoScaleWorkLimitLineThreshold + 1)
	if scaled > maxAutoScaledRuleWorkLimit {
		scaled = maxAutoScaledRuleWorkLimit
	}
	return scaled
}

func (m *scanManager) StartQuerySF(startIndex ...int64) error {
	scanStart := time.Now()
	applySSAScanPoolSize()
	defer func() {
		// 记录扫描耗时，但不在这里输出 Scan Summary
		// Scan Summary 应该在 processMonitor.Close() 之后输出，以确保所有 callback 都已完成
		m.scanDuration = time.Since(scanStart)

		if err := recover(); err != nil {
			log.Errorf("error: panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
			m.taskRecorder.Reason = fmt.Sprintf("%v", err)
			m.status = schema.SYNTAXFLOWSCAN_ERROR
		}
	}()

	var start int64
	if len(startIndex) == 0 {
		start = 0
	} else {
		start = startIndex[0]
	}
	if start > m.processMonitor.TotalQuery.Load() || start < 0 {
		return utils.Errorf("SyntaxFlow scan start with a wrong task index")
	}

	var errs error
	var taskIndex atomic.Int64
	var concurrency int
	if m.Config.GetScanConcurrency() <= 0 {
		concurrency = 5
	} else {
		concurrency = int(m.Config.GetScanConcurrency())
	}
	swg := utils.NewSizedWaitGroup(concurrency)
	for rule := range m.ruleChan {
		if m.IsPause() || m.IsStop() {
			break
		}
		targets := m.Config.QueryTargets
		if len(targets) == 0 {
			for _, prog := range m.Config.Programs {
				if prog != nil {
					targets = append(targets, prog)
				}
			}
		}
		for _, target := range targets {
			if m.IsPause() || m.IsStop() {
				break
			}

			taskIndex.Add(1)
			if taskIndex.Load() <= start {
				continue
			}

			swg.Add()
			go func(rule *schema.SyntaxFlowRule, target ssaapi.SyntaxFlowQueryInstance) {
				defer m.SaveTask()
				defer swg.Done()
				if utils.IsNil(target) {
					log.Errorf("SyntaxFlow Scan Failed:the query target is nil")
					return
				}
				m.Query(rule, target)
			}(rule, target)
		}
	}
	swg.Wait()
	return errs
}

// applySSAScanPoolSize expands the SSA SQLite connection pool right before the
// scan phase starts. Compile-time writes keep the default single-writer pool;
// once SaveToDatabase has finished, reads can safely use more connections and
// actually use the machine's cores. Controlled by YAK_SSA_SQLITE_MAX_OPEN_CONNS
// (default: unchanged).
func applySSAScanPoolSize() {
	raw := strings.TrimSpace(os.Getenv("YAK_SSA_SQLITE_MAX_OPEN_CONNS"))
	if raw == "" {
		return
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 16 {
		return
	}
	db := consts.GetGormSSAProjectDataBase()
	if db == nil || db.DB() == nil {
		return
	}
	db.DB().SetMaxOpenConns(n)
	db.DB().SetMaxIdleConns(n)
	log.Infof("ssa scan pool size: %d (expanded after compile writes)", n)
}

func queryTargetName(target ssaapi.SyntaxFlowQueryInstance) string {
	if target == nil {
		return ""
	}
	return target.GetProgramName()
}

func (m *scanManager) Query(rule *schema.SyntaxFlowRule, target ssaapi.SyntaxFlowQueryInstance) {
	// 语言匹配检查
	if !m.Config.GetScanIgnoreLanguage() {
		if rule.Language != ssaconfig.General && rule.Language != target.GetLanguage() {
			m.markRuleSkipped()
			return
		}
	}

	// 检查是否启用规则级别的详细性能监控
	enableRulePerf := m.Config.IsEnableRulePerformanceLog()

	// Per-rule wall-clock budget: derive a deadline context so a pathological
	// rule (e.g. dataflow(include=...) matching tens of thousands of sources on
	// a large project) is bailed at the budget instead of hanging the scan. The
	// deadline propagates via QueryWithContext -> OperationConfig.ctx ->
	// AnalyalyzeContext, which checks ctx.Done() at every recursive dataflow
	// step (analyze_context.go). 0 means no budget (legacy behavior).
	ruleTimeout := m.Config.GetScanRuleTimeout()
	ruleCtx := m.ctx
	var ruleCancel context.CancelFunc
	// A per-rule cancelable ctx is ALWAYS created (even when ruleTimeout==0) so
	// the total-work budget below has a per-rule cancel to trip without
	// affecting the whole scan ctx. The deadline still propagates via
	// QueryWithContext -> OperationConfig.ctx -> AnalyzeContext.check ctx.Done().
	if ruleTimeout > 0 {
		ruleCtx, ruleCancel = context.WithTimeout(m.ctx, ruleTimeout)
	} else {
		ruleCtx, ruleCancel = context.WithCancel(m.ctx)
	}
	defer ruleCancel()

	// Per-rule total-work budget: bounds cumulative fanout work (per
	// <typeName>/<getReturns>/.../dataflow source element) across the rule's
	// opcodes so a heavy rule bails at N operations instead of doing tens of
	// millions of per-element MergeAnchor(Clone)+AppendPredecessor ops that hang
	// for hours even within the wall-clock budget. Exceeding it cancels ruleCtx
	// (via ruleCancel) so the existing ctx-bail path surfaces partial results.
	// 0 means no work budget (only the wall-clock RuleTimeout applies).
	targetName := queryTargetName(target)
	baseWorkLimit := m.Config.GetScanRuleWorkLimit()
	totalLines := 0
	if lineCounter, ok := target.(interface{ TotalLines() int }); ok {
		totalLines = lineCounter.TotalLines()
	}
	workLimit := effectiveRuleWorkLimit(baseWorkLimit, totalLines)
	if workLimit != baseWorkLimit {
		if _, loaded := m.autoScaleLogged.LoadOrStore(targetName, struct{}{}); !loaded {
			log.Infof("rule work limit auto-scaled for program %s: %d -> %d (source lines=%d)",
				targetName, baseWorkLimit, workLimit, totalLines)
		}
	}
	var workBudget *sf.RuleWorkBudget
	if workLimit > 0 {
		workBudget = sf.NewRuleWorkBudget(workLimit, ruleCancel)
	}

	// 将查询逻辑包装到函数中
	f := func() error {
		var ruleRecorder *diagnostics.Recorder
		option := []ssaapi.QueryOption{}
		option = append(option,
			ssaapi.QueryWithContext(ruleCtx),
			ssaapi.QueryWithTaskID(m.taskID),
			ssaapi.QueryWithProcessCallback(func(f float64, info string) {
				m.processMonitor.UpdateRuleStatus(targetName, rule.RuleName, f, info)
			}),
			ssaapi.QueryWithSave(m.kind),
			ssaapi.QueryWithProjectId(m.Config.GetProjectID()),
		)
		if workBudget != nil {
			option = append(option, ssaapi.QueryWithWorkBudget(workBudget))
		}
		if m.Config.GetSyntaxFlowMemory() {
			option = append(option, ssaapi.QueryWithMemory())
		}
		if enableRulePerf {
			ruleRecorder = diagnostics.NewRecorder()
			option = append(option, ssaapi.QueryWithRuleDiagnosticsRecorder(ruleRecorder))
		}

		// QueryTargets already selected Program or ProgramOverLay — no overlay branch.
		res, err := target.SyntaxFlowRule(rule, option...)

		// Detect a per-rule budget bail. A parent scan cancellation is not a
		// rule-timeout event, so check the cancellation cause instead of treating
		// every non-nil ruleCtx.Err() as a timeout. workBudget.Exceeded() covers
		// the work-budget case even when ruleTimeout==0.
		bailedByWork := workBudget != nil && workBudget.Exceeded()
		bailedByTimeout := ruleTimeout > 0 && errors.Is(context.Cause(ruleCtx), context.DeadlineExceeded)
		bailedByBudget := bailedByWork || bailedByTimeout
		bailReason := ""
		switch {
		case bailedByWork:
			bailReason = fmt.Sprintf("work-limit=%d visited=%d", workBudget.Limit(), workBudget.Visited())
		case bailedByTimeout:
			bailReason = fmt.Sprintf("rule-timeout=%s", ruleTimeout)
		}

		if bailedByBudget {
			if res != nil {
				m.StatusTask(res)
			}
			m.markRuleSuccess()
			log.Warnf("rule %s on program %s hit per-rule budget (%s), returned partial results",
				rule.RuleName, targetName, bailReason)
			m.processMonitor.UpdateRuleStatus(targetName, rule.RuleName, 1,
				fmt.Sprintf("partial result: per-rule budget (%s)", bailReason))
			m.errorCallback("program %s exc rule %s hit per-rule budget (%s), bailed",
				targetName, rule.RuleName, bailReason)
		} else if err == nil {
			m.StatusTask(res)
			m.markRuleSuccess()
		} else {
			m.processMonitor.UpdateRuleError(targetName, rule.RuleName, err)
			m.StatusTask(nil)
			m.markRuleFailed()
			m.errorCallback("program %s exc rule %s failed: %s",
				targetName, rule.RuleName, err)
		}

		if enableRulePerf && ruleRecorder != nil {
			snapshots := ruleRecorder.Snapshot()
			if len(snapshots) > 0 {
				log.Info("========================================")
				log.Infof("Rule Performance: %s", rule.RuleName)
				log.Info("========================================")
				for _, snapshot := range snapshots {
					log.Info(snapshot.String())
				}
				log.Info("========================================")
			} else {
				log.Debugf("Rule Performance: %s - no performance data recorded", rule.RuleName)
			}
		}

		// Explicit cleanup: release rule-local references to help GC reclaim
		// SyntaxFlowResult, recorder, and work budget immediately after the
		// rule completes (success, failure, or budget bail). This is defensive
		// nil'ing — the variables would go out of scope when f() returns, but
		// explicit nil'ing helps the GC reclaim large result objects before
		// the next rule starts allocating.
		res = nil
		ruleRecorder = nil
		workBudget = nil

		// Release this rule's analysis-local accumulators before the next rule
		// reuses the program. ResetInterRuleState is a no-op unless the cache
		// exceeds its threshold (heavy rules), so small rules keep DB-read reuse
		// while heavy rules' Values don't carry Predecessors/anchorBits into the
		// next rule and don't bloat retained memory. See Program.ResetInterRuleState.
		if resetter, ok := target.(interface{ ResetInterRuleState() }); ok {
			resetter.ResetInterRuleState()
		}
		return nil
	}

	// 根据配置决定是否记录规则级别的详细性能
	if enableRulePerf && m.ruleProfiler != nil {
		// 构建 profile 名称：只使用规则名
		profileName := rule.RuleName
		m.ruleProfiler.Track(profileName, f)
	} else {
		// 不启用性能监控时，直接执行
		f()
	}
}

func (m *scanManager) notifyResult(res *ssaapi.SyntaxFlowResult) {
	if m.Config.Reporter != nil {
		m.Config.Reporter.AddSyntaxFlowResult(res)
	}
	m.processMonitor.RiskCount.Add(int64(res.RiskCount()))
	if m.Config.resultCallback != nil {
		m.Config.resultCallback(&ScanResult{
			TaskID: m.taskID,
			Status: m.status,
			Result: res,
		})
	}
}

func (m *scanManager) saveReport() {
	if m == nil || m.Config == nil || m.Config.Reporter == nil {
		return
	}
	if err := m.Config.Reporter.Save(); err != nil {
		log.Errorf("save report failed: %v", err)
	}
}

func (m *scanManager) errorCallback(format string, a ...interface{}) {
	m.callback.Error(m.taskID, m.status, format, a...)
	log.Errorf(format, a...)
}

// logScanPerformance 记录扫描性能统计报告
func (m *scanManager) logScanPerformance(totalDuration time.Duration, enableRulePerf bool) {
	// 使用 log.Info 确保性能日志总是输出
	log.Info("=== Scan Total ===")
	log.Infof("Time: %v", totalDuration)
	log.Info("==================")
	if os.Getenv("YAK_SSA_FAST_MATCH_DEBUG") != "" {
		hits, fallbacks := ssaapi.FastPathMatchStats()
		log.Infof("fast-path match stats: hits=%d fallbacks=%d", hits, fallbacks)
	}

	if enableRulePerf && m.ruleProfiler != nil {
		snapshots := m.ruleProfiler.Snapshot()
		if len(snapshots) > 0 {
			// 生成并输出性能汇总表格
			table := diagnostics.FormatPerformanceTable("Scan Performance Summary", snapshots)
			log.Info("\n" + table)
		} else {
			log.Infof("Rule Performance (scan): no data recorded")
		}
	}
	// 总是输出扫描汇总表格
	summaryData := map[string]string{
		"Total Scan Time": totalDuration.String(),
		"Total Rules":     fmt.Sprintf("%d", m.processMonitor.TotalQuery.Load()),
		"Success Rules":   fmt.Sprintf("%d", m.processMonitor.SuccessQuery.Load()),
		"Failed Rules":    fmt.Sprintf("%d", m.processMonitor.FailedQuery.Load()),
		"Risk Count":      fmt.Sprintf("%d", m.processMonitor.RiskCount.Load()),
	}
	table := diagnostics.FormatSimpleTable("Scan Summary", summaryData)
	log.Info("\n" + table)
}
