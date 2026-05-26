package scannode

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jinzhu/gorm"
	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func (b *legionJobBridge) handleAIMemoryEntityCreate(ctx context.Context, raw []byte) error {
	var command aiv1.CreateAIMemoryEntityCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai memory create command: %w", err)
	}

	ref := aiMemoryRefFromCreateCommand(&command)
	if err := validateAIMemoryEntityCreateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIMemoryEntityCreateFailed(
			ctx,
			ref,
			strings.TrimSpace(command.GetSessionId()),
			"invalid_ai_memory_create_command",
			err.Error(),
		)
	}

	if err := createAIMemoryEntity(ctx, &command); err != nil {
		return b.ensureAIPublisher().PublishAIMemoryEntityCreateFailed(
			ctx,
			ref,
			strings.TrimSpace(command.GetSessionId()),
			"ai_memory_create_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIMemoryEntityCreated(ctx, ref, strings.TrimSpace(command.GetSessionId()), "ok")
}

func (b *legionJobBridge) handleAIMemoryEntityGet(ctx context.Context, raw []byte) error {
	var command aiv1.GetAIMemoryEntityCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai memory get command: %w", err)
	}

	ref := aiMemoryRefFromGetCommand(&command)
	if err := validateAIMemoryEntityGetCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIMemoryEntityFetchFailed(
			ctx,
			ref,
			"invalid_ai_memory_get_command",
			err.Error(),
		)
	}

	item, err := getAIMemoryEntity(&command)
	if err != nil {
		errorCode := "ai_memory_get_failed"
		if isAIMemoryNotFound(err) {
			errorCode = "ai_memory_not_found"
		}
		return b.ensureAIPublisher().PublishAIMemoryEntityFetchFailed(
			ctx,
			ref,
			errorCode,
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIMemoryEntityFetched(ctx, ref, item)
}

func (b *legionJobBridge) handleAIMemoryEntitiesQuery(ctx context.Context, raw []byte) error {
	var command aiv1.QueryAIMemoryEntitiesCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai memory query command: %w", err)
	}

	ref := aiMemoryRefFromQueryCommand(&command)
	if err := validateAIMemoryEntitiesQueryCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIMemoryEntitiesQueryFailed(
			ctx,
			ref,
			"invalid_ai_memory_query_command",
			err.Error(),
		)
	}

	pagination, items, total, err := queryAIMemoryEntities(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIMemoryEntitiesQueryFailed(
			ctx,
			ref,
			"ai_memory_query_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIMemoryEntitiesQueried(ctx, ref, pagination, items, total)
}

func (b *legionJobBridge) handleAIMemoryEntityUpdate(ctx context.Context, raw []byte) error {
	var command aiv1.UpdateAIMemoryEntityCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai memory update command: %w", err)
	}

	ref := aiMemoryRefFromUpdateCommand(&command)
	if err := validateAIMemoryEntityUpdateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIMemoryEntityUpdateFailed(
			ctx,
			ref,
			"invalid_ai_memory_update_command",
			err.Error(),
		)
	}

	item, err := updateAIMemoryEntity(ctx, &command)
	if err != nil {
		errorCode := "ai_memory_update_failed"
		if isAIMemoryNotFound(err) {
			errorCode = "ai_memory_not_found"
		}
		return b.ensureAIPublisher().PublishAIMemoryEntityUpdateFailed(
			ctx,
			ref,
			errorCode,
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIMemoryEntityUpdated(ctx, ref, item, "ok")
}

func (b *legionJobBridge) handleAIMemoryEntitiesDelete(ctx context.Context, raw []byte) error {
	var command aiv1.DeleteAIMemoryEntitiesCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai memory delete command: %w", err)
	}

	ref := aiMemoryRefFromDeleteCommand(&command)
	if err := validateAIMemoryEntitiesDeleteCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIMemoryEntitiesDeleteFailed(
			ctx,
			ref,
			"invalid_ai_memory_delete_command",
			err.Error(),
		)
	}

	affectedCount, err := deleteAIMemoryEntities(ctx, &command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIMemoryEntitiesDeleteFailed(
			ctx,
			ref,
			"ai_memory_delete_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIMemoryEntitiesDeleted(ctx, ref, affectedCount, "ok")
}

func (b *legionJobBridge) handleAIMemoryEntityTagsCount(ctx context.Context, raw []byte) error {
	var command aiv1.CountAIMemoryEntityTagsCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai memory tags count command: %w", err)
	}

	ref := aiMemoryRefFromCountTagsCommand(&command)
	if err := validateAIMemoryEntityTagsCountCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIMemoryEntityTagsCountFailed(
			ctx,
			ref,
			strings.TrimSpace(command.GetSessionId()),
			"invalid_ai_memory_tags_count_command",
			err.Error(),
		)
	}

	items, err := countAIMemoryEntityTags(ctx, &command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIMemoryEntityTagsCountFailed(
			ctx,
			ref,
			strings.TrimSpace(command.GetSessionId()),
			"ai_memory_tags_count_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIMemoryEntityTagsCounted(ctx, ref, strings.TrimSpace(command.GetSessionId()), items)
}

func validateAIMemoryEntityCreateCommand(nodeID string, command *aiv1.CreateAIMemoryEntityCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai memory create metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai memory create command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai memory create target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai memory create target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai memory create owner_user_id is required")
	case strings.TrimSpace(command.GetSessionId()) == "":
		return fmt.Errorf("ai memory create session_id is required")
	default:
		return nil
	}
}

