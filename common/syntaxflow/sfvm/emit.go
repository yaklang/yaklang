package sfvm

import "fmt"

func (y *SyntaxFlowVisitor[T, V]) EmitMapBuild(i int) {
	y.codes = append(y.codes, &SFI[T, V]{
		OpCode:   OpMap,
		UnaryInt: i,
	})
}

func (y *SyntaxFlowVisitor[T, V]) EmitNewRef(i string) {
	y.codes = append(y.codes, &SFI[T, V]{
		OpCode:   OpNewRef,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor[T, V]) EmitUpdate(i string) {
	y.codes = append(y.codes, &SFI[T, V]{
		OpCode:   OpUpdateRef,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor[T, V]) EmitFlat(i int) {
	y.codes = append(y.codes, &SFI[T, V]{
		OpCode:   OpFlat,
		UnaryInt: i,
	})
}

func (y *SyntaxFlowVisitor[T, V]) EmitOperator(i string) {
	switch i {
	case ">":
		y.codes = append(y.codes, &SFI[T, V]{OpCode: OpGt})
	case ">=":
		y.codes = append(y.codes, &SFI[T, V]{OpCode: OpGtEq})
	case "<":
		y.codes = append(y.codes, &SFI[T, V]{OpCode: OpLt})
	case "<=":
		y.codes = append(y.codes, &SFI[T, V]{OpCode: OpLtEq})
	case "==", "=":
		y.codes = append(y.codes, &SFI[T, V]{OpCode: OpEq})
	case "!=":
		y.codes = append(y.codes, &SFI[T, V]{OpCode: OpNotEq})
	case "&&":
		y.codes = append(y.codes, &SFI[T, V]{OpCode: OpLogicAnd})
	case "||":
		y.codes = append(y.codes, &SFI[T, V]{OpCode: OpLogicOr})
	default:
		panic(fmt.Sprintf("unknown operator: %s", i))
	}
}

func (v *SyntaxFlowVisitor[T, V]) EmitPushGlob(i string) {
	v.codes = append(v.codes, &SFI[T, V]{
		OpCode:   OpGlobMatch,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor[T, V]) EmitRegexpMatch(i string) {
	y.codes = append(y.codes, &SFI[T, V]{
		OpCode:   OpReMatch,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor[T, V]) EmitPushLiteral(i any) {
	switch ret := i.(type) {
	case string:
		v.codes = append(v.codes, &SFI[T, V]{
			OpCode:   OpPushString,
			UnaryStr: ret,
		})
	case int:
		v.codes = append(v.codes, &SFI[T, V]{
			OpCode:   OpPushNumber,
			UnaryInt: ret,
		})
	case bool:
		if ret {
			v.codes = append(v.codes, &SFI[T, V]{
				OpCode:   OpPushBool,
				UnaryInt: 1,
			})
		} else {
			v.codes = append(v.codes, &SFI[T, V]{
				OpCode:   OpPushString,
				UnaryInt: 0,
			})
		}
	default:
		panic(fmt.Sprintf("unknown type: %T", ret))
	}

}

func (v *SyntaxFlowVisitor[T, V]) EmitRef(i string) {
	v.codes = append(v.codes, &SFI[T, V]{
		OpCode:   OpPushRef,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor[T, V]) EmitEqual(i any) {
	switch i.(type) {
	case string:
	case int:
	}
}

func (v *SyntaxFlowVisitor[T, V]) EmitField(i string) {
	v.codes = append(v.codes, &SFI[T, V]{
		OpCode:   OpFetchField,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor[T, V]) EmitTypeCast(i string) {
	v.codes = append(v.codes, &SFI[T, V]{
		OpCode:   OpTypeCast,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor[T, V]) EmitSearch(i string) {
	v.codes = append(v.codes, &SFI[T, V]{
		OpCode:   OpPushMatch,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor[T, V]) EmitIndex(i int) {
	v.codes = append(v.codes, &SFI[T, V]{
		OpCode:   OpPushIndex,
		UnaryInt: i,
	})
}

func (v *SyntaxFlowVisitor[T, V]) EmitDirection(i string) {
	switch i {
	case ">>", "<<":
		v.codes = append(v.codes, &SFI[T, V]{
			OpCode:   OpSetDirection,
			UnaryStr: i,
		})
	default:
		panic("unknown direction")
	}
}

func (v *SyntaxFlowVisitor[T, V]) Show() {
	for _, c := range v.codes {
		fmt.Println(c.String())
	}
}
