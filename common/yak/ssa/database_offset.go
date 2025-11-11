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

func ConvertVariable2Offset(v *Variable, variableName string, valueID int64) []*ssadb.IrOffset {
	if utils.IsNil(v) || v.GetId() == -1 {
		return nil
	}
	ret := make([]*ssadb.IrOffset, 0, 10)
	createOffset := func(rng *memedit.Range) {
		if utils.IsNil(rng) || utils.IsNil(rng.GetEditor()) {
			return
		}
		irOffset := ssadb.CreateOffset(rng, v.GetProgram().GetApplication().GetProgramName())
		// program name \ file name \ offset
		// variable name
		irOffset.VariableName = variableName
		irOffset.ValueID = valueID
		ret = append(ret, irOffset)
	}

	createOffset(v.DefRange)
	v.ForEachUseRange(func(r *memedit.Range) {
		createOffset(r)
	})
	return ret
}
