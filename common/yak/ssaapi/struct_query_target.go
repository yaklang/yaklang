package ssaapi

import (
	"context"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// StructQueryTarget is a SyntaxFlow feed root for mode=struct rules.
// It must not embed *Program: unimplemented ValueOperator methods would scan
// the whole program.
type StructQueryTarget struct {
	prog  *Program
	unit  *ssa.CompileUnit
	bound *structBound
}

var _ sfvm.ValueOperator = (*StructQueryTarget)(nil)

func NewStructQueryTarget(prog *Program, unit *ssa.CompileUnit, bound *structBound) *StructQueryTarget {
	if bound == nil && unit != nil && prog != nil && prog.Program != nil {
		bound = newStructBound(unit, prog.Program)
	}
	return &StructQueryTarget{prog: prog, unit: unit, bound: bound}
}

func (t *StructQueryTarget) ResultProgram() *Program {
	if t == nil {
		return nil
	}
	return t.prog
}

func (t *StructQueryTarget) Unit() *ssa.CompileUnit {
	if t == nil {
		return nil
	}
	return t.unit
}

func (t *StructQueryTarget) String() string {
	if t == nil || t.unit == nil {
		return "struct-query"
	}
	return "struct-query:" + t.unit.Key
}

func (t *StructQueryTarget) IsMap() bool  { return false }
func (t *StructQueryTarget) IsList() bool { return false }
func (t *StructQueryTarget) IsEmpty() bool {
	return t == nil || t.prog == nil || t.prog.Program == nil
}
func (t *StructQueryTarget) ShouldUseConditionCandidate() bool { return true }
func (t *StructQueryTarget) GetOpcode() string {
	return ssa.SSAOpcode2Name[ssa.SSAOpcodeUnKnow]
}
func (t *StructQueryTarget) GetBinaryOperator() string {
	return ssa.SSAOpcode2Name[ssa.SSAOpcodeUnKnow]
}
func (t *StructQueryTarget) GetUnaryOperator() string {
	return ssa.SSAOpcode2Name[ssa.SSAOpcodeUnKnow]
}
func (t *StructQueryTarget) GetCalled() (sfvm.Values, error) {
	return nil, utils.Error("struct query target is not supported called")
}
func (t *StructQueryTarget) GetCallActualParams(int, bool) (sfvm.Values, error) {
	return nil, utils.Error("struct query target is not supported call actual params")
}
func (t *StructQueryTarget) GetFields() (sfvm.Values, error) {
	return sfvm.NewEmptyValues(), nil
}
func (t *StructQueryTarget) GetSyntaxFlowUse() (sfvm.Values, error) {
	return nil, utils.Error("struct query target is not supported syntax flow use")
}
func (t *StructQueryTarget) GetSyntaxFlowDef() (sfvm.Values, error) {
	return nil, utils.Error("struct query target is not supported syntax flow def")
}
func (t *StructQueryTarget) GetSyntaxFlowTopDef(*sfvm.SFFrameResult, *sfvm.Config, ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	return nil, utils.Error("struct query target is not supported syntax flow top def")
}
func (t *StructQueryTarget) GetSyntaxFlowBottomUse(*sfvm.SFFrameResult, *sfvm.Config, ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	return nil, utils.Error("struct query target is not supported syntax flow bottom use")
}
func (t *StructQueryTarget) ListIndex(int) (sfvm.ValueOperator, error) {
	return nil, utils.Error("struct query target is not supported list index")
}
func (t *StructQueryTarget) AppendPredecessor(sfvm.ValueOperator, ...sfvm.AnalysisContextOption) error {
	return nil
}
func (t *StructQueryTarget) CompareConst(*sfvm.ConstComparator) bool { return false }
func (t *StructQueryTarget) NewConst(i any, rng ...*memedit.Range) sfvm.ValueOperator {
	if t == nil || t.prog == nil {
		return nil
	}
	return t.prog.NewConstValue(i, rng...)
}
func (t *StructQueryTarget) GetAnchorBitVector() *utils.BitVector { return nil }
func (t *StructQueryTarget) SetAnchorBitVector(*utils.BitVector)  {}

func (t *StructQueryTarget) Recursive(f func(sfvm.ValueOperator) error) error {
	if t == nil {
		return nil
	}
	return f(t)
}

func (t *StructQueryTarget) allowedInsts(insts []ssa.Instruction) Values {
	if t == nil || t.prog == nil {
		return nil
	}
	var out Values
	for _, inst := range insts {
		if inst == nil {
			continue
		}
		if t.bound != nil && !t.bound.Allow(inst) {
			continue
		}
		val, err := t.prog.NewValue(inst)
		if err != nil || val == nil {
			continue
		}
		out = append(out, val)
	}
	return out
}

func (t *StructQueryTarget) CompareOpcode(opcodeItems *sfvm.OpcodeComparator) (sfvm.Values, []bool) {
	if t == nil || t.prog == nil || t.prog.Program == nil || opcodeItems == nil {
		return sfvm.NewEmptyValues(), nil
	}
	insts := ssa.MatchInstructionByOpcodesResident(t.prog.Program, opcodeItems.Opcodes...)
	return ToSFVMValues(t.allowedInsts(insts)), nil
}

func (t *StructQueryTarget) CompareString(comparator *sfvm.StringComparator) (sfvm.Values, []bool) {
	if t == nil || t.prog == nil {
		return sfvm.NewEmptyValues(), nil
	}
	vals, _ := t.prog.compareStringWithFileFilter(comparator, t.includeFiles(), nil)
	return filterSFVMValuesByStructBound(t.bound, vals), nil
}

func (t *StructQueryTarget) ExactMatch(ctx context.Context, mod ssadb.MatchMode, s string) (bool, sfvm.Values, error) {
	return t.matchVariable(ctx, ssadb.ExactCompare, mod, s)
}
func (t *StructQueryTarget) GlobMatch(ctx context.Context, mod ssadb.MatchMode, g string) (bool, sfvm.Values, error) {
	return t.matchVariable(ctx, ssadb.GlobCompare, mod, g)
}
func (t *StructQueryTarget) RegexpMatch(ctx context.Context, mod ssadb.MatchMode, re string) (bool, sfvm.Values, error) {
	return t.matchVariable(ctx, ssadb.RegexpCompare, mod, re)
}

func (t *StructQueryTarget) includeFiles() []string {
	if t == nil || t.unit == nil {
		return nil
	}
	return t.unit.Files
}

func (t *StructQueryTarget) matchVariable(ctx context.Context, compareMode ssadb.CompareMode, mod ssadb.MatchMode, pattern string) (bool, sfvm.Values, error) {
	if t == nil || t.prog == nil || t.prog.Program == nil {
		return false, sfvm.NewEmptyValues(), nil
	}
	insts := ssa.MatchInstructionsByVariableWithIncludeFiles(ctx, t.prog.Program, compareMode, mod, pattern, t.includeFiles())
	vals := t.allowedInsts(insts)
	return len(vals) > 0, ToSFVMValues(vals), nil
}

func (t *StructQueryTarget) FileFilter(path string, match string, rule map[string]string, rule2 []string) (sfvm.Values, error) {
	if t == nil || t.prog == nil {
		return nil, nil
	}
	vals, err := t.prog.FileFilter(path, match, rule, rule2)
	if err != nil {
		return vals, err
	}
	return filterSFVMValuesByStructBound(t.bound, vals), nil
}

func (t *StructQueryTarget) SyntaxFlowWithError(i string, opts ...QueryOption) (*SyntaxFlowResult, error) {
	return t.syntaxFlow(opts, QueryWithRuleContent(i))
}

func (t *StructQueryTarget) SyntaxFlowRule(rule *schema.SyntaxFlowRule, opts ...QueryOption) (*SyntaxFlowResult, error) {
	if t == nil {
		return nil, utils.Error("nil StructQueryTarget")
	}
	if rule != nil && !rule.IsStructMode() {
		return nil, utils.Errorf(
			"struct target cannot execute non-struct rule %s (mode=%s)",
			rule.RuleName,
			schema.ValidRuleMode(rule.Mode),
		)
	}
	return t.syntaxFlow(opts, QueryWithRule(rule))
}

func (t *StructQueryTarget) syntaxFlow(opts []QueryOption, ruleOpt QueryOption) (*SyntaxFlowResult, error) {
	if t == nil || t.unit == nil {
		return nil, utils.Error("nil StructQueryTarget")
	}
	all := make([]QueryOption, 0, len(opts)+4)
	all = append(all, QueryWithValue(t), QueryWithResultProgram(t.prog), QueryWithStruct(t.unit), ruleOpt)
	all = append(all, opts...)
	return QuerySyntaxflow(all...)
}

func (t *StructQueryTarget) GetProgramName() string {
	if t == nil || t.prog == nil {
		return ""
	}
	return t.prog.GetProgramName()
}

func (t *StructQueryTarget) GetLanguage() ssaconfig.Language {
	if t == nil || t.prog == nil {
		return ssaconfig.General
	}
	return t.prog.GetLanguage()
}

func (t *StructQueryTarget) IsIncrementalCompile() bool {
	if t == nil || t.prog == nil {
		return false
	}
	return t.prog.IsIncrementalCompile()
}
func (t *StructQueryTarget) IsBaseProgram() bool {
	if t == nil || t.prog == nil {
		return true
	}
	return t.prog.IsBaseProgram()
}
func (t *StructQueryTarget) GetBaseProgramName() string {
	if t == nil || t.prog == nil {
		return ""
	}
	return t.prog.GetBaseProgramName()
}
func (t *StructQueryTarget) Recompile(inputOpt ...ssaconfig.Option) error {
	return nil
}

var _ SyntaxFlowQueryInstance = (*StructQueryTarget)(nil)
