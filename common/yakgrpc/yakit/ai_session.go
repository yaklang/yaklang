package yakit

import (
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// QueryAttachedAIReActScheduleUUIDs returns schedules whose lifecycle is owned
// by one of the supplied chats. Isolated schedules only retain an informational
// creation-session id and deliberately do not match this query.
func QueryAttachedAIReActScheduleUUIDs(projectDB *gorm.DB, sessionIDs []string) ([]string, error) {
	if projectDB == nil {
		return nil, utils.Errorf("projectDB is nil")
	}
	if !projectDB.HasTable((&schema.AIReActSchedule{}).TableName()) {
		return nil, nil
	}
	cleaned := make([]string, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if value := strings.TrimSpace(sessionID); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	if len(cleaned) == 0 {
		return nil, nil
	}
	var uuids []string
	err := projectDB.Model(&schema.AIReActSchedule{}).
		Where("target_mode = ? AND target_session_id IN (?)", schema.AIReActScheduleTargetContinueSession, cleaned).
		Pluck("uuid", &uuids).Error
	return uuids, err
}

func QueryAllAttachedAIReActScheduleUUIDs(projectDB *gorm.DB) ([]string, error) {
	if projectDB == nil {
		return nil, utils.Errorf("projectDB is nil")
	}
	if !projectDB.HasTable((&schema.AIReActSchedule{}).TableName()) {
		return nil, nil
	}
	var uuids []string
	err := projectDB.Model(&schema.AIReActSchedule{}).
		Where("target_mode = ?", schema.AIReActScheduleTargetContinueSession).
		Pluck("uuid", &uuids).Error
	return uuids, err
}

func DeleteAttachedAIReActSchedules(projectDB *gorm.DB, sessionIDs []string) (int64, error) {
	if projectDB == nil {
		return 0, utils.Errorf("projectDB is nil")
	}
	if !projectDB.HasTable((&schema.AIReActSchedule{}).TableName()) {
		return 0, nil
	}
	cleaned := make([]string, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if value := strings.TrimSpace(sessionID); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	if len(cleaned) == 0 {
		return 0, nil
	}
	result := projectDB.Where("target_mode = ? AND target_session_id IN (?)", schema.AIReActScheduleTargetContinueSession, cleaned).
		Delete(&schema.AIReActSchedule{})
	return result.RowsAffected, result.Error
}

func DeleteAllAttachedAIReActSchedules(projectDB *gorm.DB) (int64, error) {
	if projectDB == nil {
		return 0, utils.Errorf("projectDB is nil")
	}
	if !projectDB.HasTable((&schema.AIReActSchedule{}).TableName()) {
		return 0, nil
	}
	result := projectDB.Where("target_mode = ?", schema.AIReActScheduleTargetContinueSession).
		Delete(&schema.AIReActSchedule{})
	return result.RowsAffected, result.Error
}

// DeleteAISession deletes all persistent-session scoped data from projectDB:
// - AIAgentRuntime rows (by persistent_session)
// - AiCheckpoint, AiOutputEvent rows and their process associations (by session_id/runtime)
func DeleteAISession(projectDB *gorm.DB, sessionId string) (deletedRuntimes int64, deletedEvents int64, err error) {
	if sessionId == "" {
		return 0, 0, utils.Errorf("sessionId is empty")
	}
	if projectDB == nil {
		return 0, 0, utils.Errorf("projectDB is nil")
	}
	if _, err = DeleteAttachedAIReActSchedules(projectDB, []string{sessionId}); err != nil {
		return 0, 0, err
	}

	_, err = DeleteAISessionMetaBySessionID(projectDB, sessionId)
	if err != nil {
		return 0, 0, err
	}

	coordinatorUUIDs, err := QueryAgentRuntimeUUIDsBySessionID(projectDB, sessionId)
	if err != nil {
		return 0, 0, err
	}

	deletedRuntimes, err = DeleteAgentRuntime(projectDB, &ypb.AITaskFilter{
		SessionID: []string{sessionId},
	})
	if err != nil {
		return 0, 0, err
	}

	if _, err = DeleteCheckpointByCoordinatorUUIDs(projectDB, coordinatorUUIDs); err != nil {
		return deletedRuntimes, 0, err
	}

	deletedEvents, err = DeleteAIEventBySessionID(projectDB, sessionId)
	if err != nil {
		return deletedRuntimes, 0, err
	}

	if err = DeleteAISessionPlanAndExecBySessionID(projectDB, sessionId); err != nil {
		return deletedRuntimes, deletedEvents, err
	}

	return deletedRuntimes, deletedEvents, nil
}

// DeleteAllAISessionData deletes all session-scoped data from projectDB:
// - AISession meta, AIAgentRuntime, AiCheckpoint, AiOutputEvent + associations, AISessionPlanAndExec
func DeleteAllAISessionData(projectDB *gorm.DB) (deletedSessions int64, deletedRuntimes int64, deletedEvents int64, deletedPlanExec int64, err error) {
	if projectDB == nil {
		return 0, 0, 0, 0, utils.Errorf("projectDB is nil")
	}
	if _, err = DeleteAllAttachedAIReActSchedules(projectDB); err != nil {
		return 0, 0, 0, 0, err
	}

	deletedSessions, err = DeleteAllAISessionMeta(projectDB)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	deletedRuntimes, err = DeleteAllAgentRuntime(projectDB)
	if err != nil {
		return deletedSessions, 0, 0, 0, err
	}

	if _, err = DeleteAllCheckpoint(projectDB); err != nil {
		return deletedSessions, deletedRuntimes, 0, 0, err
	}

	deletedEvents, err = DeleteAllAIEventWithCount(projectDB)
	if err != nil {
		return deletedSessions, deletedRuntimes, 0, 0, err
	}

	deletedPlanExec, err = DeleteAllAISessionPlanAndExec(projectDB)
	if err != nil {
		return deletedSessions, deletedRuntimes, deletedEvents, 0, err
	}

	return deletedSessions, deletedRuntimes, deletedEvents, deletedPlanExec, nil
}
