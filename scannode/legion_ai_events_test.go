package scannode

import (
	"testing"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/proto"
)

func TestAttachAIEventMetadataSupportsGlobalConfigEvents(t *testing.T) {
	metadata := &nodev1.EventMetadata{EventId: "evt-global-config"}
	messages := []proto.Message{
		&aiv1.AIGlobalConfigFetched{},
		&aiv1.AIGlobalConfigFetchFailed{},
		&aiv1.AIGlobalConfigUpdated{},
		&aiv1.AIGlobalConfigUpdateFailed{},
	}

	for _, message := range messages {
		if err := attachAIEventMetadata(message, metadata); err != nil {
			t.Fatalf("attach metadata for %T: %v", message, err)
		}
		switch value := message.(type) {
		case *aiv1.AIGlobalConfigFetched:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIGlobalConfigFetchFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIGlobalConfigUpdated:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIGlobalConfigUpdateFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		}
	}
}

func TestAttachAIEventMetadataSupportsAIFocusEvents(t *testing.T) {
	metadata := &nodev1.EventMetadata{EventId: "evt-ai-focus"}
	messages := []proto.Message{
		&aiv1.AIFocusQueried{},
		&aiv1.AIFocusQueryFailed{},
	}

	for _, message := range messages {
		if err := attachAIEventMetadata(message, metadata); err != nil {
			t.Fatalf("attach metadata for %T: %v", message, err)
		}
		switch value := message.(type) {
		case *aiv1.AIFocusQueried:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIFocusQueryFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		}
	}
}

func TestAttachAIEventMetadataSupportsAIMaterialsEvents(t *testing.T) {
	metadata := &nodev1.EventMetadata{EventId: "evt-ai-materials"}
	messages := []proto.Message{
		&aiv1.AIMaterialsRandomQueried{},
		&aiv1.AIMaterialsRandomQueryFailed{},
	}

	for _, message := range messages {
		if err := attachAIEventMetadata(message, metadata); err != nil {
			t.Fatalf("attach metadata for %T: %v", message, err)
		}
		switch value := message.(type) {
		case *aiv1.AIMaterialsRandomQueried:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIMaterialsRandomQueryFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		}
	}
}

func TestAttachAIEventMetadataSupportsSessionHistoryEvents(t *testing.T) {
	metadata := &nodev1.EventMetadata{EventId: "evt-session-history"}
	messages := []proto.Message{
		&aiv1.AISessionTitleUpdated{},
		&aiv1.AISessionTitleUpdateFailed{},
		&aiv1.AISessionDeleteCompleted{},
		&aiv1.AISessionDeleteFailed{},
	}

	for _, message := range messages {
		if err := attachAIEventMetadata(message, metadata); err != nil {
			t.Fatalf("attach metadata for %T: %v", message, err)
		}
		switch value := message.(type) {
		case *aiv1.AISessionTitleUpdated:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AISessionTitleUpdateFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AISessionDeleteCompleted:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AISessionDeleteFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		}
	}
}

func TestAttachAIEventMetadataSupportsAIToolEvents(t *testing.T) {
	metadata := &nodev1.EventMetadata{EventId: "evt-ai-tool-events"}
	messages := []proto.Message{
		&aiv1.AIToolCreated{},
		&aiv1.AIToolCreateFailed{},
		&aiv1.AIToolUpdated{},
		&aiv1.AIToolUpdateFailed{},
		&aiv1.AIToolMetadataGenerated{},
		&aiv1.AIToolMetadataGenerateFailed{},
	}

	for _, message := range messages {
		if err := attachAIEventMetadata(message, metadata); err != nil {
			t.Fatalf("attach metadata for %T: %v", message, err)
		}
		switch value := message.(type) {
		case *aiv1.AIToolCreated:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIToolCreateFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIToolUpdated:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIToolUpdateFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIToolMetadataGenerated:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIToolMetadataGenerateFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		}
	}
}

