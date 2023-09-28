package ssa4yak

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
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
	// declear Variable Expression
	if s, ok := stmt.DeclearVariableExpressionStmt().(*yak.DeclearVariableExpressionStmtContext); ok {
		b.buildDeclearVariableExpressionStmt(s)
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

	//TODO: try Stmt

	// if stmt
	if s, ok := stmt.IfStmt().(*yak.IfStmtContext); ok {
		b.buildIfStmt(s)
		return
	}

	if s, ok := stmt.SwitchStmt().(*yak.SwitchStmtContext); ok {
		b.buildSwitchStmt(s)
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
			b.NewError(ssa.Error, TAG, "unexpection break stmt")
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
			b.NewError(ssa.Error, TAG, "unexpection continue stmt")
		}
		return
	}

	if _, ok := stmt.FallthroughStmt().(*yak.FallthroughStmtContext); ok {
		if _fall := b.GetFallthrough(); _fall != nil {
			b.EmitJump(_fall)
		} else {
			b.NewError(ssa.Error, TAG, "unexpection fallthrough stmt")
		}
		return
	}
	//TODO: include stmt
	// defer stmt
	if s, ok := stmt.DeferStmt().(*yak.DeferStmtContext); ok {
		b.buildDeferStmt(s)
		return
	}
	//TODO: go stmt
	//TODO: assert stmt

}

//TODO: try stmt

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
// TODO: defer stmt
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

