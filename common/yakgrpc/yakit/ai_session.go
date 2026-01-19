package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
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

	deletedRuntimes, err = DeleteAgentRuntimeByPersistentSession(profileDB, sessionId)
	if err != nil {
		return 0, 0, err
	}

	deletedEvents, err = DeleteAIEventBySessionID(projectDB, sessionId)
	if err != nil {
		return deletedRuntimes, 0, err
	}

	return deletedRuntimes, deletedEvents, nil
}
