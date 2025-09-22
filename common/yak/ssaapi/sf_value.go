package ssaapi

import (
	"context"
	"fmt"
	"regexp"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/yakunquote"

	"golang.org/x/exp/slices"

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

func (v *Value) IsEmpty() bool {
	return v == nil
}

func (v *Value) GetOpcode() string {
	return ssa.SSAOpcode2Name[v.getOpcode()]
}

func (v *Value) GetBinaryOperator() string {
	sa := v.GetSSAInst()
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
	sa := v.GetSSAInst()
	if utils.IsNil(sa) {
		return ""
	}
	if sa.GetOpcode() == ssa.SSAOpcodeUnOp {
		unOp, ok := ssa.ToUnOp(sa)
		if !ok {
			return ""
		}
		return string(unOp.Op)
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
	return value.Len() != 0, value, nil
}

func (v *Value) GlobMatch(ctx context.Context, mod int, g string) (bool, sfvm.ValueOperator, error) {
	value := _SearchValue(v, mod, func(s string) bool {
		return glob.MustCompile(g).Match(s)
	}, sfvm.WithAnalysisContext_Label("search-glob:"+g))
	return value.Len() != 0, value, nil
}

func (v *Value) Merge(sf ...sfvm.ValueOperator) (sfvm.ValueOperator, error) {
	var vals = []sfvm.ValueOperator{v}
	vals = append(vals, sf...)
	return MergeSFValueOperator(vals...), nil
}

func (v *Value) RegexpMatch(ctx context.Context, mod int, re string) (bool, sfvm.ValueOperator, error) {
	value := _SearchValue(v, mod, func(s string) bool {
		return regexp.MustCompile(re).MatchString(s)
	}, sfvm.WithAnalysisContext_Label("search-regexp:"+re))
	return value.Len() != 0, value, nil
}

func (v *Value) CompareString(items *sfvm.StringComparator) (sfvm.ValueOperator, []bool) {
	if v == nil || items == nil {
		return nil, []bool{false}
	}

	names := getValueNames(v)
	names = append(names, yakunquote.TryUnquote(v.String()))
	return v, []bool{items.Matches(names...)}
}

func (v *Value) CompareConst(comparator *sfvm.ConstComparator) []bool {
	if v == nil || comparator == nil {
		return []bool{false}
	}
	return []bool{comparator.Matches(v.String())}
}

func (v *Value) CompareOpcode(comparator *sfvm.OpcodeComparator) (sfvm.ValueOperator, []bool) {
	if v == nil || comparator == nil {
		return nil, []bool{false}
	}
	checkOp := func(opcode ssa.Opcode) bool {
		return v.getOpcode() == opcode
	}
	checkBinOrUnaryOp := func(binOp string) bool {
		ops := []string{v.GetBinaryOperator(), v.GetUnaryOperator()}
		return slices.Contains(ops, binOp)
	}
	return v, []bool{comparator.AllSatisfy(checkOp, checkBinOrUnaryOp)}
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
		return sfvm.NewEmptyValues(), nil
	}
	return v, nil
}

func (v *Value) GetCallActualParams(start int, contain bool) (sfvm.ValueOperator, error) {
	call, isCall := ssa.ToCall(v.innerValue)
	if !isCall {
		return nil, utils.Errorf("ssa.Value is not a call")
	}

	rets := make(Values, 0)
	addvalue := func(id int64) {
		value, ok := call.GetValueById(id)
		if !ok {
			return
		}
		ret := v.NewValue(value)
		ret.AppendPredecessor(v, sfvm.WithAnalysisContext_Label(
			fmt.Sprintf("actual-args[%d](containRest:%v)", start, contain),
		))
		rets = append(rets, ret)
	}
	add := func(param []int64) {
		if len(param) <= start {
			return
		}
		if contain {
			for i := start; i < len(param); i++ {
				value := param[i]
				addvalue(value)
			}
		} else {
			value := param[start]
			addvalue(value)
		}
	}
	add(call.Args)
	if utils.IsNil(rets) {
		return nil, utils.Errorf("ssa.Value no actual params")
	}
	return rets, nil
}

func (v *Value) GetCalled() (sfvm.ValueOperator, error) {
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
	return sfvm.NewEmptyValues(), nil
}

func (v *Value) GetMembersByString(key string) (sfvm.ValueOperator, bool) {
	if v.IsMap() || v.IsList() || v.IsObject() {
		for _, m := range v.GetMember(v.NewValue(ssa.NewConst(key))) {
			if m != nil {
				return m, true
			}
		}
		return nil, false
	}
	// return v.GetUsers(), nil
	return nil, false
}

func (v *Value) GetSyntaxFlowUse() (sfvm.ValueOperator, error) {
	return v.GetUsers(), nil
}
func (v *Value) GetSyntaxFlowDef() (sfvm.ValueOperator, error) {
	return v.GetOperands(), nil
}
func (v *Value) GetSyntaxFlowTopDef(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return DataFlowWithSFConfig(sfResult, sfConfig, v, TopDefAnalysis, config...), nil
}

func (v *Value) GetSyntaxFlowBottomUse(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return DataFlowWithSFConfig(sfResult, sfConfig, v, BottomUseAnalysis, config...), nil
}

func (v *Value) ListIndex(i int) (sfvm.ValueOperator, error) {
	if i == 0 {
		return v, nil
	}
	if !v.IsList() {
		return nil, utils.Error("ssa.Value is not a list")
	}
	members := v.GetMember(v.NewValue(ssa.NewConst(i)))
	for _, member := range members {
		if member != nil {
			return member, nil
		}
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
			if len(v.Predecessors) > 30 {
				log.Warnf("Value %s Predecessors too many: %d", v.StringWithRange(), len(v.Predecessors))
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

func (v *Value) NewConst(i any, rng ...*memedit.Range) sfvm.ValueOperator {
	return v.NewConstValue(i, rng...)
}
