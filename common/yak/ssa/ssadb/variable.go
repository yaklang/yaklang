package ssadb

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/jinzhu/gorm"
)

type IrVariable struct {
	gorm.Model

	ProgramName  string `json:"program_name" gorm:"index"`
	VariableName string `json:"variable_name" gorm:"index"`

	IsClassInstance      bool `json:"is_class_instance"`
	IsAnnotationInstance bool `json:"is_annotation_instance"`

	// OOP Index
	ObjectID        int64  `json:"object_id" gorm:"index"`
	SliceMemberName string `json:"slice_member_name" gorm:"index"`
	FieldMemberName string `json:"field_member_name" gorm:"index"`

	InstructionID Int64Slice `json:"instruction_id" gorm:"type:text"`
}

type SSAValue interface {
	IsAnnotation() bool
	GetId() int64
}

func SaveVariable(db *gorm.DB, program, variable string, insts []SSAValue) error {
	start := time.Now()
	defer func() {
		atomic.AddUint64(&_SSAVariableCost, uint64(time.Now().Sub(start).Nanoseconds()))
	}()

	db = db.Model(&IrVariable{})
	// save new ircode

	irVariable := &IrVariable{}
	irVariable.ProgramName = program
	irVariable.VariableName = variable
	instIDs := make([]int64, 0, len(insts))
	for _, inst := range insts {
		if inst == nil {
			log.Warnf("ssadb.SaveVariable failed: the inst %T is nil(recog)", inst)
			continue
		}
		if !irVariable.IsAnnotationInstance && inst.IsAnnotation() {
			irVariable.IsAnnotationInstance = true
		}
		instIDs = append(instIDs, inst.GetId())
	}
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

func SaveClassInstance(db *gorm.DB, program, class string, instIDs []int64) error {
	start := time.Now()
	defer func() {
		atomic.AddUint64(&_SSAVariableCost, uint64(time.Now().Sub(start).Nanoseconds()))
	}()

	db = db.Model(&IrVariable{})
	irVariable := &IrVariable{}
	irVariable.IsClassInstance = true
	irVariable.ProgramName = program
	irVariable.VariableName = class
	irVariable.InstructionID = instIDs
	return db.Save(irVariable).Error
}

func GetVariable(db *gorm.DB, program, variable string) (*IrVariable, error) {
	db = db.Model(&IrVariable{})
	irVariable := &IrVariable{}
	err := db.Where("program_name = ? AND variable_name = ?", program, variable).First(irVariable).Error
	return irVariable, err
}
