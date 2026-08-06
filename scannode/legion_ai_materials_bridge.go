package scannode

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func (b *legionJobBridge) handleAIMaterialsRandomQuery(ctx context.Context, raw []byte) error {
	var command aiv1.GetRandomAIMaterialsCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai materials random query command: %w", err)
	}

	ref := aiMaterialsRefFromCommand(&command)
	if err := validateAIMaterialsRandomQueryCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIMaterialsRandomQueryFailed(
			ctx,
			ref,
			"invalid_ai_materials_random_query_command",
			err.Error(),
		)
	}

	db := consts.GetGormProfileDatabase()
	if db == nil {
		return b.ensureAIPublisher().PublishAIMaterialsRandomQueryFailed(
			ctx,
			ref,
			"ai_materials_random_query_unavailable",
			"database not initialized",
		)
	}

	limit := int(command.GetLimit())
	if limit <= 0 {
		limit = 3
	}

	tools, entries, forges, err := yakit.GetRandomAIMaterials(db, limit)
	if err != nil {
		return b.ensureAIPublisher().PublishAIMaterialsRandomQueryFailed(
			ctx,
			ref,
			"ai_materials_random_query_failed",
			err.Error(),
		)
	}

	toolRecords := make([]*aiv1.AIToolRecord, 0, len(tools))
	for _, item := range tools {
		if item == nil {
			continue
		}
		toolRecords = append(toolRecords, mapSchemaAIToolToLegion(item))
	}

	forgeRecords := make([]*aiv1.AIForgeRecord, 0, len(forges))
	for _, item := range forges {
		if item == nil {
			continue
		}
		forgeRecords = append(forgeRecords, mapSchemaAIForgeToLegion(item))
	}

	entryRecords := make([]*aiv1.AIKnowledgeBaseEntryRecord, 0, len(entries))
	for _, item := range entries {
		if item == nil {
			continue
		}
		entryRecords = append(entryRecords, mapSchemaKnowledgeBaseEntryToLegion(item))
	}

	return b.ensureAIPublisher().PublishAIMaterialsRandomQueried(
		ctx,
		ref,
		entryRecords,
		toolRecords,
		forgeRecords,
	)
}

func validateAIMaterialsRandomQueryCommand(nodeID string, command *aiv1.GetRandomAIMaterialsCommand) error {
	nodeID = strings.TrimSpace(nodeID)
	switch {
	case command == nil:
		return fmt.Errorf("command is required")
	case command.GetMetadata() == nil:
		return fmt.Errorf("metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("owner_user_id is required")
	case command.GetLimit() < 0:
		return fmt.Errorf("limit must be greater than or equal to zero")
	default:
		return nil
	}
}

func aiMaterialsRefFromCommand(command *aiv1.GetRandomAIMaterialsCommand) aiMaterialsCommandRef {
	if command == nil {
		return aiMaterialsCommandRef{}
	}
	return aiMaterialsCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}