func validateAIMemoryEntityGetCommand(nodeID string, command *aiv1.GetAIMemoryEntityCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai memory get metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai memory get command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai memory get target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai memory get target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai memory get owner_user_id is required")
	case strings.TrimSpace(command.GetSessionId()) == "":
		return fmt.Errorf("ai memory get session_id is required")
	case strings.TrimSpace(command.GetMemoryId()) == "":
		return fmt.Errorf("ai memory get memory_id is required")
	default:
		return nil
	}
}

func validateAIMemoryEntitiesQueryCommand(nodeID string, command *aiv1.QueryAIMemoryEntitiesCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai memory query metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai memory query command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai memory query target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai memory query target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai memory query owner_user_id is required")
	default:
		return nil
	}
}

func validateAIMemoryEntityUpdateCommand(nodeID string, command *aiv1.UpdateAIMemoryEntityCommand) error {
	item := command.GetItem()
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai memory update metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai memory update command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai memory update target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai memory update target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai memory update owner_user_id is required")
	case item == nil:
		return fmt.Errorf("ai memory update item is required")
	case strings.TrimSpace(item.GetSessionId()) == "":
		return fmt.Errorf("ai memory update session_id is required")
	case strings.TrimSpace(item.GetMemoryId()) == "":
		return fmt.Errorf("ai memory update memory_id is required")
	case strings.TrimSpace(item.GetContent()) == "":
		return fmt.Errorf("ai memory update content is required")
	default:
		return nil
	}
}

func validateAIMemoryEntitiesDeleteCommand(nodeID string, command *aiv1.DeleteAIMemoryEntitiesCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai memory delete metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai memory delete command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai memory delete target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai memory delete target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai memory delete owner_user_id is required")
	default:
		return nil
	}
}

func validateAIMemoryEntityTagsCountCommand(nodeID string, command *aiv1.CountAIMemoryEntityTagsCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai memory tags count metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai memory tags count command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai memory tags count target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai memory tags count target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai memory tags count owner_user_id is required")
	case strings.TrimSpace(command.GetSessionId()) == "":
		return fmt.Errorf("ai memory tags count session_id is required")
	default:
		return nil
	}
}

