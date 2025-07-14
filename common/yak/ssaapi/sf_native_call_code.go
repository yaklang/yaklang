package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func nativeCallOpCodes(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	var vals []sfvm.ValueOperator
	opCodeMap := make(map[ssa.Opcode]struct{})
	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	checkAndAddOpCode := func(block int64, f *ssa.Function) {
		b, ok := f.GetBasicBlockByID(block)
		if !ok || b == nil {
			log.Warnf("function %s has a non-block instruction: %T", f, block)
			return
		}
		for _, p := range b.Phis {
			p, ok := b.GetValueById(p)
			if !ok {
				continue
			}
			opCodeMap[p.GetOpcode()] = struct{}{}
		}
		for _, id := range b.Insts {
			inst, ok := b.GetInstructionById(id)
			if !ok {
				continue
			}
			opCodeMap[inst.GetOpcode()] = struct{}{}
		}
	}

	v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		f, ok := ssa.ToFunction(val.GetFunction().innerValue)
		if !ok {
			log.Warnf("value %s is not a function", val.GetName())
			return nil
		}
		opCodeMap[f.GetOpcode()] = struct{}{}

		for _, freeValue := range f.FreeValues {
			freeValue, ok := f.GetValueById(freeValue)
			if !ok {
				continue
			}
			opCodeMap[freeValue.GetOpcode()] = struct{}{}
		}
		for _, param := range f.Params {
			param, ok := f.GetValueById(param)
			if !ok {
				continue
			}
			opCodeMap[param.GetOpcode()] = struct{}{}
		}
		for _, paramMember := range f.ParameterMembers {
			paramMember, ok := f.GetValueById(paramMember)
			if !ok {
				continue
			}
			opCodeMap[paramMember.GetOpcode()] = struct{}{}
		}
		for _, returnIns := range f.Return {
			returnIns, ok := f.GetValueById(returnIns)
			if !ok {
				continue
			}
			opCodeMap[returnIns.GetOpcode()] = struct{}{}
		}

		for _, b := range f.Blocks {
			checkAndAddOpCode(b, f)
		}
		return nil
	})
	for opCode := range opCodeMap {
		result := prog.NewConstValue(ssa.SSAOpcode2Name[opCode])
		result.AppendPredecessor(v, frame.WithPredecessorContext("opcodes"))
		vals = append(vals, result)
	}
	return true, sfvm.NewValues(vals), nil
}

func nativeCallSourceCode(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	context := params.GetInt("context")
	if context == -1 {
		context = 0
	}
	var vals []sfvm.ValueOperator
	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		var text string
		r := val.GetRange()
		if r != nil {
			text = r.GetTextContext(context)
		}
		if text == "" {
			return nil
		}
		result := prog.NewConstValue(text, r)
		result.AppendPredecessor(val, frame.WithPredecessorContext("source-code"))
		vals = append(vals, result)
		return nil
	})
	return true, sfvm.NewValues(vals), nil
}
