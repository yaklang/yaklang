package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

func (s *Server) QueryFingerprint(ctx context.Context, req *ypb.QueryFingerprintRequest) (*ypb.QueryFingerprintResponse, error) {
	paging, data, err := yakit.QueryGeneralRule(s.GetProfileDatabase(), req.GetFilter(), req.GetPagination())
	if err != nil {
		return nil, err
	}

	start := time.Now()
	var res []*ypb.FingerprintRule
	for _, r := range data {
		m := yakit.SchemaGeneralRuleToGRPCGeneralRule(r)
		if m == nil {
			log.Errorf("failed to convert schema.GeneralRule to ypb.FingerprintRule: %v", r)
		} else {
			res = append(res, m)
		}
	}
	cost := time.Now().Sub(start)
	if cost.Milliseconds() > 200 {
		log.Infof("finished converting httpflow(%v) cost: %s", len(res), cost)
	}

	return &ypb.QueryFingerprintResponse{
		Pagination: &ypb.Paging{
			Page:    int64(paging.Page),
			Limit:   int64(paging.Limit),
			OrderBy: req.GetPagination().GetOrderBy(),
			Order:   req.GetPagination().GetOrder(),
		},
		Total: int64(paging.TotalRecord),
		Data:  res,
	}, nil
}

func (s *Server) DeleteFingerprint(ctx context.Context, req *ypb.DeleteFingerprintRequest) (*ypb.DbOperateMessage, error) {
	return nil, nil
}

func (s *Server) UpdateFingerprint(ctx context.Context, req *ypb.UpdateFingerprintRequest) (*ypb.DbOperateMessage, error) {
	return nil, nil
}

func (s *Server) CreateFingerprint(ctx context.Context, req *ypb.CreateFingerprintRequest) (*ypb.DbOperateMessage, error) {
	return nil, nil
}