func aiMemoryRefFromCreateCommand(command *aiv1.CreateAIMemoryEntityCommand) aiMemoryCommandRef {
	return aiMemoryCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiMemoryRefFromGetCommand(command *aiv1.GetAIMemoryEntityCommand) aiMemoryCommandRef {
	return aiMemoryCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiMemoryRefFromQueryCommand(command *aiv1.QueryAIMemoryEntitiesCommand) aiMemoryCommandRef {
	return aiMemoryCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiMemoryRefFromUpdateCommand(command *aiv1.UpdateAIMemoryEntityCommand) aiMemoryCommandRef {
	return aiMemoryCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiMemoryRefFromDeleteCommand(command *aiv1.DeleteAIMemoryEntitiesCommand) aiMemoryCommandRef {
	return aiMemoryCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiMemoryRefFromCountTagsCommand(command *aiv1.CountAIMemoryEntityTagsCommand) aiMemoryCommandRef {
	return aiMemoryCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func createAIMemoryEntity(ctx context.Context, command *aiv1.CreateAIMemoryEntityCommand) error {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return utils.Errorf("database not initialized")
	}

	memory, err := aimem.NewAIMemory(
		strings.TrimSpace(command.GetSessionId()),
		aimem.WithDatabase(db),
		aimem.WithAutoReActInvoker(aicommon.WithContext(ctx)),
	)
	if err != nil {
		return err
	}
	return memory.HandleMemory(command.GetFreeInput())
}

func getAIMemoryEntity(command *aiv1.GetAIMemoryEntityCommand) (*aiv1.AIMemoryEntityRecord, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	entity, err := yakit.GetAIMemoryEntity(
		db,
		strings.TrimSpace(command.GetSessionId()),
		strings.TrimSpace(command.GetMemoryId()),
	)
	if err != nil {
		return nil, err
	}
	return mapSchemaAIMemoryEntityToLegion(entity), nil
}

func queryAIMemoryEntities(command *aiv1.QueryAIMemoryEntitiesCommand) (*aiv1.AIMemoryPagination, []*aiv1.AIMemoryEntityRecord, int64, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, nil, 0, utils.Errorf("database not initialized")
	}

	paging := mapLegionAIMemoryPaginationToGRPC(command.GetPagination())
	filter := mapLegionAIMemoryFilterToGRPC(command.GetFilter())
	if filter != nil {
		filter.SessionID = strings.TrimSpace(filter.GetSessionID())
	}

	if filter != nil && strings.TrimSpace(filter.GetSemanticQuery()) != "" {
		return queryAIMemoryEntitiesBySemantic(db, paging, filter)
	}
	if filter != nil && len(filter.GetCorePactQueryVector()) > 0 {
		return queryAIMemoryEntitiesByScoreVector(db, paging, filter)
	}

	paginator, entities, err := yakit.QueryAIMemoryEntityPaging(db, filter, paging)
	if err != nil {
		return nil, nil, 0, err
	}

	items := make([]*aiv1.AIMemoryEntityRecord, 0, len(entities))
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		items = append(items, mapSchemaAIMemoryEntityToLegion(entity))
	}

	return &aiv1.AIMemoryPagination{
		Page:    int64(paginator.Page),
		Limit:   int64(paginator.Limit),
		OrderBy: paging.GetOrderBy(),
		Order:   paging.GetOrder(),
	}, items, int64(paginator.TotalRecord), nil
}

func updateAIMemoryEntity(ctx context.Context, command *aiv1.UpdateAIMemoryEntityCommand) (*aiv1.AIMemoryEntityRecord, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	next := schema.GRPC2AIMemoryEntity(mapLegionAIMemoryRecordToGRPC(command.GetItem()))
	if next == nil {
		return nil, utils.Errorf("item is required")
	}

	var prev schema.AIMemoryEntity
	if err := db.Where("session_id = ? AND memory_id = ?", next.SessionID, next.MemoryID).First(&prev).Error; err != nil {
		return nil, err
	}
	old := prev

	prev.Content = next.Content
	prev.Tags = next.Tags
	prev.PotentialQuestions = next.PotentialQuestions
	prev.C_Score = next.C_Score
	prev.O_Score = next.O_Score
	prev.R_Score = next.R_Score
	prev.E_Score = next.E_Score
	prev.P_Score = next.P_Score
	prev.A_Score = next.A_Score
	prev.T_Score = next.T_Score
	prev.CorePactVector = next.CorePactVector

	if err := db.Save(&prev).Error; err != nil {
		return nil, err
	}

	_ = syncAIMemoryVectors(ctx, db, &prev, &old)
	return mapSchemaAIMemoryEntityToLegion(&prev), nil
}

func deleteAIMemoryEntities(ctx context.Context, command *aiv1.DeleteAIMemoryEntitiesCommand) (int64, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return 0, utils.Errorf("database not initialized")
	}

	singleton := newAIMemoryVectorSessionSingleton(db)
	return yakit.DeleteAIMemoryEntityBatched(
		ctx,
		db,
		mapLegionAIMemoryFilterToGRPC(command.GetFilter()),
		200,
		func(ctx context.Context, _ *gorm.DB, entities []schema.AIMemoryEntity) error {
			return deleteAIMemoryVectorsBatch(ctx, singleton, entities)
		},
	)
}

func countAIMemoryEntityTags(ctx context.Context, command *aiv1.CountAIMemoryEntityTagsCommand) ([]*aiv1.AIMemoryTagCount, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	items, err := yakit.CountAIMemoryEntityTags(ctx, db, strings.TrimSpace(command.GetSessionId()))
	if err != nil {
		return nil, err
	}

	result := make([]*aiv1.AIMemoryTagCount, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, &aiv1.AIMemoryTagCount{
			Value: strings.TrimSpace(item.GetValue()),
			Total: item.GetTotal(),
		})
	}
	return result, nil
}

func mapSchemaAIMemoryEntityToLegion(entity *schema.AIMemoryEntity) *aiv1.AIMemoryEntityRecord {
	if entity == nil {
		return nil
	}

	return &aiv1.AIMemoryEntityRecord{
		Id:                 int64(entity.ID),
		CreatedAt:          entity.CreatedAt.Unix(),
		UpdatedAt:          entity.UpdatedAt.Unix(),
		MemoryId:           entity.MemoryID,
		SessionId:          entity.SessionID,
		Content:            entity.Content,
		Tags:               append([]string(nil), []string(entity.Tags)...),
		PotentialQuestions: append([]string(nil), []string(entity.PotentialQuestions)...),
		CScore:             entity.C_Score,
		OScore:             entity.O_Score,
		RScore:             entity.R_Score,
		EScore:             entity.E_Score,
		PScore:             entity.P_Score,
		AScore:             entity.A_Score,
		TScore:             entity.T_Score,
		CorePactVector:     append([]float32(nil), []float32(entity.CorePactVector)...),
	}
}

func mapLegionAIMemoryRecordToGRPC(item *aiv1.AIMemoryEntityRecord) *ypb.AIMemoryEntity {
	if item == nil {
		return nil
	}

	return &ypb.AIMemoryEntity{
		Id:                 item.GetId(),
		CreatedAt:          item.GetCreatedAt(),
		UpdatedAt:          item.GetUpdatedAt(),
		MemoryID:           strings.TrimSpace(item.GetMemoryId()),
		SessionID:          strings.TrimSpace(item.GetSessionId()),
		Content:            strings.TrimSpace(item.GetContent()),
		Tags:               append([]string(nil), item.GetTags()...),
		PotentialQuestions: append([]string(nil), item.GetPotentialQuestions()...),
		CScore:             item.GetCScore(),
		OScore:             item.GetOScore(),
		RScore:             item.GetRScore(),
		EScore:             item.GetEScore(),
		PScore:             item.GetPScore(),
		AScore:             item.GetAScore(),
		TScore:             item.GetTScore(),
		CorePactVector:     append([]float32(nil), item.GetCorePactVector()...),
	}
}

func mapLegionAIMemoryFilterToGRPC(filter *aiv1.AIMemoryEntityFilter) *ypb.AIMemoryEntityFilter {
	if filter == nil {
		return nil
	}

	return &ypb.AIMemoryEntityFilter{
		SessionID:                strings.TrimSpace(filter.GetSessionId()),
		MemoryID:                 append([]string(nil), filter.GetMemoryIds()...),
		ContentKeyword:           strings.TrimSpace(filter.GetContentKeyword()),
		Tags:                     append([]string(nil), filter.GetTags()...),
		TagMatchAll:              filter.GetTagMatchAll(),
		PotentialQuestionKeyword: strings.TrimSpace(filter.GetPotentialQuestionKeyword()),
		CScore:                   mapLegionAIMemoryFloatRangeToGRPC(filter.GetCScore()),
		OScore:                   mapLegionAIMemoryFloatRangeToGRPC(filter.GetOScore()),
		RScore:                   mapLegionAIMemoryFloatRangeToGRPC(filter.GetRScore()),
		EScore:                   mapLegionAIMemoryFloatRangeToGRPC(filter.GetEScore()),
		PScore:                   mapLegionAIMemoryFloatRangeToGRPC(filter.GetPScore()),
		AScore:                   mapLegionAIMemoryFloatRangeToGRPC(filter.GetAScore()),
		TScore:                   mapLegionAIMemoryFloatRangeToGRPC(filter.GetTScore()),
		CreatedAt:                mapLegionAIMemoryInt64RangeToGRPC(filter.GetCreatedAt()),
		UpdatedAt:                mapLegionAIMemoryInt64RangeToGRPC(filter.GetUpdatedAt()),
		SemanticQuery:            strings.TrimSpace(filter.GetSemanticQuery()),
		CorePactQueryVector:      append([]float32(nil), filter.GetCorePactQueryVector()...),
		VectorTopK:               filter.GetVectorTopK(),
	}
}

func mapLegionAIMemoryPaginationToGRPC(pagination *aiv1.AIMemoryPagination) *ypb.Paging {
	if pagination == nil {
		return &ypb.Paging{Page: 1, Limit: 10, OrderBy: "created_at", Order: "desc"}
	}

	page := pagination.GetPage()
	limit := pagination.GetLimit()
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}
	return &ypb.Paging{
		Page:    page,
		Limit:   limit,
		OrderBy: pagination.GetOrderBy(),
		Order:   pagination.GetOrder(),
	}
}

