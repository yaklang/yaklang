package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func SaveValueOffset(inst Instruction) {
	if inst.GetId() == -1 {
		return
	}

	rng := inst.GetRange()
	if utils.IsNil(rng) || utils.IsNil(rng.GetEditor()) {
		log.Errorf("CreateOffset: rng or editor is nil")
		return
	}
	irOffset := ssadb.CreateOffset(rng)

	// program name \ file name \ offset
	irOffset.ProgramName = inst.GetProgram().GetProgramName()
	// value id
	irOffset.ValueID = int64(inst.GetId())
	ssadb.SaveIrOffset(irOffset)
}

func SaveVariableOffset(v *Variable, variableName string) {
	if v.GetId() == -1 {
		return
	}
	add := func(rng memedit.RangeIf) {
		if utils.IsNil(rng) || utils.IsNil(rng.GetEditor()) {
			return
		}
		irOffset := ssadb.CreateOffset(rng)
		// program name \ file name \ offset
		irOffset.ProgramName = v.GetProgram().GetProgramName()
		// variable name
		irOffset.VariableName = variableName
		irOffset.ValueID = v.GetValue().GetId()
		ssadb.SaveIrOffset(irOffset)
	}

	add(v.DefRange)
	for r := range v.UseRange {
		add(r)
	}
}
