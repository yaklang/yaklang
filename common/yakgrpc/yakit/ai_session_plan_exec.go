package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func CreateOrUpdateAISessionPlanAndExec(db *gorm.DB, record *schema.AISessionPlanAndExec) error {
	if db == nil {
		return utils.Errorf("db is nil")
	}
	if record == nil {
		return utils.Errorf("record is nil")
	}
	if record.SessionID == "" {
		return utils.Errorf("session_id is empty")
	}
	if record.CoordinatorID == "" {
		return utils.Errorf("coordinator_id is empty")
	}

	var existing schema.AISessionPlanAndExec
	if err := db.Where("coordinator_id = ?", record.CoordinatorID).First(&existing).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return db.Create(record).Error
		}
		return err
	}

	updates := map[string]any{
		"session_id":    record.SessionID,
		"task_tree":     record.TaskTree,
		"task_progress": record.TaskProgress,
	}
	return db.Model(&existing).Updates(updates).Error
}

func DeleteAISessionPlanAndExecBySessionID(db *gorm.DB, sessionID string) error {
	if db == nil {
		return utils.Errorf("db is nil")
	}
	if sessionID == "" {
		return utils.Errorf("session_id is empty")
	}
	err := db.Model(&schema.AISessionPlanAndExec{}).Where("session_id = ?", sessionID).Unscoped().Delete(&schema.AISessionPlanAndExec{}).Error
	if err != nil && (strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "doesn't exist")) {
		return nil
	}
	return err
}

func GetLatestAISessionPlanAndExecBySessionID(db *gorm.DB, sessionID string) (*schema.AISessionPlanAndExec, error) {
	if db == nil {
		return nil, utils.Errorf("db is nil")
	}
	if sessionID == "" {
		return nil, utils.Errorf("session_id is empty")
	}
	var record schema.AISessionPlanAndExec
	if err := db.Where("session_id = ?", sessionID).Order("updated_at desc").First(&record).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) || strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "doesn't exist") {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}
