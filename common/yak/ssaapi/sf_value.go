package ssaapi

import (
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
)

func (v *Value) IsMap() bool {
	kind := v.GetTypeKind()
	return kind == ssa.MapTypeKind || kind == ssa.ObjectTypeKind
}

func (v *Value) GetNames() []string {
	var results []string
	if v.IsCall() {
		results = append(results, v.GetCallee().GetNames()...)
	}
	if v.IsMember() {
		results = append(results, v.GetKey().GetNames()...)
	}
	if v.IsConstInst() {
		results = append(results, codec.AnyToString(v.GetConstValue()))
	}
	results = append(results, v.GetName())
	return results
}

func (v *Value) IsList() bool {
	return v.GetTypeKind() == ssa.SliceTypeKind
}

func (v *Value) ExactMatch(s string) (bool, sfvm.ValueOperator, error) {
	for _, name := range v.GetNames() {
		if name == s {
			return true, v, nil
		}
	}
	return false, nil, nil
}

func (v *Value) GlobMatch(g glob.Glob) (bool, sfvm.ValueOperator, error) {
	for _, name := range v.GetNames() {
		if g.Match(name) {
			return true, v, nil
		}
	}
	return false, nil, nil
}

func (v *Value) RegexpMatch(regexp *regexp.Regexp) (bool, sfvm.ValueOperator, error) {
	if regexp.MatchString(v.GetName()) {
		return true, v, nil
	}
	return false, nil, nil
}

func (v *Value) GetCallActualParams() (sfvm.ValueOperator, error) {
	if !v.IsCall() {
		return nil, utils.Error("ssa.Value is not a call instruction")
	}
	return v.GetCallArgs(), nil
}

func (v *Value) GetCalled() (sfvm.ValueOperator, error) {
	if v.IsCalled() {
		return v.GetCalledBy(), nil
	}
	return nil, utils.Errorf("ssa.Value %v is not called", v.String())
}

func (v *Value) GetMembers() (sfvm.ValueOperator, error) {
	if v.IsMap() || v.IsList() || v.IsObject() {
		return v.GetAllMember(), nil
	}
	return v.GetUsers(), nil
}

func (v *Value) GetSyntaxFlowTopDef() (sfvm.ValueOperator, error) {
	return v.GetTopDefs(), nil
}

func (v *Value) GetSyntaxFlowBottomUse() (sfvm.ValueOperator, error) {
	return v.GetBottomUses(), nil
}

func (v *Value) ListIndex(i int) (sfvm.ValueOperator, error) {
	if !v.IsList() {
		return nil, utils.Error("ssa.Value is not a list")
	}
	member := v.GetMember(NewValue(ssa.NewConst(i)))
	if member != nil {
		return member, nil
	}
	return nil, utils.Errorf("ssa.Value %v cannot call by slice, like v[%v]", v.String(), i)
}
