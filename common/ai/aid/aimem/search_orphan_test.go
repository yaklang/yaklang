package aimem

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
)

func orphanSearchMemory(t testing.TB, stale, live int) *AIMemoryTriage {
	t.Helper()
	return orphanSearchMemoryWithDimension(t, stale, live, 2)
}

func orphanSearchMemoryWithDimension(t testing.TB, stale, live, dimension int) *AIMemoryTriage {
	t.Helper()
	db := cleanupRegressionDB(t)
	_, _, err := claimCleanup(context.Background(), db, time.Now())
	require.NoError(t, err)
	embedder := vectorstore.NewMockEmbedder(func(input string) ([]float32, error) {
		vector := make([]float32, dimension)
		vector[0] = 1
		if strings.HasPrefix(input, "live:") {
			vector[1] = 1
		}
		return vector, nil
	})
	store, err := vectorstore.NewSQLiteVectorStoreHNSW(Session2MemoryName("default"), "", "mock", dimension, embedder, db,
		vectorstore.WithDisableEmbedCollectionInfo(true))
	require.NoError(t, err)
	var docs []*vectorstore.Document
	for i := 0; i < stale+live; i++ {
		id := fmt.Sprintf("memory-%03d", i)
		content := id
		if i >= stale {
			content = "live:" + id
			require.NoError(t, db.Create(&schema.AIMemoryEntity{MemoryID: id, SessionID: "default", Content: id}).Error)
		}
		docs = append(docs, &vectorstore.Document{ID: id, Content: content,
			Metadata: schema.MetadataMap{"memory_id": id, "session_id": "default"}})
	}
	require.NoError(t, store.Add(docs...))
	var collection schema.VectorStoreCollection
	require.NoError(t, db.Where("name = ?", store.GetName()).First(&collection).Error)
	vectorstore.GraphWrapperManager.RemoveCollectionFromCache(db, &collection)
	store, err = vectorstore.LoadCollection(db, store.GetName(), vectorstore.WithEmbeddingClient(embedder))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.Eventually(t, func() bool { return atomic.LoadInt32(&globalCoordinator.cleanupRunning) == 0 }, 3*time.Second, time.Millisecond)
		vectorstore.GraphWrapperManager.RemoveCollectionFromCache(db, &collection)
	})
	return &AIMemoryTriage{ctx: context.Background(), db: db, sessionID: "default", embeddingAvailable: true,
		rag: &rag.RAGSystem{VectorStore: store, Name: store.GetName()}}
}

func TestMUSTPASS_SemanticOrphanRepairRefillsAndPersists(t *testing.T) {
	mem := orphanSearchMemory(t, 3, 3)
	for i := 0; i < 3; i++ {
		results, err := mem.SearchBySemantics("你好？", 3)
		require.NoError(t, err)
		require.Len(t, results, 3, "orphan hits must not hide surviving memories")
		for _, result := range results {
			require.GreaterOrEqual(t, result.Entity.Id, "memory-003")
		}
	}
	for i := 0; i < 3; i++ {
		require.False(t, mem.rag.VectorStore.Has(fmt.Sprintf("memory-%03d", i)))
	}
	var collection schema.VectorStoreCollection
	require.NoError(t, mem.db.Where("name = ?", mem.rag.Name).First(&collection).Error)
	vectorstore.GraphWrapperManager.RemoveCollectionFromCache(mem.db, &collection)
	store, err := vectorstore.LoadCollection(mem.db, mem.rag.Name, vectorstore.WithEmbeddingClient(vectorstore.NewMockEmbedder(func(string) ([]float32, error) { return []float32{1, 0}, nil })))
	require.NoError(t, err)
	for i := 0; i < 6; i++ {
		require.Equal(t, i >= 3, store.Has(fmt.Sprintf("memory-%03d", i)))
	}
}

func TestMUSTPASS_SemanticOrphanIDOnlyAndEmptyGraph(t *testing.T) {
	mem := orphanSearchMemory(t, 32, 0)
	results, err := mem.SearchBySemanticsMemoryIDs("query", 64)
	require.NoError(t, err)
	require.Empty(t, results, "ID-only retrieval must not publish deleted memories")
	count, err := mem.rag.CountDocuments()
	require.NoError(t, err)
	require.Zero(t, count)
	var collection schema.VectorStoreCollection
	require.NoError(t, mem.db.Where("name = ?", mem.rag.Name).First(&collection).Error)
	require.Empty(t, collection.GraphBinary)
}

