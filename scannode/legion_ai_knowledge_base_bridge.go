package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jinzhu/gorm"
	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func (b *legionJobBridge) handleAIKnowledgeBasesList(ctx context.Context, raw []byte) error {
	var command aiv1.ListAIKnowledgeBasesCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge bases list command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromListCommand(&command)
	if err := validateAIKnowledgeBasesListCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBasesListFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_bases_list_command",
			err.Error(),
		)
	}

	items, pagination, total, err := listAIKnowledgeBases(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBasesListFailed(
			ctx,
			ref,
			"ai_knowledge_bases_list_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIKnowledgeBasesListed(ctx, ref, items, pagination, total)
}

func (b *legionJobBridge) handleAIKnowledgeBaseCreate(ctx context.Context, raw []byte) error {
	var command aiv1.CreateAIKnowledgeBaseCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base create command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromCreateCommand(&command)
	if err := validateAIKnowledgeBaseCreateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseCreateFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_create_command",
			err.Error(),
		)
	}

	record, err := createAIKnowledgeBase(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseCreateFailed(
			ctx,
			ref,
			"ai_knowledge_base_create_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIKnowledgeBaseCreated(ctx, ref, record, "ok")
}

func (b *legionJobBridge) handleAIKnowledgeBaseUpdate(ctx context.Context, raw []byte) error {
	var command aiv1.UpdateAIKnowledgeBaseCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base update command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromUpdateCommand(&command)
	if err := validateAIKnowledgeBaseUpdateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseUpdateFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_update_command",
			err.Error(),
		)
	}

	record, err := updateAIKnowledgeBase(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseUpdateFailed(
			ctx,
			ref,
			"ai_knowledge_base_update_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIKnowledgeBaseUpdated(ctx, ref, record, "ok")
}

func (b *legionJobBridge) handleAIKnowledgeBaseImport(ctx context.Context, raw []byte) error {
	var command aiv1.ImportAIKnowledgeBaseCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base import command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromImportCommand(&command)
	if err := validateAIKnowledgeBaseImportCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseImportFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_import_command",
			err.Error(),
		)
	}

	session, ok := b.agent.node.GetSessionState()
	if !ok {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseImportFailed(
			ctx,
			ref,
			"node_session_not_ready",
			"node session is not ready",
		)
	}

	record, err := importAIKnowledgeBase(ctx, b.agent.httpClient, session.SessionToken, &command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseImportFailed(
			ctx,
			ref,
			"ai_knowledge_base_import_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIKnowledgeBaseImported(ctx, ref, record, "ok")
}

func (b *legionJobBridge) handleAIKnowledgeBaseDelete(ctx context.Context, raw []byte) error {
	var command aiv1.DeleteAIKnowledgeBaseCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base delete command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromDeleteCommand(&command)
	if err := validateAIKnowledgeBaseDeleteCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseDeleteFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_delete_command",
			err.Error(),
		)
	}

	record, err := deleteAIKnowledgeBase(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseDeleteFailed(
			ctx,
			ref,
			"ai_knowledge_base_delete_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIKnowledgeBaseDeleted(ctx, ref, record, "ok")
}

func (b *legionJobBridge) handleAIKnowledgeBaseExport(ctx context.Context, raw []byte) error {
	var command aiv1.ExportAIKnowledgeBaseCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base export command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromExportCommand(&command)
	if err := validateAIKnowledgeBaseExportCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseExportFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_export_command",
			err.Error(),
		)
	}

	record, fileName, contentType, content, err := exportAIKnowledgeBase(ctx, &command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseExportFailed(
			ctx,
			ref,
			"ai_knowledge_base_export_failed",
			err.Error(),
		)
	}

	publisher := b.ensureAIPublisher()
	if err := publisher.putObjectBytes(ctx, strings.TrimSpace(command.GetObjectStoreBucket()), strings.TrimSpace(command.GetObjectStoreKey()), content); err != nil {
		return publisher.PublishAIKnowledgeBaseExportFailed(
			ctx,
			ref,
			"ai_knowledge_base_export_store_failed",
			err.Error(),
		)
	}

	return publisher.PublishAIKnowledgeBaseExported(
		ctx,
		ref,
		record,
		"ok",
		fileName,
		contentType,
		command.GetObjectStoreBucket(),
		command.GetObjectStoreKey(),
		int64(len(content)),
	)
}

func (b *legionJobBridge) handleAIKnowledgeBaseEntriesSearch(ctx context.Context, raw []byte) error {
	var command aiv1.SearchAIKnowledgeBaseEntriesCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base entries search command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromEntriesSearchCommand(&command)
	if err := validateAIKnowledgeBaseEntriesSearchCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseEntriesSearchFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_entries_search_command",
			err.Error(),
		)
	}

	items, pagination, total, err := searchAIKnowledgeBaseEntries(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseEntriesSearchFailed(
			ctx,
			ref,
			"ai_knowledge_base_entries_search_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIKnowledgeBaseEntriesSearched(ctx, ref, items, pagination, total)
}

func (b *legionJobBridge) handleAIKnowledgeBaseQueryByAI(ctx context.Context, raw []byte) error {
	var command aiv1.QueryAIKnowledgeBaseByAICommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base query command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromQueryByAICommand(&command)
	if err := validateAIKnowledgeBaseQueryByAICommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseQueryByAIFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_query_command",
			err.Error(),
		)
	}

	queryCommandID := strings.TrimSpace(command.GetMetadata().GetCommandId())
	queryCtx, cancel := context.WithCancel(ctx)
	b.aiKnowledgeBaseQueries.Store(queryCommandID, cancel)
	go func() {
		defer cancel()
		b.runAIKnowledgeBaseQueryByAI(queryCtx, ref, queryCommandID, &command)
	}()
	return nil
}

