//go:build !no_language
// +build !no_language

package js2ssa

import (
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// entry point
func (b *astbuilder) build(ast *JS.ProgramContext) {
	if s, ok := ast.Statements().(*JS.StatementsContext); ok {
		b.buildStatements(s)
	}
}

// statement list
func (b *astbuilder) buildStatements(stmtlist *JS.StatementsContext) {
	recoverRange := b.SetRange(stmtlist.BaseParserRuleContext)
	defer recoverRange()
	allstmt := stmtlist.AllStatement()
	if len(allstmt) == 0 {
		b.NewError(ssa.Warn, TAG, "empty statement list")
	} else {
		for _, stmt := range allstmt {
			if stmt, ok := stmt.(*JS.StatementContext); ok {
				b.buildStatement(stmt)
			}
		}
	}
}

func (b *astbuilder) buildStatement(stmt *JS.StatementContext) {
	if b.IsBlockFinish() {
		return
	}
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	b.AppendBlockRange()
	// var
	if s, ok := stmt.VariableStatement().(*JS.VariableStatementContext); ok {
		b.buildVariableStatement(s)
	}

	// expr
	if s, ok := stmt.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
		b.buildExpressionSequence(s)
	}

	// if
	if s, ok := stmt.IfStatement().(*JS.IfStatementContext); ok {
		b.buildIfStatementContext(s)
	}

	// block
	if s, ok := stmt.Block().(*JS.BlockContext); ok {
		b.buildBlock(s)
	}

	// do while
	if s, ok := stmt.IterationStatement().(*JS.DoStatementContext); ok {
		b.buildDoStatement(s)
	}

	// for
	if s, ok := stmt.IterationStatement().(*JS.ForStatementContext); ok {
		b.buildForStatement(s)
	}

	// while
	if s, ok := stmt.IterationStatement().(*JS.WhileStatementContext); ok {
		b.buildwhileStatement(s)
	}

	// forIn
	if s, ok := stmt.IterationStatement().(*JS.ForInStatementContext); ok {
		b.buildForInStatement(s)
	}

	// forOf
	if s, ok := stmt.IterationStatement().(*JS.ForOfStatementContext); ok {
		b.buildForOfStatement(s)
	}

	// function
	if s, ok := stmt.FunctionDeclaration().(*JS.FunctionDeclarationContext); ok {
		b.buildFunctionDeclaration(s)
	}

	// ret
	if s, ok := stmt.ReturnStatement().(*JS.ReturnStatementContext); ok {
		b.buildReturnStatement(s)
	}

	// break
	if s, ok := stmt.BreakStatement().(*JS.BreakStatementContext); ok {
		b.buildBreakStatement(s)
	}

	// label
	if s, ok := stmt.LabelledStatement().(*JS.LabelledStatementContext); ok {
		b.buildLabelledStatement(s)
	}

	// try
	if s, ok := stmt.TryStatement().(*JS.TryStatementContext); ok {
		b.buildTryStatement(s)
	}

	// switch
	if s, ok := stmt.SwitchStatement().(*JS.SwitchStatementContext); ok {
		b.buildSwitchStatement(s)
	}

}

func (b *astbuilder) buildVariableStatement(stmt *JS.VariableStatementContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.VariableDeclarationList().(*JS.VariableDeclarationListContext); ok {
		b.buildAllVariableDeclaration(s, false)
		return
	}

}

func (b *astbuilder) buildAllVariableDeclaration(stmt *JS.VariableDeclarationListContext, left bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	// var ret []ssa.Value

	// checking varModifier - decorator (var / let / const)
	// think `var a = 1`, `let a = 1`, `const a = 1`;

	var variable *ssa.Variable
	var value ssa.Value
	declare := ""
	if mI := stmt.GetModifier(); mI != nil {
		text := mI.GetText()
		declare = string(text[0])
		// if mI.GetText() {
		// 	declare = "c"
		// } else if m.Var() != nil {
		// 	// 定义域特殊，允许重赋值，宽松的很
		// 	declare = "v"
		// } else if m.Let_() != nil {
		// 	// 脑子正常的定义域处理，不允许重复定义
		// 	declare = "l"
		// } else {
		// 	// strict mode ?
		// 	b.NewError(ssa.Error, TAG, "wrong declare varmodifier")
		// 	return nil, nil
		// }
		for _, iVariableDeclarationStmt := range stmt.AllVariableDeclaration() {
			variableDeclarationStmt, ok := iVariableDeclarationStmt.(*JS.VariableDeclarationContext)
			if !ok {
				continue
			}
			value, variable = b.buildVariableDeclaration(variableDeclarationStmt, declare, left)
		}
		// fmt.Println(ret)
		return value, variable
	}
	return nil, nil
}

func (b *astbuilder) buildVariableDeclaration(stmt *JS.VariableDeclarationContext, Type string, left bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	a := stmt.Assign()
	varText := stmt.Assignable().GetText()

	if a == nil && !left {
		if Type == "c" {
			v := b.GetFromCmap(varText)
			if v {
				b.NewError(ssa.Error, TAG, "the const have been declared in the block")
			} else {
				b.NewError(ssa.Error, TAG, "const must have value")
			}
			return nil, nil
		} else if Type == "l" {
			v := b.GetFromLmap(varText)
			if v {
				b.NewError(ssa.Error, TAG, "the let have been declared in the block")
				return nil, nil
			} else {
				b.AddToLmap(varText)
			}
		}

		// 返回一个any
		return ssa.NewAny(), nil
	} else {
		assignValue := func() (ssa.Value, *ssa.Variable) {
			var variable *ssa.Variable

			// 得到一个左值
			if as, ok := stmt.Assignable().(*JS.AssignableContext); ok {
				if i := as.Identifier(); i != nil {
					text := i.GetText()
					_, lv := b.buildIdentifierExpression(text, true, true)
					variable = lv
				}
			}

			if left {
				return nil, variable
			}

			x := stmt.SingleExpression()
			result, _ := b.buildSingleExpression(x, false)

			b.AssignVariable(variable, result)
			return b.ReadValueByVariable(variable), variable
		}

		if Type == "c" {
			v := b.GetFromCmap(varText)
			if v {
				b.NewError(ssa.Error, TAG, "the const have been declared in the block")
				return nil, nil
			} else {
				rv, lv := assignValue()
				b.AddToCmap(varText)
				return rv, lv
			}
		} else if Type == "l" {
			v := b.GetFromLmap(varText)
			if v {
				b.NewError(ssa.Error, TAG, "the let have been declared in the block")
				return nil, nil
			} else {
				rv, lv := assignValue()
				b.AddToLmap(varText)
				return rv, lv
			}
		} else {
			return assignValue()
		}
	}
}

func (b *astbuilder) buildAssignableContext(stmt *JS.AssignableContext) *ssa.Variable {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if i := stmt.Identifier(); i != nil {
		text := i.GetText()
		_, lv := b.buildIdentifierExpression(text, true, false)
		return lv
	}

	return nil
}

