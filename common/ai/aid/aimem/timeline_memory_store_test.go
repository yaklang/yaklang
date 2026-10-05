package aimem

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
)

func reliableMemoryCandidate(content string, questions ...string) map[string]any {
	return map[string]any{"content": content, "tags": []string{"验证"}, "potential_questions": questions,
		"scores": map[string]float64{"connectivity": 0.2, "origin": 1, "relevance": 0.8,
			"emotion": 0, "perference": 0.8, "actionability": 0.8, "temporality": 0.5}}
}

func reliableMemoryStore(t *testing.T, db *gorm.DB, namespace string, embed func(string) ([]float32, error)) *MemoryStore {
	t.Helper()
	s, err := NewMemoryStore(namespace, WithDatabase(db), WithRAGOptions(rag.WithModelDimension(2), rag.WithEmbeddingClient(vectorstore.NewMockEmbedder(embed))))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func memoryReceipt(t *testing.T, s *MemoryStore, candidate any) timelineMemoryReceipt {
	t.Helper()
	entity, err := MemoryEntityFromCompression(candidate)
	require.NoError(t, err)
	var row schema.ProjectGeneralStorage
	require.NoError(t, s.db.Where("key = ?", strconv.Quote("aimemory-timeline-v1/"+timelineMemoryID(s.sessionID, entity))).First(&row).Error)
	value, err := strconv.Unquote(row.Value)
	require.NoError(t, err)
	var receipt timelineMemoryReceipt
	require.NoError(t, json.Unmarshal([]byte(value), &receipt))
	return receipt
}

func countMemoryRows(t *testing.T, s *MemoryStore, expected int) {
	t.Helper()
	var count int
	require.NoError(t, s.db.Model(&schema.AIMemoryEntity{}).Where("session_id = ?", s.sessionID).Count(&count).Error)
	require.Equal(t, expected, count)
}

func TestTimelineMemoryPersistenceConcurrentDuplicateAndNamespace(t *testing.T) {
	db, err := getTestDatabase(t)
	require.NoError(t, err)
	var embeddings atomic.Int32
	embed := func(text string) ([]float32, error) {
		if text == "what was verified?" {
			embeddings.Add(1)
		}
		return []float32{1, 0}, nil
	}
	s := reliableMemoryStore(t, db, uuid.NewString(), embed)
	candidate := reliableMemoryCandidate("verified local source", "what was verified?")
	var wg sync.WaitGroup
	errors := make(chan error, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- s.PersistTimelineMemories(context.Background(), []any{candidate, candidate})
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	countMemoryRows(t, s, 1)
	require.EqualValues(t, 1, embeddings.Load(), "duplicate notifications must not repeat successful embedding calls")
	before := memoryReceipt(t, s, candidate)
	require.True(t, before.Complete)
	// Deprecated/irrelevant response fields must not change the durable identity.
	candidate["title"], candidate["extra"] = "ignored", "ignored"
	require.NoError(t, s.PersistTimelineMemories(context.Background(), []any{candidate}))
	require.Equal(t, before, memoryReceipt(t, s, candidate), "replay must not extend TTL or creation time")
	other := reliableMemoryStore(t, db, uuid.NewString(), embed)
	require.NoError(t, other.PersistTimelineMemories(context.Background(), []any{candidate}))
	require.NotEqual(t, before.Entity.Id, memoryReceipt(t, other, candidate).Entity.Id)
	countMemoryRows(t, other, 1)
}

func TestTimelineMemoryPersistenceStaleBackendsKeepAllScoreNodes(t *testing.T) {
	db, err := getTestDatabase(t)
	require.NoError(t, err)
	embed := func(string) ([]float32, error) { return []float32{1, 0}, nil }
	a := reliableMemoryStore(t, db, uuid.NewString(), embed)
	b := reliableMemoryStore(t, db, a.sessionID, embed)
	require.NoError(t, a.PersistTimelineMemories(context.Background(), []any{reliableMemoryCandidate("first backend")}))
	require.NoError(t, b.PersistTimelineMemories(context.Background(), []any{reliableMemoryCandidate("second backend")}))
	require.NoError(t, a.Close(), "a delayed flush of an old backend must not discard newer persisted nodes")
	c := reliableMemoryStore(t, db, a.sessionID, embed)
	results, err := c.hnswBackend.Search([]float32{0.2, 1, 0.8, 0, 0.8, 0.8, 0.5}, 10)
	require.NoError(t, err)
	require.Len(t, results, 2)
}

func TestTimelineMemoryPersistenceAdoptsExistingRandomIDs(t *testing.T) {
	db, err := getTestDatabase(t)
	require.NoError(t, err)
	s := reliableMemoryStore(t, db, uuid.NewString(), func(string) ([]float32, error) { return []float32{1, 0}, nil })
	candidate := reliableMemoryCandidate("already saved before upgrade", "old question")
	entity, err := MemoryEntityFromCompression(candidate)
	require.NoError(t, err)
	require.NoError(t, s.SaveMemoryEntities(entity))
	var old schema.AIMemoryEntity
	require.NoError(t, db.Where("memory_id = ?", entity.Id).First(&old).Error)
	require.NoError(t, s.PersistTimelineMemories(context.Background(), []any{candidate}))
	countMemoryRows(t, s, 1)
	receipt := memoryReceipt(t, s, candidate)
	require.True(t, receipt.Complete)
	require.Equal(t, old.MemoryID, receipt.Entity.Id)
	require.True(t, old.CreatedAt.Equal(receipt.Entity.CreatedAt))
	require.True(t, old.ExpiresAt.Equal(*receipt.Entity.ExpiresAt))
}

func TestTimelineMemoryPersistenceStaleCleanupKeepsNewScoreNodes(t *testing.T) {
	db, err := getTestDatabase(t)
	require.NoError(t, err)
	embed := func(string) ([]float32, error) { return []float32{1, 0}, nil }
	a := reliableMemoryStore(t, db, uuid.NewString(), embed)
	first := reliableMemoryCandidate("before cleanup")
	require.NoError(t, a.PersistTimelineMemories(context.Background(), []any{first}))
	b := reliableMemoryStore(t, db, a.sessionID, embed)
	second := reliableMemoryCandidate("saved by another backend")
	require.NoError(t, b.PersistTimelineMemories(context.Background(), []any{second}))
	require.NoError(t, a.hnswBackend.deleteAndSave(context.Background(), []string{memoryReceipt(t, a, first).Entity.Id}, true))
	c := reliableMemoryStore(t, db, a.sessionID, embed)
	results, err := c.hnswBackend.Search([]float32{0.2, 1, 0.8, 0, 0.8, 0.8, 0.5}, 10)
	require.NoError(t, err)
	require.Len(t, results, 1, "cleanup of an older snapshot must retain newer memories")
	require.Equal(t, memoryReceipt(t, b, second).Entity.Id, results[0].Entity.Id)
}

func TestTimelineMemoryPersistencePartialDBFailureAndRestart(t *testing.T) {
	db, err := getTestDatabase(t)
	require.NoError(t, err)
	embed := func(string) ([]float32, error) { return []float32{1, 0}, nil }
	s := reliableMemoryStore(t, db, uuid.NewString(), embed)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_memory BEFORE INSERT ON ai_memory_entities_v1 WHEN NEW.content = 'fail' BEGIN SELECT RAISE(ABORT, 'injected entity failure'); END`).Error)
	good, bad := reliableMemoryCandidate("ok", "good question"), reliableMemoryCandidate("fail", "failed question")
	require.ErrorContains(t, s.PersistTimelineMemories(context.Background(), []any{bad, good}), "injected entity failure")
	countMemoryRows(t, s, 1)
	require.True(t, memoryReceipt(t, s, good).Complete)
	require.False(t, memoryReceipt(t, s, bad).Saved)
	require.NoError(t, db.Exec(`DROP TRIGGER fail_memory`).Error)
	restored := reliableMemoryStore(t, db, s.sessionID, embed)
	require.NoError(t, restored.PersistTimelineMemories(context.Background(), nil))
	countMemoryRows(t, restored, 2)
	require.True(t, memoryReceipt(t, restored, bad).Complete)
	require.NoError(t, restored.PersistTimelineMemories(context.Background(), []any{bad, good}))
	countMemoryRows(t, restored, 2)
}

func TestTimelineMemoryPersistencePartialQuestionsAndRestart(t *testing.T) {
	db, err := getTestDatabase(t)
	require.NoError(t, err)
	var fail atomic.Bool
	fail.Store(true)
	var mu sync.Mutex
	calls := map[string]int{}
	embed := func(text string) ([]float32, error) {
		mu.Lock()
		calls[text]++
		mu.Unlock()
		if text == "bad question" && fail.Load() {
			return nil, fmt.Errorf("injected embedding failure")
		}
		return []float32{1, 0}, nil
	}
	s := reliableMemoryStore(t, db, uuid.NewString(), embed)
	candidate := reliableMemoryCandidate("source", "first question", "bad question", "last question")
	require.ErrorContains(t, s.PersistTimelineMemories(context.Background(), []any{candidate}), "injected embedding failure")
	before := memoryReceipt(t, s, candidate)
	require.True(t, before.Saved)
	require.True(t, before.Score)
	require.False(t, before.Complete)
	require.Len(t, before.Questions, 2, "a failed question must not prevent later questions being saved")
	fail.Store(false)
	restored := reliableMemoryStore(t, db, s.sessionID, embed)
	require.NoError(t, restored.PersistTimelineMemories(context.Background(), nil))
	after := memoryReceipt(t, restored, candidate)
	require.True(t, after.Complete)
	require.Equal(t, before.Entity, after.Entity, "retry must preserve immutable entity and TTL")
	countMemoryRows(t, restored, 1)
	mu.Lock()
	require.Equal(t, 1, calls["first question"])
	require.Equal(t, 1, calls["last question"])
	require.Equal(t, 2, calls["bad question"])
	mu.Unlock()
	results, err := restored.hnswBackend.Search(after.Entity.CorePactVector, 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	for _, q := range after.Entity.PotentialQuestions {
		var doc schema.VectorStoreDocument
		row := memoryEntityRow(s.sessionID, after.Entity)
		require.NoError(t, db.Where("document_id = ?", row.QuestionHashID(q)).First(&doc).Error)
	}
}

func TestTimelineMemoryPersistenceCheckpointAndScoreFailure(t *testing.T) {
	for _, stage := range []string{"receipt", "score", "question-graph"} {
		t.Run(stage, func(t *testing.T) {
			db, err := getTestDatabase(t)
			require.NoError(t, err)
			var embeds atomic.Int32
			s := reliableMemoryStore(t, db, uuid.NewString(), func(text string) ([]float32, error) {
				if text == "persisted vector question" {
					embeds.Add(1)
				}
				return []float32{1, 0}, nil
			})
			trigger := `CREATE TRIGGER fail_stage BEFORE UPDATE ON project_general_storages BEGIN SELECT RAISE(ABORT, 'injected receipt failure'); END`
			if stage == "score" {
				trigger = `CREATE TRIGGER fail_stage BEFORE UPDATE OF graph_binary ON ai_memory_collections_v1 BEGIN SELECT RAISE(ABORT, 'injected score failure'); END`
			}
			if stage == "question-graph" {
				trigger = fmt.Sprintf(`CREATE TRIGGER fail_stage BEFORE UPDATE OF graph_binary ON %s BEGIN SELECT RAISE(ABORT, 'injected question graph failure'); END`, (&schema.VectorStoreCollection{}).TableName())
			}
			require.NoError(t, db.Exec(trigger).Error)
			candidate := reliableMemoryCandidate("checkpoint source")
			if stage == "question-graph" {
				candidate = reliableMemoryCandidate("checkpoint source", "persisted vector question")
			}
			require.Error(t, s.PersistTimelineMemories(context.Background(), []any{candidate}))
			countMemoryRows(t, s, 1)
			receipt := memoryReceipt(t, s, candidate)
			require.False(t, receipt.Complete)
			require.Equal(t, stage != "receipt", receipt.Saved)
			require.Equal(t, stage == "question-graph", receipt.Score)
			require.NoError(t, db.Exec(`DROP TRIGGER fail_stage`).Error)
			require.NoError(t, s.PersistTimelineMemories(context.Background(), nil))
			countMemoryRows(t, s, 1)
			require.True(t, memoryReceipt(t, s, candidate).Complete)
			if stage == "question-graph" {
				require.EqualValues(t, 1, embeds.Load(), "graph repair must reuse the persisted document vector")
			}
		})
	}
}

func TestTimelineMemoryPersistenceStaleReceiptCannotUndoCompletion(t *testing.T) {
	db, err := getTestDatabase(t)
	require.NoError(t, err)
	s := reliableMemoryStore(t, db, uuid.NewString(), func(string) ([]float32, error) { return []float32{1, 0}, nil })
	candidate := reliableMemoryCandidate("stale checkpoint")
	entity, err := MemoryEntityFromCompression(candidate)
	require.NoError(t, err)
	entity.Id = timelineMemoryID(s.sessionID, entity)
	require.NoError(t, s.enqueueTimelineMemory(entity))
	var stale schema.ProjectGeneralStorage
	require.NoError(t, db.Where("key = ?", strconv.Quote("aimemory-timeline-v1/"+entity.Id)).First(&stale).Error)
	require.NoError(t, s.PersistTimelineMemories(context.Background(), nil))
	receipt := memoryReceipt(t, s, candidate)
	receipt.Complete = false
	require.ErrorContains(t, s.checkpointTimelineMemory(&stale, &receipt), "changed concurrently")
	require.True(t, memoryReceipt(t, s, candidate).Complete)
}

func TestTimelineMemoryPersistenceCancellationAndPagination(t *testing.T) {
	db, err := getTestDatabase(t)
	require.NoError(t, err)
	s := reliableMemoryStore(t, db, uuid.NewString(), func(string) ([]float32, error) { return []float32{1, 0}, nil })
	ctx, cancel := context.WithCancel(context.Background())
	db.Callback().Create().After("gorm:create").Register("cancel_after_journal", func(scope *gorm.Scope) {
		if _, ok := scope.Value.(*schema.ProjectGeneralStorage); ok {
			cancel()
		}
	})
	candidate := reliableMemoryCandidate("cancelled after outbox commit")
	require.ErrorIs(t, s.PersistTimelineMemories(ctx, []any{candidate}), context.Canceled)
	db.Callback().Create().Remove("cancel_after_journal")
	countMemoryRows(t, s, 0)
	require.False(t, memoryReceipt(t, s, candidate).Saved)
	// Recovery can drain an outbox without any Timeline or new model request.
	require.NoError(t, s.PersistTimelineMemories(context.Background(), nil))
	countMemoryRows(t, s, 1)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_first BEFORE INSERT ON ai_memory_entities_v1 WHEN NEW.content = 'page-0' BEGIN SELECT RAISE(ABORT, 'injected page failure'); END`).Error)
	var candidates []any
	for i := 0; i < 70; i++ {
		candidates = append(candidates, reliableMemoryCandidate(fmt.Sprintf("page-%d", i)))
	}
	require.Error(t, s.PersistTimelineMemories(context.Background(), candidates))
	countMemoryRows(t, s, 70)
	require.True(t, memoryReceipt(t, s, candidates[69]).Complete)
	require.NoError(t, db.Exec(`DROP TRIGGER fail_first`).Error)
	require.NoError(t, s.PersistTimelineMemories(context.Background(), nil))
	countMemoryRows(t, s, 71)
}