func (b *legionJobBridge) handleAIKnowledgeBaseQueryByAICancel(_ context.Context, raw []byte) error {
	var command aiv1.CancelAIKnowledgeBaseByAIQueryCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base query cancel command: %w", err)
	}
	if err := validateAIKnowledgeBaseQueryByAICancelCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return nil
	}
	_ = b.aiKnowledgeBaseQueries.Cancel(strings.TrimSpace(command.GetQueryCommandId()))
	return nil
}

func (b *legionJobBridge) runAIKnowledgeBaseQueryByAI(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	queryCommandID string,
	command *aiv1.QueryAIKnowledgeBaseByAICommand,
) {
	defer b.aiKnowledgeBaseQueries.Remove(queryCommandID)

	results, err := queryAIKnowledgeBaseByAI(ctx, command)
	if err != nil {
		if ctx.Err() == nil {
			_ = b.ensureAIPublisher().PublishAIKnowledgeBaseQueryByAIFailed(
				context.Background(),
				ref,
				"ai_knowledge_base_query_failed",
				err.Error(),
			)
		}
		return
	}

	for result := range results {
		if ctx.Err() != nil {
			return
		}
		data, err := marshalAIKnowledgeBaseQueryData(result.Data)
		if err != nil {
			_ = b.ensureAIPublisher().PublishAIKnowledgeBaseQueryByAIFailed(
				context.Background(),
				ref,
				"ai_knowledge_base_query_marshal_failed",
				err.Error(),
			)
			return
		}
		if err := b.ensureAIPublisher().PublishAIKnowledgeBaseQueryByAIChunk(
			context.Background(),
			ref,
			utils.EscapeInvalidUTF8Byte([]byte(result.Message)),
			strings.TrimSpace(result.Type),
			data,
		); err != nil {
			if ctx.Err() != nil {
				return
			}
			_ = b.ensureAIPublisher().PublishAIKnowledgeBaseQueryByAIFailed(
				context.Background(),
				ref,
				"ai_knowledge_base_query_publish_failed",
				err.Error(),
			)
			return
		}
	}

	if ctx.Err() != nil {
		return
	}
	_ = b.ensureAIPublisher().PublishAIKnowledgeBaseQueryByAICompleted(context.Background(), ref, "ok")
}

func (b *legionJobBridge) handleAIKnowledgeBaseQuestionIndexGenerate(ctx context.Context, raw []byte) error {
	var command aiv1.GenerateAIKnowledgeBaseQuestionIndexCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base question index command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromQuestionIndexCommand(&command)
	if err := validateAIKnowledgeBaseQuestionIndexCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseQuestionIndexFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_question_index_command",
			err.Error(),
		)
	}

	requestCommandID := strings.TrimSpace(command.GetMetadata().GetCommandId())
	jobCtx, cancel := context.WithCancel(ctx)
	b.aiKnowledgeBaseQuestionIndexes.Store(requestCommandID, cancel)
	go func() {
		defer cancel()
		b.runAIKnowledgeBaseQuestionIndex(jobCtx, ref, requestCommandID, &command)
	}()
	return nil
}

func (b *legionJobBridge) handleAIKnowledgeBaseQuestionIndexCancel(_ context.Context, raw []byte) error {
	var command aiv1.CancelAIKnowledgeBaseQuestionIndexCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base question index cancel command: %w", err)
	}
	if err := validateAIKnowledgeBaseQuestionIndexCancelCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return nil
	}
	_ = b.aiKnowledgeBaseQuestionIndexes.Cancel(strings.TrimSpace(command.GetRequestCommandId()))
	return nil
}

func (b *legionJobBridge) runAIKnowledgeBaseQuestionIndex(
	ctx context.Context,
	ref aiKnowledgeBaseCommandRef,
	requestCommandID string,
	command *aiv1.GenerateAIKnowledgeBaseQuestionIndexCommand,
) {
	defer b.aiKnowledgeBaseQuestionIndexes.Remove(requestCommandID)

	err := generateAIKnowledgeBaseQuestionIndex(ctx, command, func(percent float64, message string, messageType string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return b.ensureAIPublisher().PublishAIKnowledgeBaseQuestionIndexProgress(
			context.Background(),
			ref,
			percent,
			utils.EscapeInvalidUTF8Byte([]byte(message)),
			messageType,
		)
	})
	if err != nil {
		if ctx.Err() == nil {
			_ = b.ensureAIPublisher().PublishAIKnowledgeBaseQuestionIndexFailed(
				context.Background(),
				ref,
				"ai_knowledge_base_question_index_failed",
				err.Error(),
			)
		}
		return
	}
	if ctx.Err() != nil {
		return
	}
	_ = b.ensureAIPublisher().PublishAIKnowledgeBaseQuestionIndexCompleted(context.Background(), ref, "ok")
}

func (b *legionJobBridge) handleAIKnowledgeBaseEntryCreate(ctx context.Context, raw []byte) error {
	var command aiv1.CreateAIKnowledgeBaseEntryCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base entry create command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromEntryCreateCommand(&command)
	if err := validateAIKnowledgeBaseEntryCreateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseEntryCreateFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_entry_create_command",
			err.Error(),
		)
	}

	record, err := createAIKnowledgeBaseEntry(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseEntryCreateFailed(
			ctx,
			ref,
			"ai_knowledge_base_entry_create_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIKnowledgeBaseEntryCreated(ctx, ref, record, "ok")
}

func (b *legionJobBridge) handleAIKnowledgeBaseEntryUpdate(ctx context.Context, raw []byte) error {
	var command aiv1.UpdateAIKnowledgeBaseEntryCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base entry update command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromEntryUpdateCommand(&command)
	if err := validateAIKnowledgeBaseEntryUpdateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseEntryUpdateFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_entry_update_command",
			err.Error(),
		)
	}

	record, err := updateAIKnowledgeBaseEntry(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseEntryUpdateFailed(
			ctx,
			ref,
			"ai_knowledge_base_entry_update_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIKnowledgeBaseEntryUpdated(ctx, ref, record, "ok")
}

func (b *legionJobBridge) handleAIKnowledgeBaseEntryDelete(ctx context.Context, raw []byte) error {
	var command aiv1.DeleteAIKnowledgeBaseEntryCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base entry delete command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromEntryDeleteCommand(&command)
	if err := validateAIKnowledgeBaseEntryDeleteCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseEntryDeleteFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_entry_delete_command",
			err.Error(),
		)
	}

	if err := deleteAIKnowledgeBaseEntry(&command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseEntryDeleteFailed(
			ctx,
			ref,
			"ai_knowledge_base_entry_delete_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIKnowledgeBaseEntryDeleted(
		ctx,
		ref,
		command.GetKnowledgeBaseId(),
		command.GetKnowledgeBaseEntryId(),
		command.GetKnowledgeBaseEntryHiddenIndex(),
		"ok",
	)
}

