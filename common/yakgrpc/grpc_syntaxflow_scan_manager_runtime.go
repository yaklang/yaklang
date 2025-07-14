package yakgrpc

import (
	"fmt"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (m *SyntaxFlowScanManager) StartQuerySF(startIndex ...int64) error {
	if m == nil || m.stream == nil {
		return utils.Errorf("SyntaxFlowScanManager or stream is nil")
	}
	defer func() {
		if err := recover(); err != nil {
			m.taskRecorder.Reason = fmt.Sprintf("%v", err)
			m.status = schema.SYNTAXFLOWSCAN_ERROR
		}
		m.notifyStatus("")
		m.SaveTask()
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

	cache := utils.NewSafeMap[*ssaapi.Program]()
	getProgram := func(name string) (*ssaapi.Program, error) {
		if prog, ok := cache.Get(name); ok {
			return prog, nil
		}
		prog, err := ssaapi.FromDatabase(name)
		if err != nil {
			return nil, err
		}
		cache.Set(name, prog)
		return prog, nil
	}

	var errs error
	var taskIndex int64

	swg := utils.NewSizedWaitGroup(int(m.GetConcurrency()))

	for rule := range m.ruleChan {
		if m.IsPause() || m.IsStop() {
			break
		}
		for _, progName := range m.programs {
			if m.IsPause() || m.IsStop() {
				break
			}
			taskIndex++
			if taskIndex <= start {
				continue
			}

			swg.Add(1)
			go func(rule *schema.SyntaxFlowRule, progName string) {
				defer swg.Done()

				prog, err := getProgram(progName)
				if err != nil {
					atomic.AddInt64(&m.skipQuery, 1)
					return
				}
				m.Query(rule, prog)
			}(rule, progName)
		}
	}
	swg.Wait()
	m.notifyStatus("")
	return errs
}

func (m *SyntaxFlowScanManager) Query(rule *schema.SyntaxFlowRule, prog *ssaapi.Program) {
	m.notifyStatus(rule.RuleName)
	defer m.SaveTask()
	// log.Infof("executing rule %s", rule.RuleName)
	if !m.ignoreLanguage {
		if rule.Language != string(consts.General) && string(rule.Language) != prog.GetLanguage() {
			atomic.AddInt64(&m.skipQuery, 1)
			// m.client.YakitInfo("program %s(lang:%s) exec rule %s(lang:%s) failed: language not match", programName, prog.GetLanguage(), rule.RuleName, rule.Language)
			return
		}
	}

	// if language match or ignore language
	if res, err := prog.SyntaxFlowRule(rule, ssaapi.QueryWithContext(m.ctx),
		ssaapi.QueryWithTaskID(m.taskID), ssaapi.QueryWithSave(m.kind),
		ssaapi.QueryWithProcessCallback(func(f float64, s string) {
			m.client.StatusCard("当前执行规则进度", fmt.Sprintf("%.2f%%", f*100), "规则执行进度")
		}),
	); err == nil {
		atomic.AddInt64(&m.successQuery, 1)
		m.notifyResult(res)
	} else {
		atomic.AddInt64(&m.failedQuery, 1)
		m.client.YakitError("program %s exc rule %s failed: %s", prog.GetProgramName(), rule.RuleName, err)
	}
}
func (m *SyntaxFlowScanManager) notifyResult(res *ssaapi.SyntaxFlowResult) {
	if riskLen := len(res.GetGRPCModelRisk()); riskLen != 0 {
		atomic.AddInt64(&m.riskCount, int64(riskLen))
	}
	for key, count := range res.GetRiskCountMap() {
		m.riskCountMap[key] = count
	}
	// m.riskQuery
	m.stream.Send(&ypb.SyntaxFlowScanResponse{
		TaskID:   m.taskID,
		Status:   m.status,
		Result:   res.GetGRPCModelResult(),
		SSARisks: res.GetGRPCModelRisk(),
	})
}

func (m *SyntaxFlowScanManager) notifyStatus(ruleName string) {
	successQuery := atomic.LoadInt64(&m.successQuery)
	failedQuery := atomic.LoadInt64(&m.failedQuery)
	skipQuery := atomic.LoadInt64(&m.skipQuery)
	riskCount := atomic.LoadInt64(&m.riskCount)

	finishQuery := successQuery + failedQuery + skipQuery
	// process
	m.client.StatusCard("已执行规则", fmt.Sprintf("%d/%d", finishQuery, m.totalQuery), "规则执行状态")
	m.client.StatusCard("已跳过规则", skipQuery, "规则执行状态")
	// runtime status
	m.client.StatusCard("执行成功个数", successQuery, "规则执行状态")
	m.client.StatusCard("执行失败个数", failedQuery, "规则执行状态")
	// risk status
	m.client.StatusCard("检出漏洞/风险个数", riskCount, "漏洞/风险状态")

	// current rule  status
	if finishQuery == m.totalQuery {
		m.status = schema.SYNTAXFLOWSCAN_DONE
		m.client.StatusCard("当前执行规则", "已执行完毕", "规则执行进度")
	} else {
		if ruleName != "" {
			m.client.StatusCard("当前执行规则", ruleName, "规则执行进度")
		}
	}
	m.client.YakitSetProgress(float64(finishQuery) / float64(m.totalQuery))
}
