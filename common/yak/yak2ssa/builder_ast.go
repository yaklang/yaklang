package yak2ssa

import (
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *astbuilder) handlerWs(ws *yak.WsContext) {
	recoverRange := b.SetRange(ws.BaseParserRuleContext)
	defer recoverRange()
	for _, line := range ws.AllLINE_COMMENT() {
		token := line.GetSymbol()
		if err := b.AddErrorComment(line.GetText(), token.GetLine()); err != nil {
			b.NewErrorWithPos(ssa.Warn, TAG, b.CurrentRange, err.Error())
		}
	}
}

// entry point
func (b *astbuilder) build(raw ssa.FrontAST) {
	if utils.IsNil(raw) {
		return
	}
	ast, ok := raw.(*yak.ProgramContext)
	if !ok {
		return
	}

	prog := b.GetProgram()
	currentEditor := prog.GetCurrentEditor()
	hasFile := func(p *ssa.Program) bool {
		if hash, ok := p.FileList[currentEditor.GetUrl()]; ok {
			if hash == currentEditor.GetIrSourceHash() {
				return true
			}
		}
		return false
	}
	skip := hasFile(prog)
	if skip {
		return
	}

	// if prog.ProgramKind == ssa.Application {
	// 	prog = prog.GetSubProgram(currentEditor.GetUrl())
	// 	b.FunctionBuilder = prog.GetAndCreateFunctionBuilder(currentEditor.GetUrl(), string(ssa.MainFunctionName))
	// }

	for _, ws := range ast.AllWs() {
		b.handlerWs(ws.(*yak.WsContext))
	}
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	b.Function.SetRange(b.CurrentRange)
	if stmt, ok := ast.StatementList().(*yak.StatementListContext); ok {
		b.buildStatementList(stmt)
	}
}

// statement list
func (b *astbuilder) buildStatementList(stmtlist *yak.StatementListContext) {
	recoverRange := b.SetRange(stmtlist.BaseParserRuleContext)
	defer recoverRange()
	allstmt := stmtlist.AllStatement()
	for _, stmt := range allstmt {
		if b.IsStop() {
			return
		}
		if stmt, ok := stmt.(*yak.StatementContext); ok {
			b.buildStatement(stmt)
		}
	}
}

func (b *astbuilder) buildLineComment(stmt *yak.LineCommentStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	if line := stmt.LINE_COMMENT(); line != nil {
		if err := b.AddErrorComment(line.GetText(), line.GetSymbol().GetLine()); err != nil {
			b.NewErrorWithPos(ssa.Warn, TAG, b.CurrentRange, err.Error())
		}
	}
}

func (b *astbuilder) buildEmpty(stmt *yak.EmptyContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if ws, ok := stmt.Ws().(*yak.WsContext); ok {
		b.handlerWs(ws)
	}
}

func (b *astbuilder) buildStatement(stmt *yak.StatementContext) {
	if b.IsBlockFinish() {
		return
	}
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	b.AppendBlockRange()
	if s, ok := stmt.LineCommentStmt().(*yak.LineCommentStmtContext); ok {
		b.buildLineComment(s)
	}
	if s, ok := stmt.Empty().(*yak.EmptyContext); ok {
		b.buildEmpty(s)
	}
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
		b.buildBlock(s, true)
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

	// return stmt
	if s, ok := stmt.ReturnStmt().(*yak.ReturnStmtContext); ok {
		b.buildReturnStmt(s)
		return
	}
	// break stmt
	if _, ok := stmt.BreakStmt().(*yak.BreakStmtContext); ok {
		if !b.Break() {
			b.NewError(ssa.Error, TAG, UnexpectedBreakStmt())
		}
		return
	}
	// continue stmt
	if _, ok := stmt.ContinueStmt().(*yak.ContinueStmtContext); ok {
		if !b.Continue() {
			b.NewError(ssa.Error, TAG, UnexpectedContinueStmt())
		}
		return
	}

	if _, ok := stmt.FallthroughStmt().(*yak.FallthroughStmtContext); ok {
		if !b.Fallthrough() {
			b.NewError(ssa.Error, TAG, UnexpectedFallthroughStmt())
		}
		return
	}
	// TODO: include stmt and check file path
	if s, ok := stmt.IncludeStmt().(*yak.IncludeStmtContext); ok {
		b.buildInclude(s)
		return
	}

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
		b.NewError(ssa.Error, TAG, UnexpectedAssertStmt())
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
	revcoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer revcoverRange()

	tryBuilder := b.BuildTry()

	tryBuilder.BuildTryBlock(func() {
		if s, ok := stmt.Block(0).(*yak.BlockContext); ok {
			b.buildBlock(s)
		}
	})

	tryBuilder.BuildErrorCatch(func() string {
		var id string
		if i := stmt.Identifier(); i != nil {
			id = i.GetText()
		}
		return id
	}, func() {
		if s, ok := stmt.Block(1).(*yak.BlockContext); ok {
			b.buildBlock(s)
		}
	})

	if s, ok := stmt.Block(2).(*yak.BlockContext); ok {
		tryBuilder.BuildFinally(func() {
			b.buildBlock(s)
		})
	}

	tryBuilder.Finish()
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

func (b *astbuilder) buildCallExpr(stmt *yak.CallExprContext) (c *ssa.Call) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if funcCallExprStmt, ok := stmt.FunctionCallExpr().(*yak.FunctionCallExprContext); ok {
		v := b.buildExpression(funcCallExprStmt.Expression().(*yak.ExpressionContext))
		c = b.buildFunctionCall(funcCallExprStmt.FunctionCall().(*yak.FunctionCallContext), v)
	} else if instanceCodeStmt, ok := stmt.InstanceCode().(*yak.InstanceCodeContext); ok {
		c = b.buildInstanceCode(instanceCodeStmt)
	}
	return c
}

