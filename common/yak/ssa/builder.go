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
	f.buildStatementList(ast.StatementList().(*yak.StatementListContext))
}

func (f *Function) buildStatementList(stmtlist *yak.StatementListContext) {
	for _, stmt := range stmtlist.AllStatement() {
		if stmt, ok := stmt.(*yak.StatementContext); ok {
			f.buildStatement(stmt)
		}
	}
}

func (f *Function) buildOrdinaryArguments(stmt *yak.OrdinaryArgumentsContext) []Value {
	ellipsis := stmt.Ellipsis()
	allexpre := stmt.AllExpression()
	v := make([]Value, 0, len(allexpre))
	for _, expr := range allexpre {
		v = append(v, f.buildExpression(expr.(*yak.ExpressionContext)))
	}
	if ellipsis != nil {
		//TODO: handler "..." to array
		// v[len(v)-1]
	}
	return v
}

func (f *Function) buildFunctionCall(stmt *yak.FunctionCallContext, v *MakeClosure) Value {
	var args []Value
	isDropErr := false
	if s, ok := stmt.OrdinaryArguments().(*yak.OrdinaryArgumentsContext); ok {
		args = f.buildOrdinaryArguments(s)
	}
	if stmt.Wavy() != nil {
		isDropErr = true
	}
	return f.emitCall(v, args, isDropErr)
}

func (f *Function) buildAnonymouseFunctionDecl(stmt *yak.AnonymousFunctionDeclContext) Value {
	funcName := ""
	if name := stmt.FunctionNameDecl(); name != nil {
		funcName = name.GetText()
	}
	newfunc := f.Package.NewFunction(funcName)
	f.AddAnonymous(newfunc)

	if stmt.EqGt() != nil {
		if stmt.LParen() != nil && stmt.RParen() != nil {
			// has param
			// stmt.FunctionParamDecl()
			if para, ok := stmt.FunctionParamDecl().(*yak.FunctionParamDeclContext); ok {
				newfunc.buildFunctionParamDecl(para)
			}
		} else {
			// only this param
			newfunc.NewParam(stmt.Identifier().GetText(), true)
		}
		if block, ok := stmt.Block().(*yak.BlockContext); ok {
			// build block
			newfunc.buildBlock(block)
		} else if expression, ok := stmt.Expression().(*yak.ExpressionContext); ok {
			// hanlder expression
			v := newfunc.buildExpression(expression)
			newfunc.emitReturn([]Value{v})
		} else {
			panic("BUG: arrow function need expression or block at least")
		}
	} else {
		// this global function
		if para, ok := stmt.FunctionParamDecl().(*yak.FunctionParamDeclContext); ok {
			newfunc.buildFunctionParamDecl(para)
		}
		if block, ok := stmt.Block().(*yak.BlockContext); ok {
			newfunc.buildBlock(block)
		}
	}

	closure := f.emitMakeClosure(newfunc)
	if funcName != "" {
		f.wirteVariable(funcName, closure)
	}
	return closure
}

