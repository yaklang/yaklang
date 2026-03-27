package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func CreateVariableIndexByName(name string, inst Instruction) *ssadb.IrIndex {
	return CreateVariableIndex(inst, name, "")
}

func CreateVariableIndexByMember(member string, inst Instruction) *ssadb.IrIndex {
	return CreateVariableIndex(inst, "", member)
}

func CreateVariableIndex(inst Instruction, name, member string) *ssadb.IrIndex {
	if utils.IsNil(inst) {
		return nil
	}
	if inst.GetId() == -1 {
		return nil
	}
	prog := inst.GetProgram()
	if utils.IsNil(prog) || utils.IsNil(prog.GetApplication()) || utils.IsNil(prog.NameCache) {
		return nil
	}
	progName := prog.GetApplication().GetProgramName()

	index := ssadb.CreateIndex(progName)

	// index
	index.ProgramName = prog.GetApplication().Name
	index.ValueID = inst.GetId()
	id := prog.NameCache.GetID(name)
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
			if scope := variable.GetScope(); scope != nil {
				index.ScopeName = scope.GetScopeName()
			}
		}

		// field
		fieldID := prog.NameCache.GetID(member)
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
	classID := prog.NameCache.GetID(name)
	index.ClassID = &classID
	return index
}
