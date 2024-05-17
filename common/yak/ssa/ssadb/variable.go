package ssadb

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/jinzhu/gorm"
)

type IrVariable struct {
	gorm.Model

	ProgramName  string `json:"program_name" gorm:"index"`
	VariableName string `json:"variable_name" gorm:"index"`

	// OOP Index
	ObjectID        int64  `json:"object_id" gorm:"index"`
	SliceMemberName string `json:"slice_member_name" gorm:"index"`
	FieldMemberName string `json:"field_member_name" gorm:"index"`

	InstructionID Int64Slice `json:"instruction_id" gorm:"type:text"`
}

func SaveVariable(db *gorm.DB, program, variable string, instIDs []int64) error {
	db = db.Model(&IrVariable{})
	// save new ircode

	irVariable := &IrVariable{}
	irVariable.ProgramName = program
	irVariable.VariableName = variable
	irVariable.InstructionID = instIDs

	if strings.HasPrefix(variable, "#") {
		if before, member, ok := strings.Cut(variable, "."); ok {
			irVariable.FieldMemberName = member
			irVariable.ObjectID = int64(codec.Atoi(strings.TrimLeft(before, "#")))
		}

		if before, member, ok := strings.Cut(variable, "["); ok {
			irVariable.SliceMemberName, _ = strings.CutSuffix(member, "]")
			irVariable.ObjectID = int64(codec.Atoi(strings.TrimLeft(before, "#")))
		}
	}

	return db.Save(irVariable).Error
}

func GetVariable(db *gorm.DB, program, variable string) (*IrVariable, error) {
	db = db.Model(&IrVariable{})
	irVariable := &IrVariable{}
	err := db.Where("program_name = ? AND variable_name = ?", program, variable).First(irVariable).Error
	return irVariable, err
}
