package ssa

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func SaveVariableIndexByName(db *gorm.DB, name string, inst Instruction) {
	SaveVariableIndex(db, inst, name, "")
}

func SaveVariableIndexByMember(db *gorm.DB, member string, inst Instruction) {
	SaveVariableIndex(db, inst, "", member)
}

func SaveVariableIndex(db *gorm.DB, inst Instruction, name, member string) {
	if inst.GetId() == -1 {
		return
	}
	prog := inst.GetProgram()
	progName := prog.GetApplication().GetProgramName()

	index := ssadb.CreateIndex(db, progName)

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
	ssadb.SaveIrIndex(db, index)
}

func SaveClassIndex(db *gorm.DB, name string, inst Instruction) {
	if inst.GetId() == -1 {
		return
	}
	prog := inst.GetProgram()
	progName := prog.GetApplication().GetProgramName()

	index := ssadb.CreateIndex(db, progName)

	// index
	index.ProgramName = prog.GetApplication().Name
	index.ValueID = inst.GetId()
	index.ClassName = name
	ssadb.SaveIrIndex(db, index)
}
