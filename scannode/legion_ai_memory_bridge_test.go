package scannode

import (
	"strings"
	"testing"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func TestValidateAIMemoryEntityGetCommand(t *testing.T) {
	valid := func() *aiv1.GetAIMemoryEntityCommand {
		return &aiv1.GetAIMemoryEntityCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-1"},
			TargetNodeId: "node-a",
			OwnerUserId:  "user-a",
			SessionId:    "session-a",
			MemoryId:     "memory-a",
		}
	}

	tests := []struct {
		name    string
		mutate  func(*aiv1.GetAIMemoryEntityCommand)
		wantErr string
	}{
		{name: "accepts valid command"},
		{
			name: "requires metadata",
			mutate: func(command *aiv1.GetAIMemoryEntityCommand) {
				command.Metadata = nil
			},
			wantErr: "metadata is required",
		},
		{
			name: "requires node match",
			mutate: func(command *aiv1.GetAIMemoryEntityCommand) {
				command.TargetNodeId = "node-b"
			},
			wantErr: "target_node_id mismatch",
		},
		{
			name: "requires memory id",
			mutate: func(command *aiv1.GetAIMemoryEntityCommand) {
				command.MemoryId = " "
			},
			wantErr: "memory_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := valid()
			if tt.mutate != nil {
				tt.mutate(command)
			}

			err := validateAIMemoryEntityGetCommand("node-a", command)
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

func TestAIMemoryRefFromQueryCommand(t *testing.T) {
	command := &aiv1.QueryAIMemoryEntitiesCommand{
		Metadata:     &nodev1.CommandMetadata{CommandId: " cmd-1 "},
		TargetNodeId: "node-a",
		OwnerUserId:  " user-a ",
	}

	ref := aiMemoryRefFromQueryCommand(command)
	if ref.CommandID != "cmd-1" || ref.OwnerUserID != "user-a" {
		t.Fatalf("unexpected ref: %#v", ref)
	}
}

func TestMapLegionAIMemoryFilterToGRPC(t *testing.T) {
	filter := mapLegionAIMemoryFilterToGRPC(&aiv1.AIMemoryEntityFilter{
		SessionId:                " session-a ",
		MemoryIds:                []string{"memory-a"},
		ContentKeyword:           "keyword",
		Tags:                     []string{"infra"},
		TagMatchAll:              true,
		PotentialQuestionKeyword: "question",
		SemanticQuery:            "semantic",
		VectorTopK:               12,
	})

	if filter.GetSessionID() != "session-a" ||
		len(filter.GetMemoryID()) != 1 ||
		filter.GetMemoryID()[0] != "memory-a" ||
		!filter.GetTagMatchAll() ||
		filter.GetVectorTopK() != 12 {
		t.Fatalf("unexpected filter: %#v", filter)
	}
}
