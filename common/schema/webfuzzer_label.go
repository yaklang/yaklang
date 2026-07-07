package schema

import (
	"github.com/yaklang/yaklang/common/utils"
	"gorm.io/gorm"
)

type WebFuzzerLabel struct {
	gorm.Model
	Label string `json:"label"`
	// 模版数据唯一标识，用来兼容做对比
	DefaultDescription string `json:"default_description"`
	Description        string `json:"description"`
	Hash               string `gorm:"uniqueIndex"`
}

func (w *WebFuzzerLabel) CalcHash() string {
	return utils.CalcSha1(w.DefaultDescription, w.Label)
}
