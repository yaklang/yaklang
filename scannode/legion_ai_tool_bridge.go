package scannode

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata/genmetadata"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func (b *legionJobBridge) handleAIToolsList(ctx context.Context, raw []byte) error {
	var command aiv1.ListAIToolsCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai tools list command: %w", err)
	}

	ref := aiToolRefFromListCommand(&command)
	if err := validateAIToolsListCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIToolsListFailed(
			ctx,
			ref,
			"invalid_ai_tools_list_command",
			err.Error(),
		)
	}

	items, pagination, total, err := listAITools(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIToolsListFailed(
			ctx,
			ref,
			"ai_tools_list_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIToolsListed(ctx, ref, items, pagination, total)
}

func (b *legionJobBridge) handleAIToolCreate(ctx context.Context, raw []byte) error {
	var command aiv1.CreateAIToolCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai tool create command: %w", err)
	}

	ref := aiToolRefFromCreateCommand(&command)
	if err := validateAIToolCreateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIToolCreateFailed(
			ctx,
			ref,
			"invalid_ai_tool_create_command",
			err.Error(),
		)
	}

	db := consts.GetGormProfileDatabase()
	if db == nil {
		return b.ensureAIPublisher().PublishAIToolCreateFailed(
			ctx,
			ref,
			"ai_tool_create_unavailable",
			"database not initialized",
		)
	}

	tool := &schema.AIYakTool{
		Name:        strings.TrimSpace(command.GetName()),
		Description: strings.TrimSpace(command.GetDescription()),
		Content:     command.GetContent(),
		Path:        strings.TrimSpace(command.GetToolPath()),
		Keywords:    strings.Join(normalizeAIToolKeywords(command.GetKeywords()), ","),
	}
	if err := fixAIToolMetadata(tool); err != nil {
		return b.ensureAIPublisher().PublishAIToolCreateFailed(
			ctx,
			ref,
			"ai_tool_create_failed",
			err.Error(),
		)
	}
	tool.EnableAIOutputLog = yakscripttools.ParseAIToolEnableAIOutputLog(tool.Content)
	if _, err := yakit.CreateAIYakTool(db, tool); err != nil {
		errorCode := "ai_tool_create_failed"
		if isAIToolNameUniqueConflict(err) {
			errorCode = "ai_tool_conflict"
		}
		return b.ensureAIPublisher().PublishAIToolCreateFailed(
			ctx,
			ref,
			errorCode,
			err.Error(),
		)
	}
	fresh, err := yakit.GetAIYakToolByID(db, tool.ID)
	if err != nil {
		return b.ensureAIPublisher().PublishAIToolCreateFailed(
			ctx,
			ref,
			"ai_tool_create_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIToolCreated(ctx, ref, mapSchemaAIToolToLegion(fresh), "ok")
}

func (b *legionJobBridge) handleAIToolUpdate(ctx context.Context, raw []byte) error {
	var command aiv1.UpdateAIToolCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai tool update command: %w", err)
	}

	ref := aiToolRefFromUpdateCommand(&command)
	if err := validateAIToolUpdateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIToolUpdateFailed(
			ctx,
			ref,
			"invalid_ai_tool_update_command",
			err.Error(),
		)
	}

	db := consts.GetGormProfileDatabase()
	if db == nil {
		return b.ensureAIPublisher().PublishAIToolUpdateFailed(
			ctx,
			ref,
			"ai_tool_update_unavailable",
			"database not initialized",
		)
	}

	tool := &schema.AIYakTool{
		Name:        strings.TrimSpace(command.GetName()),
		Description: strings.TrimSpace(command.GetDescription()),
		Content:     command.GetContent(),
		Path:        strings.TrimSpace(command.GetToolPath()),
		Keywords:    strings.Join(normalizeAIToolKeywords(command.GetKeywords()), ","),
	}
	tool.ID = uint(command.GetToolId())
	if err := fixAIToolMetadata(tool); err != nil {
		return b.ensureAIPublisher().PublishAIToolUpdateFailed(
			ctx,
			ref,
			"ai_tool_update_failed",
			err.Error(),
		)
	}
	tool.EnableAIOutputLog = yakscripttools.ParseAIToolEnableAIOutputLog(tool.Content)
	affected, err := yakit.UpdateAIYakToolByID(db, tool)
	if err != nil {
		errorCode := "ai_tool_update_failed"
		if isAIToolNameUniqueConflict(err) {
			errorCode = "ai_tool_conflict"
		}
		if strings.Contains(strings.ToLower(err.Error()), "record not found") {
			errorCode = "ai_tool_not_found"
		}
		return b.ensureAIPublisher().PublishAIToolUpdateFailed(
			ctx,
			ref,
			errorCode,
			err.Error(),
		)
	}
	if affected == 0 {
		return b.ensureAIPublisher().PublishAIToolUpdateFailed(
			ctx,
			ref,
			"ai_tool_not_found",
			"ai tool not found",
		)
	}
	fresh, err := yakit.GetAIYakToolByID(db, uint(command.GetToolId()))
	if err != nil {
		return b.ensureAIPublisher().PublishAIToolUpdateFailed(
			ctx,
			ref,
			"ai_tool_update_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIToolUpdated(ctx, ref, mapSchemaAIToolToLegion(fresh), "ok")
}

func (b *legionJobBridge) handleAIToolGenerateMetadata(ctx context.Context, raw []byte) error {
	var command aiv1.GenerateAIToolMetadataCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai tool metadata generate command: %w", err)
	}

	ref := aiToolRefFromGenerateMetadataCommand(&command)
	if err := validateAIToolGenerateMetadataCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIToolMetadataGenerateFailed(
			ctx,
			ref,
			"invalid_ai_tool_metadata_generate_command",
			err.Error(),
		)
	}

	metadata, err := genmetadata.GenerateMetadataFromCodeContent(
		strings.TrimSpace(command.GetToolName()),
		command.GetContent(),
	)
	if err != nil {
		return b.ensureAIPublisher().PublishAIToolMetadataGenerateFailed(
			ctx,
			ref,
			"ai_tool_metadata_generate_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIToolMetadataGenerated(
		ctx,
		ref,
		strings.TrimSpace(metadata.Name),
		strings.TrimSpace(metadata.Description),
		normalizeAIToolKeywords(metadata.Keywords),
	)
}

