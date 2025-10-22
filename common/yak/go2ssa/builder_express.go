package go2ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"

	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type getSingleExpr interface {
	Expression(i int) gol.IExpressionContext
}

func (b *astbuilder) buildExpression(exp *gol.ExpressionContext, islValue bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	getValue := func(single getSingleExpr, i int) ssa.Value {
		if s := single.Expression(i); s != nil {
			rightv, _ := b.buildExpression(s.(*gol.ExpressionContext), false)
			return rightv
		} else {
			b.NewError(ssa.Error, TAG, "can't get expression")
			return b.EmitConstInstPlaceholder(0)
		}
	}
	getVariable := func(single getSingleExpr, i int) *ssa.Variable {
		if s := single.Expression(i); s != nil {
			_, leftv := b.buildExpression(s.(*gol.ExpressionContext), true)
			return leftv
		} else {
			b.NewError(ssa.Error, TAG, "can't get expression")
			return b.CreateVariable("")
		}
	}

	// fmt.Printf("exp = %v\n", exp.GetText())

	if ret := exp.PrimaryExpr(); ret != nil {
		return b.buildPrimaryExpression(ret.(*gol.PrimaryExprContext), islValue)
	}

	if !islValue { // right
		if op := exp.GetUnary_op(); op != nil {
			var ssaop ssa.UnaryOpcode
			op1 := getValue(exp, 0)
			if op1 == nil {
				b.NewError(ssa.Error, TAG, NeedTwoExpression())
				return b.EmitConstInstPlaceholder(0), b.CreateVariable("")
			}

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
				if op1.GetType().GetTypeKind() == ssa.PointerKind {
					return b.GetOriginValue(op1), nil
				}
			case "&":
				if op1Var := getVariable(exp, 0); op1Var != nil {
					return b.EmitConstPointer(op1Var), nil
				}
			default:
				b.NewError(ssa.Error, TAG, UnaryOperatorNotSupport(op.GetText()))
			}

			if op1 == nil {
				b.NewError(ssa.Error, TAG, NeedTwoExpression())
				return b.EmitConstInstPlaceholder(0), b.CreateVariable("")
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
				return b.EmitConstInstPlaceholder(0), b.CreateVariable("")
			}
			if op2 == nil {
				b.NewError(ssa.Error, TAG, AssignRightSideEmpty())
				return b.EmitConstInstPlaceholder(0), b.CreateVariable("")
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
				return b.EmitConstInstPlaceholder(0), b.CreateVariable("")
			}
			if op2 == nil {
				b.NewError(ssa.Error, TAG, AssignRightSideEmpty())
				return b.EmitConstInstPlaceholder(0), b.CreateVariable("")
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
				return b.EmitConstInstPlaceholder(0), b.CreateVariable("")
			}
			if op2 == nil {
				b.NewError(ssa.Error, TAG, AssignRightSideEmpty())
				return b.EmitConstInstPlaceholder(0), b.CreateVariable("")
			}
			return b.EmitBinOp(ssaop, op1, op2), nil
		}
	} else { // left
		if op := exp.GetUnary_op(); op != nil {
			op1 := getValue(exp, 0)
			if op1 == nil {
				b.NewError(ssa.Error, TAG, NeedTwoExpression())
				return b.EmitConstInstPlaceholder(0), b.CreateVariable("")
			}
			switch op.GetText() {
			case "*":
				if op1.GetType().GetTypeKind() == ssa.PointerKind {
					return nil, b.GetAndCreateOriginPointer(op1)
				} else if p, ok := ssa.ToParameter(op1); ok && !p.IsFreeValue {
					if op1Var := getVariable(exp, 0); op1Var != nil {
						b.ReferenceParameter(op1Var.GetName(), p.FormalParameterIndex, ssa.PointerSideEffect)
						// b.AssignVariable(op1Var, op1)
						return nil, op1Var
					}
				}
			}
		}
	}

	return b.EmitConstInstPlaceholder(0), b.CreateVariable("")
}

