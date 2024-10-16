package yakgrpc

import (
	"context"
	_ "embed"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
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
	taskID string
	status SyntaxFlowScanStatus
	ctx    context.Context

	stream ypb.Yak_SyntaxFlowScanServer
	client *yaklib.YakitClient

	// rules
	rules      *gorm.DB
	rulesCount int

	// program
	programs []string

	// process
	currentQuery int64
	totalQuery   int64
}

func CreateSyntaxFlowScanManager(ctx context.Context, stream ypb.Yak_SyntaxFlowScanServer) *SyntaxFlowScanManager {
	taskID := uuid.NewString()
	m := &SyntaxFlowScanManager{
		status: Executing,
		taskID: taskID,
		ctx:    ctx,
		stream: stream,
	}
	yakitClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
		result.RuntimeID = taskID
		return stream.Send(&ypb.SyntaxFlowScanResponse{
			TaskID:     taskID,
			Status:     string(m.status),
			ExecResult: result,
		})
	}, taskID)
	m.client = yakitClient
	return m
}

func (m *SyntaxFlowScanManager) Start(req *ypb.SyntaxFlowScanRequest) error {
	if req.GetFilter() == nil || len(req.GetProgramName()) == 0 {
		return utils.Errorf("filter or program name is empty")
	}
	m.programs = req.GetProgramName()

	// get rules
	m.rules = yakit.FilterSyntaxFlowRule(consts.GetGormProfileDatabase(), req.GetFilter())
	// rules := sfdb.YieldSyntaxFlowRules(db, m.ctx)

	rulesCount := 0
	if err := m.rules.Model(&schema.SyntaxFlowRule{}).Count(&rulesCount).Error; err != nil {
		return utils.Errorf("count rules failed: %s", err)
	}
	m.rulesCount = rulesCount

	m.totalQuery = int64(m.rulesCount) * int64(len(m.programs))

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
	for rule := range sfdb.YieldSyntaxFlowRules(m.rules, m.ctx) {
		res, err := prog.SyntaxFlowRule(rule)
		if err != nil {
			m.client.YakitError("program %s exc rule %s failed: %s", programName, rule.RuleName, err)
			// continue
		}
		if _, err := res.Save(m.taskID); err != nil {
			m.client.YakitError("program %s exec rule %s result save failed: %s", programName, rule.RuleName, err)
			// continue
		}
		m.notifyResult(res)
	}
	return nil
}

func (m *SyntaxFlowScanManager) notifyResult(res *ssaapi.SyntaxFlowResult) {
	m.currentQuery++
	if m.currentQuery == m.totalQuery {
		m.status = Done
	}
	m.client.YakitSetProgress(float64(m.currentQuery) / float64(m.totalQuery))
	m.stream.Send(&ypb.SyntaxFlowScanResponse{
		TaskID: m.taskID,
		Status: string(m.status),
		Result: res.GetGRPCModelResult(),
		Risks:  res.GetGRPCModelRisk(),
	})
}
