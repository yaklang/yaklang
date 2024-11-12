package ssaapi

import (
	"context"
	"regexp"

	"github.com/samber/lo"

	"github.com/gobwas/glob"

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

func (v *Value) GetBinaryOperator() string {
	sa := v.GetSSAValue()
	if utils.IsNil(sa) {
		return ""
	}
	if sa.GetOpcode() == ssa.SSAOpcodeBinOp {
		binop, ok := ssa.ToBinOp(sa)
		if !ok {
			return ""
		}
		return string(binop.Op)
	}
	return ""
}

func (v *Value) GetUnaryOperator() string {
	sa := v.GetSSAValue()
	if utils.IsNil(sa) {
		return ""
	}
	if sa.GetOpcode() == ssa.SSAOpcodeUnOp {

	}
	return ""
}

func (v *Value) Recursive(f func(operator sfvm.ValueOperator) error) error {
	return f(v)
}

func (v *Value) IsList() bool {
	return v.GetTypeKind() == ssa.SliceTypeKind
}

func (v *Value) ExactMatch(ctx context.Context, mod int, want string) (bool, sfvm.ValueOperator, error) {
	value := _SearchValue(v, mod, func(s string) bool { return s == want }, sfvm.WithAnalysisContext_Label("search-exact:"+want))
	return value != nil, value, nil
}

func (v *Value) GlobMatch(ctx context.Context, mod int, g string) (bool, sfvm.ValueOperator, error) {
	value := _SearchValue(v, mod, func(s string) bool {
		return glob.MustCompile(g).Match(s)
	}, sfvm.WithAnalysisContext_Label("search-glob:"+g))
	return value != nil, value, nil
}

func (v *Value) Merge(sf ...sfvm.ValueOperator) (sfvm.ValueOperator, error) {
	var vals []sfvm.ValueOperator
	vals = append(vals, v)
	vals = append(vals, sf...)
	return sfvm.NewValues(vals), nil
}

func (v *Value) RegexpMatch(ctx context.Context, mod int, re string) (bool, sfvm.ValueOperator, error) {
	value := _SearchValue(v, mod, func(s string) bool {
		return regexp.MustCompile(re).MatchString(s)
	}, sfvm.WithAnalysisContext_Label("search-regexp:"+re))
	return value != nil, value, nil
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
	for _, value := range vs {
		if utils.IsNil(value) {
			continue
		}
		value.AppendPredecessor(v, sfvm.WithAnalysisContext_Label("all-actual-args"))
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
	// return v.GetCalledBy(), nil
	ret := v.GetCalledBy()
	ret.AppendPredecessor(v, sfvm.WithAnalysisContext_Label("call"))
	return ret, nil
}

func (v *Value) GetFields() (sfvm.ValueOperator, error) {
	if v.IsMap() || v.IsObject() {
		members := lo.Map(v.GetAllMember(), func(item *Value, index int) sfvm.ValueOperator {
			return item
		})
		return sfvm.NewValues(members), nil
	}
	return sfvm.NewValues(nil), nil
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
func (v *Value) GetSyntaxFlowTopDef(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return WithSyntaxFlowConfig(sfResult, sfConfig, v.GetTopDefs, config...), nil
}

func (v *Value) GetSyntaxFlowBottomUse(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return WithSyntaxFlowConfig(sfResult, sfConfig, v.GetBottomUses, config...), nil
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

func (v *Value) FileFilter(path string, match string, rule map[string]string, rule2 []string) (sfvm.ValueOperator, error) {
	return v.ParentProgram.FileFilter(path, match, rule, rule2)
}