func (b *astbuilder) buildPrimaryExpression(exp *gol.PrimaryExprContext, IslValue bool, isFunction ...bool) (ssa.Value, *ssa.Variable) {
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
	var handleObjectType func(ssa.Value, *ssa.ObjectType)

	if IslValue {
		rv, _ := b.buildPrimaryExpression(exp.PrimaryExpr().(*gol.PrimaryExprContext), false)

		if ret := exp.Index(); ret != nil {
			index := b.buildIndexExpression(ret.(*gol.IndexContext))
			leftv = b.CreateMemberCallVariable(rv, index)
		}

		if ret := exp.DOT(); ret != nil {
			id := exp.IDENTIFIER()
			text := id.GetText()

			handleObjectType = func(rv ssa.Value, typ *ssa.ObjectType) {
				if typ.GetTypeKind() == ssa.PointerKind {
					rv = b.ReadMemberCallValue(rv, b.EmitConstInstPlaceholder("@value"))
					if typ, ok := ssa.ToObjectType(rv.GetType()); ok {
						handleObjectType(rv, typ)
					}
					return
				} else if p, ok := ssa.ToParameter(rv); ok && !p.IsFreeValue {
					if key := typ.GetKeybyName(text); key != nil {
						leftv = b.CreateMemberCallVariable(rv, key)
						b.ReferenceParameter(leftv.GetName(), p.FormalParameterIndex, ssa.PointerSideEffect)
						// TODO: 匿名结构体指针使用其他逻辑实现，需要兼容
						return
					}
				}

				if key := typ.GetKeybyName(text); key != nil {
					leftv = b.CreateMemberCallVariable(rv, key)
				} else {
					for n, a := range typ.AnonymousField {
						rv = b.ReadMemberCallValueByName(rv, n)
						if rv == nil {
							b.NewError(ssa.Error, TAG, NotFindAnonymousFieldObject(n))
							return
						}
						if key := a.GetKeybyName(text); key != nil {
							handleObjectType(rv, a)
						}
					}
				}
			}

			if typ, ok := ssa.ToObjectType(rv.GetType()); ok {
				handleObjectType(rv, typ)
			}

			if leftv == nil {
				leftv = b.CreateMemberCallVariable(rv, b.EmitConstInstPlaceholder(text))
			}
		}
	} else {
		if ret := exp.Arguments(); ret != nil {
			rv, _ := b.buildPrimaryExpression(exp.PrimaryExpr().(*gol.PrimaryExprContext), false, true)
			args := b.buildArgumentsExpression(ret.(*gol.ArgumentsContext))
			if rv.GetName() == "make" {
				rightv = b.InterfaceAddFieldBuild(0, func(i int) ssa.Value {
					return b.EmitConstInst(0)
				}, func(i int) ssa.Value {
					return b.EmitConstInst(0)
				})
				if len(args) > 0 {
					rightv.SetType(args[0].GetType())
				}
				return rightv, nil
			}
			rightv = b.EmitCall(b.NewCall(rv, args))
			return rightv, leftv
		}

		rv, _ := b.buildPrimaryExpression(exp.PrimaryExpr().(*gol.PrimaryExprContext), false)
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
			text := id.GetText()

			if a := exp.TypeArgs(); a != nil {
				_ = a
			}

			readMemberCall := func(rv, key ssa.Value) (ssa.Value, bool) {
				if len(isFunction) > 0 && isFunction[0] {
					return b.ReadMemberCallMethod(rv, key), false
				}
				return b.ReadMemberCallValue(rv, key), true
			}

			handleObjectType = func(rv ssa.Value, typ *ssa.ObjectType) {
				if typ.GetTypeKind() == ssa.PointerKind {
					rv = b.ReadMemberCallValue(rv, b.EmitConstInstPlaceholder("@value"))
					if typ, ok := ssa.ToObjectType(rv.GetType()); ok {
						handleObjectType(rv, typ)
					}
					return
				}

				if key := typ.GetKeybyName(text); key != nil {
					rightv = b.ReadMemberCallValue(rv, key)
				} else {
					for n, a := range typ.AnonymousField {
						/*
						 a.A.b
						*/
						if key := a.GetKeybyName(text); !utils.IsNil(key) {
							rightv = b.ReadMemberCallValueByName(rv, n)
							if rightv == nil {
								rightv, _ = readMemberCall(rv, b.EmitConstInstPlaceholder(text))
							}
						}
						handleObjectType(rightv, a)
					}
				}
			}

			if typ, ok := ssa.ToObjectType(rv.GetType()); ok {
				handleObjectType(rv, typ)
			} else if value, ok := b.GetProgram().ReadImportValueWithPkg(rv.GetName(), text); ok {
				rightv = value
			}

			if rightv == nil {
				var ok bool
				if rightv, ok = readMemberCall(rv, b.EmitConstInstPlaceholder(text)); ok {
					rightv.SetType(HandleFullTypeNames(rv.GetType(), rv.GetType().GetFullTypeNames()))
				} else {
					rightv.SetType(HandleFullTypeNames(rightv.GetType(), rv.GetType().GetFullTypeNames()))
				}
			}
			// log.Infof("rightv = %v", rightv)
			// log.Infof("rightv type = %v", rightv.GetType())
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
	return b.EmitConstInstPlaceholder(0), b.CreateVariable("")
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
				return b.EmitConstInstPlaceholder(i)
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
		if literal := exp.Literal(); literal != nil {
			rightv = b.buildLiteral(literal.(*gol.LiteralContext))
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
			return b.EmitConstInstPlaceholder(c)
		}

		if v := b.PeekValueInRoot(text); !utils.IsNil(v) {
			if ex, ok := ssa.ToExternLib(v); ok {
				return ex
			}
		}

		if v := b.PeekValue(text); !utils.IsNil(v) {
			return v
		}

		if g := b.GetGlobalVariableR(text); !utils.IsNil(g) {
			return g
		}

		if f, ok := b.GetFunc(text, ""); ok {
			return f
		}

		b.NewError(ssa.Warn, TAG, fmt.Sprintf("not find variable %s in current scope", text))
		return b.ReadValue(text)
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.EmitConstInst(0)
}
