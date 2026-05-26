package scannode

import (
	"strings"
	"testing"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func TestValidateAIToolGenerateMetadataCommand(t *testing.T) {
	valid := func() *aiv1.GenerateAIToolMetadataCommand {
		return &aiv1.GenerateAIToolMetadataCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-1"},
			TargetNodeId: "node-a",
			OwnerUserId:  "user-a",
			ToolName:     "tool-a",
			Content:      "println('hi')",
		}
	}

	tests := []struct {
		name    string
		mutate  func(*aiv1.GenerateAIToolMetadataCommand)
		wantErr string
	}{
		{
			name: "accepts valid command",
		},
		{
			name: "requires metadata",
			mutate: func(command *aiv1.GenerateAIToolMetadataCommand) {
				command.Metadata = nil
			},
			wantErr: "metadata is required",
		},
		{
			name: "requires target node match",
			mutate: func(command *aiv1.GenerateAIToolMetadataCommand) {
				command.TargetNodeId = "node-b"
			},
			wantErr: "target_node_id mismatch",
		},
		{
			name: "requires owner user id",
			mutate: func(command *aiv1.GenerateAIToolMetadataCommand) {
				command.OwnerUserId = " "
			},
			wantErr: "owner_user_id is required",
		},
		{
			name: "requires content",
			mutate: func(command *aiv1.GenerateAIToolMetadataCommand) {
				command.Content = " "
			},
			wantErr: "content is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := valid()
			if tt.mutate != nil {
				tt.mutate(command)
			}

			err := validateAIToolGenerateMetadataCommand("node-a", command)
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

func TestAIToolRefFromGenerateMetadataCommand(t *testing.T) {
	command := &aiv1.GenerateAIToolMetadataCommand{
		Metadata:     &nodev1.CommandMetadata{CommandId: " cmd-1 "},
		OwnerUserId:  " user-a ",
		TargetNodeId: "node-a",
		Content:      "println('hi')",
	}

	ref := aiToolRefFromGenerateMetadataCommand(command)
	if ref.CommandID != "cmd-1" || ref.OwnerUserID != "user-a" {
		t.Fatalf("unexpected ref: %#v", ref)
	}
}
