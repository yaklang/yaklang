package yakgrpc

import (
	"context"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySSARisks(ctx context.Context, req *ypb.QuerySSARisksRequest) (*ypb.QuerySSARisksResponse, error) {
	p, risks, err := yakit.QuerySSARisk(s.GetSSADatabase().Debug(), req.GetFilter(), req.GetPagination())
	if err != nil {
		return nil, err
	}
	return &ypb.QuerySSARisksResponse{
		Pagination: req.Pagination,
		Total:      int64(p.TotalRecord),
		Data: lo.Map(risks, func(risk *schema.SSARisk, _ int) *ypb.SSARisk {
			return risk.ToGRPCModel()
		}),
	}, nil
}

func (s *Server) DeleteSSARisks(ctx context.Context, req *ypb.DeleteSSARisksRequest) (*ypb.DbOperateMessage, error) {
	err := yakit.DeleteSSARisks(s.GetSSADatabase(), req.GetFilter())
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName: "ssa risk",
		Operation: "delete",
	}, nil
}

func (s *Server) UpdateSSARiskTags(ctx context.Context, req *ypb.UpdateSSARiskTagsRequest) (*ypb.DbOperateMessage, error) {
	err := yakit.UpdateSSARiskTags(s.GetSSADatabase(), req.GetID(), req.GetTags())
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "ssa risk",
		Operation:  "update",
		EffectRows: 1,
	}, nil
}

func FieldGroup2FiledGroupName(fgs []*ypb.FieldGroup, verbose func(string) string) []*ypb.FieldName {
	return lo.Map(fgs, func(f *ypb.FieldGroup, _ int) *ypb.FieldName {
		return &ypb.FieldName{
			Name:    f.GetName(),
			Verbose: verbose(f.GetName()),
			Total:   f.Total,
		}
	})
}

func (s *Server) GetSSARiskFieldGroup(ctx context.Context, req *ypb.Empty) (*ypb.SSARiskFieldGroupResponse, error) {
	db := s.GetSSADatabase()
	return &ypb.SSARiskFieldGroupResponse{
		FileField:     yakit.SSARiskColumnGroupCount(db, "code_source_url"),
		SeverityField: FieldGroup2FiledGroupName(yakit.SSARiskColumnGroupCount(db, "severity"), severityVerbose),
		RiskTypeField: FieldGroup2FiledGroupName(yakit.SSARiskColumnGroupCount(db, "risk_type"), schema.SSARiskTypeVerbose),
	}, nil
}

func (s *Server) NewSSARiskRead(ctx context.Context, req *ypb.NewSSARiskReadRequest) (*ypb.NewSSARiskReadResponse, error) {
	err := yakit.NewSSARiskReadRequest(s.GetSSADatabase(), req.GetFilter())
	if err != nil {
		return nil, err
	}
	return &ypb.NewSSARiskReadResponse{}, nil
}

func (s *Server) QueryNewSSARisks(ctx context.Context, req *ypb.QueryNewSSARisksRequest) (*ypb.QueryNewSSARiskResponse, error) {
	// query new ssa-risk
	p, data, err := yakit.QuerySSARisk(s.GetSSADatabase().Debug(), &ypb.SSARisksFilter{
		IsRead: -1, // unread
	}, &ypb.Paging{
		Limit:   -1,
		OrderBy: "id",
		Order:   "desc",
		AfterId: req.GetAfterID(),
	})
	if err != nil {
		return nil, err
	}

	total, _ := yakit.QuerySSARiskCount(s.GetSSADatabase(), nil)
	totalUnread := p.TotalRecord // paging not effect p.TotalRecord, so we don't need to query again

	// yakit.SSARiskColumnGroupCount()
	return &ypb.QueryNewSSARiskResponse{
		// new ssa-risk unread  data
		Data: lo.Map(data, func(risk *schema.SSARisk, _ int) *ypb.SSARisk {
			return risk.ToGRPCModel()
		}),
		// new ssa-risk unread count
		NewRiskTotal: int64(len(data)),

		// total ssa-risk
		Total:  int64(total),
		Unread: int64(totalUnread),
	}, nil

}
