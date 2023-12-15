package sfvm

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func (y *SyntaxFlowVisitor[V]) EmitMapBuild(i int) {
	y.codes = append(y.codes, &SFI[V]{
		OpCode:   OpMap,
		UnaryInt: i,
	})
}

func (y *SyntaxFlowVisitor[V]) EmitNewRef(i string) {
	y.codes = append(y.codes, &SFI[V]{
		OpCode:   OpNewRef,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor[V]) EmitUpdate(i string) {
	y.codes = append(y.codes, &SFI[V]{
		OpCode:   OpUpdateRef,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor[V]) EmitFlat(i int) {
	y.codes = append(y.codes, &SFI[V]{
		OpCode:   OpFlat,
		UnaryInt: i,
	})
}

func (y *SyntaxFlowVisitor[V]) EmitOperator(i string) {
	switch i {
	case ">":
		y.codes = append(y.codes, &SFI[V]{OpCode: OpGt})
	case ">=":
		y.codes = append(y.codes, &SFI[V]{OpCode: OpGtEq})
	case "<":
		y.codes = append(y.codes, &SFI[V]{OpCode: OpLt})
	case "<=":
		y.codes = append(y.codes, &SFI[V]{OpCode: OpLtEq})
	case "==", "=":
		y.codes = append(y.codes, &SFI[V]{OpCode: OpEq})
	case "!=":
		y.codes = append(y.codes, &SFI[V]{OpCode: OpNotEq})
	case "&&":
		y.codes = append(y.codes, &SFI[V]{OpCode: OpLogicAnd})
	case "||":
		y.codes = append(y.codes, &SFI[V]{OpCode: OpLogicOr})
	default:
		panic(fmt.Sprintf("unknown operator: %s", i))
	}
}

func (v *SyntaxFlowVisitor[V]) EmitPushGlob(i string) {
	v.codes = append(v.codes, &SFI[V]{
		OpCode:   OpGlobMatch,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor[V]) EmitRegexpMatch(i string) {
	y.codes = append(y.codes, &SFI[V]{
		OpCode:   OpReMatch,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor[V]) EmitPushLiteral(i any) {
	switch ret := i.(type) {
	case string:
		v.codes = append(v.codes, &SFI[V]{
			OpCode:   OpPushString,
			UnaryStr: ret,
		})
	case int:
		v.codes = append(v.codes, &SFI[V]{
			OpCode:   OpPushNumber,
			UnaryInt: ret,
		})
	case bool:
		if ret {
			v.codes = append(v.codes, &SFI[V]{
				OpCode:   OpPushBool,
				UnaryInt: 1,
			})
		} else {
			v.codes = append(v.codes, &SFI[V]{
				OpCode:   OpPushString,
				UnaryInt: 0,
			})
		}
	default:
		panic(fmt.Sprintf("unknown type: %T", ret))
	}

}

func (v *SyntaxFlowVisitor[V]) EmitRef(i string) {
	v.codes = append(v.codes, &SFI[V]{
		OpCode:   OpPushRef,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor[V]) EmitEqual(i any) {
	switch i.(type) {
	case string:
	case int:
	}
}

func (v *SyntaxFlowVisitor[V]) EmitField(i string) {
	v.codes = append(v.codes, &SFI[V]{
		OpCode:   OpFetchField,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor[V]) EmitTypeCast(i string) {
	v.codes = append(v.codes, &SFI[V]{
		OpCode:   OpTypeCast,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor[V]) EmitSearch(i string) {
	v.codes = append(v.codes, &SFI[V]{
		OpCode:   OpPushMatch,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor[V]) EmitIndex(i int) {
	v.codes = append(v.codes, &SFI[V]{
		OpCode:   OpPushIndex,
		UnaryInt: i,
	})
}

func (v *SyntaxFlowVisitor[V]) EmitDirection(i string) {
	switch i {
	case ">>", "<<":
		v.codes = append(v.codes, &SFI[V]{
			OpCode:   OpSetDirection,
			UnaryStr: i,
		})
	default:
		panic("unknown direction")
	}
}

func (v *SyntaxFlowVisitor[V]) Show() {
	for _, c := range v.codes {
		fmt.Println(c.String())
	}
}

func (v *SyntaxFlowVisitor[V]) CreateFrame(vars *omap.OrderedMap[string, *omap.OrderedMap[string, V]]) *SFFrame[V] {
	return NewSFFrame(vars, v.text, v.codes)
}