// TODO: go stmt
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

	loop.BuildCondtion(func() ssa.Value {
		condition := b.buildExpression(cond)
		if condition == nil {
			condition = ssa.NewConst(true)
			b.NewError(ssa.Warn, TAG, "if condition expression is nil, default is true")
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
	}
	if e, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		return []ssa.Value{b.buildExpression(e)}
	}
	return nil
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
			b.PushTarget(done, nil, _fallthrough) // fall throught just jump to next handler
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

		i := b.IfBuilder()

		i.IfBranch(
			// if instruction condition
			func() ssa.Value {
				return b.buildExpression(stmt.Expression(0).(*yak.ExpressionContext))
			},
			// build true body
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
			i.ElifBranch(
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

		// hanlder "else" and "else if "
		elseStmt, ok := stmt.ElseBlock().(*yak.ElseBlockContext)
		if !ok {
			return i
		}
		if elseblock, ok := elseStmt.Block().(*yak.BlockContext); ok {
			i.ElseBranch(
				// create false block
				func() {
					b.buildBlock(elseblock)
				},
			)
		} else if elifstmt, ok := elseStmt.IfStmt().(*yak.IfStmtContext); ok {
			// "else if"
			// create elif block
			i.AddChild(buildIf(elifstmt))
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
	if s, ok := stmt.StatementList().(*yak.StatementListContext); ok {
		b.PushBlockSymbolTable()
		b.buildStatementList(s)
		b.PopBlockSymbolTable()
	} else {
		b.NewError(ssa.Warn, TAG, "empty block")
	}
}

type assiglist interface {
	AssignEq() antlr.TerminalNode
	ColonAssignEq() antlr.TerminalNode
	ExpressionList() yak.IExpressionListContext
	LeftExpressionList() yak.ILeftExpressionListContext
}

func (b *astbuilder) AssignList(stmt assiglist) []ssa.Value {
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
				if c.GetType().GetTypeKind() != ssa.InterfaceTypeKind {
					b.NewError(ssa.Error, TAG, "assign right side is not interface function call")
					return nil
				}
				vs := make([]ssa.Value, 0)
				it := c.GetType().(*ssa.InterfaceType)
				for i := 0; i < it.Len; i++ {
					field := b.EmitField(c, ssa.NewConst(i))
					vs = append(vs, field)
				}
				if len(vs) == len(lvalues) {
					for i := range vs {
						lvalues[i].Assign(vs[i], b.FunctionBuilder)
					}
				} else {
					b.NewError(ssa.Error, TAG, "multi-assign failed: left value length[%d] != right value length[%d]", len(lvalues), len(rvalues))
					return nil
				}

			}

			// (n) = field(1, #index)
			for i, lv := range lvalues {
				field := b.EmitField(inter, ssa.NewConst(i))
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
			// (n) = (m) && n!=m  faltal
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
		rvalue := b.EmitArith(ssa.OpAdd, lvalue.GetValue(b.FunctionBuilder), ssa.NewConst(1))
		lvalue.Assign(rvalue, b.FunctionBuilder)
		return []ssa.Value{lvalue.GetValue(b.FunctionBuilder)}
	} else if stmt.SubSub() != nil { // --
		lvalue := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		if lvalue == nil {
			b.NewError(ssa.Error, TAG, "assign left side is undefine type")
			return nil
		}
		rvalue := b.EmitArith(ssa.OpSub, lvalue.GetValue(b.FunctionBuilder), ssa.NewConst(1))
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
		rvalue = b.EmitArith(opcode, lvalue.GetValue(b.FunctionBuilder), rvalue)
		lvalue.Assign(rvalue, b.FunctionBuilder)
		return []ssa.Value{lvalue.GetValue(b.FunctionBuilder)}
	}
	return nil
}

// declear variable expression
func (b *astbuilder) buildDeclearVariableExpressionStmt(stmt *yak.DeclearVariableExpressionStmtContext) {
	// recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	// defer recoverRange()
	if s, ok := stmt.DeclearVariableExpression().(*yak.DeclearVariableExpressionContext); ok {
		b.buildDeclearVariableExpression(s)
	}
}

func (b *astbuilder) buildDeclearVariableExpression(stmt *yak.DeclearVariableExpressionContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.DeclearVariableOnly().(*yak.DeclearVariableOnlyContext); ok {
		b.buildDeclearVariableOnly(s)
	}
	if s, ok := stmt.DeclearAndAssignExpression().(*yak.DeclearAndAssignExpressionContext); ok {
		b.buildDeclearAndAssignExpression(s)
	}
}

func (b *astbuilder) buildDeclearVariableOnly(stmt *yak.DeclearVariableOnlyContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	for _, idstmt := range stmt.AllIdentifier() {
		id := idstmt.GetText()
		b.WriteVariable(id, b.EmitUndefine(id))
	}
}

func (b *astbuilder) buildDeclearAndAssignExpression(stmt *yak.DeclearAndAssignExpressionContext) {
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
		if forceAssign {
			text = b.MapBlockSymbolTable(text)
		} else if v := b.ReadVariable(text); v != nil {
			// when v exist
			switch v := v.(type) {
			case *ssa.Field:
				if v.OutCapture {
					return v
				}
			case *ssa.Parameter:
			default:
			}
		} else if b.CanBuildFreeValue(text) {
			field := b.GetParentBuilder().NewField(text)
			field.OutCapture = true
			b.FreeValues = append(b.FreeValues, field)
			b.SetReg(field)
			b.GetParentBuilder().WriteVariable(text, field)
			b.WriteVariable(text, field)
			return field
		}
		return ssa.NewIndentifierLV(text)
	}
	if s, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		expr := b.buildExpression(s)
		if expr == nil {
			b.NewError(ssa.Error, TAG, "leftexpression expression is nil")
			return nil
		}
		//TODO: check interface type
		var inter ssa.User
		if expr, ok := expr.(ssa.User); ok {
			inter = expr
		} else {
			b.NewError(ssa.Error, TAG, "leftexprssion exprssion is not interface")
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
				key := b.ReadVariable(id.GetText()[1:])
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
	//TODO: typeliteral expression

	// literal
	if s, ok := stmt.Literal().(*yak.LiteralContext); ok {
		return b.buildLiteral(s)
	}

	// anonymous function decl
	if s, ok := stmt.AnonymousFunctionDecl().(*yak.AnonymousFunctionDeclContext); ok {
		return b.buildAnonymouseFunctionDecl(s)
	}
	//TODO: panic

	//TODO: RECOVER

	// identifier
	if s := stmt.Identifier(); s != nil { // 解析变量
		text := s.GetText()
		if ret := b.ReadVariable(text); ret != nil {
			return ret
		} else if b.CanBuildFreeValue(text) {
			return b.BuildFreeValue(text)
		} else {
			return b.EmitUndefine(text)
		}
	}

	getValue := func(index int) ssa.Value {
		if s, ok := stmt.Expression(index).(*yak.ExpressionContext); ok {
			return b.buildExpression(s)
		}
		return nil
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
			return nil
			// 	}
		}

		if id := s.Identifier(); id != nil {
			idText := id.GetText()
			return b.EmitField(inter, ssa.NewConst(idText))
		} else if id := s.IdentifierWithDollar(); id != nil {
			key := b.ReadVariable(id.GetText()[1:])
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
			return b.EmitInterfaceSlice(expr, keys[0], keys[1], nil)
		} else if len(keys) == 3 {
			return b.EmitInterfaceSlice(expr, keys[0], keys[1], keys[2])
		} else {
			b.NewError(ssa.Error, TAG, "slice call expression argument too much")
		}
	}

	// function call
	if s, ok := stmt.FunctionCall().(*yak.FunctionCallContext); ok {
		return b.EmitCall(b.buildFunctionCallWarp(stmt, s))
	}

	//TODO: parent expression

	// instance code
	if s, ok := stmt.InstanceCode().(*yak.InstanceCodeContext); ok {
		return b.EmitCall(b.buildInstanceCode(s))
	}

	// make expression
	if s, ok := stmt.MakeExpression().(*yak.MakeExpressionContext); ok {
		return b.buildMakeExpression(s)
	}
	//TODO: unary operator expression

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
		}
		return b.EmitArith(opcode, op0, op1)
	}

	// //TODO: 高级逻辑
	// | expression '&&' ws* expression
	// | expression '||' ws* expression
	// | expression 'not'? 'in' expression
	// | expression '<-' expression
	// | expression '?' ws* expression ws* ':' ws* expression
	// ;

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
	case *ssa.InterfaceType:
		switch typ.Kind {
		case ssa.Slice:
			if len(exprs) == 0 {
				return b.EmitInterfaceBuildWithType(typ, zero, zero)
			} else if len(exprs) == 1 {
				return b.EmitInterfaceBuildWithType(typ, exprs[0], exprs[0])
			} else if len(exprs) == 2 {
				return b.EmitInterfaceBuildWithType(typ, exprs[0], exprs[1])
			} else {
				b.NewError(ssa.Error, TAG, "make slice expression argument too much!")
			}
		case ssa.Map:
			return b.EmitInterfaceBuildWithType(typ, zero, zero)
		case ssa.Struct:
		}
	// case *ssa.ChanType:
	// 	fmt.Printf("debug %v\n", "make chan")
	default:
		b.NewError(ssa.Error, TAG, "make unknow type")
	}
	return nil
}

