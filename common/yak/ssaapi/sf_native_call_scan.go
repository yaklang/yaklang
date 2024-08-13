package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type basicBlockap map[int64]struct{}

func scanBasicBlockPrevious(value ssa.Value, prog *Program, basicBlockMap basicBlockap) []sfvm.ValueOperator {
	if value == nil {
		return nil
	}

	var results []sfvm.ValueOperator
	block, ok := ssa.ToBasicBlock(value)
	if !ok {
		block = value.GetBlock()
	}
	if block == nil {
		return nil
	}

	if _, ok := basicBlockMap[block.GetId()]; ok {
		return nil
	}
	basicBlockMap[block.GetId()] = struct{}{}
	for _, inst := range block.Insts {
		if _, ok := ssa.ToConst(inst); ok {
			continue
		}
		if v, ok := ssa.ToValue(inst); ok {
			if v.GetOpcode() == ssa.SSAOpcodeJump {
				continue
			}
			res := prog.NewValue(v)
			results = append(results, res)
		}
	}
	for _, pred := range block.Preds {
		res := scanBasicBlockPrevious(pred, prog, basicBlockMap)
		results = append(results, res...)
	}
	return results
}

var nativeCallScanPrevious = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	var vals []sfvm.ValueOperator
	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	basicBlockMap := make(basicBlockap)
	v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		results := scanBasicBlockPrevious(val.node, prog, basicBlockMap)
		if !ok {
			return nil
		}
		vals = append(vals, results...)
		return nil
	})
	return true, sfvm.NewValues(vals), nil
}

var nativeCallScanNext = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	return false, nil, nil
}
