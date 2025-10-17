package php2ssa

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitExpressionStatement(raw phpparser.IExpressionStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ExpressionStatementContext)
	if i == nil {
		return nil
	}
	va := y.VisitExpression(i.Expression())
	return va
}

func (y *builder) VisitParentheses(raw phpparser.IParenthesesContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ParenthesesContext)
	if i == nil {
		return nil
	}

	return y.VisitExpression(i.Expression())
}

func (y *builder) VisitExpression(raw phpparser.IExpressionContext) (v ssa.Value) {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	defer func() {
		if v == nil {
			log.Errorf("VisitExpression failed: %v", raw.GetText())
		}
	}()
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if raw.GetText() == "" {
		return y.EmitUndefined("")
	}

	switch ret := raw.(type) {
	case *phpparser.CloneExpressionContext:
		// 浅拷贝，一个对象
		// 如果类定义了 __clone，就执行 __clone
		target := y.VisitExpression(ret.Expression())
		val, ok := target.GetStringMember("__clone")
		if !ok {
			return target
		}
		return y.EmitCall(y.NewCall(val, []ssa.Value{}))
	case *phpparser.VariableExpressionContext:
		return y.VisitRightValue(ret.FlexiVariable())
	case *phpparser.ParentExpressionContext:
		text := ret.Parent_().GetText()
		parent := y.PeekValue(text)
		if parent == nil {
			parent = y.EmitConstInstPlaceholder(text)
		}
		currentBlueprint := y.Function.GetCurrentBlueprint()
		if currentBlueprint != nil {
			parentBlueprint := currentBlueprint.GetSuperBlueprint()
			if parentBlueprint != nil {
				parent.SetType(parentBlueprint)
				key := y.VisitMemberCallKey(ret.MemberCallKey())
				if y.isFunction {
					return y.ReadMemberCallMethod(parent, key)
				}
				return y.ReadMemberCallValue(parent, key)
			}
		}
		return y.EmitUndefined("parent")
	case *phpparser.MemberCallExpressionContext:
		obj := y.VisitExpression(ret.Expression())
		key := y.VisitMemberCallKey(ret.MemberCallKey())
		return y.ReadMemberCallValue(obj, key)
	case *phpparser.KeywordNewExpressionContext:
		return y.VisitNewExpr(ret.NewExpr())
	case *phpparser.FullyQualifiedNamespaceExpressionContext:
		return y.VisitFullyQualifiedNamespaceExpr(ret.FullyQualifiedNamespaceExpr(), false)
	case *phpparser.IndexCallExpressionContext: // $a[1]
		obj := y.VisitExpression(ret.Expression())
		key := y.VisitIndexMemberCallKey(ret.IndexMemberCallKey())
		if key == nil {
			return obj
		}
		return y.ReadMemberCallValue(obj, key)
	case *phpparser.IndexLegacyCallExpressionContext: // $a{1}
		obj := y.VisitExpression(ret.Expression())
		key := y.VisitIndexMemberCallKey(ret.IndexMemberCallKey())
		if key == nil {
			return obj
		}
		return y.ReadMemberCallValue(obj, key)
	case *phpparser.FunctionCallExpressionContext:
		tmp := y.isFunction
		y.isFunction = true
		defer func() {
			y.isFunction = tmp
		}()
		target := y.VisitExpression(ret.Expression())
		args, ellipsis := y.VisitArguments(ret.Arguments())
		callInst := y.NewCall(target, args)
		if ellipsis {
			callInst.IsEllipsis = true
		}
		return y.EmitCall(callInst)
	case *phpparser.CastExpressionContext:
		target := y.VisitExpression(ret.Expression())
		return y.EmitTypeCast(target, y.VisitCastOperation(ret.CastOperation()))
	case *phpparser.UnaryOperatorExpressionContext:
		/*
			| ('~' | '@') expression                                      # UnaryOperatorExpression
			| ('!' | '+' | '-') expression                                # UnaryOperatorExpression
		*/
		val := y.VisitExpression(ret.Expression())
		switch {
		case ret.Bang() != nil:
			return y.EmitUnOp(ssa.OpNot, val)
		case ret.Plus() != nil:
			return y.EmitUnOp(ssa.OpPlus, val)
		case ret.Minus() != nil:
			return y.EmitUnOp(ssa.OpNeg, val)
		case ret.Tilde() != nil:
			return y.EmitUnOp(ssa.OpBitwiseNot, val)
		case ret.SuppressWarnings() != nil:
			/*
				TODO:
				  var a;
				  try {
				    $a = exec expr;
				  }
			*/
			return val
		}
	case *phpparser.PrefixIncDecExpressionContext:
		// variable := y.variable
		// val := y.VisitExpression(ret.Expression())
		variable := y.VisitLeftVariable(ret.FlexiVariable())
		val := y.ReadValueByVariable(variable)
		if ret.Inc() != nil {
			after := y.EmitBinOp(ssa.OpAdd, val, y.EmitConstInst(1))
			y.AssignVariable(variable, after)
			// y.EmitUpdate(val, after)
			return after
		} else if ret.Dec() != nil {
			after := y.EmitBinOp(ssa.OpSub, val, y.EmitConstInst(1))
			y.AssignVariable(variable, after)
			return after
		}
		return y.EmitConstInstNil()
	case *phpparser.PostfixIncDecExpressionContext:
		variable := y.VisitLeftVariable(ret.FlexiVariable())
		val := y.ReadValueByVariable(variable)
		if ret.Inc() != nil {
			after := y.EmitBinOp(ssa.OpAdd, val, y.EmitConstInst(1))
			y.AssignVariable(variable, after)
			return val
		} else if ret.Dec() != nil {
			after := y.EmitBinOp(ssa.OpSub, val, y.EmitConstInst(1))
			y.AssignVariable(variable, after)
			return val
		}
		return y.EmitConstInstNil()
	case *phpparser.PrintExpressionContext:
		caller := y.ReadValue("print")
		args := y.VisitExpression(ret.Expression())
		callInst := y.NewCall(caller, []ssa.Value{args})
		return y.EmitCall(callInst)
	case *phpparser.ArrayCreationExpressionContext:
		creation, _ := y.VisitArrayCreation(ret.ArrayCreation())
		container := y.EmitEmptyContainer()
		for _, values := range creation {
			key, value := values[0], values[1]
			y.AssignVariable(y.CreateMemberCallVariable(container, key), value)
		}
		return container
	case *phpparser.ScalarExpressionContext: // constant / string / label / php literal
		if i := ret.Constant(); i != nil {
			return y.VisitConstant(i)
		} else if i := ret.String_(); i != nil {
			return y.VisitString_(i)
		} else if ret.Label() != nil {
			return y.EmitConstInst(i.GetText())
		} else {
			log.Warnf("PHP Scalar Expr Failed: %s", ret.GetText())
		}
	case *phpparser.BackQuoteStringExpressionContext:
		r := ret.GetText()
		if len(r) >= 2 {
			r = r[1 : len(r)-1]
		}
		return y.EmitConstInst(r)
	case *phpparser.ParenthesisExpressionContext:
		return y.VisitExpression(ret.Expression())
	case *phpparser.SpecialWordExpressionContext:
		if i := ret.Yield(); i != nil {
			return y.EmitConstInstNil()
		} else if i := ret.List(); i != nil {
		}
		return y.EmitConstInstNil()
	case *phpparser.IncludeExpressionContext:
		if ret.Expression() != nil {
			value := y.VisitExpression(ret.Expression())
			y.AddIncludePath(value.String())
			return y.EmitConstInst(true)
		}
		return y.VisitIncludeExpression(ret.Include())
	case *phpparser.LambdaFunctionExpressionContext:
		return y.VisitLambdaFunctionExpr(ret.LambdaFunctionExpr())
	case *phpparser.MatchExpressionContext:
	case *phpparser.ArithmeticExpressionContext:
		exprs := ret.AllExpression()
		if len(exprs) == 0 {
			log.Error("Arithmetic Expression need 2 ops")
		}
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		var o ssa.BinaryOpcode
		opStr := ret.GetOp().GetText()
		switch opStr {
		case "**":
			o = ssa.OpPow
		case "+":
			o = ssa.OpAdd
		case "-":
			o = ssa.OpSub
		case "*":
			o = ssa.OpMul
		case "/":
			o = ssa.OpDiv
		case "%":
			o = ssa.OpMod
		case ".":
			o = ssa.OpAdd
		default:
			log.Errorf("unexpected op: %v", opStr)
			return y.EmitUndefined("")
		}
		return y.EmitBinOp(o, op1, op2)
	case *phpparser.InstanceOfExpressionContext:
		// instanceof
		instace := y.ReadValue("instanceOf")
		expression := y.VisitExpression(ret.Expression(0))
		visitExpression := y.VisitExpression(ret.Expression(1))
		call := y.NewCall(instace, []ssa.Value{expression, visitExpression})
		return y.EmitCall(call)
	case *phpparser.ComparisonExpressionContext:
		switch ret.GetOp().GetText() {
		case "<<":
			return y.EmitBinOp(ssa.OpShl, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case ">>":
			return y.EmitBinOp(ssa.OpShr, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "<":
			return y.EmitBinOp(ssa.OpLt, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case ">":
			return y.EmitBinOp(ssa.OpGt, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "<=":
			return y.EmitBinOp(ssa.OpLtEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case ">=":
			return y.EmitBinOp(ssa.OpGtEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "==":
			return y.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "===":
			return y.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "!=":
			return y.EmitBinOp(ssa.OpNotEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "<>", "!==":
			return y.EmitBinOp(ssa.OpNotEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		default:
			log.Errorf("unhandled comparison expression: %v", ret.GetText())
		}
		return y.EmitConstInstNil()
	case *phpparser.BitwiseExpressionContext:
		switch ret.GetOp().GetText() {
		case "&&":
			id := ssa.AndExpressionVariable
			v1 := y.VisitExpression(ret.Expression(0))
			y.AssignVariable(y.CreateVariable(id), y.EmitValueOnlyDeclare(id))
			y.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.EmitBinOp(ssa.OpEq, v1, y.EmitConstInst(true))
			}, func() {
				v2 := y.VisitExpression(ret.Expression(1))
				y.AssignVariable(y.CreateVariable(id), y.EmitBinOp(ssa.OpEq, v2, y.EmitConstInst(true)))
			}).SetElse(func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitConstInst(false))
			}).Build()
			return y.ReadValue(id)
		case "||":
			id := ssa.OrExpressionVariable
			v1 := y.VisitExpression(ret.Expression(0))
			y.AssignVariable(y.CreateVariable(id), y.EmitValueOnlyDeclare(id))
			y.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.EmitBinOp(ssa.OpEq, v1, y.EmitConstInst(true))
			}, func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitConstInst(true))
			}).SetElse(func() {
				v2 := y.VisitExpression(ret.Expression(1))
				y.AssignVariable(y.CreateVariable(id), y.EmitBinOp(ssa.OpEq, v2, y.EmitConstInst(true)))
			}).Build()
			return y.ReadValue(id)
		case "|":
			return y.EmitBinOp(ssa.OpOr, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "^":
			return y.EmitBinOp(ssa.OpXor, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		case "&":
			return y.EmitBinOp(ssa.OpAnd, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		default:
			return y.EmitConstInstNil()
		}
	case *phpparser.ConditionalExpressionContext:
		/*
			<?php

			$a=$_GET[1] ?:"aa";
			$a = $_GET[1]? "1": "2";
		*/
		variableName := ssa.TernaryExpressionVariable
		variable := y.CreateVariable(variableName)
		y.AssignVariable(variable, y.EmitUndefined(variableName))
		y.CreateIfBuilder().SetCondition(func() ssa.Value {
			return y.VisitExpression(ret.Expression(0))
		}, func() {
			if len(ret.AllExpression()) == 2 {
				y.AssignVariable(y.CreateVariable(variableName), y.VisitExpression(ret.Expression(0)))
			} else {
				y.AssignVariable(y.CreateVariable(variableName), y.VisitExpression(ret.Expression(1)))
			}
		}).SetElse(func() {
			if len(ret.AllExpression()) == 2 {
				y.AssignVariable(y.CreateVariable(variableName), y.VisitExpression(ret.Expression(1)))
			} else {
				y.AssignVariable(y.CreateVariable(variableName), y.VisitExpression(ret.Expression(2)))
			}
		}).Build()
		return y.ReadValue(variableName)
	case *phpparser.NullCoalescingExpressionContext:
		name := uuid.NewString()
		variable := y.CreateVariable(name)
		y.AssignVariable(variable, y.VisitExpression(ret.Expression(0)))
		y.CreateIfBuilder().SetCondition(func() ssa.Value {
			return y.VisitExpression(ret.Expression(1))
		}, func() {
			y.AssignVariable(y.CreateVariable(name), y.VisitExpression(ret.Expression(1)))
		}).Build()
		return y.ReadValue(name)
	case *phpparser.DefinedOrScanDefinedExpressionContext:
		return y.VisitDefineExpr(ret.DefineExpr())
	case *phpparser.SpaceshipExpressionContext:
		var result ssa.Value
		y.CreateIfBuilder().SetCondition(func() ssa.Value {
			return y.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
		}, func() {
			result = y.EmitConstInst(0)
		}).SetElse(func() {
			y.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.EmitBinOp(ssa.OpLt, y.VisitExpression(ret.Expression(0)), y.VisitExpression(ret.Expression(1)))
			}, func() {
				result = y.EmitConstInst(-1)
			}).SetElse(func() {
				result = y.EmitConstInst(1)
			})
		})
		return result
	case *phpparser.ArrayCreationUnpackExpressionContext:
		// [$1, $2, $3] = $arr;
		// unpacking
		creation := y.VisitLeftArrayCreation(ret.LeftArrayCreation())
		expression := y.VisitExpression(ret.Expression())
		for _, variable := range creation {
			y.AssignVariable(variable, expression) //连接上数据流
		}
		return y.EmitConstInstNil()
	case *phpparser.OrdinaryAssignmentExpressionContext:
		variable := y.VisitLeftVariable(ret.FlexiVariable())
		rightValue := y.VisitExpression(ret.Expression())
		rightValue = y.reduceAssignCalcExpression(ret.AssignmentOperator().GetText(), variable, rightValue)
		y.AssignVariable(variable, rightValue)
		return rightValue

	case *phpparser.LogicalExpressionContext:
		id := uuid.NewString()
		y.AssignVariable(y.CreateVariable(id), y.EmitValueOnlyDeclare(id))
		if ret.LogicalXor() != nil {
			v1 := y.VisitExpression(ret.Expression(0))
			v2 := y.VisitExpression(ret.Expression(1))
			y.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.EmitBinOp(ssa.OpEq, v1, v2)
			}, func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitConstInst(true))
			}).SetElse(func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitConstInst(false))
			}).Build()
		}
		if ret.LogicalOr() != nil {
			value := y.VisitExpression(ret.Expression(0))
			y.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.EmitBinOp(ssa.OpEq, value, y.EmitConstInst(true))
			}, func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitConstInst(true))
			}).SetElse(func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(1)), y.EmitConstInst(true)))
			}).Build()
		}
		if ret.LogicalAnd() != nil {
			value := y.VisitExpression(ret.Expression(0))
			y.CreateIfBuilder().SetCondition(func() ssa.Value {
				return y.EmitBinOp(ssa.OpEq, value, y.EmitConstInst(true))
			}, func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitBinOp(ssa.OpEq, y.VisitExpression(ret.Expression(1)), y.EmitConstInst(true)))
			}).SetElse(func() {
				y.AssignVariable(y.CreateVariable(id), y.EmitConstInst(false))
			}).Build()
		}
		return y.ReadValue(id)

	case *phpparser.ShortQualifiedNameExpressionContext:
		// 因为涉及到函数，先peek如果没有读取到说明是一个常量 （define定义的常量会出现问题）
		var unquote string
		_unquote, err := yakunquote.Unquote(ret.Identifier().GetText())
		if err != nil {
			unquote = ret.Identifier().GetText()
		} else {
			unquote = _unquote
		}
		valName := y.VisitIdentifier(ret.Identifier())
		if !y.isFunction {
			readConst, ok := y.ReadConst(unquote)
			if ok {
				return readConst
			}
		}
		if value := y.PeekValue(valName); !utils.IsNil(value) {
			if function, b := ssa.ToFunction(value); b {
				function.Build()
			}
			if printType, b := ssa.ToClassBluePrintType(value.GetType()); b {
				printType.Build()
			}
			return value
		}
		if funcx, ok := y.GetFunc(valName, ""); ok {
			return funcx
		}
		if !y.isFunction {

			if s, ok := y.ReadConst(unquote); ok {
				return s
			}
		} else {
			if value := y.PeekValue(valName); !utils.IsNil(value) {
				if function, b := ssa.ToFunction(value); b {
					function.Build()
				}
				if printType, b := ssa.ToClassBluePrintType(value.GetType()); b {
					printType.Build()
				}
				return value
			}
			if funcx, ok := y.GetFunc(valName, ""); ok {
				return funcx
			}
		}
		undefined := y.EmitUndefined(valName)
		y.AssignVariable(y.CreateVariable(valName), undefined)
		return undefined

	case *phpparser.StaticClassAccessExpressionContext:
		if expr := y.VisitStaticClassExpr(ret.StaticClassExpr()); utils.IsNil(expr) {
			return y.EmitUndefined(ret.GetText())
		} else {
			return expr
		}

	case *phpparser.StaticClassMemberCallAssignmentExpressionContext:
		rightValue := y.VisitExpression(ret.Expression())
		if bluePrint, key := y.VisitStaticClassExprVariableMember(ret.StaticClassExprVariableMember()); bluePrint != nil {
			member := y.GetStaticMember(bluePrint, key)
			y.AssignVariable(member, rightValue)
			bluePrint.RegisterStaticMember(key, rightValue)
		}
		return rightValue
	}
	log.Errorf("-------------unhandled expression: %v(%T)", raw.GetText(), raw)
	log.Errorf("-------------unhandled expression: %v(%T)", raw.GetText(), raw)
	log.Errorf("-------------unhandled expression: %v(%T)", raw.GetText(), raw)
	log.Errorf("-------------unhandled expression: %v(%T)", raw.GetText(), raw)
	return y.EmitConstInstNil()
}

