package ssadb

import (
	"github.com/jinzhu/gorm"
)

type IrIndex struct {
	gorm.Model

	ProgramName string `json:"program_name" gorm:"index"`

	// class
	ClassName string `json:"class_name" gorm:"index"`

	// variable
	VariableName string `json:"variable_name" gorm:"index"`
	VersionID    int64  `json:"version_id" gorm:"index"`
	// member call
	FieldName string `json:"field_name" gorm:"index"`

	// scope
	ScopeName string `json:"scope_name" gorm:"index"`
	// ScopeID   int64  `json:"scope_id" gorm:"index"`

	// value
	ValueID int64 `json:"value_id" gorm:"index"`
}

func CreateIndex() *IrIndex {
	ret := &IrIndex{}
	return ret
}
func SaveIrIndex(idx *IrIndex) {
	db := GetDB()
	db.Save(idx)
}

func GetScope(programName, scopeName string) ([]IrIndex, error) {
	db := GetDB()
	var ret []IrIndex
	if err := db.Where("scope_name = ?", scopeName).
		Where("program_name = ?", programName).
		Group("variable_name").
		Order("version_id desc").
		First(&ret).Error; err != nil {
		return nil, err
	}
	return ret, nil
}
