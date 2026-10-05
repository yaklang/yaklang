package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
)

type timelinePersistenceMemory struct {
	aicommon.MemoryTriage
	store       *aimem.MemoryStore
	db          *gorm.DB
	calls       atomic.Int32
	recoverOnce atomic.Bool
}

func (m *timelinePersistenceMemory) SaveMemoryEntities(entities ...*aicommon.MemoryEntity) error {
	return fmt.Errorf("unexpected legacy best-effort memory save")
}

func (m *timelinePersistenceMemory) PersistTimelineMemories(ctx context.Context, candidates []any) error {
	if len(candidates) > 0 {
		m.calls.Add(1)
	}
	err := m.store.PersistTimelineMemories(ctx, candidates)
	if err != nil && m.recoverOnce.Swap(false) {
		if dropErr := m.db.Exec(`DROP TRIGGER fail_memory`).Error; dropErr != nil {
			return dropErr
		}
	}
	return err
}

func timelinePersistenceCandidate() map[string]any {
	return map[string]any{"content": "用户明确要求报告附实际验证范围。",
		"tags": []string{"报告"}, "potential_questions": []string{"报告需附哪些验证信息？"},
		"scores": map[string]any{"connectivity": 0.2, "origin": 1.0, "relevance": 0.8,
			"emotion": 0.0, "perference": 1.0, "actionability": 0.8, "temporality": 0.9}}
}

func (m *timelinePersistenceMemory) AddRawText(string) ([]*aicommon.MemoryEntity, error) {
	return nil, fmt.Errorf("unexpected second memory extraction")
}

