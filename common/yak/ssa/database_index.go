package ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func CreateVariableIndexByName(name string, inst Instruction) *ssadb.IrIndex {
	return CreateVariableIndex(inst, name, "")
}

func CreateVariableIndexByMember(member string, inst Instruction) *ssadb.IrIndex {
	return CreateVariableIndex(inst, "", member)
}

func CreateVariableIndex(inst Instruction, name, member string) *ssadb.IrIndex {
	if inst.GetId() == -1 {
		return nil
	}
	prog := inst.GetProgram()
	progName := prog.GetApplication().GetProgramName()

	index := ssadb.CreateIndex(progName)

	// index
	index.ProgramName = prog.GetApplication().Name
	index.ValueID = inst.GetId()
	id := prog.NameCache.GetID(progName, name)
	index.VariableID = &id

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
			scopeID := prog.NameCache.GetID(progName, scope.GetScopeName())
			index.ScopeID = &scopeID
		}

		// field
		fieldID := prog.NameCache.GetID(progName, member)
		index.FieldID = &fieldID

	}
	return index
}

func CreateClassIndex(name string, inst Instruction) *ssadb.IrIndex {
	if inst.GetId() == -1 {
		return nil
	}
	prog := inst.GetProgram()
	progName := prog.GetApplication().GetProgramName()

	index := ssadb.CreateIndex(progName)

	// index
	index.ProgramName = prog.GetApplication().Name
	index.ValueID = inst.GetId()
	classID := prog.NameCache.GetID(progName, name)
	index.ClassID = &classID
	return index
}
