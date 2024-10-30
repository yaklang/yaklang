package yakgrpc

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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
	taskID string
	status SyntaxFlowScanStatus
	ctx    context.Context

	// stream
	stream ypb.Yak_SyntaxFlowScanServer
	client *yaklib.YakitClient

	// rules
	rules      *gorm.DB
	rulesCount int

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
		if err := m.Query(progName); err != nil {
			errs = utils.JoinErrors(errs, err)
		}
	}
	return errs
}

func (m *SyntaxFlowScanManager) Query(programName string) error {
	prog, err := ssaapi.FromDatabase(programName)
	if err != nil {
		return err
	}
	for rule := range sfdb.YieldSyntaxFlowRulesWithoutLib(m.rules, m.ctx) {
		m.currentRuleName = rule.RuleName

		if res, err := prog.SyntaxFlowRule(rule); err == nil {
			if _, err := res.Save(m.taskID); err == nil {
				m.successQuery++
				m.notifyResult(res)
			} else {
				m.failedQuery++
				m.client.YakitError("program %s exec rule %s result save failed: %s", programName, rule.RuleName, err)
			}
		} else {
			m.failedQuery++
			m.client.YakitError("program %s exc rule %s failed: %s", programName, rule.RuleName, err)
		}
		m.notifyProgress()
	}
	return nil
}

func (m *SyntaxFlowScanManager) notifyResult(res *ssaapi.SyntaxFlowResult) {
	if riskLen := len(res.GetGRPCModelRisk()); riskLen != 0 {
		m.riskCount += int64(riskLen)
	}
	// m.riskQuery
	m.stream.Send(&ypb.SyntaxFlowScanResponse{
		TaskID: m.taskID,
		Status: string(m.status),
		Result: res.GetGRPCModelResult(),
		Risks:  res.GetGRPCModelRisk(),
	})
}

func (m *SyntaxFlowScanManager) notifyProgress() {
	finishQuery := m.successQuery + m.failedQuery + m.skipQuery
	if finishQuery == m.totalQuery {
		m.status = Done
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
