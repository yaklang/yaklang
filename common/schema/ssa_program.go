package schema

import (
	"github.com/jinzhu/gorm"
)

type SSAProgram struct {
	gorm.Model

	Name        string `json:"name" gorm:"type:varchar(255);unique_index"`
	Description string `json:"description" gorm:"type:text"`

	DBPath string `json:"db_path"`
	// program language when set
	Language      string `json:"language" gorm:"type:varchar(255)"`
	EngineVersion string `json:"engine_version" gorm:"type:varchar(255)"`
}
