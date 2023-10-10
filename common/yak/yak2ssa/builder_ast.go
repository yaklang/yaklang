package yak2ssa

import (
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const TAG ssa.ErrorTag = "yakast"

// entry point
func (b *astbuilder) build(ast *yak.YaklangParser) {
	// ast.StatementList()
	b.buildStatementList(ast.StatementList().(*yak.StatementListContext))
}

// statement list
func (b *astbuilder) buildStatementList(stmtlist *yak.StatementListContext) {
	recoverRange := b.SetRange(stmtlist.BaseParserRuleContext)
	defer recoverRange()
	allstmt := stmtlist.AllStatement()
	if len(allstmt) == 0 {
		b.NewError(ssa.Warn, TAG, "empty statement list")
	} else {
		for _, stmt := range allstmt {
			if stmt, ok := stmt.(*yak.StatementContext); ok {
				b.buildStatement(stmt)
			}
		}
	}
}

func (b *astbuilder) buildStatement(stmt *yak.StatementContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	// declare Variable Expression
	if s, ok := stmt.DeclareVariableExpressionStmt().(*yak.DeclareVariableExpressionStmtContext); ok {
		b.buildDeclareVariableExpressionStmt(s)
		return
	}

	// assign Expression
	if s, ok := stmt.AssignExpressionStmt().(*yak.AssignExpressionStmtContext); ok {
		b.buildAssignExpressionStmt(s)
		return
	}

	// expression
	if s, ok := stmt.ExpressionStmt().(*yak.ExpressionStmtContext); ok {
		b.buildExpressionStmt(s)
		return
	}

	// block
	if s, ok := stmt.Block().(*yak.BlockContext); ok {
		b.buildBlock(s)
		return
	}

	// try Stmt
	if s, ok := stmt.TryStmt().(*yak.TryStmtContext); ok {
		b.buildTryCatchStmt(s)
		return
	}

	// if stmt
	if s, ok := stmt.IfStmt().(*yak.IfStmtContext); ok {
		b.buildIfStmt(s)
		return
	}

	if s, ok := stmt.SwitchStmt().(*yak.SwitchStmtContext); ok {
		b.buildSwitchStmt(s)
		return
	}

	// for range stmt
	if s, ok := stmt.ForRangeStmt().(*yak.ForRangeStmtContext); ok {
		b.buildForRangeStmt(s)
		return
	}

	// for stmt
	if s, ok := stmt.ForStmt().(*yak.ForStmtContext); ok {
		b.buildForStmt(s)
		return
	}

	// break stmt
	if _, ok := stmt.BreakStmt().(*yak.BreakStmtContext); ok {
		if _break := b.GetBreak(); _break != nil {
			b.EmitJump(_break)
		} else {
			b.NewError(ssa.Error, TAG, "unexpected break stmt")
		}
		return
	}
	// return stmt
	if s, ok := stmt.ReturnStmt().(*yak.ReturnStmtContext); ok {
		b.buildReturnStmt(s)
		return
	}
	// continue stmt
	if _, ok := stmt.ContinueStmt().(*yak.ContinueStmtContext); ok {
		if _continue := b.GetContinue(); _continue != nil {
			b.EmitJump(_continue)
		} else {
			b.NewError(ssa.Error, TAG, "unexpected continue stmt")
		}
		return
	}

	if _, ok := stmt.FallthroughStmt().(*yak.FallthroughStmtContext); ok {
		if _fall := b.GetFallthrough(); _fall != nil {
			b.EmitJump(_fall)
		} else {
			b.NewError(ssa.Error, TAG, "unexpected fallthrough stmt")
		}
		return
	}
	//TODO: include stmt and check file path

	// defer stmt
	if s, ok := stmt.DeferStmt().(*yak.DeferStmtContext); ok {
		b.buildDeferStmt(s)
		return
	}

	// go stmt
	if s, ok := stmt.GoStmt().(*yak.GoStmtContext); ok {
		b.buildGoStmt(s)
	}

	// assert stmt
	if s, ok := stmt.AssertStmt().(*yak.AssertStmtContext); ok {
		b.buildAssertStmt(s)
	}
}

func (b *astbuilder) buildAssertStmt(stmt *yak.AssertStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	getExpr := func(i int) ssa.Value {
		if expr, ok := stmt.Expression(i).(*yak.ExpressionContext); ok {
			return b.buildExpression(expr)
		}
		b.NewError(ssa.Error, TAG, "unexpected assert stmt, this not expression")
		return nil
	}

	exprs := stmt.AllExpression()
	lenexprs := len(exprs)

	var cond, msgV ssa.Value

	cond = getExpr(0)
	if lenexprs > 1 {
		msgV = getExpr(1)
	}

	b.EmitAssert(cond, msgV, exprs[0].GetText())
}

// try stmt
func (b *astbuilder) buildTryCatchStmt(stmt *yak.TryStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	var final *ssa.BasicBlock
	enter := b.CurrentBlock

	b.CurrentBlock = enter
	try := b.NewBasicBlock("error.try")
	catch := b.NewBasicBlock("error.catch")
	e := b.EmitErrorHandler(try, catch)

	b.CurrentBlock = try
	b.buildBlock(stmt.Block(0).(*yak.BlockContext))

	b.CurrentBlock = catch
	if id := stmt.Identifier(); id != nil {
		p := ssa.NewParam(id.GetText(), false, b.Function)
		p.SetType(ssa.BasicTypes[ssa.Error])
		b.WriteVariable(id.GetText(), p)
	}
	b.buildBlock(stmt.Block(1).(*yak.BlockContext))

	if fblock, ok := stmt.Block(2).(*yak.BlockContext); ok {
		b.CurrentBlock = enter
		final = b.NewBasicBlock("error.final")
		e.AddFinal(final)
		b.CurrentBlock = final
		b.buildBlock(fblock)
	}

	b.CurrentBlock = enter
	done := b.NewBasicBlock("")
	e.AddDone(done)

	b.CurrentBlock = done
}

// expression stmt
func (b *astbuilder) buildExpressionStmt(stmt *yak.ExpressionStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	if s, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		b.buildExpression(s)
	}
}

