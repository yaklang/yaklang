package aisessioncleanup

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestReconcileOrphanArtifacts_RemovesOrphanMemory(t *testing.T) {
	projectDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, projectDB.AutoMigrate(
		&schema.AISession{},
		&schema.AIAgentRuntime{},
		&schema.AIMemoryEntity{},
		&schema.AIMemoryCollection{},
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},
	).Error)

	validSessionID := "sess-valid-" + uuid.NewString()
	orphanSessionID := "sess-orphan-" + uuid.NewString()

	_, err = yakit.CreateOrUpdateAISessionMeta(projectDB, validSessionID, "valid")
	require.NoError(t, err)

	require.NoError(t, projectDB.Create(&schema.AIMemoryEntity{
		MemoryID:  uuid.NewString(),
		SessionID: validSessionID,
		Content:   "keep",
	}).Error)
	require.NoError(t, projectDB.Create(&schema.AIMemoryEntity{
		MemoryID:  uuid.NewString(),
		SessionID: orphanSessionID,
		Content:   "remove",
	}).Error)
	require.NoError(t, projectDB.Create(&schema.AIMemoryCollection{
		SessionID: orphanSessionID,
	}).Error)

	result, err := ReconcileOrphanArtifacts(projectDB)
	require.NoError(t, err)
	require.Equal(t, int64(1), result.DeletedMemoryEntities)
	require.Equal(t, int64(1), result.DeletedMemoryCollections)
	require.Equal(t, 0, result.DeletedWorkDirs)

	var validMemoryCount int64
	require.NoError(t, projectDB.Model(&schema.AIMemoryEntity{}).Where("session_id = ?", validSessionID).Count(&validMemoryCount).Error)
	require.Equal(t, int64(1), validMemoryCount)

	var orphanMemoryCount int64
	require.NoError(t, projectDB.Model(&schema.AIMemoryEntity{}).Where("session_id = ?", orphanSessionID).Count(&orphanMemoryCount).Error)
	require.Equal(t, int64(0), orphanMemoryCount)
}
