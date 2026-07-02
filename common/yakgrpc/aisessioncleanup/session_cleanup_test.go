package aisessioncleanup

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&schema.AISession{},
		&schema.AIAgentRuntime{},
		&schema.AIMemoryEntity{},
		&schema.AIMemoryCollection{},
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},
		&schema.EntityRepository{},
		&schema.ERModelEntity{},
		&schema.ERModelRelationship{},
		&schema.KnowledgeBaseInfo{},
		&schema.KnowledgeBaseEntry{},
	).Error)
	return db
}

func TestDeleteSessionArtifacts_RemovesMidtermSessionAndFork(t *testing.T) {
	db := setupTestDB(t)

	persistentSessionID := "Fp4wxob9ZAhrAsctD4owIdtW5TYtnEDK7SULK1Xi"
	otherPersistentSessionID := uuid.NewString()
	midtermSessionID := memoryMidtermSessionID(persistentSessionID)
	forkSessionID := midtermSessionID + "@fork1-1"
	forkRAGName := ragMidtermTableName(persistentSessionID) + "@fork1-1"
	baseRAGName := ragMidtermTableName(persistentSessionID)
	otherRAGName := ragMidtermTableName(otherPersistentSessionID)

	seedMidtermSessionArtifacts(t, db, persistentSessionID, midtermSessionID, baseRAGName, forkSessionID, forkRAGName)
	seedMidtermSessionArtifacts(t, db, otherPersistentSessionID, memoryMidtermSessionID(otherPersistentSessionID), otherRAGName, "", "")

	result, err := DeleteSessionArtifacts(db, persistentSessionID)
	require.NoError(t, err)
	require.Equal(t, int64(2), result.DeletedMemoryEntities)
	require.Equal(t, int64(2), result.DeletedMemoryCollections)
	require.Equal(t, int64(2), result.DeletedRAGCollections)
	require.Equal(t, int64(2), result.DeletedRAGDocuments)
	require.Equal(t, int64(2), result.DeletedEntityRepositories)
	require.Equal(t, int64(2), result.DeletedEntityRelationships)
	require.Equal(t, int64(1), result.DeletedERModelEntities)
	require.Equal(t, int64(2), result.DeletedKnowledgeBases)
	require.Equal(t, int64(2), result.DeletedKnowledgeEntries)

	assertZeroCount(t, db.Model(&schema.AIMemoryEntity{}).Where("session_id IN (?)", []string{persistentSessionID, midtermSessionID, forkSessionID}))
	assertZeroCount(t, db.Model(&schema.VectorStoreCollection{}).Where("name IN (?)", []string{baseRAGName, forkRAGName}))
	assertOneCount(t, db.Model(&schema.VectorStoreCollection{}).Where("name = ?", otherRAGName))
	assertOneCount(t, db.Model(&schema.AIMemoryEntity{}).Where("session_id = ?", memoryMidtermSessionID(otherPersistentSessionID)))
}

func TestDeleteSessionArtifacts_EmptySessionID(t *testing.T) {
	db := setupTestDB(t)
	_, err := DeleteSessionArtifacts(db, "")
	require.Error(t, err)
}

func TestDeleteSessionArtifacts_NoData(t *testing.T) {
	db := setupTestDB(t)
	result, err := DeleteSessionArtifacts(db, uuid.NewString())
	require.NoError(t, err)
	require.Equal(t, int64(0), result.DeletedMemoryEntities)
	require.Equal(t, int64(0), result.DeletedMemoryCollections)
	require.Equal(t, int64(0), result.DeletedRAGCollections)
}

func TestDeleteAllSessionArtifacts_RemovesAllMidtermRAG(t *testing.T) {
	db := setupTestDB(t)

	sessA := uuid.NewString()
	sessB := uuid.NewString()
	seedMidtermSessionArtifacts(t, db, sessA, memoryMidtermSessionID(sessA), ragMidtermTableName(sessA), memoryMidtermSessionID(sessA)+"@fork1-1", ragMidtermTableName(sessA)+"@fork1-1")
	seedMidtermSessionArtifacts(t, db, sessB, memoryMidtermSessionID(sessB), ragMidtermTableName(sessB), "", "")

	unrelatedRepo := &schema.EntityRepository{EntityBaseName: "knowledge-base-keep", Uuid: uuid.NewString()}
	require.NoError(t, db.Create(unrelatedRepo).Error)
	require.NoError(t, db.Create(&schema.ERModelRelationship{
		RepositoryUUID: unrelatedRepo.Uuid, Uuid: uuid.NewString(),
		SourceEntityIndex: uuid.NewString(), TargetEntityIndex: uuid.NewString(),
		RelationshipType: "rel",
	}).Error)

	unrelatedKB := &schema.KnowledgeBaseInfo{KnowledgeBaseName: "knowledge-base-keep", KnowledgeBaseType: "kb"}
	require.NoError(t, db.Create(unrelatedKB).Error)
	require.NoError(t, db.Create(&schema.KnowledgeBaseEntry{
		KnowledgeBaseID: int64(unrelatedKB.ID),
		KnowledgeTitle:  "keep-knowledge",
		KnowledgeType:   "fact",
		HiddenIndex:     uuid.NewString(),
	}).Error)

	unrelatedCol := &schema.VectorStoreCollection{Name: "knowledge-base-keep"}
	require.NoError(t, db.Create(unrelatedCol).Error)
	require.NoError(t, db.Create(&schema.VectorStoreDocument{DocumentID: "kb-doc", CollectionID: unrelatedCol.ID}).Error)

	result, err := DeleteAllSessionArtifacts(db)
	require.NoError(t, err)
	require.Equal(t, int64(3), result.DeletedMemoryEntities)
	require.Equal(t, int64(3), result.DeletedMemoryCollections)
	require.Equal(t, int64(3), result.DeletedRAGCollections)
	require.Equal(t, int64(3), result.DeletedRAGDocuments)
	require.Equal(t, int64(3), result.DeletedEntityRepositories)
	require.Equal(t, int64(3), result.DeletedEntityRelationships)
	require.Equal(t, int64(2), result.DeletedERModelEntities)
	require.Equal(t, int64(3), result.DeletedKnowledgeBases)
	require.Equal(t, int64(3), result.DeletedKnowledgeEntries)

	assertZeroCount(t, db.Model(&schema.VectorStoreCollection{}).Where("name LIKE ?", ragMidtermTableNamePrefix+"%"))
	assertOneCount(t, db.Model(&schema.VectorStoreCollection{}).Where("name = ?", "knowledge-base-keep"))
	assertOneCount(t, db.Model(&schema.EntityRepository{}).Where("uuid = ?", unrelatedRepo.Uuid))
	assertOneCount(t, db.Model(&schema.KnowledgeBaseInfo{}).Where("id = ?", unrelatedKB.ID))
}

