package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func CreateOrUpdateMarkdownDoc(db *gorm.DB, sid int64, name string, i interface{}) error {
	db = db.Model(&schema.MarkdownDoc{})

	if db := db.Where("yak_script_id = ? OR yak_script_name = ?", sid, name).Assign(i).FirstOrCreate(&schema.MarkdownDoc{}); db.Error != nil {
		return utils.Errorf("create/update MarkdownDoc failed: %s", db.Error)
	}

	return nil
}

func GetMarkdownDoc(db *gorm.DB, id int64) (*schema.MarkdownDoc, error) {
	var req schema.MarkdownDoc
	if db := db.Model(&schema.MarkdownDoc{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get MarkdownDoc failed: %s", db.Error)
	}

	return &req, nil
}

func GetMarkdownDocByName(db *gorm.DB, sid int64, name string) (*schema.MarkdownDoc, error) {
	var req schema.MarkdownDoc
	if db := db.Model(&schema.MarkdownDoc{}).Where("yak_script_id = ? OR yak_script_name = ?", sid, name).First(&req); db.Error != nil {
		return nil, utils.Errorf("get MarkdownDoc failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteMarkdownDocByID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.MarkdownDoc{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.MarkdownDoc{}); db.Error != nil {
		return db.Error
	}
	return nil
}
