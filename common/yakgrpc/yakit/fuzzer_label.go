package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_PROFILE_DATABASE, &schema.WebFuzzerLabel{})
}

func CreateOrUpdateWebFuzzerLabel(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&schema.WebFuzzerLabel{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.WebFuzzerLabel{}); db.Error != nil {
		return utils.Errorf("create/update WebFuzzerLabel failed: %s", db.Error)
	}

	return nil
}

func QueryWebFuzzerLabel(db *gorm.DB) ([]*schema.WebFuzzerLabel, error) {
	var task []*schema.WebFuzzerLabel

	db = db.Model(&schema.WebFuzzerLabel{})
	db = bizhelper.QueryOrder(db, "id", "desc")
	db = db.Find(&task)
	if db.Error != nil {
		return nil, utils.Errorf("query webFuzzerLabel failed: %s", db.Error)
	}
	return task, nil
}

func DeleteWebFuzzerLabel(db *gorm.DB, hash string) error {
	db = db.Model(&schema.WebFuzzerLabel{})
	if hash != "" {
		db = db.Where("hash = ?", hash)
	}
	db = db.Unscoped().Delete(&schema.WebFuzzerLabel{})
	if db.Error != nil {
		return utils.Errorf("delete web fuzzer label by label failed: %s", db.Error)
	}
	return nil
}

func QueryWebFuzzerLabelCount(db *gorm.DB) int64 {
	var count int64
	db = db.Model(&schema.WebFuzzerLabel{})
	db = db.Count(&count)
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