// assign expression stmt
func (b *astbuilder) buildAssignExpressionStmt(stmt *yak.AssignExpressionStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	s := stmt.AssignExpression()
	if s == nil {
		return
	}
	if i, ok := s.(*yak.AssignExpressionContext); ok {
		b.buildAssignExpression(i)
	}
}

// TODO: include stmt

// defer stmt
func (b *astbuilder) buildDeferStmt(stmt *yak.DeferStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if stmt, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		// instance code
		if s, ok := stmt.InstanceCode().(*yak.InstanceCodeContext); ok {
			b.AddDefer(b.buildInstanceCode(s))
		}

		// function call
		if s, ok := stmt.FunctionCall().(*yak.FunctionCallContext); ok {
			if c := b.buildFunctionCallWarp(stmt, s); c != nil {
				b.AddDefer(c)
			}
		}
	}
}

// go stmt
func (b *astbuilder) buildGoStmt(stmt *yak.GoStmtContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var c *ssa.Call
	if s, ok := stmt.InstanceCode().(*yak.InstanceCodeContext); ok {
		c = b.buildInstanceCode(s)
	} else {
		v := b.buildExpression(stmt.Expression().(*yak.ExpressionContext))
		c = b.buildFunctionCall(stmt.FunctionCall().(*yak.FunctionCallContext), v)
	}
	c.Async = true
	b.EmitCall(c)
	return c
}

// return stmt
func (b *astbuilder) buildReturnStmt(stmt *yak.ReturnStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	if list, ok := stmt.ExpressionList().(*yak.ExpressionListContext); ok {
		values := b.buildExpressionList(list)
		b.EmitReturn(values)
	} else {
		b.EmitReturn(nil)
	}
}

// for stmt
func (b *astbuilder) buildForStmt(stmt *yak.ForStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	// current := f.currentBlock
	loop := b.BuildLoop()

	// var cond ssa.Value
	var cond *yak.ExpressionContext
	if e, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		// if only expression; just build expression in header;
		cond = e
	} else if condition, ok := stmt.ForStmtCond().(*yak.ForStmtCondContext); ok {
		if first, ok := condition.ForFirstExpr().(*yak.ForFirstExprContext); ok {
			// first expression is initialization, in enter block
			loop.BuildFirstExpr(func() []ssa.Value {
				recoverRange := b.SetRange(first.BaseParserRuleContext)
				defer recoverRange()
				return b.ForExpr(first)
			})
		}
		if expr, ok := condition.Expression().(*yak.ExpressionContext); ok {
			// build expression in header
			cond = expr
		}

		if third, ok := condition.ForThirdExpr().(*yak.ForThirdExprContext); ok {
			// build latch
			loop.BuildThird(func() []ssa.Value {
				// build third expression in loop.latch
				recoverRange := b.SetRange(third.BaseParserRuleContext)
				defer recoverRange()
				return b.ForExpr(third)
			})
		}
	}

	loop.BuildCondition(func() ssa.Value {
		var condition ssa.Value
		if cond == nil {
			condition = ssa.NewConst(true)
		} else {
			// recoverRange := b.SetRange(cond.BaseParserRuleContext)
			// defer recoverRange()
			condition = b.buildExpression(cond)
			if condition == nil {
				condition = ssa.NewConst(true)
				// b.NewError(ssa.Warn, TAG, "loop condition expression is nil, default is true")
			}
		}
		return condition
	})

	//  build body
	loop.BuildBody(func() {
		if block, ok := stmt.Block().(*yak.BlockContext); ok {
			b.buildBlock(block)
		}
	})

	loop.Finish()
}

type forExpr interface {
	Expression() yak.IExpressionContext
	AssignExpression() yak.IAssignExpressionContext
}

func (b *astbuilder) ForExpr(stmt forExpr) []ssa.Value {
	if ae, ok := stmt.AssignExpression().(*yak.AssignExpressionContext); ok {
		return b.buildAssignExpression(ae)
	} else if e, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		return []ssa.Value{b.buildExpression(e)}
	} else {
		// Impossible to get here
		return nil
	}
}

// for range stmt
func (b *astbuilder) buildForRangeStmt(stmt *yak.ForRangeStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	// current := f.currentBlock
	loop := b.BuildLoop()

	loop.BuildCondition(func() ssa.Value {
		var lefts []ssa.LeftValue
		if leftList, ok := stmt.LeftExpressionList().(*yak.LeftExpressionListContext); ok {
			lefts = b.buildLeftExpressionList(true, leftList)
			// } else {
		}
		value := b.buildExpression(stmt.Expression().(*yak.ExpressionContext))
		key, field, ok := b.EmitNext(value)
		if len(lefts) == 1 {
			if stmt.In() != nil {
				// in
				lefts[0].Assign(field, b.FunctionBuilder)
			} else {
				// range
				lefts[0].Assign(key, b.FunctionBuilder)
			}
		} else if len(lefts) >= 2 {
			lefts[0].Assign(key, b.FunctionBuilder)
			lefts[1].Assign(field, b.FunctionBuilder)
		}
		return ok
	})

	loop.BuildBody(func() {
		b.buildBlock(stmt.Block().(*yak.BlockContext))
	})

	loop.Finish()
}

