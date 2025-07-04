package ssa

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func SaveValueOffset(db *gorm.DB, inst Instruction) {
	if inst.GetId() == -1 {
		return
	}

	if block, ok := ToBasicBlock(inst); ok {
		if len(block.Preds) == 0 && len(block.Succs) == 0 && len(block.Insts) == 0 {
			return
		}
	}

	rng := inst.GetRange()
	if utils.IsNil(rng) || utils.IsNil(rng.GetEditor()) {
		// inst.GetRange()
		// log.Errorf("%v: CreateOffset: rng or editor is nil", inst.GetVerboseName())
		return
	}
	irOffset := ssadb.CreateOffset(rng, inst.GetProgram().GetApplication().GetProgramName())
	// program name \ file name \ offset
	// value id
	irOffset.ValueID = int64(inst.GetId())
	ssadb.SaveIrOffset(db, irOffset)
}

func SaveVariableOffset(db *gorm.DB, v *Variable, variableName string, valueID int64) {
	if v.GetId() == -1 {
		return
	}
	add := func(rng memedit.RangeIf) {
		if utils.IsNil(rng) || utils.IsNil(rng.GetEditor()) {
			return
		}
		irOffset := ssadb.CreateOffset(rng, v.GetProgram().GetApplication().GetProgramName())
		// program name \ file name \ offset
		// variable name
		irOffset.VariableName = variableName
		irOffset.ValueID = valueID
		ssadb.SaveIrOffset(db, irOffset)
	}

	add(v.DefRange)
	for r := range v.UseRange {
		add(r)
	}
}
