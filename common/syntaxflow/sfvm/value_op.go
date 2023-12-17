package sfvm

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
)

func (op1 *Value) Exec(i SFVMOpCode, op2 *Value) (*Value, error) {
	if op1.IsMap() {
		return NewValue(op1.AsMap().Filter(func(s string, a any) (bool, error) {
			result, err := ExecuteBoolResult(NewValue(a), op2, i)
			if err != nil {
				return false, nil
			}
			return result, nil
		})), nil
	} else {
		result, err := ExecuteBoolResult(op1, op2, i)
		if err != nil {
			return nil, err
		}
		if result {
			return op1, nil
		} else {
			return nil, nil
		}
	}
}

func ExecuteBoolResult(op1, op2 *Value, opcode SFVMOpCode) (bool, error) {
	if op1.IsMap() || op2.IsMap() {
		return false, utils.Error("")
	}
	switch opcode {
	case OpNotEq:
		return !funk.Equal(op1.Value(), op2.Value()), nil
	case OpEq:
		return funk.Equal(op1.Value(), op2.Value()), nil
	default:
		panic("NOT IMPLEMENTED")
	}
}