func TestTimelineMemoryPersistenceDoesNotResurrectDeletedOrExpired(t *testing.T) {
	for _, mode := range []string{"deleted", "hard-deleted", "expired", "queued-expired"} {
		t.Run(mode, func(t *testing.T) {
			db, err := getTestDatabase(t)
			require.NoError(t, err)
			s := reliableMemoryStore(t, db, uuid.NewString(), func(string) ([]float32, error) { return []float32{1, 0}, nil })
			candidate := reliableMemoryCandidate("retired source", "pending question")
			// Leave an outstanding index repair so recovery really visits the item.
			s.rag = nil
			require.Error(t, s.PersistTimelineMemories(context.Background(), []any{candidate}))
			receipt := memoryReceipt(t, s, candidate)
			switch mode {
			case "deleted":
				require.NoError(t, db.Where("memory_id = ?", receipt.Entity.Id).Delete(&schema.AIMemoryEntity{}).Error)
			case "hard-deleted":
				require.NoError(t, db.Unscoped().Where("memory_id = ?", receipt.Entity.Id).Delete(&schema.AIMemoryEntity{}).Error)
			case "expired":
				require.NoError(t, db.Table("ai_memory_entities_v1").Where("memory_id = ?", receipt.Entity.Id).Update("expires_at", time.Now().Add(-time.Hour)).Error)
			case "queued-expired":
				require.NoError(t, db.Unscoped().Where("memory_id = ?", receipt.Entity.Id).Delete(&schema.AIMemoryEntity{}).Error)
				receipt.Saved = false
				past := time.Now().Add(-time.Hour)
				receipt.Entity.ExpiresAt = &past
				var row schema.ProjectGeneralStorage
				require.NoError(t, db.Where("key = ?", strconv.Quote("aimemory-timeline-v1/"+receipt.Entity.Id)).First(&row).Error)
				require.NoError(t, s.checkpointTimelineMemory(&row, &receipt))
			}
			restored := reliableMemoryStore(t, db, s.sessionID, func(string) ([]float32, error) { return []float32{1, 0}, nil })
			require.NoError(t, restored.PersistTimelineMemories(context.Background(), []any{candidate}))
			require.True(t, memoryReceipt(t, s, candidate).Complete)
			if mode == "expired" {
				var row schema.AIMemoryEntity
				require.NoError(t, db.Where("memory_id = ?", receipt.Entity.Id).First(&row).Error)
				require.True(t, row.ExpiresAt.Before(time.Now()))
			} else {
				countMemoryRows(t, restored, 0)
			}
		})
	}
}