type getSingleExpr interface {
	SingleExpression(i int) JS.ISingleExpressionContext
}

func (b *astbuilder) buildSingleExpression(stmt JS.ISingleExpressionContext, IslValue bool) (ssa.Value, *ssa.Variable) {
	// TODO: singleExpressions unfinish

	if v := b.buildOnlyRightSingleExpression(stmt); v != nil {
		return v, nil
	} else {
		if IslValue {
			_, lValue := b.buildSingleExpressionEx(stmt, IslValue)
			return nil, lValue
		} else {
			rValue, _ := b.buildSingleExpressionEx(stmt, IslValue)
			return rValue, nil
		}
	}
}

func (b *astbuilder) buildOnlyRightSingleExpression(stmt JS.ISingleExpressionContext) ssa.Value {

	// fmt.Println("build single expression: ", stmt.GetText())

	getValue := func(single getSingleExpr, i int) ssa.Value {
		if s := single.SingleExpression(i); s != nil {
			v, _ := b.buildSingleExpression(s, false)
			return v
		} else {
			return nil
		}
	}
	getBinaryOp := func() (single getSingleExpr, Op ssa.BinaryOpcode, IsBinOp bool) {
		for {
			// a := stmt
			// fmt.Println(a.GetText())

			// +/-
			if s, ok := stmt.(*JS.AdditiveExpressionContext); ok {
				if op := s.Plus(); op != nil {
					single, Op, IsBinOp = s, ssa.OpAdd, true
				} else if op := s.Minus(); op != nil {
					single, Op, IsBinOp = s, ssa.OpSub, true
				} else {
					break
				}
			}

			// TODO: js ==and===need to handle
			// ('==' | '!=' | '===' | '!==')
			if s, ok := stmt.(*JS.EqExpressionContext); ok {
				if op := s.Equals_(); op != nil {
					single, Op, IsBinOp = s, ssa.OpEq, true
				} else if op := s.NotEquals(); op != nil {
					single, Op, IsBinOp = s, ssa.OpNotEq, true
				} else if op := s.IdentityEquals(); op != nil {
					single, Op, IsBinOp = s, ssa.OpEq, true
				} else if op := s.IdentityNotEquals(); op != nil {
					single, Op, IsBinOp = s, ssa.OpNotEq, true
				} else {
					break
				}
			}

			// ('<' | '>' | '<=' | '>=')
			if s, ok := stmt.(*JS.RelationalExpressionContext); ok {
				if op := s.LessThan(); op != nil {
					single, Op, IsBinOp = s, ssa.OpLt, true
				} else if op := s.MoreThan(); op != nil {
					single, Op, IsBinOp = s, ssa.OpGt, true
				} else if op := s.LessThanEquals(); op != nil {
					single, Op, IsBinOp = s, ssa.OpLtEq, true
				} else if op := s.GreaterThanEquals(); op != nil {
					single, Op, IsBinOp = s, ssa.OpGtEq, true
				} else if op := s.In(); op != nil {
					single, Op, IsBinOp = s, ssa.OpIn, true
				} else if op := s.Instanceof(); op != nil {
					single, Op, IsBinOp = s, ssa.OpIn, true
				}
				break
			}

			// ('<<' | '>>' | '>>>') 缺>>>
			if s, ok := stmt.(*JS.BitShiftExpressionContext); ok {
				if op := s.LeftShiftArithmetic(); op != nil {
					single, Op, IsBinOp = s, ssa.OpShl, true
				} else if op := s.RightShiftArithmetic(); op != nil {
					single, Op, IsBinOp = s, ssa.OpShr, true
				}
			}

			// ('*' | '/' | '%')
			if s, ok := stmt.(*JS.MultiplicativeExpressionContext); ok {
				if op := s.Multiply(); op != nil {
					single, Op, IsBinOp = s, ssa.OpMul, true
				} else if op := s.Divide(); op != nil {
					single, Op, IsBinOp = s, ssa.OpDiv, true
				} else if op := s.Modulus(); op != nil {
					single, Op, IsBinOp = s, ssa.OpMod, true
				} else {
					break
				}
			}

			// '^' '&' '|'
			if s, ok := stmt.(*JS.BitExpressionContext); ok {
				if op := s.BitXOr(); op != nil {
					single, Op, IsBinOp = s, ssa.OpXor, true
				} else if op := s.BitAnd(); op != nil {
					single, Op, IsBinOp = s, ssa.OpAnd, true
				} else if op := s.BitOr(); op != nil {
					single, Op, IsBinOp = s, ssa.OpOr, true
				} else {
					break
				}
			}

			return
		}

		b.NewError(ssa.Error, TAG, "binary operator not support: %s", stmt.GetText())
		return
	}

	getUnaryOp := func() (single *JS.PreUnaryExpressionContext, Op ssa.UnaryOpcode, IsUnaryOp bool) {
		for {
			// + - ! ~
			if s, ok := stmt.(*JS.PreUnaryExpressionContext); ok {
				Un := s.PreUnaryOperator().GetText()

				flag := 0

				switch Un {
				case "+":
					single, Op, IsUnaryOp = s, ssa.OpPlus, true
				case "-":
					single, Op, IsUnaryOp = s, ssa.OpNeg, true
				case "~":
					single, Op, IsUnaryOp = s, ssa.OpBitwiseNot, true
				case "!":
					single, Op, IsUnaryOp = s, ssa.OpNot, true
				case "++":
					break
				case "--":
					break
				case "delete":
					break
				case "void":
					break
				case "typeof":
					break
				default:
					flag = 1
				}

				if flag == 1 {
					break
				}
			}
			return
		}
		b.NewError(ssa.Error, TAG, "unary operator not support: %s", stmt.GetText())
		return
	}

	// advanced expression
	// 处理的时候需要知道哪些是高级逻辑：
	// 高级逻辑需要处理成类似 “分支” 的行为，一般都会牵扯类似“短路”特性；
	// 也不是说长得像二元运算就一定是二元运算
	// 例如：false && dump() 这个表达式，dump()是不会执行的，因为false && dump()的结果一定是false
	handlePrimaryBinaryOperation := func() ssa.Value {
		// 数学运算
		single, opcode, IsBinOp := getBinaryOp()
		if IsBinOp {
			op1 := getValue(single, 0)
			op2 := getValue(single, 1)
			if op1 == nil || op2 == nil {
				b.NewError(ssa.Error, TAG, "in operator need two expression")
				return nil
			}
			return b.EmitBinOp(opcode, op1, op2)
		}

		// fallback is right?
		b.NewError(ssa.Error, TAG, "error binary operator")
		return nil
	}

	handlePrimaryUnaryOperation := func() ssa.Value {
		// 比特运算
		single, opcode, IsUnOp := getUnaryOp()
		if IsUnOp {
			op, _ := b.buildSingleExpression(single.SingleExpression(), false)
			if op == nil {
				b.NewError(ssa.Error, TAG, "in operator need expression")
				return nil
			}
			return b.EmitUnOp(opcode, op)
		}

		b.NewError(ssa.Error, TAG, "error unary operator")
		return nil
	}

	//advanced expression
	handlerAdvancedExpression := func(cond func(string) ssa.Value, trueExpr, falseExpr func() ssa.Value) ssa.Value {
		// 逻辑运算聚合产生phi指令
		id := uuid.NewString()
		variable := b.CreateVariable(id)
		b.AssignVariable(variable, b.EmitValueOnlyDeclare(id))
		ifb := b.CreateIfBuilder()
		ifb.AppendItem(
			func() ssa.Value {
				return cond(id)
			},

			func() {
				v := trueExpr()
				variable := b.CreateVariable(id)
				b.AssignVariable(variable, v)
			},
		)

		if falseExpr != nil {
			ifb.SetElse(func() {
				v := falseExpr()
				variable := b.CreateVariable(id)
				b.AssignVariable(variable, v)
			})
		}

		ifb.Build()
		return b.ReadValue(id)
	}

	switch s := stmt.(type) {
	case *JS.KeywordExpressionContext:
		if expr := s.KeywordSingleExpression(); expr != nil {
			return b.buildKeywordSingleExpression(expr)
		}
	case *JS.FunctionExpressionContext:
		return b.buildFunctionExpression(s)
	case *JS.ClassExpressionContext:
	case *JS.OptionalChainExpressionContext:
		// advanced
		// let c = a?.b
		// roughly means: c = a ? a.b : undefined
		// roughly means: let c = undefined; if (a) {c = a.b }
	case *JS.MemberIndexExpressionContext:
	case *JS.ArgumentsExpressionContext:
		// function call
		return b.EmitCall(b.buildArgumentsExpression(s))
	case *JS.PostUnaryExpressionContext:
		// TODO: error 后返回nil会不会报错
		if expr := s.SingleExpression(); expr != nil {
			_, lValue := b.buildSingleExpression(expr, true)
			if v := lValue.GetValue(); v == nil {
				b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
				return nil
			} else {
				var rValue ssa.Value
				if s.GetOp().GetText() == "--" {
					rValue = b.EmitBinOp(ssa.OpSub, lValue.GetValue(), b.EmitConstInst(1))
				} else if s.GetOp().GetText() == "++" {
					rValue = b.EmitBinOp(ssa.OpAdd, lValue.GetValue(), b.EmitConstInst(1))
				}
				b.AssignVariable(lValue, rValue)
				return lValue.GetValue()
			}
		}
	case *JS.PreUnaryExpressionContext:
		if Unop, ok := s.PreUnaryOperator().(*JS.PreUnaryOperatorContext); ok {
			if Unop.GetText() == "typeof" {
				if expr := s.SingleExpression(); expr != nil {
					rv, _ := b.buildSingleExpression(expr, false)
					return b.EmitTypeValue(rv.GetType())
				}
			} else if Unop.GetText() == "delete" {
				// TODO:删除元素列表？
				if expr := s.SingleExpression(); expr != nil {
					rv, _ := b.buildSingleExpression(expr, false)
					return rv
				}
			} else if Unop.GetText() == "void" {
				if expr := s.SingleExpression(); expr != nil {
					rv, _ := b.buildSingleExpression(expr, false)
					return b.EmitUndefined(rv.String())
				}
			} else if Unop.GetText() == "++" || Unop.GetText() == "--" {
				if expr := s.SingleExpression(); expr != nil {
					_, variable := b.buildSingleExpression(expr, true)
					if v := b.ReadValueByVariable(variable); v != nil {
						b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
						return nil
					} else {
						var value ssa.Value
						if Unop.GetText() == "--" {
							value = b.EmitBinOp(ssa.OpSub, b.ReadValueByVariable(variable), b.EmitConstInst(1))
						} else if Unop.GetText() == "++" {
							value = b.EmitBinOp(ssa.OpAdd, b.ReadValueByVariable(variable), b.EmitConstInst(1))
						}
						b.AssignVariable(variable, value)
						return b.ReadValueByVariable(variable)
					}
				}
			}
		}
		return handlePrimaryUnaryOperation()
	case *JS.BitExpressionContext:
		return handlePrimaryUnaryOperation()
	case *JS.PowerExpressionContext:
		return handlePrimaryBinaryOperation()
	case *JS.MultiplicativeExpressionContext:
		return handlePrimaryBinaryOperation()
	case *JS.AdditiveExpressionContext:
		return handlePrimaryBinaryOperation()
	case *JS.CoalesceExpressionContext:
		// advanced
		if expr := s.SingleExpression(0); expr != nil {
			rv, _ := b.buildSingleExpression(expr, false)
			if rv != nil {
				return rv
			} else {
				v, _ := b.buildSingleExpression(expr, false)
				return v
			}
		}
	case *JS.BitShiftExpressionContext:
		return handlePrimaryBinaryOperation()
	case *JS.RelationalExpressionContext:
		return handlePrimaryBinaryOperation()
	case *JS.EqExpressionContext:
		return handlePrimaryBinaryOperation()
	case *JS.LogicalAndExpressionContext:
		// advanced
		return handlerAdvancedExpression(
			func(id string) ssa.Value {
				v := getValue(s, 0)
				variable := b.CreateVariable(id)
				b.AssignVariable(variable, v)
				return v
			},
			func() ssa.Value {
				v := getValue(s, 1)
				return v
			},
			nil,
		)
	case *JS.LogicalOrExpressionContext:
		// advanced
		return handlerAdvancedExpression(
			func(id string) ssa.Value {
				v := getValue(s, 0)
				variable := b.CreateVariable(id)
				b.AssignVariable(variable, v)
				return b.EmitUnOp(ssa.OpNot, v)
			},
			func() ssa.Value {
				return getValue(s, 1)
			},
			nil,
		)
	case *JS.TernaryExpressionContext:
		// advanced
		return handlerAdvancedExpression(
			func(_ string) ssa.Value {
				return getValue(s, 0)
			},
			func() ssa.Value {
				return getValue(s, 1)
			},
			func() ssa.Value {
				return getValue(s, 2)
			},
		)
	case *JS.AssignmentOperatorExpressionContext:
		_, lValue := b.buildSingleExpression(s.SingleExpression(0), true)
		rValue, _ := b.buildSingleExpression(s.SingleExpression(1), false)

		if lValue == nil || rValue == nil {
			b.NewError(ssa.Error, TAG, "in operator need two expression")
			return nil
		}

		if f, ok := s.AssignmentOperator().(*JS.AssignmentOperatorContext); ok {
			return b.buildAssignmentOperatorContext(f, lValue, rValue)
		}
	case *JS.TemplateStringExpressionContext:
	case *JS.YieldExpressionContext:
	case *JS.ThisExpressionContext:
		return ssa.NewParam("this", false, b.FunctionBuilder)
	case *JS.IdentifierExpressionContext:
	// identify是左值那边的
	// 	rv, _ :=  b.buildIdentifierExpression(s.GetText(), false)
	// 	return rv
	case *JS.SuperExpressionContext:
	case *JS.LiteralExpressionContext:
		return b.buildLiteralExpression(s)
	case *JS.ArrayLiteralExpressionContext:
		if expr, ok := s.ArrayLiteral().(*JS.ArrayLiteralContext); ok {
			return b.buildArrayLiteral(expr)
		}
	case *JS.ObjectLiteralExpressionContext:
		if expr, ok := s.ObjectLiteral().(*JS.ObjectLiteralContext); ok {
			return b.buildObjectLiteral(expr)
		}
	case *JS.ParenthesizedExpressionContext:
		if expr, ok := s.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
			exprs := b.buildExpressionSequence(expr)
			return exprs
		}
	case *JS.ChainExpressionContext:
	default:
		log.Warnf("not support expression: [%s] %s", stmt.GetText(), stmt)
		return nil
	}
	// log.Warnf("unfinished expression")
	return nil
}

