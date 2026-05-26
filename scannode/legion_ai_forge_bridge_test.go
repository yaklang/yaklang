package scannode

import (
	"strings"
	"testing"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func TestValidateAIForgeUpdateCommand(t *testing.T) {
	valid := func() *aiv1.UpdateAIForgeCommand {
		return &aiv1.UpdateAIForgeCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-1"},
			TargetNodeId: "node-a",
			OwnerUserId:  "user-a",
			ForgeId:      42,
			ForgeName:    "forge-a",
			ForgeType:    "yak",
		}
	}

	tests := []struct {
		name    string
		mutate  func(*aiv1.UpdateAIForgeCommand)
		wantErr string
	}{
		{name: "accepts valid command"},
		{
			name: "requires metadata",
			mutate: func(command *aiv1.UpdateAIForgeCommand) {
				command.Metadata = nil
			},
			wantErr: "metadata is required",
		},
		{
			name: "requires target node match",
			mutate: func(command *aiv1.UpdateAIForgeCommand) {
				command.TargetNodeId = "node-b"
			},
			wantErr: "target_node_id mismatch",
		},
		{
			name: "requires forge id",
			mutate: func(command *aiv1.UpdateAIForgeCommand) {
				command.ForgeId = 0
			},
			wantErr: "forge_id must be greater than 0",
		},
		{
			name: "requires forge name",
			mutate: func(command *aiv1.UpdateAIForgeCommand) {
				command.ForgeName = " "
			},
			wantErr: "forge_name is required",
		},
		{
			name: "requires forge type",
			mutate: func(command *aiv1.UpdateAIForgeCommand) {
				command.ForgeType = " "
			},
			wantErr: "forge_type is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := valid()
			if tt.mutate != nil {
				tt.mutate(command)
			}

			err := validateAIForgeUpdateCommand("node-a", command)
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

func TestAIForgeRefFromCreateCommand(t *testing.T) {
	command := &aiv1.CreateAIForgeCommand{
		Metadata:     &nodev1.CommandMetadata{CommandId: " cmd-1 "},
		OwnerUserId:  " user-a ",
		TargetNodeId: "node-a",
		ForgeName:    "forge-a",
		ForgeType:    "yak",
	}

	ref := aiForgeRefFromCreateCommand(command)
	if ref.CommandID != "cmd-1" || ref.OwnerUserID != "user-a" {
		t.Fatalf("unexpected ref: %#v", ref)
	}
}

func TestValidateAIForgeImportCommand(t *testing.T) {
	valid := func() *aiv1.ImportAIForgeCommand {
		return &aiv1.ImportAIForgeCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-1"},
			TargetNodeId: "node-a",
			OwnerUserId:  "user-a",
			Attachment: &aiv1.AIForgeImportAttachment{
				AttachmentId: "aiatt_1",
				DownloadUrl:  "https://platform.example/v1/ai/attachments/aiatt_1/download",
			},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*aiv1.ImportAIForgeCommand)
		wantErr string
	}{
		{name: "accepts valid command"},
		{
			name: "requires attachment",
			mutate: func(command *aiv1.ImportAIForgeCommand) {
				command.Attachment = nil
			},
			wantErr: "attachment is required",
		},
		{
			name: "requires attachment id",
			mutate: func(command *aiv1.ImportAIForgeCommand) {
				command.Attachment.AttachmentId = " "
			},
			wantErr: "attachment_id is required",
		},
		{
			name: "requires download url",
			mutate: func(command *aiv1.ImportAIForgeCommand) {
				command.Attachment.DownloadUrl = " "
			},
			wantErr: "download_url is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := valid()
			if tt.mutate != nil {
				tt.mutate(command)
			}

			err := validateAIForgeImportCommand("node-a", command)
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

func TestNormalizeAIForgeStringSlice(t *testing.T) {
	got := normalizeAIForgeStringSlice([]string{" one ", "", "two", "   "})
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("unexpected normalized slice: %#v", got)
	}
}
