package sfvm

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func (y *SyntaxFlowVisitor) EmitMapBuildDone(refs ...string) {
	i := len(refs)
	y.codes = append(y.codes, &SFI{
		OpCode: OpMapDone, UnaryInt: i,
		Values: refs,
	})
}

func (y *SyntaxFlowVisitor) EmitMapBuildStart() {
	y.codes = append(y.codes, &SFI{
		OpCode: OpMapStart,
	})
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

func (y *SyntaxFlowVisitor) EmitWithdraw() {
	y.codes = append(y.codes, &SFI{
		OpCode: OpWithdraw,
	})
}

func (y *SyntaxFlowVisitor) EmitRestoreMapContext() {
	y.codes = append(y.codes, &SFI{OpCode: OpRestoreMapContext})
}

func (y *SyntaxFlowVisitor) EmitRestoreFlatContext() {
	y.codes = append(y.codes, &SFI{OpCode: OpRestoreFlatContext})
}

func (y *SyntaxFlowVisitor) EmitFlatDone(i int) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpFlatDone,
		UnaryInt: i,
	})
}

func (y *SyntaxFlowVisitor) EmitFlatStart(i int) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpFlatStart,
		UnaryInt: i,
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

func (v *SyntaxFlowVisitor) EmitRef(i string) {
	v.codes = append(v.codes, &SFI{
		OpCode:   OpPushRef,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor) EmitEqual(i any) {
	switch i.(type) {
	case string:
	case int:
	}
}

func (v *SyntaxFlowVisitor) EmitField(i string) {
	v.codes = append(v.codes, &SFI{
		OpCode:   OpFetchField,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor) EmitFetchIndex(i int) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpFetchIndex,
		UnaryInt: i,
	})
}

func (v *SyntaxFlowVisitor) EmitTypeCast(i string) {
	v.codes = append(v.codes, &SFI{
		OpCode:   OpTypeCast,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor) EmitSearch(i string) {
	v.codes = append(v.codes, &SFI{
		OpCode:   OpPushMatch,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor) EmitPushIndex(i int) {
	v.codes = append(v.codes, &SFI{
		OpCode:   OpPushIndex,
		UnaryInt: i,
	})
}

func (v *SyntaxFlowVisitor) EmitDirection(i string) {
	switch i {
	case ">>", "<<":
		v.codes = append(v.codes, &SFI{
			OpCode:   OpSetDirection,
			UnaryStr: i,
		})
	default:
		panic("unknown direction")
	}
}

func (v *SyntaxFlowVisitor) Show() {
	for _, c := range v.codes {
		fmt.Println(c.String())
	}
}

func (v *SyntaxFlowVisitor) CreateFrame(vars *omap.OrderedMap[string, any]) *SFFrame {
	return NewSFFrame(vars, v.text, v.codes)
}

func (y *SyntaxFlowVisitor) EmitPop() {
	y.codes = append(y.codes, &SFI{
		OpCode: OpPop,
	})
}