// switch stmt
func (b *astbuilder) buildSwitchStmt(stmt *yak.SwitchStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	//  parse expression
	var cond ssa.Value
	if expr, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		cond = b.buildExpression(expr)
	} else {
		// expression is nil
		b.NewError(ssa.Warn, TAG, "switch expression is nil")
	}
	enter := b.CurrentBlock
	allcase := stmt.AllCase()
	slabel := make([]ssa.SwitchLabel, 0)
	handlers := make([]*ssa.BasicBlock, 0, len(allcase))
	done := b.NewBasicBlock("switch.done")
	defaultb := b.NewBasicBlock("switch.default")
	enter.AddSucc(defaultb)

	// handler label
	for i := range allcase {
		if exprlist, ok := stmt.ExpressionList(i).(*yak.ExpressionListContext); ok {
			exprs := b.buildExpressionList(exprlist)
			handler := b.NewBasicBlock("switch.handler")
			enter.AddSucc(handler)
			handlers = append(handlers, handler)
			if len(exprs) == 1 {
				// only one expr
				slabel = append(slabel, ssa.NewSwitchLabel(exprs[0], handler))
			} else {
				for _, expr := range exprs {
					slabel = append(slabel, ssa.NewSwitchLabel(expr, handler))
				}
			}
		}
	}
	// build body
	for i := range allcase {
		if stmtlist, ok := stmt.StatementList(i).(*yak.StatementListContext); ok {
			var _fallthrough *ssa.BasicBlock
			if i == len(allcase)-1 {
				_fallthrough = defaultb
			} else {
				_fallthrough = handlers[i+1]
			}
			b.PushTarget(done, nil, _fallthrough) // fallthrough just jump to next handler
			// build handlers block
			b.CurrentBlock = handlers[i]
			b.buildStatementList(stmtlist)
			// jump handlers-block -> done
			b.EmitJump(done)
			b.PopTarget()
		}
	}
	// default
	if stmt.Default() != nil {
		if stmtlist, ok := stmt.StatementList(len(allcase)).(*yak.StatementListContext); ok {
			b.PushTarget(done, nil, nil) // con't fallthrough
			// build default block
			b.CurrentBlock = defaultb
			b.buildStatementList(stmtlist)
			// jump default -> done
			b.EmitJump(done)
			b.PopTarget() // pop target
		}
	}

	b.CurrentBlock = enter
	b.EmitSwitch(cond, defaultb, slabel)
	rest := b.NewBasicBlock("")
	b.CurrentBlock = done
	b.EmitJump(rest)
	b.CurrentBlock = rest
}

// if stmt
func (b *astbuilder) buildIfStmt(stmt *yak.IfStmtContext) {
	var buildIf func(stmt *yak.IfStmtContext) *ssa.IfBuilder
	buildIf = func(stmt *yak.IfStmtContext) *ssa.IfBuilder {
		recoverRange := b.SetRange(stmt.BaseParserRuleContext)
		defer recoverRange()

		i := b.BuildIf()

		// if instruction condition
		i.BuildCondition(
			func() ssa.Value {
				return b.buildExpression(stmt.Expression(0).(*yak.ExpressionContext))
			})
		// build true body
		i.BuildTrue(
			func() {
				if blockstmt, ok := stmt.Block(0).(*yak.BlockContext); ok {
					b.buildBlock(blockstmt)
				}
			},
		)
		// add elif block to prev-if false
		for index := range stmt.AllElif() {
			// build condition
			condstmt, ok := stmt.Expression(index + 1).(*yak.ExpressionContext)
			if !ok {
				continue
			}
			i.BuildElif(
				// condition
				func() ssa.Value {
					recoverRange := b.SetRange(condstmt.BaseParserRuleContext)
					defer recoverRange()
					return b.buildExpression(condstmt)
				},
				// body
				func() {
					recoverRange := b.SetRange(condstmt.BaseParserRuleContext)
					defer recoverRange()
					if blockstmt, ok := stmt.Block(index + 1).(*yak.BlockContext); ok {
						b.buildBlock(blockstmt)
					}
				},
			)
		}

		// handle "else" and "else if "
		elseStmt, ok := stmt.ElseBlock().(*yak.ElseBlockContext)
		if !ok {
			return i
		}
		if elseblock, ok := elseStmt.Block().(*yak.BlockContext); ok {
			i.BuildFalse(
				// create false block
				func() {
					b.buildBlock(elseblock)
				},
			)
		} else if elifstmt, ok := elseStmt.IfStmt().(*yak.IfStmtContext); ok {
			// "else if"
			// create elif block
			i.BuildChild(buildIf(elifstmt))
		}
		return i
	}

	i := buildIf(stmt)
	i.Finish()
}

// block
func (b *astbuilder) buildBlock(stmt *yak.BlockContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	b.CurrentBlock.SetPosition(b.CurrentPos)
	if s, ok := stmt.StatementList().(*yak.StatementListContext); ok {
		b.PushBlockSymbolTable()
		b.buildStatementList(s)
		b.PopBlockSymbolTable()
	} else {
		b.NewError(ssa.Warn, TAG, "empty block")
	}
}

type assignlist interface {
	AssignEq() antlr.TerminalNode
	ColonAssignEq() antlr.TerminalNode
	ExpressionList() yak.IExpressionListContext
	LeftExpressionList() yak.ILeftExpressionListContext
}