// type literal
func (b *astbuilder) buildTypeLiteral(stmt *yak.TypeLiteralContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	text := stmt.GetText()
	// var type name
	if b := ssa.GetTypeByStr(text); b != nil {
		return b
	}

	// slice type literal
	if s, ok := stmt.SliceTypeLiteral().(*yak.SliceTypeLiteralContext); ok {
		return b.buildSliceTypeLiteral(s)
	}

	// map type literal
	if strings.HasPrefix(text, "map") {
		if s, ok := stmt.MapTypeLiteral().(*yak.MapTypeLiteralContext); ok {
			return b.buildMapTypeLiteral(s)
		}
	}

	// chan type literal
	if strings.HasPrefix(text, "chan") {
		if s, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
			if typ := b.buildTypeLiteral(s); typ != nil {
				// return ssa.NewChanType(typ)
			}
		}
	}

	return nil
}

// slice type literal
func (b *astbuilder) buildSliceTypeLiteral(stmt *yak.SliceTypeLiteralContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	if s, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
		if eleTyp := b.buildTypeLiteral(s); eleTyp != nil {
			return ssa.NewSliceType(eleTyp)
		}
	}
	return nil
}

// map type literal
func (b *astbuilder) buildMapTypeLiteral(stmt *yak.MapTypeLiteralContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	// key
	var keyTyp ssa.Type
	var valueTyp ssa.Type
	if s, ok := stmt.TypeLiteral(0).(*yak.TypeLiteralContext); ok {
		keyTyp = b.buildTypeLiteral(s)
	}

	// value
	if s, ok := stmt.TypeLiteral(1).(*yak.TypeLiteralContext); ok {
		valueTyp = b.buildTypeLiteral(s)
	}
	if keyTyp != nil && valueTyp != nil {
		return ssa.NewMapType(keyTyp, valueTyp)

	}

	return nil
}

// instance code
func (b *astbuilder) buildInstanceCode(stmt *yak.InstanceCodeContext) *ssa.Call {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	newfunc := b.Package.NewFunctionWithParent("", b.Function)
	b.FunctionBuilder = b.PushFunction(newfunc)

	if block, ok := stmt.Block().(*yak.BlockContext); ok {
		b.buildBlock(block)
	}

	b.Finish()
	b.FunctionBuilder = b.PopFunction()

	return b.NewCall(newfunc, nil, false)
}

