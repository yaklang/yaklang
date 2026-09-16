package aimem

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
)

func TestMemoryFeedbackIndexAccounting(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprint(disabled), func(t *testing.T) {
			invoker := mock.NewMockInvoker(context.Background())
			var mu sync.Mutex
			var indexEvents []map[string]any
			invoker.GetConfig().(*mock.MockedAIConfig).Emitter = aicommon.NewEmitter("test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
				if e.NodeId == "memory-index" {
					var payload map[string]any
					require.NoError(t, json.Unmarshal(e.Content, &payload))
					mu.Lock()
					indexEvents = append(indexEvents, payload)
					mu.Unlock()
				}
				return e, nil
			})
			mem, err := CreateTestAIMemory(uuid.NewString(), WithInvoker(invoker), WithRAGOptions(rag.WithEmbeddingClient(vectorstore.NewMockEmbedder(func(text string) ([]float32, error) {
				if text == "fail" {
					return nil, fmt.Errorf("embedding unavailable")
				}
				return []float32{1, 0}, nil
			}))))
			require.NoError(t, err)
			defer mem.Close()
			originalRAG := mem.rag
			if disabled {
				mem.rag = nil
				defer func() { mem.rag = originalRAG }()
			}
			entity := &aicommon.MemoryEntity{Id: uuid.NewString(), Content: "persist despite index failure", PotentialQuestions: []string{"one", "fail", "two", "", " ", "three"}, CorePactVector: []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7}}
			require.NoError(t, mem.SaveMemoryEntities(entity))
			var stored schema.AIMemoryEntity
			require.NoError(t, mem.db.Table(mem.entityTableName()).Where("memory_id = ?", entity.Id).First(&stored).Error)
			require.Equal(t, entity.Content, stored.Content)
			invoker.GetConfig().GetEmitter().WaitForStream()
			mu.Lock()
			defer mu.Unlock()
			require.Len(t, indexEvents, 1)
			event := indexEvents[0]
			require.Equal(t, true, event["entity_saved"])
			require.EqualValues(t, 6, event["index_requested"])
			if disabled {
				require.EqualValues(t, 0, event["index_succeeded"])
				require.EqualValues(t, 0, event["index_failed"])
				require.EqualValues(t, 6, event["index_skipped"])
			} else {
				require.EqualValues(t, 3, event["index_succeeded"])
				require.EqualValues(t, 1, event["index_failed"])
				require.EqualValues(t, 2, event["index_skipped"])
			}
		})
	}
}