func (b *astbuilder) AssignList(stmt assignlist) []ssa.Value {
	// Colon Assign Means: ... create symbol to recv value force
	if op, op2 := stmt.AssignEq(), stmt.ColonAssignEq(); op != nil || op2 != nil {
		// right value
		var rvalues []ssa.Value
		if ri, ok := stmt.ExpressionList().(*yak.ExpressionListContext); ok {
			rvalues = b.buildExpressionList(ri)
		}

		// left
		var lvalues []ssa.LeftValue
		if li, ok := stmt.LeftExpressionList().(*yak.LeftExpressionListContext); ok {
			lvalues = b.buildLeftExpressionList(op2 != nil, li)
		}

		// assign
		// (n) = (n), just assign
		if len(rvalues) == len(lvalues) {
			for i := range rvalues {
				lvalues[i].Assign(rvalues[i], b.FunctionBuilder)
			}
		} else if len(rvalues) == 1 {
			if len(lvalues) == 0 {
				// (0) = (1)
				b.NewError(ssa.Error, TAG, "assign left side is empty")
				return nil
			}

			// (n) = (1)
			inter, ok := rvalues[0].(ssa.User)
			if !ok {
				return nil
			}
			if c, ok := rvalues[0].(*ssa.Call); ok {
				var length int
				// 可以通过是否存在variable确定是函数调用是否存在左值
				c.SetVariable(uuid.NewString())
				if c.GetType().GetTypeKind() != ssa.ObjectTypeKind {
					// b.NewError(ssa.Error, TAG, "assign right side is not interface function call")
					// return nil
					length = len(lvalues)
				} else {
					it := c.GetType().(*ssa.ObjectType)
					length = it.Len
				}
				vs := make([]ssa.Value, 0)
				for i := 0; i < length; i++ {
					field := b.EmitField(c, ssa.NewConst(i))
					vs = append(vs, field)
				}
				if len(vs) == len(lvalues) {
					for i := range vs {

						if inst, ok := vs[i].(ssa.Instruction); ok {
							inst.SetPosition(lvalues[i].GetPosition())
						}

						lvalues[i].Assign(vs[i], b.FunctionBuilder)
					}
				} else {
					b.NewError(ssa.Error, TAG, "multi-assign failed: left value length[%d] != right value length[%d]", len(lvalues), len(rvalues))
				}
				return nil
			}

			// (n) = field(1, #index)
			for i, lv := range lvalues {
				field := b.EmitField(inter, ssa.NewConst(i))
				// if inst, ok := field.(ssa.Instruction); ok {
				// 	inst.SetPosition(lv.GetPosition())
				// }
				lv.Assign(field, b.FunctionBuilder)
			}
		} else if len(lvalues) == 1 {
			if len(rvalues) == 0 {
				// (1) = (0) undefine
				b.NewError(ssa.Error, TAG, "assign right side is empty")
				return nil
			}
			// (1) = (n)
			// (1) = interface(n)
			_interface := b.CreateInterfaceWithVs(nil, rvalues)
			lvalues[0].Assign(_interface, b.FunctionBuilder)
		} else {
			// (n) = (m) && n!=m
			b.NewError(ssa.Error, TAG, "multi-assign failed: left value length[%d] != right value length[%d]", len(lvalues), len(rvalues))
			return nil
		}
		return lo.Map(lvalues, func(lv ssa.LeftValue, _ int) ssa.Value { return lv.GetValue(b.FunctionBuilder) })
	}
	return nil
}

// assign expression
func (b *astbuilder) buildAssignExpression(stmt *yak.AssignExpressionContext) []ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if ret := b.AssignList(stmt); ret != nil {
		return ret
	}

	if stmt.PlusPlus() != nil { // ++
		lvalue := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		if lvalue == nil {
			b.NewError(ssa.Error, TAG, "assign left side is undefine type")
			return nil
		}
		rvalue := b.EmitBinOp(ssa.OpAdd, lvalue.GetValue(b.FunctionBuilder), ssa.NewConst(1))
		lvalue.Assign(rvalue, b.FunctionBuilder)
		return []ssa.Value{lvalue.GetValue(b.FunctionBuilder)}
	} else if stmt.SubSub() != nil { // --
		lvalue := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		if lvalue == nil {
			b.NewError(ssa.Error, TAG, "assign left side is undefine type")
			return nil
		}
		rvalue := b.EmitBinOp(ssa.OpSub, lvalue.GetValue(b.FunctionBuilder), ssa.NewConst(1))
		lvalue.Assign(rvalue, b.FunctionBuilder)
		return []ssa.Value{lvalue.GetValue(b.FunctionBuilder)}
	}

	if op, ok := stmt.InplaceAssignOperator().(*yak.InplaceAssignOperatorContext); ok {
		lvalue := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		if lvalue == nil {
			b.NewError(ssa.Error, TAG, "assign left side is undefine type")
			return nil
		}
		rvalue := b.buildExpression(stmt.Expression().(*yak.ExpressionContext))
		var opcode ssa.BinaryOpcode
		switch op.GetText() {
		case "+=":
			opcode = ssa.OpAdd
		case "-=":
			opcode = ssa.OpSub
		case "*=":
			opcode = ssa.OpMul
		case "/=":
			opcode = ssa.OpDiv
		case "%=":
			opcode = ssa.OpMod
		case "<<=":
			opcode = ssa.OpShl
		case ">>=":
			opcode = ssa.OpShr
		case "&=":
			opcode = ssa.OpAnd
		case "&^=":
			opcode = ssa.OpAndNot
		case "|=":
			opcode = ssa.OpOr
		case "^=":
			opcode = ssa.OpXor
		}
		rvalue = b.EmitBinOp(opcode, lvalue.GetValue(b.FunctionBuilder), rvalue)
		lvalue.Assign(rvalue, b.FunctionBuilder)
		return []ssa.Value{lvalue.GetValue(b.FunctionBuilder)}
	}
	return nil
}