func (y *builder) VisitChainList(raw phpparser.IChainListContext) []ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ChainListContext)
	if i == nil {
		return nil
	}
	var tmpValue []ssa.Value
	for _, chain := range i.AllChain() {
		tmpValue = append(tmpValue, y.VisitChain(chain))
	}
	return tmpValue
}

func (y *builder) VisitChainLeft(raw phpparser.IChainContext) *ssa.Variable {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ChainContext)
	if i == nil {
		return nil
	}
	if i.FlexiVariable() != nil {
		return y.VisitLeftVariable(i.FlexiVariable())
	}
	member, s := y.VisitStaticClassExprVariableMember(i.StaticClassExprVariableMember())
	staticMember := y.GetStaticMember(member, s)
	return staticMember
}

func (y *builder) VisitChain(raw phpparser.IChainContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ChainContext)
	if i == nil {
		return nil
	}
	if i.FlexiVariable() != nil {
		return y.VisitRightValue(i.FlexiVariable())
	} else {
		member, key := y.VisitStaticClassExprVariableMember(i.StaticClassExprVariableMember())
		if member != nil {
			variable := y.GetStaticMember(member, key)
			if value := y.PeekValueByVariable(variable); !utils.IsNil(value) {
				return value
			}
			if staticMember := member.GetStaticMember(key); !utils.IsNil(staticMember) {
				return staticMember
			}
		}
		return y.EmitUndefined(key)
	}
}