func mapLegionAIMemoryFloatRangeToGRPC(value *aiv1.AIMemoryFloatRange) *ypb.FloatRange {
	if value == nil {
		return nil
	}
	return &ypb.FloatRange{
		Enabled: value.GetEnabled(),
		Min:     value.GetMin(),
		Max:     value.GetMax(),
	}
}

func mapLegionAIMemoryInt64RangeToGRPC(value *aiv1.AIMemoryInt64Range) *ypb.Int64Range {
	if value == nil {
		return nil
	}
	return &ypb.Int64Range{
		Enabled: value.GetEnabled(),
		Min:     value.GetMin(),
		Max:     value.GetMax(),
	}
}

func queryAIMemoryEntitiesBySemantic(
	db *gorm.DB,
	paging *ypb.Paging,
	filter *ypb.AIMemoryEntityFilter,
) (*aiv1.AIMemoryPagination, []*aiv1.AIMemoryEntityRecord, int64, error) {
	sessionID := strings.TrimSpace(filter.GetSessionID())
	if sessionID == "" {
		return nil, nil, 0, utils.Errorf("session_id is required for semantic query")
	}

	semanticQuery := strings.TrimSpace(filter.GetSemanticQuery())
	if semanticQuery == "" {
		return nil, nil, 0, utils.Errorf("semantic_query is required")
	}

	topK := int(paging.GetPage() * paging.GetLimit())
	if filter.GetVectorTopK() > int64(topK) {
		topK = int(filter.GetVectorTopK())
	}
	if topK <= 0 {
		topK = 10
	}

	triage, err := aimem.NewAIMemoryForQuery(sessionID, aimem.WithDatabase(db))
	if err != nil {
		return nil, nil, 0, err
	}
	idResults, err := triage.SearchBySemanticsMemoryIDs(semanticQuery, topK)
	if err != nil {
		return nil, nil, 0, err
	}

	orderedMemoryIDs := make([]string, 0, len(idResults))
	for _, result := range idResults {
		if result == nil || result.Entity == nil {
			continue
		}
		memoryID := strings.TrimSpace(result.Entity.Id)
		if memoryID == "" {
			continue
		}
		orderedMemoryIDs = append(orderedMemoryIDs, memoryID)
	}

	if len(orderedMemoryIDs) == 0 {
		return &aiv1.AIMemoryPagination{
			Page:    1,
			Limit:   int64(topK),
			OrderBy: paging.GetOrderBy(),
			Order:   paging.GetOrder(),
		}, []*aiv1.AIMemoryEntityRecord{}, 0, nil
	}

	query := yakit.FilterAIMemoryEntity(db, filter).Where("memory_id IN (?)", orderedMemoryIDs)
	var entities []*schema.AIMemoryEntity
	if err := query.Find(&entities).Error; err != nil {
		return nil, nil, 0, err
	}

	entityByMemoryID := make(map[string]*schema.AIMemoryEntity, len(entities))
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		entityByMemoryID[entity.MemoryID] = entity
	}

	items := make([]*aiv1.AIMemoryEntityRecord, 0, len(orderedMemoryIDs))
	for _, memoryID := range orderedMemoryIDs {
		entity := entityByMemoryID[memoryID]
		if entity == nil {
			continue
		}
		items = append(items, mapSchemaAIMemoryEntityToLegion(entity))
	}

	return &aiv1.AIMemoryPagination{
		Page:    1,
		Limit:   int64(topK),
		OrderBy: paging.GetOrderBy(),
		Order:   paging.GetOrder(),
	}, items, int64(len(items)), nil
}

