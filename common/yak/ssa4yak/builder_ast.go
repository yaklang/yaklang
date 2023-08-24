package ssa4yak

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// entry point
func (b *astbuilder) build(ast *yak.YaklangParser) {
	// ast.StatementList()
	b.buildStatementList(ast.StatementList().(*yak.StatementListContext))
}

// statement list
func (b *astbuilder) buildStatementList(stmtlist *yak.StatementListContext) {
	recover := b.SetRange(stmtlist.BaseParserRuleContext)
	defer recover()
	allstmt := stmtlist.AllStatement()
	if len(allstmt) == 0 {
		b.NewError(ssa.Warn, ssa.ASTTAG, "empty statement list")
	} else {
		for _, stmt := range allstmt {
			if stmt, ok := stmt.(*yak.StatementContext); ok {
				b.buildStatement(stmt)
			}
		}
	}
}

func (b *astbuilder) buildStatement(stmt *yak.StatementContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
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
		b.buildIfStmt(s, nil)
		return
	}

	if s, ok := stmt.SwitchStmt().(*yak.SwitchStmtContext); ok {
		b.buildSwitchStmt(s)
		return
	}

	//TODO: for range stmt

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
			b.NewError(ssa.Error, ssa.ASTTAG, "unexpection break stmt")
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
			b.NewError(ssa.Error, ssa.ASTTAG, "unexpection continue stmt")
		}
		return
	}

	if _, ok := stmt.FallthroughStmt().(*yak.FallthroughStmtContext); ok {
		if _fall := b.GetFallthrough(); _fall != nil {
			b.EmitJump(_fall)
		} else {
			b.NewError(ssa.Error, ssa.ASTTAG, "unexpection fallthrough stmt")
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
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	if s, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		b.buildExpression(s)
	}
}

// assign expression stmt
func (b *astbuilder) buildAssignExpressionStmt(stmt *yak.AssignExpressionStmtContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
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
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

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
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	if list, ok := stmt.ExpressionList().(*yak.ExpressionListContext); ok {
		values := b.buildExpressionList(list)
		b.EmitReturn(values)
	} else {
		b.EmitReturn(nil)
	}
}

// for stmt
func (b *astbuilder) buildForStmt(stmt *yak.ForStmtContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	//	    ...enter...
	//	    // for first expre in here
	//      jump loop.header
	// loop.header: 		    <- enter, loop.latch
	//      // for stmt cond in here
	//      If [cond] true -> loop.body, false -> loop.exit
	// loop.body:	    		<- loop.header
	//      // for body block in here
	// loop.latch:              <- loop.body      (target of continue)
	//      // for third expr in here
	//      jump loop.header
	// loop.exit:	    		<- loop.header    (target of break)
	//      jump rest
	// rest:
	//      ...rest.code....

	// current := f.currentBlock
	enter := b.CurrentBlock
	header := b.NewBasicBlockUnSealed("loop.header")

	body := b.NewBasicBlock("loop.body")
	exit := b.NewBasicBlock("loop.exit")
	latch := b.NewBasicBlock("loop.latch")
	var endThird *yak.ForThirdExprContext
	endThird = nil

	var cond ssa.Value
	if e, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		// if only expression; just build expression in header;
		cond = b.buildExpression(e)
	} else if condition, ok := stmt.ForStmtCond().(*yak.ForStmtCondContext); ok {
		if first, ok := condition.ForFirstExpr().(*yak.ForFirstExprContext); ok {
			// first expression is initialization, in enter block
			b.CurrentBlock = enter
			recover := b.SetRange(first.BaseParserRuleContext)
			b.ForExpr(first)
			recover()
		}
		if expr, ok := condition.Expression().(*yak.ExpressionContext); ok {
			// build expression in header
			b.CurrentBlock = header
			cond = b.buildExpression(expr)
		} else {
			// not found expression; default is true
			cond = ssa.NewConst(true)
			b.NewError(ssa.Warn, ssa.ASTTAG, "if condition expression is nil, default is true")
		}

		if third, ok := condition.ForThirdExpr().(*yak.ForThirdExprContext); ok {
			// third exprssion in latch block, when loop.body builded
			endThird = third
		}
	}
	// jump enter->header
	b.CurrentBlock = enter
	b.EmitJump(header)
	// build if in header end; to exit or body
	b.CurrentBlock = header
	ifssa := b.EmitIf(cond)
	ifssa.AddFalse(exit)
	ifssa.AddTrue(body)

	//  build body
	b.CurrentBlock = body
	if block, ok := stmt.Block().(*yak.BlockContext); ok {
		b.PushTarget(exit, latch, nil) // push target for break and continue
		b.buildBlock(block)            // block can create block
		b.PopTarget()                  // pop
		// // f.currentBlock is end block in body
		// body = b.CurrentBlock
	}
	// jump body -> latch
	b.EmitJump(latch)

	// build latch
	b.CurrentBlock = latch
	if endThird != nil {
		// build third expression in loop.body end
		recover := b.SetRange(endThird.BaseParserRuleContext)
		b.ForExpr(endThird)
		recover()
	}
	// jump latch -> header
	b.EmitJump(header)

	// now header sealed
	header.Sealed()

	rest := b.NewBasicBlock("")
	// jump exit -> rest
	b.CurrentBlock = exit
	b.EmitJump(rest)
	// continue in rest code
	b.CurrentBlock = rest
}