func (y *builder) VisitMemberAccess(origin ssa.Value, raw phpparser.IMemberAccessContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.MemberAccessContext)
	if i == nil {
		return nil
	}

	fieldName := y.VisitKeyedFieldName(i.KeyedFieldName())
	origin = y.ReadOrCreateMemberCallVariable(origin, fieldName)
	if i.ActualArguments() != nil {
		y.VisitActualArguments(i.ActualArguments())
	}

	return origin
}

func (y *builder) VisitKeyedFieldName(raw phpparser.IKeyedFieldNameContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i := raw.(*phpparser.KeyedFieldNameContext)

	if i.KeyedSimpleFieldName() != nil {
		return y.VisitKeyedSimpleFieldName(i.KeyedSimpleFieldName())
	} else if i.KeyedVariable() != nil {
		return y.VisitKeyedVariable(i.KeyedVariable())
	}
	return y.EmitUndefined(raw.GetText())
}

func (y *builder) VisitKeyedVariable(raw phpparser.IKeyedVariableContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.KeyedVariableContext)
	if i == nil {
		return nil
	}

	dollarCount := len(i.AllDollar())
	var varMain ssa.Value
	if i.VarName() != nil {
		// ($*)$a
		//// {} as index [] as sliceCall
		variable := y.ReadOrCreateVariable(i.VarName().GetText()).GetLastVariable()
		if variable == nil {
			variable = y.CreateVariable(i.VarName().GetText())
		}
		varMain = variable.GetValue()
		if varMain == nil {
			varMain = y.EmitUndefined(i.VarName().GetText())
		}
		if dollarCount > 1 {
			for i := 0; i < dollarCount-1; i++ {
				// 处理变量的变量
			}
		}

	} else {
		// {} as name [] as sliceCall
		varMain = y.VisitExpression(i.Expression())
	}

	for _, a := range i.AllSquareCurlyExpression() {
		v := y.VisitSquareCurlyExpression(a)
		if v != nil {
			varMain = y.ReadOrCreateMemberCallVariable(varMain, v)
		}
	}

	return varMain
}