func (b *astbuilder) buildSingleExpressionEx(stmt JS.ISingleExpressionContext, IslValue bool) (ssa.Value, *ssa.Variable) {
	//可能是左值也可能是右值的

	//标识符
	if s, ok := stmt.(*JS.IdentifierExpressionContext); ok {
		i := s.GetText()
		value, lValue := b.buildIdentifierExpression(i, IslValue, false)
		return value, lValue
	}

	if s, ok := stmt.(*JS.MemberIndexExpressionContext); ok {
		value, lValue := b.buildMemberIndexExpression(s, IslValue)
		return value, lValue
	}

	if s, ok := stmt.(*JS.ChainExpressionContext); ok {
		value, lValue := b.buildChainExpression(s, IslValue)
		return value, lValue
	}

	if s, ok := stmt.(*JS.OptionalChainExpressionContext); ok {
		value, lValue := b.buildOptionalChainExpression(s, IslValue)
		return value, lValue
	}

	b.NewError(ssa.Error, TAG, "error singleExpression")
	return b.EmitConstInst("error"), b.CreateVariable("error")
}

func (b *astbuilder) buildKeywordSingleExpression(stmt JS.IKeywordSingleExpressionContext) ssa.Value {
	if s, ok := stmt.(*JS.NewExpressionContext); ok {
		return b.EmitCall(b.buildArgumentsExpression(s))
	}

	if s, ok := stmt.(*JS.NewExpressionWithoutArgumentsExpressionContext); ok {
		args := make([]ssa.Value, 0)
		rv, _ := b.buildSingleExpression(s.SingleExpression(), false)
		return b.EmitCall(b.NewCall(rv, args))
	}

	if s, ok := stmt.(*JS.ImportExpressionContext); ok {
		rv, _ := b.buildSingleExpression(s.SingleExpression(), false)
		return rv
	}

	if s, ok := stmt.(*JS.AwaitExpressionContext); ok {
		rv, _ := b.buildSingleExpression(s.SingleExpression(), false)
		return rv
	}

	if s, ok := stmt.(*JS.NewExpressionContext); ok {
		rv, _ := b.buildSingleExpression(s.SingleExpression(), false)
		return rv
	}

	return nil
}

