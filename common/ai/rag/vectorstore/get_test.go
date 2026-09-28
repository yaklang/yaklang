package vectorstore

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMUSTPASS_LoadCollectionWithInvalidGraphBinary(t *testing.T) {
	testDB, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { testDB.Close() })

	collectionName := utils.RandStringBytes(10)
	var embedCalls atomic.Int64
	embedder := NewMockEmbedder(func(string) ([]float32, error) {
		embedCalls.Add(1)
		vector := make([]float32, 1024)
		vector[0] = 1
		return vector, nil
	})
	store, err := GetCollection(testDB, collectionName, WithEmbeddingClient(embedder))
	require.NoError(t, err)
	require.NoError(t, store.AddWithOptions("repairable-document", "repairable content"))

	corruptedBinary := []byte{0x00, 0x01, 0x02, 0x03}
	require.NoError(t, testDB.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).
		Update("graph_binary", corruptedBinary).Error)
	GraphWrapperManager.ClearCache()
	embedCalls.Store(0)

	repaired, err := LoadCollection(testDB, collectionName, WithEmbeddingClient(embedder))
	require.NoError(t, err)
	require.True(t, repaired.Has("repairable-document"))
	require.Zero(t, embedCalls.Load(), "binary recovery must use stored vectors without invoking embedding")

	var collection schema.VectorStoreCollection
	require.NoError(t, testDB.Where("name = ?", collectionName).First(&collection).Error)
	require.NotEqual(t, corruptedBinary, collection.GraphBinary)
	require.NotEmpty(t, collection.GraphBinary)
}

func TestMUSTPASS_UnrecoverableGraphBinaryDeletesCompleteRAG(t *testing.T) {
	testDB, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { testDB.Close() })
	require.NoError(t, testDB.AutoMigrate(
		&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{},
		&schema.EntityRepository{}, &schema.ERModelEntity{}, &schema.ERModelRelationship{},
	).Error)

	collectionName := utils.RandStringBytes(10)
	var embedCalls atomic.Int64
	embedder := NewMockEmbedder(func(string) ([]float32, error) {
		embedCalls.Add(1)
		vector := make([]float32, 1024)
		vector[0] = 1
		return vector, nil
	})
	store, err := GetCollection(testDB, collectionName, WithEmbeddingClient(embedder))
	require.NoError(t, err)
	require.NoError(t, store.AddWithOptions("unrecoverable-document", "content"))
	ragID := "unrecoverable-rag-id"
	require.NoError(t, testDB.Model(&schema.VectorStoreCollection{}).Where("id = ?", store.collection.ID).
		Updates(map[string]interface{}{"rag_id": ragID, "graph_binary": []byte{0x01, 0x02}}).Error)
	require.NoError(t, testDB.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", store.collection.ID).
		Update("embedding", nil).Error)

	knowledgeBase := &schema.KnowledgeBaseInfo{KnowledgeBaseName: "linked-kb", KnowledgeBaseType: "test", RAGID: ragID}
	require.NoError(t, testDB.Create(knowledgeBase).Error)
	require.NoError(t, testDB.Create(&schema.KnowledgeBaseEntry{KnowledgeBaseID: int64(knowledgeBase.ID), KnowledgeTitle: "entry", KnowledgeType: "test"}).Error)
	repository := &schema.EntityRepository{EntityBaseName: "linked-entity", Uuid: "linked-repository", RAGID: ragID}
	require.NoError(t, testDB.Create(repository).Error)
	require.NoError(t, testDB.Create(&schema.ERModelEntity{RepositoryUUID: repository.Uuid, EntityName: "entity"}).Error)
	require.NoError(t, testDB.Create(&schema.ERModelRelationship{RepositoryUUID: repository.Uuid, Hash: "relationship"}).Error)

	// Exact-ID cleanup must preserve an unrelated RAG even when every table is
	// populated, rather than broadening the delete by name or binary contents.
	unrelatedRAGID := "unrelated-rag-id"
	unrelatedCollection := &schema.VectorStoreCollection{Name: "unrelated-collection", UUID: "unrelated-collection-uuid", RAGID: unrelatedRAGID, Dimension: 1024}
	require.NoError(t, testDB.Create(unrelatedCollection).Error)
	require.NoError(t, testDB.Create(&schema.VectorStoreDocument{
		DocumentID: "unrelated-document", CollectionID: unrelatedCollection.ID, CollectionUUID: unrelatedCollection.UUID,
		Embedding: make([]float32, 1024),
	}).Error)
	unrelatedKB := &schema.KnowledgeBaseInfo{KnowledgeBaseName: "unrelated-kb", KnowledgeBaseType: "test", RAGID: unrelatedRAGID}
	require.NoError(t, testDB.Create(unrelatedKB).Error)
	require.NoError(t, testDB.Create(&schema.KnowledgeBaseEntry{KnowledgeBaseID: int64(unrelatedKB.ID), KnowledgeTitle: "unrelated-entry", KnowledgeType: "test"}).Error)
	unrelatedRepository := &schema.EntityRepository{EntityBaseName: "unrelated-entity", Uuid: "unrelated-repository", RAGID: unrelatedRAGID}
	require.NoError(t, testDB.Create(unrelatedRepository).Error)
	require.NoError(t, testDB.Create(&schema.ERModelEntity{RepositoryUUID: unrelatedRepository.Uuid, EntityName: "unrelated-entity"}).Error)
	require.NoError(t, testDB.Create(&schema.ERModelRelationship{RepositoryUUID: unrelatedRepository.Uuid, Hash: "unrelated-relationship"}).Error)

	GraphWrapperManager.ClearCache()
	embedCalls.Store(0)
	_, err = LoadCollection(testDB, collectionName, WithEmbeddingClient(embedder))
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrCorruptedRAGDeleted), "unexpected error: %v", err)
	require.Zero(t, embedCalls.Load(), "failed recovery and deletion must not invoke embedding")

	models := []interface{}{
		&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{},
		&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{},
		&schema.EntityRepository{}, &schema.ERModelEntity{}, &schema.ERModelRelationship{},
	}
	for _, model := range models {
		var count int64
		require.NoError(t, testDB.Unscoped().Model(model).Count(&count).Error)
		require.Equal(t, int64(1), count, "exact cleanup removed unrelated data or leaked corrupted rows for %T", model)
	}
}

