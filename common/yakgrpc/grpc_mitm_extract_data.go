package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryMITMRuleExtractedData(ctx context.Context, req *ypb.QueryMITMRuleExtractedDataRequest) (*ypb.QueryMITMRuleExtractedDataResponse, error) {
	db := s.GetProjectDatabase()
	if req.GetHTTPFlowHiddenIndex() != "" {
		db = db.Where("source_type == 'httpflow' and trace_id = ?", req.GetHTTPFlowHiddenIndex())
	} else {
		return nil, utils.Error("httpflow hiddenindex must be set")
	}
	p, data, err := yakit.QueryExtractedData(db, req)
	if err != nil {
		return nil, err
	}
	return &ypb.QueryMITMRuleExtractedDataResponse{
		Data: funk.Map(data, func(i *schema.ExtractedData) *ypb.MITMRuleExtractedData {
			return &ypb.MITMRuleExtractedData{
				Id:             int64(i.ID),
				CreatedAt:      i.CreatedAt.Unix(),
				SourceType:     i.SourceType,
				TraceId:        i.TraceId,
				Regexp:         utils.EscapeInvalidUTF8Byte([]byte(i.Regexp)),
				RuleName:       utils.EscapeInvalidUTF8Byte([]byte(i.RuleVerbose)),
				Data:           utils.EscapeInvalidUTF8Byte([]byte(i.Data)),
				Index:          int64(i.DataIndex),
				Length:         int64(i.Length),
				IsMatchRequest: i.IsMatchRequest,
			}
		}).([]*ypb.MITMRuleExtractedData),
		Total:      int64(p.TotalRecord),
		Pagination: req.GetPagination(),
	}, nil
}
