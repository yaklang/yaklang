package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func CreateOrUpdateAIForge(db *gorm.DB, name string, forge *schema.AIForge) error {
	db = db.Model(&schema.AIForge{})
	if db := db.Where("forge_name = ?", name).Assign(forge).FirstOrCreate(&schema.AIForge{}); db.Error != nil {
		return utils.Errorf("create/update AI Forge failed: %s", db.Error)
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