type forExpr interface {
	Expression() yak.IExpressionContext
	AssignExpression() yak.IAssignExpressionContext
}

func (b *astbuilder) ForExpr(stmt forExpr) {
	if ae, ok := stmt.AssignExpression().(*yak.AssignExpressionContext); ok {
		b.buildAssignExpression(ae)
	}
	if e, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		b.buildExpression(e)
	}
}

//TODO: for range stmt

// switch stmt
func (b *astbuilder) buildSwitchStmt(stmt *yak.SwitchStmtContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	//	    ...enter...
	//      // switch stmt cond in here
	//      switch cond default:[%switch.default] {var1:%switch.handler_var1, var2:%switch.handler_var2...}
	// switch.done:   				<- switch.[*] // all switch block will jump to here
	//      jump rest
	// switch.default: 			  	<- enter
	//      // default stmt in here
	//      jump switch.done
	// switch.handler_var1: 		<- enter
	//      // case var1 stmt in here
	//      jump switch.done
	//      jump switch.{next_case} // if fallthough
	// switch.handler_var1: 		<- enter
	//      // case var1 stmt in here
	//      jump switch.done
	// rest: <- switch.done
	//      ...rest.code....

	//  parse expression
	var cond ssa.Value
	if expr, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		cond = b.buildExpression(expr)
	} else {
		// expression is nil
		b.NewError(ssa.Warn, ssa.ASTTAG, "switch expression is nil")
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
func (b *astbuilder) buildIfStmt(stmt *yak.IfStmtContext, done *ssa.BasicBlock) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	//	    ...enter...
	//      // if stmt cond in here
	//      If [cond] true -> if.true, false -> if.elif
	// if.true: 					<- enter
	//      // if-true-body block in here
	//      jump if.done
	// if.elif: 					<- enter
	//      // if-elif cond in here    (this build in "elif" and "else if")
	//      If [cond] true -> if.elif_true, false -> if.false
	// if.elif_true:				<- if.elif
	//      // if-elif-true-body block in here
	//      jump if.done
	// if.false: 					<- if.elif
	//      // if-elif-false-body block in here
	//      jump if.done
	// if.done:				        <- if.elif_true,if.true,if.false  (target of all if block)
	//      jump rest
	// rest:
	//      ...rest.code....

	// condition
	cond := b.buildExpression(stmt.Expression(0).(*yak.ExpressionContext))
	// if instruction
	ifssa := b.EmitIf(cond)
	isOutIf := false
	if done == nil {
		done = b.NewBasicBlock("if.done")
		isOutIf = true
	}

	// create true block
	trueBlock := b.NewBasicBlock("if.true")
	ifssa.AddTrue(trueBlock)

	// build true block
	b.CurrentBlock = trueBlock
	if blockstmt, ok := stmt.Block(0).(*yak.BlockContext); ok {
		b.buildBlock(blockstmt)
	}
	// b.buildBlock(stmt.Block(0).(*yak.BlockContext))
	b.EmitJump(done)

	// handler "elif"
	previf := ifssa
	// add elif block to prev-if false
	for index := range stmt.AllElif() {
		// create false block
		if previf.False == nil {
			previf.AddFalse(b.NewBasicBlock("if.elif"))
		}
		// in false block
		b.CurrentBlock = previf.False
		// build condition
		if condstmt, ok := stmt.Expression(index + 1).(*yak.ExpressionContext); ok {
			recover := b.SetRange(condstmt.BaseParserRuleContext)
			cond := b.buildExpression(condstmt)
			// if instruction
			currentif := b.EmitIf(cond)
			// create true block
			trueBlock := b.NewBasicBlock("if.true")
			currentif.AddTrue(trueBlock)
			// build true block
			b.CurrentBlock = trueBlock
			if blockstmt, ok := stmt.Block(index + 1).(*yak.BlockContext); ok {
				b.buildBlock(blockstmt)
			}
			// jump to done
			b.EmitJump(done)
			// for next elif
			previf = currentif
			recover()
		}
	}

	// hanlder "else" and "else if "
	if elseStmt, ok := stmt.ElseBlock().(*yak.ElseBlockContext); ok {
		if elseblock, ok := elseStmt.Block().(*yak.BlockContext); ok {
			// "else"
			// create false block
			falseBlock := b.NewBasicBlock("if.false")
			previf.AddFalse(falseBlock)

			// build false block
			b.CurrentBlock = falseBlock
			b.buildBlock(elseblock)
			b.EmitJump(done)
		} else if elifstmt, ok := elseStmt.IfStmt().(*yak.IfStmtContext); ok {
			// "else if"
			// create elif block
			elifBlock := b.NewBasicBlock("if.elif")
			previf.AddFalse(elifBlock)

			// build elif block
			b.CurrentBlock = elifBlock
			b.buildIfStmt(elifstmt, done)
		}
	} else {
		previf.AddFalse(done)
	}
	b.CurrentBlock = done
	if isOutIf {
		// in exit if; set rest block
		rest := b.NewBasicBlock("")
		b.EmitJump(rest)

		// continue rest code
		b.CurrentBlock = rest
	}
}

