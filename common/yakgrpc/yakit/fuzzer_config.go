package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CreateOrUpdateWebFuzzerConfig(db *gorm.DB, config *schema.WebFuzzerConfig) (int64, error) {
	db = db.Model(&schema.WebFuzzerConfig{})
	if db := db.Where("page_id = ?", config.PageId).Assign(config).FirstOrCreate(&schema.WebFuzzerConfig{}); db.Error != nil {
		return 0, utils.Errorf("create/update WebFuzzerLabel failed: %s", db.Error)
	}
	return db.RowsAffected, nil
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

func DeleteWebFuzzerConfig(db *gorm.DB, pageIds []string, deleteAll bool) (int64, error) {
	if deleteAll {
		db = db.Unscoped().Delete(&schema.WebFuzzerConfig{})
	} else if len(pageIds) > 0 {
		db = bizhelper.ExactOrQueryStringArrayOr(db, "page_id", pageIds).Unscoped().Delete(&schema.WebFuzzerConfig{})
	}
	if db.Error != nil {
		return 0, utils.Errorf("delete web fuzzer failed: %s", db.Error)
	}
	return db.RowsAffected, nil
}
