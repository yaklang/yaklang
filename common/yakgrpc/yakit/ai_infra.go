package yakit

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

func CreateOrUpdateAIAgentRuntime(db *gorm.DB, runtime *schema.AIAgentRuntime) error {
	if runtime.Uuid == "" {
		return db.Create(runtime).Error
	}

	var existingRuntime schema.AIAgentRuntime
	if err := db.Where("uuid = ?", runtime.Uuid).First(&existingRuntime).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return db.Create(runtime).Error
		}
		return err
	}

	return db.Model(&existingRuntime).Updates(runtime).Error
}

// GetLatestAIAgentRuntimeByPersistentSession 获取某个持久化会话的最新运行时
func GetLatestAIAgentRuntimeByPersistentSession(db *gorm.DB, sessionId string) (*schema.AIAgentRuntime, error) {
	var runtime schema.AIAgentRuntime
	if err := db.Where("persistent_session = ?", sessionId).Order("updated_at DESC").First(&runtime).Error; err != nil {
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
