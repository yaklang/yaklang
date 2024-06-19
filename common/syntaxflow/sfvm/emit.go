package sfvm

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils/omap"
)

func (y *SyntaxFlowVisitor) EmitExitStatement() {
	y.codes = append(y.codes, &SFI{
		OpCode: OpExitStatement,
	})
}

func (y *SyntaxFlowVisitor) EmitEnterStatement() *SFI {
	enter := &SFI{
		OpCode: OpEnterStatement,
	}
	y.codes = append(y.codes, enter)
	return enter
}

func (y *SyntaxFlowVisitor) EmitNewRef(i string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpNewRef,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor) EmitUpdate(i string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpUpdateRef,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor) EmitOperator(i string) {
	switch i {
	case ">":
		y.codes = append(y.codes, &SFI{OpCode: OpGt})
	case ">=":
		y.codes = append(y.codes, &SFI{OpCode: OpGtEq})
	case "<":
		y.codes = append(y.codes, &SFI{OpCode: OpLt})
	case "<=":
		y.codes = append(y.codes, &SFI{OpCode: OpLtEq})
	case "==", "=":
		y.codes = append(y.codes, &SFI{OpCode: OpEq})
	case "!=":
		y.codes = append(y.codes, &SFI{OpCode: OpNotEq})
	case "&&":
		y.codes = append(y.codes, &SFI{OpCode: OpLogicAnd})
	case "||":
		y.codes = append(y.codes, &SFI{OpCode: OpLogicOr})
	case "!":
		y.codes = append(y.codes, &SFI{OpCode: OpLogicBang})
	default:
		panic(fmt.Sprintf("unknown operator: %s", i))
	}
}

func (y *SyntaxFlowVisitor) EmitCheckParam(ref string, then string, elseString string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpCheckParams,
		UnaryStr: ref,
		Values:   []string{then, elseString},
	})
}

func (y *SyntaxFlowVisitor) EmitAddDescription(key string, value string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpAddDescription,
		UnaryStr: key,
		Values:   []string{key, value},
	})
}

