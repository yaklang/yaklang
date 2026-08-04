package scannode

import (
	"testing"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/proto"
)

func TestValidateAIKnowledgeBaseEntriesSearchCommand(t *testing.T) {
	command := &aiv1.SearchAIKnowledgeBaseEntriesCommand{
		Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-kb-search"},
		TargetNodeId: "node-1",
		OwnerUserId:  "user-1",
		Filter: &aiv1.AIKnowledgeBaseEntryFilter{
			KnowledgeBaseId: 42,
		},
	}
	if err := validateAIKnowledgeBaseEntriesSearchCommand("node-1", command); err != nil {
		t.Fatalf("expected search command to validate, got %v", err)
	}
}

func TestValidateAIKnowledgeBaseQueryByAICommandAllowsQueryAllCollections(t *testing.T) {
	command := &aiv1.QueryAIKnowledgeBaseByAICommand{
		Metadata:            &nodev1.CommandMetadata{CommandId: "cmd-kb-query"},
		TargetNodeId:        "node-1",
		OwnerUserId:         "user-1",
		Query:               "how does this work",
		QueryAllCollections: true,
	}
	if err := validateAIKnowledgeBaseQueryByAICommand("node-1", command); err != nil {
		t.Fatalf("expected query command to validate, got %v", err)
	}
}

func TestValidateAIKnowledgeBaseQueryByAICommandRequiresKnowledgeBaseIDWhenScoped(t *testing.T) {
	command := &aiv1.QueryAIKnowledgeBaseByAICommand{
		Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-kb-query"},
		TargetNodeId: "node-1",
		OwnerUserId:  "user-1",
		Query:        "how does this work",
	}
	if err := validateAIKnowledgeBaseQueryByAICommand("node-1", command); err == nil {
		t.Fatal("expected scoped query command without knowledge_base_id to fail validation")
	}
}

func TestValidateAIKnowledgeBaseQuestionIndexCommandAllowsKnowledgeBaseName(t *testing.T) {
	command := &aiv1.GenerateAIKnowledgeBaseQuestionIndexCommand{
		Metadata:          &nodev1.CommandMetadata{CommandId: "cmd-kb-question-index"},
		TargetNodeId:      "node-1",
		OwnerUserId:       "user-1",
		KnowledgeBaseName: "default-kb",
	}
	if err := validateAIKnowledgeBaseQuestionIndexCommand("node-1", command); err != nil {
		t.Fatalf("expected question-index command to validate, got %v", err)
	}
}

func TestValidateAIKnowledgeBaseQuestionIndexCommandRequiresTarget(t *testing.T) {
	command := &aiv1.GenerateAIKnowledgeBaseQuestionIndexCommand{
		Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-kb-question-index"},
		TargetNodeId: "node-1",
		OwnerUserId:  "user-1",
	}
	if err := validateAIKnowledgeBaseQuestionIndexCommand("node-1", command); err == nil {
		t.Fatal("expected question-index command without target to fail validation")
	}
}

func TestValidateAIKnowledgeBaseEntryUpdateCommandRequiresHiddenIndex(t *testing.T) {
	command := &aiv1.UpdateAIKnowledgeBaseEntryCommand{
		Metadata:             &nodev1.CommandMetadata{CommandId: "cmd-kb-entry-update"},
		TargetNodeId:         "node-1",
		OwnerUserId:          "user-1",
		KnowledgeBaseId:      42,
		KnowledgeBaseEntryId: 7,
		KnowledgeTitle:       "entry-title",
	}
	if err := validateAIKnowledgeBaseEntryUpdateCommand("node-1", command); err == nil {
		t.Fatal("expected missing hidden index to fail validation")
	}
}

func TestValidateAIKnowledgeBaseVectorIndexBuildCommandRejectsInvalidDistanceType(t *testing.T) {
	command := &aiv1.BuildAIKnowledgeBaseVectorIndexCommand{
		Metadata:         &nodev1.CommandMetadata{CommandId: "cmd-kb-vector-index"},
		TargetNodeId:     "node-1",
		OwnerUserId:      "user-1",
		KnowledgeBaseId:  42,
		DistanceFuncType: "l2",
	}
	if err := validateAIKnowledgeBaseVectorIndexBuildCommand("node-1", command); err == nil {
		t.Fatal("expected invalid distance func type to fail validation")
	}
}

