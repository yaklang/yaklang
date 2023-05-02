package yakgrpc

import (
	"context"
	"yaklang.io/yaklang/common/go-funk"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yakgrpc/yakit"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryMITMRuleExtractedData(ctx context.Context, req *ypb.QueryMITMRuleExtractedDataRequest) (*ypb.QueryMITMRuleExtractedDataResponse, error) {
	db := s.GetProjectDatabase()
	if req.GetHTTPFlowHash() != "" {
		db = db.Where("source_type == 'httpflow' and trace_id = ?", req.GetHTTPFlowHash())
	}

	if req.GetHTTPFlowHash() == "" {
		return nil, utils.Error("httpflow hash must be set")
	}
	p, data, err := yakit.QueryExtractedData(db, req)
	if err != nil {
		return nil, err
	}
	return &ypb.QueryMITMRuleExtractedDataResponse{
		Data: funk.Map(data, func(i *yakit.ExtractedData) *ypb.MITMRuleExtractedData {
			return &ypb.MITMRuleExtractedData{
				Id:         int64(i.ID),
				CreatedAt:  i.CreatedAt.Unix(),
				SourceType: i.SourceType,
				TraceId:    i.TraceId,
				Regexp:     utils.EscapeInvalidUTF8Byte([]byte(i.Regexp)),
				RuleName:   utils.EscapeInvalidUTF8Byte([]byte(i.RuleVerbose)),
				Data:       utils.EscapeInvalidUTF8Byte([]byte(i.Data)),
			}
		}).([]*ypb.MITMRuleExtractedData),
		Total:      int64(p.TotalRecord),
		Pagination: req.GetPagination(),
	}, nil
}
