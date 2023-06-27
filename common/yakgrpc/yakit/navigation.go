package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type NavigationBar struct {
	gorm.Model
	Group         string `json:"group" `
	YakScriptName string `json:"yak_script_name"`
	Hash          string `json:"-" gorm:"unique_index"`
	Mode          string `json:"mode"`
	VerboseSort   int64  `json:"verbose_sort"`
	GroupSort     int64  `json:"group_sort"`
	Route         string  `json:"route"`
	Verbose       string `json:"verbose"`
	GroupLabel    string  `json:"group_label"`
	VerboseLabel  string  `json:"verbose_label"`
}
func init() {
	RegisterPostInitDatabaseFunction(func() error {
		if db := consts.GetGormProfileDatabase(); db != nil {
			db.AutoMigrate(&NavigationBar{})
		}
		return nil
	})
}

func (m *NavigationBar) CalcHash() string {
	key := m.VerboseLabel
	if key == "" {
		key = m.YakScriptName
	}

	return utils.CalcSha1(m.Group, m.Mode, key)
}

func CreateOrUpdateNavigation(db *gorm.DB, hash string, i interface{}) error {
	db = UserDataAndPluginDatabaseScope(db)

	db = db.Model(&NavigationBar{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&NavigationBar{}); db.Error != nil {
		return utils.Errorf("create/update NavigationBar failed: %s", db.Error)
	}

	return nil
}

func GetAllNavigation(db *gorm.DB, req *ypb.GetAllNavigationRequest) []*NavigationBar {
	var items []*NavigationBar
	db = UserDataAndPluginDatabaseScope(db)
	db = db.Model(&NavigationBar{})
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

func DeleteNavigationByWhere(db *gorm.DB, req *ypb.GetAllNavigationRequest) error  {
	db = UserDataAndPluginDatabaseScope(db)
	db = db.Model(&NavigationBar{})
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
	db = db.Unscoped().Delete(&NavigationBar{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}