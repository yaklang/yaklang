package schema

import "gorm.io/gorm"

type PluginEnv struct {
	gorm.Model
	Key   string `gorm:"uniqueIndex"`
	Value string
}
