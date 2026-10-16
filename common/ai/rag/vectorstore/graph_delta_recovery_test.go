package vectorstore

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMUSTPASS_GraphDeltaAddFailureRollsBackDocumentsAndTopology(t *testing.T) {
	db, store := persistTestStore(t, 32, WithGraphPersistence(0, time.Hour))
	require.NoError(t, store.AddWithOptions("existing", "original"))
	before := graphTopology(store.hnsw.graph)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_delta BEFORE INSERT ON rag_vector_graph_delta_v1
		BEGIN SELECT RAISE(ABORT, 'injected delta failure'); END`).Error)
	require.ErrorContains(t, store.Add(&Document{ID: "existing", Content: "replacement"}, &Document{ID: "new", Content: "new"}), "injected delta failure")
	require.Equal(t, before, graphTopology(store.hnsw.graph))
	require.Empty(t, store.hnsw.graph.TakeAdjacencyChanges())
	doc, found, err := store.Get("existing")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "original", doc.Content)
	_, found, err = store.Get("new")
	require.NoError(t, err)
	require.False(t, found)
	require.NoError(t, db.Exec("DROP TRIGGER fail_delta").Error)
	require.NoError(t, store.AddWithOptions("new", "new"))
	restarted := restartStoreWithoutFlush(t, db, store)
	require.Equal(t, graphTopology(store.hnsw.graph), graphTopology(restarted.hnsw.graph))
}

func TestMUSTPASS_GraphDeltaCommitFailureRestoresTracking(t *testing.T) {
	db, store := persistTestStore(t, 16, WithGraphPersistence(0, time.Hour))
	require.NoError(t, store.AddWithOptions("existing", "original"))
	before := graphTopology(store.hnsw.graph)
	require.NoError(t, db.Exec("PRAGMA foreign_keys=ON").Error)
	require.NoError(t, db.Exec("CREATE TABLE delta_parent (id INTEGER PRIMARY KEY)").Error)
	require.NoError(t, db.Exec("CREATE TABLE delta_child (id INTEGER REFERENCES delta_parent(id) DEFERRABLE INITIALLY DEFERRED)").Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_delta_commit AFTER INSERT ON rag_vector_graph_delta_v1
		BEGIN INSERT INTO delta_child VALUES (123); END`).Error)
	require.Error(t, store.AddWithOptions("new", "new"))
	require.Equal(t, before, graphTopology(store.hnsw.graph))
	require.Empty(t, store.hnsw.graph.TakeAdjacencyChanges())
	require.Error(t, store.Delete("existing"))
	require.Equal(t, before, graphTopology(store.hnsw.graph))
	require.Empty(t, store.hnsw.graph.TakeAdjacencyChanges())
	require.NoError(t, db.Exec("DROP TRIGGER fail_delta_commit").Error)
	require.NoError(t, store.AddWithOptions("after", "after"))
	restarted := restartStoreWithoutFlush(t, db, store)
	require.Equal(t, graphTopology(store.hnsw.graph), graphTopology(restarted.hnsw.graph))
}

func TestMUSTPASS_GraphDeltaColdRecoveryMatchesTopologyAndSearch(t *testing.T) {
	for _, withSnapshot := range []bool{false, true} {
		t.Run(fmt.Sprint(withSnapshot), func(t *testing.T) {
			db, store := persistTestStore(t, 32, WithGraphPersistence(0, time.Hour))
			for i := 0; i < 40; i++ {
				require.NoError(t, store.AddWithOptions(fmt.Sprint(i), fmt.Sprintf("document-%d", i)))
			}
			if withSnapshot {
				require.NoError(t, store.PersistGraph(context.Background()))
			}
			require.NoError(t, store.Delete("1", "2", "3", "4", "5", "6", "7"))
			require.NoError(t, store.AddWithOptions("3", "replacement-vector"))
			restarted := restartStoreWithoutFlush(t, db, store)
			require.Equal(t, graphTopology(store.hnsw.graph), graphTopology(restarted.hnsw.graph))
			for _, text := range []string{"document-11", "replacement-vector", "unseen query"} {
				query, err := store.embedder.Embedding(text)
				require.NoError(t, err)
				left := store.hnsw.SearchWithDistanceAndFilter(query, 5, nil)
				right := restarted.hnsw.SearchWithDistanceAndFilter(query, 5, nil)
				require.Len(t, right, len(left))
				for i := range left {
					require.Equal(t, left[i].Key, right[i].Key)
					require.InDelta(t, left[i].Distance, right[i].Distance, 1e-9)
				}
			}
			require.NoError(t, restarted.PersistGraph(context.Background()))
			var collection schema.VectorStoreCollection
			require.NoError(t, restarted.db.First(&collection, restarted.collection.ID).Error)
			persistent, err := hnsw.LoadBinary[string](bytes.NewReader(collection.GraphBinary))
			require.NoError(t, err)
			require.Equal(t, hnsw.ExportModeStrUID, persistent.ExportMode)
		})
	}
}