func (b *astbuilder) buildOptionalChainExpression(stmt *JS.OptionalChainExpressionContext, left bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var expr ssa.Value

	if s := stmt.SingleExpression(); s != nil {
		expr, _ = b.buildSingleExpression(s, false)
	} else {
		b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
		return nil, nil
	}

	var index ssa.Value
	if s, ok := stmt.OptionalChainMember().(*JS.OptionalChainMemberContext); ok {
		if expr, ok := s.IdentifierName().(*JS.IdentifierNameContext); ok {
			index = b.EmitConstInst(expr.GetText())
		} else if expr := s.SingleExpression(); expr != nil {
			//TODO:handle[singleexpr]
			index = b.EmitConstInst(expr.GetText())
		}
	}

	if left {
		return nil, b.CreateMemberCallVariable(expr, index)
	} else {
		return b.ReadMemberCallValue(expr, index), nil
	}
}

func (b *astbuilder) buildFunctionExpression(stmt *JS.FunctionExpressionContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.AnonymousFunction().(*JS.ArrowFunctionContext); ok {
		funcName := ""
		if a, ok := s.ArrowFunctionParameters().(*JS.ArrowFunctionParametersContext); ok {
			if Name := a.Identifier(); Name != nil {
				funcName = Name.GetText()
			}
		}

		newFunc := b.NewFunc(funcName)
		{
			recoverRange := b.SetRange(s.BaseParserRuleContext)

			b.FunctionBuilder = b.PushFunction(newFunc)

			if p, ok := s.ArrowFunctionParameters().(*JS.ArrowFunctionParametersContext); ok {
				if f, ok := p.FormalParameterList().(*JS.FormalParameterListContext); ok {
					b.buildFormalParameterList(f)
				}
			}

			if f, ok := s.ArrowFunctionBody().(*JS.ArrowFunctionBodyContext); ok {
				if fb, ok := f.FunctionBody().(*JS.FunctionBodyContext); ok {
					b.buildFunctionBody(fb)
				} else if s := f.SingleExpression(); s != nil {
					rv, _ := b.buildSingleExpression(s, false)
					var values []ssa.Value
					values = append(values, rv)
					b.EmitReturn(values)
				}
			}

			b.Finish()
			b.FunctionBuilder = b.PopFunction()

			recoverRange()
		}

		if funcName != "" {
			variable := b.CreateVariable(funcName)
			b.AssignVariable(variable, newFunc)
		}

		return newFunc
	} else {
		if s, ok := stmt.AnonymousFunction().(*JS.AnonymousFunctionDeclContext); ok {
			funcName := ""
			if name := s.Identifier(); name != nil {
				funcName = s.Identifier().GetText()
			}
			newFunc := b.NewFunc(funcName)
			{
				b.FunctionBuilder = b.PushFunction(newFunc)

				if f, ok := s.FormalParameterList().(*JS.FormalParameterListContext); ok {
					b.buildFormalParameterList(f)
				}

				if f, ok := s.FunctionBody().(*JS.FunctionBodyContext); ok {
					b.buildFunctionBody(f)
				}

				b.Finish()
				b.FunctionBuilder = b.PopFunction()

			}

			if funcName != "" {
				variable := b.CreateVariable(funcName)
				b.AssignVariable(variable, newFunc)
			}

			return newFunc
		}
	}

	return nil
}

type funcCall interface {
	SingleExpression() JS.ISingleExpressionContext
	Arguments() JS.IArgumentsContext
}

func (b *astbuilder) buildArgumentsExpression(stmt funcCall) *ssa.Call {
	Iscall := false
	var args []ssa.Value
	isEllipsis := false

	if s := stmt.SingleExpression(); s != nil {
		rv, _ := b.buildSingleExpression(s, false)
		if rv != nil {
			if s, ok := stmt.Arguments().(*JS.ArgumentsContext); ok {
				args, isEllipsis = b.buildArguments(s)
			}
			Iscall = true
		}
		if Iscall {
			c := b.NewCall(rv, args)
			if isEllipsis {
				c.IsEllipsis = true
			}

			return c
		}
	}
	b.NewError(ssa.Error, TAG, "call target is nil")
	return nil
}

