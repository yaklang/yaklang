package ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func SaveVariableIndexByName(name string, inst Instruction) {
	SaveVariableIndex(inst, name, "")
}

func SaveVariableIndexByMember(member string, inst Instruction) {
	SaveVariableIndex(inst, "", member)
}

func SaveVariableIndex(inst Instruction, name, member string) {
	if inst.GetId() == -1 {
		return
	}
	prog := inst.GetProgram()

	index := ssadb.CreateIndex()
	defer ssadb.SaveIrIndex(index)

	// index
	index.ProgramName = prog.GetApplication().Name
	index.ValueID = inst.GetId()
	index.VariableName = name

	{
		// variable and scope
		value, ok := inst.(Value)
		if !ok {
			return
		}
		variable := value.GetVariable(name)
		if variable == nil {
			return
		}
		index.VersionID = variable.GetVersion()

		// field
		if member != "" {
			index.FieldName = member
		}

		// TODO : scope ID
		scope := variable.GetScope()
		index.ScopeName = scope.GetScopeName()
	}
}

func SaveClassIndex(name string, inst Instruction) {
	if inst.GetId() == -1 {
		return
	}
	prog := inst.GetProgram()

	index := ssadb.CreateIndex()
	defer ssadb.SaveIrIndex(index)

	// index
	index.ProgramName = prog.GetApplication().Name
	index.ValueID = inst.GetId()
	index.ClassName = name
}
