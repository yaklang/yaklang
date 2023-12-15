package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

type WebFuzzerLabel struct {
	gorm.Model
	Label string `json:"label"`
	// 模版数据唯一标识，用来兼容做对比
	DefaultDescription string `json:"default_description"`
	Description        string `json:"description"`
	Hash               string `gorm:"unique_index"`
}

func init() {
	RegisterPostInitDatabaseFunction(func() error {
		if db := consts.GetGormProfileDatabase(); db != nil {
			db.AutoMigrate(&WebFuzzerLabel{})
		}
		return nil
	})
}

func (w *WebFuzzerLabel) CalcHash() string {
	return utils.CalcSha1(w.DefaultDescription, w.Label)
}

func CreateOrUpdateWebFuzzerLabel(db *gorm.DB, hash string, i interface{}) error {
	db = UserDataAndPluginDatabaseScope(db)

	db = db.Model(&WebFuzzerLabel{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&WebFuzzerLabel{}); db.Error != nil {
		return utils.Errorf("create/update WebFuzzerLabel failed: %s", db.Error)
	}

	return nil
}

func QueryWebFuzzerLabel(db *gorm.DB) ([]*WebFuzzerLabel, error) {
	var task []*WebFuzzerLabel
	db = UserDataAndPluginDatabaseScope(db)
	db = db.Model(&WebFuzzerLabel{})
	db = bizhelper.QueryOrder(db, "id", "desc")
	db = db.Find(&task)
	if db.Error != nil {
		return nil, utils.Errorf("query webFuzzerLabel failed: %s", db.Error)
	}
	return task, nil
}

func DeleteWebFuzzerLabel(db *gorm.DB, hash string) error {
	db = db.Model(&WebFuzzerLabel{})
	if hash != "" {
		db = db.Where("hash = ?", hash)
	}
	db = db.Unscoped().Delete(&WebFuzzerLabel{})
	if db.Error != nil {
		return utils.Errorf("delete web fuzzer label by label failed: %s", db.Error)
	}
	return nil
}

func QueryWebFuzzerLabelCount(db *gorm.DB) int64 {
	var count int64
	db = db.Model(&WebFuzzerLabel{})
	db = db.Find(&count)
	if db.Error != nil {
		return 0
	}
	return count
}

/*func GetWebFuzzerLabel(db *gorm.DB, hash string) ([]*WebFuzzerLabel, error) {
	var task []*WebFuzzerLabel
	db = db.Model(&WebFuzzerLabel{})
	if hash != "" {
		db = db.Where("hash = ?", hash)
	}
	db = db.Find(&task)
	if db.Error != nil {
		return nil, db.Error
	}
	return task, nil
}*/