func (b *astbuilder) buildArguments(stmt *JS.ArgumentsContext) ([]ssa.Value, bool) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	hasEll := false
	var v []ssa.Value

	for _, i := range stmt.AllArgument() {
		if a, ok := i.(*JS.ArgumentContext); ok {
			if a.Ellipsis() != nil {
				hasEll = true
			}

			if s := a.SingleExpression(); s != nil {
				rv, _ := b.buildSingleExpression(s, false)
				v = append(v, rv)
			} else if s := a.Identifier(); s != nil {
				text := a.Identifier().GetText()
				rv, _ := b.buildIdentifierExpression(text, false, false)
				v = append(v, rv)
			}
		}
	}
	return v, hasEll
}

func (b *astbuilder) buildAssignmentOperatorContext(stmt *JS.AssignmentOperatorContext, variable *ssa.Variable, rValue ssa.Value) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var Op ssa.BinaryOpcode
	if op := stmt.Assign(); op != nil {
		b.AssignVariable(variable, rValue)
		return b.ReadValueByVariable(variable)
	} else if op := stmt.PlusAssign(); op != nil {
		Op = ssa.OpAdd // +=
	} else if op := stmt.MinusAssign(); op != nil {
		Op = ssa.OpSub // -=
	} else if op := stmt.DivideAssign(); op != nil {
		Op = ssa.OpDiv // /=
	} else if op := stmt.ModulusAssign(); op != nil {
		Op = ssa.OpMod // %=
	} else if op := stmt.MultiplyAssign(); op != nil {
		Op = ssa.OpMul // *=
	} else if op := stmt.LeftShiftArithmeticAssign(); op != nil {
		Op = ssa.OpShl // <<=
	} else if op := stmt.RightShiftArithmeticAssign(); op != nil {
		Op = ssa.OpShr // >>=
	} else if op := stmt.BitOrAssign(); op != nil {
		Op = ssa.OpOr // |=
	} else if op := stmt.BitXorAssign(); op != nil {
		Op = ssa.OpXor // ^=
	} else if op := stmt.BitAndAssign(); op != nil {
		Op = ssa.OpAnd // &=
	} else if op := stmt.RightShiftLogicalAssign(); op != nil {
		// TODO:logical
		Op = ssa.OpShr // >>>=
	} else if op := stmt.PowerAssign(); op != nil {
		// TODO:**=
		Op = ssa.OpMul
	}

	value := b.EmitBinOp(Op, b.ReadValueByVariable(variable), rValue)
	// fmt.Println("value :", rValue)
	b.AssignVariable(variable, value)

	// fmt.Println("test assignOpreator: ", lValue.GetValue(b.FunctionBuilder))
	return b.ReadValueByVariable(variable)
}

func (b *astbuilder) buildIdentifierExpression(text string, IslValue bool, forceAssign bool) (ssa.Value, *ssa.Variable) {

	if IslValue {
		if b.GetFromCmap(text) {
			b.NewError(ssa.Error, TAG, "const cannot be assigned")
			return nil, nil
		}

		if forceAssign {
			return nil, b.CreateLocalVariable(text)
		}
		return nil, b.CreateVariable(text)
	}
	value := b.ReadValue(text)
	return value, nil
}

func (b *astbuilder) buildMemberIndexExpression(stmt *JS.MemberIndexExpressionContext, IsValue bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	// fmt.Println("memberIndex: ", stmt.GetText())

	var expr ssa.Value

	if IsValue {
		if s := stmt.SingleExpression(0); s != nil {
			expr, _ = b.buildSingleExpression(s, false)
		} else {
			b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
			return nil, nil
		}

		// left
		var index ssa.Value
		if s := stmt.SingleExpression(1); s != nil {
			index, _ = b.buildSingleExpression(s, false)
		}

		variable := b.CreateMemberCallVariable(expr, index)
		return nil, variable
	}

	if s := stmt.SingleExpression(0); s != nil {
		expr, _ = b.buildSingleExpression(s, false)
	}

	var value ssa.Value
	if s := stmt.SingleExpression(1); s != nil {
		value, _ = b.buildSingleExpression(s, false)
	}
	return b.ReadMemberCallValue(expr, value), nil
}

func (b *astbuilder) buildChainExpression(stmt *JS.ChainExpressionContext, IsValue bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var expr ssa.Value

	if s := stmt.SingleExpression(); s != nil {
		expr, _ = b.buildSingleExpression(s, false)
	} else {
		b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
		return nil, nil
	}

	var index ssa.Value
	if s, ok := stmt.IdentifierName().(*JS.IdentifierNameContext); ok {
		index = b.EmitConstInst(s.GetText())
	}

	if IsValue {
		variable := b.CreateMemberCallVariable(expr, index)
		return nil, variable
	}
	return b.ReadMemberCallValue(expr, index), nil
}

func (b *astbuilder) buildArrayLiteral(stmt *JS.ArrayLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var value []ssa.Value

	if s, ok := stmt.ElementList().(*JS.ElementListContext); ok {
		for _, iIface := range s.AllArrayElement() {
			i := iIface.(*JS.ArrayElementContext)
			if e := i.Ellipsis(); e != nil {
				b.HandlerEllipsis()
			}
			if s := i.SingleExpression(); s != nil {
				rv, _ := b.buildSingleExpression(s, false)
				value = append(value, rv)
			}
		}
	}

	return b.CreateObjectWithSlice(value)
}

