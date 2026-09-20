package vectorstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func startupGraphFixture(t testing.TB, n int, uidMode bool) (*gorm.DB, *schema.VectorStoreCollection, []schema.VectorStoreDocument) {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.VectorStoreDocument{}).Error)
	collection := &schema.VectorStoreCollection{Model: gorm.Model{ID: 7}, Name: "startup-test", Dimension: 4}
	graph := NewHNSWGraph(collection.Name)
	docs := make([]schema.VectorStoreDocument, n)
	for i := range docs {
		key := fmt.Sprintf("doc-%04d", i)
		vec := []float32{1, float32(i + 1), 2, 3}
		// An earlier row with identical identifiers in another collection must
		// never supply this graph's lazy embedding or PQ code.
		uid := []byte("uid-" + key)
		require.NoError(t, db.Create(&schema.VectorStoreDocument{CollectionID: 6, DocumentID: key, UID: uid,
			Embedding: []float32{99, 99, 99, 99}, PQCode: []byte{99}}).Error)
		docs[i] = schema.VectorStoreDocument{CollectionID: collection.ID, DocumentID: key, UID: uid, Embedding: vec, PQCode: []byte{byte(i), 1}}
		require.NoError(t, db.Create(&docs[i]).Error)
		graph.Add(hnsw.MakeInputNode(key, vec))
	}
	persistent, err := hnsw.ExportHNSWGraph(graph)
	require.NoError(t, err)
	persistent.ExportMode = hnsw.ExportModeStrUID
	for _, node := range persistent.OffsetToKey {
		if node != nil {
			node.Code = node.Key
		}
	}
	if uidMode {
		persistent.ExportMode = hnsw.ExportModeUID
		for _, node := range persistent.OffsetToKey {
			if node != nil {
				node.Code = []byte("uid-" + node.Key)
			}
		}
	}
	reader, err := persistent.ToBinary(context.Background())
	require.NoError(t, err)
	collection.GraphBinary, err = io.ReadAll(reader)
	require.NoError(t, err)
	return db, collection, docs
}

func TestMUSTPASS_GraphStartupBatchesIdentifiersAndKeepsVectorsLazy(t *testing.T) {
	for _, uidMode := range []bool{false, true} {
		for _, pqMode := range []bool{false, true} {
			t.Run(fmt.Sprintf("uid=%t/pq=%t", uidMode, pqMode), func(t *testing.T) {
				db, collection, docs := startupGraphFixture(t, 128, uidMode)
				collection.EnablePQMode = pqMode
				stats := &startupQueryStats{}
				db.SetLogger(stats)
				db.LogMode(true)
				graph, err := parseHNSWGraphFromBinary(db, collection, LoadConfigFromCollectionInfo(collection), bytes.NewReader(collection.GraphBinary))
				require.NoError(t, err)
				require.Equal(t, 1, stats.count, "restore must not query once per node/layer or eagerly read vectors")
				require.Equal(t, len(docs), graph.Len())
				for _, doc := range docs {
					node := graph.Layers[0].Nodes[doc.DocumentID]
					require.NotNil(t, node)
					if pqMode {
						codes, ok := node.GetPQCodes()
						require.True(t, ok)
						require.Equal(t, doc.PQCode, codes)
					} else {
						require.Equal(t, []float32(doc.Embedding), node.GetVector()())
					}
				}
				require.Equal(t, 1+len(docs), stats.count)
			})
		}
	}
}

func TestMUSTPASS_GraphStartupRejectsMissingOrDeletedDocuments(t *testing.T) {
	for _, hardDelete := range []bool{false, true} {
		t.Run(fmt.Sprint(hardDelete), func(t *testing.T) {
			db, collection, docs := startupGraphFixture(t, 2, false)
			query := db
			if hardDelete {
				query = db.Unscoped()
			}
			require.NoError(t, query.Delete(&docs[0]).Error)
			_, err := parseHNSWGraphFromBinary(db, collection, LoadConfigFromCollectionInfo(collection), bytes.NewReader(collection.GraphBinary))
			require.Error(t, err, "same identifier in another collection must not hide a stale graph")
		})
	}
}

func TestMUSTPASS_GraphStartupPreservesFirstDuplicate(t *testing.T) {
	db, collection, docs := startupGraphFixture(t, 1, false)
	duplicate := docs[0]
	duplicate.Model = gorm.Model{}
	duplicate.Embedding = []float32{77, 77, 77, 77}
	require.NoError(t, db.Create(&duplicate).Error)
	graph, err := parseHNSWGraphFromBinary(db, collection, LoadConfigFromCollectionInfo(collection), bytes.NewReader(collection.GraphBinary))
	require.NoError(t, err)
	require.Equal(t, []float32(docs[0].Embedding), graph.Layers[0].Nodes[docs[0].DocumentID].GetVector()())
}

func BenchmarkGraphStartup(b *testing.B) {
	for _, n := range []int{128, 1024} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			db, collection, _ := startupGraphFixture(b, n, false)
			config := LoadConfigFromCollectionInfo(collection)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := parseHNSWGraphFromBinary(db, collection, config, bytes.NewReader(collection.GraphBinary))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
