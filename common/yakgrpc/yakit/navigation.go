package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_PROFILE_DATABASE, &schema.NavigationBar{})
}

func CreateOrUpdateNavigation(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&schema.NavigationBar{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.NavigationBar{}); db.Error != nil {
		return utils.Errorf("create/update NavigationBar failed: %s", db.Error)
	}

	return nil
}

func GetAllNavigation(db *gorm.DB, req *ypb.GetAllNavigationRequest) []*schema.NavigationBar {
	var items []*schema.NavigationBar

	db = db.Model(&schema.NavigationBar{})
	if req.Mode != "" {
		db = db.Where("mode = ?", req.Mode)
	} else {
		db = db.Where("mode IS NULL OR mode = '' ")
	}
	if req.Group != "" {
		db = db.Where("`group` = ?", req.Group)
	}
	db = db.Order("group_sort, verbose_sort asc ").Scan(&items)
	if db.Error != nil {
		return nil
	}
	return items
}

func DeleteNavigationByWhere(db *gorm.DB, req *ypb.GetAllNavigationRequest) error {
	db = db.Model(&schema.NavigationBar{})
	if req.GetMode() != "" {
		db = db.Where("mode = ? ", req.Mode)
	} else {
		db = db.Where("mode IS NULL OR mode = '' ")
	}
	if req.Group != "" {
		db = db.Where("`group` = ?", req.Group)
	}
	if req.YakScriptName != "" {
		db = db.Where("yak_script_name = ?", req.YakScriptName)
	}
	db = db.Unscoped().Delete(&schema.NavigationBar{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}
