package yakgrpc

import (
	"context"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryAIEvent(ctx context.Context, req *ypb.AIEventQueryRequest) (*ypb.AIEventQueryResponse, error) {
	if req.GetProcessID() != "" {
		process, err := yakit.GetAIProcessByID(s.GetProjectDatabase(), req.GetProcessID())
		if err != nil {
			return nil, err
		}
		return &ypb.AIEventQueryResponse{
			Events: lo.Map(process.Events, func(item *schema.AiOutputEvent, _ int) *ypb.AIOutputEvent {
				return item.ToGRPC()
			}),
		}, nil
	} else {
		event, err := yakit.QueryAIEvent(s.GetProjectDatabase(), req.GetFilter())
		if err != nil {
			return nil, err
		}
		return &ypb.AIEventQueryResponse{
			Events: lo.Map(event, func(item *schema.AiOutputEvent, _ int) *ypb.AIOutputEvent {
				return item.ToGRPC()
			}),
		}, nil
	}

}