// anonymous function decl
func (b *astbuilder) buildAnonymouseFunctionDecl(stmt *yak.AnonymousFunctionDeclContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	funcName := ""
	if name := stmt.FunctionNameDecl(); name != nil {
		funcName = name.GetText()
	}
	newfunc := b.Package.NewFunctionWithParent(funcName, b.Function)
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
			// hanlder expression
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
		if expr != nil {
			//TODO handler buildin function
			// if f, ok := buildin[expr.GetText()]; ok {
			// 	return b.buildFunctionCall(stmt, f)
			// }
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
	isDropErr := false
	if s, ok := stmt.OrdinaryArguments().(*yak.OrdinaryArgumentsContext); ok {
		args = b.buildOrdinaryArguments(s)
	}
	if stmt.Wavy() != nil {
		isDropErr = true
	}
	return b.NewCall(v, args, isDropErr)
}

// ordinary argument
func (b *astbuilder) buildOrdinaryArguments(stmt *yak.OrdinaryArgumentsContext) []ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	ellipsis := stmt.Ellipsis()
	allexpre := stmt.AllExpression()
	v := make([]ssa.Value, 0, len(allexpre))
	for _, expr := range allexpre {
		v = append(v, b.buildExpression(expr.(*yak.ExpressionContext)))
	}
	if ellipsis != nil {
		//handler "..." to array
		v[len(v)-1].SetType(ssa.NewInterfaceType())
	}
	return v
}

// slice call
func (b *astbuilder) buildSliceCall(stmt *yak.SliceCallContext) []ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	exprs := stmt.AllExpression()
	values := make([]ssa.Value, len(exprs))
	if len(exprs) == 0 {
		b.NewError(ssa.Error, TAG, "slicecall expression is zero")
		return nil
	}
	if len(exprs) > 3 {
		b.NewError(ssa.Error, TAG, "slicecall expression too much")
		return nil
	}
	for i, expr := range exprs {
		if s, ok := expr.(*yak.ExpressionContext); ok {
			values[i] = b.buildExpression(s)
		}
	}
	return values
}

func (b *astbuilder) buildLiteral(stmt *yak.LiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	//TODO: template stirng literal

	// string literal
	if s, ok := stmt.StringLiteral().(*yak.StringLiteralContext); ok {
		return b.buildStringLiteral(s)
	} else if s, ok := stmt.NumericLiteral().(*yak.NumericLiteralContext); ok {
		return b.buildNumericLiteral(s)
	} else if s, ok := stmt.BoolLiteral().(*yak.BoolLiteralContext); ok {
		boolLit, err := strconv.ParseBool(s.GetText())
		if err != nil {
			b.NewError(ssa.Error, TAG, "Unhandled bool literal")
		}
		return ssa.NewConst(boolLit)
	} else if stmt.UndefinedLiteral() != nil {
		return b.EmitUndefine(stmt.GetText())
	} else if stmt.NilLiteral() != nil {
		return ssa.NewConst(nil)
	} else if stmt.CharaterLiteral() != nil {
		lit := stmt.CharaterLiteral().GetText()
		var s string
		var err error
		if lit == "'\\'" {
			s = "'"
		} else {
			lit = strings.ReplaceAll(lit, `"`, `\"`)
			s, err = strconv.Unquote(fmt.Sprintf("\"%s\"", lit[1:len(lit)-1]))
			if err != nil {
				b.NewError(ssa.Error, TAG, "unquote error %s", err)
				return nil
			}
		}
		runeChar := []rune(s)[0]
		if runeChar < 256 {
			return ssa.NewConst(byte(runeChar))
		} else {
			// unbelievable
			log.Warnf("charater literal is rune: %s", stmt.CharaterLiteral().GetText())
			return ssa.NewConst(runeChar)
		}
	} else if s := stmt.MapLiteral(); s != nil {
		if s, ok := s.(*yak.MapLiteralContext); ok {
			return b.buildMapLiteral(s)
		} else {
			b.NewError(ssa.Error, TAG, "Unhandled Map(Object) Literal: "+stmt.MapLiteral().GetText())
		}
	} else if s := stmt.SliceLiteral(); s != nil {
		if s, ok := s.(*yak.SliceLiteralContext); ok {
			return b.buildSliceLiteral(s)
		} else {
			b.NewError(ssa.Error, TAG, "Unhandled Slice Literal: "+stmt.SliceLiteral().GetText())
		}
	} else if s := stmt.SliceTypedLiteral(); s != nil {
		if s, ok := s.(*yak.SliceTypedLiteralContext); ok {
			return b.buildSliceTypedLiteral(s)
		} else {
			b.NewError(ssa.Error, TAG, "unhandled Slice Typed Literal: "+stmt.SliceTypedLiteral().GetText())
		}
	}

	//TODO: slice typed literal

	// type literal
	if _, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
		// b.buildTypeLiteral(s)
		b.NewError(ssa.Warn, TAG, "this expression is a type")
	}

	// mixed

	return nil
}