// declare variable expression
func (b *astbuilder) buildDeclareVariableExpressionStmt(stmt *yak.DeclareVariableExpressionStmtContext) {
	// recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	// defer recoverRange()
	if s, ok := stmt.DeclareVariableExpression().(*yak.DeclareVariableExpressionContext); ok {
		b.buildDeclareVariableExpression(s)
	}
}

func (b *astbuilder) buildDeclareVariableExpression(stmt *yak.DeclareVariableExpressionContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.DeclareVariableOnly().(*yak.DeclareVariableOnlyContext); ok {
		b.buildDeclareVariableOnly(s)
	}
	if s, ok := stmt.DeclareAndAssignExpression().(*yak.DeclareAndAssignExpressionContext); ok {
		b.buildDeclareAndAssignExpression(s)
	}
}

func (b *astbuilder) buildDeclareVariableOnly(stmt *yak.DeclareVariableOnlyContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	for _, idstmt := range stmt.AllIdentifier() {
		id := idstmt.GetText()
		b.WriteVariable(id, ssa.NewAny())
	}
}

func (b *astbuilder) buildDeclareAndAssignExpression(stmt *yak.DeclareAndAssignExpressionContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	b.AssignList(stmt)
}

// left expression list
func (b *astbuilder) buildLeftExpressionList(forceAssign bool, stmt *yak.LeftExpressionListContext) []ssa.LeftValue {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	exprs := stmt.AllLeftExpression()
	valueLen := len(exprs)
	values := make([]ssa.LeftValue, 0, valueLen)
	for _, e := range exprs {
		if e, ok := e.(*yak.LeftExpressionContext); ok {
			if v := b.buildLeftExpression(forceAssign, e); !utils.IsNil(v) {
				values = append(values, v)
			}
		}
	}
	return values
}

// left  expression
func (b *astbuilder) buildLeftExpression(forceAssign bool, stmt *yak.LeftExpressionContext) ssa.LeftValue {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	if s := stmt.Identifier(); s != nil {
		text := s.GetText()
		if text == "_" {
			return ssa.NewIdentifierLV("_", b.CurrentPos)
		}
		if forceAssign {
			text = b.MapBlockSymbolTable(text)
		} else if v := b.ReadVariable(text, false); v != nil {
			// when v exist
			switch value := v.(type) {
			// case *ssa.Field:
			// 	if value.OutCapture {
			// 		return value
			// 	}
			case *ssa.Parameter:
				if value.IsFreeValue {
					field := b.NewCaptureField(text)
					var tmp ssa.Value = field
					ssa.ReplaceValue(v, tmp)
					if index := slices.Index(b.FreeValues, v); index != -1 {
						b.FreeValues[index] = tmp
					}
					b.SetReg(field)
					b.ReplaceVariable(text, value, field)
					return field
				}
			default:
			}
			// } else if b.CanBuildFreeValue(text) {
			// 	field := b.NewCaptureField(text)
			// 	b.FreeValues = append(b.FreeValues, field)
			// 	b.SetReg(field)
			// 	return field
		}
		return ssa.NewIdentifierLV(text, b.CurrentPos)
	}
	if s, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		expr := b.buildExpression(s)
		if expr == nil {
			b.NewError(ssa.Error, TAG, "left expression expression is nil")
			return nil
		}
		//TODO: check interface type
		var inter ssa.User
		if expr, ok := expr.(ssa.User); ok {
			inter = expr
		} else {
			b.NewError(ssa.Error, TAG, "left expression is not interface")
			return nil
		}

		if s, ok := stmt.LeftSliceCall().(*yak.LeftSliceCallContext); ok {
			index := b.buildLeftSliceCall(s)
			return b.EmitFieldMust(inter, index)
		}

		if s, ok := stmt.LeftMemberCall().(*yak.LeftMemberCallContext); ok {
			if id := s.Identifier(); id != nil {
				idText := id.GetText()
				return b.EmitFieldMust(inter, ssa.NewConst(idText))
			} else if id := s.IdentifierWithDollar(); id != nil {
				key := b.ReadVariable(id.GetText()[1:], true)
				if key == nil {
					b.NewError(ssa.Error, TAG, "Expression: %s is not a variable", id.GetText())
					return nil
				}
				return b.EmitFieldMust(inter, key)
			}
		}
	}
	return nil
}

// left slice call
func (b *astbuilder) buildLeftSliceCall(stmt *yak.LeftSliceCallContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	if s, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		return b.buildExpression(s)
	}
	return nil
}

