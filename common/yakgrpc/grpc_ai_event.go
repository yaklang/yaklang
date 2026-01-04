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

	queryAll := func() []*schema.AiOutputEvent {
		eventCh := yakit.YieldAIEvent(ctx, db, filter)
		var events []*schema.AiOutputEvent
		for event := range eventCh {
			events = append(events, event)
		}
		return events
	}
	paging := req.GetPagination()
	// 不带分页查询
	if paging == nil || lo.IsEmpty(paging) {
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
		events := queryAll()
		return &ypb.AIEventQueryResponse{
			Events: lo.Map(events, func(item *schema.AiOutputEvent, _ int) *ypb.AIOutputEvent {
				return item.ToGRPC()
			}),
		}, nil
	} else {
		var isQueryEventUUIDByProcessID bool
		if req.GetProcessID() != "" {
			eventIDs, err := yakit.QueryAIEventIDByProcessID(db, req.GetProcessID())
			if err != nil {
				return nil, err
			}
			if len(eventIDs) == 0 {
				return nil, utils.Errorf("no events found for process ID: %s", req.GetProcessID())
			}
			filter.EventUUIDS = append(filter.EventUUIDS, eventIDs...)
			isQueryEventUUIDByProcessID = true
		}
		if isQueryEventUUIDByProcessID && len(filter.EventUUIDS) > 10 {
			return nil, utils.Errorf("event UUIDs by process ID is too many, max 10")
		}
		paginator, events, err := yakit.QueryAIEventPaging(db, filter, paging)
		if err != nil {
			return nil, err
		}
		return &ypb.AIEventQueryResponse{
			Events: lo.Map(events, func(item *schema.AiOutputEvent, _ int) *ypb.AIOutputEvent {
				return item.ToGRPC()
			}),
			Total: int64(paginator.TotalRecord),
			Pagination: &ypb.Paging{
				Page:    int64(paginator.Page),
				Limit:   int64(paginator.Limit),
				OrderBy: paging.GetOrderBy(),
				Order:   paging.GetOrder(),
			},
		}, nil
	}
}
