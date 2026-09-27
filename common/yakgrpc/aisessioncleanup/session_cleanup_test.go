package aisessioncleanup

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AIMemoryEntity{}, &schema.AIMemoryCollection{},
		&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}).Error)
	return db
}

func seedMemory(t *testing.T, db *gorm.DB, session string) {
	t.Helper()
	require.NoError(t, db.Create(&schema.AIMemoryEntity{MemoryID: uuid.NewString(), SessionID: session, Content: "memory"}).Error)
	require.NoError(t, db.Create(&schema.AIMemoryCollection{SessionID: session}).Error)
	collection := &schema.VectorStoreCollection{Name: "ai-memory-" + session}
	require.NoError(t, db.Create(collection).Error)
	require.NoError(t, db.Create(&schema.VectorStoreDocument{DocumentID: uuid.NewString(), CollectionID: collection.ID, Content: "document"}).Error)
}

func assertCount(t *testing.T, q *gorm.DB, want int64) {
	t.Helper()
	var count int64
	require.NoError(t, q.Count(&count).Error)
	require.Equal(t, want, count)
}

func TestDeleteSessionArtifactsExactSession(t *testing.T) {
	db := setupTestDB(t)
	// Neither LIKE wildcards nor prefix collisions can widen the deletion.
	for _, session := range []string{"session_%", "session_%other", "session_AB"} {
		seedMemory(t, db, session)
	}
	result, err := DeleteSessionArtifacts(db, "session_%")
	require.NoError(t, err)
	require.Equal(t, &SessionCleanupResult{DeletedMemoryEntities: 1, DeletedMemoryCollections: 1, DeletedRAGCollections: 1, DeletedRAGDocuments: 1}, result)
	assertCount(t, db.Model(&schema.AIMemoryEntity{}).Where("session_id = ?", "session_%"), 0)
	assertCount(t, db.Model(&schema.AIMemoryEntity{}), 2)
	assertCount(t, db.Model(&schema.AIMemoryCollection{}), 2)
	assertCount(t, db.Model(&schema.VectorStoreCollection{}), 2)
	assertCount(t, db.Model(&schema.VectorStoreDocument{}), 2)
	result, err = DeleteSessionArtifacts(db, "session_%")
	require.NoError(t, err)
	require.Equal(t, &SessionCleanupResult{}, result)
}

func TestDeleteSessionArtifactsValidatesInput(t *testing.T) {
	db := setupTestDB(t)
	_, err := DeleteSessionArtifacts(db, " ")
	require.Error(t, err)
	_, err = DeleteSessionArtifacts(nil, "session")
	require.Error(t, err)
	_, err = DeleteAllAIMemoryArtifacts(nil)
	require.Error(t, err)
}

func TestDeleteAllAIMemoryArtifactsKeepsUnrelatedVectors(t *testing.T) {
	db := setupTestDB(t)
	seedMemory(t, db, "one")
	seedMemory(t, db, "two")
	other := &schema.VectorStoreCollection{Name: "knowledge-base-keep"}
	require.NoError(t, db.Create(other).Error)
	require.NoError(t, db.Create(&schema.VectorStoreDocument{DocumentID: uuid.NewString(), CollectionID: other.ID}).Error)
	result, err := DeleteAllAIMemoryArtifacts(db)
	require.NoError(t, err)
	require.Equal(t, &SessionCleanupResult{DeletedMemoryEntities: 2, DeletedMemoryCollections: 2, DeletedRAGCollections: 2, DeletedRAGDocuments: 2}, result)
	assertCount(t, db.Model(&schema.AIMemoryEntity{}), 0)
	assertCount(t, db.Model(&schema.AIMemoryCollection{}), 0)
	assertCount(t, db.Model(&schema.VectorStoreCollection{}).Where("name = ?", other.Name), 1)
	assertCount(t, db.Model(&schema.VectorStoreDocument{}), 1)
	// Ordinary storage remains usable after cleanup.
	seedMemory(t, db, "next")
	assertCount(t, db.Model(&schema.AIMemoryEntity{}), 1)
}

func TestDeleteSessionArtifactsRollsBackOnStorageError(t *testing.T) {
	db := setupTestDB(t)
	seedMemory(t, db, "one")
	require.NoError(t, db.Exec("CREATE TRIGGER reject_memory_delete BEFORE DELETE ON ai_memory_entities_v1 BEGIN SELECT RAISE(ABORT, 'injected failure'); END").Error)
	result, err := DeleteSessionArtifacts(db, "one")
	require.Error(t, err)
	require.Equal(t, &SessionCleanupResult{}, result)
	assertCount(t, db.Model(&schema.AIMemoryEntity{}), 1)
	assertCount(t, db.Model(&schema.AIMemoryCollection{}), 1)
	assertCount(t, db.Model(&schema.VectorStoreCollection{}), 1)
	assertCount(t, db.Model(&schema.VectorStoreDocument{}), 1)
}