func TestMUSTPASS_SemanticOrphanFailedRepairRetries(t *testing.T) {
	mem := orphanSearchMemory(t, 2, 1)
	require.NoError(t, mem.db.Exec(`CREATE TRIGGER fail_orphan_save BEFORE UPDATE ON rag_vector_collection_v1
		BEGIN SELECT RAISE(ABORT, 'injected orphan save failure'); END`).Error)
	results, err := mem.SearchBySemantics("query", 4)
	require.NoError(t, err, "optional repair must not discard valid search results")
	require.Len(t, results, 1)
	require.True(t, mem.rag.VectorStore.Has("memory-000"), "failed repair must restore rows and graph")
	require.NoError(t, mem.db.Exec("DROP TRIGGER fail_orphan_save").Error)
	results, err = mem.SearchBySemantics("query", 4)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.False(t, mem.rag.VectorStore.Has("memory-000"))
}

func TestMUSTPASS_SemanticOrphanQueryFailureDoesNotDelete(t *testing.T) {
	mem := orphanSearchMemory(t, 2, 1)
	require.NoError(t, mem.db.Exec("ALTER TABLE ai_memory_entities_v1 RENAME TO unavailable_memory_entities").Error)
	_, err := mem.SearchBySemanticsMemoryIDs("query", 4)
	require.Error(t, err, "database failure must not be interpreted as missing entities")
	require.True(t, mem.rag.VectorStore.Has("memory-000"))
	require.True(t, mem.rag.VectorStore.Has("memory-002"))
	require.NoError(t, mem.db.Exec("ALTER TABLE unavailable_memory_entities RENAME TO ai_memory_entities_v1").Error)
}

func TestMUSTPASS_SemanticOrphanBacklogBounded(t *testing.T) {
	mem := orphanSearchMemory(t, 384, 16)
	var queries, saves atomic.Int32
	mem.db.Callback().Query().After("gorm:query").Register("test:bounded-memory-query", func(scope *gorm.Scope) {
		if scope.TableName() == mem.entityTableName() {
			queries.Add(1)
			require.LessOrEqual(t, len(scope.SQLVars), semanticSearchBatchSize+1, "SQL parameter count must stay bounded")
		}
	})
	mem.db.Callback().Update().Before("gorm:update").Register("test:one-orphan-save", func(scope *gorm.Scope) {
		if scope.TableName() == (&schema.VectorStoreCollection{}).TableName() {
			saves.Add(1)
		}
	})
	results, err := mem.SearchBySemanticsMemoryIDs("query", 1024)
	require.NoError(t, err)
	require.Len(t, results, 16)
	require.EqualValues(t, 1, saves.Load(), "interactive search may only persist one repair batch")
	require.LessOrEqual(t, queries.Load(), int32(9), "entity lookup must be batched, not one query per hit")
	count, err := mem.rag.CountDocuments()
	require.NoError(t, err)
	require.Equal(t, 400-semanticSearchBatchSize, count, "a search must not drain an arbitrarily large backlog")
}

func TestMUSTPASS_SemanticOrphanRevalidationPreservesRestoredAndReassigned(t *testing.T) {
	for _, restored := range []bool{true, false} {
		t.Run(fmt.Sprintf("restored=%v", restored), func(t *testing.T) {
			mem := orphanSearchMemory(t, 2, 1)
			if restored {
				require.NoError(t, mem.db.Create(&schema.AIMemoryEntity{MemoryID: "memory-000", SessionID: "default", Content: "restored"}).Error)
			} else {
				require.NoError(t, mem.rag.VectorStore.Add(&vectorstore.Document{ID: "memory-000", Content: "reassigned",
					Metadata: schema.MetadataMap{"memory_id": "memory-002", "session_id": "default"}}))
			}
			require.ErrorIs(t, mem.deleteOrphanSemanticDocuments(map[string]string{"memory-000": "memory-000", "memory-001": "memory-001"}), errSemanticOrphanChanged)
			require.True(t, mem.rag.VectorStore.Has("memory-000"))
			require.True(t, mem.rag.VectorStore.Has("memory-001"), "failed precondition rolls back the entire batch")
		})
	}
}