// expression
func (b *astbuilder) buildExpression(stmt *yak.ExpressionContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	getValue := func(index int) ssa.Value {
		if s, ok := stmt.Expression(index).(*yak.ExpressionContext); ok {
			return b.buildExpression(s)
		}
		return nil
	}

	// typeLiteral expression
	if s, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
		if stmt.LParen() != nil && stmt.RParen() != nil {
			v := getValue(0)
			if v == nil {
				//TODO:  int() => type-cast [number] undefine-""
				v = b.EmitUndefine("")
			}
			typ := b.buildTypeLiteral(s)
			return b.EmitTypeCast(v, typ)
		}
	}

	// literal
	if s, ok := stmt.Literal().(*yak.LiteralContext); ok {
		return b.buildLiteral(s)
	}

	// anonymous function decl
	if s, ok := stmt.AnonymousFunctionDecl().(*yak.AnonymousFunctionDeclContext); ok {
		return b.buildAnonymousFunctionDecl(s)
	}
	// panic
	if s := stmt.Panic(); s != nil {
		b.EmitPanic(getValue(0))
		return nil
	}

	// RECOVER
	if s := stmt.Recover(); s != nil {
		return b.EmitRecover()
	}

	// identifier
	if s := stmt.Identifier(); s != nil { // 解析变量
		text := s.GetText()
		if text == "_" {
			b.NewError(ssa.Error, TAG, "cannot use _ as value")
			return nil
		}
		return b.ReadVariable(text, true)
	}

	// member call
	if s, ok := stmt.MemberCall().(*yak.MemberCallContext); ok {
		value := getValue(0)
		var inter ssa.User
		// inter, ok := value.(*ssa.Interface)
		// inter, ok := getValue(0).(*ssa.Interface)
		// if !ok {
		// 	b.NewError(ssa.Error, TAG, "Expression: need a interface")
		// 	// return nil
		if user, ok := value.(ssa.User); ok {
			inter = user
		} else {
			if v, ok := value.(*ssa.Const); ok {
				inter = b.EmitConstInst(v)
			} else {
				b.NewError(ssa.Error, TAG, "member call target Error")
				// return nil
				{
					expr := stmt.Expression(0).(*yak.ExpressionContext)
					recoverRange := b.SetRange(expr.BaseParserRuleContext)
					text := expr.GetText()
					inter = b.EmitUndefine(text)
					recoverRange()
				}
			}
		}

		if id := s.Identifier(); id != nil {
			idText := id.GetText()
			return b.EmitField(inter, ssa.NewConst(idText))
		} else if id := s.IdentifierWithDollar(); id != nil {
			key := b.ReadVariable(id.GetText()[1:], true)
			if key == nil {
				b.NewError(ssa.Error, TAG, "Expression: %s is not a variable", id.GetText())
				return nil
			}
			return b.EmitField(inter, key)
		}
	}

	// slice call
	if s, ok := stmt.SliceCall().(*yak.SliceCallContext); ok {
		expr, ok := getValue(0).(ssa.User)
		if !ok {
			b.NewError(ssa.Error, TAG, "Expression: need a interface")
			return nil
		}
		keys := b.buildSliceCall(s)
		if len(keys) == 1 {
			return b.EmitField(expr, keys[0])
		} else if len(keys) == 2 {
			return b.EmitMakeSlice(expr, keys[0], keys[1], nil)
		} else if len(keys) == 3 {
			return b.EmitMakeSlice(expr, keys[0], keys[1], keys[2])
		} else {
			b.NewError(ssa.Error, TAG, "slice call expression argument too much")
		}
	}

	// function call
	if s, ok := stmt.FunctionCall().(*yak.FunctionCallContext); ok {
		return b.EmitCall(b.buildFunctionCallWarp(stmt, s))
	}

	// paren expression
	if s, ok := stmt.ParenExpression().(*yak.ParenExpressionContext); ok {
		if e, ok := s.Expression().(*yak.ExpressionContext); ok {
			return b.buildExpression(e)
		} else {
			return b.EmitUndefine("")
		}
	}

	// instance code
	if s, ok := stmt.InstanceCode().(*yak.InstanceCodeContext); ok {
		return b.EmitCall(b.buildInstanceCode(s))
	}

	// make expression
	if s, ok := stmt.MakeExpression().(*yak.MakeExpressionContext); ok {
		return b.buildMakeExpression(s)
	}

	// unary operator expression
	if s, ok := stmt.UnaryOperator().(*yak.UnaryOperatorContext); ok {
		x := getValue(0)
		var opcode ssa.UnaryOpcode
		switch s.GetText() {
		case "!":
			opcode = ssa.OpNot
		case "+":
			opcode = ssa.OpPlus
		case "-":
			opcode = ssa.OpNeg
		case "<-":
			opcode = ssa.OpChan
		case "^":
			opcode = ssa.OpBitwiseNot
		default:
			b.NewError(ssa.Error, TAG, "unary operator not support: %s", s.GetText())
			return nil
		}
		return b.EmitUnOp(opcode, x)
	}

	// 二元运算（位运算全面优先于数字运算，数字运算全面优先于高级逻辑运算）
	// | expression bitBinaryOperator ws* expression

	// // 普通数学运算 done
	// | expression multiplicativeBinaryOperator ws* expression
	// | expression additiveBinaryOperator ws* expression
	// | expression comparisonBinaryOperator ws* expression

	type op interface {
		GetText() string
	}
	getBinaryOp := func() op {
		if op := stmt.BitBinaryOperator(); op != nil {
			return op
		}
		if op := stmt.AdditiveBinaryOperator(); op != nil {
			return op
		}
		if op := stmt.MultiplicativeBinaryOperator(); op != nil {
			return op
		}
		if op := stmt.ComparisonBinaryOperator(); op != nil {
			return op
		}
		return nil
	}

	if op := getBinaryOp(); op != nil {
		op0 := getValue(0)
		op1 := getValue(1)
		if op0 == nil || op1 == nil {
			b.NewError(ssa.Error, TAG, "additive binary operator need two expression")
			return nil
		}
		var opcode ssa.BinaryOpcode
		switch op.GetText() {
		// BitBinaryOperator
		case "<<":
			opcode = ssa.OpShl
		case ">>":
			opcode = ssa.OpShr
		case "&":
			opcode = ssa.OpAnd
		case "&^":
			opcode = ssa.OpAndNot
		case "|":
			opcode = ssa.OpOr
		case "^":
			opcode = ssa.OpXor

		// AdditiveBinaryOperator
		case "+":
			opcode = ssa.OpAdd
		case "-":
			opcode = ssa.OpSub

		// MultiplicativeBinaryOperator
		case "*":
			opcode = ssa.OpMul
		case "/":
			opcode = ssa.OpDiv
		case "%":
			opcode = ssa.OpMod

		// ComparisonBinaryOperator
		case ">":
			opcode = ssa.OpGt
		case "<":
			opcode = ssa.OpLt
		case "<=":
			opcode = ssa.OpLtEq
		case ">=":
			opcode = ssa.OpGtEq
		case "!=", "<>":
			opcode = ssa.OpNotEq
		case "==":
			opcode = ssa.OpEq
		default:
			b.NewError(ssa.Error, TAG, "binary operator not support: %s", op.GetText())
			return nil
		}
		return b.EmitBinOp(opcode, op0, op1)
	}

	// | expression '<-' expression
	if stmt.ChanIn() == nil {
		op1, op2 := getValue(0), getValue(1)
		if u, ok := op1.(ssa.User); ok {
			return b.EmitUpdate(u, op2)
		} else {
			b.NewError(ssa.Error, TAG, "left of <- must be a chan variable")
			return nil
		}
	}

	// | expression 'not'? 'in' expression
	if s := stmt.In(); s != nil {
		op1, op2 := getValue(0), getValue(1)
		if op1 == nil || op2 == nil {
			b.NewError(ssa.Error, TAG, "in operator need two expression")
			return nil
		}
		res := b.EmitBinOp(ssa.OpIn, op1, op2)
		if stmt.NotLiteral() != nil {
			res = b.EmitUnOp(ssa.OpNot, res)
		}
		return res
	}

	/*
		expression:
			c = t0, t1, t2
		cfg:
			enter:
				t0 ...
				if [t0] true-> if.true; false -> if.false
			if.true:
				t1 ...
				jump if.done
			if.false:
				t2 ...
				jump if.done
			if.done
				c = phi[....]

		ast statement:
			c = a || b
				t0 = !a; t1 = b
				c = phi[a enter; b if.true]

			c = a && b
				t0 = a; t1 = b
				c = phi[a enter; b if.true]

			c = cond ? a : b
				t0 = cond; t1 = a; t2 = b
				c = phi[a if.true; b if.false]
	*/
	handlerJumpExpression := func(cond func(string) ssa.Value, trueExpr, falseExpr func() ssa.Value) ssa.Value {
		// 为了聚合产生Phi指令
		id := uuid.NewString()
		// 只需要使用b.WriteValue设置value到此ID，并最后调用b.ReadValue可聚合产生Phi指令，完成语句预期行为
		ifb := b.BuildIf()
		ifb.BuildCondition(
			func() ssa.Value {
				// 在上层函数中决定是否设置id, 在三元运算符时不会将condition加入结果中
				return cond(id)
			})
		ifb.BuildTrue(
			func() {
				v := trueExpr()
				b.WriteVariable(id, v)
			},
		)
		if falseExpr != nil {
			ifb.BuildFalse(func() {
				v := falseExpr()
				b.WriteVariable(id, v)
			})
		}
		ifb.Finish()
		// generator phi instruction
		return b.ReadVariable(id, true)
	}

	// | expression '&&' ws* expression
	if s := stmt.LogicAnd(); s != nil {
		return handlerJumpExpression(
			func(id string) ssa.Value {
				v := getValue(0)
				b.WriteVariable(id, v)
				return v
			},
			func() ssa.Value {
				return getValue(1)
			},
			nil,
		)
	}

	// | expression '||' ws* expression
	if s := stmt.LogicOr(); s != nil {
		return handlerJumpExpression(
			func(id string) ssa.Value {
				v := getValue(0)
				b.WriteVariable(id, v)
				return b.EmitUnOp(ssa.OpNot, v)
			},
			func() ssa.Value {
				return getValue(1)
			},
			nil,
		)
	}

	// | expression '?' ws* expression ws* ':' ws* expression
	if s := stmt.Question(); s != nil {
		return handlerJumpExpression(
			func(_ string) ssa.Value {
				return getValue(0)
			},
			func() ssa.Value {
				return getValue(1)
			},
			func() ssa.Value {
				return getValue(2)
			},
		)
	}

	return nil
}