func (b *legionJobBridge) handleAIKnowledgeBaseVectorIndexBuild(ctx context.Context, raw []byte) error {
	var command aiv1.BuildAIKnowledgeBaseVectorIndexCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base vector index build command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromVectorIndexBuildCommand(&command)
	if err := validateAIKnowledgeBaseVectorIndexBuildCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseVectorIndexBuildFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_vector_index_build_command",
			err.Error(),
		)
	}

	if err := buildAIKnowledgeBaseVectorIndex(&command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseVectorIndexBuildFailed(
			ctx,
			ref,
			"ai_knowledge_base_vector_index_build_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIKnowledgeBaseVectorIndexBuilt(ctx, ref, command.GetKnowledgeBaseId(), "ok")
}

func (b *legionJobBridge) handleAIKnowledgeBaseEntryVectorIndexBuild(ctx context.Context, raw []byte) error {
	var command aiv1.BuildAIKnowledgeBaseEntryVectorIndexCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai knowledge base entry vector index build command: %w", err)
	}

	ref := aiKnowledgeBaseRefFromEntryVectorIndexBuildCommand(&command)
	if err := validateAIKnowledgeBaseEntryVectorIndexBuildCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseEntryVectorIndexBuildFailed(
			ctx,
			ref,
			"invalid_ai_knowledge_base_entry_vector_index_build_command",
			err.Error(),
		)
	}

	if err := buildAIKnowledgeBaseEntryVectorIndex(&command); err != nil {
		return b.ensureAIPublisher().PublishAIKnowledgeBaseEntryVectorIndexBuildFailed(
			ctx,
			ref,
			"ai_knowledge_base_entry_vector_index_build_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIKnowledgeBaseEntryVectorIndexBuilt(
		ctx,
		ref,
		command.GetKnowledgeBaseId(),
		command.GetKnowledgeBaseEntryId(),
		command.GetKnowledgeBaseEntryHiddenIndex(),
		"ok",
	)
}