func TestMUSTPASS_GraphDeltaUncachedExportCompactsPendingChanges(t *testing.T) {
	db, store := persistTestStore(t, 32, WithGraphPersistence(0, time.Hour))
	require.NoError(t, store.AddWithOptions("one", "one"))
	require.NoError(t, store.AddWithOptions("two", "two"))
	coldDB := copyGraphDatabase(t, db)
	collection, err := QueryCollectionForExport(coldDB, store.collection.Name)
	require.NoError(t, err)
	t.Cleanup(func() { GraphWrapperManager.RemoveCollectionFromCache(coldDB, collection) })
	require.Equal(t, 2, persistedBaseNodes(t, collection.GraphBinary))
	require.Zero(t, countGraphDeltas(t, coldDB, collection.UUID))
}

func TestMUSTPASS_GraphSnapshotAndDeltaCleanupAreAtomic(t *testing.T) {
	db, store := persistTestStore(t, 32, WithGraphPersistence(0, time.Hour))
	require.NoError(t, store.AddWithOptions("one", "one"))
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_delta_cleanup BEFORE DELETE ON rag_vector_graph_delta_v1
		BEGIN SELECT RAISE(ABORT, 'injected cleanup failure'); END`).Error)
	require.ErrorContains(t, store.PersistGraph(context.Background()), "injected cleanup failure")
	var collection schema.VectorStoreCollection
	require.NoError(t, db.First(&collection, store.collection.ID).Error)
	require.Empty(t, collection.GraphBinary, "failed cleanup must roll back the new snapshot")
	require.EqualValues(t, 1, countGraphDeltas(t, db, collection.UUID))
	require.NoError(t, db.Exec("DROP TRIGGER fail_delta_cleanup").Error)
	require.NoError(t, store.PersistGraph(context.Background()))
	require.Zero(t, countGraphDeltas(t, db, collection.UUID))
}

func TestMUSTPASS_GraphSnapshotRetriesAfterTransientFailure(t *testing.T) {
	db, store := persistTestStore(t, 32, WithGraphPersistence(0, 30*time.Millisecond))
	attempts := countCollectionUpdates(t, db)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_snapshot BEFORE UPDATE ON rag_vector_collection_v1
		BEGIN SELECT RAISE(ABORT, 'injected snapshot failure'); END`).Error)
	require.NoError(t, store.AddWithOptions("one", "one"))
	require.Eventually(t, func() bool { return attempts.Load() > 0 }, 3*time.Second, 5*time.Millisecond)
	require.NoError(t, db.Exec("DROP TRIGGER fail_snapshot").Error)
	require.Eventually(t, func() bool {
		var collection schema.VectorStoreCollection
		return db.First(&collection, store.collection.ID).Error == nil && len(collection.GraphBinary) > 0
	}, 3*time.Second, 5*time.Millisecond)
	// A queue barrier also waits for the atomic log cleanup.
	require.NoError(t, store.hnsw.queuePersist(false))
	require.Zero(t, countGraphDeltas(t, db, store.collection.UUID))
}

func TestMUSTPASS_GraphManualPersistenceDoesNotRetainDirtyAdjacency(t *testing.T) {
	db, store := persistTestStore(t, 32, WithEnableAutoUpdateGraphInfos(false))
	require.NoError(t, store.AddWithOptions("one", "one"))
	require.Empty(t, store.hnsw.graph.TakeAdjacencyChanges())
	require.NoError(t, store.PersistGraph(context.Background()))
	var collection schema.VectorStoreCollection
	require.NoError(t, db.First(&collection, store.collection.ID).Error)
	require.Equal(t, 1, persistedBaseNodes(t, collection.GraphBinary))
}

