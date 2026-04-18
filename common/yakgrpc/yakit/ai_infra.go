package yakit

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

func CreateOrUpdateCheckpoint(db *gorm.DB, checkpoint *schema.AiCheckpoint) error {
	if checkpoint.Hash == "" {
		checkpoint.Hash = checkpoint.CalcHash()
	}

	var existingCheckpoint schema.AiCheckpoint
	if err := db.Where("hash = ?", checkpoint.Hash).First(&existingCheckpoint).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return db.Create(checkpoint).Error
		}
		return err
	}

	return db.Model(&existingCheckpoint).Updates(checkpoint).Error
}

func YieldCheckpoint(ctx context.Context, db *gorm.DB, uuid string) chan *schema.AiCheckpoint {
	db = db.Model(&schema.AiCheckpoint{}).Where("coordinator_uuid = ?", uuid)
	return bizhelper.YieldModel[*schema.AiCheckpoint](ctx, db, bizhelper.WithYieldModel_PageSize(100))
}

func GetAIInteractiveCheckpoint(db *gorm.DB, coordinatorUuid string, seq int64) (*schema.AiCheckpoint, bool) {
	var checkpoint schema.AiCheckpoint
	if err := db.Where("coordinator_uuid = ? AND seq = ?", coordinatorUuid, seq).First(&checkpoint).Error; err != nil {
		return nil, false
	}

	if checkpoint.Type != schema.AiCheckpointType_AIInteractive {
		return &checkpoint, false
	}

	return &checkpoint, true
}

func GetToolCallCheckpoint(db *gorm.DB, coordinatorUuid string, seq int64) (*schema.AiCheckpoint, bool) {
	var checkpoint schema.AiCheckpoint
	if err := db.Where("coordinator_uuid = ? AND seq = ?", coordinatorUuid, seq).First(&checkpoint).Error; err != nil {
		return nil, false
	}

	if checkpoint.Type != schema.AiCheckpointType_ToolCall {
		return &checkpoint, false
	}

	return &checkpoint, true
}

func GetReviewCheckpoint(db *gorm.DB, coordinatorUuid string, seq int64) (*schema.AiCheckpoint, bool) {
	var checkpoint schema.AiCheckpoint
	if err := db.Where("coordinator_uuid = ? AND seq = ?", coordinatorUuid, seq).First(&checkpoint).Error; err != nil {
		return nil, false
	}

	if checkpoint.Type != schema.AiCheckpointType_Review {
		return &checkpoint, false
	}

	return &checkpoint, true
}

func CreateOrUpdateAIAgentRuntime(db *gorm.DB, runtime *schema.AIAgentRuntime) (uint, error) {
	if runtime.Uuid == "" {
		err := db.Create(runtime).Error
		return runtime.ID, err
	}

	var existingRuntime schema.AIAgentRuntime
	if err := db.Where("uuid = ?", runtime.Uuid).First(&existingRuntime).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			err = db.Create(runtime).Error
			return runtime.ID, err
		}
		return 0, err
	}

	err := db.Model(&existingRuntime).Updates(runtime).Error
	return existingRuntime.ID, err
}

func UpdateAIAgentRuntimeTimeline(db *gorm.DB, uuid string, timeline string) error {
	return db.Model(&schema.AIAgentRuntime{}).Where("uuid = ?", uuid).Update("quoted_timeline", timeline).Error
}

// UpdateAIAgentRuntimeWorkDir updates the working directory and semantic label for an AIAgentRuntime
func UpdateAIAgentRuntimeWorkDir(db *gorm.DB, uuid string, workDir string, semanticLabel string) error {
	updates := map[string]interface{}{
		"work_dir": workDir,
	}
	if semanticLabel != "" {
		updates["semantic_label"] = semanticLabel
	}
	return db.Model(&schema.AIAgentRuntime{}).Where("uuid = ?", uuid).Updates(updates).Error
}

func UpdateAIAgentRuntimeTimelineWithPersistentId(db *gorm.DB, persistentId string, timeline string) error {
	return db.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", persistentId).Update("quoted_timeline", timeline).Error
}

func UpdateAIAgentRuntimeLoadedSkillNames(db *gorm.DB, persistentId string, skillNames string) error {
	return db.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", persistentId).Update("loaded_skill_names", skillNames).Error
}

func UpdateAIAgentRuntimeRecentToolsCache(db *gorm.DB, persistentId string, cacheJSON string) error {
	return db.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", persistentId).Update("recent_tools_cache", cacheJSON).Error
}

// UpdateAIAgentRuntimeUserInput updates the quoted_user_input field for an AIAgentRuntime by uuid.
func UpdateAIAgentRuntimeUserInput(db *gorm.DB, uuid string, quotedInput string) error {
	return db.Model(&schema.AIAgentRuntime{}).Where("uuid = ?", uuid).Update("quoted_user_input", quotedInput).Error
}

// UpdateAIAgentRuntimeEvidence updates the quoted_evidence field for all AIAgentRuntime rows
// matching the given persistent session ID, consistent with other session-level update functions.
func UpdateAIAgentRuntimeEvidence(db *gorm.DB, persistentId string, quotedEvidence string) error {
	return db.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", persistentId).Update("quoted_evidence", quotedEvidence).Error
}