// Real compression parsing, runtime registration, SQLite/TTL and both indexes.
// Only the model response and embedding vector are controlled by this fixture.
func TestTimelineMemoryPersistenceRuntimeAndTaskForks(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, path := range []string{"default", "coordinator-fork", "task-only", "restored", "empty", "invalid-memory", "retry", "recover-pending", "recover-history"} {
			t.Run(fmt.Sprintf("native=%v/%s", native, path), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "memory.db"))
				require.NoError(t, err)
				defer db.Close()
				require.NoError(t, db.AutoMigrate(&schema.AIMemoryEntity{}, &schema.AIMemoryCollection{},
					&schema.ProjectGeneralStorage{}, &schema.VectorStoreCollection{}, &schema.VectorStoreDocument{},
					&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{}, &schema.ERModelEntity{},
					&schema.ERModelRelationship{}, &schema.EntityRepository{}).Error)
				namespace := "compression-" + uuid.NewString()
				store, err := aimem.NewMemoryStore(namespace, aimem.WithDatabase(db),
					aimem.WithRAGOptions(rag.WithModelDimension(2), rag.WithEmbeddingClient(vectorstore.NewMockEmbedder(func(string) ([]float32, error) {
						return []float32{1, 0}, nil
					}))))
				require.NoError(t, err)
				defer store.Close()
				memory := &timelinePersistenceMemory{MemoryTriage: aimem.NewMockMemoryTriage(), store: store, db: db}
				parentTimeline := aicommon.NewTimeline(nil, nil)
				if path == "retry" || path == "recover-pending" {
					require.NoError(t, db.Exec(`CREATE TRIGGER fail_memory BEFORE INSERT ON ai_memory_entities_v1 BEGIN SELECT RAISE(ABORT, 'injected runtime write failure'); END`).Error)
					if path == "retry" {
						memory.recoverOnce.Store(true)
					} else {
						require.Error(t, store.PersistTimelineMemories(ctx, []any{timelinePersistenceCandidate()}))
						require.NoError(t, db.Exec(`DROP TRIGGER fail_memory`).Error)
					}
				}
				if path == "recover-history" {
					raw, err := aicommon.MarshalTimeline(parentTimeline)
					require.NoError(t, err)
					var archive map[string]any
					require.NoError(t, json.Unmarshal([]byte(raw), &archive))
					archive["session_memory_candidates"] = []any{timelinePersistenceCandidate()}
					encoded, err := json.Marshal(archive)
					require.NoError(t, err)
					parentTimeline, err = aicommon.UnmarshalTimeline(string(encoded))
					require.NoError(t, err)
				}
				if path == "restored" {
					parentTimeline.PushText(1, "persisted historical observation")
					raw, err := aicommon.MarshalTimeline(parentTimeline)
					require.NoError(t, err)
					parentTimeline, err = aicommon.UnmarshalTimeline(raw)
					require.NoError(t, err)
				}
				var requests atomic.Int32
				callback := func(_ aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if request.GetCallerLabel() != aicommon.CallerLabelTimelineCompress {
						return nil, fmt.Errorf("unexpected model request: %s", request.GetCallerLabel())
					}
					requests.Add(1)
					candidate := timelinePersistenceCandidate()
					entities := []any{candidate}
					if path == "empty" {
						entities = []any{}
					}
					if path == "invalid-memory" {
						candidate["scores"].(map[string]any)["actionability"] = -1.0
					}
					payload, err := json.Marshal(map[string]any{"@action": "timeline-summary", "summary": "验证完成，保留持续偏好。",
						"ratain_timeline_item_range": "", "memory_entities": entities})
					if err != nil {
						return nil, err
					}
					response := aicommon.NewUnboundAIResponse()
					response.EmitOutputStream(strings.NewReader(string(payload)))
					response.Close()
					return response, nil
				}
				parent, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
					aicommon.WithTimeline(parentTimeline), aicommon.WithMemoryTriage(memory),
					aicommon.WithDisableCreateDBRuntime(true), aicommon.WithEnableFunctionCallMode(native),
					aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1), aicommon.WithSpeedPriorityAICallback(callback))
				require.NoError(t, err)
				tl := parent.config.Timeline
				if path == "recover-pending" || path == "recover-history" {
					before, err := aicommon.MarshalTimeline(tl)
					require.NoError(t, err)
					require.Eventually(t, func() bool {
						var rows []schema.VectorStoreDocument
						return db.Find(&rows).Error == nil && len(rows) == 1
					}, 5*time.Second, 10*time.Millisecond, "runtime registration must recover without another compression")
					require.Zero(t, requests.Load(), "recovery must not ask the model again")
					after, err := aicommon.MarshalTimeline(tl)
					require.NoError(t, err)
					require.Equal(t, before, after, "storage acknowledgements must not enter the prompt/archive")
				}
				if path == "coordinator-fork" || path == "task-only" {
					fork, err := tl.ForkForTask("1", "worker", parent.config, parent.config)
					require.NoError(t, err)
					child, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
						aicommon.WithTimeline(fork.Branch), aicommon.WithMemoryTriage(memory),
						aicommon.WithDisableMemoryTriage(path == "task-only"), aicommon.WithDisableCreateDBRuntime(true),
						aicommon.WithEnableFunctionCallMode(native), aicommon.WithAIAutoRetry(1),
						aicommon.WithAITransactionAutoRetry(1), aicommon.WithSpeedPriorityAICallback(callback))
					require.NoError(t, err)
					tl = child.config.Timeline
				}
				// Rebinding the same owner replaces one callback, never duplicates it.
				aimem.RegisterTimelineMemoryPersistence(parent.config.Timeline, memory)
				completed := make(chan aicommon.TimelineMemoryEvent, 1)
				parent.config.Timeline.RegisterSessionMemoryCallback("test.receipt", func(event aicommon.TimelineMemoryEvent) { completed <- event })
				tl.PushText(10, "用户要求后续工程报告附实际验证范围。")
				_, err = tl.CompressOnce(aicommon.TimelineCompressionOptions{Context: ctx, MaxInputTokens: 100000, MaxSummaryTokens: 4096})
				require.NoError(t, err)
				select {
				case event := <-completed:
					require.NoError(t, event.Err)
				case <-ctx.Done():
					t.Fatal("session memory notification did not complete")
				}
				if path == "empty" || path == "invalid-memory" {
					require.Zero(t, memory.calls.Load())
					var count int
					require.NoError(t, db.Model(&schema.AIMemoryEntity{}).Where("session_id = ?", namespace).Count(&count).Error)
					require.Zero(t, count)
					require.EqualValues(t, 1, requests.Load())
					return
				}
				{
					var rows []schema.AIMemoryEntity
					require.NoError(t, db.Where("session_id = ?", namespace).Find(&rows).Error)
					require.Len(t, rows, 1)
					row := rows[0]
					require.Equal(t, "用户明确要求报告附实际验证范围。", row.Content)
					require.Nil(t, row.ExpiresAt, "long-term score keeps the existing TTL policy")
					require.Equal(t, 0.0, row.E_Score)
					require.Equal(t, 1.0, row.P_Score)
					var documents []schema.VectorStoreDocument
					require.NoError(t, db.Where("document_id = ?", row.QuestionHashID(row.PotentialQuestions[0])).Find(&documents).Error)
					require.Len(t, documents, 1, "potential question must reach the semantic index")
				}
				if path == "retry" {
					require.GreaterOrEqual(t, memory.calls.Load(), int32(2))
				}
				require.EqualValues(t, 1, requests.Load(), "persistence must not call an auxiliary extractor")
			})
		}
	}
}
