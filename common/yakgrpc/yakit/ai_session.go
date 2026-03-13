package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// DeleteAISession deletes all persistent-session scoped data:
// - profileDB: AIAgentRuntime rows (by persistent_session)
// - projectDB: AiOutputEvent rows and their process associations (by session_id)
func DeleteAISession(profileDB, projectDB *gorm.DB, sessionId string) (deletedRuntimes int64, deletedEvents int64, err error) {
	if sessionId == "" {
		return 0, 0, utils.Errorf("sessionId is empty")
	}
	if profileDB == nil {
		return 0, 0, utils.Errorf("profileDB is nil")
	}
	if projectDB == nil {
		return 0, 0, utils.Errorf("projectDB is nil")
	}

	_, err = DeleteAISessionMetaBySessionID(projectDB, sessionId)
	if err != nil {
		return 0, 0, err
	}

	deletedRuntimes, err = DeleteAgentRuntime(projectDB, &ypb.AITaskFilter{
		SessionID: []string{sessionId},
	})
	if err != nil {
		return 0, 0, err
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

// DeleteAllAISessionData deletes all session-scoped data across databases:
// - projectDB: AISession meta, AiOutputEvent + associations, AISessionPlanAndExec
// - profileDB: AIAgentRuntime
func DeleteAllAISessionData(profileDB, projectDB *gorm.DB) (deletedSessions int64, deletedRuntimes int64, deletedEvents int64, deletedPlanExec int64, err error) {
	if profileDB == nil {
		return 0, 0, 0, 0, utils.Errorf("profileDB is nil")
	}
	if projectDB == nil {
		return 0, 0, 0, 0, utils.Errorf("projectDB is nil")
	}

	deletedSessions, err = DeleteAllAISessionMeta(projectDB)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	deletedRuntimes, err = DeleteAllAgentRuntime(profileDB)
	if err != nil {
		return deletedSessions, 0, 0, 0, err
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
