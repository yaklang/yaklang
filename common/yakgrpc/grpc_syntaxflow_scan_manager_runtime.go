package yakgrpc

// 迁移到 common/yak/syntaxflow_scan
//import (
//	"encoding/json"
//	"fmt"
//	"sync/atomic"
//
//	"github.com/yaklang/yaklang/common/consts"
//	"github.com/yaklang/yaklang/common/schema"
//	"github.com/yaklang/yaklang/common/utils"
//	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
//	"github.com/yaklang/yaklang/common/yak/ssaapi"
//	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
//)
//
//func (m *SyntaxFlowScanManager) StartQuerySF(startIndex ...int64) error {
//	if m == nil || m.stream == nil {
//		return utils.Errorf("SyntaxFlowScanManager or stream is nil")
//	}
//	defer func() {
//		if err := recover(); err != nil {
//			m.taskRecorder.Reason = fmt.Sprintf("%v", err)
//			m.status = schema.SYNTAXFLOWSCAN_ERROR
//		}
//		m.StatusTask()
//		m.SaveTask()
//	}()
//
//	// wait for pause signal
//	go func() {
//		for {
//			rsp, err := m.stream.Recv()
//			if err != nil {
//				m.taskRecorder.Reason = err.Error()
//				return
//			}
//			if rsp.GetControlMode() == "pause" {
//				m.status = schema.SYNTAXFLOWSCAN_PAUSED
//				m.Pause()
//				m.Stop()
//			}
//		}
//	}()
//
//	var start int64
//	if len(startIndex) == 0 {
//		start = 0
//	} else {
//		start = startIndex[0]
//	}
//	if start > m.totalQuery || start < 0 {
//		return utils.Errorf("SyntaxFlow scan start with a wrong task index")
//	}
//
//	var errs error
//	var taskIndex atomic.Int64
//
//	swg := utils.NewSizedWaitGroup(int(m.GetConcurrency()))
//
//	for rule := range m.ruleChan {
//		if m.IsPause() || m.IsStop() {
//			break
//		}
//		for _, progName := range m.programs {
//			if m.IsPause() || m.IsStop() {
//				break
//			}
//
//			taskIndex.Add(1)
//			if taskIndex.Load() <= start {
//				continue
//			}
//
//			swg.Add()
//			go func(rule *schema.SyntaxFlowRule, progName string) {
//				defer func() {
//					m.StatusTask()
//					swg.Done()
//				}()
//
//				prog, err := ssaapi.FromDatabase(progName)
//				if err != nil {
//					m.markRuleSkipped()
//					return
//				}
//				f1 := func() {
//					m.Query(rule, prog)
//				}
//				ssaprofile.ProfileAdd(true, "manager.query", f1)
//			}(rule, progName)
//		}
//	}
//	swg.Wait()
//	m.StatusTask()
//	return errs
//}
//
//func (m *SyntaxFlowScanManager) Query(rule *schema.SyntaxFlowRule, prog *ssaapi.Program) {
//	m.notifyStatus(rule.RuleName)
//	defer m.SaveTask()
//	// log.Infof("executing rule %s", rule.RuleName)
//	if !m.ignoreLanguage {
//		if rule.Language != string(consts.General) && string(rule.Language) != prog.GetLanguage() {
//			m.markRuleSkipped()
//			// m.client.YakitInfo("program %s(lang:%s) exec rule %s(lang:%s) failed: language not match", programName, prog.GetLanguage(), rule.RuleName, rule.Language)
//			return
//		}
//	}
//	option := []ssaapi.QueryOption{}
//	option = append(option,
//		ssaapi.QueryWithContext(m.ctx),
//		ssaapi.QueryWithTaskID(m.taskID),
//		ssaapi.QueryWithProcessCallback(func(f float64, s string) {
//			//m.client.StatusCard("当前执行规则进度", fmt.Sprintf("%.2f%%", f*100), "规则执行进度")
//			m.notifyRuleProcess(prog.GetProgramName(), rule.RuleName, f)
//		}),
//		ssaapi.QueryWithSave(m.kind),
//	)
//	if m.memory {
//		option = append(option, ssaapi.QueryWithMemory())
//	}
//
//	// if language match or ignore language
//	if res, err := prog.SyntaxFlowRule(rule, option...); err == nil {
//		m.markRuleSuccess()
//		m.notifyResult(res)
//	} else {
//		m.markRuleFailed()
//		m.client.YakitError("program %s exc rule %s failed: %s", prog.GetProgramName(), rule.RuleName, err)
//	}
//}
//func (m *SyntaxFlowScanManager) notifyResult(res *ssaapi.SyntaxFlowResult) {
//	if riskLen := len(res.GetGRPCModelRisk()); riskLen != 0 {
//		m.addRiskCount(int64(riskLen))
//	}
//	for key, count := range res.GetRiskCountMap() {
//		m.riskCountMap.Set(key, count)
//	}
//	// m.riskQuery
//	m.stream.Send(&ypb.SyntaxFlowScanResponse{
//		TaskID:   m.taskID,
//		Status:   m.status,
//		Result:   res.GetGRPCModelResult(),
//		SSARisks: res.GetGRPCModelRisk(),
//	})
//}
//
//func (m *SyntaxFlowScanManager) notifyStatus(ruleName string) {
//	// 直接读取已完成的总数（单次原子操作）
//	finishQuery := atomic.LoadInt64(&m.finishedQuery)
//	successQuery := atomic.LoadInt64(&m.successQuery)
//	failedQuery := atomic.LoadInt64(&m.failedQuery)
//	skipQuery := atomic.LoadInt64(&m.skipQuery)
//	riskCount := atomic.LoadInt64(&m.riskCount)
//	// process
//	m.client.StatusCard("已执行规则", fmt.Sprintf("%d/%d", finishQuery, m.totalQuery), "规则执行状态")
//	m.client.StatusCard("已跳过规则", skipQuery, "规则执行状态")
//	// runtime status
//	m.client.StatusCard("执行成功个数", successQuery, "规则执行状态")
//	m.client.StatusCard("执行失败个数", failedQuery, "规则执行状态")
//	// risk status
//	m.client.StatusCard("检出漏洞/风险个数", riskCount, "漏洞/风险状态")
//	if finishQuery == m.totalQuery {
//		m.status = schema.SYNTAXFLOWSCAN_DONE
//	}
//	// current rule  status
//	//if finishQuery == m.totalQuery {
//	//	m.status = schema.SYNTAXFLOWSCAN_DONE
//	//	m.client.StatusCard("当前执行规则", "已执行完毕", "规则执行进度")
//	//} else {
//	//	if ruleName != "" {
//	//		m.client.StatusCard("当前执行规则", ruleName, "规则执行进度")
//	//	}
//	//}
//	//m.client.YakitInfo("规则[%s]执行进度：")
//	m.client.YakitSetProgress(float64(finishQuery) / float64(m.totalQuery))
//}
//
//func (m *SyntaxFlowScanManager) notifyRuleProcess(progName, ruleName string, f float64) {
//	output := struct {
//		ProgName string `json:"项目名称"`
//		RuleName string `json:"规则名称"`
//		Progress string `json:"执行进度"`
//	}{
//		ProgName: progName,
//		RuleName: ruleName,
//		Progress: fmt.Sprintf("%.2f%%", f*100),
//	}
//	marshal, err := json.Marshal(output)
//	if err != nil {
//		return
//	}
//	m.client.Output(marshal)
//}
