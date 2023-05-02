package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryWebsocketFlowByHTTPFlowWebsocketHash(ctx context.Context, req *ypb.QueryWebsocketFlowByHTTPFlowWebsocketHashRequest) (*ypb.WebsocketFlows, error) {
	paging, flows, err := yakit.QueryWebsocketFlowByWebsocketHash(
		s.GetProjectDatabase(),
		req.GetWebsocketRequestHash(),
		int(req.GetPagination().GetPage()),
		int(req.GetPagination().GetLimit()),
	)
	if err != nil {
		return nil, err
	}

	data := funk.Map(flows, func(i *yakit.WebsocketFlow) *ypb.WebsocketFlow {
		return i.ToGRPCModel()
	}).([]*ypb.WebsocketFlow)
	return &ypb.WebsocketFlows{
		Pagination: req.Pagination,
		Data:       data,
		Total:      int64(paging.TotalRecord),
	}, nil
}
