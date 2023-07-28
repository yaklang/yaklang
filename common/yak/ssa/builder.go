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

func (f *Function) buildStatementList(states *yak.StatementListContext) {
	for _, state := range states.AllStatement() {
		state := state.(*yak.StatementContext)
		f.buildStatement(state)
	}
}

func (f *Function) buildLeftExpressionStatmt(i *yak.LeftExpressionContext) string {
	if s := i.Identifier(); s != nil {
		return s.GetText()
	}
	return ""
}

func (f *Function) buildExpressionStatmt(i *yak.ExpressionContext) (ret Value) {
	if op := i.AdditiveBinaryOperator(); op != nil {
		op0 := f.buildExpressionStatmt(i.Expression(0).(*yak.ExpressionContext))
		op1 := f.buildExpressionStatmt(i.Expression(1).(*yak.ExpressionContext))
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

	if s := i.Literal(); s != nil {
		// literal
		i, _ := strconv.ParseInt(s.GetText(), 10, 64)
		return &Const{
			value: constant.MakeInt64(i),
		}
	}

	if s := i.Identifier(); s != nil { // 解析变量
		ret := f.readVariable(s.GetText())
		if ret == nil {
			fmt.Printf("debug undefine value: %v\n", s.GetText())
			panic("undefine value")
		}
		return ret
	}

	if op := i.ComparisonBinaryOperator(); op != nil {
		op0 := f.buildExpressionStatmt(i.Expression(0).(*yak.ExpressionContext))
		op1 := f.buildExpressionStatmt(i.Expression(1).(*yak.ExpressionContext))
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

func (f *Function) buildAssignExpressionStatmt(state *yak.AssignExpressionStmtContext) {
	s := state.AssignExpression()
	i, _ := s.(*yak.AssignExpressionContext)
	if i == nil {
		return
	}
	ei := i.ExpressionList()
	es, _ := ei.(*yak.ExpressionListContext)
	if es == nil {
		return
	}
	expres := es.AllExpression()
	rValueLen := len(expres)
	rvalues := make([]Value, rValueLen)
	for i, e := range expres {
		e, _ := e.(*yak.ExpressionContext)
		if e == nil {
			continue
		}
		rvalues[i] = f.buildExpressionStatmt(e)
	}

	lei := i.LeftExpressionList()
	les, _ := lei.(*yak.LeftExpressionListContext)
	if les == nil {
		return
	}
	lexpres := les.AllLeftExpression()
	lValueLen := len(lexpres)
	// lvalues := make([]Value, 0, lValueLen)
	lv := make([]string, lValueLen)
	for i, e := range lexpres {
		l, _ := e.(*yak.LeftExpressionContext)
		if l == nil {
			continue
		}
		lv[i] = f.buildLeftExpressionStatmt(l)
		// lvalues = append(lvalues, f.buildLeftExpressionStatmt(i))
	}
	if lValueLen == rValueLen {
		for i := range rvalues {
			f.assig(lv[i], rvalues[i])
		}
	}
}

func (f *Function) assig(lv string, rvalue Value) {
	if lv == "" || rvalue == nil {
		return
	}
	f.wirteVariable(lv, rvalue)
}

func (f *Function) buildBlock(block *yak.BlockContext) {
	f.buildStatementList(block.StatementList().(*yak.StatementListContext))
}

func (f *Function) buildIfStmt(state *yak.IfStmtContext, done *BasicBlock) {
	// condition
	cond := f.buildExpressionStatmt(state.Expression(0).(*yak.ExpressionContext))
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
		cond := f.buildExpressionStatmt(state.Expression(index + 1).(*yak.ExpressionContext))
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
	iElseStmt := state.ElseBlock()
	if iElseStmt != nil {
		elseStmt := iElseStmt.(*yak.ElseBlockContext)
		elseblock := elseStmt.Block()
		elifstmt := elseStmt.IfStmt()
		if elseblock != nil {
			// "else"
			// create false block
			falseBlock := f.newBasicBlock("false")
			previf.AddFalse(falseBlock)

			// build false block
			f.currentBlock = falseBlock
			f.buildBlock(elseblock.(*yak.BlockContext))
			f.emitJump(done)
		} else if elifstmt != nil {
			// "else if"
			// create elif block
			elifBlock := f.newBasicBlock("elif")
			previf.AddFalse(elifBlock)

			// build elif block
			f.currentBlock = elifBlock
			f.buildIfStmt(elifstmt.(*yak.IfStmtContext), done)
		}
	} else {
		previf.AddFalse(done)
	}
	f.currentBlock = done
}

func (f *Function) buildStatement(state *yak.StatementContext) {
	if s := state.AssignExpressionStmt(); s != nil {
		s, ok := s.(*yak.AssignExpressionStmtContext)
		if !ok {
			return
		}
		f.buildAssignExpressionStatmt(s)
		return
	}

	if s := state.IfStmt(); s != nil {
		s, ok := s.(*yak.IfStmtContext)
		if !ok {
			return
		}
		f.buildIfStmt(s, nil)
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