func (b *legionJobBridge) handleAIToolFavoriteToggle(ctx context.Context, raw []byte) error {
	var command aiv1.ToggleAIToolFavoriteCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai tool favorite toggle command: %w", err)
	}

	ref := aiToolRefFromFavoriteToggleCommand(&command)
	if err := validateAIToolFavoriteToggleCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIToolFavoriteToggleFailed(
			ctx,
			ref,
			"invalid_ai_tool_favorite_toggle_command",
			err.Error(),
		)
	}

	db := consts.GetGormProfileDatabase()
	if db == nil {
		return b.ensureAIPublisher().PublishAIToolFavoriteToggleFailed(
			ctx,
			ref,
			"ai_tool_favorite_toggle_unavailable",
			"database not initialized",
		)
	}
	isFavorite, err := yakit.ToggleAIYakToolFavoriteByID(db, uint(command.GetToolId()))
	if err != nil {
		return b.ensureAIPublisher().PublishAIToolFavoriteToggleFailed(
			ctx,
			ref,
			"ai_tool_favorite_toggle_failed",
			err.Error(),
		)
	}
	message := "Tool added to favorites"
	if !isFavorite {
		message = "Tool removed from favorites"
	}
	return b.ensureAIPublisher().PublishAIToolFavoriteToggled(
		ctx,
		ref,
		command.GetToolId(),
		isFavorite,
		message,
	)
}

func (b *legionJobBridge) handleAIToolsDelete(ctx context.Context, raw []byte) error {
	var command aiv1.DeleteAIToolsCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai tools delete command: %w", err)
	}

	ref := aiToolRefFromDeleteCommand(&command)
	if err := validateAIToolsDeleteCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIToolsDeleteFailed(
			ctx,
			ref,
			"invalid_ai_tools_delete_command",
			err.Error(),
		)
	}

	db := consts.GetGormProfileDatabase()
	if db == nil {
		return b.ensureAIPublisher().PublishAIToolsDeleteFailed(
			ctx,
			ref,
			"ai_tools_delete_unavailable",
			"database not initialized",
		)
	}

	ids := command.GetToolIds()
	uintIDs := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		uintIDs = append(uintIDs, uint(id))
	}
	if len(uintIDs) == 0 {
		return b.ensureAIPublisher().PublishAIToolsDeleteFailed(
			ctx,
			ref,
			"invalid_ai_tools_delete_command",
			"tool_ids must contain at least one positive id",
		)
	}
	if _, err := yakit.DeleteAIYakToolByID(db, uintIDs...); err != nil {
		return b.ensureAIPublisher().PublishAIToolsDeleteFailed(
			ctx,
			ref,
			"ai_tools_delete_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIToolsDeleted(ctx, ref, ids, "ok")
}

func validateAIToolsListCommand(nodeID string, command *aiv1.ListAIToolsCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai tools list metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai tools list command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai tools list target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai tools list target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai tools list owner_user_id is required")
	default:
		return nil
	}
}

