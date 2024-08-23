package ssa

import "github.com/yaklang/yaklang/common/yak/ssa/ssadb"

func SaveValueAndVariableOffset(inst Instruction) {
	if inst.GetId() == -1 {
		return
	}
	prog := inst.GetProgram()
	saveOffset := func(offset *ssadb.IrOffset) {
		offset.ProgramName = prog.GetProgramName()
		offset.ValueID = inst.GetId()
		ssadb.SaveIrOffset(offset)
	}

	instEndOffset := ssadb.CreateOffset()
	instEndOffset.Offset = int64(inst.GetRange().GetEndOffset())
	saveOffset(instEndOffset)
	
	{
		// set variable def range offset
		value, ok := inst.(Value)
		if !ok {
			return
		}
		variables := value.GetAllVariables()
		for name, variable := range variables {
		// Save variable def range offset
		defOffset := ssadb.CreateOffset()
		defOffset.Variable = name
		defOffset.Offset = int64(variable.DefRange.GetEndOffset())
		saveOffset(defOffset)

		// Save variable use range offsets
		for rng := range variable.UseRange {
			useOffset := ssadb.CreateOffset()
			useOffset.Variable = name
			useOffset.Offset = int64(rng.GetEndOffset())
			saveOffset(useOffset)
		}
		}
	}
}