// GetLatestAIAgentRuntimeByPersistentSession 获取某个持久化会话的最新运行时
func GetLatestAIAgentRuntimeByPersistentSession(db *gorm.DB, sessionId string) (*schema.AIAgentRuntime, error) {
	start := time.Now()
	defer func() {
		if du := time.Since(start); du > time.Second {
			log.Warnf("GetLatestAIAgentRuntimeByPersistentSession with sessionId '%s' took %v, it's abnormal case.", sessionId, du)
		}
	}()

	var runtime schema.AIAgentRuntime
	if err := db.Where("persistent_session = ?", sessionId).Order("id DESC").First(&runtime).Error; err != nil {
		return nil, err
	}
	return &runtime, nil
}

func GetAgentRuntime(db *gorm.DB, uuid string) (*schema.AIAgentRuntime, error) {
	var runtime schema.AIAgentRuntime
	if err := db.Where("uuid = ?", uuid).First(&runtime).Error; err != nil {
		return nil, err
	}
	return &runtime, nil
}

func FilterAgentRuntime(db *gorm.DB, filter *ypb.AITaskFilter) *gorm.DB {
	db = db.Model(&schema.AIAgentRuntime{})
	db = bizhelper.ExactQueryStringArrayOr(db, "forge_name", filter.GetForgeName())
	db = bizhelper.ExactQueryStringArrayOr(db, "persistent_session", filter.GetSessionID())
	db = bizhelper.ExactQueryStringArrayOr(db, "uuid", filter.GetCoordinatorId())
	db = bizhelper.ExactQueryStringArrayOr(db, "name", filter.GetName())
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		"name", "quoted_user_input", "forge_name",
	}, filter.GetKeyword(), false)
	return db
}

func QueryAgentRuntime(db *gorm.DB, filter *ypb.AITaskFilter, paging *ypb.Paging) (*bizhelper.Paginator, []schema.AIAgentRuntime, error) {
	db = FilterAgentRuntime(db, filter)
	db = bizhelper.OrderByPaging(db, paging)
	var aiTasks []schema.AIAgentRuntime
	pag, db := bizhelper.Paging(db, int(paging.Page), int(paging.Limit), &aiTasks)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return pag, aiTasks, nil
}

func DeleteAgentRuntime(db *gorm.DB, filter *ypb.AITaskFilter) (int64, error) {
	db = FilterAgentRuntime(db, filter)
	if db = db.Unscoped().Delete(&schema.AIAgentRuntime{}); db.Error != nil {
		return 0, db.Error
	}
	return db.RowsAffected, nil
}

// DeleteAgentRuntimeByPersistentSession deletes all runtimes under a persistent session.
func DeleteAgentRuntimeByPersistentSession(db *gorm.DB, sessionId string) (int64, error) {
	if sessionId == "" {
		return 0, utils.Errorf("sessionId is empty")
	}
	db = db.Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", sessionId)
	if db = db.Unscoped().Delete(&schema.AIAgentRuntime{}); db.Error != nil {
		return 0, db.Error
	}
	return db.RowsAffected, nil
}

func DeleteAllAgentRuntime(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, utils.Errorf("database is nil")
	}
	deletedRuntimes, err := countRowsIgnoreMissingTable(db, &schema.AIAgentRuntime{})
	if err != nil {
		return 0, err
	}
	if err := schema.DropRecreateTable(db, &schema.AIAgentRuntime{}); err != nil {
		return deletedRuntimes, err
	}
	return deletedRuntimes, nil
}

func QueryAgentRuntimeUUIDsBySessionID(db *gorm.DB, sessionId string) ([]string, error) {
	if db == nil {
		return nil, utils.Errorf("database is nil")
	}
	if sessionId == "" {
		return nil, utils.Errorf("sessionId is empty")
	}

	var coordinatorUUIDs []string
	if err := db.Model(&schema.AIAgentRuntime{}).
		Where("persistent_session = ?", sessionId).
		Pluck("uuid", &coordinatorUUIDs).Error; err != nil {
		if isMissingTableErr(err) {
			return nil, nil
		}
		return nil, err
	}
	return coordinatorUUIDs, nil
}

func DeleteCheckpointByCoordinatorUUIDs(db *gorm.DB, coordinatorUUIDs []string) (int64, error) {
	if db == nil {
		return 0, utils.Errorf("database is nil")
	}
	if len(coordinatorUUIDs) == 0 {
		return 0, nil
	}

	result := db.Model(&schema.AiCheckpoint{}).
		Where("coordinator_uuid IN (?)", coordinatorUUIDs).
		Unscoped().
		Delete(&schema.AiCheckpoint{})
	if result.Error != nil {
		if isMissingTableErr(result.Error) {
			return 0, nil
		}
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func DeleteAllCheckpoint(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, utils.Errorf("database is nil")
	}

	deletedCheckpoints, err := countRowsIgnoreMissingTable(db, &schema.AiCheckpoint{})
	if err != nil {
		return 0, err
	}
	if err := schema.DropRecreateTable(db, &schema.AiCheckpoint{}); err != nil {
		return deletedCheckpoints, err
	}
	return deletedCheckpoints, nil
}
