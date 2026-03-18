package syntaxflow_scan

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/diagnostics"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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

	doQuery := func() error {
		option := []ssaapi.QueryOption{
			ssaapi.QueryWithContext(m.ctx),
			ssaapi.QueryWithTaskID(m.taskID),
			ssaapi.QueryWithProcessCallback(func(f float64, info string) {
				m.processMonitor.UpdateRuleStatus(prog.GetProgramName(), rule.RuleName, f, info)
			}),
			ssaapi.QueryWithSave(m.kind),
			ssaapi.QueryWithProjectId(m.Config.GetProjectID()),
		}
		if m.Config.GetSyntaxFlowMemory() {
			option = append(option, ssaapi.QueryWithMemory())
		}

		var err error
		var res *ssaapi.SyntaxFlowResult
		if overlay := prog.GetOverlay(); overlay != nil {
			res, err = overlay.SyntaxFlowRule(rule, option...)
		} else {
			res, err = prog.SyntaxFlowRule(rule, option...)
		}

		if err == nil {
			m.StatusTask(res)
			m.markRuleSuccess()
		} else {
			m.processMonitor.UpdateRuleError(prog.GetProgramName(), rule.RuleName, err)
			m.StatusTask(nil)
			m.markRuleFailed()
			m.errorCallback("program %s exc rule %s failed: %s", prog.GetProgramName(), rule.RuleName, err)
		}
		return nil
	}

	// 仅在启用规则级性能且等级满足时走 Track 路径，否则直接执行，避免 diagnostics 影响扫描结果
	if m.ruleProfiler != nil {
		_, _ = m.ruleProfiler.ForKind(ssa.TrackKindScan).TrackHigh(rule.RuleName, doQuery)
	} else {
		_ = doQuery()
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

// logScanSummary 使用 LogTable，内部仅 LevelHigh 时输出表格
func (m *scanManager) logScanSummary() {
	diagnostics.LogLow(ssa.TrackKindScan, "", fmt.Sprintf("total scan elapsed %v", m.scanDuration))
	if m.ruleProfiler != nil {
		snap := m.ruleProfiler.Snapshot()
		if len(snap) > 0 {
			headers, rows := diagnostics.MeasurementsToRows(snap)
			if len(rows) > 0 {
				diagnostics.LogTable(ssa.TrackKindScan, &diagnostics.TablePayload{Title: "Scan Performance Summary", Headers: headers, Rows: rows}, false)
			}
		}
	}
	summaryData := map[string]string{
		"Total Scan Time": m.scanDuration.String(),
		"Total Rules":     fmt.Sprintf("%d", m.processMonitor.TotalQuery.Load()),
		"Success Rules":   fmt.Sprintf("%d", m.processMonitor.SuccessQuery.Load()),
		"Failed Rules":    fmt.Sprintf("%d", m.processMonitor.FailedQuery.Load()),
		"Risk Count":      fmt.Sprintf("%d", m.processMonitor.RiskCount.Load()),
	}
	headers, rows := diagnostics.MapToRows(summaryData)
	diagnostics.LogTable(ssa.TrackKindScan, &diagnostics.TablePayload{Title: "Scan Summary", Headers: headers, Rows: rows}, false)
}