func seedMidtermSessionArtifacts(
	t *testing.T,
	db *gorm.DB,
	persistentSessionID, midtermSessionID, baseRAGName, forkSessionID, forkRAGName string,
) {
	t.Helper()

	require.NoError(t, db.Create(&schema.AIMemoryEntity{
		MemoryID: uuid.NewString(), SessionID: midtermSessionID, Content: "midterm",
	}).Error)

	baseCol := &schema.VectorStoreCollection{Name: baseRAGName}
	require.NoError(t, db.Create(baseCol).Error)
	require.NoError(t, db.Create(&schema.VectorStoreDocument{
		DocumentID: uuid.NewString(), CollectionID: baseCol.ID, Content: "base-doc",
	}).Error)

	baseRepo := &schema.EntityRepository{EntityBaseName: baseRAGName, Uuid: uuid.NewString()}
	require.NoError(t, db.Create(baseRepo).Error)
	require.NoError(t, db.Create(&schema.ERModelEntity{
		RepositoryUUID: baseRepo.Uuid, EntityName: "base-entity", Uuid: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&schema.ERModelRelationship{
		RepositoryUUID: baseRepo.Uuid, Uuid: uuid.NewString(),
		SourceEntityIndex: uuid.NewString(), TargetEntityIndex: uuid.NewString(),
		RelationshipType: "rel",
	}).Error)

	baseKB := &schema.KnowledgeBaseInfo{KnowledgeBaseName: baseRAGName, KnowledgeBaseType: "session"}
	require.NoError(t, db.Create(baseKB).Error)
	require.NoError(t, db.Create(&schema.KnowledgeBaseEntry{
		KnowledgeBaseID: int64(baseKB.ID), KnowledgeTitle: "base-knowledge",
		KnowledgeType: "fact", HiddenIndex: uuid.NewString(),
	}).Error)

	require.NoError(t, db.Create(&schema.AIMemoryCollection{SessionID: midtermSessionID}).Error)

	if forkSessionID == "" || forkRAGName == "" {
		return
	}

	require.NoError(t, db.Create(&schema.AIMemoryEntity{
		MemoryID: uuid.NewString(), SessionID: forkSessionID, Content: "fork",
	}).Error)
	require.NoError(t, db.Create(&schema.AIMemoryCollection{SessionID: forkSessionID}).Error)

	forkCol := &schema.VectorStoreCollection{Name: forkRAGName}
	require.NoError(t, db.Create(forkCol).Error)
	require.NoError(t, db.Create(&schema.VectorStoreDocument{
		DocumentID: uuid.NewString(), CollectionID: forkCol.ID, Content: "fork-doc",
	}).Error)

	forkRepo := &schema.EntityRepository{EntityBaseName: forkRAGName, Uuid: uuid.NewString()}
	require.NoError(t, db.Create(forkRepo).Error)
	require.NoError(t, db.Create(&schema.ERModelRelationship{
		RepositoryUUID: forkRepo.Uuid, Uuid: uuid.NewString(),
		SourceEntityIndex: uuid.NewString(), TargetEntityIndex: uuid.NewString(),
		RelationshipType: "rel",
	}).Error)

	forkKB := &schema.KnowledgeBaseInfo{KnowledgeBaseName: forkRAGName, KnowledgeBaseType: "session"}
	require.NoError(t, db.Create(forkKB).Error)
	require.NoError(t, db.Create(&schema.KnowledgeBaseEntry{
		KnowledgeBaseID: int64(forkKB.ID), KnowledgeTitle: "fork-knowledge",
		KnowledgeType: "fact", HiddenIndex: uuid.NewString(),
	}).Error)
}

func assertZeroCount(t *testing.T, q *gorm.DB) {
	t.Helper()
	var count int64
	require.NoError(t, q.Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func assertOneCount(t *testing.T, q *gorm.DB) {
	t.Helper()
	var count int64
	require.NoError(t, q.Count(&count).Error)
	require.Equal(t, int64(1), count)
}
