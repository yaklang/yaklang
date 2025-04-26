package aiddb

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
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
