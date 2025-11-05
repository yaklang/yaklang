package yakgrpc

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/yaklib"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySSARisks(ctx context.Context, req *ypb.QuerySSARisksRequest) (*ypb.QuerySSARisksResponse, error) {
	var risks []*schema.SSARisk
	var p *bizhelper.Paginator
	var err error

	filter := req.GetFilter()
	if dr := filter.GetSSARiskDiffRequest(); dr != nil {
		baseline := &ypb.SSARiskDiffItem{
			ProgramName:   strings.Join(filter.GetProgramName(), ","),
			RiskRuntimeId: strings.Join(filter.GetRuntimeID(), ","),
		}
		filter.SSARiskDiffRequest.BaseLine = baseline
		res, err := yakit.DoRiskDiff(ctx, baseline, dr.GetCompare())
		if err != nil {
			return nil, err
		}
		for re := range res {
			_ = re
		}
	}
	p, risks, err = yakit.QuerySSARisk(s.GetSSADatabase(), filter, req.GetPagination())
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

func (s *Server) GetSSARiskFieldGroupEx(ctx context.Context, req *ypb.GetSSARiskFieldGroupRequest) (*ypb.SSARiskFieldGroupResponse, error) {
	if req == nil {
		return nil, utils.Errorf("GetSSARiskFieldGroupRequest is nil")
	}
	db := yakit.FilterSSARisk(s.GetSSADatabase(), req.GetFilter())
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

func (s *Server) QueryNewSSARisks(ctx context.Context, req *ypb.QueryNewSSARisksRequest) (*ypb.QueryNewSSARisksResponse, error) {
	db := s.GetSSADatabase().Where("id > ?", req.GetAfterID())
	// query new ssa-risk
	p, data, err := yakit.QuerySSARisk(db, &ypb.SSARisksFilter{
		IsRead: -1, // unread
	}, &ypb.Paging{
		Limit:   5,
		OrderBy: "id",
		Order:   "desc",
	})
	if err != nil {
		return nil, err
	}

	total, _ := yakit.QuerySSARiskCount(s.GetSSADatabase(), nil)
	totalUnread, _ := yakit.QuerySSARiskCount(s.GetSSADatabase(), &ypb.SSARisksFilter{
		IsRead: -1, // unread
	})

	// yakit.SSARiskColumnGroupCount()
	return &ypb.QueryNewSSARisksResponse{
		// new ssa-risk unread  data
		Data: lo.Map(data, func(risk *schema.SSARisk, _ int) *ypb.SSARisk {
			return risk.ToGRPCModel()
		}),
		// new ssa-risk unread count
		NewRiskTotal: int64(p.TotalRecord),

		// total ssa-risk
		Total:  int64(total),
		Unread: int64(totalUnread),
	}, nil

}

func (s *Server) SSARiskFeedbackToOnline(ctx context.Context, req *ypb.SSARiskFeedbackToOnlineRequest) (*ypb.Empty, error) {
	if req.Token == "" {
		return nil, utils.Errorf("params empty")
	}
	db := s.GetSSADatabase()
	db = yakit.FilterSSARisk(db, req.Filter)
	data := yakit.YieldSSARisk(db, context.Background())
	for k := range data {
		content, err := json.Marshal(k)
		if err != nil {
			continue
		}
		client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())

		raw, err := json.Marshal(yaklib.UploadOnlineRequest{
			content,
		})
		err = client.UploadToOnline(ctx, req.Token, raw, "api/ssa/risk/feed/back")
		if err != nil {
			log.Errorf("uploadRiskToOnline failed: %s", err)
			return &ypb.Empty{}, nil
		}
	}

	return &ypb.Empty{}, nil
}
