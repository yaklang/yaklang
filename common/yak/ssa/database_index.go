package ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func SaveVariableIndex(inst Instruction, name string) {
	if inst.GetId() == -1 {
		return
	}
	prog := inst.GetProgram()

	index := ssadb.CreateIndex()
	defer ssadb.SaveIrIndex(index)

	// index
	index.ProgramName = prog.GetProgramName()
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
		if strings.HasPrefix(name, "#") {
			if _, member, ok := strings.Cut(name, "."); ok {
				index.FieldName = member
			}

			if _, member, ok := strings.Cut(name, "["); ok {
				index.FieldName, _ = strings.CutSuffix(member, "]")
			}
		}

		// TODO : scope ID
		scope := variable.GetScope()
		index.ScopeName = scope.GetScopeName()
	}
}

func SaveClassIndex(inst Instruction, name string) {
	if inst.GetId() == -1 {
		return
	}
	prog := inst.GetProgram()

	index := ssadb.CreateIndex()
	defer ssadb.SaveIrIndex(index)

	// index
	index.ProgramName = prog.GetProgramName()
	index.ValueID = inst.GetId()
	index.ClassName = name
}