func (y *builder) VisitKeyedSimpleFieldName(raw phpparser.IKeyedSimpleFieldNameContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.KeyedSimpleFieldNameContext)
	if i == nil {
		return nil
	}

	var val ssa.Value
	if i.Identifier() != nil {
		val = y.EmitConstInstPlaceholder(y.VisitIdentifier(i.Identifier()))
	} else if i.Expression() != nil {
		val = y.VisitExpression(i.Expression())
	} else {
		val = y.EmitEmptyContainer()
	}

	for _, sce := range i.AllSquareCurlyExpression() {
		v := y.VisitSquareCurlyExpression(sce)
		if v != nil {
			val = y.ReadOrCreateMemberCallVariable(val, v)
		}
	}

	return val
}

func (y *builder) VisitSquareCurlyExpression(raw phpparser.ISquareCurlyExpressionContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.SquareCurlyExpressionContext)
	if i == nil {
		return nil
	}

	if i.OpenSquareBracket() != nil {
		if i.Expression() != nil {
			return y.VisitExpression(i.Expression())
		} else {
			/*
				$a = array("apple", "banana");
				$a[] = "cherry";

				// 现在，$a 包含 "apple", "banana", "cherry"
			*/
			// call len
			log.Errorf("UNIMPLEMTED SquareCurlyExpressionContext like a[] = ...: %v", raw.GetText())
			log.Errorf("UNIMPLEMTED SquareCurlyExpressionContext like a[] = ...: %v", raw.GetText())
			log.Errorf("UNIMPLEMTED SquareCurlyExpressionContext like a[] = ...: %v", raw.GetText())
			log.Errorf("UNIMPLEMTED SquareCurlyExpressionContext like a[] = ...: %v", raw.GetText())
			log.Error("PHP $a[...] call empty")
			return y.EmitUndefined("$var[]")
		}
	} else {
		return y.VisitExpression(i.Expression())
	}
}

