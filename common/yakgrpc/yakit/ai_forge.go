package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func CreateOrUpdateAIForge(db *gorm.DB, name string, forge *schema.AIForge) error {
	db = db.Model(&schema.AIForge{})
	if findDb := db.Where("forge_name = ?", name).Find(&schema.AIForge{}); findDb.Error != nil {
		if !findDb.RecordNotFound() {
			return findDb.Error
		}
		if db := db.Create(forge); db.Error != nil {
			return db.Error
		}
	} else {
		if db := db.Where("forge_name = ?", name).UpdateColumns(&schema.AIForge{
			ForgeContent:  forge.ForgeContent,
			Params:        forge.Params,
			DefaultParams: forge.DefaultParams,
		}); db.Error != nil {
			return db.Error
		}
	}
	return nil
}

func DeleteAIForge(db *gorm.DB, name string) error {
	var forge schema.AIForge
	if db := db.Unscoped().Where("forge_name = ?", name).Delete(&forge); db.Error != nil {
		return db.Error
	}
	return nil
}

func GetAIForgeByName(db *gorm.DB, name string) (*schema.AIForge, error) {
	var forge schema.AIForge
	if db := db.Where("forge_name = ?", name).First(&forge); db.Error != nil {
		return nil, db.Error
	}
	return &forge, nil
}