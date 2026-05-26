package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func (b *legionJobBridge) handleAILogsCheckpointsExport(ctx context.Context, raw []byte) error {
	var command aiv1.ExportAILogsCheckpointsCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai logs checkpoint export command: %w", err)
	}

	ref := aiLogsRefFromCheckpointsExportCommand(&command)
	if err := validateAILogsCheckpointsExportCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAILogsCheckpointsExportFailed(
			ctx,
			ref,
			"invalid_ai_logs_checkpoint_export_command",
			err.Error(),
		)
	}

	checkpointsJSON, total, err := exportAILogsCheckpoints(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAILogsCheckpointsExportFailed(
			ctx,
			ref,
			"ai_logs_checkpoint_export_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAILogsCheckpointsExported(ctx, ref, checkpointsJSON, total)
}

func validateAILogsCheckpointsExportCommand(nodeID string, command *aiv1.ExportAILogsCheckpointsCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai logs checkpoint export metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai logs checkpoint export command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai logs checkpoint export target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai logs checkpoint export target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai logs checkpoint export owner_user_id is required")
	case strings.TrimSpace(command.GetSessionId()) == "" && len(normalizeAILogsCheckpointCoordinatorIDs(command.GetCoordinatorIds())) == 0:
		return fmt.Errorf("ai logs checkpoint export session_id or coordinator_ids is required")
	default:
		return nil
	}
}

func aiLogsRefFromCheckpointsExportCommand(command *aiv1.ExportAILogsCheckpointsCommand) aiLogsCommandRef {
	if command == nil {
		return aiLogsCommandRef{}
	}
	commandID := ""
	if command.GetMetadata() != nil {
		commandID = strings.TrimSpace(command.GetMetadata().GetCommandId())
	}
	return aiLogsCommandRef{
		CommandID:   commandID,
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
		SessionID:   strings.TrimSpace(command.GetSessionId()),
	}
}

func exportAILogsCheckpoints(command *aiv1.ExportAILogsCheckpointsCommand) ([]byte, int64, error) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	coordinatorIDs, err := resolveAILogsCheckpointCoordinatorIDs(
		db,
		strings.TrimSpace(command.GetSessionId()),
		command.GetCoordinatorIds(),
	)
	if err != nil {
		return nil, 0, err
	}

	checkpoints := make([]*schema.AiCheckpoint, 0)
	if len(coordinatorIDs) > 0 {
		if err := db.
			Where("coordinator_uuid IN (?)", coordinatorIDs).
			Order("coordinator_uuid ASC").
			Order("seq ASC").
			Find(&checkpoints).Error; err != nil {
			return nil, 0, fmt.Errorf("failed to query checkpoints: %w", err)
		}
	}

	raw, err := json.Marshal(checkpoints)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal checkpoints export: %w", err)
	}
	return raw, int64(len(checkpoints)), nil
}

func resolveAILogsCheckpointCoordinatorIDs(
	db *gorm.DB,
	sessionID string,
	provided []string,
) ([]string, error) {
	if strings.TrimSpace(sessionID) != "" {
		var events []*schema.AiOutputEvent
		if err := db.
			Where("session_id = ?", strings.TrimSpace(sessionID)).
			Find(&events).Error; err != nil {
			return nil, fmt.Errorf("failed to query session events: %w", err)
		}
		coordinatorIDs := make([]string, 0, len(events))
		for _, event := range events {
			if event == nil {
				continue
			}
			coordinatorIDs = append(coordinatorIDs, event.CoordinatorId)
		}
		return normalizeAILogsCheckpointCoordinatorIDs(coordinatorIDs), nil
	}
	return normalizeAILogsCheckpointCoordinatorIDs(provided), nil
}

func normalizeAILogsCheckpointCoordinatorIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	items := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		items = append(items, normalized)
	}
	return items
}

func buildAILogsCheckpointsExportCommand(
	commandID string,
	targetNodeID string,
	ownerUserID string,
	sessionID string,
	coordinatorIDs []string,
) *aiv1.ExportAILogsCheckpointsCommand {
	return &aiv1.ExportAILogsCheckpointsCommand{
		Metadata: &nodev1.CommandMetadata{
			CommandId: commandID,
		},
		TargetNodeId:   targetNodeID,
		OwnerUserId:    ownerUserID,
		SessionId:      sessionID,
		CoordinatorIds: coordinatorIDs,
	}
}
