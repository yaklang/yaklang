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
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

var _ sfvm.ValueOperator = (*Value)(nil)

func (v *Value) IsMap() bool {
	kind := v.GetTypeKind()
	return kind == ssa.MapTypeKind || kind == ssa.ObjectTypeKind
}

func (v *Value) IsEmpty() bool {
	return v == nil
}

func (v *Value) ShouldUseConditionCandidate() bool {
	return false
}

func (v *Value) MergeProvenanceFrom(source sfvm.ValueOperator) {
	if v == nil || utils.IsNil(source) {
		return
	}
	other, ok := source.(*Value)
	if !ok || other == nil {
		return
	}

	for _, pred := range other.Predecessors {
		v.Predecessors = utils.AppendSliceItemWhenNotExists(v.Predecessors, pred)
	}
	if other.EffectOn != nil {
		other.EffectOn.ForEach(func(_ string, effect *Value) bool {
			effect.RemoveDependOn(other)
			v.AppendEffectOn(effect)
			return true
		})
	}
	if other.DependOn != nil {
		other.DependOn.ForEach(func(_ string, depend *Value) bool {
			depend.RemoveEffectOn(other)
			v.AppendDependOn(depend)
			return true
		})
	}
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

func (v *Value) GetAnchorBitVector() *utils.BitVector {
	if v == nil || v.anchorBits == nil {
		return nil
	}
	return v.anchorBits
}

func (v *Value) SetAnchorBitVector(bits *utils.BitVector) {
	if v == nil {
		return
	}
	if bits == nil {
		v.anchorBits = nil
		return
	}
	v.anchorBits = bits.Clone()
}

func (v *Value) ExactMatch(ctx context.Context, mod ssadb.MatchMode, want string) (bool, sfvm.Values, error) {
	value := _SearchValue(v, mod, func(s string) bool { return s == want }, sfvm.WithAnalysisContext_Label("search-exact:"+want))
	return len(value) > 0, ToSFVMValues(value), nil
}

func (v *Value) GlobMatch(ctx context.Context, mod ssadb.MatchMode, g string) (bool, sfvm.Values, error) {
	value := _SearchValue(v, mod, func(s string) bool {
		return glob.MustCompile(g).Match(s)
	}, sfvm.WithAnalysisContext_Label("search-glob:"+g))
	return len(value) > 0, ToSFVMValues(value), nil
}

func (v *Value) RegexpMatch(ctx context.Context, mod ssadb.MatchMode, re string) (bool, sfvm.Values, error) {
	value := _SearchValue(v, mod, func(s string) bool {
		return regexp.MustCompile(re).MatchString(s)
	}, sfvm.WithAnalysisContext_Label("search-regexp:"+re))
	return len(value) > 0, ToSFVMValues(value), nil
}

func (v *Value) CompareString(items *sfvm.StringComparator) (sfvm.Values, []bool) {
	if v == nil || items == nil {
		return nil, []bool{false}
	}

	names := getValueNames(v)
	names = append(names, yakunquote.TryUnquote(v.String()))
	return sfvm.ValuesOf(v), []bool{items.Matches(names...)}
}

func (v *Value) CompareConst(comparator *sfvm.ConstComparator) bool {
	if v == nil || comparator == nil {
		return false
	}
	return comparator.Matches(v.String())
}

func (v *Value) CompareOpcode(comparator *sfvm.OpcodeComparator) (sfvm.Values, []bool) {
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
	return sfvm.ValuesOf(v), []bool{comparator.AllSatisfy(checkOp, checkBinOrUnaryOp)}
}

func (v *Value) GetCallActualParams(start int, contain bool) (sfvm.Values, error) {
	call, isCall := ssa.ToCall(v.getValue())
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
		sfvm.MergeAnchor(v, ret)
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
	return ToSFVMValues(rets), nil
}

func (v *Value) GetCalled() (sfvm.Values, error) {
	ret := v.GetCalledBy()
	results := ToSFVMValues(ret)
	results.AppendPredecessor(v, sfvm.WithAnalysisContext_Label("call"))
	return results, nil
}

func (v *Value) GetFields() (sfvm.Values, error) {
	if v.IsMap() || v.IsObject() {
		members := lo.Map(v.GetAllMember(), func(item *Value, index int) sfvm.ValueOperator {
			return item
		})
		return sfvm.NewValues(members), nil
	}
	return sfvm.NewEmptyValues(), nil
}

func (v *Value) GetMembersByString(key string) (sfvm.Values, bool) {
	if v.IsMap() || v.IsList() || v.IsObject() {
		for _, m := range v.GetMember(v.NewValue(ssa.NewConst(key))) {
			if m != nil {
				return sfvm.ValuesOf(m), true
			}
		}
		return nil, false
	}
	// return v.GetUsers(), nil
	return nil, false
}

func (v *Value) GetSyntaxFlowUse() (sfvm.Values, error) {
	return ToSFVMValues(v.GetUsers()), nil
}
func (v *Value) GetSyntaxFlowDef() (sfvm.Values, error) {
	return ToSFVMValues(v.GetOperands()), nil
}
func (v *Value) GetSyntaxFlowTopDef(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	// DataFlowWithSFConfig 返回 Values，需要转换为 sfvm.ValueOperator
	return DataFlowWithSFConfig(sfResult, sfConfig, v, TopDefAnalysis, config...), nil
}

func (v *Value) GetSyntaxFlowBottomUse(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	// DataFlowWithSFConfig 返回 Values，需要转换为 sfvm.ValueOperator
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
	result, ok := operator.(*Value)
	if !ok {
		return nil
	}
	ctx := sfvm.NewDefaultAnalysisContext()
	for _, opt := range opts {
		opt(ctx)
	}
	for _, pred := range v.Predecessors {
		if pred == nil || pred.Node == nil {
			continue
		}
		if !ValueCompare(pred.Node, result) {
			continue
		}
		if pred.Info == nil {
			if ctx.Label == "" && ctx.Step == -1 {
				return nil
			}
			continue
		}
		if pred.Info.Label == ctx.Label && pred.Info.Step == ctx.Step {
			return nil
		}
	}
	if len(v.Predecessors) > 30 {
		log.Warnf("Value %s Predecessors too many: %d", v.StringWithRange(), len(v.Predecessors))
	}
	v.Predecessors = append(v.Predecessors, &PredecessorValue{Node: result, Info: ctx})
	return nil
}

func (v *Value) FileFilter(path string, match string, rule map[string]string, rule2 []string) (sfvm.Values, error) {
	return v.ParentProgram.FileFilter(path, match, rule, rule2)
}

func (v *Value) NewConst(i any, rng ...*memedit.Range) sfvm.ValueOperator {
	return v.NewConstValue(i, rng...)
}