// block
func (b *astbuilder) buildBlock(stmt *yak.BlockContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	if s, ok := stmt.StatementList().(*yak.StatementListContext); ok {
		// b.symbolBlock[]
		// b.symbolBlock = NewBlockSymbolTable(NewBlockId(), b.symbolBlock)
		b.PushBlockSymbolTable()
		b.buildStatementList(s)
		// b.symbolBlock = b.symbolBlock.next
		b.PopBlockSymbolTable()
	} else {
		b.NewError(ssa.Warn, ssa.ASTTAG, "empty block")
	}
}

type assiglist interface {
	AssignEq() antlr.TerminalNode
	ColonAssignEq() antlr.TerminalNode
	ExpressionList() yak.IExpressionListContext
	LeftExpressionList() yak.ILeftExpressionListContext
}

func (b *astbuilder) AssignList(stmt assiglist) {
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
			// (n) = (1)
			// (n) = field(1, #index)
			for i, lv := range lvalues {
				field := b.EmitField(rvalues[0], ssa.NewConst(i))
				lv.Assign(field, b.FunctionBuilder)
			}
		} else if len(lvalues) == 1 {
			// (1) = (n)
			// (1) = interface(n)
			_interface := b.CreateInterfaceWithVs(nil, rvalues)
			lvalues[0].Assign(_interface, b.FunctionBuilder)
		} else {
			// (n) = (m) && n!=m  faltal
			b.NewError(ssa.Error, ssa.ASTTAG, "multi-assign failed: left value length[%d] != right value length[%d]", len(lvalues), len(rvalues))
		}
	}
}