func TestMUSTPASS_SemanticOrphanSessionIsolationAndSoftDelete(t *testing.T) {
	mem := orphanSearchMemory(t, 1, 3)
	// Another session's valid entity must not make this session's orphan valid.
	require.NoError(t, mem.db.Create(&schema.AIMemoryEntity{MemoryID: "memory-000", SessionID: "other", Content: "foreign"}).Error)
	require.NoError(t, mem.db.Where("memory_id = ?", "memory-001").Delete(&schema.AIMemoryEntity{}).Error)
	// ID-only validation must not decode unrelated malformed legacy JSON.
	require.NoError(t, mem.db.Exec("UPDATE ai_memory_entities_v1 SET tags = ? WHERE memory_id = ?", "invalid-json", "memory-002").Error)
	require.NoError(t, mem.rag.VectorStore.Add(
		&vectorstore.Document{ID: "foreign", Content: "foreign", Metadata: schema.MetadataMap{"memory_id": "missing", "session_id": "other"}},
		&vectorstore.Document{ID: "unrelated", Content: "collection metadata", Metadata: schema.MetadataMap{}},
		&vectorstore.Document{ID: "second-question", Content: "second question", Metadata: schema.MetadataMap{"memory_id": "memory-003", "session_id": "default"}},
	))
	results, err := mem.SearchBySemanticsMemoryIDs("query", 32)
	require.NoError(t, err)
	var ids []string
	for _, hit := range results {
		ids = append(ids, hit.Entity.Id)
		require.Empty(t, hit.Entity.Content)
	}
	require.ElementsMatch(t, []string{"memory-002", "memory-003"}, ids)
	for _, id := range []string{"foreign", "unrelated", "second-question"} {
		require.True(t, mem.rag.VectorStore.Has(id), id)
	}
	for _, id := range []string{"memory-000", "memory-001"} {
		require.False(t, mem.rag.VectorStore.Has(id), id)
	}
	var count int
	require.NoError(t, mem.db.Model(&schema.AIMemoryEntity{}).Where("session_id = ?", "other").Count(&count).Error)
	require.Equal(t, 1, count)
}

func TestMUSTPASS_SemanticOrphanConcurrentSearches(t *testing.T) {
	mem := orphanSearchMemory(t, 32, 8)
	var wg sync.WaitGroup
	errors := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				results, err := mem.SearchBySemantics("query", 64)
				if err != nil {
					errors <- err
					return
				}
				if len(results) != 8 {
					errors <- fmt.Errorf("expected 8 survivors, got %d", len(results))
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	count, err := mem.rag.CountDocuments()
	require.NoError(t, err)
	require.Equal(t, 8, count)
}

// Keep the larger dirty-data experiment opt-in; CI runs the bounded regression above.
func BenchmarkSemanticOrphanBacklog(b *testing.B) {
	for _, size := range []int{1000, 10000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				mem := orphanSearchMemoryWithDimension(b, size, 10, 1024)
				b.StartTimer()
				results, err := mem.SearchBySemanticsMemoryIDs("query", 100)
				b.StopTimer()
				require.NoError(b, err)
				for _, hit := range results {
					var index int
					_, err := fmt.Sscanf(hit.Entity.Id, "memory-%d", &index)
					require.NoError(b, err)
					require.GreaterOrEqual(b, index, size)
				}
				for j := size; j < size+10; j++ {
					require.True(b, mem.rag.VectorStore.Has(fmt.Sprintf("memory-%03d", j)), "repair lost a surviving memory")
				}
				count, err := mem.rag.CountDocuments()
				require.NoError(b, err)
				require.Equal(b, size+10-semanticSearchBatchSize, count)
				b.StartTimer()
			}
		})
	}
}

func TestMUSTPASS_SemanticOrphanMidtermUsesArchiveTable(t *testing.T) {
	mem := orphanSearchMemory(t, 1, 1)
	mem.midtermArchiveMode = true
	require.NoError(t, mem.db.Table(mem.entityTableName()).AutoMigrate(&schema.AIMemoryEntity{}).Error)
	require.NoError(t, mem.db.Table(mem.entityTableName()).Create(&schema.AIMemoryEntity{MemoryID: "memory-000", SessionID: "default", Content: "restored"}).Error)
	results, err := mem.SearchBySemanticsMemoryIDs("query", 8)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "memory-000", results[0].Entity.Id, "archive validity must not depend on regular memory rows")
	require.False(t, mem.rag.VectorStore.Has("memory-001"))
}
