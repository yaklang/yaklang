package vectorstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMUSTPASS_PQSubspaceCount(t *testing.T) {
	require.Equal(t, 32, pqSubspaceCount(1024))
	require.Equal(t, 32, pqSubspaceCount(768))
	require.Equal(t, 16, pqSubspaceCount(128))
	require.Equal(t, 1, pqSubspaceCount(3))
}

func TestMUSTPASS_NewCollectionUsesTunedHNSWParameters(t *testing.T) {
	config := NewCollectionConfig()
	require.Equal(t, 64, config.EfSearch)
	require.Equal(t, 64, config.EfConstruct)

	db, store := persistTestStore(t, 32)
	require.Equal(t, 64, store.collection.EfSearch)
	require.Equal(t, 64, store.collection.EfConstruct)
	require.Equal(t, 64, store.hnsw.graph.EfSearch)
	require.Equal(t, 64, store.hnsw.graph.EfConstruction)

	require.NoError(t, db.Model(&schema.VectorStoreCollection{}).Where("id = ?", store.collection.ID).
		Updates(map[string]any{"ef_search": 20, "ef_construct": 200}).Error)
	GraphWrapperManager.RemoveCollectionFromCache(db, store.collection)
	loaded, err := LoadCollection(db, store.collection.Name, WithEmbeddingClient(store.embedder))
	require.NoError(t, err)
	require.Equal(t, 20, loaded.collection.EfSearch)
	require.Equal(t, 200, loaded.collection.EfConstruct)
	require.Equal(t, 20, loaded.hnsw.graph.EfSearch)
	require.Equal(t, 200, loaded.hnsw.graph.EfConstruction)
}

func TestMUSTPASS_GraphPersistFlushesSmallCollectionsImmediately(t *testing.T) {
	db, store := persistTestStore(t, 32)
	for i := 0; i < 3; i++ {
		require.NoError(t, store.AddWithOptions(fmt.Sprintf("doc-%d", i), fmt.Sprintf("doc-%d", i)))
	}
	require.Zero(t, countGraphDeltas(t, db, store.collection.UUID))
	var collection schema.VectorStoreCollection
	require.NoError(t, db.First(&collection, store.collection.ID).Error)
	require.Equal(t, 3, persistedBaseNodes(t, collection.GraphBinary))
}

func TestMUSTPASS_GraphPersistCoalescesLargeCollections(t *testing.T) {
	db, store := persistTestStore(t, 32, WithGraphPersistence(0, time.Hour))
	ids := make([]string, 20)
	for i := range ids {
		ids[i] = fmt.Sprintf("doc-%02d", i)
		require.NoError(t, store.AddWithOptions(ids[i], ids[i]))
	}
	require.EqualValues(t, len(ids), countGraphDeltas(t, db, store.collection.UUID))
	var collection schema.VectorStoreCollection
	require.NoError(t, db.First(&collection, store.collection.ID).Error)
	require.Empty(t, collection.GraphBinary)

	replayed := replayDeltasForTest(t, db, &collection)
	require.Equal(t, len(ids), replayed.Len())

	require.NoError(t, store.PersistGraph(context.Background()))
	require.Zero(t, countGraphDeltas(t, db, store.collection.UUID))
	require.NoError(t, db.First(&collection, store.collection.ID).Error)
	require.Equal(t, store.hnsw.GetSize(), persistedBaseNodes(t, collection.GraphBinary))

	GraphWrapperManager.RemoveCollectionFromCache(db, store.collection)
	loaded, err := LoadCollection(db, store.collection.Name, WithEmbeddingClient(store.embedder))
	require.NoError(t, err)
	for _, id := range ids {
		require.True(t, loaded.Has(id))
	}
}

func TestMUSTPASS_GraphPersistDelayedFlush(t *testing.T) {
	db, store := persistTestStore(t, 32, WithGraphPersistence(0, 100*time.Millisecond))
	require.NoError(t, store.AddWithOptions("only", "only"))
	require.Eventually(t, func() bool {
		var collection schema.VectorStoreCollection
		if err := db.First(&collection, store.collection.ID).Error; err != nil {
			return false
		}
		var pending int64
		err := db.Model(&schema.VectorStoreGraphDelta{}).Where("collection_uuid = ?", store.collection.UUID).Count(&pending).Error
		return err == nil && len(collection.GraphBinary) > 0 && pending == 0
	}, 3*time.Second, 10*time.Millisecond)
}

