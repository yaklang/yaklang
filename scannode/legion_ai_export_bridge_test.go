package scannode

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func TestValidateAILogsCheckpointsExportCommand(t *testing.T) {
	valid := func() *aiv1.ExportAILogsCheckpointsCommand {
		return &aiv1.ExportAILogsCheckpointsCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-1"},
			TargetNodeId: "node-a",
			OwnerUserId:  "user-a",
			SessionId:    "session-a",
		}
	}

	tests := []struct {
		name    string
		mutate  func(*aiv1.ExportAILogsCheckpointsCommand)
		wantErr string
	}{
		{name: "accepts valid command"},
		{
			name: "requires metadata",
			mutate: func(command *aiv1.ExportAILogsCheckpointsCommand) {
				command.Metadata = nil
			},
			wantErr: "metadata is required",
		},
		{
			name: "requires node match",
			mutate: func(command *aiv1.ExportAILogsCheckpointsCommand) {
				command.TargetNodeId = "node-b"
			},
			wantErr: "target_node_id mismatch",
		},
		{
			name: "requires owner user id",
			mutate: func(command *aiv1.ExportAILogsCheckpointsCommand) {
				command.OwnerUserId = " "
			},
			wantErr: "owner_user_id is required",
		},
		{
			name: "requires session or coordinator ids",
			mutate: func(command *aiv1.ExportAILogsCheckpointsCommand) {
				command.SessionId = " "
			},
			wantErr: "session_id or coordinator_ids is required",
		},
		{
			name: "allows coordinator ids without session",
			mutate: func(command *aiv1.ExportAILogsCheckpointsCommand) {
				command.SessionId = " "
				command.CoordinatorIds = []string{"coor-1"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := valid()
			if tt.mutate != nil {
				tt.mutate(command)
			}

			err := validateAILogsCheckpointsExportCommand("node-a", command)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAILogsRefFromCheckpointsExportCommand(t *testing.T) {
	command := &aiv1.ExportAILogsCheckpointsCommand{
		Metadata:     &nodev1.CommandMetadata{CommandId: " cmd-1 "},
		TargetNodeId: "node-a",
		OwnerUserId:  " user-a ",
		SessionId:    " session-a ",
	}

	ref := aiLogsRefFromCheckpointsExportCommand(command)
	if ref.CommandID != "cmd-1" || ref.OwnerUserID != "user-a" || ref.SessionID != "session-a" {
		t.Fatalf("unexpected ref: %#v", ref)
	}
}

func TestExportAILogsCheckpointsUsesSessionEventsToResolveCoordinatorIDs(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		t.Fatal("expected project database")
	}

	sessionID := uuid.NewString()
	coordinatorID1 := uuid.NewString()
	coordinatorID2 := uuid.NewString()

	events := []*schema.AiOutputEvent{
		{
			SessionId:     sessionID,
			CoordinatorId: coordinatorID1,
			EventUUID:     uuid.NewString(),
			Type:          schema.EVENT_TYPE_STREAM,
			Content:       []byte("event-1"),
		},
		{
			SessionId:     sessionID,
			CoordinatorId: coordinatorID2,
			EventUUID:     uuid.NewString(),
			Type:          schema.EVENT_TYPE_RESULT,
			Content:       []byte("event-2"),
		},
	}
	for _, event := range events {
		if err := db.Create(event).Error; err != nil {
			t.Fatalf("create event: %v", err)
		}
	}

	checkpoints := []*schema.AiCheckpoint{
		{
			CoordinatorUuid:    coordinatorID1,
			Seq:                1,
			Type:               schema.AiCheckpointType_AIInteractive,
			RequestQuotedJson:  "{}",
			ResponseQuotedJson: "{}",
		},
		{
			CoordinatorUuid:    coordinatorID2,
			Seq:                2,
			Type:               schema.AiCheckpointType_ToolCall,
			RequestQuotedJson:  "{}",
			ResponseQuotedJson: "{}",
			Finished:           true,
		},
	}
	for _, checkpoint := range checkpoints {
		if err := db.Create(checkpoint).Error; err != nil {
			t.Fatalf("create checkpoint: %v", err)
		}
	}

	defer func() {
		db.Where("session_id = ?", sessionID).Delete(&schema.AiOutputEvent{})
		db.Where("coordinator_uuid IN (?)", []string{coordinatorID1, coordinatorID2}).Delete(&schema.AiCheckpoint{})
	}()

	raw, total, err := exportAILogsCheckpoints(&aiv1.ExportAILogsCheckpointsCommand{
		SessionId: sessionID,
	})
	if err != nil {
		t.Fatalf("export checkpoints: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", total)
	}

	var exported []map[string]any
	if err := json.Unmarshal(raw, &exported); err != nil {
		t.Fatalf("unmarshal checkpoints: %v", err)
	}
	if len(exported) != 2 {
		t.Fatalf("expected 2 exported checkpoints, got %d", len(exported))
	}
}