// numeric literal
func (b *astbuilder) buildNumericLiteral(stmt *yak.NumericLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	// integer literal
	if ilit := stmt.IntegerLiteral(); ilit != nil {
		var err error
		var originIntStr = ilit.GetText()
		var intStr = strings.ToLower(originIntStr)
		var resultInt64 int64
		switch true {
		case strings.HasPrefix(intStr, "0b"): // 二进制
			resultInt64, err = strconv.ParseInt(intStr[2:], 2, 64)
		case strings.HasPrefix(intStr, "0x"): // 十六进制
			resultInt64, err = strconv.ParseInt(intStr[2:], 16, 64)
		case strings.HasPrefix(intStr, "0o"): // 八进制
			resultInt64, err = strconv.ParseInt(intStr[2:], 8, 64)
		case len(intStr) > 1 && intStr[0] == '0':
			resultInt64, err = strconv.ParseInt(intStr[1:], 8, 64)
		default:
			resultInt64, err = strconv.ParseInt(intStr, 10, 64)
		}
		if err != nil {
			b.NewError(ssa.Error, TAG, "const parse %s as integer literal... is to large for int64: %v", originIntStr, err)
			return nil
		}
		if resultInt64 > math.MaxInt {
			return ssa.NewConst(int64(resultInt64))
		} else {
			return ssa.NewConst(int(resultInt64))
		}
	}

	// float literal
	if iFloat := stmt.FloatLiteral(); iFloat != nil {
		lit := iFloat.GetText()
		if strings.HasPrefix(lit, ".") {
			lit = "0" + lit
		}
		var f, _ = strconv.ParseFloat(lit, 64)
		return ssa.NewConst(f)
	}
	b.NewError(ssa.Error, TAG, "cannot parse num for literal: %s", stmt.GetText())
	return nil
}

// string literal
func (b *astbuilder) buildStringLiteral(stmt *yak.StringLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	var text = stmt.GetText()
	if text == "" {
		return ssa.NewConst(text)
	}

	var prefix byte
	var hasPrefix = false
	var supportPrefix = []byte{'x', 'b', 'r'}
ParseStrLit:
	switch text[0] {
	case '"':
		if prefix == 'r' {
			var val string
			if lit := text; len(lit) >= 2 {
				val = lit[1 : len(lit)-1]
			} else {
				val = lit
			}
			prefix = 0
			return ssa.NewConstWithUnary(val, int(prefix))
		} else {
			val, err := strconv.Unquote(text)
			if err != nil {
				fmt.Printf("parse %v to stirng literal fieled: %s", stmt.GetText(), err.Error())
			}
			return ssa.NewConstWithUnary(val, int(prefix))
		}
	case '\'':
		if prefix == 'r' {
			var val string
			if lit := stmt.GetText(); len(lit) >= 2 {
				val = lit[1 : len(lit)-1]
			} else {
				val = lit
			}
			prefix = 0
			return ssa.NewConstWithUnary(val, int(prefix))

		} else {
			if lit := stmt.GetText(); len(lit) >= 2 {
				text = lit[1 : len(lit)-1]
			} else {
				text = lit
			}
			text = strings.Replace(text, "\\'", "'", -1)
			text = strings.Replace(text, `"`, `\"`, -1)
			val, err := strconv.Unquote(`"` + text + `"`)
			if err != nil {
				fmt.Printf("pars %v to string literal field: %s", stmt.GetText(), err.Error())
			}
			return ssa.NewConstWithUnary(val, int(prefix))
		}
	case '`':
		val := text[1 : len(text)-1]
		return ssa.NewConstWithUnary(val, int(prefix))
	case '0':
		switch text[1] {
		case 'h':
			text = text[2:]
			hex, err := codec.DecodeHex(text)
			if err != nil {
				fmt.Printf("parse hex string error: %v", err)
			}
			return ssa.NewConst(hex)
		}
	default:
		if !hasPrefix {
			hasPrefix = true
			prefix = text[0]
			for _, p := range supportPrefix {
				if p == prefix {
					text = text[1:]
					goto ParseStrLit
				}
			}
		}
		if hasPrefix {
			fmt.Printf("invalid string literal: %s", stmt.GetText())
		}
	}

	return nil
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
	allexpr := stmt.AllExpression()
	exprs := make([]ssa.Value, 0, len(allexpr))
	for _, expr := range allexpr {
		if expr, ok := expr.(*yak.ExpressionContext); ok {
			exprs = append(exprs, b.buildExpression(expr))
		}
	}
	return exprs
}
