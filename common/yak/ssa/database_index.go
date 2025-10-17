package ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func SaveVariableIndexByName(name string, inst Instruction) *ssadb.IrIndex {
	return SaveVariableIndex(inst, name, "")
}

func SaveVariableIndexByMember(member string, inst Instruction) *ssadb.IrIndex {
	return SaveVariableIndex(inst, "", member)
}

func SaveVariableIndex(inst Instruction, name, member string) *ssadb.IrIndex {
	if inst.GetId() == -1 {
		return nil
	}
	prog := inst.GetProgram()
	progName := prog.GetApplication().GetProgramName()

	index := ssadb.CreateIndex(progName)

	// index
	index.ProgramName = prog.GetApplication().Name
	index.ValueID = inst.GetId()
	index.VariableName = name

	{
		// variable and scope
		value, ok := inst.(Value)
		if !ok {
			return nil
		}
		variable := value.GetVariable(name)
		if variable != nil {
			index.VersionID = variable.GetVersion()
			// TODO : scope ID
			scope := variable.GetScope()
			index.ScopeName = scope.GetScopeName()
		}

		// field
		if member != "" {
			index.FieldName = member
		}

	}
	return index
}

func SaveClassIndex(name string, inst Instruction) *ssadb.IrIndex {
	if inst.GetId() == -1 {
		return nil
	}
	prog := inst.GetProgram()
	progName := prog.GetApplication().GetProgramName()

	index := ssadb.CreateIndex(progName)

	// index
	index.ProgramName = prog.GetApplication().Name
	index.ValueID = inst.GetId()
	index.ClassName = name
	return index
}
