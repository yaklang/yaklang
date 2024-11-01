package schema

import "github.com/jinzhu/gorm"

type PluginEnv struct {
	gorm.Model
	Key   string `gorm:"unique_index"`
	Value string
}