func validateAIKnowledgeBasesListCommand(nodeID string, command *aiv1.ListAIKnowledgeBasesCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge bases list metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge bases list command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge bases list target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge bases list target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge bases list owner_user_id is required")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseCreateCommand(nodeID string, command *aiv1.CreateAIKnowledgeBaseCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base create metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base create command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base create target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base create target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base create owner_user_id is required")
	case strings.TrimSpace(command.GetKnowledgeBaseName()) == "":
		return fmt.Errorf("ai knowledge base create knowledge_base_name is required")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseUpdateCommand(nodeID string, command *aiv1.UpdateAIKnowledgeBaseCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base update metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base update command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base update target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base update target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base update owner_user_id is required")
	case command.GetKnowledgeBaseId() <= 0:
		return fmt.Errorf("ai knowledge base update knowledge_base_id must be greater than 0")
	case strings.TrimSpace(command.GetKnowledgeBaseName()) == "":
		return fmt.Errorf("ai knowledge base update knowledge_base_name is required")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseImportCommand(nodeID string, command *aiv1.ImportAIKnowledgeBaseCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base import metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base import command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base import target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base import target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base import owner_user_id is required")
	case strings.TrimSpace(command.GetKnowledgeBaseName()) == "":
		return fmt.Errorf("ai knowledge base import knowledge_base_name is required")
	case command.GetAttachment() == nil:
		return fmt.Errorf("ai knowledge base import attachment is required")
	case strings.TrimSpace(command.GetAttachment().GetAttachmentId()) == "":
		return fmt.Errorf("ai knowledge base import attachment_id is required")
	case strings.TrimSpace(command.GetAttachment().GetDownloadUrl()) == "":
		return fmt.Errorf("ai knowledge base import download_url is required")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseDeleteCommand(nodeID string, command *aiv1.DeleteAIKnowledgeBaseCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base delete metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base delete command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base delete target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base delete target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base delete owner_user_id is required")
	case command.GetKnowledgeBaseId() <= 0:
		return fmt.Errorf("ai knowledge base delete knowledge_base_id must be greater than 0")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseExportCommand(nodeID string, command *aiv1.ExportAIKnowledgeBaseCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base export metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base export command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base export target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base export target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base export owner_user_id is required")
	case command.GetKnowledgeBaseId() <= 0:
		return fmt.Errorf("ai knowledge base export knowledge_base_id must be greater than 0")
	case strings.TrimSpace(command.GetObjectStoreBucket()) == "":
		return fmt.Errorf("ai knowledge base export object_store_bucket is required")
	case strings.TrimSpace(command.GetObjectStoreKey()) == "":
		return fmt.Errorf("ai knowledge base export object_store_key is required")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseEntriesSearchCommand(nodeID string, command *aiv1.SearchAIKnowledgeBaseEntriesCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base entries search metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base entries search command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base entries search target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base entries search target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base entries search owner_user_id is required")
	case command.GetFilter() == nil:
		return fmt.Errorf("ai knowledge base entries search filter is required")
	case command.GetFilter().GetKnowledgeBaseId() <= 0:
		return fmt.Errorf("ai knowledge base entries search knowledge_base_id must be greater than 0")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseQueryByAICommand(nodeID string, command *aiv1.QueryAIKnowledgeBaseByAICommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base query metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base query command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base query target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base query target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base query owner_user_id is required")
	case strings.TrimSpace(command.GetQuery()) == "":
		return fmt.Errorf("ai knowledge base query query is required")
	case !command.GetQueryAllCollections() && command.GetKnowledgeBaseId() <= 0:
		return fmt.Errorf("ai knowledge base query knowledge_base_id must be greater than 0")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseQueryByAICancelCommand(nodeID string, command *aiv1.CancelAIKnowledgeBaseByAIQueryCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base query cancel metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base query cancel command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base query cancel target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base query cancel target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base query cancel owner_user_id is required")
	case strings.TrimSpace(command.GetQueryCommandId()) == "":
		return fmt.Errorf("ai knowledge base query cancel query_command_id is required")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseQuestionIndexCommand(nodeID string, command *aiv1.GenerateAIKnowledgeBaseQuestionIndexCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base question index metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base question index command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base question index target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base question index target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base question index owner_user_id is required")
	case command.GetKnowledgeBaseId() <= 0 && strings.TrimSpace(command.GetKnowledgeBaseName()) == "":
		return fmt.Errorf("ai knowledge base question index knowledge_base_id or knowledge_base_name is required")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseQuestionIndexCancelCommand(nodeID string, command *aiv1.CancelAIKnowledgeBaseQuestionIndexCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base question index cancel metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base question index cancel command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base question index cancel target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base question index cancel target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base question index cancel owner_user_id is required")
	case strings.TrimSpace(command.GetRequestCommandId()) == "":
		return fmt.Errorf("ai knowledge base question index cancel request_command_id is required")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseEntryCreateCommand(nodeID string, command *aiv1.CreateAIKnowledgeBaseEntryCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base entry create metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base entry create command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base entry create target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base entry create target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base entry create owner_user_id is required")
	case command.GetKnowledgeBaseId() <= 0:
		return fmt.Errorf("ai knowledge base entry create knowledge_base_id must be greater than 0")
	case strings.TrimSpace(command.GetKnowledgeTitle()) == "":
		return fmt.Errorf("ai knowledge base entry create knowledge_title is required")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseEntryUpdateCommand(nodeID string, command *aiv1.UpdateAIKnowledgeBaseEntryCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base entry update metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base entry update command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base entry update target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base entry update target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base entry update owner_user_id is required")
	case command.GetKnowledgeBaseId() <= 0:
		return fmt.Errorf("ai knowledge base entry update knowledge_base_id must be greater than 0")
	case command.GetKnowledgeBaseEntryId() <= 0:
		return fmt.Errorf("ai knowledge base entry update knowledge_base_entry_id must be greater than 0")
	case strings.TrimSpace(command.GetKnowledgeBaseEntryHiddenIndex()) == "":
		return fmt.Errorf("ai knowledge base entry update knowledge_base_entry_hidden_index is required")
	case strings.TrimSpace(command.GetKnowledgeTitle()) == "":
		return fmt.Errorf("ai knowledge base entry update knowledge_title is required")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseEntryDeleteCommand(nodeID string, command *aiv1.DeleteAIKnowledgeBaseEntryCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base entry delete metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base entry delete command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base entry delete target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base entry delete target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base entry delete owner_user_id is required")
	case command.GetKnowledgeBaseId() <= 0:
		return fmt.Errorf("ai knowledge base entry delete knowledge_base_id must be greater than 0")
	case command.GetKnowledgeBaseEntryId() <= 0:
		return fmt.Errorf("ai knowledge base entry delete knowledge_base_entry_id must be greater than 0")
	case strings.TrimSpace(command.GetKnowledgeBaseEntryHiddenIndex()) == "":
		return fmt.Errorf("ai knowledge base entry delete knowledge_base_entry_hidden_index is required")
	default:
		return nil
	}
}

func validateAIKnowledgeBaseVectorIndexBuildCommand(nodeID string, command *aiv1.BuildAIKnowledgeBaseVectorIndexCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base vector index build metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base vector index build command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base vector index build target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base vector index build target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base vector index build owner_user_id is required")
	case command.GetKnowledgeBaseId() <= 0:
		return fmt.Errorf("ai knowledge base vector index build knowledge_base_id must be greater than 0")
	case strings.TrimSpace(command.GetDistanceFuncType()) == "":
		return fmt.Errorf("ai knowledge base vector index build distance_func_type is required")
	case strings.TrimSpace(command.GetDistanceFuncType()) != "cosine":
		return fmt.Errorf("ai knowledge base vector index build distance_func_type is invalid: %s", command.GetDistanceFuncType())
	default:
		return nil
	}
}

func validateAIKnowledgeBaseEntryVectorIndexBuildCommand(nodeID string, command *aiv1.BuildAIKnowledgeBaseEntryVectorIndexCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai knowledge base entry vector index build metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai knowledge base entry vector index build command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai knowledge base entry vector index build target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai knowledge base entry vector index build target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai knowledge base entry vector index build owner_user_id is required")
	case command.GetKnowledgeBaseId() <= 0:
		return fmt.Errorf("ai knowledge base entry vector index build knowledge_base_id must be greater than 0")
	case command.GetKnowledgeBaseEntryId() <= 0:
		return fmt.Errorf("ai knowledge base entry vector index build knowledge_base_entry_id must be greater than 0")
	case strings.TrimSpace(command.GetKnowledgeBaseEntryHiddenIndex()) == "":
		return fmt.Errorf("ai knowledge base entry vector index build knowledge_base_entry_hidden_index is required")
	case strings.TrimSpace(command.GetDistanceFuncType()) == "":
		return fmt.Errorf("ai knowledge base entry vector index build distance_func_type is required")
	case strings.TrimSpace(command.GetDistanceFuncType()) != "cosine":
		return fmt.Errorf("ai knowledge base entry vector index build distance_func_type is invalid: %s", command.GetDistanceFuncType())
	default:
		return nil
	}
}

func aiKnowledgeBaseRefFromListCommand(command *aiv1.ListAIKnowledgeBasesCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromCreateCommand(command *aiv1.CreateAIKnowledgeBaseCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromImportCommand(command *aiv1.ImportAIKnowledgeBaseCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromUpdateCommand(command *aiv1.UpdateAIKnowledgeBaseCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromDeleteCommand(command *aiv1.DeleteAIKnowledgeBaseCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromExportCommand(command *aiv1.ExportAIKnowledgeBaseCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromEntriesSearchCommand(command *aiv1.SearchAIKnowledgeBaseEntriesCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromQueryByAICommand(command *aiv1.QueryAIKnowledgeBaseByAICommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromQuestionIndexCommand(command *aiv1.GenerateAIKnowledgeBaseQuestionIndexCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromEntryCreateCommand(command *aiv1.CreateAIKnowledgeBaseEntryCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromEntryUpdateCommand(command *aiv1.UpdateAIKnowledgeBaseEntryCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromEntryDeleteCommand(command *aiv1.DeleteAIKnowledgeBaseEntryCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromVectorIndexBuildCommand(command *aiv1.BuildAIKnowledgeBaseVectorIndexCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiKnowledgeBaseRefFromEntryVectorIndexBuildCommand(command *aiv1.BuildAIKnowledgeBaseEntryVectorIndexCommand) aiKnowledgeBaseCommandRef {
	return aiKnowledgeBaseCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func queryAIKnowledgeBaseByAI(ctx context.Context, command *aiv1.QueryAIKnowledgeBaseByAICommand) (chan *knowledgebase.SearchKnowledgebaseResult, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	query := strings.TrimSpace(command.GetQuery())
	opts := []knowledgebase.QueryOption{
		knowledgebase.WithCtx(ctx),
		knowledgebase.WithEnhancePlan(strings.TrimSpace(command.GetEnhancePlan())),
		knowledgebase.WithEnableAISummary(true),
		knowledgebase.WithAIService(strings.TrimSpace(command.GetAiService())),
	}

	if command.GetQueryAllCollections() {
		return knowledgebase.Query(db, query, opts...)
	}

	kb, err := knowledgebase.LoadKnowledgeBaseByID(db, command.GetKnowledgeBaseId())
	if err != nil {
		return nil, err
	}
	return kb.SearchKnowledgeEntriesWithEnhance(query, opts...)
}

func marshalAIKnowledgeBaseQueryData(data any) (string, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return utils.EscapeInvalidUTF8Byte(raw), nil
}

func generateAIKnowledgeBaseQuestionIndex(
	ctx context.Context,
	command *aiv1.GenerateAIKnowledgeBaseQuestionIndexCommand,
	progress func(percent float64, message string, messageType string) error,
) error {
	id := command.GetKnowledgeBaseId()
	knowledgeBaseName := strings.TrimSpace(command.GetKnowledgeBaseName())
	hiddenIndex := strings.TrimSpace(command.GetHiddenIndex())
	if knowledgeBaseName == "" && id != 0 {
		kb, err := knowledgebase.LoadKnowledgeBaseByID(consts.GetGormProfileDatabase(), id)
		if err != nil {
			return utils.Errorf("加载知识库失败: %v", err)
		}
		knowledgeBaseName = kb.GetKnowledgeBaseInfo().KnowledgeBaseName
	}
	if knowledgeBaseName == "" && id == 0 {
		return utils.Errorf("知识库名称或ID不能为空")
	}

	ragSystem, err := rag.Get(knowledgeBaseName)
	if err != nil {
		return utils.Errorf("加载知识库失败: %v", err)
	}
	if hiddenIndex != "" {
		err = ragSystem.GenerateQuestionIndexForKnowledge(hiddenIndex, rag.WithRAGCtx(ctx))
		if err != nil {
			return utils.Errorf("生成问题索引失败: %v", err)
		}
		return nil
	}

	err = ragSystem.GenerateQuestionIndex(
		rag.WithRAGCtx(ctx),
		rag.WithProgressHandler(func(percent float64, message string, messageType string) {
			if progress == nil {
				return
			}
			if err := progress(percent, message, messageType); err != nil {
				log.Warnf("publish knowledge base question index progress failed: %v", err)
			}
		}),
	)
	if err != nil {
		return utils.Errorf("生成问题索引失败: %v", err)
	}
	return nil
}

func listAIKnowledgeBases(command *aiv1.ListAIKnowledgeBasesCommand) ([]*aiv1.AIKnowledgeBaseRecord, *aiv1.AIKnowledgeBasePagination, int64, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, nil, 0, utils.Errorf("database not initialized")
	}

	pagination := command.GetPagination()
	if pagination == nil {
		pagination = &aiv1.AIKnowledgeBasePagination{Page: 1, Limit: 100}
	}
	if pagination.GetPage() <= 0 {
		pagination.Page = 1
	}
	if pagination.GetLimit() <= 0 {
		pagination.Limit = 100
	}

	req := &ypb.GetKnowledgeBaseRequest{
		Keyword:           strings.TrimSpace(command.GetQuery()),
		KnowledgeBaseId:   command.GetKnowledgeBaseId(),
		OnlyCreatedFromUI: command.GetOnlyCreatedFromUi(),
		OnlyIsDefault:     command.GetOnlyIsDefault(),
	}
	paginator, items, err := yakit.QueryKnowledgeBasePagingByFilter(db, req, &ypb.Paging{
		Page:    pagination.GetPage(),
		Limit:   pagination.GetLimit(),
		OrderBy: "updated_at",
		Order:   "desc",
	})
	if err != nil {
		return nil, nil, 0, err
	}

	records := make([]*aiv1.AIKnowledgeBaseRecord, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		records = append(records, mapSchemaKnowledgeBaseToLegion(db, item))
	}
	return records, &aiv1.AIKnowledgeBasePagination{
		Page:  int64(paginator.Page),
		Limit: int64(paginator.Limit),
	}, int64(paginator.TotalRecord), nil
}

func createAIKnowledgeBase(command *aiv1.CreateAIKnowledgeBaseCommand) (*aiv1.AIKnowledgeBaseRecord, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	ragSystem, err := rag.Get(
		command.GetKnowledgeBaseName(),
		rag.WithDB(db),
		rag.WithDescription(command.GetKnowledgeBaseDescription()),
		rag.WithTags(command.GetTags()...),
		rag.WithTryRebuildHNSWIndex(true),
	)
	if err != nil {
		return nil, err
	}
	if ragSystem == nil || ragSystem.KnowledgeBase == nil {
		return nil, utils.Errorf("knowledge base runtime is unavailable")
	}

	if err := ragSystem.KnowledgeBase.UpdateKnowledgeBaseInfo(
		command.GetKnowledgeBaseName(),
		command.GetKnowledgeBaseDescription(),
		command.GetKnowledgeBaseType(),
		command.GetTags()...,
	); err != nil {
		return nil, err
	}

	kbInfo, err := ragSystem.KnowledgeBase.GetInfo()
	if err != nil || kbInfo == nil {
		return nil, utils.Errorf("get knowledge base info failed")
	}
	if command.GetIsDefault() {
		if err := yakit.SetDefaultKnowledgeBase(db, int64(kbInfo.ID)); err != nil {
			return nil, err
		}
		kbInfo.IsDefault = true
	}
	if command.GetCreatedFromUi() {
		if err := db.Model(&schema.KnowledgeBaseInfo{}).Where("id = ?", kbInfo.ID).Update("created_from_ui", true).Error; err != nil {
			return nil, err
		}
		kbInfo.CreatedFromUI = true
	}
	return mapSchemaKnowledgeBaseToLegion(db, kbInfo), nil
}

func importAIKnowledgeBase(
	ctx context.Context,
	httpClient *http.Client,
	sessionToken string,
	command *aiv1.ImportAIKnowledgeBaseCommand,
) (*aiv1.AIKnowledgeBaseRecord, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	attachment := command.GetAttachment()
	content, err := downloadKnowledgeBaseImportAttachment(ctx, httpClient, sessionToken, attachment)
	if err != nil {
		return nil, err
	}

	tempFile, err := os.CreateTemp("", "legion-kb-import-*.rag")
	if err != nil {
		return nil, err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if _, err := tempFile.Write(content); err != nil {
		tempFile.Close()
		return nil, err
	}
	if err := tempFile.Close(); err != nil {
		return nil, err
	}

	if err := rag.ImportRAG(
		tempPath,
		rag.WithRAGCtx(ctx),
		rag.WithExportOverwriteExisting(true),
		rag.WithName(strings.TrimSpace(command.GetKnowledgeBaseName())),
		rag.WithDB(db),
	); err != nil {
		return nil, err
	}

	kbInfo, err := yakit.GetKnowledgeBaseByName(db, strings.TrimSpace(command.GetKnowledgeBaseName()))
	if err != nil {
		return nil, err
	}
	return mapSchemaKnowledgeBaseToLegion(db, kbInfo), nil
}

func downloadKnowledgeBaseImportAttachment(
	ctx context.Context,
	httpClient *http.Client,
	sessionToken string,
	attachment *aiv1.AIKnowledgeBaseImportAttachment,
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

func updateAIKnowledgeBase(command *aiv1.UpdateAIKnowledgeBaseCommand) (*aiv1.AIKnowledgeBaseRecord, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	if err := yakit.UpdateKnowledgeBaseInfo(db, command.GetKnowledgeBaseId(), &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        command.GetKnowledgeBaseName(),
		KnowledgeBaseDescription: command.GetKnowledgeBaseDescription(),
		KnowledgeBaseType:        command.GetKnowledgeBaseType(),
		Tags:                     strings.Join(command.GetTags(), ","),
	}); err != nil {
		return nil, err
	}
	if command.GetIsDefault() {
		if err := yakit.SetDefaultKnowledgeBase(db, command.GetKnowledgeBaseId()); err != nil {
			return nil, err
		}
	}

	kbInfo, err := yakit.GetKnowledgeBase(db, command.GetKnowledgeBaseId())
	if err != nil {
		return nil, err
	}
	return mapSchemaKnowledgeBaseToLegion(db, kbInfo), nil
}

func deleteAIKnowledgeBase(command *aiv1.DeleteAIKnowledgeBaseCommand) (*aiv1.AIKnowledgeBaseRecord, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	kbInfo, err := yakit.GetKnowledgeBase(db, command.GetKnowledgeBaseId())
	if err != nil {
		return nil, err
	}
	record := mapSchemaKnowledgeBaseToLegion(db, kbInfo)
	if err := rag.DeleteRAG(db, kbInfo.KnowledgeBaseName); err != nil {
		return nil, err
	}
	return record, nil
}

func exportAIKnowledgeBase(
	ctx context.Context,
	command *aiv1.ExportAIKnowledgeBaseCommand,
) (*aiv1.AIKnowledgeBaseRecord, string, string, []byte, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, "", "", nil, utils.Errorf("database not initialized")
	}

	kbInfo, err := yakit.GetKnowledgeBase(db, command.GetKnowledgeBaseId())
	if err != nil {
		return nil, "", "", nil, err
	}

	fileName := strings.TrimSpace(command.GetOutputName())
	if fileName == "" {
		fileName = strings.TrimSpace(kbInfo.KnowledgeBaseName)
	}
	if fileName == "" {
		fileName = fmt.Sprintf("knowledge-base-%d", command.GetKnowledgeBaseId())
	}
	if ext := strings.ToLower(filepath.Ext(fileName)); ext != ".rag" {
		fileName += ".rag"
	}

	tempDir, err := os.MkdirTemp("", "legion-kb-export-*")
	if err != nil {
		return nil, "", "", nil, err
	}
	defer os.RemoveAll(tempDir)

	targetPath := filepath.Join(tempDir, fileName)
	if err := rag.ExportRAG(
		kbInfo.KnowledgeBaseName,
		targetPath,
		rag.WithRAGCtx(ctx),
	); err != nil {
		return nil, "", "", nil, err
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, "", "", nil, err
	}

	return mapSchemaKnowledgeBaseToLegion(db, kbInfo), fileName, "application/octet-stream", content, nil
}

func searchAIKnowledgeBaseEntries(command *aiv1.SearchAIKnowledgeBaseEntriesCommand) ([]*aiv1.AIKnowledgeBaseEntryRecord, *aiv1.AIKnowledgeBasePagination, int64, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, nil, 0, utils.Errorf("database not initialized")
	}

	filter := command.GetFilter()
	pagination := command.GetPagination()
	if pagination == nil {
		pagination = &aiv1.AIKnowledgeBasePagination{Page: 1, Limit: 100}
	}
	if pagination.GetPage() <= 0 {
		pagination.Page = 1
	}
	if pagination.GetLimit() <= 0 {
		pagination.Limit = 100
	}
	orderBy := strings.TrimSpace(pagination.GetOrderBy())
	if orderBy == "" {
		orderBy = "id"
	}
	order := strings.TrimSpace(pagination.GetOrder())
	if order == "" {
		order = "desc"
	}

	paginator, entries, err := yakit.QueryKnowledgeBaseEntryPaging(db, &ypb.SearchKnowledgeBaseEntryFilter{
		KnowledgeBaseId:    filter.GetKnowledgeBaseId(),
		Keyword:            strings.TrimSpace(filter.GetKeyword()),
		RelatedEntityUUIDS: append([]string(nil), filter.GetRelatedEntityUuids()...),
		HiddenIndex:        append([]string(nil), filter.GetHiddenIndex()...),
	}, &ypb.Paging{
		Page:    pagination.GetPage(),
		Limit:   pagination.GetLimit(),
		OrderBy: orderBy,
		Order:   order,
	})
	if err != nil {
		return nil, nil, 0, err
	}

	items := make([]*aiv1.AIKnowledgeBaseEntryRecord, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		items = append(items, mapSchemaKnowledgeBaseEntryToLegion(entry))
	}
	return items, &aiv1.AIKnowledgeBasePagination{
		Page:    int64(paginator.Page),
		Limit:   int64(paginator.Limit),
		OrderBy: orderBy,
		Order:   order,
	}, int64(paginator.TotalRecord), nil
}

func createAIKnowledgeBaseEntry(command *aiv1.CreateAIKnowledgeBaseEntryCommand) (*aiv1.AIKnowledgeBaseEntryRecord, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	kb, err := knowledgebase.LoadKnowledgeBaseByID(db, command.GetKnowledgeBaseId())
	if err != nil {
		return nil, utils.Errorf("加载知识库失败: %v", err)
	}

	entry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:          command.GetKnowledgeBaseId(),
		KnowledgeTitle:           command.GetKnowledgeTitle(),
		KnowledgeType:            command.GetKnowledgeType(),
		ImportanceScore:          int(command.GetImportanceScore()),
		Keywords:                 append([]string(nil), command.GetKeywords()...),
		KnowledgeDetails:         command.GetKnowledgeDetails(),
		Summary:                  command.GetSummary(),
		SourcePage:               int(command.GetSourcePage()),
		PotentialQuestions:       append([]string(nil), command.GetPotentialQuestions()...),
		PotentialQuestionsVector: append([]float32(nil), command.GetPotentialQuestionsVector()...),
	}
	if err := kb.AddKnowledgeEntry(entry); err != nil {
		return nil, err
	}
	saved, err := yakit.GetKnowledgeBaseEntryByHiddenIndex(db, strings.TrimSpace(entry.HiddenIndex))
	if err != nil {
		return nil, err
	}
	return mapSchemaKnowledgeBaseEntryToLegion(saved), nil
}

func updateAIKnowledgeBaseEntry(command *aiv1.UpdateAIKnowledgeBaseEntryCommand) (*aiv1.AIKnowledgeBaseEntryRecord, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	kb, err := knowledgebase.LoadKnowledgeBaseByID(db, command.GetKnowledgeBaseId())
	if err != nil {
		return nil, utils.Errorf("获取知识库信息失败: %v", err)
	}

	hiddenIndex := strings.TrimSpace(command.GetKnowledgeBaseEntryHiddenIndex())
	err = kb.UpdateKnowledgeEntry(hiddenIndex, &schema.KnowledgeBaseEntry{
		KnowledgeTitle:     command.GetKnowledgeTitle(),
		KnowledgeType:      command.GetKnowledgeType(),
		ImportanceScore:    int(command.GetImportanceScore()),
		Keywords:           append([]string(nil), command.GetKeywords()...),
		KnowledgeDetails:   command.GetKnowledgeDetails(),
		Summary:            command.GetSummary(),
		SourcePage:         int(command.GetSourcePage()),
		PotentialQuestions: append([]string(nil), command.GetPotentialQuestions()...),
	})
	if err != nil {
		return nil, utils.Errorf("更新知识库条目失败: %v", err)
	}
	saved, err := yakit.GetKnowledgeBaseEntryByHiddenIndex(db, hiddenIndex)
	if err != nil {
		return nil, err
	}
	return mapSchemaKnowledgeBaseEntryToLegion(saved), nil
}

func deleteAIKnowledgeBaseEntry(command *aiv1.DeleteAIKnowledgeBaseEntryCommand) error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("database not initialized")
	}

	kb, err := knowledgebase.LoadKnowledgeBaseByID(db, command.GetKnowledgeBaseId())
	if err != nil {
		return utils.Errorf("获取知识库信息失败: %v", err)
	}
	if err := kb.DeleteKnowledgeEntry(strings.TrimSpace(command.GetKnowledgeBaseEntryHiddenIndex())); err != nil {
		return utils.Errorf("删除知识库条目失败: %v", err)
	}
	return nil
}

func buildAIKnowledgeBaseVectorIndex(command *aiv1.BuildAIKnowledgeBaseVectorIndexCommand) error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("database not initialized")
	}
	_, err := rag.BuildVectorIndexForKnowledgeBase(db, command.GetKnowledgeBaseId(), buildAIKnowledgeBaseRAGOptions(
		command.GetBaseUrl(),
		command.GetApiKey(),
		command.GetProxy(),
		command.GetModelName(),
		command.GetDimension(),
		command.GetM(),
		command.GetMl(),
		command.GetEfSearch(),
		command.GetEfConstruct(),
		command.GetDistanceFuncType(),
	)...)
	return err
}

func buildAIKnowledgeBaseEntryVectorIndex(command *aiv1.BuildAIKnowledgeBaseEntryVectorIndexCommand) error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("database not initialized")
	}
	_, err := rag.BuildVectorIndexForKnowledgeBaseEntry(db, command.GetKnowledgeBaseId(), command.GetKnowledgeBaseEntryHiddenIndex(), buildAIKnowledgeBaseRAGOptions(
		command.GetBaseUrl(),
		command.GetApiKey(),
		command.GetProxy(),
		command.GetModelName(),
		command.GetDimension(),
		command.GetM(),
		command.GetMl(),
		command.GetEfSearch(),
		command.GetEfConstruct(),
		command.GetDistanceFuncType(),
	)...)
	return err
}

func buildAIKnowledgeBaseRAGOptions(
	baseURL string,
	apiKey string,
	proxy string,
	modelName string,
	dimension int32,
	m int32,
	ml float32,
	efSearch int32,
	efConstruct int32,
	distanceFuncType string,
) []rag.RAGSystemConfigOption {
	aiConfig := []aispec.AIConfigOption{
		aispec.WithBaseURL(strings.TrimSpace(baseURL)),
		aispec.WithAPIKey(strings.TrimSpace(apiKey)),
		aispec.WithModel(strings.TrimSpace(modelName)),
		aispec.WithProxy(strings.TrimSpace(proxy)),
	}
	options := []rag.RAGSystemConfigOption{
		rag.WithEmbeddingModel(strings.TrimSpace(modelName)),
		rag.WithModelDimension(int(dimension)),
		rag.WithHNSWParameters(int(m), float64(ml), int(efSearch), int(efConstruct)),
		rag.WithAIOptions(aiConfig...),
	}
	switch strings.TrimSpace(distanceFuncType) {
	case "cosine":
		options = append(options, rag.WithCosineDistance())
	}
	return options
}

func mapSchemaKnowledgeBaseToLegion(db *gorm.DB, item *schema.KnowledgeBaseInfo) *aiv1.AIKnowledgeBaseRecord {
	if item == nil {
		return nil
	}
	isImported := strings.TrimSpace(item.SerialVersionUID) != ""
	if !isImported && db != nil {
		collectionInfo, _ := yakit.GetRAGCollectionInfoByName(db, item.KnowledgeBaseName)
		isImported = collectionInfo != nil && strings.TrimSpace(collectionInfo.SerialVersionUID) != ""
	}
	return &aiv1.AIKnowledgeBaseRecord{
		Id:                       int64(item.ID),
		KnowledgeBaseName:        item.KnowledgeBaseName,
		KnowledgeBaseDescription: item.KnowledgeBaseDescription,
		KnowledgeBaseType:        item.KnowledgeBaseType,
		Tags:                     utils.StringSplitAndStrip(item.Tags, ","),
		IsDefault:                item.IsDefault,
		CreatedFromUi:            item.CreatedFromUI,
		IsImported:               isImported,
		SerialVersionId:          item.SerialVersionUID,
		CreatedAt:                item.CreatedAt.UnixMilli(),
		UpdatedAt:                item.UpdatedAt.UnixMilli(),
	}
}

func mapSchemaKnowledgeBaseEntryToLegion(item *schema.KnowledgeBaseEntry) *aiv1.AIKnowledgeBaseEntryRecord {
	if item == nil {
		return nil
	}
	return &aiv1.AIKnowledgeBaseEntryRecord{
		Id:                       int64(item.ID),
		KnowledgeBaseId:          item.KnowledgeBaseID,
		KnowledgeTitle:           utils.EscapeInvalidUTF8Byte([]byte(item.KnowledgeTitle)),
		KnowledgeType:            utils.EscapeInvalidUTF8Byte([]byte(item.KnowledgeType)),
		ImportanceScore:          int32(item.ImportanceScore),
		Keywords:                 append([]string(nil), item.Keywords...),
		KnowledgeDetails:         utils.EscapeInvalidUTF8Byte([]byte(item.KnowledgeDetails)),
		Summary:                  utils.EscapeInvalidUTF8Byte([]byte(item.Summary)),
		SourcePage:               int32(item.SourcePage),
		PotentialQuestions:       append([]string(nil), item.PotentialQuestions...),
		PotentialQuestionsVector: append([]float32(nil), item.PotentialQuestionsVector...),
		HiddenIndex:              utils.EscapeInvalidUTF8Byte([]byte(item.HiddenIndex)),
		RelatedEntityUuids:       utils.EscapeInvalidUTF8Byte([]byte(item.RelatedEntityUUIDS)),
		CreatedAt:                item.CreatedAt.UnixMilli(),
		UpdatedAt:                item.UpdatedAt.UnixMilli(),
	}
}
