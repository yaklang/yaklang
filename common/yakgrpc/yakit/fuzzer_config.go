package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func CreateOrUpdateWebFuzzerConfig(db *gorm.DB, config *schema.WebFuzzerConfig) error {
	db = db.Model(&schema.WebFuzzerConfig{})
	if db := db.Where("page_id = ?", config.PageId).Assign(config).FirstOrCreate(&schema.WebFuzzerConfig{}); db.Error != nil {
		log.Warnf("create/update WebFuzzerLabel failed: %s", db.Error)
		return utils.Errorf("create/update WebFuzzerLabel failed: %s", db.Error)
	}
	return nil
}

func QueryWebFuzzerConfig(db *gorm.DB, limit int64) ([]*schema.WebFuzzerConfig, error) {
	var result []*schema.WebFuzzerConfig
	db = db.Model(&schema.WebFuzzerConfig{})
	if limit == -1 {
		db = db.Order("updated_at DESC").Find(&result)
	} else {
		db = db.Order("updated_at DESC").Limit(limit).Find(&result)
	}
	if db.Error != nil {
		return nil, utils.Errorf("query webFuzzerConfig failed: %s", db.Error)
	}
	return result, nil
}

func DeleteWebFuzzerConfig(db *gorm.DB, pageId string, deleteAll bool) error {
	if deleteAll {
		db = db.Unscoped().Delete(&schema.WebFuzzerConfig{})
	} else if pageId != "" {
		db = db.Unscoped().Where("page_id = ?", pageId).Delete(&schema.WebFuzzerConfig{})
	}
	if db.Error != nil {
		return utils.Errorf("delete web fuzzer failed: %s", db.Error)
	}
	return nil
}
