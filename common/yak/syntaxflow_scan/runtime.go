package syntaxflow_scan

import (
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func (m *scanManager) startQuerySF(startIndex ...int64) error {
	if m == nil {
		return utils.Errorf("SyntaxFlowScan Failed: scanManager or stream is nil")
	}
	defer func() {
		if err := recover(); err != nil {
			m.taskRecorder.Reason = fmt.Sprintf("%v", err)
			m.status = schema.SYNTAXFLOWSCAN_ERROR
		} else {
			// 暂停or结束
			if m.isFinishScan() {
				m.status = schema.SYNTAXFLOWSCAN_DONE
			} else {
				m.status = schema.SYNTAXFLOWSCAN_PAUSED
			}
		}
		m.saveReport()
		m.notifyStatus()
		_ = m.SaveTask()
	}()

	var start int64
	if len(startIndex) == 0 {
		start = 0
	} else {
		start = startIndex[0]
	}
	if start > m.totalQuery || start < 0 {
		return utils.Errorf("SyntaxFlow scan start with a wrong task index")
	}

	var errs error
	var taskIndex atomic.Int64
	var concurrency int
	if m.ssaConfig.GetScanConcurrency() <= 0 {
		concurrency = 5
	} else {
		concurrency = int(m.ssaConfig.GetScanConcurrency())
	}
	swg := utils.NewSizedWaitGroup(concurrency)
	for rule := range m.ruleChan {
		if m.IsPause() || m.IsStop() {
			break
		}
		for _, progName := range m.ProgramNames {
			if m.IsPause() || m.IsStop() {
				break
			}

			taskIndex.Add(1)
			if taskIndex.Load() <= start {
				continue
			}

			swg.Add()
			go func(rule *schema.SyntaxFlowRule, progName string) {
				defer func() {
					m.notifyStatus()
					swg.Done()
				}()
				// TODO:传入实例
				prog, err := ssaapi.FromDatabase(progName)
				if err != nil {
					m.markRuleSkipped()
					return
				}
				f1 := func() {
					m.Query(rule, prog)
				}
				ssaprofile.ProfileAdd(true, "manager.query", f1)
			}(rule, progName)
		}
	}
	swg.Wait()
	m.notifyStatus()
	return errs
}

func (m *scanManager) isFinishScan() bool {
	if m.finishedQuery >= m.totalQuery {
		return true
	}
	return false
}

func (m *scanManager) Query(rule *schema.SyntaxFlowRule, prog *ssaapi.Program) {
	m.notifyStatus()
	defer m.SaveTask()

	if m.ssaConfig.GetScanIgnoreLanguage() {
		if rule.Language != string(consts.General) && rule.Language != prog.GetLanguage() {
			m.markRuleSkipped()
			return
		}
	}
	option := []ssaapi.QueryOption{}
	option = append(option,
		ssaapi.QueryWithContext(m.ctx),
		ssaapi.QueryWithTaskID(m.taskID),
		ssaapi.QueryWithProcessCallback(func(f float64, s string) {
			m.notifyRuleProcess(prog.GetProgramName(), rule.RuleName, f)
		}),
		ssaapi.QueryWithSave(m.kind),
	)
	if m.ssaConfig.GetScanMemory() {
		option = append(option, ssaapi.QueryWithMemory())
	}

	// if language match or ignore language
	if res, err := prog.SyntaxFlowRule(rule, option...); err == nil {
		m.markRuleSuccess()
		m.notifyResult(res)
	} else {
		m.markRuleFailed()
		if m.client != nil {
			m.client.YakitError("program %s exc rule %s failed: %s", prog.GetProgramName(), rule.RuleName, err)
		}
	}
}
func (m *scanManager) notifyResult(res *ssaapi.SyntaxFlowResult) {
	if riskLen := len(res.GetGRPCModelRisk()); riskLen != 0 {
		m.addRiskCount(int64(riskLen))
	}
	for key, count := range res.GetRiskCountMap() {
		m.riskCountMap.Set(key, count)
	}

	if m.reporter != nil {
		m.reporter.AddSyntaxFlowResult(res)
	}
	// TODO:notify for  stream
	//m.stream.Send(&ypb.SyntaxFlowScanResponse{
	//	TaskID:   m.taskID,
	//	Status:   m.status,
	//	Result:   res.GetGRPCModelResult(),
	//	SSARisks: res.GetGRPCModelRisk(),
	//})
}

func (m *scanManager) notifyStatus() {
	finishQuery := atomic.LoadInt64(&m.finishedQuery)
	successQuery := atomic.LoadInt64(&m.successQuery)
	failedQuery := atomic.LoadInt64(&m.failedQuery)
	skipQuery := atomic.LoadInt64(&m.skipQuery)
	riskCount := atomic.LoadInt64(&m.riskCount)
	if finishQuery == m.totalQuery {
		m.status = schema.SYNTAXFLOWSCAN_DONE
	}
	process := float64(finishQuery) / float64(m.totalQuery)
	// process
	if m.client != nil {
		m.client.StatusCard("已执行规则", fmt.Sprintf("%d/%d", finishQuery, m.totalQuery), "规则执行状态")
		m.client.StatusCard("已跳过规则", skipQuery, "规则执行状态")
		m.client.StatusCard("执行成功个数", successQuery, "规则执行状态")
		m.client.StatusCard("执行失败个数", failedQuery, "规则执行状态")
		m.client.StatusCard("检出漏洞/风险个数", riskCount, "漏洞/风险状态")
		m.client.YakitSetProgress(process)
	}
	if m.ssaConfig.GetScanProcessCallback() != nil {
		m.ssaConfig.GetScanProcessCallback()(process)
	}
}

func (m *scanManager) notifyRuleProcess(progName, ruleName string, f float64) {
	output := struct {
		ProgName string `json:"项目名称"`
		RuleName string `json:"规则名称"`
		Progress string `json:"执行进度"`
	}{
		ProgName: progName,
		RuleName: ruleName,
		Progress: fmt.Sprintf("%.2f%%", f*100),
	}
	marshal, err := json.Marshal(output)
	if err != nil {
		return
	}
	if m.client != nil {
		m.client.Output(marshal)
	}
}

func (m *scanManager) saveReport() {
	if m == nil || m.reporter == nil {
		return
	}
	if err := m.reporter.Save(m.reporterWriter); err != nil {
		log.Errorf("save report failed: %v", err)
	}
}
