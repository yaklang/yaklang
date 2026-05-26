package scannode

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"google.golang.org/protobuf/proto"

	scriptmetadata "github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	forgepkg "github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func (b *legionJobBridge) handleAIForgesList(ctx context.Context, raw []byte) error {
	var command aiv1.ListAIForgesCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai forges list command: %w", err)
	}

	ref := aiForgeRefFromListCommand(&command)
	if err := validateAIForgesListCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIForgesListFailed(
			ctx,
			ref,
			"invalid_ai_forges_list_command",
			err.Error(),
		)
	}

	items, pagination, total, err := listAIForges(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIForgesListFailed(
			ctx,
			ref,
			"ai_forges_list_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIForgesListed(ctx, ref, items, pagination, total)
}

func (b *legionJobBridge) handleAIForgeCreate(ctx context.Context, raw []byte) error {
	var command aiv1.CreateAIForgeCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai forge create command: %w", err)
	}

	ref := aiForgeRefFromCreateCommand(&command)
	if err := validateAIForgeCreateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIForgeCreateFailed(
			ctx,
			ref,
			"invalid_ai_forge_create_command",
			err.Error(),
		)
	}

	item, err := createAIForge(&command)
	if err != nil {
		errorCode := "ai_forge_create_failed"
		if isAIForgeNameUniqueConflict(err) {
			errorCode = "ai_forge_conflict"
		}
		return b.ensureAIPublisher().PublishAIForgeCreateFailed(
			ctx,
			ref,
			errorCode,
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIForgeCreated(ctx, ref, item, "ok")
}

func (b *legionJobBridge) handleAIForgeUpdate(ctx context.Context, raw []byte) error {
	var command aiv1.UpdateAIForgeCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai forge update command: %w", err)
	}

	ref := aiForgeRefFromUpdateCommand(&command)
	if err := validateAIForgeUpdateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIForgeUpdateFailed(
			ctx,
			ref,
			"invalid_ai_forge_update_command",
			err.Error(),
		)
	}

	item, err := updateAIForge(&command)
	if err != nil {
		errorCode := "ai_forge_update_failed"
		switch {
		case isAIForgeNotFound(err):
			errorCode = "ai_forge_not_found"
		case isAIForgeNameUniqueConflict(err):
			errorCode = "ai_forge_conflict"
		}
		return b.ensureAIPublisher().PublishAIForgeUpdateFailed(
			ctx,
			ref,
			errorCode,
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIForgeUpdated(ctx, ref, item, "ok")
}

func (b *legionJobBridge) handleAIForgeDelete(ctx context.Context, raw []byte) error {
	var command aiv1.DeleteAIForgeCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai forge delete command: %w", err)
	}

	ref := aiForgeRefFromDeleteCommand(&command)
	if err := validateAIForgeDeleteCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIForgeDeleteFailed(
			ctx,
			ref,
			"invalid_ai_forge_delete_command",
			err.Error(),
		)
	}

	forgeID, err := deleteAIForge(&command)
	if err != nil {
		errorCode := "ai_forge_delete_failed"
		if isAIForgeNotFound(err) {
			errorCode = "ai_forge_not_found"
		}
		return b.ensureAIPublisher().PublishAIForgeDeleteFailed(
			ctx,
			ref,
			errorCode,
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIForgeDeleted(ctx, ref, forgeID, "ok")
}

func (b *legionJobBridge) handleAIForgeExport(ctx context.Context, raw []byte) error {
	var command aiv1.ExportAIForgeCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai forge export command: %w", err)
	}

	ref := aiForgeRefFromExportCommand(&command)
	if err := validateAIForgeExportCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIForgeExportFailed(ctx, ref, "invalid_ai_forge_export_command", err.Error())
	}

	fileName, content, err := exportAIForges(ctx, b.ensureAIPublisher(), ref, &command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIForgeExportFailed(ctx, ref, "ai_forge_export_failed", err.Error())
	}
	contentType := "application/octet-stream"
	if strings.HasSuffix(strings.ToLower(fileName), ".zip") {
		contentType = "application/zip"
	}
	return b.ensureAIPublisher().PublishAIForgeExported(
		ctx,
		ref,
		fileName,
		contentType,
		command.GetObjectStoreBucket(),
		command.GetObjectStoreKey(),
		int64(len(content)),
		"export completed",
	)
}