func (y *builder) VisitFunctionCall(raw phpparser.IFunctionCallContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.FunctionCallContext)
	if i == nil {
		return nil
	}

	v := y.VisitFunctionCallName(i.FunctionCallName())
	var c *ssa.Call
	args, ellipsis := y.VisitActualArguments(i.ActualArguments())
	if _, exit := y.GetProgram().ExternInstance[strings.ToLower(v.String())]; exit {
		c = y.NewCall(y.EmitConstInstPlaceholder(strings.ToLower(v.String())), args)
	} else {
		c = y.NewCall(v, args)
	}
	c.IsEllipsis = ellipsis
	return y.EmitCall(c)
}

func (y *builder) VisitFunctionCallName(raw phpparser.IFunctionCallNameContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.FunctionCallNameContext)
	if i == nil {
		return nil
	}

	if ret := i.QualifiedNamespaceName(); ret != nil {
		name, s := y.VisitQualifiedNamespaceName(ret)
		_, _ = name, s
		//return y.ReadValue(text)
	} else if ret := i.ChainBase(); ret != nil {
		return y.VisitChainBase(ret)
	} else if ret := i.ClassConstant(); ret != nil {
		return y.VisitClassConstant(ret)
	} else if ret := i.Parentheses(); ret != nil {
		return y.VisitParentheses(ret)
	} else if ret := i.Label(); ret != nil {
		return y.ReadValue(i.Label().GetText())
	}
	log.Errorf("BUG: unknown function call name: %v", i.GetText())
	return nil
}

func (y *builder) VisitChainOrigin(raw phpparser.IChainOriginContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ChainOriginContext)
	if i == nil {
		return nil
	}

	if ret := i.NewExpr(); ret != nil {
		return y.VisitNewExpr(ret)
	} else if ret := i.FunctionCall(); ret != nil {
		return y.VisitFunctionCall(ret)
	} else if ret := i.ChainBase(); ret != nil {
		return y.VisitChainBase(ret)
	} else {
		log.Errorf("BUG: unknown chain origin: %v", i.GetText())
	}

	return y.EmitUndefined(i.GetText())
}

func (y *builder) VisitChainBase(raw phpparser.IChainBaseContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ChainBaseContext)
	if i == nil {
		return nil
	}
	if ret := i.QualifiedStaticTypeRef(); ret != nil {
		log.Error(`QualifiedStaticTypeRef unfinished`)
		log.Error(`QualifiedStaticTypeRef unfinished`)
		log.Error(`QualifiedStaticTypeRef unfinished`)
	} else {
		var ret ssa.Value
		for _, i := range i.AllKeyedVariable() {
			if ret == nil {
				ret = y.VisitKeyedVariable(i)
				continue
			}
			ret = y.CreateMemberCallVariable(ret, y.VisitKeyedVariable(i)).GetValue()
		}
		return ret
	}
	return nil
}

