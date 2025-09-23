package syntaxflow_scan

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (m *scanManager) StartQuerySF(startIndex ...int64) error {
	if m == nil || m.stream == nil {
		return utils.Errorf("scanManager or stream is nil")
	}
	defer func() {
		if err := recover(); err != nil {
			m.taskRecorder.Reason = fmt.Sprintf("%v", err)
			m.status = schema.SYNTAXFLOWSCAN_ERROR
		}
		m.StatusTask()
		m.SaveTask()
		m.saveReport()
	}()

	// wait for pause signal
	go func() {
		for {
			rsp, err := m.stream.Recv()
			if err != nil {
				m.taskRecorder.Reason = err.Error()
				return
			}
			if rsp.GetControlMode() == "pause" {
				m.status = schema.SYNTAXFLOWSCAN_PAUSED
				m.Pause()
				m.Stop()
			}
		}
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

	swg := utils.NewSizedWaitGroup(int(m.GetConcurrency()))

	for rule := range m.ruleChan {
		if m.IsPause() || m.IsStop() {
			break
		}
		for _, progName := range m.programs {
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
					m.StatusTask()
					swg.Done()
				}()

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
	m.StatusTask()
	return errs
}

func (m *scanManager) Query(rule *schema.SyntaxFlowRule, prog *ssaapi.Program) {
	m.notifyStatus()
	defer m.SaveTask()

	if !m.ignoreLanguage {
		if rule.Language != string(consts.General) && string(rule.Language) != prog.GetLanguage() {
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
	if m.memory {
		option = append(option, ssaapi.QueryWithMemory())
	}

	// if language match or ignore language
	if res, err := prog.SyntaxFlowRule(rule, option...); err == nil {
		m.markRuleSuccess()
		m.notifyResult(res)
	} else {
		m.markRuleFailed()
		m.client.YakitError("program %s exc rule %s failed: %s", prog.GetProgramName(), rule.RuleName, err)
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
		f1 := func() {
			m.reporter.AddSyntaxFlowResult(res)
		}
		ssaprofile.ProfileAdd(true, "convert result to report", f1)
	}
	m.stream.Send(&ypb.SyntaxFlowScanResponse{
		TaskID:   m.taskID,
		Status:   m.status,
		Result:   res.GetGRPCModelResult(),
		SSARisks: res.GetGRPCModelRisk(),
	})
}

func (m *scanManager) notifyStatus() {
	finishQuery := atomic.LoadInt64(&m.finishedQuery)
	successQuery := atomic.LoadInt64(&m.successQuery)
	failedQuery := atomic.LoadInt64(&m.failedQuery)
	skipQuery := atomic.LoadInt64(&m.skipQuery)
	riskCount := atomic.LoadInt64(&m.riskCount)
	// process
	m.client.StatusCard("已执行规则", fmt.Sprintf("%d/%d", finishQuery, m.totalQuery), "规则执行状态")
	m.client.StatusCard("已跳过规则", skipQuery, "规则执行状态")
	m.client.StatusCard("执行成功个数", successQuery, "规则执行状态")
	m.client.StatusCard("执行失败个数", failedQuery, "规则执行状态")
	m.client.StatusCard("检出漏洞/风险个数", riskCount, "漏洞/风险状态")
	if finishQuery == m.totalQuery {
		m.status = schema.SYNTAXFLOWSCAN_DONE
	}

	process := float64(finishQuery) / float64(m.totalQuery)
	m.client.YakitSetProgress(process)
	if m.processCallback != nil {
		m.processCallback(process)
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
	m.client.Output(marshal)
	if m.ruleProcessCallback != nil {
		m.ruleProcessCallback(progName, ruleName, f)
	}

}

func (m *scanManager) saveReport() {
	if m == nil || m.reporter == nil {
		return
	}
	err := m.reporter.Save()
	if err != nil {
		log.Errorf("save report failed: %v", err)
	}
	if m.reporter != nil {
		err := m.reporter.PrettyWrite(m.reporterWriter)
		if err != nil {
			log.Errorf("write report failed: %v", err)
		}
	}
}