func TestAttachAIEventMetadataSupportsAIForgeEvents(t *testing.T) {
	metadata := &nodev1.EventMetadata{EventId: "evt-ai-forge-events"}
	messages := []proto.Message{
		&aiv1.AIForgesListed{},
		&aiv1.AIForgesListFailed{},
		&aiv1.AIForgeCreated{},
		&aiv1.AIForgeCreateFailed{},
		&aiv1.AIForgeUpdated{},
		&aiv1.AIForgeUpdateFailed{},
		&aiv1.AIForgeDeleted{},
		&aiv1.AIForgeDeleteFailed{},
		&aiv1.AIForgeExportProgressed{},
		&aiv1.AIForgeExported{},
		&aiv1.AIForgeExportFailed{},
		&aiv1.AIForgeImportProgressed{},
		&aiv1.AIForgeImported{},
		&aiv1.AIForgeImportFailed{},
	}

	for _, message := range messages {
		if err := attachAIEventMetadata(message, metadata); err != nil {
			t.Fatalf("attach metadata for %T: %v", message, err)
		}
		switch value := message.(type) {
		case *aiv1.AIForgesListed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgesListFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgeCreated:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgeCreateFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgeUpdated:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgeUpdateFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgeDeleted:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgeDeleteFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgeExportProgressed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgeExported:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgeExportFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgeImportProgressed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgeImported:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIForgeImportFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		}
	}
}

func TestAttachAIEventMetadataSupportsAIKnowledgeBaseImportEvents(t *testing.T) {
	metadata := &nodev1.EventMetadata{EventId: "evt-ai-knowledge-base-import"}
	messages := []proto.Message{
		&aiv1.AIKnowledgeBaseImported{},
		&aiv1.AIKnowledgeBaseImportFailed{},
	}

	for _, message := range messages {
		if err := attachAIEventMetadata(message, metadata); err != nil {
			t.Fatalf("attach metadata for %T: %v", message, err)
		}
		switch value := message.(type) {
		case *aiv1.AIKnowledgeBaseImported:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIKnowledgeBaseImportFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		}
	}
}

func TestAttachAIEventMetadataSupportsAIMemoryEvents(t *testing.T) {
	metadata := &nodev1.EventMetadata{EventId: "evt-ai-memory"}
	messages := []proto.Message{
		&aiv1.AIMemoryEntityCreated{},
		&aiv1.AIMemoryEntityCreateFailed{},
		&aiv1.AIMemoryEntityFetched{},
		&aiv1.AIMemoryEntityFetchFailed{},
		&aiv1.AIMemoryEntitiesQueried{},
		&aiv1.AIMemoryEntitiesQueryFailed{},
		&aiv1.AIMemoryEntityUpdated{},
		&aiv1.AIMemoryEntityUpdateFailed{},
		&aiv1.AIMemoryEntitiesDeleted{},
		&aiv1.AIMemoryEntitiesDeleteFailed{},
		&aiv1.AIMemoryEntityTagsCounted{},
		&aiv1.AIMemoryEntityTagsCountFailed{},
	}

	for _, message := range messages {
		if err := attachAIEventMetadata(message, metadata); err != nil {
			t.Fatalf("attach metadata for %T: %v", message, err)
		}
		switch value := message.(type) {
		case *aiv1.AIMemoryEntityCreated:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIMemoryEntityCreateFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIMemoryEntityFetched:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIMemoryEntityFetchFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIMemoryEntitiesQueried:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIMemoryEntitiesQueryFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIMemoryEntityUpdated:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIMemoryEntityUpdateFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIMemoryEntitiesDeleted:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIMemoryEntitiesDeleteFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIMemoryEntityTagsCounted:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AIMemoryEntityTagsCountFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		}
	}
}

func TestAttachAIEventMetadataSupportsAILogsEvents(t *testing.T) {
	metadata := &nodev1.EventMetadata{EventId: "evt-ai-logs"}
	messages := []proto.Message{
		&aiv1.AILogsCheckpointsExported{},
		&aiv1.AILogsCheckpointsExportFailed{},
	}

	for _, message := range messages {
		if err := attachAIEventMetadata(message, metadata); err != nil {
			t.Fatalf("attach metadata for %T: %v", message, err)
		}
		switch value := message.(type) {
		case *aiv1.AILogsCheckpointsExported:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		case *aiv1.AILogsCheckpointsExportFailed:
			if value.GetMetadata() != metadata {
				t.Fatalf("expected metadata to be attached for %T", message)
			}
		}
	}
}
