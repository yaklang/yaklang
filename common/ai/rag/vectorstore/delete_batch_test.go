package vectorstore

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func batchDeleteStore(t *testing.T, n int) (*gorm.DB, *SQLiteVectorStoreHNSW, []string) {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}).Error)
	embedder := NewMockEmbedder(func(s string) ([]float32, error) {
		v := make([]float32, 1024)
		v[0], v[len(s)%1023+1] = 1, 1
		return v, nil
	})
	store, err := NewSQLiteVectorStoreHNSW(t.Name(), "", "mock", 1024, embedder, db,
		WithDisableEmbedCollectionInfo(true))
	require.NoError(t, err)
	var ids []string
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("doc-%04d", i)
		require.NoError(t, store.AddWithOptions(id, id))
		ids = append(ids, id)
	}
	// Exercise database-backed lazy vectors, as on process restart.
	GraphWrapperManager.RemoveCollectionFromCache(db, store.collection)
	store, err = LoadCollection(db, t.Name(), WithEmbeddingClient(embedder))
	require.NoError(t, err)
	t.Cleanup(func() { GraphWrapperManager.RemoveCollectionFromCache(db, store.collection) })
	return db, store, ids
}

func TestMUSTPASS_DeleteBatchLazyVectors(t *testing.T) {
	db, store, ids := batchDeleteStore(t, 128)
	var missing, saves atomic.Int64
	db.Callback().Query().After("gorm:query").Register("test:missing-vector", func(scope *gorm.Scope) {
		if errors.Is(scope.DB().Error, gorm.ErrRecordNotFound) {
			missing.Add(1)
		}
	})
	db.Callback().Update().Before("gorm:update").Register("test:graph-saves", func(scope *gorm.Scope) {
		if scope.TableName() == (&schema.VectorStoreCollection{}).TableName() {
			saves.Add(1)
		}
	})
	require.NoError(t, store.Delete(ids[:100]...))
	require.Zero(t, missing.Load(), "repair/export read an already deleted vector")
	require.EqualValues(t, 1, saves.Load(), "batch must persist the graph only once")
	GraphWrapperManager.RemoveCollectionFromCache(db, store.collection)
	reloaded, err := LoadCollection(db, t.Name(), WithEmbeddingClient(store.embedder))
	require.NoError(t, err)
	for i, id := range ids {
		require.Equal(t, i >= 100, reloaded.Has(id))
	}
	require.NoError(t, reloaded.Delete(ids...))
	var collection schema.VectorStoreCollection
	require.NoError(t, db.First(&collection, store.collection.ID).Error)
	require.Empty(t, collection.GraphBinary)
	GraphWrapperManager.RemoveCollectionFromCache(db, &collection)
	reloaded, err = LoadCollection(db, t.Name(), WithEmbeddingClient(store.embedder))
	require.NoError(t, err)
	require.Zero(t, reloaded.hnsw.GetSize())
}

func TestMUSTPASS_DeleteBatchRollback(t *testing.T) {
	db, store, ids := batchDeleteStore(t, 24)
	var before schema.VectorStoreCollection
	require.NoError(t, db.First(&before, store.collection.ID).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_graph_save BEFORE UPDATE ON rag_vector_collection_v1
		BEGIN SELECT RAISE(ABORT, 'injected graph save failure'); END`).Error)
	require.ErrorContains(t, store.Delete(ids[:12]...), "injected graph save failure")
	for _, id := range ids {
		require.True(t, store.Has(id), "rollback lost graph node %s", id)
		_, ok, err := store.Get(id)
		require.NoError(t, err)
		require.True(t, ok, "rollback lost document %s", id)
	}
	var after schema.VectorStoreCollection
	require.NoError(t, db.First(&after, store.collection.ID).Error)
	require.Equal(t, before.GraphBinary, after.GraphBinary)
	require.NoError(t, db.Exec("DROP TRIGGER fail_graph_save").Error)
	require.NoError(t, store.Delete(ids[:12]...))
	for i, id := range ids {
		require.Equal(t, i >= 12, store.Has(id))
	}
}

func TestMUSTPASS_DeleteBatchConcurrentReaders(t *testing.T) {
	_, store, ids := batchDeleteStore(t, 64)
	var readers sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			<-start
			near := make([]float32, 1024)
			near[0] = 1
			for j := 0; j < 40; j++ {
				store.hnsw.SearchWithDistanceAndFilter(near, 10, nil)
			}
		}()
	}
	close(start)
	require.NoError(t, store.Delete(ids[:32]...))
	readers.Wait()
	for i, id := range ids {
		require.Equal(t, i >= 32, store.Has(id))
	}
}
