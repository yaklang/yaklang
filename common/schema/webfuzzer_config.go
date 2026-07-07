package schema

import (
	"gorm.io/gorm"
)

type WebFuzzerConfig struct {
	gorm.Model
	PageId string `gorm:"uniqueIndex"`
	Type   string `gorm:"string"`
	Config string `gorm:"string"`
}