func (y *builder) VisitArrayCreation(raw phpparser.IArrayCreationContext) ([][2]ssa.Value, int) {
	if y == nil || raw == nil || y.IsStop() {
		return [][2]ssa.Value{}, 0
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ArrayCreationContext)
	if i == nil {
		return [][2]ssa.Value{}, 0
	}
	return y.VisitArrayItemList(i.ArrayItemList())
}

func (y *builder) VisitArrayItemList(raw phpparser.IArrayItemListContext) ([][2]ssa.Value, int) {
	if y == nil || raw == nil || y.IsStop() {
		return [][2]ssa.Value{}, 0
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ArrayItemListContext)
	if i == nil {
		return [][2]ssa.Value{}, 0
	}

	countIndex := 0
	var results [][2]ssa.Value
	for _, a := range i.AllArrayItem() {

		k, v := y.VisitArrayItem(a)
		if utils.IsNil(k) {
			k = y.EmitConstInstPlaceholder(countIndex)
			countIndex++
		}
		kv := [2]ssa.Value{k, v}
		results = append(results, kv)
	}
	return results, countIndex
}

func (y *builder) VisitArrayItem(raw phpparser.IArrayItemContext) (ssa.Value, ssa.Value) {
	if y == nil || raw == nil || y.IsStop() {
		return nil, nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.ArrayItemContext)
	if i == nil {
		return nil, nil
	}
	switch {
	case i.Chain() != nil:
		if i.Expression(0) != nil {
			expr := y.VisitExpression(i.Expression(0))
			chain := y.VisitChain(i.Chain())
			return expr, chain
		} else {
			return y.EmitConstInstNil(), y.VisitChain(i.Chain())
		}
	default:
		if len(i.AllExpression()) == 2 {
			return y.VisitExpression(i.Expression(0)), y.VisitExpression(i.Expression(1))
		} else {
			return y.EmitConstInstNil(), y.VisitExpression(i.Expression(0))
		}
	}
}

func (y *builder) VisitAttributes(raw phpparser.IAttributesContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.AttributesContext)
	if i == nil {
		return nil
	}

	for _, g := range i.AllAttributeGroup() {
		y.VisitAttributeGroup(g)
	}

	return nil
}

func (y *builder) VisitAttributeGroup(raw phpparser.IAttributeGroupContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.AttributeGroupContext)
	if i == nil {
		return nil
	}

	y.VisitIdentifier(i.Identifier())

	for _, a := range i.AllAttribute() {
		y.VisitAttribute(a)
	}

	return nil
}

func (y *builder) VisitAttribute(raw phpparser.IAttributeContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.AttributeContext)
	if i == nil {
		return nil
	}

	y.VisitQualifiedNamespaceName(i.QualifiedNamespaceName())
	if i.Arguments() != nil {
		y.VisitArguments(i.Arguments())
	}

	return nil
}

func (y *builder) VisitLeftArrayCreation(raw phpparser.ILeftArrayCreationContext) []*ssa.Variable {
	if y == nil || raw == nil || y.IsStop() {
		return []*ssa.Variable{}
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	arraycreation := raw.(*phpparser.LeftArrayCreationContext)
	return y.VisitArrayDestructuring(arraycreation.ArrayDestructuring())
}
func (y *builder) VisitArrayDestructuring(raw phpparser.IArrayDestructuringContext) []*ssa.Variable {
	if y == nil || raw == nil || y.IsStop() {
		return []*ssa.Variable{}
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	arrayDest, ok := raw.(*phpparser.ArrayDestructuringContext)
	if !ok {
		return []*ssa.Variable{}
	}
	var result = make([]*ssa.Variable, 0)
	for _, itemContext := range arrayDest.AllIndexedDestructItem() {
		item := y.VisitIndexDestructItem(itemContext)
		result = append(result, item)
	}
	return result
}

func (y *builder) VisitIndexDestructItem(raw phpparser.IIndexedDestructItemContext) *ssa.Variable {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	arrayDest, ok := raw.(*phpparser.IndexedDestructItemContext)
	if !ok {
		return nil
	}
	return y.VisitChainLeft(arrayDest.Chain())
}

type arrayKeyValuePair struct {
	Key   ssa.Value
	Value ssa.Value
}

func (y *builder) VisitConstantInitializer(raw phpparser.IConstantInitializerContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return y.EmitUndefined(raw.GetText())
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	switch ret := raw.(type) {
	case *phpparser.ConstantStringitializerContext:
		var initVal ssa.Value
		for _, c := range ret.AllConstantString() {
			if utils.IsNil(initVal) {
				initVal = y.VisitConstantString(c)
				continue
			}
			initVal = y.EmitBinOp(ssa.OpAdd, initVal, y.VisitConstantString(c))
		}
		return initVal
	case *phpparser.ArrayInitializerContext:
		if ret.ArrayItemList() != nil {
			results, l := y.VisitArrayItemList(ret.ArrayItemList())
			array := y.EmitMakeWithoutType(y.EmitConstInstPlaceholder(l), y.EmitConstInstPlaceholder(l))
			for _, values := range results {
				k, v := values[0], values[1]
				variable := y.CreateMemberCallVariable(array, k)
				y.AssignVariable(variable, v)
			}
			return array
		} else {
			array := y.EmitMakeWithoutType(y.EmitConstInstPlaceholder(0), y.EmitConstInstPlaceholder(0))
			return array
		}
	case *phpparser.ExpressionitializerContext:
		return y.VisitExpression(ret.Expression())
	case *phpparser.UnitializerContext:
		initializer := y.VisitConstantInitializer(ret.ConstantInitializer())
		if ret.Minus() != nil {
			return y.EmitUnOp(ssa.OpNeg, initializer)
		}
		return y.EmitUnOp(ssa.OpPlus, initializer)
	default:
		log.Errorf("emit undefined")
		return y.EmitUndefined(ret.GetText())
	}
}

func (y *builder) VisitExpressionList(raw phpparser.IExpressionListContext) []ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ExpressionListContext)
	if i == nil {
		return nil
	}
	value := make([]ssa.Value, 0, len(i.AllExpression()))
	for _, expressionContext := range i.AllExpression() {
		value = append(value, y.VisitExpression(expressionContext))
	}
	return value
}

