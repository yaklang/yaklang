package yakit

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestDeleteAISession_DeletesRuntimeAndEvents(t *testing.T) {
	projectDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, projectDB.AutoMigrate(&schema.AISession{}, &schema.AIAgentRuntime{}, &schema.AiCheckpoint{}, &schema.AiOutputEvent{}, &schema.AiProcessAndAiEvent{}).Error)

	sessionA := "sess-" + uuid.NewString()
	sessionB := "sess-" + uuid.NewString()

	// runtimes (project DB)
	runtimeA1 := uuid.NewString()
	runtimeA2 := uuid.NewString()
	runtimeB1 := uuid.NewString()
	require.NoError(t, projectDB.Create(&schema.AIAgentRuntime{Uuid: runtimeA1, PersistentSession: sessionA, Name: "a1"}).Error)
	require.NoError(t, projectDB.Create(&schema.AIAgentRuntime{Uuid: runtimeA2, PersistentSession: sessionA, Name: "a2"}).Error)
	require.NoError(t, projectDB.Create(&schema.AIAgentRuntime{Uuid: runtimeB1, PersistentSession: sessionB, Name: "b1"}).Error)

	require.NoError(t, projectDB.Create(&schema.AiCheckpoint{
		CoordinatorUuid: runtimeA1,
		Seq:             1,
		Type:            schema.AiCheckpointType_AIInteractive,
	}).Error)
	require.NoError(t, projectDB.Create(&schema.AiCheckpoint{
		CoordinatorUuid: runtimeA2,
		Seq:             1,
		Type:            schema.AiCheckpointType_ToolCall,
	}).Error)
	require.NoError(t, projectDB.Create(&schema.AiCheckpoint{
		CoordinatorUuid: runtimeB1,
		Seq:             1,
		Type:            schema.AiCheckpointType_Review,
	}).Error)

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

	deletedRuntimes, deletedEvents, err := DeleteAISession(projectDB, sessionA)
	require.NoError(t, err)
	require.Equal(t, int64(2), deletedRuntimes)
	require.Equal(t, int64(2), deletedEvents)

	var runtimeCount int64
	require.NoError(t, projectDB.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", sessionA).Count(&runtimeCount).Error)
	require.Equal(t, int64(0), runtimeCount)
	require.NoError(t, projectDB.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", sessionB).Count(&runtimeCount).Error)
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

	var checkpointCount int64
	require.NoError(t, projectDB.Model(&schema.AiCheckpoint{}).Where("coordinator_uuid IN (?)", []string{runtimeA1, runtimeA2}).Count(&checkpointCount).Error)
	require.Equal(t, int64(0), checkpointCount)
	require.NoError(t, projectDB.Model(&schema.AiCheckpoint{}).Where("coordinator_uuid = ?", runtimeB1).Count(&checkpointCount).Error)
	require.Equal(t, int64(1), checkpointCount)
}

func TestDeleteAllAISessionData(t *testing.T) {
	projectDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)

	require.NoError(t, projectDB.AutoMigrate(
		&schema.AISession{},
		&schema.AIAgentRuntime{},
		&schema.AiCheckpoint{},
		&schema.AiOutputEvent{},
		&schema.AiProcessAndAiEvent{},
		&schema.AISessionPlanAndExec{},
	).Error)

	sessionA := "sess-" + uuid.NewString()
	sessionB := "sess-" + uuid.NewString()

	_, err = CreateOrUpdateAISessionMeta(projectDB, sessionA, "title-a")
	require.NoError(t, err)
	_, err = CreateOrUpdateAISessionMeta(projectDB, sessionB, "title-b")
	require.NoError(t, err)

	require.NoError(t, projectDB.Create(&schema.AISessionPlanAndExec{
		SessionID:     sessionA,
		CoordinatorID: "coord-a",
		TaskTree:      "{}",
		TaskProgress:  "{}",
	}).Error)
	require.NoError(t, projectDB.Create(&schema.AISessionPlanAndExec{
		SessionID:     sessionB,
		CoordinatorID: "coord-b",
		TaskTree:      "{}",
		TaskProgress:  "{}",
	}).Error)

	runtimeA1 := uuid.NewString()
	runtimeA2 := uuid.NewString()
	runtimeB1 := uuid.NewString()
	require.NoError(t, projectDB.Create(&schema.AIAgentRuntime{Uuid: runtimeA1, PersistentSession: sessionA, Name: "a1"}).Error)
	require.NoError(t, projectDB.Create(&schema.AIAgentRuntime{Uuid: runtimeA2, PersistentSession: sessionA, Name: "a2"}).Error)
	require.NoError(t, projectDB.Create(&schema.AIAgentRuntime{Uuid: runtimeB1, PersistentSession: sessionB, Name: "b1"}).Error)

	require.NoError(t, projectDB.Create(&schema.AiCheckpoint{
		CoordinatorUuid: runtimeA1,
		Seq:             1,
		Type:            schema.AiCheckpointType_AIInteractive,
	}).Error)
	require.NoError(t, projectDB.Create(&schema.AiCheckpoint{
		CoordinatorUuid: runtimeA2,
		Seq:             1,
		Type:            schema.AiCheckpointType_ToolCall,
	}).Error)
	require.NoError(t, projectDB.Create(&schema.AiCheckpoint{
		CoordinatorUuid: runtimeB1,
		Seq:             1,
		Type:            schema.AiCheckpointType_Review,
	}).Error)

	e1 := uuid.NewString()
	e2 := uuid.NewString()
	e3 := uuid.NewString()
	require.NoError(t, projectDB.Create(&schema.AiOutputEvent{EventUUID: e1, SessionId: sessionA}).Error)
	require.NoError(t, projectDB.Create(&schema.AiOutputEvent{EventUUID: e2, SessionId: sessionA}).Error)
	require.NoError(t, projectDB.Create(&schema.AiOutputEvent{EventUUID: e3, SessionId: sessionB}).Error)
	require.NoError(t, projectDB.Create(&schema.AiProcessAndAiEvent{ProcessesId: "p1", EventId: e1}).Error)
	require.NoError(t, projectDB.Create(&schema.AiProcessAndAiEvent{ProcessesId: "p2", EventId: e2}).Error)
	require.NoError(t, projectDB.Create(&schema.AiProcessAndAiEvent{ProcessesId: "p3", EventId: e3}).Error)

	deletedSessions, deletedRuntimes, deletedEvents, deletedPlanExec, err := DeleteAllAISessionData(projectDB)
	require.NoError(t, err)
	require.Equal(t, int64(2), deletedSessions)
	require.Equal(t, int64(3), deletedRuntimes)
	require.Equal(t, int64(3), deletedEvents)
	require.Equal(t, int64(2), deletedPlanExec)

	var count int64
	require.NoError(t, projectDB.Model(&schema.AIAgentRuntime{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
	require.NoError(t, projectDB.Model(&schema.AiOutputEvent{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
	require.NoError(t, projectDB.Model(&schema.AiProcessAndAiEvent{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
	require.NoError(t, projectDB.Model(&schema.AiCheckpoint{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
	require.NoError(t, projectDB.Model(&schema.AISession{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
	require.NoError(t, projectDB.Model(&schema.AISessionPlanAndExec{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestQueryAISessionIDsForDelete_ByAfterTimestamp(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	sessionOld := "sess-old-" + uuid.NewString()
	sessionNew := "sess-new-" + uuid.NewString()

	_, err = CreateOrUpdateAISessionMeta(db, sessionOld, "old")
	require.NoError(t, err)
	_, err = CreateOrUpdateAISessionMeta(db, sessionNew, "new")
	require.NoError(t, err)

	oldTime := time.Unix(1000, 0)
	newTime := time.Unix(2000, 0)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", sessionOld).UpdateColumn("updated_at", oldTime).Error)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", sessionNew).UpdateColumn("updated_at", newTime).Error)

	sessionIDs, err := QueryAISessionIDsForDelete(db, &ypb.DeleteAISessionFilter{
		AfterTimestamp: 1500,
	}, false)
	require.NoError(t, err)
	require.Equal(t, []string{sessionNew}, sessionIDs)
}

func TestQueryAISessionIDsForDelete_ByBeforeTimestamp(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	sessionOld := "sess-before-old-" + uuid.NewString()
	sessionNew := "sess-before-new-" + uuid.NewString()

	_, err = CreateOrUpdateAISessionMeta(db, sessionOld, "old")
	require.NoError(t, err)
	_, err = CreateOrUpdateAISessionMeta(db, sessionNew, "new")
	require.NoError(t, err)

	oldTime := time.Unix(1000, 0)
	newTime := time.Unix(2000, 0)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", sessionOld).UpdateColumn("updated_at", oldTime).Error)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", sessionNew).UpdateColumn("updated_at", newTime).Error)

	sessionIDs, err := QueryAISessionIDsForDelete(db, &ypb.DeleteAISessionFilter{
		BeforeTimestamp: 1500,
	}, false)
	require.NoError(t, err)
	require.Equal(t, []string{sessionOld}, sessionIDs)
}

func TestQueryAISessionIDsForDelete_ByTimestampRange(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	sessionOld := "sess-range-old-" + uuid.NewString()
	sessionMid := "sess-range-mid-" + uuid.NewString()
	sessionNew := "sess-range-new-" + uuid.NewString()

	_, err = CreateOrUpdateAISessionMeta(db, sessionOld, "old")
	require.NoError(t, err)
	_, err = CreateOrUpdateAISessionMeta(db, sessionMid, "mid")
	require.NoError(t, err)
	_, err = CreateOrUpdateAISessionMeta(db, sessionNew, "new")
	require.NoError(t, err)

	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", sessionOld).UpdateColumn("updated_at", time.Unix(1000, 0)).Error)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", sessionMid).UpdateColumn("updated_at", time.Unix(2000, 0)).Error)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", sessionNew).UpdateColumn("updated_at", time.Unix(3000, 0)).Error)

	sessionIDs, err := QueryAISessionIDsForDelete(db, &ypb.DeleteAISessionFilter{
		AfterTimestamp:  1500,
		BeforeTimestamp: 2500,
	}, false)
	require.NoError(t, err)
	require.Equal(t, []string{sessionMid}, sessionIDs)
}

func TestQueryAISessionIDsForDelete_DeleteAll(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	sessionA := "sess-a-" + uuid.NewString()
	sessionB := "sess-b-" + uuid.NewString()
	_, err = CreateOrUpdateAISessionMeta(db, sessionA, "a")
	require.NoError(t, err)
	_, err = CreateOrUpdateAISessionMeta(db, sessionB, "b")
	require.NoError(t, err)

	sessionIDs, err := QueryAISessionIDsForDelete(db, nil, true)
	require.NoError(t, err)
	require.Len(t, sessionIDs, 2)
	require.Contains(t, sessionIDs, sessionA)
	require.Contains(t, sessionIDs, sessionB)
}

func TestQueryAllAISessionMetaOrderByUpdated(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	sessionOld := "sess-order-old-" + uuid.NewString()
	sessionNew := "sess-order-new-" + uuid.NewString()
	_, err = CreateOrUpdateAISessionMeta(db, sessionOld, "old")
	require.NoError(t, err)
	_, err = CreateOrUpdateAISessionMeta(db, sessionNew, "new")
	require.NoError(t, err)

	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", sessionOld).UpdateColumn("updated_at", time.Unix(1000, 0)).Error)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", sessionNew).UpdateColumn("updated_at", time.Unix(2000, 0)).Error)

	records, err := QueryAllAISessionMetaOrderByUpdated(db)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(records), 2)
	require.Equal(t, sessionNew, records[0].SessionID)
	require.Equal(t, sessionOld, records[1].SessionID)
}
