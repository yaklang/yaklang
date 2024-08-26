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

	
	if rng := inst.GetRange();rng != nil {
		instEndOffset := ssadb.CreateOffset()
		instEndOffset.Offset = int64(rng.GetEndOffset())
		saveOffset(instEndOffset)
	}
	
	{
		// set variable def range offset
		value, ok := inst.(Value)
		if !ok {
			return
		}
		variables := value.GetAllVariables()
		for name, variable := range variables {
		// Save variable def range offset
		if rng := variable.DefRange;rng != nil {
			defOffset := ssadb.CreateOffset()
			defOffset.Offset = int64(rng.GetEndOffset())
			defOffset.VariableName = name
			defOffset.IsVariable=true
			saveOffset(defOffset)
		}

		// Save variable use range offsets
		for rng := range variable.UseRange {
			useOffset := ssadb.CreateOffset()
			useOffset.VariableName = name
			useOffset.IsVariable=true
			useOffset.Offset = int64(rng.GetEndOffset())
			saveOffset(useOffset)
		}
		}
	}
}