func (b *legionJobBridge) handleAIForgeImport(ctx context.Context, raw []byte) error {
	var command aiv1.ImportAIForgeCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai forge import command: %w", err)
	}

	ref := aiForgeRefFromImportCommand(&command)
	if err := validateAIForgeImportCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIForgeImportFailed(ctx, ref, "invalid_ai_forge_import_command", err.Error())
	}

	session, ok := b.agent.node.GetSessionState()
	if !ok {
		return b.ensureAIPublisher().PublishAIForgeImportFailed(
			ctx,
			ref,
			"node_session_not_ready",
			"node session is not ready",
		)
	}

	result, err := importAIForges(ctx, b.agent.httpClient, session.SessionToken, b.ensureAIPublisher(), ref, &command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIForgeImportFailed(ctx, ref, "ai_forge_import_failed", err.Error())
	}
	return b.ensureAIPublisher().PublishAIForgeImported(
		ctx,
		ref,
		result.Created,
		result.Updated,
		result.Skipped,
		result.Items,
		summarizeAIForgeImportResult(result),
	)
}

func validateAIForgesListCommand(nodeID string, command *aiv1.ListAIForgesCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai forges list metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai forges list command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai forges list target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai forges list target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai forges list owner_user_id is required")
	default:
		return nil
	}
}

func validateAIForgeCreateCommand(nodeID string, command *aiv1.CreateAIForgeCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai forge create metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai forge create command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai forge create target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai forge create target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai forge create owner_user_id is required")
	case strings.TrimSpace(command.GetForgeName()) == "":
		return fmt.Errorf("ai forge create forge_name is required")
	case strings.TrimSpace(command.GetForgeType()) == "":
		return fmt.Errorf("ai forge create forge_type is required")
	default:
		return nil
	}
}

func validateAIForgeUpdateCommand(nodeID string, command *aiv1.UpdateAIForgeCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai forge update metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai forge update command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai forge update target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai forge update target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai forge update owner_user_id is required")
	case command.GetForgeId() <= 0:
		return fmt.Errorf("ai forge update forge_id must be greater than 0")
	case strings.TrimSpace(command.GetForgeName()) == "":
		return fmt.Errorf("ai forge update forge_name is required")
	case strings.TrimSpace(command.GetForgeType()) == "":
		return fmt.Errorf("ai forge update forge_type is required")
	default:
		return nil
	}
}

func validateAIForgeDeleteCommand(nodeID string, command *aiv1.DeleteAIForgeCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai forge delete metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai forge delete command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai forge delete target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai forge delete target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai forge delete owner_user_id is required")
	case command.GetForgeId() <= 0:
		return fmt.Errorf("ai forge delete forge_id must be greater than 0")
	default:
		return nil
	}
}

func validateAIForgeExportCommand(nodeID string, command *aiv1.ExportAIForgeCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai forge export metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai forge export command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai forge export target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai forge export target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai forge export owner_user_id is required")
	case strings.TrimSpace(command.GetObjectStoreBucket()) == "":
		return fmt.Errorf("ai forge export object_store_bucket is required")
	case strings.TrimSpace(command.GetObjectStoreKey()) == "":
		return fmt.Errorf("ai forge export object_store_key is required")
	default:
		return nil
	}
}

func validateAIForgeImportCommand(nodeID string, command *aiv1.ImportAIForgeCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai forge import metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai forge import command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai forge import target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai forge import target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai forge import owner_user_id is required")
	case command.GetAttachment() == nil:
		return fmt.Errorf("ai forge import attachment is required")
	case strings.TrimSpace(command.GetAttachment().GetAttachmentId()) == "":
		return fmt.Errorf("ai forge import attachment_id is required")
	case strings.TrimSpace(command.GetAttachment().GetDownloadUrl()) == "":
		return fmt.Errorf("ai forge import download_url is required")
	default:
		return nil
	}
}

