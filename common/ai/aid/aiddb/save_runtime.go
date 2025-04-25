package aiddb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func CreateOrUpdateRuntime(db *gorm.DB, runtime *schema.AiCoordinatorRuntime) error {
	if runtime.Uuid == "" {
		return db.Create(runtime).Error
	}

	var existingRuntime schema.AiCoordinatorRuntime
	if err := db.Where("uuid = ?", runtime.Uuid).First(&existingRuntime).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return db.Create(runtime).Error
		}
		return err
	}

	return db.Model(&existingRuntime).Updates(runtime).Error
}

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