func (f *Function) buildFunctionParamDecl(stmt *yak.FunctionParamDeclContext) {
	ellipsis := stmt.Ellipsis() // if has "...",  use array pass this argument
	ids := stmt.AllIdentifier()
	param := make(map[string]*Parameter, len(ids))

	for _, id := range ids {
		param[id.GetText()] = f.NewParam(id.GetText(), false)
	}
	if ellipsis != nil {
		//TODO: handler "..." to array
		// param[len(ids)-1]
	}
	f.Param = param
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
		}
		return f.emitArith(opcode, op0, op1)
	}

	if op := stmt.MultiplicativeBinaryOperator(); op != nil {
		op0 := f.buildExpression(stmt.Expression(0).(*yak.ExpressionContext))
		op1 := f.buildExpression(stmt.Expression(1).(*yak.ExpressionContext))
		var opcode yakvm.OpcodeFlag
		switch op.GetText() {
		case "*":
			opcode = yakvm.OpMul
		case "/":
			opcode = yakvm.OpDiv
		case "%":
			opcode = yakvm.OpMod
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

	if s, ok := stmt.FunctionCall().(*yak.FunctionCallContext); ok {
		var v Value
		if expr, ok := stmt.Expression(0).(*yak.ExpressionContext); ok {
			v = f.buildExpression(expr)
		}
		if fun, ok := v.(*MakeClosure); ok {
			return f.buildFunctionCall(s, fun)
		} else {
			panic("call target is not function object")
		}
	}

	if s, ok := stmt.AnonymousFunctionDecl().(*yak.AnonymousFunctionDeclContext); ok {
		return f.buildAnonymouseFunctionDecl(s)
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
		rvalue := f.emitArith(yakvm.OpAdd, lvalue.GetValue(), NewConst(int64(1)))
		lvalue.Assign(rvalue)
	} else if stmt.SubSub() != nil { // --
		lvalue := f.buildLeftExpression(stmt.LeftExpression().(*yak.LeftExpressionContext))
		rvalue := f.emitArith(yakvm.OpSub, lvalue.GetValue(), NewConst(int64(1)))
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
	isOutIf := false
	if done == nil {
		done = f.newBasicBlock("if.done")
		isOutIf = true
	}

	// create true block
	trueBlock := f.newBasicBlock("if.true")
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
			previf.AddFalse(f.newBasicBlock("if.elif"))
		}
		// in false block
		f.currentBlock = previf.False
		// build condition
		cond := f.buildExpression(state.Expression(index + 1).(*yak.ExpressionContext))
		// if instruction
		currentif := f.emitIf(cond)
		// create true block
		trueBlock := f.newBasicBlock("if.true")
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
			falseBlock := f.newBasicBlock("if.false")
			previf.AddFalse(falseBlock)

			// build false block
			f.currentBlock = falseBlock
			f.buildBlock(elseblock)
			f.emitJump(done)
		} else if elifstmt, ok := elseStmt.IfStmt().(*yak.IfStmtContext); ok {
			// "else if"
			// create elif block
			elifBlock := f.newBasicBlock("if.elif")
			previf.AddFalse(elifBlock)

			// build elif block
			f.currentBlock = elifBlock
			f.buildIfStmt(elifstmt, done)
		}
	} else {
		previf.AddFalse(done)
	}
	f.currentBlock = done
	if isOutIf {
		// in exit if; set rest block
		rest := f.newBasicBlock("")
		f.emitJump(rest)

		// continue rest code
		f.currentBlock = rest
	}
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
		// if only expression; just build expression in header;  to exit or body
		f.currentBlock = header
		cond := f.buildExpression(e)
		ifssa := f.emitIf(cond)
		ifssa.AddFalse(exit)
		ifssa.AddTrue(body)
	} else if cond, ok := stmt.ForStmtCond().(*yak.ForStmtCondContext); ok {
		if first, ok := cond.ForFirstExpr().(*yak.ForFirstExprContext); ok {
			// first expression is initialization, in enter block
			f.currentBlock = enter
			f.buildForFirstExpr(first)
		}
		var condition Value
		if expr, ok := cond.Expression().(*yak.ExpressionContext); ok {
			// build expression in header
			f.currentBlock = header
			condition = f.buildExpression(expr)
		} else {
			// not found expression; default is true
			condition = NewConst(true)
		}
		// build if in header end; to exit or body
		f.currentBlock = header
		ifssa := f.emitIf(condition)
		ifssa.AddFalse(exit)
		ifssa.AddTrue(body)

		if third, ok := cond.ForThirdExpr().(*yak.ForThirdExprContext); ok {
			// third exprssion in latch block, when loop.body builded
			endThird = third
		}
	}

	//  build body
	if b, ok := stmt.Block().(*yak.BlockContext); ok {
		f.currentBlock = body
		f.buildBlock(b)
	}

	if endThird != nil {
		// new latch block for third expression
		latch := f.newBasicBlock("loop.latch")

		// jump from body to latch
		f.currentBlock = body
		f.emitJump(latch)

		// build third expression
		f.currentBlock = latch
		f.buildForThirdExpr(endThird)

		// jump from latch to header
		f.emitJump(header)
	} else {
		// jupm from body to header; when haven't third expression
		f.currentBlock = body
		f.emitJump(header)
	}

	// now header sealed
	header.Sealed()

	rest := f.newBasicBlock("")
	// continue rest code
	f.currentBlock = exit
	f.emitJump(rest)
	f.currentBlock = rest
}

func (f *Function) buildReturn(stmt *yak.ReturnStmtContext) {
	if list, ok := stmt.ExpressionList().(*yak.ExpressionListContext); ok {
		value := f.buildExpressionList(list)
		f.emitReturn(value)
	} else {
		f.emitReturn(nil)
	}
}

func (f *Function) buildExpressionStmt(stmt *yak.ExpressionStmtContext) {
	if s, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		f.buildExpression(s)
	}
}

func (f *Function) buildStatement(stmt *yak.StatementContext) {
	if s, ok := stmt.AssignExpressionStmt().(*yak.AssignExpressionStmtContext); ok {
		f.buildAssignExpressionStmt(s)
		return
	}

	if s, ok := stmt.ExpressionStmt().(*yak.ExpressionStmtContext); ok {
		f.buildExpressionStmt(s)
	}

	if s, ok := stmt.IfStmt().(*yak.IfStmtContext); ok {
		f.buildIfStmt(s, nil)
	}

	if s, ok := stmt.ForStmt().(*yak.ForStmtContext); ok {
		f.buildForStmt(s)
	}

	if s, ok := stmt.ReturnStmt().(*yak.ReturnStmtContext); ok {
		f.buildReturn(s)
	}
}

func (pkg *Package) build() {
	main := pkg.NewFunction("yak-main")
	main.build(pkg.ast)
	for _, f := range pkg.funcs {
		f.ExitBlock = f.Blocks[len(f.Blocks)-1]
		f.EnterBlock = f.Blocks[0]
	}
}

func (pkg *Package) Build() { pkg.buildOnece.Do(pkg.build) }

func (prog *Program) Build() {
	for _, pkg := range prog.Packages {
		pkg.Build()
	}
}