// assign expression
func (b *astbuilder) buildAssignExpression(stmt *yak.AssignExpressionContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	b.AssignList(stmt)

	if stmt.PlusPlus() != nil { // ++
		lvalue := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		rvalue := b.EmitArith(ssa.OpAdd, lvalue.GetValue(b.FunctionBuilder), ssa.NewConst(1))
		lvalue.Assign(rvalue, b.FunctionBuilder)
	} else if stmt.SubSub() != nil { // --
		lvalue := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		rvalue := b.EmitArith(ssa.OpSub, lvalue.GetValue(b.FunctionBuilder), ssa.NewConst(1))
		lvalue.Assign(rvalue, b.FunctionBuilder)
	}

	if op, ok := stmt.InplaceAssignOperator().(*yak.InplaceAssignOperatorContext); ok {
		rvalue := b.buildExpression(stmt.Expression().(*yak.ExpressionContext))
		lvalue := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
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
	}
}

// declear variable expression
func (b *astbuilder) buildDeclearVariableExpressionStmt(stmt *yak.DeclearVariableExpressionStmtContext) {
	// recover := b.SetRange(stmt.BaseParserRuleContext)
	// defer recover()
	if s, ok := stmt.DeclearVariableExpression().(*yak.DeclearVariableExpressionContext); ok {
		b.buildDeclearVariableExpression(s)
	}
}

func (b *astbuilder) buildDeclearVariableExpression(stmt *yak.DeclearVariableExpressionContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	if s, ok := stmt.DeclearVariableOnly().(*yak.DeclearVariableOnlyContext); ok {
		b.buildDeclearVariableOnly(s)
	}
	if s, ok := stmt.DeclearAndAssignExpression().(*yak.DeclearAndAssignExpressionContext); ok {
		b.buildDeclearAndAssignExpression(s)
	}
}

func (b *astbuilder) buildDeclearVariableOnly(stmt *yak.DeclearVariableOnlyContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	// TODO: how handler this ?
	for _, id := range stmt.AllIdentifier() {
		b.WriteVariable(id.GetText(), nil)
	}
}

func (b *astbuilder) buildDeclearAndAssignExpression(stmt *yak.DeclearAndAssignExpressionContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	b.AssignList(stmt)
}

// left expression list
func (b *astbuilder) buildLeftExpressionList(forceAssign bool, stmt *yak.LeftExpressionListContext) []ssa.LeftValue {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	exprs := stmt.AllLeftExpression()
	valueLen := len(exprs)
	values := make([]ssa.LeftValue, valueLen)
	for i, e := range exprs {
		if e, ok := e.(*yak.LeftExpressionContext); ok {
			values[i] = b.buildLeftExpression(forceAssign, e)
		}
	}
	return values
}

// left  expression
func (b *astbuilder) buildLeftExpression(forceAssign bool, stmt *yak.LeftExpressionContext) ssa.LeftValue {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
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
			b.NewError(ssa.Error, ssa.ASTTAG, "leftexpression expression is nil")
		}

		if s, ok := stmt.LeftSliceCall().(*yak.LeftSliceCallContext); ok {
			index := b.buildLeftSliceCall(s)
			if expr, ok := expr.(*ssa.Interface); ok {
				return b.EmitField(expr, index)
			} else {
				b.NewError(ssa.Error, ssa.ASTTAG, "leftexprssion exprssion is not interface")
			}
		}

		//TODO: leftMemberCall
	}
	return nil
}

//TODO: left member call

// left slice call
func (b *astbuilder) buildLeftSliceCall(stmt *yak.LeftSliceCallContext) ssa.Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	if s, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		return b.buildExpression(s)
	}
	return nil
}