func (y *builder) VisitConstantString(raw phpparser.IConstantStringContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ConstantStringContext)
	if i == nil {
		return nil
	}

	if r := i.String_(); r != nil {
		return y.VisitString_(r)
	}
	return y.VisitConstant(i.Constant())
}

func (y *builder) reduceAssignCalcExpression(operator string, variable *ssa.Variable, value ssa.Value) ssa.Value {
	if operator == "=" {
		return value
	}
	// y.GetReferenceFiles()
	return y.reduceAssignCalcExpressionEx(operator, y.ReadValueByVariable(variable), value)
}

func (y *builder) reduceAssignCalcExpressionEx(operator string, leftValues ssa.Value, rightValue ssa.Value) ssa.Value {
	switch operator {
	case "=":
		return rightValue
	case "+=":
		rightValue = y.EmitBinOp(ssa.OpAdd, leftValues, rightValue)
	case "-=":
		rightValue = y.EmitBinOp(ssa.OpSub, leftValues, rightValue)
	case "*=":
		rightValue = y.EmitBinOp(ssa.OpMul, leftValues, rightValue)
	case "**=":
		rightValue = y.EmitBinOp(ssa.OpPow, leftValues, rightValue)
		// rightValue = ssa.CalcConstBinary(y.c, rightValue, ssa.OpPow)
	case "/=":
		rightValue = y.EmitBinOp(ssa.OpDiv, leftValues, rightValue)
	case "%=":
		rightValue = y.EmitBinOp(ssa.OpMod, leftValues, rightValue)
	case ".=":
		rightValue = y.EmitBinOp(ssa.OpAdd, leftValues, rightValue)
		// rightValue = y.EmitBinOp(ssa.OpAdd, leftValues, rightValue)
	case "&=":
		rightValue = y.EmitBinOp(ssa.OpAnd, leftValues, rightValue)
	case "|=":
		rightValue = y.EmitBinOp(ssa.OpOr, leftValues, rightValue)
	case "^=":
		rightValue = y.EmitBinOp(ssa.OpXor, leftValues, rightValue)
	case "<<=":
		rightValue = y.EmitBinOp(ssa.OpShl, leftValues, rightValue)
	case ">>=":
		rightValue = y.EmitBinOp(ssa.OpShr, leftValues, rightValue)
	case "??=":
		if leftValues == nil || leftValues.IsUndefined() {
			return rightValue
		} else {
			return leftValues
		}
	default:
		log.Errorf("unhandled assignment operator: %v", operator)
	}
	return rightValue
}

// VisitLeftVariable
func (y *builder) VisitLeftVariable(raw phpparser.IFlexiVariableContext) *ssa.Variable {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	switch i := raw.(type) {
	case *phpparser.CustomVariableContext:
		variable := y.VisitVariable(i.Variable())
		return y.CreateVariable(variable)
	case *phpparser.IndexVariableContext:
		value := y.VisitRightValue(i.FlexiVariable())
		if key := y.VisitIndexMemberCallKey(i.IndexMemberCallKey()); key != nil {
			return y.CreateMemberCallVariable(value, key) //这里有问题?
		} else {
			return y.VisitLeftVariable(i.FlexiVariable())
		}
	case *phpparser.IndexLegacyCallVariableContext:
		obj := y.VisitRightValue(i.FlexiVariable())
		if key := y.VisitIndexMemberCallKey(i.IndexMemberCallKey()); key != nil {
			return y.VisitLeftVariable(i.FlexiVariable())
		} else {
			return y.CreateMemberCallVariable(obj, key)
		}
	case *phpparser.MemberVariableContext:
		value := y.VisitRightValue(i.FlexiVariable())
		key := y.VisitMemberCallKey(i.MemberCallKey())
		member := y.CreateMemberCallVariable(value, key)
		return member
	default:
		return nil
	}
}

