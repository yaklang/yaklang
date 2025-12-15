package yakgrpc

import (
	"context"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryAIEvent(ctx context.Context, req *ypb.AIEventQueryRequest) (*ypb.AIEventQueryResponse, error) {
	filter := req.GetFilter()
	if filter == nil {
		filter = &ypb.AIEventFilter{}
	}
	db := s.GetProjectDatabase()
	if req.GetProcessID() != "" {
		eventIDs, err := yakit.QueryAIEventIDByProcessID(db, req.GetProcessID())
		if err != nil {
			return nil, err
		}
		if len(eventIDs) == 0 {
			return nil, utils.Errorf("no events found for process ID: %s", req.GetProcessID())
		}
		filter.EventUUIDS = append(filter.EventUUIDS, eventIDs...)
	}
	event, err := yakit.QueryAIEvent(db, filter)
	if err != nil {
		return nil, err
	}
	return &ypb.AIEventQueryResponse{
		Events: lo.Map(event, func(item *schema.AiOutputEvent, _ int) *ypb.AIOutputEvent {
			return item.ToGRPC()
		}),
	}, nil

}