func TestMUSTPASS_GraphReloadWaitsForEvictionSnapshot(t *testing.T) {
	db, store := persistTestStore(t, 16, WithGraphPersistence(0, time.Hour))
	require.NoError(t, store.AddWithOptions("one", "one"))
	started, release := make(chan struct{}), make(chan struct{})
	originalPersist := store.hnsw.persistLocked
	store.hnsw.persistLocked = func(force bool) error {
		close(started)
		<-release
		return originalPersist(force)
	}
	evicted := make(chan struct{})
	go func() {
		GraphWrapperManager.RemoveCollectionFromCache(db, store.collection)
		close(evicted)
	}()
	<-started
	loaded := make(chan error, 1)
	go func() {
		restored, err := LoadCollection(db, store.collection.Name, WithEmbeddingClient(store.embedder))
		if err == nil && !restored.Has("one") {
			err = fmt.Errorf("reload lost a committed node")
		}
		loaded <- err
	}()
	select {
	case err := <-loaded:
		close(release)
		<-evicted
		t.Fatalf("reload returned before the previous writer closed: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	<-evicted
	require.NoError(t, <-loaded)
}

func TestMUSTPASS_GraphPQConversionAndDeltaRecovery(t *testing.T) {
	db, store := persistTestStore(t, 16, WithGraphPersistence(0, time.Hour))
	for i := 0; i < 20; i++ {
		require.NoError(t, store.AddWithOptions(fmt.Sprint(i), fmt.Sprintf("document-%d", i)))
	}
	before := graphTopology(store.hnsw.graph)
	require.NoError(t, store.ConvertToPQMode())
	require.Equal(t, before, graphTopology(store.hnsw.graph))
	require.Zero(t, countGraphDeltas(t, db, store.collection.UUID))
	require.NoError(t, store.AddWithOptions("pq-new", "new PQ document"))
	require.NoError(t, store.AddWithOptions("3", "replacement PQ document"))
	require.NoError(t, store.Delete("0", "1", "2"))
	restarted := restartStoreWithoutFlush(t, db, store)
	require.True(t, restarted.hnsw.IsPQEnabled())
	require.Equal(t, graphTopology(store.hnsw.graph), graphTopology(restarted.hnsw.graph))
	query, err := store.embedder.Embedding("new PQ document")
	require.NoError(t, err)
	left := store.hnsw.SearchWithDistanceAndFilter(query, 5, nil)
	right := restarted.hnsw.SearchWithDistanceAndFilter(query, 5, nil)
	require.Len(t, right, len(left))
	for i := range left {
		require.Equal(t, left[i].Key, right[i].Key)
		require.InDelta(t, left[i].Distance, right[i].Distance, 1e-9)
	}
}

func TestMUSTPASS_GraphPQConversionFailureRollsBack(t *testing.T) {
	db, store := persistTestStore(t, 16, WithGraphPersistence(0, time.Hour))
	for i := 0; i < 10; i++ {
		require.NoError(t, store.AddWithOptions(fmt.Sprint(i), fmt.Sprint(i)))
	}
	before := graphTopology(store.hnsw.graph)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_pq BEFORE UPDATE ON rag_vector_collection_v1
		BEGIN SELECT RAISE(ABORT, 'injected PQ failure'); END`).Error)
	require.ErrorContains(t, store.ConvertToPQMode(), "injected PQ failure")
	require.False(t, store.hnsw.IsPQEnabled())
	require.Equal(t, before, graphTopology(store.hnsw.graph))
	var count int64
	require.NoError(t, db.Model(&schema.VectorStoreDocument{}).Where("length(pq_code) > 0").Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Exec("DROP TRIGGER fail_pq").Error)
	restarted := restartStoreWithoutFlush(t, db, store)
	require.False(t, restarted.hnsw.IsPQEnabled())
	require.Equal(t, before, graphTopology(restarted.hnsw.graph))
}

func TestMUSTPASS_GraphDeltaUpdatesLegacyFullVectorSnapshot(t *testing.T) {
	db, store := persistTestStore(t, 16, WithGraphPersistence(0, time.Hour))
	require.NoError(t, store.AddWithOptions("one", "one"))
	require.NoError(t, store.AddWithOptions("two", "two"))
	persistent, err := hnsw.ExportHNSWGraph(store.hnsw.graph)
	require.NoError(t, err)
	persistent.ExportMode = hnsw.ExportModeStandard
	for _, node := range persistent.OffsetToKey {
		vector, err := store.embedder.Embedding(node.Key)
		require.NoError(t, err)
		values := make([]float64, len(vector))
		for i, value := range vector {
			values[i] = float64(value)
		}
		node.Code = values
	}
	reader, err := persistent.ToBinary(context.Background())
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, db.Model(&schema.VectorStoreCollection{}).Where("id = ?", store.collection.ID).Update("graph_binary", data).Error)
	require.NoError(t, clearGraphDeltas(db, store.collection.UUID, 0))
	require.NoError(t, store.AddWithOptions("one", "changed content"))
	restarted := restartStoreWithoutFlush(t, db, store)
	for key, node := range store.hnsw.graph.Layers[0].Nodes {
		require.Equal(t, node.GetVector()(), restarted.hnsw.graph.Layers[0].Nodes[key].GetVector()())
	}
	require.Equal(t, graphTopology(store.hnsw.graph), graphTopology(restarted.hnsw.graph))
}

func TestMUSTPASS_GraphImportOverwriteRetainsDestinationIdentity(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprint(cached), func(t *testing.T) {
			sourceDB, source := persistTestStore(t, 16)
			require.NoError(t, source.AddWithOptions("fresh", "fresh"))
			destDB, dest := persistTestStore(t, 16, WithGraphPersistence(0, time.Hour))
			require.NoError(t, dest.AddWithOptions("stale", "stale"))
			if !cached {
				destDB = copyGraphDatabase(t, destDB)
			}
			reader, err := ExportRAGToBinary(source.collection.Name, WithImportExportDB(sourceDB))
			require.NoError(t, err)
			require.NoError(t, ImportRAGFromReader(reader, WithImportExportDB(destDB), WithOverwriteExisting(true)))
			loaded, err := LoadCollection(destDB, dest.collection.Name, WithEmbeddingClient(dest.embedder), WithTryRebuildHNSWIndex(false))
			require.NoError(t, err)
			t.Cleanup(func() { GraphWrapperManager.RemoveCollectionFromCache(destDB, loaded.collection) })
			require.Equal(t, dest.collection.UUID, loaded.collection.UUID)
			require.True(t, loaded.Has("fresh"))
			require.False(t, loaded.Has("stale"))
			require.Zero(t, countGraphDeltas(t, destDB, dest.collection.UUID))
			var doc schema.VectorStoreDocument
			require.NoError(t, destDB.Where("collection_id = ?", loaded.collection.ID).First(&doc).Error)
			require.Equal(t, loaded.collection.UUID, doc.CollectionUUID)
		})
	}
}

func TestMUSTPASS_GraphDeltaDecoderRejectsMalformedPayloads(t *testing.T) {
	valid, err := encodeAdjacencyDelta([]hnsw.AdjacencyChange[string]{{Key: "one", Neighbors: []string{"two"}}})
	require.NoError(t, err)
	for length := 0; length < len(valid); length++ {
		_, err := decodeAdjacencyDelta(valid[:length])
		require.Error(t, err, "truncated length %d", length)
	}
	for _, offset := range []int{8, 12, 16, 21, 28} {
		payload := bytes.Clone(valid)
		binary.LittleEndian.PutUint32(payload[offset:], ^uint32(0))
		_, err := decodeAdjacencyDelta(payload)
		require.Error(t, err)
	}
	_, err = decodeAdjacencyDelta(append(bytes.Clone(valid), 0))
	require.Error(t, err)
}

func FuzzDecodeAdjacencyDelta(f *testing.F) {
	valid, _ := encodeAdjacencyDelta([]hnsw.AdjacencyChange[string]{{Key: "one"}})
	f.Add(valid)
	f.Add([]byte("YAKDELTA"))
	f.Fuzz(func(t *testing.T, payload []byte) {
		changes, err := decodeAdjacencyDelta(payload)
		if err == nil {
			roundtrip, err := encodeAdjacencyDelta(changes)
			require.NoError(t, err)
			require.Equal(t, payload, roundtrip)
		}
	})
}

func graphTopology(graph *hnsw.Graph[string]) []map[string]map[string]bool {
	layers := make([]map[string]map[string]bool, len(graph.Layers))
	for i, layer := range graph.Layers {
		layers[i] = make(map[string]map[string]bool)
		for key, node := range layer.Nodes {
			neighbors := make(map[string]bool)
			for neighbor := range node.GetNeighbors() {
				neighbors[neighbor] = true
			}
			layers[i][key] = neighbors
		}
	}
	return layers
}

// Copy only committed rows; closing/evicting the live wrapper would flush it
// and accidentally hide the crash-recovery behavior under test.
func copyGraphDatabase(t *testing.T, db *gorm.DB) *gorm.DB {
	t.Helper()
	cold, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = cold.Close() })
	require.NoError(t, cold.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}, &schema.VectorStoreGraphDelta{}).Error)
	var collections []schema.VectorStoreCollection
	var documents []schema.VectorStoreDocument
	var deltas []schema.VectorStoreGraphDelta
	require.NoError(t, db.Find(&collections).Error)
	require.NoError(t, db.Find(&documents).Error)
	require.NoError(t, db.Find(&deltas).Error)
	for i := range collections {
		require.NoError(t, cold.Create(&collections[i]).Error)
	}
	for i := range documents {
		require.NoError(t, cold.Create(&documents[i]).Error)
	}
	for i := range deltas {
		require.NoError(t, cold.Create(&deltas[i]).Error)
	}
	return cold
}

func restartStoreWithoutFlush(t *testing.T, db *gorm.DB, store *SQLiteVectorStoreHNSW) *SQLiteVectorStoreHNSW {
	t.Helper()
	cold := copyGraphDatabase(t, db)
	restarted, err := LoadCollection(cold, store.collection.Name, WithEmbeddingClient(store.embedder),
		WithGraphPersistence(0, time.Hour), WithTryRebuildHNSWIndex(false), WithAutoDeleteCorruptedRAG(false))
	require.NoError(t, err)
	t.Cleanup(func() { GraphWrapperManager.RemoveCollectionFromCache(cold, restarted.collection) })
	return restarted
}