// flexivariable读右值
func (y *builder) VisitRightValue(raw phpparser.IFlexiVariableContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	switch i := raw.(type) {
	case *phpparser.CustomVariableContext:
		variable := y.VisitVariable(i.Variable())
		var (
			position     = ""
			force_create = ""
		)

		handler := func() ssa.Value {
			app := y.GetProgram().GetApplication()
			if g, ok := app.GetGlobalVariable(position); ok {
				return g
			}
			return nil
		}

		switch variable {
		case "$GLOBALS":
			force_create = "GLOBALS"
		case "$_GET":
			force_create = "_GET"
		case "$_POST":
			force_create = "_POST"
		case "$_REQUEST":
			force_create = "_REQUEST"
		case "$_SERVER":
			position = "_SERVER"
		case "$_COOKIE":
			force_create = "_COOKIE"
		case "$_ENV":
			force_create = "_ENV"
		case "$_SESSION":
			force_create = "_SESSION"
		case "$_FILES":
			force_create = "_FILES"
		}
		if position != "" {
			return handler()
		} else if force_create != "" {
			createVariable := y.CreateVariable(force_create)
			val := y.EmitUndefined(force_create)
			y.AssignVariable(createVariable, val)
			return val
		}
		return y.ReadValue(variable)
	case *phpparser.IndexVariableContext:
		obj := y.VisitRightValue(i.FlexiVariable())
		key := y.VisitIndexMemberCallKey(i.IndexMemberCallKey())
		return y.ReadMemberCallValue(obj, key)
	case *phpparser.IndexLegacyCallVariableContext:
		obj := y.VisitRightValue(i.FlexiVariable())
		key := y.VisitIndexMemberCallKey(i.IndexMemberCallKey())
		return y.ReadMemberCallValue(obj, key)
	case *phpparser.MemberVariableContext:
		obj := y.VisitRightValue(i.FlexiVariable())
		key := y.VisitMemberCallKey(i.MemberCallKey())
		return y.ReadMemberCallValue(obj, key)
	case *phpparser.MemberFunctionContext:
		obj := y.VisitRightValue(i.FlexiVariable())
		key := y.VisitMemberCallKey(i.MemberCallKey())
		method := y.ReadMemberCallMethod(obj, key)
		arguments, _ := y.VisitArguments(i.Arguments())
		call := y.NewCall(method, arguments)
		return y.EmitCall(call)
	default:
		return y.EmitUndefined(raw.GetText())
	}
}

func (y *builder) VisitVariable(raw phpparser.IVariableContext) string {
	if y == nil || raw == nil || y.IsStop() {
		return ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	switch ret := raw.(type) {
	case *phpparser.NormalVariableContext:
		return ret.VarName().GetText()

	case *phpparser.DynamicVariableContext:
		id := ret.VarName().GetText()
		var value ssa.Value
		for i := range ret.AllDollar() {
			_ = i
			value = y.ReadValue(id)
			if value.IsUndefined() {
				var varName = fmt.Sprintf("dollar%v", y.fetchDollarId())
				variable := y.CreateVariable(varName)
				y.AssignVariable(variable, value)
				return varName
			}
			id = "$" + value.String()
		}
		return "$" + value.String()
	case *phpparser.MemberCallVariableContext:
		value := y.VisitExpression(ret.Expression())
		// TODO: handler this
		return value.String()

	default:
		raw.GetText()
		log.Errorf("unhandled expression: %v(T: %T)", raw.GetText(), raw)
		log.Errorf("-------------unhandled expression: %v(%T)", raw.GetText(), raw)
		return ""
	}
}

func (y *builder) VisitIncludeExpression(raw phpparser.IIncludeContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, ok := raw.(*phpparser.IncludeContext)
	if !ok {
		return nil
	}
	var once bool
	var flag = y.EmitEmptyContainer()
	if i.IncludeOnce() != nil || i.RequireOnce() != nil {
		once = true
	}
	expr := i.Expression()
	value := y.VisitExpression(expr)
	call := y.NewCall(y.ReadValue("include"), []ssa.Value{value})
	y.EmitCall(call)
	if utils.IsNil(value) {
		log.Errorf("_________________BUG___EXPR IS NIL: %v________________", expr.GetText())
		log.Errorf("_________________BUG___EXPR IS NIL: %v________________", expr.GetText())
		log.Errorf("_________________BUG___EXPR IS NIL: %v________________", expr.GetText())
		log.Errorf("_________________BUG___EXPR IS NIL: %v________________", expr.GetText())
		return flag
	}
	if value.IsUndefined() {
		log.Warnf("include statement expression is undefined")
	} else {
		//todo： __dir__ 等魔术方法的转换
		file := value.String()
		application := y.GetProgram().Application
		includeStack := application.CurrentIncludingStack
		includeStack.Push(file)
		defer includeStack.Pop()
		if err := y.BuildFilePackage(file, once); err != nil {
			//todo: 目前拿不到include的返回值
			//flag = ssa.NewConst(false)
		} else {
			//flag = ssa.NewConst(true)
		}
	}
	return flag
}

func (y *builder) VisitDefineExpr(raw phpparser.IDefineExprContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, ok := raw.(*phpparser.DefineExprContext)
	if !ok {
		return nil
	}
	var flag bool
	if i.Defined() != nil {
		undefined := y.EmitUndefined(i.Defined().GetText())
		visitConstantString := y.VisitConstantString(i.ConstantString())
		call := y.NewCall(undefined, []ssa.Value{visitConstantString})
		emitCall := y.EmitCall(call)
		return emitCall
	}
	if i.Define() != nil {
		value := y.VisitExpression(i.Expression())
		constantString := y.VisitConstantString(i.ConstantString())
		if y.AssignConst(constantString.String(), value) {
			flag = true
		}
	}
	return y.EmitConstInstPlaceholder(flag)
}
