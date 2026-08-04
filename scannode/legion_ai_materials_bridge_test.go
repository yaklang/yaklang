package scannode

import (
	"strings"
	"testing"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func TestValidateAIMaterialsRandomQueryCommand(t *testing.T) {
	valid := func() *aiv1.GetRandomAIMaterialsCommand {
		return &aiv1.GetRandomAIMaterialsCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-1"},
			TargetNodeId: "node-a",
			OwnerUserId:  "user-a",
			Limit:        3,
		}
	}

	tests := []struct {
		name    string
		mutate  func(*aiv1.GetRandomAIMaterialsCommand)
		wantErr string
	}{
		{name: "accepts valid command"},
		{
			name: "requires metadata",
			mutate: func(command *aiv1.GetRandomAIMaterialsCommand) {
				command.Metadata = nil
			},
			wantErr: "metadata is required",
		},
		{
			name: "requires command id",
			mutate: func(command *aiv1.GetRandomAIMaterialsCommand) {
				command.Metadata.CommandId = " "
			},
			wantErr: "command_id is required",
		},
		{
			name: "requires target node match",
			mutate: func(command *aiv1.GetRandomAIMaterialsCommand) {
				command.TargetNodeId = "node-b"
			},
			wantErr: "target_node_id mismatch",
		},
		{
			name: "requires owner user id",
			mutate: func(command *aiv1.GetRandomAIMaterialsCommand) {
				command.OwnerUserId = " "
			},
			wantErr: "owner_user_id is required",
		},
		{
			name: "rejects negative limit",
			mutate: func(command *aiv1.GetRandomAIMaterialsCommand) {
				command.Limit = -1
			},
			wantErr: "limit must be greater than or equal to zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := valid()
			if tt.mutate != nil {
				tt.mutate(command)
			}

			err := validateAIMaterialsRandomQueryCommand("node-a", command)
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

func TestAIMaterialsRefFromCommand(t *testing.T) {
	command := &aiv1.GetRandomAIMaterialsCommand{
		Metadata:     &nodev1.CommandMetadata{CommandId: " cmd-1 "},
		TargetNodeId: "node-a",
		OwnerUserId:  " user-a ",
	}

	ref := aiMaterialsRefFromCommand(command)
	if ref.CommandID != "cmd-1" || ref.OwnerUserID != "user-a" {
		t.Fatalf("unexpected ref: %#v", ref)
	}
}
