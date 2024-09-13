package yakit

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CreateOrUpdateWebFuzzerConfig(db *gorm.DB, config *schema.WebFuzzerConfig) error {
	db = db.Model(&schema.WebFuzzerConfig{})
	if db := db.Where("page_id = ?", config.PageId).Assign(config).FirstOrCreate(&schema.WebFuzzerConfig{}); db.Error != nil {
		log.Warnf("create/update WebFuzzerLabel failed: %s", db.Error)
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

func DeleteWebFuzzerConfig(db *gorm.DB, pageIds []string, deleteAll bool) (*ypb.DbOperateMessage, error) {
	msg := &ypb.DbOperateMessage{
		TableName: "WebFuzzerConfig",
		Operation: "Delete",
	}
	if deleteAll {
		var count int64
		db = db.Model(&schema.WebFuzzerConfig{}).Count(&count)
		db = db.Unscoped().Delete(&schema.WebFuzzerConfig{})
		msg = &ypb.DbOperateMessage{
			EffectRows:   count,
			ExtraMessage: "Delete all webFuzzerConfig",
		}
	} else if len(pageIds) > 0 {
		db = db.Unscoped().Where("page_id IN (?)", pageIds).Delete(&schema.WebFuzzerConfig{})
		msg = &ypb.DbOperateMessage{
			EffectRows:   int64(len(pageIds)),
			ExtraMessage: fmt.Sprintf("Delete webFuzzerConfig with pageId"),
		}
	}
	if db.Error != nil {
		return msg, utils.Errorf("delete web fuzzer failed: %s", db.Error)
	}
	return msg, nil
}
