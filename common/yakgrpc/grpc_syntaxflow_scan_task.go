package yakgrpc

import (
	"context"
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySyntaxFlowScanTask(ctx context.Context, request *ypb.QuerySyntaxFlowScanTaskRequest) (*ypb.QuerySyntaxFlowScanTaskResponse, error) {
	if request.Pagination == nil {
		request.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	p, tasks, err := yakit.QuerySyntaxFlowScanTask(s.GetProjectDatabase(), request)
	if err != nil {
		return nil, err
	}
	datas := lo.Map(tasks, func(task *schema.SyntaxFlowScanTask, index int) *ypb.SyntaxFlowScanTask {
		data := task.ToGRPCModel()
		return data
	})
	return &ypb.QuerySyntaxFlowScanTaskResponse{
		Pagination: &ypb.Paging{
			Page:     int64(p.Page),
			Limit:    int64(p.Limit),
			OrderBy:  request.Pagination.OrderBy,
			Order:    request.Pagination.Order,
			RawOrder: request.Pagination.RawOrder,
		},
		Data:  datas,
		Total: int64(p.TotalRecord),
	}, nil
}

func (s *Server) DeleteSyntaxFlowScanTask(ctx context.Context, request *ypb.DeleteSyntaxFlowScanTaskRequest) (*ypb.DbOperateMessage, error) {
	dbMsg := &ypb.DbOperateMessage{
		TableName: "syntax_flow_scan_task",
		Operation: DbOperationDelete,
	}
	if request.GetDeleteAll() {
		deleted, err := yakit.DeleteAllSyntaxFlowScanTask(s.GetProjectDatabase())
		if err != nil {
			return nil, err
		}
		dbMsg.EffectRows += deleted
		return dbMsg, nil
	}
	if request.GetFilter() != nil {
		deleted, err := yakit.DeleteSyntaxFlowScanTask(s.GetProjectDatabase(), request)
		if err != nil {
			return nil, err
		}
		dbMsg.EffectRows += deleted
		return dbMsg, nil
	}
	return dbMsg, nil
}

func (s *Server) syntaxFlowScanNewTask(m *SyntaxFlowScanManager, req *ypb.SyntaxFlowScanRequest, stream ypb.Yak_SyntaxFlowScanServer) error {
	defer m.Stop()
	if len(req.GetProgramName()) == 0 {
		return utils.Errorf("program name is empty")
	}
	taskId := m.TaskId()
	m.status = schema.SYNTAXFLOWSCAN_EXECUTING
	m.stream = stream
	m.programs = req.GetProgramName()
	m.ignoreLanguage = req.GetIgnoreLanguage()
	m.rules = yakit.FilterSyntaxFlowRule(consts.GetGormProfileDatabase(), req.GetFilter())

	rulesCount, err := yakit.QuerySyntaxFlowRuleCount(consts.GetGormProfileDatabase(), req.GetFilter())
	if err != nil {
		return utils.Errorf("count rules failed: %s", err)
	}
	m.rulesCount = rulesCount
	m.totalQuery = m.rulesCount * int64(len(m.programs))
	yakitClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
		result.RuntimeID = taskId
		return m.stream.Send(&ypb.SyntaxFlowScanResponse{
			TaskID:     taskId,
			Status:     m.status,
			ExecResult: result,
		})
	}, taskId)
	m.client = yakitClient
	m.SaveTask()
	// start task
	err = m.Start()
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) syntaxFlowResumeTask(m *SyntaxFlowScanManager, stream ypb.Yak_SyntaxFlowScanServer) error {
	// resume syntax flow manager from task which is saved in database
	err := m.ResumeManagerFromTask()
	if err != nil {
		return err
	}
	m.status = schema.SYNTAXFLOWSCAN_EXECUTING
	m.stream = stream
	if m.client == nil {
		yakitClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
			result.RuntimeID = m.TaskId()
			return m.stream.Send(&ypb.SyntaxFlowScanResponse{
				TaskID:     m.TaskId(),
				Status:     m.status,
				ExecResult: result,
			})
		}, m.TaskId())
		m.client = yakitClient
	}

	m.SaveTask()
	defer func() {
		if err := recover(); err != nil {
			m.taskRecorder.Reason = fmt.Sprintf("PANIC from resume:%v", err)
			m.status = schema.SYNTAXFLOWSCAN_ERROR
			m.SaveTask()
			return
		}
		if m.status == schema.SYNTAXFLOWSCAN_PAUSED {
			m.SaveTask()
			return
		}
		m.status = schema.SYNTAXFLOWSCAN_DONE
		m.SaveTask()
	}()

	taskIndex := m.CurrentTaskIndex()
	if taskIndex > m.totalQuery {
		m.status = schema.SYNTAXFLOWSCAN_DONE
		m.SaveTask()
		return nil
	}
	err = m.Start(taskIndex)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) syntaxFlowStatusTask(m *SyntaxFlowScanManager, stream ypb.Yak_SyntaxFlowScanServer) error {
	err := m.ResumeManagerFromTask()
	if err != nil {
		return err
	}
	m.stream = stream
	if m.client == nil {
		yakitClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
			result.RuntimeID = m.TaskId()
			return m.stream.Send(&ypb.SyntaxFlowScanResponse{
				TaskID:     m.TaskId(),
				Status:     m.status,
				ExecResult: result,
			})
		}, m.TaskId())
		m.client = yakitClient
	}
	m.notifyStatus()
	return nil
}
