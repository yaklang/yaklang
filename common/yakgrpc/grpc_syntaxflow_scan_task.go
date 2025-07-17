package yakgrpc

import (
	"context"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
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

	p, tasks, err := yakit.QuerySyntaxFlowScanTask(ssadb.GetDB(), request)
	if err != nil {
		return nil, err
	}

	if progNames := request.GetFilter().GetPrograms(); len(progNames) == 1 {
		var lastTask *schema.SyntaxFlowScanTask
		for i, task := range tasks {
			risks := []*schema.SSARisk{}
			if i == 0 {
				lastTask = task
				continue
			}
			baseline := &ypb.SSARiskDiffItem{
				ProgramName:   progNames[0],
				RiskRuntimeId: task.TaskId,
			}
			compare := &ypb.SSARiskDiffItem{
				ProgramName:   progNames[0],
				RiskRuntimeId: lastTask.TaskId,
			}
			res, err := yakit.DoRiskDiff(ctx, baseline, compare)
			if err != nil {
				return nil, err
			}
			for re := range res {
				if re.Status == yakit.Add {
					risks = append(risks, re.NewValue)
				}
			}
			task.NewRiskCount = int64(len(risks))
			lastTask = task
		}
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
		deleted, err := yakit.DeleteAllSyntaxFlowScanTask(ssadb.GetDB())
		if err != nil {
			return nil, err
		}
		dbMsg.EffectRows += deleted
		return dbMsg, nil
	}
	if request.GetFilter() != nil {
		deleted, err := yakit.DeleteSyntaxFlowScanTask(ssadb.GetDB(), request)
		if err != nil {
			return nil, err
		}
		dbMsg.EffectRows += deleted
		return dbMsg, nil
	}
	return dbMsg, nil
}
