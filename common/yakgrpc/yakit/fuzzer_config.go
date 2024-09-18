package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CreateOrUpdateWebFuzzerConfig(db *gorm.DB, config *schema.WebFuzzerConfig) error {
	db = db.Model(&schema.WebFuzzerConfig{})
	if db := db.Where("page_id = ?", config.PageId).Assign(config).FirstOrCreate(&schema.WebFuzzerConfig{}); db.Error != nil {
		return utils.Errorf("create/update WebFuzzerLabel failed: %s", db.Error)
	}
	return nil
}

func QueryWebFuzzerConfig(db *gorm.DB, params *ypb.QueryFuzzerConfigRequest) ([]*schema.WebFuzzerConfig, error) {
	var result []*schema.WebFuzzerConfig
	db = db.Model(&schema.WebFuzzerConfig{})
	db = bizhelper.ExactOrQueryStringArrayOr(db, "page_id", params.GetPageId())
	_, db = bizhelper.PagingByPagination(db, params.Pagination, &result)
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
		db = db.Unscoped().Delete(&schema.WebFuzzerConfig{})
		msg = &ypb.DbOperateMessage{
			EffectRows:   db.RowsAffected,
			ExtraMessage: "Delete all webFuzzerConfig",
		}
	} else if len(pageIds) > 0 {
		db = bizhelper.ExactOrQueryStringArrayOr(db, "page_id", pageIds).Unscoped().Delete(&schema.WebFuzzerConfig{})
		msg = &ypb.DbOperateMessage{
			EffectRows:   db.RowsAffected,
			ExtraMessage: "Delete webFuzzerConfig with pageId",
		}
	}
	if db.Error != nil {
		return msg, utils.Errorf("delete web fuzzer failed: %s", db.Error)
	}
	return msg, nil
}