// paren expression

// make expression
func (b *astbuilder) buildMakeExpression(stmt *yak.MakeExpressionContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	var typ ssa.Type
	if s, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
		typ = b.buildTypeLiteral(s)
	}
	if typ == nil {
		b.NewError(ssa.Error, TAG, "not set type in make expression")
		return nil
	}

	var exprs []ssa.Value
	if s, ok := stmt.ExpressionListMultiline().(*yak.ExpressionListMultilineContext); ok {
		exprs = b.buildExpressionListMultiline(s)
	}
	zero := ssa.NewConst(0)
	switch typ := typ.(type) {
	case *ssa.ObjectType:
		switch typ.Kind {
		case ssa.Slice:
			if len(exprs) == 0 {
				return b.EmitMakeBuildWithType(typ, zero, zero)
			} else if len(exprs) == 1 {
				return b.EmitMakeBuildWithType(typ, exprs[0], exprs[0])
			} else if len(exprs) == 2 {
				return b.EmitMakeBuildWithType(typ, exprs[0], exprs[1])
			} else {
				b.NewError(ssa.Error, TAG, "make slice expression argument too much!")
			}
		case ssa.Map:
			return b.EmitMakeBuildWithType(typ, zero, zero)
		case ssa.Struct:
		}
	case *ssa.ChanType:
		if len(exprs) == 0 {
			return b.EmitMakeBuildWithType(typ, zero, zero)
		} else {
			return b.EmitMakeBuildWithType(typ, exprs[0], exprs[0])
		}
	default:
		b.NewError(ssa.Error, TAG, "make unknown type")
	}
	return nil
}

// instance code
func (b *astbuilder) buildInstanceCode(stmt *yak.InstanceCodeContext) *ssa.Call {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	newfunc := b.Package.NewFunctionWithParent("", b.Function)
	buildFunc := func() {
		b.FunctionBuilder = b.PushFunction(newfunc)

		if block, ok := stmt.Block().(*yak.BlockContext); ok {
			b.buildBlock(block)
		}

		b.Finish()
		b.FunctionBuilder = b.PopFunction()
	}
	b.AddSubFunction(buildFunc)

	return b.NewCall(newfunc, nil)
}

