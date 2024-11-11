package yakgrpc

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SyntaxFlowScanStatus string

// "executing" | "done" | "paused" | "error"

const (
	Executing SyntaxFlowScanStatus = "executing"
	Paused                         = "paused"
	Done                           = "done"
	Error                          = "error"
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

func CreateSyntaxFlowScanManager(ctx context.Context, stream ypb.Yak_SyntaxFlowScanServer, req *ypb.SyntaxFlowScanRequest) (*SyntaxFlowScanManager, error) {
	if len(req.GetProgramName()) == 0 {
		return nil, utils.Errorf("program name is empty")
	}

	taskID := uuid.NewString()
	m := &SyntaxFlowScanManager{
		status: Executing,
		taskID: taskID,
		ctx:    ctx,
		stream: stream,
	}
	m.programs = req.GetProgramName()
	m.ignoreLanguage = req.GetIgnoreLanguage()

	// get rules
	m.rules = yakit.FilterSyntaxFlowRule(consts.GetGormProfileDatabase(), req.GetFilter())

	rulesCount := 0
	if err := m.rules.Model(&schema.SyntaxFlowRule{}).Count(&rulesCount).Error; err != nil {
		return nil, utils.Errorf("count rules failed: %s", err)
	}
	m.rulesCount = rulesCount

	m.totalQuery = int64(m.rulesCount) * int64(len(m.programs))

	yakitClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
		result.RuntimeID = taskID
		return stream.Send(&ypb.SyntaxFlowScanResponse{
			TaskID:     taskID,
			Status:     string(m.status),
			ExecResult: result,
		})
	}, taskID)
	m.client = yakitClient
	return m, nil
}

func (m *SyntaxFlowScanManager) Start() error {
	var errs error
	for _, progName := range m.programs {
		prog, err := ssaapi.FromDatabase(progName)
		if err != nil {
			errs = utils.JoinErrors(errs, err)
			m.skipQuery += int64(m.rulesCount) // skip all rules for this program
			continue
		}
		for rule := range sfdb.YieldSyntaxFlowRules(m.rules, m.ctx) {
			m.Query(rule, prog)
		}
	}
	m.notifyProgress("")
	return errs
}

// SaveTask save task info which is from manager to database
func (m *SyntaxFlowScanManager) SaveTask() {
	if m.taskRecorder == nil {
		m.taskRecorder = &schema.SyntaxFlowScanTask{}
	}
	m.taskRecorder.Programs = strings.Join(m.programs, ",")
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
	err = yakit.SaveSyntaxFlowScanTask(consts.GetGormProjectDatabase(), m.taskRecorder)
	if err != nil {
		log.Error(err)
	}
}

func (m *SyntaxFlowScanManager) Query(rule *schema.SyntaxFlowRule, prog *ssaapi.Program) {

	m.notifyProgress(rule.RuleName)
	defer m.SaveTask()
	// log.Infof("executing rule %s", rule.RuleName)
	if !m.ignoreLanguage {
		if rule.Language != prog.GetLanguage() {
			m.skipQuery++
			// m.client.YakitInfo("program %s(lang:%s) exec rule %s(lang:%s) failed: language not match", programName, prog.GetLanguage(), rule.RuleName, rule.Language)
			return
		}
	}

	// if language match or ignore language
	if res, err := prog.SyntaxFlowRule(rule, sfvm.WithContext(m.ctx)); err == nil {
		if _, err := res.Save(m.taskID); err == nil {
			m.successQuery++
			m.notifyResult(res)
		} else {
			m.failedQuery++
			m.client.YakitError("program %s exec rule %s result save failed: %s", prog.GetProgramName(), rule.RuleName, err)
		}
	} else {
		m.failedQuery++
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
	finishQuery := m.successQuery + m.failedQuery + m.skipQuery
	if finishQuery == m.totalQuery {
		m.status = yakit.SYNTAXFLOWSCAN_DONE
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
}

func (m *SyntaxFlowScanManager) notifyStatus() {
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

func (m *SyntaxFlowScanManager) ResumeManagerFromTask() error {
	task, err := yakit.GetSyntaxFlowScanTaskById(consts.GetGormProjectDatabase(), m.TaskId())
	if err != nil {
		return utils.Wrapf(err, "Resume SyntaxFlow task by is failed")
	}
	m.taskRecorder = task
	m.status = task.Status
	m.status = task.Status
	m.programs = strings.Split(task.Programs, ",")
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