// expression
func (b *astbuilder) buildExpression(stmt *yak.ExpressionContext) ssa.Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
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
			b.NewError(ssa.Error, ssa.ASTTAG, "Expression: undefine value %s", s.GetText())
			return ssa.UnDefineConst
		}
	}

	getValue := func(index int) ssa.Value {
		if s, ok := stmt.Expression(index).(*yak.ExpressionContext); ok {
			return b.buildExpression(s)
		}
		return nil
	}

	//TODO: member call

	// slice call
	if s, ok := stmt.SliceCall().(*yak.SliceCallContext); ok {
		expr, ok := getValue(0).(*ssa.Interface)
		if !ok {
			b.NewError(ssa.Error, ssa.ASTTAG, "expression slice need expression")
		}
		keys := b.buildSliceCall(s)
		if len(keys) == 1 {
			return b.EmitField(expr, keys[0])
		} else if len(keys) == 2 {
			return b.EmitInterfaceSlice(expr, keys[0], keys[1], nil)
		} else if len(keys) == 3 {
			return b.EmitInterfaceSlice(expr, keys[0], keys[1], keys[2])
		} else {
			b.NewError(ssa.Error, ssa.ASTTAG, "slice call expression argument too much")
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
			b.NewError(ssa.Error, ssa.ASTTAG, "additive binary operator need two expression")
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
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	var typ ssa.Type
	if s, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
		typ = b.buildTypeLiteral(s)
	}
	if typ == nil {
		b.NewError(ssa.Error, ssa.ASTTAG, "not set type in make expression")
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
				return b.EmitInterfaceBuildWithType(ssa.Types{typ}, zero, zero)
			} else if len(exprs) == 1 {
				return b.EmitInterfaceBuildWithType(ssa.Types{typ}, exprs[0], exprs[0])
			} else if len(exprs) == 2 {
				return b.EmitInterfaceBuildWithType(ssa.Types{typ}, exprs[0], exprs[1])
			} else {
				b.NewError(ssa.Error, ssa.ASTTAG, "make slice expression argument too much!")
			}
		case ssa.Map:
		case ssa.Struct:
		}
	case *ssa.ChanType:
		fmt.Printf("debug %v\n", "make chan")
	default:
		b.NewError(ssa.Error, ssa.ASTTAG, "make unknow type")
	}
	return nil
}

// type literal
func (b *astbuilder) buildTypeLiteral(stmt *yak.TypeLiteralContext) ssa.Type {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
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
				return ssa.NewChanType(ssa.Types{typ})
			}
		}
	}

	return nil
}

// slice type literal
func (b *astbuilder) buildSliceTypeLiteral(stmt *yak.SliceTypeLiteralContext) ssa.Type {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	if s, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
		if eleTyp := b.buildTypeLiteral(s); eleTyp != nil {
			return ssa.NewSliceType(ssa.Types{eleTyp})
		}
	}
	return nil
}

// map type literal
func (b *astbuilder) buildMapTypeLiteral(stmt *yak.MapTypeLiteralContext) ssa.Type {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
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
		return ssa.NewMapType(ssa.Types{keyTyp}, ssa.Types{valueTyp})

	}

	return nil
}

// instance code
func (b *astbuilder) buildInstanceCode(stmt *yak.InstanceCodeContext) *ssa.Call {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

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
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
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
			b.NewError(ssa.Error, ssa.ASTTAG, "BUG: arrow function need expression or block at least")
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
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
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
			if f, ok := buildin[expr.GetText()]; ok {
				return b.buildFunctionCall(stmt, f)
			}
		}
	}
	b.NewError(ssa.Error, ssa.ASTTAG, "call target is nil")
	return nil
}

// function call
func (b *astbuilder) buildFunctionCall(stmt *yak.FunctionCallContext, v ssa.Value) *ssa.Call {
	// recover := b.SetRange(stmt.BaseParserRuleContext)
	// defer recover()
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
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	ellipsis := stmt.Ellipsis()
	allexpre := stmt.AllExpression()
	v := make([]ssa.Value, 0, len(allexpre))
	for _, expr := range allexpre {
		v = append(v, b.buildExpression(expr.(*yak.ExpressionContext)))
	}
	if ellipsis != nil {
		//handler "..." to array
		typ := v[len(v)-1].GetType()
		typ = append(typ, ssa.NewInterfaceType())
		v[len(v)-1].SetType(typ)
	}
	return v
}

