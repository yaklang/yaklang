package ssaapi

import (
	"regexp"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var _ sfvm.ValueOperator = (*Value)(nil)

func (v *Value) IsMap() bool {
	kind := v.GetTypeKind()
	return kind == ssa.MapTypeKind || kind == ssa.ObjectTypeKind
}

func (v *Value) Recursive(f func(operator sfvm.ValueOperator) error) error {
	return f(v)
}

func (v *Value) IsList() bool {
	return v.GetTypeKind() == ssa.SliceTypeKind
}

func (v *Value) ExactMatch(mod int, want string) (bool, sfvm.ValueOperator, error) {
	value := _SearchValue(v, mod, func(s string) bool { return s == want })
	return value != nil, value, nil
}

func (v *Value) GlobMatch(mod int, g sfvm.Glob) (bool, sfvm.ValueOperator, error) {
	value := _SearchValue(v, mod, g.Match)
	return value != nil, value, nil
}

func (v *Value) RegexpMatch(mod int, regexp *regexp.Regexp) (bool, sfvm.ValueOperator, error) {
	value := _SearchValue(v, mod, regexp.MatchString)
	return value != nil, value, nil
}

func (v *Value) GetAllCallActualParams() (sfvm.ValueOperator, error) {
	if !v.IsCall() {
		return nil, utils.Error("ssa.Value is not a call instruction")
	}
	return v.GetCallArgs(), nil
}

func (v *Value) GetCallActualParams(i int) (sfvm.ValueOperator, error) {
	if !v.IsCall() {
		return nil, utils.Error("ssa.Value is not a call instruction")
	}
	if c, ok := ssa.ToCall(v.node); ok {
		if len(c.Args) < i {
			return v.NewValue(c.Args[i]), nil
		} else {
			return nil, utils.Errorf("ssa.Value %v has %d argument,but index %v", v.String(), len(c.Args), i)
		}
	} else {
		return nil, utils.Errorf("ssa.Value %v cannot get call actual params %v", v.String(), i)
	}
	// return v.GetCallArgs(), nil
}

func (v *Value) GetCalled() (sfvm.ValueOperator, error) {
	if v.IsCalled() {
		return v.GetCalledBy(), nil
	}
	return nil, utils.Errorf("ssa.Value %v is not called", v.String())
}

func (v *Value) GetMembersByString(key string) (sfvm.ValueOperator, error) {
	if v.IsMap() || v.IsList() || v.IsObject() {
		return v.GetMember(v.NewValue(ssa.NewConst(key))), nil
	}
	// return v.GetUsers(), nil
	return nil, nil
}

func (v *Value) GetSyntaxFlowUse() (sfvm.ValueOperator, error) {
	return v.GetUsers(), nil
}
func (v *Value) GetSyntaxFlowDef() (sfvm.ValueOperator, error) {
	return v.GetOperands(), nil
}
func (v *Value) GetSyntaxFlowTopDef(config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return v.GetTopDefs(), nil
}

func (v *Value) GetSyntaxFlowBottomUse(config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return v.GetBottomUses(), nil
}

func (v *Value) ListIndex(i int) (sfvm.ValueOperator, error) {
	if !v.IsList() {
		return nil, utils.Error("ssa.Value is not a list")
	}
	member := v.GetMember(v.NewValue(ssa.NewConst(i)))
	if member != nil {
		return member, nil
	}
	return nil, utils.Errorf("ssa.Value %v cannot call by slice, like v[%v]", v.String(), i)
}
