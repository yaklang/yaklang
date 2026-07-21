package schema

import (
	"github.com/yaklang/gorm"
)

type WebFuzzerConfig struct {
	gorm.Model
	PageId string `gorm:"unique_index"`
	Type   string `gorm:"string"`
	Config string `gorm:"string"`
}