func queryAIMemoryEntitiesByScoreVector(
	db *gorm.DB,
	paging *ypb.Paging,
	filter *ypb.AIMemoryEntityFilter,
) (*aiv1.AIMemoryPagination, []*aiv1.AIMemoryEntityRecord, int64, error) {
	sessionID := strings.TrimSpace(filter.GetSessionID())
	if sessionID == "" {
		return nil, nil, 0, utils.Errorf("session_id is required for score-vector query")
	}

	queryVector := filter.GetCorePactQueryVector()
	if len(queryVector) != 7 {
		return nil, nil, 0, utils.Errorf("core_pact_query_vector must be 7 dimensions, got %d", len(queryVector))
	}

	topK := int(paging.GetPage() * paging.GetLimit())
	if filter.GetVectorTopK() > int64(topK) {
		topK = int(filter.GetVectorTopK())
	}
	if topK <= 0 {
		topK = 10
	}

	triage, err := aimem.NewAIMemoryForQuery(sessionID, aimem.WithDatabase(db))
	if err != nil {
		return nil, nil, 0, err
	}

	searchResults, err := triage.SearchByScoreVectorMemoryIDs(queryVector, topK)
	if err != nil {
		return nil, nil, 0, err
	}

	orderedMemoryIDs := make([]string, 0, len(searchResults))
	for _, result := range searchResults {
		if result == nil || result.Entity == nil {
			continue
		}
		memoryID := strings.TrimSpace(result.Entity.Id)
		if memoryID == "" {
			continue
		}
		orderedMemoryIDs = append(orderedMemoryIDs, memoryID)
	}

	if len(orderedMemoryIDs) == 0 {
		return &aiv1.AIMemoryPagination{
			Page:    paging.GetPage(),
			Limit:   paging.GetLimit(),
			OrderBy: paging.GetOrderBy(),
			Order:   paging.GetOrder(),
		}, []*aiv1.AIMemoryEntityRecord{}, 0, nil
	}

	query := yakit.FilterAIMemoryEntity(db, filter).Where("memory_id IN (?)", orderedMemoryIDs)
	var entities []*schema.AIMemoryEntity
	if err := query.Find(&entities).Error; err != nil {
		return nil, nil, 0, err
	}

	entityByMemoryID := make(map[string]*schema.AIMemoryEntity, len(entities))
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		entityByMemoryID[entity.MemoryID] = entity
	}

	allItems := make([]*aiv1.AIMemoryEntityRecord, 0, len(orderedMemoryIDs))
	for _, memoryID := range orderedMemoryIDs {
		entity := entityByMemoryID[memoryID]
		if entity == nil {
			continue
		}
		allItems = append(allItems, mapSchemaAIMemoryEntityToLegion(entity))
	}

	page := int(paging.GetPage())
	limit := int(paging.GetLimit())
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}
	start := (page - 1) * limit
	if start >= len(allItems) {
		return &aiv1.AIMemoryPagination{
			Page:    paging.GetPage(),
			Limit:   paging.GetLimit(),
			OrderBy: paging.GetOrderBy(),
			Order:   paging.GetOrder(),
		}, []*aiv1.AIMemoryEntityRecord{}, int64(len(allItems)), nil
	}
	end := start + limit
	if end > len(allItems) {
		end = len(allItems)
	}

	return &aiv1.AIMemoryPagination{
		Page:    paging.GetPage(),
		Limit:   paging.GetLimit(),
		OrderBy: paging.GetOrderBy(),
		Order:   paging.GetOrder(),
	}, allItems[start:end], int64(len(allItems)), nil
}

