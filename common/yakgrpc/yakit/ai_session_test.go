package yakit

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestDeleteAISession_DeletesRuntimeAndEvents(t *testing.T) {
	profileDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	projectDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)

	require.NoError(t, profileDB.AutoMigrate(&schema.AIAgentRuntime{}).Error)
	require.NoError(t, projectDB.AutoMigrate(&schema.AiOutputEvent{}, &schema.AiProcessAndAiEvent{}).Error)

	sessionA := "sess-" + uuid.NewString()
	sessionB := "sess-" + uuid.NewString()

	// runtimes (profile DB)
	require.NoError(t, profileDB.Create(&schema.AIAgentRuntime{Uuid: uuid.NewString(), PersistentSession: sessionA, Name: "a1"}).Error)
	require.NoError(t, profileDB.Create(&schema.AIAgentRuntime{Uuid: uuid.NewString(), PersistentSession: sessionA, Name: "a2"}).Error)
	require.NoError(t, profileDB.Create(&schema.AIAgentRuntime{Uuid: uuid.NewString(), PersistentSession: sessionB, Name: "b1"}).Error)

	// events + associations (project DB)
	e1 := uuid.NewString()
	e2 := uuid.NewString()
	e3 := uuid.NewString()
	require.NoError(t, projectDB.Create(&schema.AiOutputEvent{EventUUID: e1, SessionId: sessionA}).Error)
	require.NoError(t, projectDB.Create(&schema.AiOutputEvent{EventUUID: e2, SessionId: sessionA}).Error)
	require.NoError(t, projectDB.Create(&schema.AiOutputEvent{EventUUID: e3, SessionId: sessionB}).Error)
	require.NoError(t, projectDB.Create(&schema.AiProcessAndAiEvent{ProcessesId: "p1", EventId: e1}).Error)
	require.NoError(t, projectDB.Create(&schema.AiProcessAndAiEvent{ProcessesId: "p2", EventId: e2}).Error)
	require.NoError(t, projectDB.Create(&schema.AiProcessAndAiEvent{ProcessesId: "p3", EventId: e3}).Error)

	deletedRuntimes, deletedEvents, err := DeleteAISession(profileDB, projectDB, sessionA)
	require.NoError(t, err)
	require.Equal(t, int64(2), deletedRuntimes)
	require.Equal(t, int64(2), deletedEvents)

	var runtimeCount int64
	require.NoError(t, profileDB.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", sessionA).Count(&runtimeCount).Error)
	require.Equal(t, int64(0), runtimeCount)
	require.NoError(t, profileDB.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", sessionB).Count(&runtimeCount).Error)
	require.Equal(t, int64(1), runtimeCount)

	var eventCount int64
	require.NoError(t, projectDB.Model(&schema.AiOutputEvent{}).Where("session_id = ?", sessionA).Count(&eventCount).Error)
	require.Equal(t, int64(0), eventCount)
	require.NoError(t, projectDB.Model(&schema.AiOutputEvent{}).Where("session_id = ?", sessionB).Count(&eventCount).Error)
	require.Equal(t, int64(1), eventCount)

	var assocCount int64
	require.NoError(t, projectDB.Model(&schema.AiProcessAndAiEvent{}).Where("event_id IN (?)", []string{e1, e2}).Count(&assocCount).Error)
	require.Equal(t, int64(0), assocCount)
	require.NoError(t, projectDB.Model(&schema.AiProcessAndAiEvent{}).Where("event_id = ?", e3).Count(&assocCount).Error)
	require.Equal(t, int64(1), assocCount)
}
