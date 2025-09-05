package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type AIYakTool struct {
	gorm.Model

	Name string `json:"name" gorm:"unique_index"`
	// 展示给用户的名称
	VerboseName string `json:"verbose_name"`
	Description string `json:"description" gorm:"type:text;index"`
	Keywords    string `json:"keywords" gorm:"type:text;index"`
	Content     string `json:"content" gorm:"type:text"`
	Params      string `json:"params" gorm:"type:text"`
	Path        string `json:"path" gorm:"type:text;index"`
	Hash        string `json:"hash"`
	IsFavorite  bool   `json:"is_favorite" gorm:"default:false;index"`
}

func (*AIYakTool) TableName() string {
	return "ai_yak_tools"
}

func (d *AIYakTool) CalcHash() string {
	return utils.CalcSha1(d.Name, d.Content, d.Params, d.Path, d.Description, d.Keywords)
}

func (d *AIYakTool) BeforeSave() error {
	d.Hash = d.CalcHash()
	return nil
}
