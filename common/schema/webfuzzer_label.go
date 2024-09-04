package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type WebFuzzerLabel struct {
	gorm.Model
	Label string `json:"label"`
	// 模版数据唯一标识，用来兼容做对比
	DefaultDescription string `json:"default_description"`
	Description        string `json:"description"`
	Hash               string `gorm:"unique_index"`
	Config             string `json:"config"`
}

func (w *WebFuzzerLabel) CalcHash() string {
	return utils.CalcSha1(w.DefaultDescription, w.Label)
}
