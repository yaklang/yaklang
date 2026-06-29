package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/aisessioncleanup"
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
		var lastUsedAt int64
		var runtimeIDs []string
		startParams, err := yakit.UnmarshalAISessionStartParams(item.StartParams)
		if err != nil {
			return nil, err
		}
		if !item.LastUsedAt.IsZero() {
			lastUsedAt = item.LastUsedAt.Unix()
		}
		if strings.TrimSpace(item.RelatedRuntimeIDS) != "" {
			json.Unmarshal([]byte(item.RelatedRuntimeIDS), &runtimeIDs)
		}
		respData = append(respData, &ypb.AISession{
			Id:                int64(item.ID),
			SessionID:         item.SessionID,
			Title:             item.Title,
			TitleInitialized:  item.TitleInitialized,
			CreatedAt:         item.CreatedAt.Unix(),
			UpdatedAt:         item.UpdatedAt.Unix(),
			LastUsedAt:        lastUsedAt,
			RelatedRuntimeIDs: runtimeIDs,
			StartParams:       startParams,
			Source:            item.Source,
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

	projectDB := s.GetProjectDatabase()

	if req.GetDeleteAll() {
		deletedWorkDirs, err := yakit.CleanupAISpaceWorkDirsForAllSessions(projectDB)
		if err != nil {
			return nil, err
		}

		memResult, err := aisessioncleanup.DeleteAllSessionArtifacts(projectDB)
		if err != nil {
			return nil, err
		}

		deletedSessions, deletedRuntimes, deletedEvents, deletedPlanExec, err := yakit.DeleteAllAISessionData(projectDB)
		if err != nil {
			return nil, err
		}

		return &ypb.DbOperateMessage{
			TableName:  (&schema.AISession{}).TableName(),
			Operation:  "delete",
			EffectRows: deletedRuntimes + deletedEvents,
			ExtraMessage: fmt.Sprintf(
				"delete_all=true deleted_sessions=%d deleted_runtimes=%d deleted_events=%d deleted_plan_exec=%d deleted_workdirs=%d deleted_memory_entities=%d deleted_memory_collections=%d deleted_rag_collections=%d deleted_entity_repositories=%d deleted_entity_relationships=%d deleted_knowledge_bases=%d deleted_knowledge_entries=%d",
				deletedSessions,
				deletedRuntimes,
				deletedEvents,
				deletedPlanExec,
				deletedWorkDirs,
				memResult.DeletedMemoryEntities,
				memResult.DeletedMemoryCollections,
				memResult.DeletedRAGCollections,
				memResult.DeletedEntityRepositories,
				memResult.DeletedEntityRelationships,
				memResult.DeletedKnowledgeBases,
				memResult.DeletedKnowledgeEntries,
			),
		}, nil
	}

	targetSessionIDs, err := yakit.QueryAISessionIDsForDelete(projectDB, filter, req.GetDeleteAll())
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

	deletedWorkDirs, err := yakit.CleanupAISpaceWorkDirsForSessions(projectDB, targetSessionIDs)
	if err != nil {
		return nil, err
	}

	var deletedRuntimes int64
	var deletedEvents int64
	var deletedMemoryEntities, deletedMemoryCollections, deletedRAGCollections int64
	var deletedEntityRepositories, deletedEntityRelationships int64
	var deletedKnowledgeBases, deletedKnowledgeEntries int64
	for _, sessionID := range targetSessionIDs {
		sessionID = strings.TrimSpace(sessionID)

		memResult, err := aisessioncleanup.DeleteSessionArtifacts(projectDB, sessionID)
		if err != nil {
			return nil, err
		}
		deletedMemoryEntities += memResult.DeletedMemoryEntities
		deletedMemoryCollections += memResult.DeletedMemoryCollections
		deletedRAGCollections += memResult.DeletedRAGCollections
		deletedEntityRepositories += memResult.DeletedEntityRepositories
		deletedEntityRelationships += memResult.DeletedEntityRelationships
		deletedKnowledgeBases += memResult.DeletedKnowledgeBases
		deletedKnowledgeEntries += memResult.DeletedKnowledgeEntries

		runtimeCount, eventCount, err := yakit.DeleteAISession(projectDB, sessionID)
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
			"deleted_sessions=%d deleted_runtimes=%d deleted_events=%d deleted_workdirs=%d deleted_memory_entities=%d deleted_memory_collections=%d deleted_rag_collections=%d deleted_entity_repositories=%d deleted_entity_relationships=%d deleted_knowledge_bases=%d deleted_knowledge_entries=%d",
			len(targetSessionIDs),
			deletedRuntimes,
			deletedEvents,
			deletedWorkDirs,
			deletedMemoryEntities,
			deletedMemoryCollections,
			deletedRAGCollections,
			deletedEntityRepositories,
			deletedEntityRelationships,
			deletedKnowledgeBases,
			deletedKnowledgeEntries,
		),
	}, nil
}
