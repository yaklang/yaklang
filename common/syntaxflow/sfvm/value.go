package sfvm

import (
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
)

type Value struct {
	actual ValueOperator
}

func (op1 *Value) IsList() bool {
	if op1 == nil || op1.actual == nil {
		return false
	}
	return op1.actual.IsList()
}

func (op1 *Value) GetName() string {
	if op1 == nil || op1.actual == nil {
		return ""
	}
	return op1.actual.GetName()
}

func (op1 *Value) ExactMatch(s string) (bool, ValueOperator, error) {
	if op1 == nil || op1.actual == nil {
		return false, nil, nil
	}
	return op1.actual.ExactMatch(s)
}

func (op1 *Value) GlobMatch(s glob.Glob) (bool, ValueOperator, error) {
	if op1 == nil || op1.actual == nil {
		return false, nil, nil
	}
	return op1.actual.GlobMatch(s)
}

func (op1 *Value) RegexpMatch(regexp *regexp.Regexp) (bool, ValueOperator, error) {
	if op1 == nil || op1.actual == nil {
		return false, nil, nil
	}
	return op1.actual.RegexpMatch(regexp)
}

func (op1 *Value) NumberEqual(i any) (bool, ValueOperator, error) {
	if op1 == nil || op1.actual == nil {
		return false, nil, nil
	}
	return op1.actual.NumberEqual(i)
}

func (op1 *Value) GetFields() (ValueOperator, error) {
	if op1 == nil || op1.actual == nil {
		return nil, nil
	}
	return op1.actual.GetFields()
}

func (op1 *Value) GetMembers() (ValueOperator, error) {
	if op1 == nil || op1.actual == nil {
		return nil, nil
	}
	return op1.actual.GetMembers()
}

func (op1 *Value) GetFunctionCallArgs() (ValueOperator, error) {
	if op1 == nil || op1.actual == nil {
		return nil, nil
	}
	return op1.actual.GetFunctionCallArgs()
}

func (op1 *Value) GetSliceCallArgs() (ValueOperator, error) {
	if op1 == nil || op1.actual == nil {
		return nil, nil
	}
	return op1.actual.GetSliceCallArgs()
}

func (op1 *Value) Next() (ValueOperator, error) {
	if op1 == nil || op1.actual == nil {
		return nil, nil
	}
	return op1.actual.Next()
}

func (op1 *Value) DeepNext() (ValueOperator, error) {
	if op1 == nil || op1.actual == nil {
		return nil, nil
	}
	return op1.actual.DeepNext()
}

var _ ValueOperator = &Value{}

func NewValue(v ValueOperator) *Value {
	return &Value{actual: v}
}

func (v *Value) AsInt() int {
	return codec.Atoi(v.GetName())
}

func (v *Value) AsString() string {
	return v.GetName()
}

func (v *Value) AsBool() bool {
	return codec.Atob(v.GetName())
}

func (v *Value) IsMap() bool {
	if v.actual == nil {
		return false
	}
	return v.actual.IsMap()
}

func (v *Value) Value() ValueOperator {
	return v.actual
}
