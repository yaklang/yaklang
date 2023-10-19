package js2ssa

import (
	"fmt"

	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// entry point
func (b *astbuilder) build(ast *JS.JavaScriptParser) {
	b.buildStatementList(ast.StatementList().(*JS.StatementListContext))
}

// statement list
func (b *astbuilder) buildStatementList(stmtlist *JS.StatementListContext) {
	recoverRange := b.SetRange(&stmtlist.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	// var
	if s, ok := stmt.VariableStatement().(*JS.VariableStatementContext); ok {
		b.buildVariableStatement(s)
		return
	}

	// expr
	if s, ok := stmt.ExpressionStatement().(*JS.ExpressionStatementContext); ok {
		b.buildExpressionStatement(s)
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

	if s, ok := stmt.FunctionDeclaration().(*JS.FunctionDeclarationContext); ok {
		b.buildFunctionDeclaration(s)
	}

}

func (b *astbuilder) buildVariableStatement(stmt *JS.VariableStatementContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.VariableDeclarationList().(*JS.VariableDeclarationListContext); ok {
		b.buildAllVariableDeclaration(s)
		return
	}

}

func (b *astbuilder) buildAllVariableDeclaration(stmt *JS.VariableDeclarationListContext) []ssa.Value {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()
	var ret []ssa.Value

	for _, jsstmt := range stmt.AllVariableDeclaration() {
		v := b.buildVariableDeclaration(jsstmt)
		ret = append(ret, v)
	}
	// fmt.Println(ret)
	return ret
}

func (b *astbuilder) buildVariableDeclaration(stmt JS.IVariableDeclarationContext) ssa.Value {
	a := stmt.Assign()

	if a == nil {
		if as, ok := stmt.Assignable().(*JS.AssignableContext); ok {
			lValue := b.buildAssignableContext(as)
			return lValue.GetValue(b.FunctionBuilder)
		}
	} else {
		var lValue ssa.LeftValue

		// 得到一个左值
		if as, ok := stmt.Assignable().(*JS.AssignableContext); ok {
			lValue = b.buildAssignableContext(as)
		}

		x := stmt.SingleExpression()
		result, _ := b.buildSingleExpression(x, false)
		// fmt.Println("result :", result)

		lValue.Assign(result, b.FunctionBuilder)
		return lValue.GetValue(b.FunctionBuilder)
	}
	return nil
}

func (b *astbuilder) buildAssignableContext(stmt *JS.AssignableContext) ssa.LeftValue {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	if i := stmt.Identifier(); i != nil {
		text := i.GetText()
		_, lv := b.buildIdentifierExpression(text, true)
		return lv
	}

	return nil
}

type getSingleExpr interface {
	SingleExpression(i int) JS.ISingleExpressionContext
}

func (b *astbuilder) buildSingleExpression(stmt JS.ISingleExpressionContext, IslValue bool) (ssa.Value, ssa.LeftValue) {
	// TODO: unfinish

	if v := b.buildOnlyRightSingleExpression(stmt); v != nil {
		return v, nil
	} else {
		//todo
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

	getValue := func(single getSingleExpr, i int) ssa.Value {
		if s := single.SingleExpression(i); s != nil {
			v, _ := b.buildSingleExpression(s, false)
			return v
		} else {
			return nil
		}
	}

	// 字面量
	if s, ok := stmt.(*JS.LiteralExpressionContext); ok {
		return b.buildLiteralExpression(s)
	}
	// expr
	if s, ok := stmt.(*JS.AssignmentExpressionContext); ok {
		return b.buildAssignmentExpression(s)
	}

	// ++
	if s, ok := stmt.(*JS.PostIncrementExpressionContext); ok {
		if expr, ok := s.SingleExpression().(JS.ISingleExpressionContext); ok {
			_, lValue := b.buildSingleExpression(expr, true)
			if v := lValue.GetValue(b.FunctionBuilder); v == nil {
				b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
				return nil
			} else {
				rValue := b.EmitBinOp(ssa.OpAdd, lValue.GetValue(b.FunctionBuilder), ssa.NewConst(1))
				lValue.Assign(rValue, b.FunctionBuilder)
				fmt.Println("++ result: ", lValue.GetValue(b.FunctionBuilder))
				return lValue.GetValue(b.FunctionBuilder)
			}
		}
	}

	// --
	if s, ok := stmt.(*JS.PostIncrementExpressionContext); ok {
		if expr, ok := s.SingleExpression().(JS.ISingleExpressionContext); ok {
			_, lValue := b.buildSingleExpression(expr, true)
			if v := lValue.GetValue(b.FunctionBuilder); v == nil {
				b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
				return nil
			} else {
				rValue := b.EmitBinOp(ssa.OpSub, lValue.GetValue(b.FunctionBuilder), ssa.NewConst(1))
				lValue.Assign(rValue, b.FunctionBuilder)
				return lValue.GetValue(b.FunctionBuilder)
			}
		}
	}

	if s, ok := stmt.(*JS.AssignmentOperatorExpressionContext); ok {
		_, lValue := b.buildSingleExpression(s.SingleExpression(0), true)
		rValue, _ := b.buildSingleExpression(s.SingleExpression(1), false)

		if lValue == nil || rValue == nil {
			b.NewError(ssa.Error, TAG, "in operator need two expression")
			return nil
		}

		if f, ok := s.AssignmentOperator().(*JS.AssignmentOperatorContext); ok {
			return b.buildAssignmentOperatorContext(f, lValue, rValue)
		}
	}

	getBinaryOp := func() (single getSingleExpr, Op ssa.BinaryOpcode, IsBinOp bool) {
		single, Op, IsBinOp = nil, 0, false
		for {
			a := stmt
			fmt.Println(a.GetText())
			if s, ok := stmt.(*JS.AdditiveExpressionContext); ok {
				if op := s.Plus(); op != nil {
					single, Op, IsBinOp = s, ssa.OpAdd, true
				} else if op := s.Minus(); op != nil {
					single, Op, IsBinOp = s, ssa.OpSub, true
				} else {
					break
				}
			}

			// todo
			if s, ok := stmt.(*JS.EqualityExpressionContext); ok {
				if op := s.Equals_(); op != nil {
					single, Op, IsBinOp = s, ssa.OpEq, true
				} else if op := s.NotEquals(); op != nil {
					single, Op, IsBinOp = s, ssa.OpNotEq, true
				} else {
					break
				}
			}

			if s, ok := stmt.(*JS.RelationalExpressionContext); ok {
				if op := s.LessThan(); op != nil {
					single, Op, IsBinOp = s, ssa.OpLt, true
				} else if op := s.MoreThan(); op != nil {
					single, Op, IsBinOp = s, ssa.OpGt, true
				} else if op := s.LessThanEquals(); op != nil {
					single, Op, IsBinOp = s, ssa.OpLtEq, true
				} else if op := s.GreaterThanEquals(); op != nil {
					single, Op, IsBinOp = s, ssa.OpGtEq, true
				} else {
					break
				}
			}
			return
		}
		b.NewError(ssa.Error, TAG, "binary operator not support: %s", stmt.GetText())
		return
	}

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

	return nil
}

func (b *astbuilder) buildSingleExpressionEx(stmt JS.ISingleExpressionContext, IslValue bool) (ssa.Value, ssa.LeftValue) {
	//可能是左值也可能是右值的

	//标识符
	if s, ok := stmt.(*JS.IdentifierExpressionContext); ok {
		i := s.GetText()
		value, lValue := b.buildIdentifierExpression(i, IslValue)
		return value, lValue
	}

	return nil, nil
}

func (b *astbuilder) buildAssignmentOperatorContext(stmt *JS.AssignmentOperatorContext, lValue ssa.LeftValue, rValue ssa.Value) ssa.Value {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	var Op ssa.BinaryOpcode
	if op := stmt.PlusAssign(); op != nil {
		Op = ssa.OpAdd
	} else if op := stmt.MinusAssign(); op != nil {
		Op = ssa.OpSub
	} else if op := stmt.DivideAssign(); op != nil {
		Op = ssa.OpDiv
	} else if op := stmt.ModulusAssign(); op != nil {
		Op = ssa.OpMod
	} else if op := stmt.DivideAssign(); op != nil {
		Op = ssa.OpDiv
	} else if op := stmt.MultiplyAssign(); op != nil {
		Op = ssa.OpMul
	} else if op := stmt.LeftShiftArithmeticAssign(); op != nil {
		Op = ssa.OpShl
	} else if op := stmt.RightShiftArithmeticAssign(); op != nil {
		Op = ssa.OpShr
	} else if op := stmt.BitOrAssign(); op != nil {
		Op = ssa.OpOr
	} else if op := stmt.BitXorAssign(); op != nil {
		Op = ssa.OpXor
	} else if op := stmt.BitAndAssign(); op != nil {
		Op = ssa.OpAnd
	}

	// TODO:powerAssign **=, RightShiftLogicalAssign >>>=

	value := b.EmitBinOp(Op, lValue.GetValue(b.FunctionBuilder), rValue)
	lValue.Assign(value, b.FunctionBuilder)

	fmt.Println("test assignOpreator: ", lValue.GetValue(b.FunctionBuilder))
	return lValue.GetValue(b.FunctionBuilder)
}

func (b *astbuilder) buildIdentifierExpression(text string, IslValue bool) (ssa.Value, ssa.LeftValue) {
	// recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	// defer recoverRange()

	if IslValue {
		//leftValue
		lValue := ssa.NewIdentifierLV(text, b.CurrentPos)
		return nil, lValue
	} else {
		rValue := b.ReadVariable(text, true)
		return rValue, nil
	}
}

func (b *astbuilder) buildAssignmentExpression(stmt *JS.AssignmentExpressionContext) ssa.Value {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	_, op1 := b.buildSingleExpression(stmt.SingleExpression(0), true)
	op2, _ := b.buildSingleExpression(stmt.SingleExpression(1), false)

	if op1 != nil && op2 != nil {
		text := stmt.SingleExpression(0).GetText()
		lValue := ssa.NewIdentifierLV(text, b.CurrentPos)
		lValue.Assign(op2, b.FunctionBuilder)
		fmt.Print(text)
		fmt.Print("=")
		fmt.Println(lValue.GetValue(b.FunctionBuilder))
	} else {
		b.NewError(ssa.Error, TAG, "AssignmentExpression cannot get right assignable: %s", stmt.GetText())
	}

	return op2
}

func (b *astbuilder) buildExpressionStatement(stmt *JS.ExpressionStatementContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
		b.buildExpressionSequence(s)
	}
}

func (b *astbuilder) buildExpressionSequence(stmt *JS.ExpressionSequenceContext) ssa.Value {
	// warining: must fix
	// 需要修改改函数及引用，不存在if中存在多个singleExpression的情况

	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	for _, expr := range stmt.AllSingleExpression() {
		if s := expr; s != nil {
			b.buildSingleExpression(s, false)
		}
	}
	return nil
}

func (b *astbuilder) buildIfStatementContext(stmt *JS.IfStatementContext) {
	// var buildIf func(stmt *JS.IfStatementContext) *ssa.IfBuilder
	buildIf := func(stmt *JS.IfStatementContext) *ssa.IfBuilder {
		recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
		defer recoverRange()

		i := b.BuildIf()

		// if instruction condition
		i.BuildCondition(
			func() ssa.Value {
				if s, ok := stmt.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
					return b.buildExpressionSequence(s)
				}
				return nil
			})

		i.BuildTrue(
			func() {
				if s, ok := stmt.Statement(0).(*JS.StatementContext); ok {
					b.buildStatement(s)
				}
			},
		)

		if s, ok := stmt.Statement(1).(*JS.StatementContext); ok {
			if !ok {
				return i
			} else {
				i.BuildFalse(
					func() {
						b.buildStatement(s)
					},
				)
			}
		}

		return i
	}

	i := buildIf(stmt)
	i.Finish()
}

func (b *astbuilder) buildBlock(stmt *JS.BlockContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.StatementList().(*JS.StatementListContext); ok {
		b.buildStatementList(s)
	}
}

// do while
func (b *astbuilder) buildDoStatement(stmt *JS.DoStatementContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	// do while需要分次

	// 先进行一次do
	if s, ok := stmt.Statement().(*JS.StatementContext); ok {
		b.buildStatement(s)
	}

	// 构建循环进行条件判断
	loop := b.BuildLoop()

	var cond *JS.ExpressionSequenceContext

	if s, ok := stmt.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
		cond = s
	}

	loop.BuildCondition(func() ssa.Value {
		var condition ssa.Value
		if cond == nil {
			condition = ssa.NewConst(true)
		} else {
			condition = b.buildExpressionSequence(cond)
			if condition == nil {
				condition = ssa.NewConst(true)
			}
		}
		return condition
	})

	loop.BuildBody(func() {
		if s, ok := stmt.Statement().(*JS.StatementContext); ok {
			b.buildStatement(s)
		}
	})

	loop.Finish()

}

func (b *astbuilder) buildForStatement(stmt *JS.ForStatementContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	loop := b.BuildLoop()

	var cond *JS.ExpressionSequenceContext

	fmt.Println("---------------------")
	if first, ok := stmt.ForFirst().(*JS.ForFirstContext); ok {
		if f, ok := first.VariableDeclarationList().(*JS.VariableDeclarationListContext); ok {
			loop.BuildFirstExpr(func() []ssa.Value {
				recoverRange := b.SetRange(&f.BaseParserRuleContext)
				defer recoverRange()
				return b.buildAllVariableDeclaration(f)
			})
		} else if f, ok := first.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
			loop.BuildFirstExpr(func() []ssa.Value {
				recoverRange := b.SetRange(&f.BaseParserRuleContext)
				defer recoverRange()
				var ret []ssa.Value
				ret = append(ret, b.buildExpressionSequence(f))
				return ret
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
			loop.BuildThird(func() []ssa.Value {
				// build third expression in loop.latch
				recoverRange := b.SetRange(&t.BaseParserRuleContext)
				defer recoverRange()
				var ret []ssa.Value
				ret = append(ret, b.buildExpressionSequence(t))
				return ret
			})
		}
	}

	// 构建条件
	loop.BuildCondition(func() ssa.Value {
		var condition ssa.Value
		// 没有条件就是永真
		if cond == nil {
			condition = ssa.NewConst(true)
		} else {
			condition = b.buildExpressionSequence(cond)
			if condition == nil {
				condition = ssa.NewConst(true)
			}
		}
		return condition
	})

	// build body
	loop.BuildBody(func() {
		if s, ok := stmt.Statement().(*JS.StatementContext); ok {
			b.buildStatement(s)
		}
	})

	loop.Finish()
}

func (b *astbuilder) buildFunctionDeclaration(stmt *JS.FunctionDeclarationContext) ssa.Value {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	funcName := ""
	if Name := stmt.Identifier(); Name != nil {
		funcName = Name.GetText()
	}

	// fmt.Println("funcName: ", funcName)

	newFunc, symbol := b.NewFunc(funcName)
	current := b.CurrentBlock
	buildFunc := func() {
		b.FunctionBuilder = b.PushFunction(newFunc, symbol, current)

		if s, ok := stmt.FormalParameterList().(*JS.FormalParameterListContext); ok {
			b.buildFormalParameterList(s)
		}

		if f, ok := stmt.FunctionBody().(*JS.FunctionBodyContext); ok {
			b.buildFunctionBody(f)
		}

		b.Finish()
		b.FunctionBuilder = b.PopFunction()

	}

	b.AddSubFunction(buildFunc)

	if funcName != "" {
		b.WriteVariable(funcName, newFunc)
	}
	return newFunc
}

func (b *astbuilder) buildFunctionBody(stmt *JS.FunctionBodyContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.SourceElements().(*JS.SourceElementsContext); ok {
		b.buildSourceElements(s)
		return
	}
}

func (b *astbuilder) buildSourceElements(stmt *JS.SourceElementsContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	if s := stmt.AllSourceElement(); s != nil {
		for _, i := range s {
			b.buildSourceElement(i)
		}
	}
}

func (b *astbuilder) buildSourceElement(stmt JS.ISourceElementContext) {
	if s, ok := stmt.Statement().(*JS.StatementContext); ok {
		b.buildStatement(s)
		return 
	}
}

func (b *astbuilder) buildFormalParameterList(stmt *JS.FormalParameterListContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	a := stmt.Assign()

	if a == nil {
		b.NewParam(stmt.GetText())
	} else {
		p := b.NewParam(stmt.Assignable().GetText())

		x := stmt.SingleExpression()
		result, _ := b.buildSingleExpression(x, false)

		p.SetDefault(result)
		return
	}
}

func (b *astbuilder) buildLastFormalParameterArg(stmt *JS.LastFormalParameterArgContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	
	if e := stmt.Ellipsis(); e != nil {
		b.HandlerEllipsis()
	}

	if s := stmt.SingleExpression(); s != nil {
		b.buildSingleExpression(s, false)
	}

	return
}