func TestValidateAIKnowledgeBaseEntryVectorIndexBuildCommandRejectsInvalidDistanceType(t *testing.T) {
	command := &aiv1.BuildAIKnowledgeBaseEntryVectorIndexCommand{
		Metadata:                      &nodev1.CommandMetadata{CommandId: "cmd-kb-entry-vector-index"},
		TargetNodeId:                  "node-1",
		OwnerUserId:                   "user-1",
		KnowledgeBaseId:               42,
		KnowledgeBaseEntryId:          7,
		KnowledgeBaseEntryHiddenIndex: "hidden-1",
		DistanceFuncType:              "l2",
	}
	if err := validateAIKnowledgeBaseEntryVectorIndexBuildCommand("node-1", command); err == nil {
		t.Fatal("expected invalid distance func type to fail validation")
	}
}

func TestAttachAIEventMetadataSupportsAIKnowledgeBaseEntryEvents(t *testing.T) {
	metadata := &nodev1.EventMetadata{EventId: "evt-ai-knowledge-base-entry"}
	messages := []proto.Message{
		&aiv1.AIKnowledgeBaseEntriesSearched{},
		&aiv1.AIKnowledgeBaseEntriesSearchFailed{},
		&aiv1.AIKnowledgeBaseQueryByAIChunk{},
		&aiv1.AIKnowledgeBaseQueryByAICompleted{},
		&aiv1.AIKnowledgeBaseQueryByAIFailed{},
		&aiv1.AIKnowledgeBaseQuestionIndexProgress{},
		&aiv1.AIKnowledgeBaseQuestionIndexCompleted{},
		&aiv1.AIKnowledgeBaseQuestionIndexFailed{},
		&aiv1.AIKnowledgeBaseEntryCreated{},
		&aiv1.AIKnowledgeBaseEntryCreateFailed{},
		&aiv1.AIKnowledgeBaseEntryUpdated{},
		&aiv1.AIKnowledgeBaseEntryUpdateFailed{},
		&aiv1.AIKnowledgeBaseEntryDeleted{},
		&aiv1.AIKnowledgeBaseEntryDeleteFailed{},
	}

	for _, message := range messages {
		if err := attachAIEventMetadata(message, metadata); err != nil {
			t.Fatalf("attach metadata for %T: %v", message, err)
		}
		switch value := message.(type) {
		case *aiv1.AIKnowledgeBaseEntriesSearched:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseEntriesSearchFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseQueryByAIChunk:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseQueryByAICompleted:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseQueryByAIFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseQuestionIndexProgress:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseQuestionIndexCompleted:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseQuestionIndexFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseEntryCreated:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseEntryCreateFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseEntryUpdated:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseEntryUpdateFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseEntryDeleted:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseEntryDeleteFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		}
	}
}

func TestAttachAIEventMetadataSupportsAIKnowledgeBaseVectorIndexEvents(t *testing.T) {
	metadata := &nodev1.EventMetadata{EventId: "evt-ai-knowledge-base-vector-index"}
	messages := []proto.Message{
		&aiv1.AIKnowledgeBaseVectorIndexBuilt{},
		&aiv1.AIKnowledgeBaseVectorIndexBuildFailed{},
		&aiv1.AIKnowledgeBaseEntryVectorIndexBuilt{},
		&aiv1.AIKnowledgeBaseEntryVectorIndexBuildFailed{},
	}

	for _, message := range messages {
		if err := attachAIEventMetadata(message, metadata); err != nil {
			t.Fatalf("attach metadata for %T: %v", message, err)
		}
		switch value := message.(type) {
		case *aiv1.AIKnowledgeBaseVectorIndexBuilt:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseVectorIndexBuildFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseEntryVectorIndexBuilt:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseEntryVectorIndexBuildFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		}
	}
}
