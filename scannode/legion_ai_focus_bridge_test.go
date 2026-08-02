package scannode

import (
	"strings"
	"testing"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func TestValidateAIFocusQueryCommand(t *testing.T) {
	valid := func() *aiv1.QueryAIFocusCommand {
		return &aiv1.QueryAIFocusCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-1"},
			TargetNodeId: "node-a",
			OwnerUserId:  "user-a",
		}
	}

	tests := []struct {
		name    string
		mutate  func(*aiv1.QueryAIFocusCommand)
		wantErr string
	}{
		{name: "accepts valid command"},
		{
			name: "requires metadata",
			mutate: func(command *aiv1.QueryAIFocusCommand) {
				command.Metadata = nil
			},
			wantErr: "metadata is required",
		},
		{
			name: "requires target node match",
			mutate: func(command *aiv1.QueryAIFocusCommand) {
				command.TargetNodeId = "node-b"
			},
			wantErr: "target_node_id mismatch",
		},
		{
			name: "requires owner user id",
			mutate: func(command *aiv1.QueryAIFocusCommand) {
				command.OwnerUserId = " "
			},
			wantErr: "owner_user_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := valid()
			if tt.mutate != nil {
				tt.mutate(command)
			}

			err := validateAIFocusQueryCommand("node-a", command)
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

func TestAIFocusRefFromQueryCommand(t *testing.T) {
	command := &aiv1.QueryAIFocusCommand{
		Metadata:     &nodev1.CommandMetadata{CommandId: " cmd-1 "},
		OwnerUserId:  " user-a ",
		TargetNodeId: "node-a",
	}

	ref := aiFocusRefFromQueryCommand(command)
	if ref.CommandID != "cmd-1" || ref.OwnerUserID != "user-a" {
		t.Fatalf("unexpected ref: %#v", ref)
	}
}