type aiMemoryVectorSessionSingleton struct {
	db *gorm.DB

	mu sync.Mutex

	hnswBackends map[string]*aimem.AIMemoryHNSWBackend
	ragStores    map[string]*vectorstore.SQLiteVectorStoreHNSW
	ragExists    map[string]bool
}

func newAIMemoryVectorSessionSingleton(db *gorm.DB) *aiMemoryVectorSessionSingleton {
	return &aiMemoryVectorSessionSingleton{
		db:           db,
		hnswBackends: make(map[string]*aimem.AIMemoryHNSWBackend, 8),
		ragStores:    make(map[string]*vectorstore.SQLiteVectorStoreHNSW, 8),
		ragExists:    make(map[string]bool, 8),
	}
}

func (s *aiMemoryVectorSessionSingleton) GetHNSWBackend(sessionID string) (*aimem.AIMemoryHNSWBackend, error) {
	s.mu.Lock()
	if backend := s.hnswBackends[sessionID]; backend != nil {
		s.mu.Unlock()
		return backend, nil
	}
	s.mu.Unlock()

	backend, err := aimem.NewAIMemoryHNSWBackend(
		aimem.WithHNSWSessionID(sessionID),
		aimem.WithHNSWDatabase(s.db),
		aimem.WithHNSWAutoSave(false),
	)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	if existed := s.hnswBackends[sessionID]; existed != nil {
		s.mu.Unlock()
		return existed, nil
	}
	s.hnswBackends[sessionID] = backend
	s.mu.Unlock()
	return backend, nil
}

