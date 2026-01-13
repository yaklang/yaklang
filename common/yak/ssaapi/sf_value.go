package ssaapi

import (
	"context"
	"fmt"
	"regexp"
	"slices"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/yakunquote"

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
	return v.getOpcode().String()
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

func (v *Value) ExactMatch(ctx context.Context, mod int, want string) sfvm.Values {
	value := _SearchValue(v, mod, func(s string) bool { return s == want }, sfvm.WithAnalysisContext_Label("search-exact:"+want))
	return sfvm.Values(lo.Map(value, func(v *Value, _ int) sfvm.ValueOperator { return v }))
}

func (v *Value) GlobMatch(ctx context.Context, mod int, g string) sfvm.Values {
	value := _SearchValue(v, mod, func(s string) bool {
		return glob.MustCompile(g).Match(s)
	}, sfvm.WithAnalysisContext_Label("search-glob:"+g))
	return sfvm.Values(lo.Map(value, func(v *Value, _ int) sfvm.ValueOperator { return v }))
}

func (v *Value) Merge(sf ...sfvm.ValueOperator) (sfvm.Values, error) {
	var vals = []sfvm.ValueOperator{v}
	vals = append(vals, sf...)
	merged := MergeSFValueOperator(vals...)
	var res sfvm.Values
	merged.ForEach(func(vo sfvm.ValueOperator) error {
		res = append(res, vo)
		return nil
	})
	return res, nil
}

func (v *Value) RegexpMatch(ctx context.Context, mod int, re string) sfvm.Values {
	value := _SearchValue(v, mod, func(s string) bool {
		return regexp.MustCompile(re).MatchString(s)
	}, sfvm.WithAnalysisContext_Label("search-regexp:"+re))
	return sfvm.Values(lo.Map(value, func(v *Value, _ int) sfvm.ValueOperator { return v }))
}

func (v *Value) CompareString(items *sfvm.StringComparator) (sfvm.Values, bool) {
	if v == nil || items == nil {
		return sfvm.NewEmptyValues(), false
	}

	names := getValueNames(v)
	names = append(names, yakunquote.TryUnquote(v.String()))
	if items.Matches(names...) {
		return sfvm.Values{v}, true
	}
	return sfvm.NewEmptyValues(), false
}

func (v *Value) CompareConst(comparator *sfvm.ConstComparator) bool {
	if v == nil || comparator == nil {
		return false
	}
	return comparator.Matches(v.String())
}

func (v *Value) CompareOpcode(comparator *sfvm.OpcodeComparator) (sfvm.Values, bool) {
	if v == nil || comparator == nil {
		return sfvm.NewEmptyValues(), false
	}
	checkOp := func(opcode ssa.Opcode) bool {
		return v.getOpcode() == opcode
	}
	checkBinOrUnaryOp := func(binOp string) bool {
		ops := []string{v.GetBinaryOperator(), v.GetUnaryOperator()}
		return slices.Contains(ops, binOp)
	}
	if comparator.AllSatisfy(checkOp, checkBinOrUnaryOp) {
		return sfvm.Values{v}, true
	}
	return sfvm.NewEmptyValues(), false
}

func (v *Value) Remove(sf ...sfvm.ValueOperator) (sfvm.Values, error) {
	for _, operator := range sf {
		err := operator.Recursive(func(vo sfvm.ValueOperator) error {
			if raw, ok := vo.(ssa.GetIdIF); ok {
				if v.GetId() == raw.GetId() {
					return utils.Error("abort")
				}
			}
			return nil
		})
		if err != nil {
			return sfvm.NewEmptyValues(), nil
		}
	}
	return sfvm.Values{v}, nil
}

func (v *Value) GetCallActualParams(start int, contain bool) (sfvm.Values, error) {
	call, isCall := ssa.ToCall(v.innerValue)
	if !isCall {
		return sfvm.NewEmptyValues(), utils.Errorf("ssa.Value is not a call")
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
	if len(rets) == 0 {
		return sfvm.NewEmptyValues(), utils.Errorf("ssa.Value no actual params")
	}
	return sfvm.Values(lo.Map(rets, func(v *Value, _ int) sfvm.ValueOperator { return v })), nil
}

func (v *Value) GetCalled() (sfvm.Values, error) {
	vs := v.GetCalledBy()
	ret := ValuesToSFValues(vs)
	ret.AppendPredecessor(v, sfvm.WithAnalysisContext_Label("call"))
	return ret, nil
}

func (v *Value) GetFields() (sfvm.Values, error) {
	if v.IsMap() || v.IsObject() {
		members := lo.Map(v.GetAllMember(), func(item *Value, index int) sfvm.ValueOperator {
			return item
		})
		return sfvm.Values(members), nil
	}
	return nil, nil
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

func (v *Value) GetSyntaxFlowUse() (sfvm.Values, error) {
	return sfvm.NewValues(lo.Map(v.GetUsers(), func(v *Value, _ int) sfvm.ValueOperator { return v })...), nil
}
func (v *Value) GetSyntaxFlowDef() (sfvm.Values, error) {
	return ValuesToSFValues(v.GetOperands()), nil
}
func (v *Value) GetSyntaxFlowTopDef(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	// DataFlowWithSFConfig 返回 sfvm.ValueOperator，需要提取其中的 Values
	result := DataFlowWithSFConfig(sfResult, sfConfig, v, TopDefAnalysis, config...)
	if result == nil {
		return sfvm.NewEmptyValues(), nil
	}
	var res sfvm.Values
	result.ForEach(func(vo sfvm.ValueOperator) error {
		res = append(res, vo)
		return nil
	})
	return res, nil
}

func (v *Value) GetSyntaxFlowBottomUse(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	// DataFlowWithSFConfig 返回 sfvm.ValueOperator，需要提取其中的 Values
	result := DataFlowWithSFConfig(sfResult, sfConfig, v, BottomUseAnalysis, config...)
	if result == nil {
		return sfvm.NewEmptyValues(), nil
	}
	var res sfvm.Values
	result.ForEach(func(vo sfvm.ValueOperator) error {
		res = append(res, vo)
		return nil
	})
	return res, nil
}

func (v *Value) ListIndex(i int) (sfvm.Values, error) {
	if i == 0 {
		return sfvm.Values{v}, nil
	}
	if !v.IsList() {
		return sfvm.NewEmptyValues(), utils.Error("ssa.Value is not a list")
	}
	members := v.GetMember(v.NewValue(ssa.NewConst(i)))
	for _, member := range members {
		if member != nil {
			return sfvm.Values{member}, nil
		}
	}
	return sfvm.NewEmptyValues(), utils.Errorf("ssa.Value %v cannot call by slice, like v[%v]", v.String(), i)
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

func (v *Value) FileFilter(path string, match string, rule map[string]string, rule2 []string) (sfvm.Values, error) {
	// FileFilter 现在返回 sfvm.Values，直接返回
	return v.ParentProgram.FileFilter(path, match, rule, rule2)
}

func (v *Value) NewConst(i any, rng ...*memedit.Range) sfvm.ValueOperator {
	return v.NewConstValue(i, rng...)
}