func validateAIToolCreateCommand(nodeID string, command *aiv1.CreateAIToolCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai tool create metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai tool create command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai tool create target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai tool create target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai tool create owner_user_id is required")
	case strings.TrimSpace(command.GetName()) == "":
		return fmt.Errorf("ai tool create name is required")
	case strings.TrimSpace(command.GetContent()) == "":
		return fmt.Errorf("ai tool create content is required")
	default:
		return nil
	}
}

func validateAIToolUpdateCommand(nodeID string, command *aiv1.UpdateAIToolCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai tool update metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai tool update command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai tool update target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai tool update target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai tool update owner_user_id is required")
	case command.GetToolId() <= 0:
		return fmt.Errorf("ai tool update tool_id must be greater than 0")
	case strings.TrimSpace(command.GetName()) == "":
		return fmt.Errorf("ai tool update name is required")
	case strings.TrimSpace(command.GetContent()) == "":
		return fmt.Errorf("ai tool update content is required")
	default:
		return nil
	}
}

func validateAIToolFavoriteToggleCommand(nodeID string, command *aiv1.ToggleAIToolFavoriteCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai tool favorite toggle metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai tool favorite toggle command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai tool favorite toggle target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai tool favorite toggle target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai tool favorite toggle owner_user_id is required")
	case command.GetToolId() <= 0:
		return fmt.Errorf("ai tool favorite toggle tool_id must be greater than 0")
	default:
		return nil
	}
}

func validateAIToolGenerateMetadataCommand(
	nodeID string,
	command *aiv1.GenerateAIToolMetadataCommand,
) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai tool metadata generate metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai tool metadata generate command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai tool metadata generate target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai tool metadata generate target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai tool metadata generate owner_user_id is required")
	case strings.TrimSpace(command.GetContent()) == "":
		return fmt.Errorf("ai tool metadata generate content is required")
	default:
		return nil
	}
}

func validateAIToolsDeleteCommand(nodeID string, command *aiv1.DeleteAIToolsCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai tools delete metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai tools delete command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai tools delete target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai tools delete target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai tools delete owner_user_id is required")
	case len(command.GetToolIds()) == 0:
		return fmt.Errorf("ai tools delete tool_ids is required")
	default:
		return nil
	}
}

