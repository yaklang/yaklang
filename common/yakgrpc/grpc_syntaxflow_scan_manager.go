package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"sync"
	"sync/atomic"
)

type SyntaxFlowScanManager struct {
	// task info
	taskID       string
	status       string
	ctx          context.Context
	resumeSignal *sync.Cond
	isPaused     *utils.AtomicBool
	cancel       context.CancelFunc
	taskRecorder *schema.SyntaxFlowScanTask
	// config
	ignoreLanguage bool

	// stream
	stream ypb.Yak_SyntaxFlowScanServer
	client *yaklib.YakitClient

	// rules
	rules      *gorm.DB
	ruleFilter *ypb.SyntaxFlowRuleFilter
	rulesCount int64

	// program
	programs []string

	// query execute
	failedQuery  int64 // query failed
	skipQuery    int64 // language not match, skip this rule
	successQuery int64
	// risk
	riskCount int64
	// query process
	totalQuery int64
}

var syntaxFlowScanManager = new(sync.Map)

func CreateSyntaxFlowTask(taskId string, ctx context.Context) (*SyntaxFlowScanManager, error) {
	_, ok := syntaxFlowScanManager.Load(taskId)
	if ok {
		return nil, utils.Errorf("task id %s already exists", taskId)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var rootctx, cancel = context.WithCancel(ctx)
	m := &SyntaxFlowScanManager{
		taskID:       taskId,
		ctx:          rootctx,
		status:       schema.SYNTAXFLOWSCAN_EXECUTING,
		resumeSignal: sync.NewCond(&sync.Mutex{}),
		isPaused:     utils.NewAtomicBool(),
		cancel:       cancel,
	}
	syntaxFlowScanManager.Store(taskId, m)
	return m, nil
}

func RemoveSyntaxFlowTask(id string) {
	r, err := GetSyntaxFlowTask(id)
	if err != nil {
		return
	}
	r.Stop()
	syntaxFlowScanManager.Delete(id)
}

func GetSyntaxFlowTask(id string) (*SyntaxFlowScanManager, error) {
	raw, ok := syntaxFlowScanManager.Load(id)
	if !ok {
		return nil, utils.Errorf("task id %s not exists", id)
	}
	if ins, ok := raw.(*SyntaxFlowScanManager); ok {
		return ins, nil
	} else {
		return nil, utils.Errorf("task id %s not exists(typeof %T err)", id, raw)
	}
}

func (m *SyntaxFlowScanManager) Start(startIndex ...int64) error {
	defer func() {
		if err := recover(); err != nil {
			m.taskRecorder.Reason = fmt.Sprintf("%v", err)
			m.status = schema.SYNTAXFLOWSCAN_ERROR
			m.notifyStatus()
			m.SaveTask()
			return
		}
		if m.status == schema.SYNTAXFLOWSCAN_PAUSED {
			m.notifyStatus()
			m.SaveTask()
			return
		}
		m.status = schema.SYNTAXFLOWSCAN_DONE
		m.notifyStatus()
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
	var errs error
	var taskIndex int64 // when taskIndex == totalQuery, the task start to run.
	for _, progName := range m.programs {
		if m.IsPause() || m.IsStop() {
			break
		}
		nextIndex := taskIndex + m.rulesCount
		if nextIndex <= start {
			taskIndex = nextIndex
		}
		prog, err := ssaapi.FromDatabase(progName)
		if err != nil {
			errs = utils.JoinErrors(errs, err)
			atomic.AddInt64(&m.skipQuery, m.rulesCount) // skip all rules for this program // skip all rules for this program
			taskIndex += m.rulesCount
			m.SaveTask()
			continue
		}
		for rule := range sfdb.YieldSyntaxFlowRules(m.rules, m.ctx) {
			taskIndex++
			if taskIndex <= start {
				continue
			}
			if m.IsPause() || m.IsStop() {
				break
			}
			m.Query(rule, prog)
		}
	}
	m.notifyProgress("")
	return errs
}

func (m *SyntaxFlowScanManager) Query(rule *schema.SyntaxFlowRule, prog *ssaapi.Program) {
	m.notifyProgress(rule.RuleName)
	defer m.SaveTask()
	// log.Infof("executing rule %s", rule.RuleName)
	if !m.ignoreLanguage {
		if string(rule.Language) != prog.GetLanguage() {
			atomic.AddInt64(&m.skipQuery, 1)
			// m.client.YakitInfo("program %s(lang:%s) exec rule %s(lang:%s) failed: language not match", programName, prog.GetLanguage(), rule.RuleName, rule.Language)
			return
		}
	}

	// if language match or ignore language
	if res, err := prog.SyntaxFlowRule(rule, ssaapi.QueryWithContext(m.ctx)); err == nil {
		if _, err := res.Save(m.taskID); err == nil {
			atomic.AddInt64(&m.successQuery, 1)
			m.notifyResult(res)
		} else {
			atomic.AddInt64(&m.failedQuery, 1)
			m.client.YakitError("program %s exec rule %s result save failed: %s", prog.GetProgramName(), rule.RuleName, err)
		}
	} else {
		atomic.AddInt64(&m.failedQuery, 1)
		m.client.YakitError("program %s exc rule %s failed: %s", prog.GetProgramName(), rule.RuleName, err)
	}
}

func (m *SyntaxFlowScanManager) Resume() {
	m.isPaused.UnSet()
}

func (m *SyntaxFlowScanManager) notifyResult(res *ssaapi.SyntaxFlowResult) {
	if riskLen := len(res.GetGRPCModelRisk()); riskLen != 0 {
		m.riskCount += int64(riskLen)
	}
	// m.riskQuery
	m.stream.Send(&ypb.SyntaxFlowScanResponse{
		TaskID: m.taskID,
		Status: m.status,
		Result: res.GetGRPCModelResult(),
		Risks:  res.GetGRPCModelRisk(),
	})
}

func (m *SyntaxFlowScanManager) notifyProgress(ruleName string) {
	m.client.StatusCard("当前执行规则", ruleName, "规则执行进度")
	m.notifyStatus()
}

func (m *SyntaxFlowScanManager) notifyStatus() {
	finishQuery := m.successQuery + m.failedQuery + m.skipQuery
	if finishQuery == m.totalQuery {
		m.status = schema.SYNTAXFLOWSCAN_DONE
		m.client.StatusCard("当前执行规则", "已执行完毕", "规则执行进度")
	}
	m.client.YakitSetProgress(float64(finishQuery) / float64(m.totalQuery))

	// process
	m.client.StatusCard("已执行规则", fmt.Sprintf("%d/%d", finishQuery, m.totalQuery), "规则执行进度")
	m.client.StatusCard("已跳过规则", m.skipQuery, "规则执行进度")
	// runtime status
	m.client.StatusCard("执行成功个数", m.successQuery, "规则执行状态")
	m.client.StatusCard("执行失败个数", m.failedQuery, "规则执行状态")
	// risk status
	m.client.StatusCard("检出漏洞/风险个数", m.riskCount, "漏洞/风险状态")
	m.stream.Send(&ypb.SyntaxFlowScanResponse{
		TaskID: m.taskID,
		Status: m.status,
	})
}

func (m *SyntaxFlowScanManager) Stop() {
	m.cancel()
}

func (m *SyntaxFlowScanManager) TaskId() string {
	return m.taskID
}

func (m *SyntaxFlowScanManager) Pause() {
	m.isPaused.Set()
}

func (m *SyntaxFlowScanManager) IsPause() bool {
	return m.isPaused.IsSet()
}

func (m *SyntaxFlowScanManager) IsStop() bool {
	select {
	case <-m.ctx.Done():
		return true
	default:
		return false
	}
}

// SaveTask save task info which is from manager to database
func (m *SyntaxFlowScanManager) SaveTask() {
	if m.taskRecorder == nil {
		m.taskRecorder = &schema.SyntaxFlowScanTask{}
	}
	m.taskRecorder.Programs = strings.Join(m.programs, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)
	m.taskRecorder.TaskId = m.taskID
	m.taskRecorder.Status = m.status
	m.taskRecorder.SuccessQuery = m.successQuery
	m.taskRecorder.FailedQuery = m.failedQuery
	m.taskRecorder.SkipQuery = m.skipQuery
	m.taskRecorder.RiskCount = m.riskCount
	m.taskRecorder.TotalQuery = m.totalQuery
	marshal, err := json.Marshal(m.ruleFilter)
	if err != nil {
		log.Error(err)
		return
	}
	m.taskRecorder.RuleFilter = marshal
	err = schema.SaveSyntaxFlowScanTask(consts.GetGormProjectDatabase(), m.taskRecorder)
	if err != nil {
		log.Error(err)
	}
}

func (m *SyntaxFlowScanManager) ResumeManagerFromTask() error {
	task, err := schema.GetSyntaxFlowScanTaskById(consts.GetGormProjectDatabase(), m.TaskId())
	if err != nil {
		return utils.Wrapf(err, "Resume SyntaxFlow task by is failed")
	}
	m.taskRecorder = task
	m.status = task.Status
	m.status = task.Status
	m.programs = strings.Split(task.Programs, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)
	m.successQuery = task.SuccessQuery
	m.failedQuery = task.FailedQuery
	m.skipQuery = task.SkipQuery
	m.riskCount = task.RiskCount
	m.totalQuery = task.TotalQuery
	m.ruleFilter = &ypb.SyntaxFlowRuleFilter{}
	err = json.Unmarshal(task.RuleFilter, m.ruleFilter)
	if err != nil {
		return utils.Wrapf(err, "Unmarshal SyntaxFlow RuleFilter: %v", task.RuleFilter)
	}
	m.rules = yakit.FilterSyntaxFlowRule(consts.GetGormProfileDatabase(), m.ruleFilter)
	return nil
}

func (m *SyntaxFlowScanManager) CurrentTaskIndex() int64 {
	return m.skipQuery + m.failedQuery + m.successQuery
}
