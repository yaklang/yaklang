package ssa

import (
	"fmt"
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

func (f *Function) buildFunctionCall(stmt *yak.FunctionCallContext, v Value) Value {
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
	newfunc := f.Package.NewFunctionWithParent(funcName, f)

	if stmt.EqGt() != nil {
		if stmt.LParen() != nil && stmt.RParen() != nil {
			// has param
			// stmt.FunctionParamDecl()
			if para, ok := stmt.FunctionParamDecl().(*yak.FunctionParamDeclContext); ok {
				newfunc.buildFunctionParamDecl(para)
			}
		} else {
			// only this param
			newfunc.NewParam(stmt.Identifier().GetText())
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
	newfunc.Finish()

	if funcName != "" {
		f.writeVariable(funcName, newfunc)
	}
	return newfunc
}

func (f *Function) buildFunctionParamDecl(stmt *yak.FunctionParamDeclContext) {
	ellipsis := stmt.Ellipsis() // if has "...",  use array pass this argument
	ids := stmt.AllIdentifier()

	for _, id := range ids {
		f.NewParam(id.GetText())
	}
	if ellipsis != nil {
		//TODO: handler "..." to array
		// param[len(ids)-1]
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
		return NewConst(int64(i))
	}

	if s := stmt.Identifier(); s != nil { // 解析变量
		text := s.GetText()
		if ret := f.readVariable(text); ret != nil {
			return ret
		} else if f.CanBuildFreeValue(text) {
			return f.BuildFreeValue(text)
		}
			fmt.Printf("debug undefine value: %v\n", s.GetText())
			panic("undefine value")
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
		case "<=":
			opcode = yakvm.OpLtEq
		case ">=":
			opcode = yakvm.OpGtEq
		case "!=", "<>":
			opcode = yakvm.OpNotEq
		case "==":
			opcode = yakvm.OpEq
		}
		return f.emitArith(opcode, op0, op1)

	}

	if s, ok := stmt.FunctionCall().(*yak.FunctionCallContext); ok {
		var v Value
		if expr, ok := stmt.Expression(0).(*yak.ExpressionContext); ok {
			v = f.buildExpression(expr)
		}
		if v != nil {
			return f.buildFunctionCall(s, v)
		} else {
			panic("call target is nil")
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

func (f *Function) buildLeftExpression(forceAssign bool, stmt *yak.LeftExpressionContext) LeftValue {
	if s := stmt.Identifier(); s != nil {
		if v := f.readVariable(s.GetText()); v != nil {
			// when v exist
			switch v := v.(type) {
			case *Field:
				return v
			case *Parameter:
			default:
			}
		} else if !forceAssign && f.CanBuildFreeValue(s.GetText()) {
			field := f.parent.newField(s.GetText())
			f.FreeValues = append(f.FreeValues, field)
			f.parent.writeVariable(s.GetText(), field)
			f.writeVariable(s.GetText(), field)
			return field
		}
		return &IdentifierLV{
			variable: s.GetText(),
		}
	}
	return nil
}
func (f *Function) buildLeftExpressionList(forceAssign bool, stmt *yak.LeftExpressionListContext) []LeftValue {
	exprs := stmt.AllLeftExpression()
	valueLen := len(exprs)
	values := make([]LeftValue, valueLen)
	for i, e := range exprs {
		if e, ok := e.(*yak.LeftExpressionContext); ok {
			values[i] = f.buildLeftExpression(forceAssign, e)
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
			lvalues = f.buildLeftExpressionList(op2 != nil, li)
		}

		// assign
		if len(rvalues) == len(lvalues) {
			for i := range rvalues {
				lvalues[i].Assign(rvalues[i], f)
			}
		}
	}

	if stmt.PlusPlus() != nil { // ++
		lvalue := f.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		rvalue := f.emitArith(yakvm.OpAdd, lvalue.GetValue(f), NewConst(int64(1)))
		lvalue.Assign(rvalue, f)
	} else if stmt.SubSub() != nil { // --
		lvalue := f.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		rvalue := f.emitArith(yakvm.OpSub, lvalue.GetValue(f), NewConst(int64(1)))
		lvalue.Assign(rvalue, f)
	}

	if op, ok := stmt.InplaceAssignOperator().(*yak.InplaceAssignOperatorContext); ok {
		rvalue := f.buildExpression(stmt.Expression().(*yak.ExpressionContext))
		lvalue := f.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		var opcode yakvm.OpcodeFlag
		switch op.GetText() {
		case "+=":
			opcode = yakvm.OpAdd
		case "-=":
			opcode = yakvm.OpSub
		case "*=":
			opcode = yakvm.OpMul
		case "/=":
			opcode = yakvm.OpDiv
		case "%=":
			opcode = yakvm.OpMod
		case "<<=":
			opcode = yakvm.OpShl
		case ">>=":
			opcode = yakvm.OpShr
		case "&=":
			opcode = yakvm.OpAnd
		case "&^=":
			opcode = yakvm.OpAndNot
		case "|=":
			opcode = yakvm.OpOr
		case "^=":
			opcode = yakvm.OpXor

		}
		rvalue = f.emitArith(opcode, lvalue.GetValue(f), rvalue)
		lvalue.Assign(rvalue, f)
	}
}

func (f *Function) buildBlock(stmt *yak.BlockContext) {
	if s, ok := stmt.StatementList().(*yak.StatementListContext); ok {
		f.buildStatementList(s)
	}
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
	hasLatch := false
	// jupm from body to header; when haven't third expression
	next := header

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
			// new latch block for third expression
			latch := f.newBasicBlock("loop.latch")
			// jump from body to latch
			next = latch
			hasLatch = true
		}
	}

	//  build body
	if b, ok := stmt.Block().(*yak.BlockContext); ok {
		f.currentBlock = body

		f.target = &target{
			tail:      f.target, // push
			_break:    exit,
			_continue: next,
		}

		f.buildBlock(b) // block can create block

		f.target = f.target.tail // pop

		// f.currentBlock is end block in body
		body = f.currentBlock
	}

	f.currentBlock = body
	f.emitJump(next)

	if hasLatch {
		// build third expression
		f.currentBlock = next
		f.buildForThirdExpr(endThird)
		// jump from latch to header
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
		values := f.buildExpressionList(list)
		f.emitReturn(values)
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
	if _, ok := stmt.ContinueStmt().(*yak.ContinueStmtContext); ok {
		if c := f.target._continue; c != nil {
			f.emitJump(c)
		} else {
			panic("unexpection continue stmt")
		}
	}
	if _, ok := stmt.BreakStmt().(*yak.BreakStmtContext); ok {
		if b := f.target._break; b != nil {
			f.emitJump(b)
		} else {
			panic("unexpection break stmt")
		}
	}

	if s, ok := stmt.ReturnStmt().(*yak.ReturnStmtContext); ok {
		f.buildReturn(s)
	}
}

func (pkg *Package) build() {
	main := pkg.NewFunction("yak-main")
	main.build(pkg.ast)
	main.Finish()
}

func (pkg *Package) Build() { pkg.buildOnece.Do(pkg.build) }

func (prog *Program) Build() {
	for _, pkg := range prog.Packages {
		pkg.Build()
	}
}