func aiToolRefFromListCommand(command *aiv1.ListAIToolsCommand) aiToolCommandRef {
	return aiToolCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiToolRefFromCreateCommand(command *aiv1.CreateAIToolCommand) aiToolCommandRef {
	return aiToolCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiToolRefFromUpdateCommand(command *aiv1.UpdateAIToolCommand) aiToolCommandRef {
	return aiToolCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiToolRefFromFavoriteToggleCommand(command *aiv1.ToggleAIToolFavoriteCommand) aiToolCommandRef {
	return aiToolCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiToolRefFromGenerateMetadataCommand(
	command *aiv1.GenerateAIToolMetadataCommand,
) aiToolCommandRef {
	return aiToolCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiToolRefFromDeleteCommand(command *aiv1.DeleteAIToolsCommand) aiToolCommandRef {
	return aiToolCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func listAITools(command *aiv1.ListAIToolsCommand) ([]*aiv1.AIToolRecord, *aiv1.AIToolPagination, int64, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, nil, 0, utils.Errorf("database not initialized")
	}

	pagination := command.GetPagination()
	if pagination == nil {
		pagination = &aiv1.AIToolPagination{Page: 1, Limit: 20}
	}
	if pagination.GetPage() <= 0 {
		pagination.Page = 1
	}
	if pagination.GetLimit() <= 0 {
		pagination.Limit = 20
	}

	if command.GetToolName() != "" {
		tool, err := yakit.GetAIYakTool(db, command.GetToolName())
		if err != nil {
			return []*aiv1.AIToolRecord{}, &aiv1.AIToolPagination{
				Page:    pagination.GetPage(),
				Limit:   pagination.GetLimit(),
				OrderBy: pagination.GetOrderBy(),
				Order:   pagination.GetOrder(),
			}, 0, nil
		}
		return []*aiv1.AIToolRecord{mapSchemaAIToolToLegion(tool)}, &aiv1.AIToolPagination{
			Page:    1,
			Limit:   1,
			OrderBy: pagination.GetOrderBy(),
			Order:   pagination.GetOrder(),
		}, 1, nil
	}

	if command.GetToolId() != 0 {
		tool, err := yakit.GetAIYakToolByID(db, uint(command.GetToolId()))
		if err != nil {
			return []*aiv1.AIToolRecord{}, &aiv1.AIToolPagination{
				Page:    pagination.GetPage(),
				Limit:   pagination.GetLimit(),
				OrderBy: pagination.GetOrderBy(),
				Order:   pagination.GetOrder(),
			}, 0, nil
		}
		return []*aiv1.AIToolRecord{mapSchemaAIToolToLegion(tool)}, &aiv1.AIToolPagination{
			Page:    1,
			Limit:   1,
			OrderBy: pagination.GetOrderBy(),
			Order:   pagination.GetOrder(),
		}, 1, nil
	}

	paginator, tools, err := yakit.SearchAIYakToolWithPagination(db, command.GetQuery(), command.GetOnlyFavorites(), &ypb.Paging{
		Page:    pagination.GetPage(),
		Limit:   pagination.GetLimit(),
		OrderBy: pagination.GetOrderBy(),
		Order:   pagination.GetOrder(),
	})
	if err != nil {
		return nil, nil, 0, err
	}

	items := make([]*aiv1.AIToolRecord, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		items = append(items, mapSchemaAIToolToLegion(tool))
	}
	return items, &aiv1.AIToolPagination{
		Page:    int64(paginator.Page),
		Limit:   int64(paginator.Limit),
		OrderBy: pagination.GetOrderBy(),
		Order:   pagination.GetOrder(),
	}, int64(paginator.TotalRecord), nil
}

func mapSchemaAIToolToLegion(tool *schema.AIYakTool) *aiv1.AIToolRecord {
	if tool == nil {
		return nil
	}
	return &aiv1.AIToolRecord{
		Id:          int64(tool.ID),
		Name:        tool.Name,
		VerboseName: tool.VerboseName,
		Description: tool.Description,
		Content:     tool.Content,
		ToolPath:    tool.Path,
		Keywords:    utils.PrettifyListFromStringSplitEx(tool.Keywords, ",", "|"),
		IsFavorite:  tool.IsFavorite,
		Author:      tool.Author,
		CreatedAt:   tool.CreatedAt.Unix(),
		UpdatedAt:   tool.UpdatedAt.Unix(),
		IsBuiltin:   tool.IsBuiltin,
	}
}

func normalizeAIToolKeywords(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		keyword := strings.TrimSpace(item)
		if keyword == "" {
			continue
		}
		if _, ok := seen[keyword]; ok {
			continue
		}
		seen[keyword] = struct{}{}
		result = append(result, keyword)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func fixAIToolMetadata(tool *schema.AIYakTool) error {
	parsed := yakscripttools.LoadYakScriptToAiTools(tool.Name, tool.Content)
	if parsed == nil {
		return utils.Errorf("failed to load yak script to AI tool")
	}
	if tool.Params == "" {
		tool.Params = parsed.Params
	}
	if tool.VerboseName == "" {
		tool.VerboseName = parsed.VerboseName
	}
	if tool.Keywords == "" {
		tool.Keywords = parsed.Keywords
	}
	if tool.Description == "" {
		tool.Description = parsed.Description
	}
	if tool.Path == "" {
		tool.Path = parsed.Path
	}
	return nil
}

func isAIToolNameUniqueConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed: ai_yak_tools.name") ||
		strings.Contains(message, "duplicate key value violates unique constraint") ||
		strings.Contains(message, "duplicate entry")
}