// TODO: member call

// slice call
func (b *astbuilder) buildSliceCall(stmt *yak.SliceCallContext) []ssa.Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	exprs := stmt.AllExpression()
	values := make([]ssa.Value, len(exprs))
	if len(exprs) == 0 {
		b.NewError(ssa.Error, ssa.ASTTAG, "slicecall expression is zero")
		return nil
	}
	if len(exprs) > 3 {
		b.NewError(ssa.Error, ssa.ASTTAG, "slicecall expression too much")
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
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	//TODO: template stirng literal

	// string literal
	if s, ok := stmt.StringLiteral().(*yak.StringLiteralContext); ok {
		return b.buildStringLiteral(s)
	} else if s, ok := stmt.NumericLiteral().(*yak.NumericLiteralContext); ok {
		return b.buildNumericLiteral(s)
	} else if s, ok := stmt.BoolLiteral().(*yak.BoolLiteralContext); ok {
		boolLit, err := strconv.ParseBool(s.GetText())
		if err != nil {
			b.NewError(ssa.Error, ssa.ASTTAG, "Unhandled bool literal")
		}
		return ssa.NewConst(boolLit)
	} else if stmt.UndefinedLiteral() != nil {
		return ssa.UnDefineConst
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
				b.NewError(ssa.Error, ssa.ASTTAG, "unquote error %s", err)
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
			b.NewError(ssa.Error, ssa.ASTTAG, "Unhandled Map(Object) Literal: "+stmt.MapLiteral().GetText())
		}
	} else if s := stmt.SliceLiteral(); s != nil {
		if s, ok := s.(*yak.SliceLiteralContext); ok {
			return b.buildSliceLiteral(s)
		} else {
			b.NewError(ssa.Error, ssa.ASTTAG, "Unhandled Slice Literal: "+stmt.SliceLiteral().GetText())
		}
	} else if s := stmt.SliceTypedLiteral(); s != nil {
		if s, ok := s.(*yak.SliceTypedLiteralContext); ok {
			return b.buildSliceTypedLiteral(s)
		} else {
			b.NewError(ssa.Error, ssa.ASTTAG, "unhandled Slice Typed Literal: "+stmt.SliceTypedLiteral().GetText())
		}
	}

	//TODO: slice typed literal

	// type literal
	if _, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
		// b.buildTypeLiteral(s)
		b.NewError(ssa.Warn, ssa.ASTTAG, "this expression is a type")
	}

	// mixed

	return nil
}

// numeric literal
func (b *astbuilder) buildNumericLiteral(stmt *yak.NumericLiteralContext) ssa.Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

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
			b.NewError(ssa.Error, ssa.ASTTAG, "const parse %s as integer literal... is to large for int64: %v", originIntStr, err)
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
	b.NewError(ssa.Error, ssa.ASTTAG, "cannot parse num for literal: %s", stmt.GetText())
	return nil
}

// string literal
func (b *astbuilder) buildStringLiteral(stmt *yak.StringLiteralContext) ssa.Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
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
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	exprs := stmt.AllExpression()
	valueLen := len(exprs)
	values := make([]ssa.Value, valueLen)
	for i, e := range exprs {
		if e, ok := e.(*yak.ExpressionContext); ok {
			values[i] = b.buildExpression(e)
		}
	}
	return values
}

// expression list multiline
func (b *astbuilder) buildExpressionListMultiline(stmt *yak.ExpressionListMultilineContext) []ssa.Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	allexpr := stmt.AllExpression()
	exprs := make([]ssa.Value, 0, len(allexpr))
	for _, expr := range allexpr {
		if expr, ok := expr.(*yak.ExpressionContext); ok {
			exprs = append(exprs, b.buildExpression(expr))
		}
	}
	return exprs
}
