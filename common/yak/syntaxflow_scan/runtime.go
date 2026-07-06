package syntaxflow_scan

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/diagnostics"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func (m *scanManager) StartQuerySF(startIndex ...int64) error {
	scanStart := time.Now()
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
		for _, prog := range m.Config.Programs {
			if m.IsPause() || m.IsStop() {
				break
			}

			taskIndex.Add(1)
			if taskIndex.Load() <= start {
				continue
			}

			swg.Add()
			go func(rule *schema.SyntaxFlowRule, prog *ssaapi.Program) {
				defer m.SaveTask()
				defer swg.Done()
				if utils.IsNil(prog) {
					log.Errorf("SyntaxFlow Scan Failed:the program to search is nil")
					return
				}
				m.Query(rule, prog)
			}(rule, prog)
		}
	}
	swg.Wait()
	return errs
}

func (m *scanManager) Query(rule *schema.SyntaxFlowRule, prog *ssaapi.Program) {
	// 语言匹配检查
	if !m.Config.GetScanIgnoreLanguage() {
		if rule.Language != ssaconfig.General && rule.Language != prog.GetLanguage() {
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
	workLimit := m.Config.GetScanRuleWorkLimit()
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
				m.processMonitor.UpdateRuleStatus(prog.GetProgramName(), rule.RuleName, f, info)
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

		// 执行规则查询
		var err error
		var res *ssaapi.SyntaxFlowResult
		if overlay := prog.GetOverlay(); overlay != nil {
			res, err = overlay.SyntaxFlowRule(rule, option...)
		} else {
			res, err = prog.SyntaxFlowRule(rule, option...)
		}

		// Detect a per-rule budget bail. ruleCtx.Err() is non-nil once the
		// deadline fired OR the total-work budget cancelled ruleCtx, regardless
		// of whether the query surfaced the ctx error or returned partial/empty
		// results with a nil err — so this catches both. workBudget.Exceeded()
		// covers the work-budget case even when ruleTimeout==0 (no wall-clock
		// deadline, so ruleCtx.Err() alone wouldn't flag it).
		bailedByBudget := (ruleTimeout > 0 && ruleCtx.Err() != nil) || (workBudget != nil && workBudget.Exceeded())
		bailReason := ""
		switch {
		case workBudget != nil && workBudget.Exceeded():
			bailReason = fmt.Sprintf("work-limit=%d", workLimit)
		case ruleTimeout > 0:
			bailReason = ruleTimeout.String()
		}

		if bailedByBudget {
			// A per-rule budget bail (wall-clock RuleTimeout or total-work
			// RuleWorkLimit) is a CONTROLLED PARTIAL: the rule produced what it
			// could before the budget fired. Count it as success so heavy rules
			// don't surface as spurious failures on large projects; the warn +
			// error callback record the bail reason. The query may surface the
			// ctx cancellation as err (e.g. "context done") — that's the expected
			// bail signal, not a real rule failure.
			if res != nil {
				m.StatusTask(res)
			}
			m.markRuleSuccess()
			log.Warnf("rule %s on program %s hit per-rule budget (%s), returned partial results",
				rule.RuleName, prog.GetProgramName(), bailReason)
			m.errorCallback("program %s exc rule %s hit per-rule budget (%s), bailed",
				prog.GetProgramName(), rule.RuleName, bailReason)
		} else if err == nil {
			m.StatusTask(res)
			m.markRuleSuccess()
		} else {
			m.processMonitor.UpdateRuleError(prog.GetProgramName(), rule.RuleName, err)
			m.StatusTask(nil)
			m.markRuleFailed()
			m.errorCallback("program %s exc rule %s failed: %s",
				prog.GetProgramName(), rule.RuleName, err)
		}

		// 在规则执行完成后输出性能日志
		if enableRulePerf && ruleRecorder != nil {
			// 确保性能日志输出，即使日志级别较高
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
				// 即使没有数据，也输出提示信息
				log.Debugf("Rule Performance: %s - no performance data recorded", rule.RuleName)
			}
		}

		// Release this rule's analysis-local accumulators before the next rule
		// reuses the program. ResetInterRuleState is a no-op unless the cache
		// exceeds its threshold (heavy rules), so small rules keep DB-read reuse
		// while heavy rules' Values don't carry Predecessors/anchorBits into the
		// next rule and don't bloat retained memory. See Program.ResetInterRuleState.
		if prog != nil {
			prog.ResetInterRuleState()
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
