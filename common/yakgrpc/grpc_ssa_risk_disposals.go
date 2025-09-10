package yakgrpc

import (
	"context"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CreateSSARiskDisposals(ctx context.Context, req *ypb.CreateSSARiskDisposalsRequest) (*ypb.CreateSSARiskDisposalsResponse, error) {
	disposals, err := yakit.CreateSSARiskDisposals(s.GetSSADatabase(), req)
	if err != nil {
		return nil, err
	}
	result := lo.Map(disposals, func(item schema.SSARiskDisposals, index int) *ypb.SSARiskDisposalData {
		return item.ToGRPCModel()
	})
	return &ypb.CreateSSARiskDisposalsResponse{
		Data: result,
	}, nil
}

func (s *Server) QuerySSARiskDisposals(ctx context.Context, req *ypb.QuerySSARiskDisposalsRequest) (*ypb.QuerySSARiskDisposalsResponse, error) {
	if req == nil {
		return nil, utils.Error("QuerySSARiskDisposals failed: QuerySSARiskDisposalsRequest is nil")
	}
	p, data, err := yakit.QuerySSARiskDisposals(s.GetSSADatabase(), req)
	if err != nil {
		return nil, utils.Errorf("QuerySSARiskDisposals failed: %v", err)
	}
	result := lo.Map(data, func(item schema.SSARiskDisposals, index int) *ypb.SSARiskDisposalData {
		return item.ToGRPCModel()
	})
	return &ypb.QuerySSARiskDisposalsResponse{
		Data:       result,
		Pagination: req.GetPagination(),
		Total:      int64(p.TotalRecord),
	}, nil
}

func (s *Server) DeleteSSARiskDisposals(ctx context.Context, req *ypb.DeleteSSARiskDisposalsRequest) (*ypb.DeleteSSARiskDisposalsResponse, error) {
	if req == nil {
		return nil, utils.Error("DeleteSSARiskDisposals failed: DeleteSSARiskDisposalsRequest is nil")
	}
	count, err := yakit.DeleteSSARiskDisposals(s.GetSSADatabase(), req)
	if err != nil {
		return nil, utils.Errorf("DeleteSSARiskDisposals failed: %v", err)
	}
	return &ypb.DeleteSSARiskDisposalsResponse{
		Message: &ypb.DbOperateMessage{
			TableName:  "ssa_risk_disposals",
			Operation:  DbOperationDelete,
			EffectRows: count,
		},
	}, nil
}

func (s *Server) UpdateSSARiskDisposals(ctx context.Context, req *ypb.UpdateSSARiskDisposalsRequest) (*ypb.UpdateSSARiskDisposalsResponse, error) {
	if req == nil {
		return nil, utils.Error("UpdateSSARiskDisposal failed: UpdateSSARiskDisposalsRequest is nil")
	}
	disposals, err := yakit.UpdateSSARiskDisposals(s.GetSSADatabase(), req)
	if err != nil {
		return nil, utils.Errorf("UpdateSSARiskDisposal failed: %v", err)
	}
	result := lo.Map(disposals, func(item schema.SSARiskDisposals, index int) *ypb.SSARiskDisposalData {
		return item.ToGRPCModel()
	})
	return &ypb.UpdateSSARiskDisposalsResponse{
		Data: result,
	}, nil
}

func (s *Server) GetSSARiskDisposal(ctx context.Context, req *ypb.GetSSARiskDisposalRequest) (*ypb.GetSSARiskDisposalResponse, error) {
	if req == nil {
		return nil, utils.Error("GetSSARiskDisposal faild: req is nil")
	}

	disposal, err := yakit.GetSSARiskDisposalsWithTaskInfo(s.GetSSADatabase(), req)
	if err != nil {
		return nil, utils.Errorf("GetSSARiskDisposal failed: %v", err)
	}

	return &ypb.GetSSARiskDisposalResponse{
		Data: disposal,
	}, nil
}
