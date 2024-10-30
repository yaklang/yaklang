package go2ssa

import (
	"fmt"

	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type getSingleExpr interface {
	Expression(i int) gol.IExpressionContext
}

func (b *astbuilder) buildExpression(exp *gol.ExpressionContext, IslValue bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	getValue := func(single getSingleExpr, i int) ssa.Value {
		if s := single.Expression(i); s != nil {
			rightv, _ := b.buildExpression(s.(*gol.ExpressionContext), IslValue)
			return rightv
		} else {
			b.NewError(ssa.Error, TAG, "can't get expression")
			return b.EmitConstInst(0)
		}
	}

	fmt.Printf("exp = %v\n", exp.GetText())

	if ret := exp.PrimaryExpr(); ret != nil {
		return b.buildPrimaryExpression(ret.(*gol.PrimaryExprContext), IslValue)
	}

	if !IslValue { // right
		if op := exp.GetUnary_op(); op != nil {
			var ssaop ssa.UnaryOpcode

			switch op.GetText() {
			case "+":
				ssaop = ssa.OpPlus
			case "-":
				ssaop = ssa.OpNeg
			case "!":
				ssaop = ssa.OpNot
			case "^":
				ssaop = ssa.OpBitwiseNot
			case "<-":
				ssaop = ssa.OpChan
			case "*":
				ssaop = ""
			case "&":
				ssaop = ""
			default:
				b.NewError(ssa.Error, TAG, UnaryOperatorNotSupport(op.GetText()))
			}

			op1 := getValue(exp, 0)
			if op1 == nil {
				b.NewError(ssa.Error, TAG, NeedTwoExpression())
				return b.EmitConstInst(0), b.CreateVariable("")
			}
			if ssaop == "" {
				return op1, nil
			}
			return b.EmitUnOp(ssaop, op1), nil
		}

		if op := exp.GetAdd_op(); op != nil {
			var ssaop ssa.BinaryOpcode

			switch op.GetText() {
			case "+":
				ssaop = ssa.OpAdd
			case "-":
				ssaop = ssa.OpSub
			case "|":
				ssaop = ssa.OpOr
			case "^":
				ssaop = ssa.OpXor
			default:
			}

			op1 := getValue(exp, 0)
			op2 := getValue(exp, 1)
			if op1 == nil {
				b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
				return b.EmitConstInst(0), b.CreateVariable("")
			}
			if op2 == nil {
				b.NewError(ssa.Error, TAG, AssignRightSideEmpty())
				return b.EmitConstInst(0), b.CreateVariable("")
			}
			return b.EmitBinOp(ssaop, op1, op2), nil
		}

		if op := exp.GetMul_op(); op != nil {
			var ssaop ssa.BinaryOpcode

			switch op.GetText() {
			case "*":
				ssaop = ssa.OpMul
			case "/":
				ssaop = ssa.OpDiv
			case "%":
				ssaop = ssa.OpMod
			case "&":
				ssaop = ssa.OpAnd
			case "<<":
				ssaop = ssa.OpShl
			case ">>":
				ssaop = ssa.OpShr
			case "&^":
				ssaop = ssa.OpAndNot
			default:
			}

			op1 := getValue(exp, 0)
			op2 := getValue(exp, 1)
			if op1 == nil {
				b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
				return b.EmitConstInst(0), b.CreateVariable("")
			}
			if op2 == nil {
				b.NewError(ssa.Error, TAG, AssignRightSideEmpty())
				return b.EmitConstInst(0), b.CreateVariable("")
			}
			return b.EmitBinOp(ssaop, op1, op2), nil
		}

		if op := exp.GetRel_op(); op != nil {
			var ssaop ssa.BinaryOpcode

			switch op.GetText() {
			case "==":
				ssaop = ssa.OpEq
			case "!=":
				ssaop = ssa.OpNotEq
			case "<":
				ssaop = ssa.OpLt
			case ">":
				ssaop = ssa.OpGt
			case "<=":
				ssaop = ssa.OpLtEq
			case ">=":
				ssaop = ssa.OpGtEq
			default:
			}

			op1 := getValue(exp, 0)
			op2 := getValue(exp, 1)
			if op1 == nil {
				b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
				return b.EmitConstInst(0), b.CreateVariable("")
			}
			if op2 == nil {
				b.NewError(ssa.Error, TAG, AssignRightSideEmpty())
				return b.EmitConstInst(0), b.CreateVariable("")
			}
			return b.EmitBinOp(ssaop, op1, op2), nil
		}
	} else { // left

	}

	return b.EmitConstInst(0), b.CreateVariable("")
}

func (b *astbuilder) buildPrimaryExpression(exp *gol.PrimaryExprContext, IslValue bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	if ret := exp.Operand(); ret != nil {
		return b.buildOperandExpression(ret.(*gol.OperandContext), IslValue)
	}
	if ret := exp.MethodExpr(); ret != nil {
		return b.buildMethodExpression(ret.(*gol.MethodExprContext), IslValue)
	}
	if ret := exp.Conversion(); ret != nil {
		return b.buildConversion(ret.(*gol.ConversionContext), IslValue)
	}

	var leftv *ssa.Variable = nil
	var rightv ssa.Value = nil

	if IslValue {
		rv, _ := b.buildPrimaryExpression(exp.PrimaryExpr().(*gol.PrimaryExprContext), false)

		if ret := exp.Index(); ret != nil {
			index := b.buildIndexExpression(ret.(*gol.IndexContext))
			leftv = b.CreateMemberCallVariable(rv, index)
		}

		if ret := exp.DOT(); ret != nil {
			id := exp.IDENTIFIER()
			test := id.GetText()
			leftv = b.CreateMemberCallVariable(rv, b.EmitConstInst(test))
		}
	} else {
		rv, _ := b.buildPrimaryExpression(exp.PrimaryExpr().(*gol.PrimaryExprContext), false)
		if ret := exp.Arguments(); ret != nil {
			args := b.buildArgumentsExpression(ret.(*gol.ArgumentsContext))
			rightv = b.EmitCall(b.NewCall(rv, args))
		}

		if ret := exp.Index(); ret != nil {
			index := b.buildIndexExpression(ret.(*gol.IndexContext))
			rightv = b.ReadMemberCallValue(rv, index)
		}

		if ret := exp.Slice_(); ret != nil {
			values := b.buildSliceExpression(ret.(*gol.Slice_Context))
			rightv = b.EmitMakeSlice(rv, values[0], values[1], values[2])
		}

		if ret := exp.DOT(); ret != nil {
			id := exp.IDENTIFIER()
			test := id.GetText()

			if a := exp.TypeArgs(); a != nil {
				_ = a
			}

			if value, ok := b.GetProgram().ReadImportValueWithPkg(rv.GetName(), test); ok {
				rightv = value
			} else {
				rightv = b.ReadMemberCallValue(rv, b.EmitConstInst(test))
			}
		}

		if ret := exp.TypeAssertion(); ret != nil {
			if t := ret.(*gol.TypeAssertionContext).Type_(); t != nil {
				ssatyp := b.buildType(t.(*gol.Type_Context))
				rv.SetType(ssatyp)
				rightv = rv
			}
		}
	}
	return rightv, leftv
}

func (b *astbuilder) buildMethodExpression(exp *gol.MethodExprContext, IslValue bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()
	var typ ssa.Type
	var text string

	if t := exp.Type_(); t != nil {
		typ = b.buildType(t.(*gol.Type_Context))
	}
	if id := exp.IDENTIFIER(); id != nil {
		text = id.GetText()
	}

	_ = typ
	_ = text

	// TODO
	b.NewError(ssa.Error, TAG, ToDo())
	return b.EmitConstInst(0), b.CreateVariable("")
}

func (b *astbuilder) buildConversion(exp *gol.ConversionContext, IslValue bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()
	var typ ssa.Type
	var rightv ssa.Value
	var leftv *ssa.Variable

	if t := exp.Type_(); t != nil {
		typ = b.buildType(t.(*gol.Type_Context))
	}
	if exp.Expression() != nil {
		rightv, leftv = b.buildExpression(exp.Expression().(*gol.ExpressionContext), IslValue)
	}

	values := []ssa.Value{rightv}
	switch typ.GetTypeKind() {
	case ssa.SliceTypeKind, ssa.BytesTypeKind:
		obj := b.InterfaceAddFieldBuild(len(values),
			func(i int) ssa.Value {
				return b.EmitConstInst(i)
			},
			func(i int) ssa.Value {
				return values[i]
			})
		coverType(obj.GetType(), typ)
		return obj, leftv
	}

	return rightv, leftv
}

func (b *astbuilder) buildSliceExpression(exp *gol.Slice_Context) [3]ssa.Value {
	var values [3]ssa.Value

	if low := exp.GetLow(); low != nil {
		rightv, _ := b.buildExpression(low.(*gol.ExpressionContext), false)
		values[0] = rightv
	}
	if high := exp.GetHigh(); high != nil {
		rightv, _ := b.buildExpression(high.(*gol.ExpressionContext), false)
		values[1] = rightv
	}
	if max := exp.GetMax(); max != nil {
		rightv, _ := b.buildExpression(max.(*gol.ExpressionContext), false)
		values[2] = rightv
	}

	return values
}

func (b *astbuilder) buildIndexExpression(arg *gol.IndexContext) ssa.Value {
	var rv ssa.Value
	if exp := arg.Expression(); exp != nil {
		rv, _ = b.buildExpression(exp.(*gol.ExpressionContext), false)
	}
	return rv
}

func (b *astbuilder) buildArgumentsExpression(arg *gol.ArgumentsContext) []ssa.Value {
	var args []ssa.Value

	if typ := arg.Type_(); typ != nil {
		ssatyp := b.buildType(typ.(*gol.Type_Context))
		args = append(args, b.EmitTypeValue(ssatyp))
	}

	if expl := arg.ExpressionList(); expl != nil {
		for _, exp := range expl.(*gol.ExpressionListContext).AllExpression() {
			rv, _ := b.buildExpression(exp.(*gol.ExpressionContext), false)
			args = append(args, rv)
		}
	}

	return args
}

func (b *astbuilder) buildExpressionStmt(stmt *gol.ExpressionStmtContext) []ssa.Value {
	var rightv ssa.Value
	if exp := stmt.Expression(); exp != nil {
		rightv, _ = b.buildExpression(exp.(*gol.ExpressionContext), false)
	}
	return []ssa.Value{rightv}
}

func (b *astbuilder) buildOperandExpression(exp *gol.OperandContext, IslValue bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()
	var rightv ssa.Value
	var leftv *ssa.Variable

	if !IslValue { // right
		if literal := exp.Literal(); literal != nil {
			rightv = b.buildLiteral(literal.(*gol.LiteralContext))
		}
		if id := exp.OperandName(); id != nil {
			if a := exp.TypeArgs(); a != nil {
				_ = a
			}
			rightv = b.buildOperandNameR(id.(*gol.OperandNameContext))
		}
		if e := exp.Expression(); e != nil {
			return b.buildExpression(e.(*gol.ExpressionContext), false)
		}
	} else { // left
		if id := exp.OperandName(); id != nil {
			leftv = b.buildOperandNameL(id.(*gol.OperandNameContext), false)
		}
	}
	return rightv, leftv
}

func (b *astbuilder) buildOperandNameL(name *gol.OperandNameContext, isLocal bool) *ssa.Variable {
	recoverRange := b.SetRange(name.BaseParserRuleContext)
	defer recoverRange()

	if id := name.IDENTIFIER(); id != nil {
		text := id.GetText()
		if text == "_" {
			b.NewError(ssa.Warn, TAG, "cannot use _ as value")
		}
		if b.GetFromCmap(text) {
			b.NewError(ssa.Warn, TAG, "cannot assign to const value")
		}
		/*if v, ok := b.GetGlobalVariableL(text); ok {
			return v
		}*/
		if isLocal {
			return b.CreateLocalVariable(text)
		} else {
			return b.CreateVariable(text)
		}
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.CreateVariable("")
}

func (b *astbuilder) buildOperandNameR(name *gol.OperandNameContext) ssa.Value {
	recoverRange := b.SetRange(name.BaseParserRuleContext)
	defer recoverRange()

	if id := name.IDENTIFIER(); id != nil {
		text := id.GetText()
		if text == "_" {
			b.NewError(ssa.Warn, TAG, "cannot use _ as value")
		}

		if c, ok := b.CheckSpecialValueByStr(text); ok {
			return b.EmitConstInst(c)
		}

		v := b.PeekValue(text)
		if v == nil {
			v = b.GetGlobalVariableR(text)
		}
		if v != nil {
			return v
		}

		if importp, path := b.GetImportPackage(text); len(path) != 0 {
			var obj ssa.Value

			if importp != nil { // user defined package
				var names []string
				var values []ssa.Value
				for n, g := range importp.ExportValue {
					names = append(names, n)
					values = append(values, g)
				}
				obj = b.InterfaceAddFieldBuild(len(importp.ExportValue),
					func(i int) ssa.Value {
						return b.EmitConstInst(names[i])
					},
					func(i int) ssa.Value {
						return values[i]
					})
			} else { // lib package
				obj = b.InterfaceAddFieldBuild(0,
					func(i int) ssa.Value {
						return nil
					},
					func(i int) ssa.Value {
						return nil
					})
			}

			obj.SetName(text)
			obj.GetType().SetFullTypeNames(path)
			b.AssignVariable(b.CreateLocalVariable(text), obj)
			return obj
		}

		v = b.GetFunc(text, "")
		if v.(*ssa.Function) == nil {
			b.NewError(ssa.Warn, TAG, fmt.Sprintf("not find variable %s in current scope", text))
			return b.ReadValue(text)
		}
		return v
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.EmitConstInst(0)
}
