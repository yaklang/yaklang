package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type MarkdownDoc struct {
	gorm.Model

	YakScriptId   int64  `json:"yak_script_id" gorm:"index"`
	YakScriptName string `json:"yak_script_name" gorm:"index"`
	Markdown      string `json:"markdown"`
}

func CreateOrUpdateMarkdownDoc(db *gorm.DB, sid int64, name string, i interface{}) error {
	db = db.Model(&MarkdownDoc{})

	if db := db.Where("yak_script_id = ? OR yak_script_name = ?", sid, name).Assign(i).FirstOrCreate(&MarkdownDoc{}); db.Error != nil {
		return utils.Errorf("create/update MarkdownDoc failed: %s", db.Error)
	}

	return nil
}

func GetMarkdownDoc(db *gorm.DB, id int64) (*MarkdownDoc, error) {
	var req MarkdownDoc
	if db := db.Model(&MarkdownDoc{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get MarkdownDoc failed: %s", db.Error)
	}

	return &req, nil
}

func GetMarkdownDocByName(db *gorm.DB, sid int64, name string) (*MarkdownDoc, error) {
	var req MarkdownDoc
	if db := db.Model(&MarkdownDoc{}).Where("yak_script_id = ? OR yak_script_name = ?", sid, name).First(&req); db.Error != nil {
		return nil, utils.Errorf("get MarkdownDoc failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteMarkdownDocByID(db *gorm.DB, id int64) error {
	if db := db.Model(&MarkdownDoc{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&MarkdownDoc{}); db.Error != nil {
		return db.Error
	}
	return nil
}