func (s *aiMemoryVectorSessionSingleton) GetRAGStore(sessionID string) (*vectorstore.SQLiteVectorStoreHNSW, bool, error) {
	collectionName := aimem.Session2MemoryName(sessionID)

	s.mu.Lock()
	if store := s.ragStores[collectionName]; store != nil {
		s.mu.Unlock()
		return store, true, nil
	}
	if ok, exists := s.ragExists[collectionName]; exists && !ok {
		s.mu.Unlock()
		return nil, false, nil
	}
	s.mu.Unlock()

	if !vectorstore.HasCollection(s.db, collectionName) {
		s.mu.Lock()
		s.ragExists[collectionName] = false
		s.mu.Unlock()
		return nil, false, nil
	}

	store, err := vectorstore.LoadCollection(s.db, collectionName, vectorstore.WithEmbeddingClient(rag.NewEmptyMockEmbedding()))
	if err != nil {
		return nil, false, err
	}

	s.mu.Lock()
	if existed := s.ragStores[collectionName]; existed != nil {
		s.mu.Unlock()
		return existed, true, nil
	}
	s.ragStores[collectionName] = store
	s.ragExists[collectionName] = true
	s.mu.Unlock()
	return store, true, nil
}

func deleteAIMemoryVectorsBatch(ctx context.Context, singleton *aiMemoryVectorSessionSingleton, entities []schema.AIMemoryEntity) error {
	if len(entities) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	uniqueStrings := func(input []string) []string {
		if len(input) <= 1 {
			return input
		}
		seen := make(map[string]struct{}, len(input))
		out := make([]string, 0, len(input))
		for _, value := range input {
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
		return out
	}

	type sessionPayload struct {
		memoryIDs []string
		docIDs    []string
	}

	bySession := make(map[string]*sessionPayload, 8)
	for index := range entities {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		sessionID := strings.TrimSpace(entities[index].SessionID)
		if sessionID == "" {
			continue
		}
		payload := bySession[sessionID]
		if payload == nil {
			payload = &sessionPayload{}
			bySession[sessionID] = payload
		}
		if entities[index].MemoryID != "" {
			payload.memoryIDs = append(payload.memoryIDs, entities[index].MemoryID)
		}
		if ids := entities[index].DocumentQuestionHashIDs(); len(ids) > 0 {
			payload.docIDs = append(payload.docIDs, ids...)
		}
	}

	for sessionID, payload := range bySession {
		payload.memoryIDs = uniqueStrings(payload.memoryIDs)
		payload.docIDs = uniqueStrings(payload.docIDs)

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		backend, err := singleton.GetHNSWBackend(sessionID)
		if err == nil {
			for _, memoryID := range payload.memoryIDs {
				_ = backend.Delete(memoryID)
			}
			if len(payload.memoryIDs) > 0 {
				if err := backend.SaveGraph(); err != nil {
					log.Warnf("AIMemory HNSW save skipped: %v", err)
				}
			}
		} else {
			log.Warnf("AIMemory HNSW delete skipped: %v", err)
		}

		if len(payload.docIDs) == 0 {
			continue
		}

		store, ok, err := singleton.GetRAGStore(sessionID)
		if err != nil {
			log.Warnf("AIMemory RAG delete skipped: %v", err)
			continue
		}
		if !ok {
			continue
		}
		if err := store.Delete(payload.docIDs...); err != nil {
			log.Warnf("AIMemory RAG delete docs skipped: %v", err)
		}
	}
	return nil
}

func syncAIMemoryVectors(ctx context.Context, db *gorm.DB, entity *schema.AIMemoryEntity, prev *schema.AIMemoryEntity) error {
	if entity == nil {
		return nil
	}

	hnswBackend, err := aimem.NewAIMemoryHNSWBackend(
		aimem.WithHNSWSessionID(entity.SessionID),
		aimem.WithHNSWDatabase(db),
	)
	if err == nil {
		_ = hnswBackend.Update(toAIMemoryEntity(entity))
	} else {
		log.Warnf("AIMemory HNSW update skipped: %v", err)
	}

	if err := syncAIMemorySemanticIndex(ctx, db, entity, prev); err != nil {
		log.Warnf("AIMemory RAG index update skipped: %v", err)
	}
	return nil
}

func syncAIMemorySemanticIndex(ctx context.Context, db *gorm.DB, entity *schema.AIMemoryEntity, prev *schema.AIMemoryEntity) error {
	sessionID := entity.SessionID
	collectionName := aimem.Session2MemoryName(sessionID)

	embeddingAvailable := rag.CheckConfigEmbeddingAvailable(rag.WithDB(db))
	if !embeddingAvailable && !vectorstore.HasCollection(db, collectionName) {
		return nil
	}

	store, err := vectorstore.LoadCollection(db, collectionName, vectorstore.WithEmbeddingClient(rag.NewEmptyMockEmbedding()))
	if err != nil {
		if !embeddingAvailable {
			return nil
		}
		store = nil
	}

	if prev != nil && len(prev.PotentialQuestions) > 0 && store != nil {
		ids := prev.DocumentQuestionHashIDs()
		if len(ids) > 0 {
			_ = store.Delete(ids...)
		}
	}

	if !embeddingAvailable {
		return nil
	}

	system, err := rag.GetRagSystem(collectionName, rag.WithDB(db))
	if err != nil {
		return err
	}

	for _, question := range entity.PotentialQuestions {
		question = strings.TrimSpace(question)
		if question == "" {
			continue
		}
		docID := entity.QuestionHashID(question)
		if err := system.Add(
			docID,
			question,
			rag.WithDocumentMetadataKeyValue("memory_id", entity.MemoryID),
			rag.WithDocumentMetadataKeyValue("question", question),
			rag.WithDocumentMetadataKeyValue("session_id", entity.SessionID),
		); err != nil {
			log.Warnf("AIMemory RAG add doc failed: %v", err)
		}
	}
	return nil
}

func toAIMemoryEntity(entity *schema.AIMemoryEntity) *aicommon.MemoryEntity {
	if entity == nil {
		return nil
	}

	return &aicommon.MemoryEntity{
		Id:                 entity.MemoryID,
		CreatedAt:          entity.CreatedAt,
		Content:            entity.Content,
		Tags:               []string(entity.Tags),
		PotentialQuestions: []string(entity.PotentialQuestions),
		C_Score:            entity.C_Score,
		O_Score:            entity.O_Score,
		R_Score:            entity.R_Score,
		E_Score:            entity.E_Score,
		P_Score:            entity.P_Score,
		A_Score:            entity.A_Score,
		T_Score:            entity.T_Score,
		CorePactVector:     []float32(entity.CorePactVector),
	}
}

func isAIMemoryNotFound(err error) bool {
	if err == nil {
		return false
	}
	if err == gorm.ErrRecordNotFound {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "record not found")
}