func TestMUSTPASS_CorruptedPQBinaryRebuildsAsStandardHNSW(t *testing.T) {
	testDB, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { testDB.Close() })

	embedder := NewDefaultMockEmbedding()
	collectionName := utils.RandStringBytes(10)
	store, err := GetCollection(testDB, collectionName, WithEmbeddingClient(embedder))
	require.NoError(t, err)
	require.NoError(t, store.AddWithOptions("pq-repair-document", "computer data"))
	require.NoError(t, testDB.Model(&schema.VectorStoreCollection{}).Where("id = ?", store.collection.ID).
		Updates(map[string]interface{}{"enable_pq_mode": true, "code_book_binary": []byte{0x01, 0x02, 0x03}}).Error)
	GraphWrapperManager.ClearCache()

	repaired, err := LoadCollection(testDB, collectionName, WithEmbeddingClient(embedder))
	require.NoError(t, err)
	require.True(t, repaired.Has("pq-repair-document"))
	var collection schema.VectorStoreCollection
	require.NoError(t, testDB.Where("id = ?", store.collection.ID).First(&collection).Error)
	require.False(t, collection.EnablePQMode)
	require.Empty(t, collection.CodeBookBinary)
}

func TestMUSTPASS_CancelledGraphRebuildDoesNotPartiallyReplaceBinary(t *testing.T) {
	testDB, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { testDB.Close() })
	store, err := GetCollection(testDB, utils.RandStringBytes(10), WithEmbeddingClient(NewDefaultMockEmbedding()))
	require.NoError(t, err)

	corruptedBinary := []byte{0x0a, 0x0b}
	sentinelUID := []byte("unchanged-uid")
	require.NoError(t, testDB.Model(&schema.VectorStoreCollection{}).Where("id = ?", store.collection.ID).
		Update("graph_binary", corruptedBinary).Error)
	require.NoError(t, testDB.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", store.collection.ID).
		Update("uid", sentinelUID).Error)
	var collection schema.VectorStoreCollection
	require.NoError(t, testDB.Where("id = ?", store.collection.ID).First(&collection).Error)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, MigrateHNSWGraphWithContext(ctx, testDB, &collection), context.Canceled)
	require.NoError(t, testDB.Where("id = ?", collection.ID).First(&collection).Error)
	require.Equal(t, corruptedBinary, collection.GraphBinary)
	var document schema.VectorStoreDocument
	require.NoError(t, testDB.Where("collection_id = ?", collection.ID).First(&document).Error)
	require.Equal(t, sentinelUID, document.UID)
}

// TestGet_RecordNotFoundError 测试确保 gorm.IsRecordNotFoundError 能正确识别
func TestMUSTPASS_RecordNotFoundError(t *testing.T) {
	// 创建临时测试数据库
	testDB, err := createTempTestDatabase()
	if err != nil {
		t.Fatal(err)
	}

	collectionName := utils.RandStringBytes(10)

	// 验证集合不存在
	assert.False(t, HasCollection(testDB, collectionName), "collection should not exist")

	// 尝试直接加载不存在的集合
	collectionMg, err := GetCollection(testDB, collectionName, WithEmbeddingClient(NewDefaultMockEmbedding()))
	assert.NoError(t, err, "should create new collection when record not found")
	assert.True(t, HasCollection(testDB, collectionName), "collection should exist")
	assert.NotNil(t, collectionMg, "collection should not be nil")
}