// defer stmt
func (b *astbuilder) buildDeferStmt(stmt *yak.DeferStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var i ssa.Instruction
	var alreadyEmit bool
	if s, ok := stmt.CallExpr().(*yak.CallExprContext); ok {
		i = b.buildCallExpr(s)
		alreadyEmit = true
	} else if s, ok := stmt.RecoverStmt().(*yak.RecoverStmtContext); ok {
		i = b.buildRecoverStmt(s)
	} else if s, ok := stmt.PanicStmt().(*yak.PanicStmtContext); ok {
		i = b.buildPanicStmt(s)
	}
	if i != nil {
		if !alreadyEmit {
			b.SetInstructionPosition(i)
		}
		b.EmitDefer(i)
	}
}

// go stmt
func (b *astbuilder) buildGoStmt(stmt *yak.GoStmtContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var c *ssa.Call
	if stmt, ok := stmt.CallExpr().(*yak.CallExprContext); ok {
		c = b.buildCallExpr(stmt)
	}
	c.Async = true
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
	loop := b.CreateLoopBuilder()

	// var cond ssa.Value
	var cond *yak.ExpressionContext
	if e, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		// if only expression; just build expression in header;
		cond = e
	} else if condition, ok := stmt.ForStmtCond().(*yak.ForStmtCondContext); ok {
		if first, ok := condition.ForFirstExpr().(*yak.ForFirstExprContext); ok {
			// first expression is initialization, in enter block
			loop.SetFirst(func() []ssa.Value {
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
			loop.SetThird(func() []ssa.Value {
				// build third expression in loop.latch
				recoverRange := b.SetRange(third.BaseParserRuleContext)
				defer recoverRange()
				return b.ForExpr(third)
			})
		}
	}

	loop.SetCondition(func() ssa.Value {
		var condition ssa.Value
		if utils.IsNil(cond) {
			condition = b.EmitConstInst(true)
		} else {
			// recoverRange := b.SetRange(cond.BaseParserRuleContext)
			// defer recoverRange()
			condition = b.buildExpression(cond)
		}
		if utils.IsNil(condition) {
			condition = b.EmitConstInst(true)
			// b.NewError(ssa.Warn, TAG, "loop condition expression is nil, default is true")
		}
		return condition
	})

	//  build body
	loop.SetBody(func() {
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
	loop := b.CreateLoopBuilder()
	var value ssa.Value
	loop.SetFirst(func() []ssa.Value {
		value = b.buildExpression(stmt.Expression().(*yak.ExpressionContext))
		return []ssa.Value{value}
	})

	loop.SetCondition(func() ssa.Value {
		var lefts []*ssa.Variable
		if leftList, ok := stmt.LeftExpressionList().(*yak.LeftExpressionListContext); ok {
			lefts = b.buildLeftExpressionList(true, leftList)
			// } else {
		}
		key, field, ok := b.EmitNext(value, stmt.In() != nil)
		if len(lefts) == 1 {
			b.AssignVariable(lefts[0], key)
			ssa.DeleteInst(field)
		} else if len(lefts) >= 2 {
			if value.GetType().GetTypeKind() == ssa.ChanTypeKind {
				b.NewError(ssa.Error, TAG, InvalidChanType(value.GetType().String()))

				b.AssignVariable(lefts[0], key)
				ssa.DeleteInst(field)
			} else {
				b.AssignVariable(lefts[0], key)
				b.AssignVariable(lefts[1], field)
			}
		}
		if utils.IsNil(ok) {
			ok = b.EmitConstInst(true)
			// b.NewError(ssa.Warn, TAG, "loop condition expression is nil, default is true")
		}
		return ok
	})

	loop.SetBody(func() {
		b.buildBlock(stmt.Block().(*yak.BlockContext))
	})
	loop.Finish()
}

// switch stmt
func (b *astbuilder) buildSwitchStmt(stmt *yak.SwitchStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	SwitchBuilder := b.BuildSwitch()
	SwitchBuilder.AutoBreak = true

	//  parse expression
	var cond ssa.Value
	if expr, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		SwitchBuilder.BuildCondition(func() ssa.Value {
			cond = b.buildExpression(expr)
			return cond
		})
	} else {
		// expression is nil
		recoverRange := b.SetRangeFromTerminalNode(stmt.Switch())
		b.NewError(ssa.Warn, TAG, "switch expression is nil")
		recoverRange()
	}

	allcase := stmt.AllCase()

	SwitchBuilder.BuildCaseSize(len(allcase))
	SwitchBuilder.SetCase(func(i int) []ssa.Value {
		if exprList, ok := stmt.ExpressionList(i).(*yak.ExpressionListContext); ok {
			return b.buildExpressionList(exprList)
		}
		return nil
	})
	SwitchBuilder.BuildBody(func(i int) {
		if stmtList, ok := stmt.StatementList(i).(*yak.StatementListContext); ok {
			b.buildStatementList(stmtList)
		}
	})

	// default
	if stmt.Default() != nil {
		if stmtlist, ok := stmt.StatementList(len(allcase)).(*yak.StatementListContext); ok {
			SwitchBuilder.BuildDefault(func() {
				b.buildStatementList(stmtlist)
			})
		}
	}

	SwitchBuilder.Finish()
}

// if stmt
func (b *astbuilder) buildIfStmt(stmt *yak.IfStmtContext) {
	builder := b.CreateIfBuilder()

	var build func(stmt *yak.IfStmtContext) func()
	build = func(stmt *yak.IfStmtContext) func() {
		// for index := range stmt.AllElif() {
		// }
		// for index :=0; index < len(stmt.AllExpression())
		for index, expression := range stmt.AllExpression() {
			builder.AppendItem(
				func() ssa.Value {
					expressionStmt, ok := expression.(*yak.ExpressionContext)
					if !ok {
						return nil
					}

					recoverRange := b.SetRange(expressionStmt.BaseParserRuleContext)
					b.AppendBlockRange()
					recoverRange()

					return b.buildExpression(expressionStmt)
				},
				func() {
					b.buildBlock(stmt.Block(index).(*yak.BlockContext))
				},
			)
		}

		elseStmt, ok := stmt.ElseBlock().(*yak.ElseBlockContext)
		if !ok {
			return nil
		}
		if elseBlock, ok := elseStmt.Block().(*yak.BlockContext); ok {
			return func() {
				b.buildBlock(elseBlock)
			}
		} else if elifstmt, ok := elseStmt.IfStmt().(*yak.IfStmtContext); ok {
			// "else if"
			// create elif block
			build := build(elifstmt)
			return build
		} else {
			return nil
		}
	}

	elseBlock := build(stmt)
	builder.SetElse(elseBlock)
	builder.Build()
}

// recover
func (b *astbuilder) buildRecoverStmt(stmt *yak.RecoverStmtContext) ssa.Value {
	if s := stmt.Recover(); s != nil {
		return b.EmitRecover()
	}
	return nil
}

// panic
func (b *astbuilder) buildPanicStmt(stmt *yak.PanicStmtContext) ssa.Value {
	if s := stmt.Panic(); s != nil {
		if expr, ok := stmt.Expression().(*yak.ExpressionContext); ok {
			value := b.buildExpression(expr)
			return b.EmitPanic(value)
		}
	}
	return nil
}

// block
func (b *astbuilder) buildBlock(stmt *yak.BlockContext, syntaxBlocks ...bool) {
	syntaxBlock := false
	if len(syntaxBlocks) > 0 {
		syntaxBlock = syntaxBlocks[0]
	}
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	// b.CurrentBlock.SetRange(b.CurrentRange)
	s, ok := stmt.StatementList().(*yak.StatementListContext)
	if !ok {
		b.NewError(ssa.Warn, TAG, "empty block")
		return
	}
	var notEmpty bool
	for _, st := range s.AllStatement() {
		insideStmt := st.(*yak.StatementContext)
		// 忽略开头和结尾的empty
		if insideStmt.Empty() == nil {
			notEmpty = true
			break
		}
	}
	if !notEmpty {
		b.NewError(ssa.Warn, TAG, "empty block")
		return
	}
	if syntaxBlock {
		b.BuildSyntaxBlock(func() {
			b.buildStatementList(s)
		})
	} else {
		b.buildStatementList(s)
	}
}

type assignlist interface {
	AssignEq() antlr.TerminalNode
	ColonAssignEq() antlr.TerminalNode
	ExpressionList() yak.IExpressionListContext
	LeftExpressionList() yak.ILeftExpressionListContext
}

func (b *astbuilder) AssignList(forceAssign bool, stmt assignlist) []ssa.Value {
	// Colon Assign Means: ... create symbol to recv value force
	if op, op2 := stmt.AssignEq(), stmt.ColonAssignEq(); op != nil || op2 != nil {
		var leftVariables []*ssa.Variable
		var rightValue []ssa.Value

		markInformation := func() func() {
			if len(leftVariables) != 1 {
				return func() {}
			}
			left := leftVariables[0]
			ri := stmt.ExpressionList().(*yak.ExpressionListContext)
			if ri != nil && len(ri.AllExpression()) != 1 {
				return func() {}
			}
			right := ri.Expression(0).(*yak.ExpressionContext)

			// is function define
			if _, ok := right.AnonymousFunctionDecl().(*yak.AnonymousFunctionDeclContext); ok {
				recoverFunc := b.SetMarkedFunction(leftVariables[0].GetName())
				return recoverFunc
			}

			{ // is object define

				literal, ok := right.Literal().(*yak.LiteralContext)
				if !ok {
					return func() {}
				}
				_, isMapLiteral := literal.MapLiteral().(*yak.MapLiteralContext)
				_, isSliceLiteral := literal.SliceLiteral().(*yak.SliceLiteralContext)
				_, isSliceTypeLiteral := literal.SliceTypedLiteral().(*yak.SliceTypedLiteralContext)
				if isMapLiteral || isSliceLiteral || isSliceTypeLiteral {
					origin := b.MarkedVariable
					b.MarkedVariable = left
					return func() {
						b.MarkedVariable = origin
					}
				}
			}
			return func() {}
		}

		GetCallField := func(c *ssa.Call, lvs []*ssa.Variable) {
			length := 1
			// 可以通过是否存在variable确定是函数调用是否存在左值
			c.SetName(uuid.NewString())
			c.Unpack = true
			if it, ok := ssa.ToObjectType(c.GetType()); c.GetType().GetTypeKind() == ssa.TupleTypeKind && ok {
				length = it.Len
				if len(leftVariables) == length {
					for i := range leftVariables {
						value := b.ReadMemberCallValue(c, b.EmitConstInstPlaceholder(i))
						b.AssignVariable(leftVariables[i], value)
					}
					return
				}
			}
			if c.GetType().GetTypeKind() == ssa.AnyTypeKind {
				for i := range leftVariables {
					b.AssignVariable(
						leftVariables[i],
						b.ReadMemberCallValue(c, b.EmitConstInstPlaceholder(i)),
					)
				}
				return
			}

			if c.IsDropError {
				c.NewError(ssa.Error, TAG,
					ssa.CallAssignmentMismatchDropError(len(leftVariables), c.GetType().String()),
				)
			} else {
				b.NewError(ssa.Error, TAG,
					ssa.CallAssignmentMismatch(len(leftVariables), c.GetType().String()),
				)
			}

			for i := range leftVariables {
				if i >= length {
					value := b.EmitUndefined(leftVariables[i].GetName())
					b.AssignVariable(leftVariables[i], value)
					continue
				}

				if length == 1 {
					// this call type not tuple type, can't read member
					// in this case `i = 0`
					b.AssignVariable(leftVariables[i], c)
					continue
				}
				// this call type is tuple type, can read member
				value := b.ReadMemberCallValue(c, b.EmitConstInstPlaceholder(i))
				b.AssignVariable(leftVariables[i], value)
			}
			return
		}

		// left
		if li, ok := stmt.LeftExpressionList().(*yak.LeftExpressionListContext); ok {
			leftVariables = b.buildLeftExpressionList(forceAssign || op2 != nil, li)
		}

		// check if defined-function
		recoverMark := markInformation()
		defer recoverMark()

		// right value
		if ri, ok := stmt.ExpressionList().(*yak.ExpressionListContext); ok {
			rightValue = b.buildExpressionList(ri)
		}

		leftLen := len(leftVariables)
		rightLen := len(rightValue)
		switch {
		case leftLen == rightLen:
			// (n) = (n) just assign
			for i := range rightValue {
				// if inst, ok := rvalues[i].(ssa.va); ok {
				// 	inst.SetLeftPosition(lvalues[i].GetPosition())
				// }
				b.AssignVariable(leftVariables[i], rightValue[i])
			}
		case leftLen == 1 && rightLen == 0:
			// (1) = (0)
			// assign to  undefined
			// TODO
			b.NewError(ssa.Error, TAG, AssignRightSideEmpty())
			// b.AssignVariable(leftVariables[0], b.EmitUndefined(leftVariables[0].GetName()))
			return nil
		case leftLen == 0 && rightLen == 1:
			// (0) = (1)
			b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
			return nil

		case rightLen == 1:
			// (n) = (1)
			// (n) = field(1, #index)
			inter := rightValue[0]

			if c, ok := inter.(*ssa.Call); ok {
				GetCallField(c, leftVariables)
				break
			}

			for i, variable := range leftVariables {
				idxVar := b.ReadMemberCallValue(inter, b.EmitConstInstPlaceholder(i))
				b.AssignVariable(variable, idxVar)
			}
		case leftLen == 1:
			// (1) = (n)
			// (1) = interface(n)
			_interface := b.CreateObjectWithSlice(rightValue)
			// lvalues[0].Assign(_interface)
			b.AssignVariable(leftVariables[0], _interface)
		default:
			// (n) = (m) && n!=m
			b.NewError(ssa.Error, TAG, MultipleAssignFailed(len(leftVariables), len(rightValue)))
			return nil
		}
		return lo.Map(leftVariables, func(lv *ssa.Variable, _ int) ssa.Value { return b.PeekValueByVariable(lv) })
	}
	return nil
}

// assign expression
func (b *astbuilder) buildAssignExpression(stmt *yak.AssignExpressionContext) []ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if ret := b.AssignList(false, stmt); ret != nil {
		return ret
	}

	if stmt.PlusPlus() != nil { // ++
		variable := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		if variable == nil {
			b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
			return nil
		}
		value := b.EmitBinOp(ssa.OpAdd, b.ReadValueByVariable(variable), b.EmitConstInst(1))
		b.AssignVariable(variable, value)
		return []ssa.Value{value}
	} else if stmt.SubSub() != nil { // --
		variable := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		if variable == nil {
			b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
			return nil
		}
		value := b.EmitBinOp(ssa.OpSub, b.ReadValueByVariable(variable), b.EmitConstInst(1))
		b.AssignVariable(variable, value)
		return []ssa.Value{b.ReadValueByVariable(variable)}
	}

	if op, ok := stmt.InplaceAssignOperator().(*yak.InplaceAssignOperatorContext); ok {
		variable := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		if variable == nil {
			b.NewError(ssa.Error, TAG, AssignLeftSideEmpty())
			return nil
		}
		rightValue := b.buildExpression(stmt.Expression().(*yak.ExpressionContext))
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
		value := b.EmitBinOp(opcode, b.ReadValueByVariable(variable), rightValue)
		b.AssignVariable(variable, value)
		return []ssa.Value{value}
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
		recoverRange := b.SetRangeFromTerminalNode(idstmt)
		id := idstmt.GetText()
		v := b.CreateLocalVariable(id)
		b.AssignVariable(v, b.EmitValueOnlyDeclare(id))
		recoverRange()
	}
}

func (b *astbuilder) buildDeclareAndAssignExpression(stmt *yak.DeclareAndAssignExpressionContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	b.AssignList(true, stmt)
}

// left expression list
func (b *astbuilder) buildLeftExpressionList(forceAssign bool, stmt *yak.LeftExpressionListContext) []*ssa.Variable {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	exprs := stmt.AllLeftExpression()
	valueLen := len(exprs)
	values := make([]*ssa.Variable, 0, valueLen)
	for _, e := range exprs {
		if e, ok := e.(*yak.LeftExpressionContext); ok {
			if v := b.buildLeftExpression(forceAssign, e); !utils.IsNil(v) {
				values = append(values, v)
			}
		}
	}
	return values
}

// buildLeftExpression build left expression
func (b *astbuilder) buildLeftExpression(forceAssign bool, stmt *yak.LeftExpressionContext) *ssa.Variable {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	if s := stmt.Identifier(); s != nil {
		text := s.GetText()
		if forceAssign {
			return b.CreateLocalVariable(text)
		} else {
			return b.CreateVariable(text)
		}
	}
	// TODO: this is member call
	if s, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		var ret *ssa.Variable
		expr := b.buildExpression(s)

		if s, ok := stmt.LeftSliceCall().(*yak.LeftSliceCallContext); ok {
			recoverRange := b.SetRange(s.BaseParserRuleContext)
			if s, ok := s.Expression().(*yak.ExpressionContext); ok {
				index := b.buildExpression(s)
				ret = b.CreateMemberCallVariable(expr, index)
			}
			recoverRange()
		}

		if s, ok := stmt.LeftMemberCall().(*yak.LeftMemberCallContext); ok {
			recoverRange := b.SetRange(s.BaseParserRuleContext)
			if id := s.Identifier(); id != nil {
				idText := id.GetText()
				callee := b.EmitConstInstPlaceholder(idText)
				ret = b.CreateMemberCallVariable(expr, callee)
			} else if id := s.IdentifierWithDollar(); id != nil {
				key := b.ReadValue(id.GetText()[1:])
				ret = b.CreateMemberCallVariable(expr, key)
			}
			recoverRange()
		}
		return ret

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

	// fmt.Printf("exp = %v\n", stmt.GetText())

	// typeLiteral expression
	if s, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
		if stmt.LParen() != nil && stmt.RParen() != nil {
			v := getValue(0)
			if v == nil {
				// TODO:  int() => type-cast [number] undefined-""
				v = b.EmitUndefined("")
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
	if s, ok := stmt.PanicStmt().(*yak.PanicStmtContext); ok {
		b.buildPanicStmt(s)
		return nil
	}

	// RECOVER
	if s, ok := stmt.RecoverStmt().(*yak.RecoverStmtContext); ok {
		return b.buildRecoverStmt(s)
	}

	// identifier
	if s := stmt.Identifier(); s != nil { // 解析变量
		text := s.GetText()
		if text == "_" {
			b.NewError(ssa.Warn, TAG, "cannot use _ as value")
			// return nil
		}
		v := b.ReadValue(text)
		return v
	}
	// member call
	if s, ok := stmt.MemberCall().(*yak.MemberCallContext); ok {
		recoverRange := b.SetRange(s.BaseParserRuleContext)
		defer recoverRange()

		expr, ok := stmt.Expression(0).(*yak.ExpressionContext)
		if !ok {
			return nil
		}
		exprx := b.buildExpression(expr)
		if id := s.Identifier(); id != nil {
			idText := id.GetText()
			return b.ReadMemberCallValue(exprx, b.EmitConstInstPlaceholder(idText))
		} else if id := s.IdentifierWithDollar(); id != nil {
			key := b.ReadValue(id.GetText()[1:])
			if key == nil {
				b.NewError(ssa.Error, TAG, ExpressionNotVariable(id.GetText()))
				return nil
			}
			return b.ReadMemberCallValue(exprx, key)
		}
	}

	// slice call
	if s, ok := stmt.SliceCall().(*yak.SliceCallContext); ok {
		expression, ok := stmt.Expression(0).(*yak.ExpressionContext)
		if !ok {
			return nil
		}
		expr := b.buildExpression(expression)
		keys := b.buildSliceCall(s)
		if len(keys) == 1 {
			return b.ReadMemberCallValue(expr, keys[0])
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
			return b.EmitUndefined("")
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
			b.NewError(ssa.Error, TAG, UnaryOperatorNotSupport(s.GetText()))
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
			b.NewError(ssa.Error, TAG, BinaryOperatorNotSupport(op.GetText()))
			return nil
		}
		return b.EmitBinOp(opcode, op0, op1)
	}

	// | expression '<-' expression
	// TODO: handler "<-"
	if stmt.ChanIn() != nil {
		// op1, op2 := getValue(0), getValue(1)
		// b.EmitUpdate(op1, op2)
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
				id = any
				if [cond] true-> if.true; false -> if.false
			if.true:
				id = trueExpr
				jump if.done
			if.false:
				id = falseExpr
				jump if.done
			if.done
				id = phi[TrueExpr, falseExpr]

	*/
	handlerJumpExpression := func(cond func(string) ssa.Value, trueExpr, falseExpr func() ssa.Value, valueName string) ssa.Value {
		// 为了聚合产生Phi指令
		id := valueName
		variable := b.CreateVariable(id)
		b.AssignVariable(variable, b.EmitValueOnlyDeclare(id))
		// 只需要使用b.WriteValue设置value到此ID，并最后调用b.ReadValue可聚合产生Phi指令，完成语句预期行为
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
		ifb.SetElse(func() {
			v := falseExpr()
			variable := b.CreateVariable(id)
			b.AssignVariable(variable, v)
		})
		ifb.Build()
		// generator phi instruction
		v := b.ReadValue(id)
		v.SetName(stmt.GetText())
		return v
	}

	// | expression '&&' ws* expression
	/*
		target = a && b

		if a {
			target = b
		}else {
			target = a
		}
	*/
	if s := stmt.LogicAnd(); s != nil {
		value1 := getValue(0)
		value2 := getValue(1)
		return handlerJumpExpression(
			func(id string) ssa.Value {
				return value1
			},
			func() ssa.Value {
				return value2
			},
			func() ssa.Value {
				return value1
			},
			ssa.AndExpressionVariable,
		)
	}

	// | expression '||' ws* expression
	/*
		target = a || b

		if a {
			target = a
		}else {
			target = b
		}
	*/
	if s := stmt.LogicOr(); s != nil {
		value1 := getValue(0)
		value2 := getValue(1)
		return handlerJumpExpression(
			func(id string) ssa.Value {
				return value1
			},
			func() ssa.Value {
				return value1
			},
			func() ssa.Value {
				return value2
			},
			ssa.OrExpressionVariable,
		)
	}

	// | expression '?' ws* expression ws* ':' ws* expression
	/*
		target = cond ? a : b

		if cond {
			target = a
		}else {
			target = b
		}
	*/
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
			ssa.TernaryExpressionVariable,
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
		b.NewError(ssa.Error, TAG, NotSetTypeInMakeExpression())
		return nil
	}

	var exprs []ssa.Value
	if s, ok := stmt.ExpressionListMultiline().(*yak.ExpressionListMultilineContext); ok {
		exprs = b.buildExpressionListMultiline(s)
	}
	zero := b.EmitConstInstPlaceholder(0)
	switch typ.GetTypeKind() {
	case ssa.SliceTypeKind, ssa.BytesTypeKind:
		if len(exprs) == 0 {
			return b.EmitMakeBuildWithType(typ, zero, zero)
		} else if len(exprs) == 1 {
			return b.EmitMakeBuildWithType(typ, exprs[0], exprs[0])
		} else if len(exprs) == 2 {
			return b.EmitMakeBuildWithType(typ, exprs[0], exprs[1])
		} else {
			b.NewError(ssa.Error, TAG, MakeArgumentTooMuch("slice"))
		}
	case ssa.MapTypeKind:
		if len(exprs) == 0 {
			return b.EmitMakeBuildWithType(typ, zero, zero)
		} else if len(exprs) == 1 {
			return b.EmitMakeBuildWithType(typ, exprs[0], exprs[0])
		} else {
			b.NewError(ssa.Error, TAG, MakeArgumentTooMuch("map"))
		}
	case ssa.StructTypeKind:
		b.NewError(ssa.Error, TAG, "cannot make struct{}; type must be slice, map, bytes, or channel")
	case ssa.ChanTypeKind:
		if len(exprs) == 0 {
			return b.EmitMakeBuildWithType(typ, zero, zero)
		} else if len(exprs) == 1 {
			return b.EmitMakeBuildWithType(typ, exprs[0], exprs[0])
		} else {
			b.NewError(ssa.Error, TAG, MakeArgumentTooMuch("chan"))
		}
	default:
		b.NewError(ssa.Error, TAG, MakeUnknownType())
	}
	return nil
}

// instance code
func (b *astbuilder) buildInstanceCode(stmt *yak.InstanceCodeContext) *ssa.Call {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	newFunc := b.NewFunc("")
	{
		b.FunctionBuilder = b.PushFunction(newFunc)

		if block, ok := stmt.Block().(*yak.BlockContext); ok {
			b.buildBlock(block)
		}

		b.Finish()
		b.FunctionBuilder = b.PopFunction()
	}

	return b.NewCall(newFunc, nil)
}

// anonymous function decl
func (b *astbuilder) buildAnonymousFunctionDecl(stmt *yak.AnonymousFunctionDeclContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	funcName := ""
	if name := stmt.FunctionNameDecl(); name != nil {
		funcName = name.GetText()
		b.SetMarkedFunction(funcName)
	}
	if funcName == "" {
		funcName = b.MarkedFuncName
	}
	newFunc := b.NewFunc(funcName)
	if funcName != "" {
		if stmt.FunctionNameDecl() != nil {
			recoverRange := b.SetRange(stmt.FunctionNameDecl())
			defer recoverRange()
		}
		variable := b.CreateVariable(funcName)
		b.AssignVariable(variable, newFunc)
	}

	hitDefinedFunction := false
	// save Current function builder marked FunctionType
	MarkedFunctionType := b.GetMarkedFunction()
	handleFunctionType := func(fun *ssa.Function) {
		fun.ParamLength = len(fun.Params)
		// in this function, builder is sub-function builder
		if MarkedFunctionType == nil {
			return
		}
		if len(fun.Params) != len(MarkedFunctionType.Parameter) {
			return
		}

		for i, p := range fun.Params {
			val, ok := fun.GetValueById(p)
			if !ok {
				continue
			}
			val.SetType(MarkedFunctionType.Parameter[i])
		}
		hitDefinedFunction = true
	}

	{
		recoverRange := b.SetRange(stmt.BaseParserRuleContext)

		b.FunctionBuilder = b.PushFunction(newFunc)
		if b.MarkedThisObject != nil {
			this := b.NewParam(b.MarkedThisObject.GetName())
			this.SetDefault(b.MarkedThisObject)
		}

		if stmt.EqGt() != nil {
			if stmt.LParen() != nil && stmt.RParen() != nil {
				// has param
				// stmt FunctionParamDecl()
				if para, ok := stmt.FunctionParamDecl().(*yak.FunctionParamDeclContext); ok {
					b.buildFunctionParamDecl(para)
				}
			} else {
				// only this param
				id := stmt.Identifier()
				recoverRange := b.SetRangeFromTerminalNode(id)
				b.NewParam(id.GetText())
				recoverRange()
			}

			// handler Marked Function
			handleFunctionType(b.Function)

			if block, ok := stmt.Block().(*yak.BlockContext); ok {
				// build block
				b.buildBlock(block)
			} else if expression, ok := stmt.Expression().(*yak.ExpressionContext); ok {
				// handler expression
				recoverRange := b.SetRange(expression)
				b.AppendBlockRange()
				v := b.buildExpression(expression)
				b.EmitReturn([]ssa.Value{v})
				recoverRange()
			} else {
				b.NewError(ssa.Error, TAG, ArrowFunctionNeedExpressionOrBlock())
			}
		} else {
			// this global function
			if para, ok := stmt.FunctionParamDecl().(*yak.FunctionParamDeclContext); ok {
				b.buildFunctionParamDecl(para)
			}
			// handler markedFunction
			handleFunctionType(b.Function)

			if block, ok := stmt.Block().(*yak.BlockContext); ok {
				b.buildBlock(block)
			}
		}
		b.Finish()
		b.FunctionBuilder = b.PopFunction()
		if hitDefinedFunction {
			b.MarkedFunctions = append(b.MarkedFunctions, newFunc)
		}

		recoverRange()
	}

	// b.AddSubFunction(buildFunc)

	return newFunc
}

// function param decl
func (b *astbuilder) buildFunctionParamDecl(stmt *yak.FunctionParamDeclContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	ellipsis := stmt.Ellipsis() // if has "...",  use array pass this argument
	ids := stmt.AllIdentifier()

	for _, id := range ids {
		recoverRange := b.SetRangeFromTerminalNode(id)
		b.NewParam(id.GetText())
		recoverRange()
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
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
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
		b.NewError(ssa.Error, TAG, SliceCallExpressionIsEmpty())
		return nil
	}
	if len(exprs) > 3 {
		b.NewError(ssa.Error, TAG, SliceCallExpressionTooMuch())
		return nil
	}
	for i, expr := range exprs {
		if s, ok := expr.(*yak.ExpressionContext); ok {
			values[i] = b.buildExpression(s)
		} else {
			values[i] = b.EmitConstInstPlaceholder(0)
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
