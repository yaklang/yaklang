package sfvm

import (
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
)

type ValueOperator interface {
	GetName() string
	IsMap() bool
	IsList() bool

	ExactMatch(string) (bool, ValueOperator, error)
	GlobMatch(glob.Glob) (bool, ValueOperator, error)
	RegexpMatch(*regexp.Regexp) (bool, ValueOperator, error)
	NumberEqual(i any) (bool, ValueOperator, error)

	// object call field
	GetFields() (ValueOperator, error)

	// list field
	GetMembers() (ValueOperator, error)

	// object call function
	GetFunctionCallArgs() (ValueOperator, error)

	// call slice
	GetSliceCallArgs() (ValueOperator, error)

	Next() (ValueOperator, error)
	DeepNext() (ValueOperator, error)
}

func (op1 *Value) Exec(i SFVMOpCode, op2 *Value) (*Value, error) {
	if op1.IsList() {
		funk.Filter(op1.Value(), func(a ValueOperator) bool {
			result, err := ExecuteBoolResult(NewValue(a), op2, i)
			if err != nil {
				return false
			}
			return result
		})
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
	return nil, nil
}

func ExecuteBoolResult(op1, op2 *Value, opcode SFVMOpCode) (bool, error) {
	if op1.IsMap() || op2.IsMap() {
		return false, utils.Error("map not supported for comparison")
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
