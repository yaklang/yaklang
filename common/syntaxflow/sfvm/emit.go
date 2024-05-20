package sfvm

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils/omap"
)

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

func (v *SyntaxFlowVisitor) EmitEqual(i any) {
	switch i.(type) {
	case string:
	case int:
	}
}

func (v *SyntaxFlowVisitor) EmitSearchExact(isMember bool, i string) {
	v.codes = append(v.codes, &SFI{
		OpCode:    OpPushSearchExact,
		UnaryStr:  i,
		UnaryBool: isMember,
	})
}

func (v *SyntaxFlowVisitor) EmitSearchGlob(isMember bool, i string) {
	v.codes = append(v.codes, &SFI{
		OpCode:    OpPushSearchGlob,
		UnaryStr:  i,
		UnaryBool: isMember,
	})
}

func (v *SyntaxFlowVisitor) EmitSearchRegexp(isMember bool, i string) {
	v.codes = append(v.codes, &SFI{
		OpCode:    OpPushSearchRegexp,
		UnaryStr:  i,
		UnaryBool: isMember,
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

func (v *SyntaxFlowVisitor) EmitGetBottomUsersWithConfig(config []*ConfigItem) {
	v.codes = append(v.codes, &SFI{OpCode: OpGetBottomUsers, SyntaxFlowConfig: config})
}

func (v *SyntaxFlowVisitor) EmitGetTopDefs() {
	v.codes = append(v.codes, &SFI{OpCode: OpGetTopDefs})
}

func (v *SyntaxFlowVisitor) EmitGetTopDefsWithConfig(config []*ConfigItem) {
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

func (y *SyntaxFlowVisitor) EmitCheckStackTop() {
	y.codes = append(y.codes, &SFI{
		OpCode: OpCheckStackTop,
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