func (v *SyntaxFlowVisitor) EmitPushGlob(i string) {
	v.codes = append(v.codes, &SFI{
		OpCode:   OpGlobMatch,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor) EmitRegexpMatch(i string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpReMatch,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor) EmitPushLiteral(i any) {
	switch ret := i.(type) {
	case string:
		v.codes = append(v.codes, &SFI{
			OpCode:   OpPushString,
			UnaryStr: ret,
		})
	case int:
		v.codes = append(v.codes, &SFI{
			OpCode:   OpPushNumber,
			UnaryInt: ret,
		})
	case bool:
		if ret {
			v.codes = append(v.codes, &SFI{
				OpCode:   OpPushBool,
				UnaryInt: 1,
			})
		} else {
			v.codes = append(v.codes, &SFI{
				OpCode:   OpPushString,
				UnaryInt: 0,
			})
		}
	default:
		panic(fmt.Sprintf("unknown type: %T", ret))
	}

}

func (v *SyntaxFlowVisitor) EmitCompareOpcode(i []string) {
	v.codes = append(v.codes, &SFI{
		OpCode: OpCompareOpcode,
		Values: i,
	})
}

const (
	CompareStringAnyMode  int = 0
	CompareStringHaveMode     = 1
)

func (v *SyntaxFlowVisitor) EmitCompareString(i []string, mode int) {
	v.codes = append(v.codes, &SFI{
		OpCode:   OpCompareString,
		Values:   i,
		UnaryInt: mode,
	})
}

func (v *SyntaxFlowVisitor) EmitCondition() {
	v.codes = append(v.codes, &SFI{
		OpCode: OpCondition,
	})
}

func (v *SyntaxFlowVisitor) EmitEqual(i any) {
	switch i.(type) {
	case string:
	case int:
	}
}

func (v *SyntaxFlowVisitor) EmitSearchExact(mod int, i string) {
	v.codes = append(v.codes, &SFI{
		OpCode:   OpPushSearchExact,
		UnaryStr: i,
		UnaryInt: mod,
	})
}

func (v *SyntaxFlowVisitor) EmitSearchGlob(mod int, i string) {
	v.codes = append(v.codes, &SFI{
		OpCode:   OpPushSearchGlob,
		UnaryStr: i,
		UnaryInt: mod,
	})
}

func (v *SyntaxFlowVisitor) EmitSearchRegexp(mod int, i string) {
	v.codes = append(v.codes, &SFI{
		OpCode:   OpPushSearchRegexp,
		UnaryStr: i,
		UnaryInt: mod,
	})
}

func (v *SyntaxFlowVisitor) EmitGetUsers() {
	v.codes = append(v.codes, &SFI{OpCode: OpGetUsers})
}

func (v *SyntaxFlowVisitor) EmitGetDefs() {
	v.codes = append(v.codes, &SFI{OpCode: OpGetDefs})
}

func (v *SyntaxFlowVisitor) EmitGetBottomUsers() {
	v.codes = append(v.codes, &SFI{OpCode: OpGetBottomUsers})
}

func (v *SyntaxFlowVisitor) EmitGetBottomUsersWithConfig(config []*RecursiveConfigItem) {
	v.codes = append(v.codes, &SFI{OpCode: OpGetBottomUsers, SyntaxFlowConfig: config})
}

func (v *SyntaxFlowVisitor) EmitGetTopDefs() {
	v.codes = append(v.codes, &SFI{OpCode: OpGetTopDefs})
}

func (v *SyntaxFlowVisitor) EmitGetTopDefsWithConfig(config []*RecursiveConfigItem) {
	v.codes = append(v.codes, &SFI{OpCode: OpGetTopDefs, SyntaxFlowConfig: config})
}

func (v *SyntaxFlowVisitor) EmitPushCallArgs(i int) {
	v.codes = append(v.codes, &SFI{OpCode: OpGetCallArgs, UnaryInt: i})
}

func (v *SyntaxFlowVisitor) EmitPushInput() {
	v.codes = append(v.codes, &SFI{OpCode: OpPushInput})
}
func (v *SyntaxFlowVisitor) EmitDuplicate() {
	v.codes = append(v.codes, &SFI{OpCode: OpDuplicate})
}

func (v *SyntaxFlowVisitor) EmitGetCall() {
	v.codes = append(v.codes, &SFI{OpCode: opGetCall})
}

func (v *SyntaxFlowVisitor) EmitPushAllCallArgs() {
	v.codes = append(v.codes, &SFI{OpCode: OpGetAllCallArgs})
}

func (v *SyntaxFlowVisitor) Show() {
	for _, c := range v.codes {
		fmt.Println(c.String())
	}
}

func (v *SyntaxFlowVisitor) CreateFrame(vars *omap.OrderedMap[string, ValueOperator]) *SFFrame {
	return NewSFFrame(vars, v.text, v.codes)
}

func (y *SyntaxFlowVisitor) EmitPop() {
	y.codes = append(y.codes, &SFI{
		OpCode: OpPop,
	})
}

func (y *SyntaxFlowVisitor) EmitGetTopDef() {
	y.codes = append(y.codes, &SFI{
		OpCode: OpGetTopDefs,
	})
}

func (y *SyntaxFlowVisitor) EmitListIndex(i int) {
	y.codes = append(y.codes, &SFI{OpCode: OpListIndex, UnaryInt: i})
}

func (v *SyntaxFlowVisitor) EmitPass() {
	v.codes = append(v.codes, &SFI{
		OpCode: OpPass,
	})
}

func (v *SyntaxFlowVisitor) EmitCreateIterator() *IterContext {
	idx := len(v.codes)
	it := &IterContext{start: idx}
	v.codes = append(v.codes, &SFI{OpCode: OpCreateIter, iter: it})
	return it
}

func (v *SyntaxFlowVisitor) EmitNextIterator(i *IterContext) {
	i.next = len(v.codes)
	v.codes = append(v.codes, &SFI{OpCode: OpIterValueNext, iter: i})
}

func (v *SyntaxFlowVisitor) EmitIterEnd(i *IterContext) {
	idx := len(v.codes)
	code := &SFI{OpCode: OpEndIter}
	i.end = idx
	v.codes = append(v.codes, code)
}