func TestMUSTPASS_GraphDeltaIsSmallerThanSnapshot(t *testing.T) {
	db, store := persistTestStore(t, 32, WithGraphPersistence(0, time.Hour))
	for i := 0; i < 300; i++ {
		require.NoError(t, store.AddWithOptions(fmt.Sprintf("doc-%03d", i), fmt.Sprintf("doc-%03d", i)))
	}
	var latest schema.VectorStoreGraphDelta
	require.NoError(t, db.Order("seq desc").First(&latest).Error)
	require.NoError(t, store.PersistGraph(context.Background()))
	var collection schema.VectorStoreCollection
	require.NoError(t, db.First(&collection, store.collection.ID).Error)
	require.Less(t, len(latest.Payload), len(collection.GraphBinary))
}

func TestMUSTPASS_GraphDeltaDeleteReloadsWithoutSnapshot(t *testing.T) {
	db, store := persistTestStore(t, 32, WithGraphPersistence(0, time.Hour))
	ids := make([]string, 10)
	for i := range ids {
		ids[i] = fmt.Sprintf("doc-%02d", i)
		require.NoError(t, store.AddWithOptions(ids[i], ids[i]))
	}
	require.NoError(t, store.Delete(ids[:4]...))
	var collection schema.VectorStoreCollection
	require.NoError(t, db.First(&collection, store.collection.ID).Error)
	replayed := replayDeltasForTest(t, db, &collection)
	require.Equal(t, 6, replayed.Len())
	for i, id := range ids {
		require.Equal(t, i >= 4, replayed.Has(id))
	}
}

func countGraphDeltas(t *testing.T, db *gorm.DB, collectionUUID string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&schema.VectorStoreGraphDelta{}).Where("collection_uuid = ?", collectionUUID).Count(&count).Error)
	return count
}

func replayDeltasForTest(t *testing.T, db *gorm.DB, collection *schema.VectorStoreCollection) *hnsw.Graph[string] {
	t.Helper()
	deltas, err := loadGraphDeltas(db, collection.UUID)
	require.NoError(t, err)
	graph := emptyHNSWGraph(collection)
	create, err := newDeltaNodeFactory(db, collection)
	require.NoError(t, err)
	for _, changes := range deltas {
		require.NoError(t, graph.ApplyAdjacencyChanges(changes, create))
	}
	return graph
}

func persistedBaseNodes(t *testing.T, binary []byte) int {
	t.Helper()
	persisted, err := hnsw.LoadBinary[string](bytes.NewReader(binary))
	require.NoError(t, err)
	require.NotEmpty(t, persisted.Layers)
	return len(persisted.Layers[0].Nodes)
}

func persistTestStore(t testing.TB, dimension int, opts ...CollectionConfigFunc) (*gorm.DB, *SQLiteVectorStoreHNSW) {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}).Error)
	embedder := NewMockEmbedder(func(text string) ([]float32, error) {
		hash := sha256.Sum256([]byte(text))
		vector := make([]float32, dimension)
		for i := range vector {
			vector[i] = float32(int(hash[i%len(hash)])-128) / 128
		}
		return vector, nil
	})
	store, err := NewSQLiteVectorStoreHNSW(t.Name(), "", "mock", dimension, embedder, db, append(opts, WithDisableEmbedCollectionInfo(true))...)
	require.NoError(t, err)
	t.Cleanup(func() { GraphWrapperManager.RemoveCollectionFromCache(db, store.collection) })
	return db, store
}

func countCollectionUpdates(t *testing.T, db *gorm.DB) *atomic.Int64 {
	t.Helper()
	var saves atomic.Int64
	name := fmt.Sprintf("test:graph-saves:%s", t.Name())
	db.Callback().Update().Before("gorm:update").Register(name, func(scope *gorm.Scope) {
		if scope.TableName() == (&schema.VectorStoreCollection{}).TableName() {
			saves.Add(1)
		}
	})
	t.Cleanup(func() { db.Callback().Update().Remove(name) })
	return &saves
}
