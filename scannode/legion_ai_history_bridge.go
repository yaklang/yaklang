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

func (b *legionJobBridge) handleAISessionTitleUpdate(ctx context.Context, raw []byte) error {
	var command aiv1.UpdateAISessionTitleCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai session title update command: %w", err)
	}

	ref := aiSessionHistoryRefFromTitleUpdateCommand(&command)
	if err := validateAISessionTitleUpdateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAISessionTitleUpdateFailed(
			ctx,
			ref,
			"invalid_ai_session_title_update_command",
			err.Error(),
		)
	}

	db := consts.GetGormProjectDatabase()
	if db == nil {
		return b.ensureAIPublisher().PublishAISessionTitleUpdateFailed(
			ctx,
			ref,
			"ai_session_project_db_unavailable",
			"project database not initialized",
		)
	}

	sessionID := strings.TrimSpace(command.GetSession().GetSessionId())
	title := strings.TrimSpace(command.GetTitle())
	affected, err := yakit.UpdateAISessionMetaTitle(db, sessionID, title)
	if err != nil {
		return b.ensureAIPublisher().PublishAISessionTitleUpdateFailed(
			ctx,
			ref,
			"ai_session_title_update_failed",
			err.Error(),
		)
	}
	if affected == 0 {
		if _, err := yakit.CreateOrUpdateAISessionMeta(db, sessionID, title); err != nil {
			return b.ensureAIPublisher().PublishAISessionTitleUpdateFailed(
				ctx,
				ref,
				"ai_session_title_create_failed",
				err.Error(),
			)
		}
	}

	return b.ensureAIPublisher().PublishAISessionTitleUpdated(ctx, ref, title, "ok")
}

func (b *legionJobBridge) handleAISessionDelete(ctx context.Context, raw []byte) error {
	var command aiv1.DeleteAISessionCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai session delete command: %w", err)
	}

	ref := aiSessionHistoryRefFromDeleteCommand(&command)
	if err := validateAISessionDeleteCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAISessionDeleteFailed(
			ctx,
			ref,
			"invalid_ai_session_delete_command",
			err.Error(),
		)
	}

	db := consts.GetGormProjectDatabase()
	if db == nil {
		return b.ensureAIPublisher().PublishAISessionDeleteFailed(
			ctx,
			ref,
			"ai_session_project_db_unavailable",
			"project database not initialized",
		)
	}

	sessionID := strings.TrimSpace(command.GetSession().GetSessionId())
	deletedRuntimes, deletedEvents, err := yakit.DeleteAISession(db, sessionID)
	if err != nil {
		return b.ensureAIPublisher().PublishAISessionDeleteFailed(
			ctx,
			ref,
			"ai_session_delete_failed",
			err.Error(),
		)
	}

	return b.ensureAIPublisher().PublishAISessionDeleteCompleted(
		ctx,
		ref,
		fmt.Sprintf(
			"deleted_runtimes=%d deleted_events=%d",
			deletedRuntimes,
			deletedEvents,
		),
	)
}

func validateAISessionTitleUpdateCommand(nodeID string, command *aiv1.UpdateAISessionTitleCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai session title update metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai session title update command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai session title update target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai session title update target_node_id mismatch: %s", command.GetTargetNodeId())
	case command.GetSession() == nil:
		return fmt.Errorf("ai session title update session reference is required")
	case strings.TrimSpace(command.GetSession().GetSessionId()) == "":
		return fmt.Errorf("ai session title update session_id is required")
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai session title update owner_user_id is required")
	default:
		return nil
	}
}

func validateAISessionDeleteCommand(nodeID string, command *aiv1.DeleteAISessionCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai session delete metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai session delete command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai session delete target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai session delete target_node_id mismatch: %s", command.GetTargetNodeId())
	case command.GetSession() == nil:
		return fmt.Errorf("ai session delete session reference is required")
	case strings.TrimSpace(command.GetSession().GetSessionId()) == "":
		return fmt.Errorf("ai session delete session_id is required")
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai session delete owner_user_id is required")
	default:
		return nil
	}
}

func aiSessionHistoryRefFromTitleUpdateCommand(
	command *aiv1.UpdateAISessionTitleCommand,
) aiSessionCommandRef {
	return aiSessionCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		SessionID:   strings.TrimSpace(command.GetSession().GetSessionId()),
		RunID:       strings.TrimSpace(command.GetSession().GetRunId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiSessionHistoryRefFromDeleteCommand(command *aiv1.DeleteAISessionCommand) aiSessionCommandRef {
	return aiSessionCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		SessionID:   strings.TrimSpace(command.GetSession().GetSessionId()),
		RunID:       strings.TrimSpace(command.GetSession().GetRunId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}
