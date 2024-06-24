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

func (v *Value) GetOpcode() string {
	return ssa.SSAOpcode2Name[v.getOpcode()]
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

func (v *Value) GlobMatch(mod int, g ssa.Glob) (bool, sfvm.ValueOperator, error) {
	value := _SearchValue(v, mod, g.Match)
	return value != nil, value, nil
}

func (v *Value) Merge(sf ...sfvm.ValueOperator) (sfvm.ValueOperator, error) {
	var vals []sfvm.ValueOperator
	vals = append(vals, v)
	vals = append(vals, sf...)
	return sfvm.NewValues(vals), nil
}

func (v *Value) Remove(sf ...sfvm.ValueOperator) (sfvm.ValueOperator, error) {
	err := sfvm.NewValues(sf).Recursive(func(operator sfvm.ValueOperator) error {
		if raw, ok := operator.(ssa.GetIdIF); ok {
			if v.GetId() == raw.GetId() {
				return utils.Error("abort")
			}
		}
		return nil
	})
	if err != nil {
		return sfvm.NewValues(nil), nil
	}
	return v, nil
}

func (v *Value) RegexpMatch(mod int, regexp *regexp.Regexp) (bool, sfvm.ValueOperator, error) {
	value := _SearchValue(v, mod, regexp.MatchString)
	return value != nil, value, nil
}

func (v *Value) GetAllCallActualParams() (sfvm.ValueOperator, error) {
	vs := make(Values, 0)
	v.GetCalledBy().ForEach(func(c *Value) {
		vs = append(vs, c.GetCallArgs()...)
	})
	if f, ok := ssa.ToFunction(v.node); ok {
		for _, para := range f.Params {
			vs = append(vs, v.NewValue(para))
		}
	}
	return vs, nil
}

func (v *Value) GetCallActualParams(i int) (sfvm.ValueOperator, error) {
	vs := make(Values, 0)
	v.GetCalledBy().ForEach(func(c *Value) {
		if c, ok := ssa.ToCall(c.node); ok {
			if len(c.Args) > i {
				vs = append(vs, v.NewValue(c.Args[i]))
			}
		}
	})
	if f, ok := ssa.ToFunction(v.node); ok {
		if len(f.Params) > i {
			vs = append(vs, v.NewValue(f.Params[i]))
		}
	}
	return vs, nil
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
	return WithSyntaxFlowConfig(v.GetTopDefs, config...), nil
}

func (v *Value) GetSyntaxFlowBottomUse(config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return WithSyntaxFlowConfig(v.GetBottomUses, config...), nil
}

func (v *Value) ListIndex(i int) (sfvm.ValueOperator, error) {
	if i == 0 {
		return v, nil
	}
	if !v.IsList() {
		return nil, utils.Error("ssa.Value is not a list")
	}
	member := v.GetMember(v.NewValue(ssa.NewConst(i)))
	if member != nil {
		return member, nil
	}
	return nil, utils.Errorf("ssa.Value %v cannot call by slice, like v[%v]", v.String(), i)
}

func (v *Value) AppendPredecessor(operator sfvm.ValueOperator, opts ...sfvm.AnalysisContextOption) error {
	return operator.Recursive(func(el sfvm.ValueOperator) error {
		if result, ok := el.(*Value); ok {
			ctx := sfvm.NewDefaultAnalysisContext()
			for _, opt := range opts {
				opt(ctx)
			}
			v.Predecessors = append(v.Predecessors, &PredecessorValue{
				Node: result, Info: ctx,
			})
		}
		return nil
	})
}