func aiForgeRefFromListCommand(command *aiv1.ListAIForgesCommand) aiForgeCommandRef {
	return aiForgeCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiForgeRefFromCreateCommand(command *aiv1.CreateAIForgeCommand) aiForgeCommandRef {
	return aiForgeCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiForgeRefFromUpdateCommand(command *aiv1.UpdateAIForgeCommand) aiForgeCommandRef {
	return aiForgeCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiForgeRefFromDeleteCommand(command *aiv1.DeleteAIForgeCommand) aiForgeCommandRef {
	return aiForgeCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiForgeRefFromExportCommand(command *aiv1.ExportAIForgeCommand) aiForgeCommandRef {
	return aiForgeCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiForgeRefFromImportCommand(command *aiv1.ImportAIForgeCommand) aiForgeCommandRef {
	return aiForgeCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func listAIForges(command *aiv1.ListAIForgesCommand) ([]*aiv1.AIForgeRecord, *aiv1.AIForgePagination, int64, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, nil, 0, utils.Errorf("database not initialized")
	}

	pagination := command.GetPagination()
	if pagination == nil {
		pagination = &aiv1.AIForgePagination{Page: 1, Limit: 20}
	}
	if pagination.GetPage() <= 0 {
		pagination.Page = 1
	}
	if pagination.GetLimit() <= 0 {
		pagination.Limit = 20
	}

	filter := &ypb.AIForgeFilter{
		ForgeName:     strings.TrimSpace(command.GetForgeName()),
		ForgeNames:    normalizeAIForgeStringSlice(command.GetForgeNames()),
		ForgeType:     strings.TrimSpace(command.GetForgeType()),
		Keyword:       strings.TrimSpace(command.GetQuery()),
		Tag:           normalizeAIForgeStringSlice(command.GetTags()),
		Id:            command.GetForgeId(),
		ShowTemporary: command.GetShowTemporary(),
	}
	requestPaging := &ypb.Paging{
		Page:    pagination.GetPage(),
		Limit:   pagination.GetLimit(),
		OrderBy: strings.TrimSpace(pagination.GetOrderBy()),
		Order:   strings.TrimSpace(pagination.GetOrder()),
	}
	pag, data, err := yakit.QueryAIForge(db, filter, requestPaging)
	if err != nil {
		return nil, nil, 0, err
	}

	items := make([]*aiv1.AIForgeRecord, 0, len(data))
	for _, forge := range data {
		if forge == nil {
			continue
		}
		items = append(items, mapSchemaAIForgeToLegion(forge))
	}
	return items, mapAIForgePagination(pag, requestPaging), int64(pag.TotalRecord), nil
}

func createAIForge(command *aiv1.CreateAIForgeCommand) (*aiv1.AIForgeRecord, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	request := buildCreateAIForgeRequest(command)
	forge := schema.GRPC2AIForge(request)
	applyAIForgeMetadata(db, forge)
	applyAIForgeRequestOverrides(request, forge)
	if err := yakit.CreateAIForge(db, forge); err != nil {
		return nil, err
	}

	fresh, err := yakit.GetAIForgeByID(db, int64(forge.ID))
	if err != nil {
		return nil, err
	}
	return mapSchemaAIForgeToLegion(fresh), nil
}

func updateAIForge(command *aiv1.UpdateAIForgeCommand) (*aiv1.AIForgeRecord, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	request := buildUpdateAIForgeRequest(command)
	forge := schema.GRPC2AIForge(request)
	applyAIForgeMetadata(db, forge)
	applyAIForgeRequestOverrides(request, forge)
	if err := yakit.UpdateAIForge(db, forge); err != nil {
		return nil, err
	}

	fresh, err := yakit.GetAIForgeByID(db, int64(forge.ID))
	if err != nil {
		return nil, err
	}
	return mapSchemaAIForgeToLegion(fresh), nil
}

func deleteAIForge(command *aiv1.DeleteAIForgeCommand) (string, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return "", utils.Errorf("database not initialized")
	}

	count, err := yakit.DeleteAIForge(db, &ypb.AIForgeFilter{Id: command.GetForgeId()})
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "", gorm.ErrRecordNotFound
	}
	return strconv.FormatInt(command.GetForgeId(), 10), nil
}

func buildCreateAIForgeRequest(command *aiv1.CreateAIForgeCommand) *ypb.AIForge {
	return &ypb.AIForge{
		ForgeName:          strings.TrimSpace(command.GetForgeName()),
		ForgeContent:       command.GetForgeContent(),
		ForgeType:          strings.TrimSpace(command.GetForgeType()),
		Description:        strings.TrimSpace(command.GetDescription()),
		ParamsUIConfig:     strings.TrimSpace(command.GetParamsUiConfig()),
		Params:             strings.TrimSpace(command.GetParams()),
		UserPersistentData: strings.TrimSpace(command.GetUserPersistentData()),
		ToolNames:          normalizeAIForgeStringSlice(command.GetToolNames()),
		ToolKeywords:       normalizeAIForgeStringSlice(command.GetToolKeywords()),
		Action:             strings.TrimSpace(command.GetAction()),
		Tag:                normalizeAIForgeStringSlice(command.GetTags()),
		InitPrompt:         strings.TrimSpace(command.GetInitPrompt()),
		PersistentPrompt:   strings.TrimSpace(command.GetPersistentPrompt()),
		PlanPrompt:         strings.TrimSpace(command.GetPlanPrompt()),
		ResultPrompt:       strings.TrimSpace(command.GetResultPrompt()),
		ForgeVerboseName:   strings.TrimSpace(command.GetForgeVerboseName()),
		IsBuiltin:          command.GetIsBuiltin(),
	}
}

func buildUpdateAIForgeRequest(command *aiv1.UpdateAIForgeCommand) *ypb.AIForge {
	return &ypb.AIForge{
		Id:                 command.GetForgeId(),
		ForgeName:          strings.TrimSpace(command.GetForgeName()),
		ForgeContent:       command.GetForgeContent(),
		ForgeType:          strings.TrimSpace(command.GetForgeType()),
		Description:        strings.TrimSpace(command.GetDescription()),
		ParamsUIConfig:     strings.TrimSpace(command.GetParamsUiConfig()),
		Params:             strings.TrimSpace(command.GetParams()),
		UserPersistentData: strings.TrimSpace(command.GetUserPersistentData()),
		ToolNames:          normalizeAIForgeStringSlice(command.GetToolNames()),
		ToolKeywords:       normalizeAIForgeStringSlice(command.GetToolKeywords()),
		Action:             strings.TrimSpace(command.GetAction()),
		Tag:                normalizeAIForgeStringSlice(command.GetTags()),
		InitPrompt:         strings.TrimSpace(command.GetInitPrompt()),
		PersistentPrompt:   strings.TrimSpace(command.GetPersistentPrompt()),
		PlanPrompt:         strings.TrimSpace(command.GetPlanPrompt()),
		ResultPrompt:       strings.TrimSpace(command.GetResultPrompt()),
		ForgeVerboseName:   strings.TrimSpace(command.GetForgeVerboseName()),
		IsBuiltin:          command.GetIsBuiltin(),
	}
}

func applyAIForgeMetadata(db *gorm.DB, forge *schema.AIForge) {
	if forge == nil {
		return
	}
	if forge.ForgeType == schema.FORGE_TYPE_Config {
		applyAIForgeDefaultsFromDB(db, forge)
	}
	if forge.ForgeType != schema.FORGE_TYPE_YAK {
		return
	}
	if forge.ForgeContent == "" {
		return
	}

	prog, err := static_analyzer.SSAParse(forge.ForgeContent, "yak")
	if err != nil {
		log.Warnf("parse forge metadata failed: %v", err)
		return
	}
	parsed, err := scriptmetadata.ParseYakScriptMetadataProg(forge.ForgeName, prog)
	if err != nil {
		log.Warnf("parse forge metadata failed: %v", err)
		return
	}
	if forge.ForgeVerboseName == "" && parsed.VerboseName != "" {
		forge.ForgeVerboseName = parsed.VerboseName
	}
	if forge.Description == "" && parsed.Description != "" {
		forge.Description = parsed.Description
	}
	if forge.Tags == "" && len(parsed.Keywords) > 0 {
		forge.Tags = strings.Join(parsed.Keywords, ",")
	}
	if forge.ToolKeywords == "" && len(parsed.Keywords) > 0 {
		forge.ToolKeywords = strings.Join(parsed.Keywords, ",")
	}
}

func applyAIForgeDefaultsFromDB(db *gorm.DB, forge *schema.AIForge) {
	if db == nil || forge == nil {
		return
	}

	var (
		existing *schema.AIForge
		err      error
	)
	if forge.ID > 0 {
		existing, err = yakit.GetAIForgeByID(db, int64(forge.ID))
	} else if forge.ForgeName != "" {
		existing, err = yakit.GetAIForgeByName(db, forge.ForgeName)
	}
	if err != nil || existing == nil {
		return
	}

	if forge.ForgeName == "" {
		forge.ForgeName = existing.ForgeName
	}
	if forge.ForgeType == "" {
		forge.ForgeType = existing.ForgeType
	}
	if forge.ForgeVerboseName == "" {
		forge.ForgeVerboseName = existing.ForgeVerboseName
	}
	if forge.ForgeContent == "" {
		forge.ForgeContent = existing.ForgeContent
	}
	if forge.ParamsUIConfig == "" {
		forge.ParamsUIConfig = existing.ParamsUIConfig
	}
	if forge.Params == "" {
		forge.Params = existing.Params
	}
	if forge.UserPersistentData == "" {
		forge.UserPersistentData = existing.UserPersistentData
	}
	if forge.Description == "" {
		forge.Description = existing.Description
	}
	if forge.Tools == "" {
		forge.Tools = existing.Tools
	}
	if forge.ToolKeywords == "" {
		forge.ToolKeywords = existing.ToolKeywords
	}
	if forge.Actions == "" {
		forge.Actions = existing.Actions
	}
	if forge.Tags == "" {
		forge.Tags = existing.Tags
	}
	if forge.InitPrompt == "" {
		forge.InitPrompt = existing.InitPrompt
	}
	if forge.PersistentPrompt == "" {
		forge.PersistentPrompt = existing.PersistentPrompt
	}
	if forge.PlanPrompt == "" {
		forge.PlanPrompt = existing.PlanPrompt
	}
	if forge.ResultPrompt == "" {
		forge.ResultPrompt = existing.ResultPrompt
	}
	if forge.SkillPath == "" {
		forge.SkillPath = existing.SkillPath
	}
	if len(forge.FSBytes) == 0 {
		forge.FSBytes = append([]byte(nil), existing.FSBytes...)
	}
}

func applyAIForgeRequestOverrides(request *ypb.AIForge, forge *schema.AIForge) {
	if request == nil || forge == nil {
		return
	}

	forge.ID = uint(request.GetId())
	forge.ForgeName = request.GetForgeName()
	forge.ForgeVerboseName = request.GetForgeVerboseName()
	forge.ForgeContent = request.GetForgeContent()
	forge.ForgeType = request.GetForgeType()
	forge.ParamsUIConfig = request.GetParamsUIConfig()
	forge.Params = request.GetParams()
	forge.UserPersistentData = request.GetUserPersistentData()
	forge.Description = request.GetDescription()
	forge.Tools = strings.Join(request.GetToolNames(), ",")
	forge.ToolKeywords = strings.Join(request.GetToolKeywords(), ",")
	forge.Actions = request.GetAction()
	forge.Tags = strings.Join(request.GetTag(), ",")
	forge.InitPrompt = request.GetInitPrompt()
	forge.PersistentPrompt = request.GetPersistentPrompt()
	forge.PlanPrompt = request.GetPlanPrompt()
	forge.ResultPrompt = request.GetResultPrompt()
	forge.SkillPath = request.GetSkillPath()
}

func mapSchemaAIForgeToLegion(forge *schema.AIForge) *aiv1.AIForgeRecord {
	if forge == nil {
		return nil
	}
	return &aiv1.AIForgeRecord{
		Id:                 int64(forge.ID),
		ForgeName:          strings.TrimSpace(forge.ForgeName),
		ForgeContent:       forge.ForgeContent,
		ForgeType:          strings.TrimSpace(forge.ForgeType),
		Description:        strings.TrimSpace(forge.Description),
		ParamsUiConfig:     strings.TrimSpace(forge.ParamsUIConfig),
		Params:             strings.TrimSpace(forge.Params),
		UserPersistentData: strings.TrimSpace(forge.UserPersistentData),
		ToolNames:          utils.StringSplitAndStrip(forge.Tools, ","),
		ToolKeywords:       utils.StringSplitAndStrip(forge.ToolKeywords, ","),
		Action:             strings.TrimSpace(forge.Actions),
		Tags:               utils.StringSplitAndStrip(forge.Tags, ","),
		InitPrompt:         strings.TrimSpace(forge.InitPrompt),
		PersistentPrompt:   strings.TrimSpace(forge.PersistentPrompt),
		PlanPrompt:         strings.TrimSpace(forge.PlanPrompt),
		ResultPrompt:       strings.TrimSpace(forge.ResultPrompt),
		ForgeVerboseName:   strings.TrimSpace(forge.ForgeVerboseName),
		Author:             strings.TrimSpace(forge.Author),
		SkillPath:          strings.TrimSpace(forge.SkillPath),
		CreatedAt:          forge.CreatedAt.Unix(),
		UpdatedAt:          forge.UpdatedAt.Unix(),
		IsBuiltin:          forge.IsBuiltin,
	}
}

func mapAIForgePagination(pag *bizhelper.Paginator, request *ypb.Paging) *aiv1.AIForgePagination {
	if pag == nil {
		return &aiv1.AIForgePagination{
			Page:    request.GetPage(),
			Limit:   request.GetLimit(),
			OrderBy: request.GetOrderBy(),
			Order:   request.GetOrder(),
		}
	}
	return &aiv1.AIForgePagination{
		Page:    int64(pag.Page),
		Limit:   int64(pag.Limit),
		OrderBy: request.GetOrderBy(),
		Order:   request.GetOrder(),
	}
}

func normalizeAIForgeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		items = append(items, value)
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func isAIForgeNameUniqueConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed: ai_forges.forge_name") ||
		strings.Contains(message, "duplicate key value violates unique constraint") ||
		strings.Contains(message, "duplicate entry")
}

func isAIForgeNotFound(err error) bool {
	if err == nil {
		return false
	}
	return gorm.IsRecordNotFoundError(err) || strings.Contains(strings.ToLower(err.Error()), "record not found")
}

func exportAIForges(
	ctx context.Context,
	publisher *aiSessionEventPublisher,
	ref aiForgeCommandRef,
	command *aiv1.ExportAIForgeCommand,
) (string, []byte, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return "", nil, utils.Errorf("database not initialized")
	}
	tmpDir, err := os.MkdirTemp("", "legion-ai-forge-export-*")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(tmpDir)

	outputName := strings.TrimSpace(command.GetOutputName())
	if outputName == "" {
		outputName = "aiforge-package"
	}
	targetPath := filepath.Join(tmpDir, outputName)
	progress := func(percent float64, message string, messageType string) {
		_ = publisher.PublishAIForgeExportProgressed(ctx, ref, percent, message, messageType)
	}
	filter := &ypb.AIForgeFilter{
		ForgeName:  strings.TrimSpace(command.GetForgeName()),
		ForgeNames: normalizeAIForgeStringSlice(command.GetForgeNames()),
		ForgeType:  strings.TrimSpace(command.GetForgeType()),
		Tag:        normalizeAIForgeStringSlice(command.GetTags()),
	}
	if command.GetForgeId() > 0 {
		filter.Id = command.GetForgeId()
	}
	exportedPath, err := forgepkg.ExportAIForgesToZip(
		ctx,
		db,
		filter,
		normalizeAIForgeStringSlice(command.GetToolNames()),
		targetPath,
		forgepkg.WithExportProgress(progress),
		forgepkg.WithExportPassword(strings.TrimSpace(command.GetPassword())),
		forgepkg.WithExportOutputName(outputName),
	)
	if err != nil {
		return "", nil, err
	}
	content, err := os.ReadFile(exportedPath)
	if err != nil {
		return "", nil, err
	}
	if err := publisher.putObjectBytes(ctx, strings.TrimSpace(command.GetObjectStoreBucket()), strings.TrimSpace(command.GetObjectStoreKey()), content); err != nil {
		return "", nil, err
	}
	return filepath.Base(exportedPath), content, nil
}

func importAIForges(
	ctx context.Context,
	httpClient *http.Client,
	sessionToken string,
	publisher *aiSessionEventPublisher,
	ref aiForgeCommandRef,
	command *aiv1.ImportAIForgeCommand,
) (*aiv1.AIForgeImported, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}
	attachment := command.GetAttachment()
	data, err := downloadAIForgeImportAttachment(ctx, httpClient, sessionToken, attachment)
	if err != nil {
		return nil, err
	}
	tmpDir, err := os.MkdirTemp("", "legion-ai-forge-import-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	archiveName := strings.TrimSpace(command.GetArchiveName())
	if archiveName == "" {
		archiveName = strings.TrimSpace(attachment.GetFilename())
	}
	if archiveName == "" {
		archiveName = "aiforge-import.zip"
	}
	archivePath := filepath.Join(tmpDir, archiveName)
	if err := os.WriteFile(archivePath, data, 0o600); err != nil {
		return nil, err
	}

	archiveInfo, err := forgepkg.LoadAIForgesFromZip(
		archivePath,
		forgepkg.WithImportNewName(strings.TrimSpace(command.GetNewForgeName())),
		forgepkg.WithImportPassword(strings.TrimSpace(command.GetPassword())),
	)
	if err != nil {
		return nil, err
	}

	existingByName := make(map[string]bool, len(archiveInfo.AIForges))
	for _, forge := range archiveInfo.AIForges {
		if forge == nil {
			continue
		}
		forgeName := strings.TrimSpace(forge.ForgeName)
		if forgeName == "" {
			continue
		}
		_, lookupErr := yakit.GetAIForgeByName(db, forgeName)
		existingByName[forgeName] = lookupErr == nil
	}

	progress := func(percent float64, message string) {
		_ = publisher.PublishAIForgeImportProgressed(ctx, ref, percent, message, "")
	}
	importedForges, err := forgepkg.ImportAIForgesFromZip(
		db,
		archivePath,
		forgepkg.WithImportProgress(progress),
		forgepkg.WithImportOverwrite(command.GetOverwrite()),
		forgepkg.WithImportNewName(strings.TrimSpace(command.GetNewForgeName())),
		forgepkg.WithImportPassword(strings.TrimSpace(command.GetPassword())),
	)
	if err != nil {
		return nil, err
	}

	result := &aiv1.AIForgeImported{
		Items: make([]*aiv1.AIForgeImportItem, 0, len(importedForges)),
	}
	for _, forge := range importedForges {
		if forge == nil {
			continue
		}
		forgeName := strings.TrimSpace(forge.ForgeName)
		status := "created"
		if existingByName[forgeName] {
			status = "updated"
			result.Updated++
		} else {
			result.Created++
		}

		item := &aiv1.AIForgeImportItem{
			ForgeName: forgeName,
			Status:    status,
		}
		if fresh, lookupErr := yakit.GetAIForgeByName(db, forgeName); lookupErr == nil && fresh != nil {
			item.ForgeId = strconv.FormatInt(int64(fresh.ID), 10)
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func downloadAIForgeImportAttachment(
	ctx context.Context,
	httpClient *http.Client,
	sessionToken string,
	attachment *aiv1.AIForgeImportAttachment,
) ([]byte, error) {
	if strings.TrimSpace(sessionToken) == "" {
		return nil, fmt.Errorf("node session token is not ready")
	}
	client := httpClient
	if client == nil {
		client = &http.Client{}
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		strings.TrimSpace(attachment.GetDownloadUrl()),
		nil,
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(sessionToken))
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return nil, utils.Errorf("download attachment failed: status=%d", response.StatusCode)
	}
	return io.ReadAll(response.Body)
}

func summarizeAIForgeImportResult(result *aiv1.AIForgeImported) string {
	if result == nil {
		return "import completed"
	}
	return fmt.Sprintf(
		"import completed: created=%d updated=%d skipped=%d",
		result.GetCreated(),
		result.GetUpdated(),
		result.GetSkipped(),
	)
}