func (b *astbuilder) buildObjectLiteral(stmt *JS.ObjectLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	// TODO: ObjectLiteral propertyAssignment remain 2

	var value []ssa.Value
	var keys []ssa.Value
	hasKey := false
	for i, p := range stmt.AllPropertyAssignment() {
		var rv ssa.Value
		var key ssa.Value

		if pro, ok := p.(*JS.PropertyExpressionAssignmentContext); ok {
			if i == 0 {
				hasKey = true
			}

			if !hasKey {
				b.NewError(ssa.Error, TAG, `Uncaught SyntaxError: Unexpected token ':'`)
				return nil
			}

			if s, ok := pro.PropertyName().(*JS.PropertyNameContext); ok {
				key = b.EmitConstInst(s.GetText())
			}

			if s := pro.SingleExpression(); s != nil {
				rv, _ = b.buildSingleExpression(s, false)
			}

		} else if pro, ok := p.(*JS.ComputedPropertyExpressionAssignmentContext); ok {
			if i == 0 {
				hasKey = true
			}

			if !hasKey {
				b.NewError(ssa.Error, TAG, `Uncaught SyntaxError: Unexpected token ':'`)
				return nil
			}

			if s := pro.SingleExpression(0); s != nil {
				key = b.EmitConstInst(s.GetText())
			}
			if s := pro.SingleExpression(1); s != nil {
				rv, _ = b.buildSingleExpression(s, false)
			}
		} else if pro, ok := p.(*JS.FunctionPropertyContext); ok {
			if hasKey {
				b.NewError(ssa.Error, TAG, `Uncaught SyntaxError: Unexpected token ':'`)
				return nil
			}

			var funcName string
			if s, ok := pro.PropertyName().(*JS.PropertyNameContext); ok {
				funcName = s.GetText()
			}

			newFunc := b.NewFunc(funcName)

			// buildFunc := func() {
			{
				recoverRange := b.SetRange(pro.BaseParserRuleContext)

				b.FunctionBuilder = b.PushFunction(newFunc)

				if s, ok := pro.FormalParameterList().(*JS.FormalParameterListContext); ok {
					b.buildFormalParameterList(s)
				}

				if f, ok := pro.FunctionBody().(*JS.FunctionBodyContext); ok {
					b.buildFunctionBody(f)
				}

				b.Finish()
				b.FunctionBuilder = b.PopFunction()

				recoverRange()
			}

			if funcName != "" {
				variable := b.CreateVariable(funcName)
				b.AssignVariable(variable, newFunc)
			}
			return newFunc

		} else if pro, ok := p.(*JS.PropertyGetterContext); ok {
			_ = pro
			// fmt.Println(pro)
		} else if pro, ok := p.(*JS.PropertySetterContext); ok {
			_ = pro
			// fmt.Println(pro)
		} else if pro, ok := p.(*JS.PropertyShorthandContext); ok {
			if hasKey {
				b.NewError(ssa.Error, TAG, `Uncaught SyntaxError: Unexpected token ':'`)
				return nil
			}

			if s := pro.SingleExpression(); s != nil {
				rv, _ = b.buildSingleExpression(s, false)
			}

			if pro.Ellipsis() != nil {
				b.HandlerEllipsis()
			}
		} else {
			b.NewError(ssa.Error, TAG, "Not propertyAssignment")
		}

		value = append(value, rv)
		if hasKey {
			keys = append(keys, key)
		}
	}

	if len(keys) == 0 {
		return b.CreateObjectWithSlice(value)
	}

	return b.CreateObjectWithMap(keys, value)
}

func (b *astbuilder) buildPropertyName(stmt *JS.PropertyNameContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.IdentifierName().(*JS.IdentifierNameContext); ok {
		return b.buildIdentifierName(s)
	} else if s := stmt.StringLiteral(); s != nil {
		return b.buildStringLiteral(s)
	} else if s, ok := stmt.NumericLiteral().(*JS.NumericLiteralContext); ok {
		return b.buildNumericLiteral(s)
	} else if s := stmt.SingleExpression(); s != nil {
		rv, _ := b.buildSingleExpression(s, false)
		return rv
	} else {
		b.NewError(ssa.Error, TAG, "Not support the propertyName")
	}

	return nil
}

func (b *astbuilder) buildIdentifierName(stmt *JS.IdentifierNameContext) ssa.Value {
	if s, ok := stmt.Identifier().(*JS.IdentifierContext); ok {
		text := s.GetText()
		_, lv := b.buildIdentifierExpression(text, true, false)
		return b.ReadValueByVariable(lv)
	} else if v := stmt.NullLiteral(); v != nil {
		return b.buildNullLiteral()
	} else if v := stmt.BooleanLiteral(); v != nil {
		return b.buildBooleanLiteral(stmt.GetText())
	} else if v := stmt.Word(); v != nil {
		return b.EmitConstInst(stmt.GetText())
	} else {
		b.NewError(ssa.Error, TAG, "not support the format")
	}
	return nil
}

// NOTE:
//
//	simgleExpr -> rightValue, leftValue
//	seqExprs (expr1, expr2, expr3) -> expr3RightValue, expr3Left
//
//	expr -> rValue, lValue
func (b *astbuilder) buildExpressionSequence(stmt *JS.ExpressionSequenceContext) ssa.Value {
	// 需要修改改函数及引用，不存在if中存在多个singleExpression的情况
	// compelte

	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var val ssa.Value

	// // -> singleExpression  (',' expressionSequence)*
	//val, _ = b.buildSingleExpression(stmt.SingleExpression().(*JS.SingleExpressionContext), false)
	//results := stmt.AllExpressionSequence()
	//if len(results) == 0 {
	//	return val
	//}
	//
	//for _, subSeq := range stmt.AllExpressionSequence() {
	//	if s := subSeq; s != nil {
	//		val = b.buildExpressionSequence(subSeq.(*JS.ExpressionSequenceContext))
	//		// values = append(values, rv)
	//	}
	//}

	for _, s := range stmt.AllSingleExpression() {
		val, _ = b.buildSingleExpression(s, false)
	}
	return val
}

func (b *astbuilder) buildIfStatementContext(stmt *JS.IfStatementContext) {
	// var buildIf func(stmt *JS.IfStatementContext) *ssa.IfBuilder
	buildIf := func(stmt *JS.IfStatementContext) *ssa.IfBuilder {
		recoverRange := b.SetRange(stmt.BaseParserRuleContext)
		defer recoverRange()

		i := b.CreateIfBuilder()

		// if instruction condition
		i.AppendItem(
			func() ssa.Value {
				if s, ok := stmt.ExpressionSequence(0).(*JS.ExpressionSequenceContext); ok {
					value := b.buildExpressionSequence(s)
					return value
				}
				return nil
			},
			func() {
				if s, ok := stmt.Statement(0).(*JS.StatementContext); ok {
					b.buildStatement(s)
				}
			},
		)

		// else if
		for index := range stmt.AllElse() {
			condstmt, ok := stmt.ExpressionSequence(index + 1).(*JS.ExpressionSequenceContext)
			if !ok {
				continue
			}

			i.AppendItem(
				// condition
				func() ssa.Value {
					rv := b.buildExpressionSequence(condstmt)
					return rv
				},
				// body
				func() {
					if s, ok := stmt.Statement(index + 1).(*JS.StatementContext); ok {
						b.buildStatement(s)
					}
				},
			)
		}

		elsestmt, ok := stmt.ElseBlock().(*JS.ElseBlockContext)
		if !ok {
			return i
		}
		if elseB, ok := elsestmt.Statement().(*JS.StatementContext); ok {
			i.SetElse(
				// create false block
				func() {
					b.buildStatement(elseB)
				},
			)
		}
		return i
	}

	i := buildIf(stmt)
	i.Build()
}

func (b *astbuilder) buildBlock(stmt *JS.BlockContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	// b.CurrentBlock.SetRange(b.CurrentRange)

	if s, ok := stmt.Statements().(*JS.StatementsContext); ok {
		// b.BuildSyntaxBlock(func() {
		b.buildStatements(s)
		// })
	} else {
		b.NewError(ssa.Warn, TAG, "empty block")
	}
}

