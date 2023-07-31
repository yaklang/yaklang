package ssa

import (
	"fmt"
	"go/constant"
	"strconv"

	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func (f *Function) build(ast *yak.YaklangParser) {
	// ast.StatementList()
	entry := f.newBasicBlock("entry")
	f.currentBlock = entry

	f.buildStatementList(ast.StatementList().(*yak.StatementListContext))
}

func (f *Function) buildStatementList(stmtlist *yak.StatementListContext) {
	for _, stmt := range stmtlist.AllStatement() {
		if stmt, ok := stmt.(*yak.StatementContext); ok {
			f.buildStatement(stmt)
		}
	}
}

func (f *Function) buildExpression(stmt *yak.ExpressionContext) (ret Value) {
	if op := stmt.AdditiveBinaryOperator(); op != nil {
		op0 := f.buildExpression(stmt.Expression(0).(*yak.ExpressionContext))
		op1 := f.buildExpression(stmt.Expression(1).(*yak.ExpressionContext))
		var opcode yakvm.OpcodeFlag
		switch op.GetText() {
		case "+":
			opcode = yakvm.OpAdd
		case "-":
			opcode = yakvm.OpSub
		case "*":
			opcode = yakvm.OpMul
		case "/":
			opcode = yakvm.OpDiv
		}
		return f.emitArith(opcode, op0, op1)
	}

	if s := stmt.Literal(); s != nil {
		// literal
		i, _ := strconv.ParseInt(s.GetText(), 10, 64)
		return &Const{
			value: constant.MakeInt64(i),
		}
	}

	if s := stmt.Identifier(); s != nil { // 解析变量
		ret := f.readVariable(s.GetText())
		if ret == nil {
			fmt.Printf("debug undefine value: %v\n", s.GetText())
			panic("undefine value")
		}
		return ret
	}

	if op := stmt.ComparisonBinaryOperator(); op != nil {
		op0 := f.buildExpression(stmt.Expression(0).(*yak.ExpressionContext))
		op1 := f.buildExpression(stmt.Expression(1).(*yak.ExpressionContext))
		var opcode yakvm.OpcodeFlag
		switch op.GetText() {
		case ">":
			opcode = yakvm.OpGt
		case "<":
			opcode = yakvm.OpLt
		}
		return f.emitArith(opcode, op0, op1)

	}
	return nil
}

func (f *Function) buildExpressionList(stmt *yak.ExpressionListContext) []Value {
	exprs := stmt.AllExpression()
	valueLen := len(exprs)
	values := make([]Value, valueLen)
	for i, e := range exprs {
		if e, ok := e.(*yak.ExpressionContext); ok {
			values[i] = f.buildExpression(e)
		}
	}
	return values
}

func (f *Function) buildLeftExpression(stmt *yak.LeftExpressionContext) LeftValue {
	if s := stmt.Identifier(); s != nil {
		return &Identifier{
			variable: s.GetText(),
			f:        f,
		}
	}
	return nil
}
func (f *Function) buildLeftExpressionList(stmt *yak.LeftExpressionListContext) []LeftValue {
	exprs := stmt.AllLeftExpression()
	valueLen := len(exprs)
	values := make([]LeftValue, valueLen)
	for i, e := range exprs {
		if e, ok := e.(*yak.LeftExpressionContext); ok {
			values[i] = f.buildLeftExpression(e)
		}
	}
	return values
}

func (f *Function) buildAssignExpressionStmt(stmt *yak.AssignExpressionStmtContext) {
	s := stmt.AssignExpression()
	if s == nil {
		return
	}
	if i, ok := s.(*yak.AssignExpressionContext); ok {
		f.buildAssignExpression(i)
	}
}
func (f *Function) buildAssignExpression(stmt *yak.AssignExpressionContext) {
	if op, op2 := stmt.AssignEq(), stmt.ColonAssignEq(); op != nil || op2 != nil {
		// right value
		var rvalues []Value
		if ri, ok := stmt.ExpressionList().(*yak.ExpressionListContext); ok {
			rvalues = f.buildExpressionList(ri)
		}

		// left
		var lvalues []LeftValue
		if li, ok := stmt.LeftExpressionList().(*yak.LeftExpressionListContext); ok {
			lvalues = f.buildLeftExpressionList(li)
		}

		// assign
		if len(rvalues) == len(lvalues) {
			for i := range rvalues {
				lvalues[i].Assign(rvalues[i])
			}
		}
	}

	if stmt.PlusPlus() != nil { // ++
		lvalue := f.buildLeftExpression(stmt.LeftExpression().(*yak.LeftExpressionContext))
		rvalue := f.emitArith(yakvm.OpAdd, lvalue.GetValue(), ConstOne)
		lvalue.Assign(rvalue)
	} else if stmt.SubSub() != nil { // --
		lvalue := f.buildLeftExpression(stmt.LeftExpression().(*yak.LeftExpressionContext))
		rvalue := f.emitArith(yakvm.OpSub, lvalue.GetValue(), ConstOne)
		lvalue.Assign(rvalue)
	}

	// inplace Assign operator
}

func (f *Function) buildBlock(block *yak.BlockContext) {
	f.buildStatementList(block.StatementList().(*yak.StatementListContext))
}

func (f *Function) buildIfStmt(state *yak.IfStmtContext, done *BasicBlock) {
	// condition
	cond := f.buildExpression(state.Expression(0).(*yak.ExpressionContext))
	// if instruction
	ifssa := f.emitIf(cond)
	if done == nil {
		done = f.newBasicBlock("done")
	}

	// create true block
	trueBlock := f.newBasicBlock("true")
	ifssa.AddTrue(trueBlock)

	// build true block
	f.currentBlock = trueBlock
	f.buildBlock(state.Block(0).(*yak.BlockContext))
	f.emitJump(done)

	// handler "elif"
	previf := ifssa
	// add elif block to prev-if false
	for index := range state.AllElif() {
		// create false block
		if previf.False == nil {
			previf.AddFalse(f.newBasicBlock("elif"))
		}
		// in false block
		f.currentBlock = previf.False
		// build condition
		cond := f.buildExpression(state.Expression(index + 1).(*yak.ExpressionContext))
		// if instruction
		currentif := f.emitIf(cond)
		// create true block
		trueBlock := f.newBasicBlock("true")
		currentif.AddTrue(trueBlock)
		// build true block
		f.currentBlock = trueBlock
		f.buildBlock(state.Block(index + 1).(*yak.BlockContext))
		// jump to done
		f.emitJump(done)
		// for next elif
		previf = currentif
	}

	// hanlder "else" and "else if "
	if elseStmt, ok := state.ElseBlock().(*yak.ElseBlockContext); ok {
		if elseblock, ok := elseStmt.Block().(*yak.BlockContext); ok {
			// "else"
			// create false block
			falseBlock := f.newBasicBlock("false")
			previf.AddFalse(falseBlock)

			// build false block
			f.currentBlock = falseBlock
			f.buildBlock(elseblock)
			f.emitJump(done)
		} else if elifstmt, ok := elseStmt.IfStmt().(*yak.IfStmtContext); ok {
			// "else if"
			// create elif block
			elifBlock := f.newBasicBlock("elif")
			previf.AddFalse(elifBlock)

			// build elif block
			f.currentBlock = elifBlock
			f.buildIfStmt(elifstmt, done)
		}
	} else {
		previf.AddFalse(done)
	}
	f.currentBlock = done
}

func (f *Function) buildForFirstExpr(state *yak.ForFirstExprContext) {
	if ae, ok := state.AssignExpression().(*yak.AssignExpressionContext); ok {
		f.buildAssignExpression(ae)
	}
	if e, ok := state.Expression().(*yak.ExpressionContext); ok {
		f.buildExpression(e)
	}
}
func (f *Function) buildForThirdExpr(state *yak.ForThirdExprContext) {
	if ae, ok := state.AssignExpression().(*yak.AssignExpressionContext); ok {
		f.buildAssignExpression(ae)
	}
	if e, ok := state.Expression().(*yak.ExpressionContext); ok {
		f.buildExpression(e)
	}
}

func (f *Function) buildForStmt(stmt *yak.ForStmtContext) {
	// current := f.currentBlock
	enter := f.currentBlock
	header := f.newBasicBlockUnSealed("loop.header")
	f.emitJump(header)

	body := f.newBasicBlock("loop.body")
	exit := f.newBasicBlock("loop.exit")
	var endThird *yak.ForThirdExprContext
	endThird = nil
	// first line, cond
	if e, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		f.currentBlock = header
		cond := f.buildExpression(e)
		ifssa := f.emitIf(cond)
		ifssa.AddFalse(exit)
		ifssa.AddTrue(body)
	} else if cond, ok := stmt.ForStmtCond().(*yak.ForStmtCondContext); ok {
		if first, ok := cond.ForFirstExpr().(*yak.ForFirstExprContext); ok {
			f.currentBlock = enter
			f.buildForFirstExpr(first)
		}
		if expr, ok := cond.Expression().(*yak.ExpressionContext); ok {
			f.currentBlock = header
			cond := f.buildExpression(expr)
			ifssa := f.emitIf(cond)
			ifssa.AddFalse(exit)
			ifssa.AddTrue(body)
		} else {
			// for i=0; ; i++{}

		}

		if third, ok := cond.ForThirdExpr().(*yak.ForThirdExprContext); ok {
			endThird = third
		}
	}

	//  body
	if b, ok := stmt.Block().(*yak.BlockContext); ok {
		f.currentBlock = body
		f.buildBlock(b)
	}

	if endThird != nil {
		latch := f.newBasicBlock("loop.latch")

		// f.currentBlock = body
		f.emitJump(latch)

		f.currentBlock = latch
		f.buildForThirdExpr(endThird)
		f.emitJump(header)
	} else {
		f.emitJump(header)
	}
	header.Sealed()

	//
	f.currentBlock = exit
}

func (f *Function) buildStatement(stmt *yak.StatementContext) {
	if s, ok := stmt.AssignExpressionStmt().(*yak.AssignExpressionStmtContext); ok {
		f.buildAssignExpressionStmt(s)
		return
	}

	if s, ok := stmt.IfStmt().(*yak.IfStmtContext); ok {
		f.buildIfStmt(s, nil)
	}

	if s, ok := stmt.ForStmt().(*yak.ForStmtContext); ok {
		f.buildForStmt(s)
	}

}

func (pkg *Package) build() {
	main := pkg.NewFunction("yak-main")
	main.build(pkg.ast)
}

func (pkg *Package) Build() { pkg.buildOnece.Do(pkg.build) }

func (prog *Program) Build() {
	for _, pkg := range prog.Packages {
		pkg.Build()
	}
}