// anonymous function decl
func (b *astbuilder) buildAnonymousFunctionDecl(stmt *yak.AnonymousFunctionDeclContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	funcName := ""
	if name := stmt.FunctionNameDecl(); name != nil {
		funcName = name.GetText()
	}
	newfunc := b.Package.NewFunctionWithParent(funcName, b.Function)
	buildFunc := func() {
		b.FunctionBuilder = b.PushFunction(newfunc)

		if stmt.EqGt() != nil {
			if stmt.LParen() != nil && stmt.RParen() != nil {
				// has param
				// stmt FunctionParamDecl()
				if para, ok := stmt.FunctionParamDecl().(*yak.FunctionParamDeclContext); ok {
					b.buildFunctionParamDecl(para)
				}
			} else {
				// only this param
				b.NewParam(stmt.Identifier().GetText())
			}
			if block, ok := stmt.Block().(*yak.BlockContext); ok {
				// build block
				b.buildBlock(block)
			} else if expression, ok := stmt.Expression().(*yak.ExpressionContext); ok {
				// handler expression
				v := b.buildExpression(expression)
				b.EmitReturn([]ssa.Value{v})
			} else {
				b.NewError(ssa.Error, TAG, "BUG: arrow function need expression or block at least")
			}
		} else {
			// this global function
			if para, ok := stmt.FunctionParamDecl().(*yak.FunctionParamDeclContext); ok {
				b.buildFunctionParamDecl(para)
			}
			if block, ok := stmt.Block().(*yak.BlockContext); ok {
				b.buildBlock(block)
			}
		}
		b.Finish()
		b.FunctionBuilder = b.PopFunction()
	}
	b.AddSubFunction(buildFunc)

	if funcName != "" {
		b.WriteVariable(funcName, newfunc)
	}
	return newfunc
}

// function param decl
func (b *astbuilder) buildFunctionParamDecl(stmt *yak.FunctionParamDeclContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	ellipsis := stmt.Ellipsis() // if has "...",  use array pass this argument
	ids := stmt.AllIdentifier()

	for _, id := range ids {
		b.NewParam(id.GetText())
	}
	if ellipsis != nil {
		// handler "..." to array
		b.HandlerEllipsis()
	}
}

func (b *astbuilder) buildFunctionCallWarp(exprstmt *yak.ExpressionContext, stmt *yak.FunctionCallContext) *ssa.Call {
	if expr, ok := exprstmt.Expression(0).(*yak.ExpressionContext); ok {
		v := b.buildExpression(expr)
		if v != nil {
			return b.buildFunctionCall(stmt, v)
		}
	}
	b.NewError(ssa.Error, TAG, "call target is nil")
	return nil
}

// function call
func (b *astbuilder) buildFunctionCall(stmt *yak.FunctionCallContext, v ssa.Value) *ssa.Call {
	// recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	// defer recoverRange()
	var args []ssa.Value
	isEllipsis := false
	if s, ok := stmt.OrdinaryArguments().(*yak.OrdinaryArgumentsContext); ok {
		args, isEllipsis = b.buildOrdinaryArguments(s)
	}
	c := b.NewCall(v, args)
	if stmt.Wavy() != nil {
		c.IsDropError = true
	}
	if isEllipsis {
		c.IsEllipsis = true
	}
	return c
}

// ordinary argument
func (b *astbuilder) buildOrdinaryArguments(stmt *yak.OrdinaryArgumentsContext) (v []ssa.Value, hasEll bool) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	ellipsis := stmt.Ellipsis()
	allExprs := stmt.AllExpression()
	// v := make([]ssa.Value, 0, len(allExprs))
	for _, expr := range allExprs {
		v = append(v, b.buildExpression(expr.(*yak.ExpressionContext)))
	}
	if ellipsis != nil {
		hasEll = true
	}
	return
}

// slice call
func (b *astbuilder) buildSliceCall(stmt *yak.SliceCallContext) []ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	exprLen := len(stmt.AllColon()) + 1
	exprs := stmt.AllExpression()
	values := make([]ssa.Value, exprLen)
	if len(exprs) == 0 {
		b.NewError(ssa.Error, TAG, "slice call expression is zero")
		return nil
	}
	if len(exprs) > 3 {
		b.NewError(ssa.Error, TAG, "slice call expression too much")
		return nil
	}
	for i, expr := range exprs {
		if s, ok := expr.(*yak.ExpressionContext); ok {
			values[i] = b.buildExpression(s)
		} else {
			values[i] = ssa.NewConst(0)
		}
	}
	return values
}

// expression list
func (b *astbuilder) buildExpressionList(stmt *yak.ExpressionListContext) []ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	exprs := stmt.AllExpression()
	valueLen := len(exprs)
	values := make([]ssa.Value, 0, valueLen)
	for _, e := range exprs {
		if e, ok := e.(*yak.ExpressionContext); ok {
			if v := b.buildExpression(e); !utils.IsNil(v) {
				values = append(values, v)
			}
		}
	}
	return values
}

// expression list multiline
func (b *astbuilder) buildExpressionListMultiline(stmt *yak.ExpressionListMultilineContext) []ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	allExprs := stmt.AllExpression()
	exprs := make([]ssa.Value, 0, len(allExprs))
	for _, expr := range allExprs {
		if expr, ok := expr.(*yak.ExpressionContext); ok {
			exprs = append(exprs, b.buildExpression(expr))
		}
	}
	return exprs
}
