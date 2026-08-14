package rag

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
)

func TestMUSTPASS_DeleteRAGRemovesAllLinkedData(t *testing.T) {
	db, err := NewTemporaryRAGDB()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	collection := &schema.VectorStoreCollection{Name: "collection-name", UUID: "collection-uuid", RAGID: "shared-rag-id", Dimension: 3}
	require.NoError(t, db.Create(collection).Error)
	wrapper, err := vectorstore.GraphWrapperManager.GetGraphWrapper(db, collection, vectorstore.NewCollectionConfig(
		vectorstore.WithModelDimension(3),
		vectorstore.WithEmbeddingClient(vectorstore.NewMockEmbedder(func(string) ([]float32, error) {
			return []float32{1, 0, 0}, nil
		})),
	))
	require.NoError(t, err)
	require.NoError(t, db.Create(&schema.VectorStoreDocument{DocumentID: "document", CollectionID: collection.ID, CollectionUUID: collection.UUID}).Error)

	knowledgeBase := &schema.KnowledgeBaseInfo{KnowledgeBaseName: "renamed-knowledge-base", KnowledgeBaseType: "test", RAGID: collection.RAGID}
	require.NoError(t, db.Create(knowledgeBase).Error)
	require.NoError(t, db.Create(&schema.KnowledgeBaseEntry{KnowledgeBaseID: int64(knowledgeBase.ID), KnowledgeTitle: "entry", KnowledgeType: "test"}).Error)

	repository := &schema.EntityRepository{EntityBaseName: "renamed-entity-repository", Uuid: "repository-uuid", RAGID: collection.RAGID}
	require.NoError(t, db.Create(repository).Error)
	require.NoError(t, db.Create(&schema.ERModelEntity{RepositoryUUID: repository.Uuid, EntityName: "entity"}).Error)
	require.NoError(t, db.Create(&schema.ERModelRelationship{RepositoryUUID: repository.Uuid, Hash: "relationship-hash"}).Error)

	// RAGID is indexed but not unique in historical databases. Delete every
	// linked component so duplicate metadata cannot survive as leaked rows.
	duplicateKnowledgeBase := &schema.KnowledgeBaseInfo{KnowledgeBaseName: "duplicate-linked-knowledge-base", KnowledgeBaseType: "test", RAGID: collection.RAGID}
	require.NoError(t, db.Create(duplicateKnowledgeBase).Error)
	require.NoError(t, db.Create(&schema.KnowledgeBaseEntry{KnowledgeBaseID: int64(duplicateKnowledgeBase.ID), KnowledgeTitle: "duplicate-entry", KnowledgeType: "test"}).Error)
	duplicateRepository := &schema.EntityRepository{EntityBaseName: "duplicate-linked-entity-repository", Uuid: "duplicate-repository-uuid", RAGID: collection.RAGID}
	require.NoError(t, db.Create(duplicateRepository).Error)
	require.NoError(t, db.Create(&schema.ERModelEntity{RepositoryUUID: duplicateRepository.Uuid, EntityName: "duplicate-entity"}).Error)
	require.NoError(t, db.Create(&schema.ERModelRelationship{RepositoryUUID: duplicateRepository.Uuid, Hash: "duplicate-relationship-hash"}).Error)

	require.NoError(t, DeleteRAG(db, collection.Name))
	_, err = wrapper.AddWithError()
	require.Error(t, err, "DeleteRAG left its graph worker alive")
	assertRAGTableCounts(t, db, 0)
}

func TestMUSTPASS_DeleteRAGRepairsLegacyNameOnlyData(t *testing.T) {
	db, err := NewTemporaryRAGDB()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	name := "legacy-name-only"
	knowledgeBase := &schema.KnowledgeBaseInfo{KnowledgeBaseName: name, KnowledgeBaseType: "test"}
	require.NoError(t, db.Create(knowledgeBase).Error)
	require.NoError(t, db.Create(&schema.KnowledgeBaseEntry{KnowledgeBaseID: int64(knowledgeBase.ID), KnowledgeTitle: "entry", KnowledgeType: "test"}).Error)
	repository := &schema.EntityRepository{EntityBaseName: name, Uuid: "legacy-repository-uuid"}
	require.NoError(t, db.Create(repository).Error)
	require.NoError(t, db.Create(&schema.ERModelEntity{RepositoryUUID: repository.Uuid, EntityName: "entity"}).Error)

	require.NoError(t, DeleteRAG(db, name))
	assertRAGTableCounts(t, db, 0)
	// The endpoint remains idempotent when every component is already absent.
	require.NoError(t, DeleteRAG(db, name))
}

func TestMUSTPASS_DeleteRAGRollsBackInsteadOfLeakingPartialData(t *testing.T) {
	db, err := NewTemporaryRAGDB()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	collection := &schema.VectorStoreCollection{Name: "rollback-delete", UUID: "rollback-collection-uuid", RAGID: "rollback-rag-id", Dimension: 3}
	require.NoError(t, db.Create(collection).Error)
	knowledgeBase := &schema.KnowledgeBaseInfo{KnowledgeBaseName: collection.Name, KnowledgeBaseType: "test", RAGID: collection.RAGID}
	require.NoError(t, db.Create(knowledgeBase).Error)
	repository := &schema.EntityRepository{EntityBaseName: collection.Name, Uuid: "rollback-repository-uuid", RAGID: collection.RAGID}
	require.NoError(t, db.Create(repository).Error)

	// Force a child-table delete failure after the knowledge-base statements.
	require.NoError(t, db.DropTableIfExists(&schema.ERModelRelationship{}).Error)
	require.Error(t, DeleteRAG(db, collection.Name))

	var collectionCount, knowledgeBaseCount, repositoryCount int64
	require.NoError(t, db.Model(&schema.VectorStoreCollection{}).Count(&collectionCount).Error)
	require.NoError(t, db.Model(&schema.KnowledgeBaseInfo{}).Count(&knowledgeBaseCount).Error)
	require.NoError(t, db.Model(&schema.EntityRepository{}).Count(&repositoryCount).Error)
	require.Equal(t, int64(1), collectionCount)
	require.Equal(t, int64(1), knowledgeBaseCount)
	require.Equal(t, int64(1), repositoryCount)
}

func assertRAGTableCounts(t *testing.T, db *gorm.DB, expected int64) {
	t.Helper()
	models := []any{
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},
		&schema.KnowledgeBaseInfo{},
		&schema.KnowledgeBaseEntry{},
		&schema.EntityRepository{},
		&schema.ERModelEntity{},
		&schema.ERModelRelationship{},
	}
	for _, model := range models {
		var count int64
		require.NoError(t, db.Unscoped().Model(model).Count(&count).Error)
		require.Equalf(t, expected, count, "unexpected rows for %T", model)
	}
}
