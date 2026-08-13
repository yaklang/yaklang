package vectorstore

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMUSTPASS_DeleteCollectionEvictsAndClosesGraph(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}).Error)

	name := "delete-collection-closes-graph"
	store, err := NewSQLiteVectorStoreHNSW(name, "", "mock", 3, NewMockEmbedder(func(string) ([]float32, error) {
		return []float32{1, 0, 0}, nil
	}), db)
	require.NoError(t, err)
	require.NoError(t, store.AddWithOptions("old-document", "old-document"))

	oldWrapper := store.hnsw
	require.False(t, oldWrapper.closed.Load())
	require.NoError(t, DeleteCollection(db, name))
	require.True(t, oldWrapper.closed.Load())
	select {
	case <-oldWrapper.done:
	default:
		t.Fatal("deleted collection graph worker did not stop")
	}

	var documentCount int64
	require.NoError(t, db.Unscoped().Model(&schema.VectorStoreDocument{}).Count(&documentCount).Error)
	require.Zero(t, documentCount)
	require.Error(t, store.AddWithOptions("stale-document", "stale-document"))
	require.NoError(t, db.Unscoped().Model(&schema.VectorStoreDocument{}).Count(&documentCount).Error)
	require.Zero(t, documentCount, "a stale store recreated documents after collection deletion")

	recreated, err := NewSQLiteVectorStoreHNSW(name, "", "mock", 3, NewMockEmbedder(func(string) ([]float32, error) {
		return []float32{1, 0, 0}, nil
	}), db)
	require.NoError(t, err)
	t.Cleanup(func() { _ = recreated.Remove() })
	require.NotSame(t, oldWrapper, recreated.hnsw)
	require.False(t, recreated.Has("old-document"), "recreated collection reused the deleted HNSW graph")
}

func TestMUSTPASS_RAGDeleteIndexes(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}, &schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{}).Error)

	indexNames := func(table string) []string {
		var rows []struct {
			Name string `gorm:"column:name"`
		}
		require.NoError(t, db.Raw("PRAGMA index_list('"+table+"')").Scan(&rows).Error)
		result := make([]string, 0, len(rows))
		for _, row := range rows {
			result = append(result, row.Name)
		}
		return result
	}

	require.Contains(t, indexNames((&schema.VectorStoreDocument{}).TableName()), "idx_rag_vector_document_collection_id")
	require.Contains(t, indexNames((&schema.KnowledgeBaseEntry{}).TableName()), "idx_rag_knowledge_entry_knowledge_base_id")

	queryPlan := func(query string) string {
		var rows []struct {
			Detail string `gorm:"column:detail"`
		}
		require.NoError(t, db.Raw("EXPLAIN QUERY PLAN "+query).Scan(&rows).Error)
		details := make([]string, 0, len(rows))
		for _, row := range rows {
			details = append(details, row.Detail)
		}
		return strings.Join(details, "\n")
	}
	require.Contains(t,
		queryPlan("DELETE FROM "+(&schema.VectorStoreDocument{}).TableName()+" WHERE collection_id = 1"),
		"idx_rag_vector_document_collection_id",
	)
	require.Contains(t,
		queryPlan("DELETE FROM "+(&schema.KnowledgeBaseEntry{}).TableName()+" WHERE knowledge_base_id = 1"),
		"idx_rag_knowledge_entry_knowledge_base_id",
	)
}

func TestMUSTPASS_GraphCacheIsolatedByDatabase(t *testing.T) {
	newDBAndCollection := func() (*gorm.DB, *schema.VectorStoreCollection) {
		db, err := utils.CreateTempTestDatabaseInMemory()
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		require.NoError(t, db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}).Error)
		collection := &schema.VectorStoreCollection{Name: utils.RandStringBytes(12), UUID: "shared-import-uuid", Dimension: 3}
		require.NoError(t, db.Create(collection).Error)
		return db, collection
	}

	db1, collection1 := newDBAndCollection()
	db2, collection2 := newDBAndCollection()
	config := NewCollectionConfig(WithEmbeddingClient(NewDefaultMockEmbedding()), WithModelDimension(3))
	wrapper1, err := GraphWrapperManager.GetGraphWrapper(db1, collection1, config)
	require.NoError(t, err)
	wrapper2, err := GraphWrapperManager.GetGraphWrapper(db2, collection2, config)
	require.NoError(t, err)
	require.NotSame(t, wrapper1, wrapper2)

	GraphWrapperManager.RemoveCollectionFromCache(db1, collection1)
	GraphWrapperManager.RemoveCollectionFromCache(db2, collection2)
}
