package ssadb

import (
	"sync"

	"github.com/jinzhu/gorm"
)

type IrVariable struct {
	gorm.Model

	ProgramName  string `json:"program_name" gorm:"index"`
	VariableName string `json:"variable_name" gorm:"index"`

	InstructionID Int64Slice `json:"instruction_id" gorm:"type:text"`
}

var variableOnce = new(sync.Once)

func SaveVariable(db *gorm.DB, program, variable string, instIDs []int64) error {
	variableOnce.Do(func() {
		db.AutoMigrate(&IrVariable{})
	})
	db = db.Model(&IrVariable{})
	// save new ircode
	irVariable := &IrVariable{}
	irVariable.ProgramName = program
	irVariable.VariableName = variable
	irVariable.InstructionID = instIDs
	return db.Save(irVariable).Error
}
