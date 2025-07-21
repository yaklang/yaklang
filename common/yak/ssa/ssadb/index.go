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

func (i *IrIndex) TableName() string {
	return "ir_indices"
}

func CreateIndex(progName string) *IrIndex {
	ret := &IrIndex{
		ProgramName: progName,
	}
	return ret
}

func SaveIrIndex(db *gorm.DB, idx *IrIndex) {
	db.Save(idx)
}

type IrVariable struct {
	VariableName string
	ValueID      int64
	VersionID    int64
}

func GetScope(programName, scopeName string) ([]IrVariable, error) {
	db := GetDB()

	var ret []IrVariable
	// get the max version of each variable
	retDB := db.Table("ir_indices").
		Select("variable_name, value_id, version_id").
		Where("scope_name = ? AND program_name = ? AND deleted_at IS NULL", scopeName, programName).
		Where(`version_id = (
			SELECT max(version_id) FROM ir_indices AS sub
			WHERE sub.variable_name = ir_indices.variable_name AND sub.scope_name = ir_indices.scope_name AND sub.program_name = ir_indices.program_name
		)`).Scan(&ret)
	if err := retDB.Error; err != nil {
		return nil, err
	}
	return ret, nil
}