// do while
func (b *astbuilder) buildDoStatement(stmt *JS.DoStatementContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	// do while需要分次

	// 先进行一次do
	if s, ok := stmt.Statement().(*JS.StatementContext); ok {
		b.buildStatement(s)
	}

	// 构建循环进行条件判断
	loop := b.CreateLoopBuilder()

	var cond *JS.ExpressionSequenceContext

	if s, ok := stmt.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
		cond = s
	}

	loop.SetCondition(func() ssa.Value {
		var condition ssa.Value
		if utils.IsNil(cond) {
			condition = b.EmitConstInst(true)
		} else {
			condition = b.buildExpressionSequence(cond)
		}
		if utils.IsNil(condition) {
			condition = b.EmitConstInst(true)
		}
		return condition
	})

	loop.SetBody(func() {
		if s, ok := stmt.Statement().(*JS.StatementContext); ok {
			b.buildStatement(s)
		}
	})

	loop.Finish()

}

// while
func (b *astbuilder) buildwhileStatement(stmt *JS.WhileStatementContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	// 构建循环进行条件判断
	loop := b.CreateLoopBuilder()

	var cond *JS.ExpressionSequenceContext

	if s, ok := stmt.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
		cond = s
	}

	loop.SetCondition(func() ssa.Value {
		var condition ssa.Value
		if utils.IsNil(cond) {
			condition = b.EmitConstInst(true)
		} else {
			condition = b.buildExpressionSequence(cond)
		}
		if utils.IsNil(condition) {
			condition = b.EmitConstInst(true)
		}
		return condition
	})

	loop.SetBody(func() {
		if s, ok := stmt.Statement().(*JS.StatementContext); ok {
			b.buildStatement(s)
		}
	})

	loop.Finish()

}

// for
func (b *astbuilder) buildForStatement(stmt *JS.ForStatementContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	loop := b.CreateLoopBuilder()

	var cond *JS.ExpressionSequenceContext

	// fmt.Println("---------------------")
	if first, ok := stmt.ForFirst().(*JS.ForFirstContext); ok {
		if f, ok := first.VariableDeclarationList().(*JS.VariableDeclarationListContext); ok {
			loop.SetFirst(func() []ssa.Value {
				recoverRange := b.SetRange(f.BaseParserRuleContext)
				defer recoverRange()
				rv, _ := b.buildAllVariableDeclaration(f, false)
				return []ssa.Value{rv}
			})
		} else if f, ok := first.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
			loop.SetFirst(func() []ssa.Value {
				// recoverRange := b.SetRange(&f.BaseParserRuleContext)
				// defer recoverRange()
				ret := b.buildExpressionSequence(f)
				return []ssa.Value{ret}
			})
		}
	}

	if expr, ok := stmt.ForSecond().(*JS.ForSecondContext); ok {
		if e, ok := expr.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
			cond = e
		}
	}

	if third, ok := stmt.ForThird().(*JS.ForThirdContext); ok {
		if t, ok := third.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
			loop.SetThird(func() []ssa.Value {
				// build third expression in loop.latch
				// recoverRange := b.SetRange(&t.BaseParserRuleContext)
				// defer recoverRange()
				// var ret []ssa.Value
				ret := b.buildExpressionSequence(t)
				return []ssa.Value{ret}
			})
		}
	}

	// 构建条件
	loop.SetCondition(func() ssa.Value {
		var condition ssa.Value
		// 没有条件就是永真
		if utils.IsNil(cond) {
			condition = b.EmitConstInst(true)
		} else {
			condition = b.buildExpressionSequence(cond)
		}
		if utils.IsNil(condition) {
			condition = b.EmitConstInst(true)
		}
		return condition
	})

	// build body
	loop.SetBody(func() {
		if s, ok := stmt.Statement().(*JS.StatementContext); ok {
			b.buildStatement(s)
		}
	})

	loop.Finish()
}

// for in 取key
func (b *astbuilder) buildForInStatement(stmt *JS.ForInStatementContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	loop := b.CreateLoopBuilder()

	loop.SetCondition(func() ssa.Value {
		var lv *ssa.Variable
		var value ssa.Value

		if s, ok := stmt.VariableDeclarationList().(*JS.VariableDeclarationListContext); ok {
			_, lv = b.buildAllVariableDeclaration(s, true)
		} else {
			_, lv = b.buildSingleExpression(stmt.SingleExpression(), true)
		}

		if s, ok := stmt.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
			value = b.buildExpressionSequence(s)
		}

		key, _, ok := b.EmitNext(value, false)
		b.AssignVariable(lv, key)
		if utils.IsNil(ok) {
			ok = b.EmitConstInst(true)
			// b.NewError(ssa.Warn, TAG, "loop condition expression is nil, default is true")
		}
		return ok
	})

	loop.SetBody(func() {
		if s, ok := stmt.Statement().(*JS.StatementContext); ok {
			b.buildStatement(s)
		}
	})

	loop.Finish()
}

// for of 取值
func (b *astbuilder) buildForOfStatement(stmt *JS.ForOfStatementContext) {
	// todo: handle await

	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	loop := b.CreateLoopBuilder()

	loop.SetCondition(func() ssa.Value {
		var lv *ssa.Variable
		var value ssa.Value

		if s, ok := stmt.VariableDeclarationList().(*JS.VariableDeclarationListContext); ok {
			_, lv = b.buildAllVariableDeclaration(s, true)
		} else {
			_, lv = b.buildSingleExpression(stmt.SingleExpression(), true)
		}

		if s, ok := stmt.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
			value = b.buildExpressionSequence(s)
		}

		_, field, ok := b.EmitNext(value, true)
		b.AssignVariable(lv, field)
		if utils.IsNil(ok) {
			ok = b.EmitConstInst(true)
			// b.NewError(ssa.Warn, TAG, "loop condition expression is nil, default is true")
		}
		return ok
	})

	loop.SetBody(func() {
		if s, ok := stmt.Statement().(*JS.StatementContext); ok {
			b.buildStatement(s)
		}
	})

	loop.Finish()
}

func (b *astbuilder) buildFunctionDeclaration(stmt *JS.FunctionDeclarationContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	funcName := ""
	if Name := stmt.Identifier(); Name != nil {
		funcName = Name.GetText()
	}

	// fmt.Println("funcName: ", funcName)

	newFunc := b.NewFunc(funcName)
	{
		recoverRange := b.SetRange(stmt.BaseParserRuleContext)

		b.FunctionBuilder = b.PushFunction(newFunc)

		if s, ok := stmt.FormalParameterList().(*JS.FormalParameterListContext); ok {
			b.buildFormalParameterList(s)
		}

		if f, ok := stmt.FunctionBody().(*JS.FunctionBodyContext); ok {
			b.buildFunctionBody(f)
		}

		b.Finish()
		b.FunctionBuilder = b.PopFunction()

		recoverRange()
	}

	if funcName != "" {
		variable := b.CreateVariable(funcName)
		b.AssignVariable(variable, newFunc)
	}
	return newFunc
}

func (b *astbuilder) buildFunctionBody(stmt *JS.FunctionBodyContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.Statements().(*JS.StatementsContext); ok {
		b.buildStatements(s)
		return
	}
}

// func (b *astbuilder) buildSourceElements(stmt *JS.SourceElementsContext) {
// 	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
// 	defer recoverRange()

