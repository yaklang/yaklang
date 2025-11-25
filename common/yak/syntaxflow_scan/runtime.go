package syntaxflow_scan

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func (m *scanManager) StartQuerySF(startIndex ...int64) error {
	scanStart := time.Now()
	defer func() {
		// 输出性能统计报告
		enableRulePerf := m.Config.IsEnableRulePerformanceLog()
		totalDuration := time.Since(scanStart)
		m.logScanPerformance(totalDuration, enableRulePerf)

		if err := recover(); err != nil {
			log.Errorf("error: panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
			m.taskRecorder.Reason = fmt.Sprintf("%v", err)
			m.status = schema.SYNTAXFLOWSCAN_ERROR
		}
		m.saveReport()
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
		for _, progName := range m.Config.GetProgramNames() {
			if m.IsPause() || m.IsStop() {
				break
			}

			taskIndex.Add(1)
			if taskIndex.Load() <= start {
				continue
			}

			swg.Add()
			go func(rule *schema.SyntaxFlowRule, progName string) {
				defer m.SaveTask()
				defer swg.Done()

				prog, err := ssaapi.FromDatabase(progName)
				if err != nil {
					m.markRuleSkipped()
					return
				}
				m.Query(rule, prog)
			}(rule, progName)
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

	// 将查询逻辑包装到函数中
	f := func() {
		option := []ssaapi.QueryOption{}
		option = append(option,
			ssaapi.QueryWithContext(m.ctx),
			ssaapi.QueryWithTaskID(m.taskID),
			ssaapi.QueryWithProcessCallback(func(f float64, info string) {
				m.processMonitor.UpdateRuleStatus(prog.GetProgramName(), rule.RuleName, f, info)
			}),
			ssaapi.QueryWithSave(m.kind),
			ssaapi.QueryWithProjectId(m.Config.GetProjectID()),
		)
		if m.Config.GetSyntaxFlowMemory() {
			option = append(option, ssaapi.QueryWithMemory())
		}
		if enableRulePerf {
			option = append(option, ssaapi.QueryWithRuleDiagnosticsRecorder())
		}

		// 执行规则查询
		if res, err := prog.SyntaxFlowRule(rule, option...); err == nil {
			m.StatusTask(res)
			m.markRuleSuccess()
		} else {
			m.processMonitor.UpdateRuleError(prog.GetProgramName(), rule.RuleName, err)
			m.StatusTask(nil)
			m.markRuleFailed()
			m.errorCallback("program %s exc rule %s failed: %s", prog.GetProgramName(), rule.RuleName, err)
		}
	}

	// 根据配置决定是否记录规则级别的详细性能
	if enableRulePerf && m.ruleProfiler != nil {
		// 构建 profile 名称：Rule[规则名].Prog[程序名]
		profileName := fmt.Sprintf("Rule[%s].Prog[%s]", rule.RuleName, prog.GetProgramName())
		m.ruleProfiler.Track(true, profileName, f)
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
	log.Infof("=== Scan Total ===")
	log.Infof("Time: %v", totalDuration)
	log.Infof("==================")

	if enableRulePerf && m.ruleProfiler != nil {
		m.ruleProfiler.Log("Rule Performance (scan)")
	}

}
