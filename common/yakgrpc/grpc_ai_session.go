package yakgrpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func defaultAISessionPaging() *ypb.Paging {
	return &ypb.Paging{
		Page:    1,
		Limit:   30,
		OrderBy: "updated_at",
		Order:   "desc",
	}
}

func (s *Server) QueryAISession(ctx context.Context, req *ypb.QueryAISessionRequest) (*ypb.QueryAISessionResponse, error) {
	if req == nil {
		req = &ypb.QueryAISessionRequest{}
	}

	paging := req.GetPagination()
	if paging == nil {
		paging = defaultAISessionPaging()
	}

	pag, sessions, err := yakit.QueryAISessionMetaPaging(s.GetProjectDatabase(), req.GetFilter(), paging)
	if err != nil {
		return nil, err
	}

	respData := make([]*ypb.AISession, 0, len(sessions))
	for _, item := range sessions {
		if item == nil {
			continue
		}
		respData = append(respData, &ypb.AISession{
			Id:               int64(item.ID),
			SessionID:        item.SessionID,
			Title:            item.Title,
			TitleInitialized: item.TitleInitialized,
			CreatedAt:        item.CreatedAt.Unix(),
			UpdatedAt:        item.UpdatedAt.Unix(),
		})
	}

	return &ypb.QueryAISessionResponse{
		Pagination: &ypb.Paging{
			Page:    int64(pag.Page),
			Limit:   int64(pag.Limit),
			OrderBy: paging.GetOrderBy(),
			Order:   paging.GetOrder(),
		},
		Total: int64(pag.TotalRecord),
		Data:  respData,
	}, nil
}

func (s *Server) UpdateAISessionTitle(ctx context.Context, req *ypb.UpdateAISessionTitleRequest) (*ypb.DbOperateMessage, error) {
	if req == nil {
		return nil, utils.Errorf("request is nil")
	}
	sessionID := strings.TrimSpace(req.GetSessionID())
	if sessionID == "" {
		return nil, utils.Errorf("session_id is required")
	}

	db := s.GetProjectDatabase()
	affected, err := yakit.UpdateAISessionMetaTitle(db, sessionID, req.GetTitle())
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		if _, err := yakit.CreateOrUpdateAISessionMeta(db, sessionID, req.GetTitle()); err != nil {
			return nil, err
		}
		affected = 1
	}

	return &ypb.DbOperateMessage{
		TableName:    (&schema.AISession{}).TableName(),
		Operation:    "update",
		EffectRows:   affected,
		ExtraMessage: "session_id=" + sessionID,
	}, nil
}

func (s *Server) DeleteAISession(ctx context.Context, req *ypb.DeleteAISessionRequest) (*ypb.DbOperateMessage, error) {
	if req == nil {
		return nil, utils.Errorf("request is nil")
	}

	filter := req.GetFilter()
	if filter == nil {
		filter = &ypb.DeleteAISessionFilter{}
	}

	if req.GetDeleteAll() {
		deletedSessions, deletedRuntimes, deletedEvents, deletedPlanExec, err := yakit.DeleteAllAISessionData(s.GetProjectDatabase())
		if err != nil {
			return nil, err
		}

		return &ypb.DbOperateMessage{
			TableName:  (&schema.AISession{}).TableName(),
			Operation:  "delete",
			EffectRows: deletedRuntimes + deletedEvents,
			ExtraMessage: fmt.Sprintf(
				"delete_all=true deleted_sessions=%d deleted_runtimes=%d deleted_events=%d deleted_plan_exec=%d",
				deletedSessions,
				deletedRuntimes,
				deletedEvents,
				deletedPlanExec,
			),
		}, nil
	}

	targetSessionIDs, err := yakit.QueryAISessionIDsForDelete(s.GetProjectDatabase(), filter, req.GetDeleteAll())
	if err != nil {
		return nil, err
	}
	if len(targetSessionIDs) == 0 {
		return &ypb.DbOperateMessage{
			TableName:    (&schema.AISession{}).TableName(),
			Operation:    "delete",
			EffectRows:   0,
			ExtraMessage: "no session matched",
		}, nil
	}

	var deletedRuntimes int64
	var deletedEvents int64
	for _, sessionID := range targetSessionIDs {
		runtimeCount, eventCount, err := yakit.DeleteAISession(s.GetProjectDatabase(), strings.TrimSpace(sessionID))
		if err != nil {
			return nil, err
		}
		deletedRuntimes += runtimeCount
		deletedEvents += eventCount
	}

	return &ypb.DbOperateMessage{
		TableName:  (&schema.AISession{}).TableName(),
		Operation:  "delete",
		EffectRows: deletedRuntimes + deletedEvents,
		ExtraMessage: fmt.Sprintf(
			"deleted_sessions=%d deleted_runtimes=%d deleted_events=%d",
			len(targetSessionIDs),
			deletedRuntimes,
			deletedEvents,
		),
	}, nil
}
