package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/bizhelper"

	"github.com/jinzhu/gorm"
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
		eventCh := yakit.YieldAIEvent(ctx, db, filter, bizhelper.WithYieldModel_PageSize(50))
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
			eventIDs, err := yakit.QueryAIEventIDByProcessID(db.Debug(), req.GetProcessID())
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

func (s *Server) DeleteAIEvent(ctx context.Context, req *ypb.AIEventDeleteRequest) (*ypb.DbOperateMessage, error) {
	if req == nil {
		req = &ypb.AIEventDeleteRequest{}
	}

	db := s.GetProjectDatabase()

	if req.GetClearAll() {

		// Fast clear: drop + recreate event tables.
		if err := yakit.DropAIEventTables(db); err != nil {
			return nil, err
		}
		return &ypb.DbOperateMessage{
			TableName:    "ai_output_events",
			Operation:    "clear",
			EffectRows:   0,
			ExtraMessage: "fast cleared by drop+automigrate",
			CreateID:     0,
		}, nil
	}

	filter := req.GetFilter()
	if filter == nil {
		return nil, utils.Errorf("filter is required unless ClearAll is true")
	}

	if filter.GetSessionID() != "" {
		effectCount, err := yakit.DeleteAIEventBySessionID(db, filter.GetSessionID())
		if err != nil {
			return nil, err
		}
		return &ypb.DbOperateMessage{
			TableName:  "ai_output_events",
			Operation:  "delete",
			EffectRows: effectCount,
		}, nil
	}

	// Generic delete by filter (chunked) to avoid huge memory usage.
	const chunkSize = 500
	var deleted int64
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		eventCh := yakit.YieldAIEvent(ctx, tx, filter)
		seen := make(map[string]struct{}, chunkSize)
		buf := make([]string, 0, chunkSize)

		flush := func() error {
			if len(buf) == 0 {
				return nil
			}
			if r := tx.Model(&schema.AiProcessAndAiEvent{}).Where("event_id IN (?)", buf).Delete(&schema.AiProcessAndAiEvent{}); r.Error != nil {
				return r.Error
			}
			r := tx.Model(&schema.AiOutputEvent{}).Where("event_uuid IN (?)", buf).Unscoped().Delete(&schema.AiOutputEvent{})
			if r.Error != nil {
				return r.Error
			}
			deleted += r.RowsAffected
			clear(seen)
			buf = buf[:0]
			return nil
		}

		for event := range eventCh {
			if event == nil || event.EventUUID == "" {
				continue
			}
			if _, ok := seen[event.EventUUID]; ok {
				continue
			}
			seen[event.EventUUID] = struct{}{}
			buf = append(buf, event.EventUUID)
			if len(buf) >= chunkSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
		return flush()
	})
	if err != nil {
		return nil, err
	}

	return &ypb.DbOperateMessage{
		TableName:  "ai_output_events",
		Operation:  "delete",
		EffectRows: deleted,
	}, nil
}
