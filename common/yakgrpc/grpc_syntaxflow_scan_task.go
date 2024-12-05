package yakgrpc

import (
	"context"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
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
