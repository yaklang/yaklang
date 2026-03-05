package yakit

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestDeleteAgentRuntime_FilterBySessionID(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AIAgentRuntime{}).Error)

	sessionA := "sess-" + uuid.NewString()
	sessionB := "sess-" + uuid.NewString()

	require.NoError(t, db.Create(&schema.AIAgentRuntime{
		Uuid:              uuid.NewString(),
		Name:              "a-1",
		PersistentSession: sessionA,
	}).Error)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{
		Uuid:              uuid.NewString(),
		Name:              "a-2",
		PersistentSession: sessionA,
	}).Error)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{
		Uuid:              uuid.NewString(),
		Name:              "b-1",
		PersistentSession: sessionB,
	}).Error)

	affected, err := DeleteAgentRuntime(db, &ypb.AITaskFilter{
		SessionID: []string{sessionA},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), affected)

	var countA int64
	require.NoError(t, db.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", sessionA).Count(&countA).Error)
	require.Equal(t, int64(0), countA)

	var countB int64
	require.NoError(t, db.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", sessionB).Count(&countB).Error)
	require.Equal(t, int64(1), countB)
}