// 	if s := stmt.AllSourceElement(); s != nil {
// 		for _, i := range s {
// 			b.buildSourceElement(i)
// 		}
// 	}
// }

// func (b *astbuilder) buildSourceElement(stmt JS.ISourceElementContext) {
// 	if s, ok := stmt.(*JS.SourceElementContext); ok {
// 		b.buildStatement(s.Statement().(*JS.StatementContext))
// 		return
// 	}
// }

func (b *astbuilder) buildFormalParameterList(stmt *JS.FormalParameterListContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if f := stmt.AllFormalParameterArg(); f != nil {
		for _, i := range f {
			if a, ok := i.(*JS.FormalParameterArgContext); ok {
				b.buildFormalParameterArg(a)
			}
		}

		if l, ok := stmt.LastFormalParameterArg().(*JS.LastFormalParameterArgContext); ok {
			b.buildLastFormalParameterArg(l)
		}
		return
	}

	if l, ok := stmt.LastFormalParameterArg().(*JS.LastFormalParameterArgContext); ok {
		b.buildLastFormalParameterArg(l)
		return
	}

	b.NewError(ssa.Error, TAG, ArrowFunctionNeedExpressionOrBlock())
}

func (b *astbuilder) buildFormalParameterArg(stmt *JS.FormalParameterArgContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	a := stmt.Assign()

	if a == nil {
		b.NewParam(stmt.GetText())
		// p.SetDefault(ssa.NewUndefined(stmt.GetText()))
	} else {
		p := b.NewParam(stmt.Assignable().GetText())

		x := stmt.SingleExpression()
		result, _ := b.buildSingleExpression(x, false)

		p.SetDefault(result)
		return
	}
}

func (b *astbuilder) buildLastFormalParameterArg(stmt *JS.LastFormalParameterArgContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if e := stmt.Ellipsis(); e != nil {
		b.HandlerEllipsis()
	}

	if s := stmt.SingleExpression(); s != nil {
		b.buildSingleExpression(s, false)
	}
}

func (b *astbuilder) buildReturnStatement(stmt *JS.ReturnStatementContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	if s, ok := stmt.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
		values := b.buildExpressionSequence(s)
		b.EmitReturn([]ssa.Value{values})
	} else {
		b.EmitReturn(nil)
	}
}

func (b *astbuilder) buildBreakStatement(stmt *JS.BreakStatementContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var _break *ssa.BasicBlock

	if s, ok := stmt.Identifier().(*JS.IdentifierContext); ok {
		text := s.GetText()
		if _break = b.GetLabel(text); _break != nil {
			b.EmitJump(_break)
		} else {
			b.NewError(ssa.Error, TAG, UndefineLabelstmt())
		}
		return
	}

	if !b.Break() {
		b.NewError(ssa.Error, TAG, UnexpectedBreakStmt())
	}
	return

}

// TODO: block sealed
func (b *astbuilder) buildLabelledStatement(stmt *JS.LabelledStatementContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	text := ""
	if s, ok := stmt.Identifier().(*JS.IdentifierContext); ok {
		text = s.GetText()
	}

	// unsealed block
	block := b.NewBasicBlockUnSealed(text)
	block.SetScope(b.CurrentBlock.ScopeTable.CreateSubScope())
	b.AddLabel(text, block)
	// to block
	b.EmitJump(block)
	b.CurrentBlock = block
	if s, ok := stmt.Statement().(*JS.StatementContext); ok {
		b.buildStatement(s)
	}
	b.DeleteLabel(text)
	// block.Sealed()
}

func (b *astbuilder) buildTryStatement(stmt *JS.TryStatementContext) {
	revcoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer revcoverRange()

	try := b.BuildTry()

	try.BuildTryBlock(func() {
		if s, ok := stmt.Block().(*JS.BlockContext); ok {
			b.buildBlock(s)
		}
	})
	try.BuildErrorCatch(func() string {
		var id string
		// TODO: Assignable could be wrong, need to fix
		if s, ok := stmt.CatchProduction().(*JS.CatchProductionContext); ok {
			if a, ok := s.Assignable().(*JS.AssignableContext); ok {
				b.buildAssignableContext(a)
				id = a.GetText()
			}
		}
		return id
	}, func() {
		if _, ok := stmt.CatchProduction().(*JS.CatchProductionContext); ok {
			if bl, ok := stmt.Block().(*JS.BlockContext); ok {
				b.buildBlock(bl)
			}
		}
	})

	if s, ok := stmt.FinallyProduction().(*JS.FinallyProductionContext); ok {

		try.BuildFinally(func() {
			if bl, ok := s.Block().(*JS.BlockContext); ok {
				b.buildBlock(bl)
			}
		})
	}

	try.Finish()

}

func (b *astbuilder) buildSwitchStatement(stmt *JS.SwitchStatementContext) {
	revcoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer revcoverRange()

	Switchb := b.BuildSwitch()
	Switchb.AutoBreak = false

	if s, ok := stmt.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
		Switchb.BuildCondition(func() ssa.Value {
			rv := b.buildExpressionSequence(s)
			return rv
		})
	} else {
		recoverRange := b.SetRangeFromTerminalNode(stmt.Switch())
		b.NewError(ssa.Warn, TAG, "switch expression is nil")
		recoverRange()
	}

	if s, ok := stmt.CaseBlock().(*JS.CaseBlockContext); ok {
		b.buildCaseBlock(s, Switchb)
	}
}

func (b *astbuilder) buildCaseBlock(stmt *JS.CaseBlockContext, Switchb *ssa.SwitchBuilder) {
	revcoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer revcoverRange()

	type caseClause struct {
		exprs *JS.ExpressionSequenceContext
		stmt  *JS.StatementsContext
	}

	var stList []caseClause

	for _, s := range stmt.AllCaseClauses() {
		cs, ok := s.(*JS.CaseClausesContext)
		if !ok {
			continue
		}
		for _, i := range cs.AllCaseClause() {
			c, ok := i.(*JS.CaseClauseContext)
			if !ok {
				continue
			}

			exprs, ok := c.ExpressionSequence().(*JS.ExpressionSequenceContext)
			if !ok {
				exprs = nil
			}

			st, ok := c.Statements().(*JS.StatementsContext)
			if !ok {
				st = nil
			}

			stList = append(stList, caseClause{
				exprs: exprs,
				stmt:  st,
			})
		}
	}

	Switchb.BuildCaseSize(len(stList))
	Switchb.SetCase(func(i int) []ssa.Value {
		if stList[i].exprs != nil {
			return []ssa.Value{
				b.buildExpressionSequence(stList[i].exprs),
			}
		}
		return []ssa.Value{}
	})

	Switchb.BuildBody(func(i int) {
		if stList[i].stmt != nil {
			b.buildStatements(stList[i].stmt)
		}
	})

	if s, ok := stmt.DefaultClause().(*JS.DefaultClauseContext); ok {
		if st, ok := s.Statements().(*JS.StatementsContext); ok {
			Switchb.BuildDefault(func() {
				b.buildStatements(st)
			})
		}
	}

	Switchb.Finish()

}
