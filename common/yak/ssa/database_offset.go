package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func ConvertValue2Offset(inst Instruction) *ssadb.IrOffset {
	if inst.GetId() == -1 {
		return nil
	}

	if block, ok := ToBasicBlock(inst); ok {
		if len(block.Preds) == 0 && len(block.Succs) == 0 && len(block.Insts) == 0 {
			return nil
		}
	}

	rng := inst.GetRange()
	if utils.IsNil(rng) || utils.IsNil(rng.GetEditor()) {
		// inst.GetRange()
		// log.Errorf("%v: CreateOffset: rng or editor is nil", inst.GetVerboseName())
		return nil
	}
	irOffset := ssadb.CreateOffset(rng, inst.GetProgram().GetApplication().GetProgramName())
	// program name \ file name \ offset
	// value id
	irOffset.ValueID = int64(inst.GetId())
	return irOffset
}

func CreateVariableOffset(v *Variable, rng *memedit.Range) *ssadb.IrOffset {
	if utils.IsNil(v) || utils.IsNil(rng) || utils.IsNil(rng.GetEditor()) {
		return nil
	}
	value := v.GetValue()
	if utils.IsNil(value) || value.GetId() == -1 {
		return nil
	}
	prog := value.GetProgram()
	if utils.IsNil(prog) || utils.IsNil(prog.GetApplication()) {
		return nil
	}
	irOffset := ssadb.CreateOffset(rng, prog.GetApplication().GetProgramName())
	irOffset.VariableName = v.GetName()
	irOffset.ValueID = value.GetId()
	return irOffset
}
