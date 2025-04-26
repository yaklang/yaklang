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

func GetCoordinatorRuntime(db *gorm.DB, uuid string) (*schema.AiCoordinatorRuntime, error) {
	var runtime schema.AiCoordinatorRuntime
	if err := db.Where("uuid = ?", uuid).First(&runtime).Error; err != nil {
		return nil, err
	}
	return &runtime, nil
}
