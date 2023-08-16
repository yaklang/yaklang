package ssa

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type builder struct {
	*Function
	next   *builder
	target *target // for break and continue
}

// entry point
func (b *builder) build(ast *yak.YaklangParser) {
	// ast.StatementList()
	b.buildStatementList(ast.StatementList().(*yak.StatementListContext))
}

// statement list
func (b *builder) buildStatementList(stmtlist *yak.StatementListContext) {
	recover := b.SetRange(stmtlist.BaseParserRuleContext)
	defer recover()

	for _, stmt := range stmtlist.AllStatement() {
		if stmt, ok := stmt.(*yak.StatementContext); ok {
			b.buildStatement(stmt)
		}
	}
}

func (b *builder) buildStatement(stmt *yak.StatementContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	// declear Variable Expression
	if s, ok := stmt.DeclearVariableExpressionStmt().(*yak.DeclearVariableExpressionStmtContext); ok {
		b.buildDeclearVariableExpressionStmt(s)
	}

	// assign Expression
	if s, ok := stmt.AssignExpressionStmt().(*yak.AssignExpressionStmtContext); ok {
		b.buildAssignExpressionStmt(s)
		return
	}

	// expression
	if s, ok := stmt.ExpressionStmt().(*yak.ExpressionStmtContext); ok {
		b.buildExpressionStmt(s)
	}

	// block
	if s, ok := stmt.Block().(*yak.BlockContext); ok {
		b.buildBlock(s)
	}

	//TODO: try Stmt

	// if stmt
	if s, ok := stmt.IfStmt().(*yak.IfStmtContext); ok {
		b.buildIfStmt(s, nil)
	}

	if s, ok := stmt.SwitchStmt().(*yak.SwitchStmtContext); ok {
		b.buildSwitchStmt(s)
	}

	//TODO: for range stmt

	// for stmt
	if s, ok := stmt.ForStmt().(*yak.ForStmtContext); ok {
		b.buildForStmt(s)
	}

	// break stmt
	if _, ok := stmt.BreakStmt().(*yak.BreakStmtContext); ok {
		if _break := b.target._break; _break != nil {
			b.emitJump(_break)
		} else {
			panic("unexpection break stmt")
		}
	}
	// return stmt
	if s, ok := stmt.ReturnStmt().(*yak.ReturnStmtContext); ok {
		b.buildReturnStmt(s)
	}
	// continue stmt
	if _, ok := stmt.ContinueStmt().(*yak.ContinueStmtContext); ok {
		if _continue := b.target._continue; _continue != nil {
			b.emitJump(_continue)
		} else {
			panic("unexpection continue stmt")
		}
	}

	if _, ok := stmt.FallthroughStmt().(*yak.FallthroughStmtContext); ok {
		if _fall := b.target._fallthrough; _fall != nil {
			b.emitJump(_fall)
		} else {
			panic("unexpection fallthrough stmt")
		}

	}
	//TODO: include stmt
	//TODO: defer stmt
	//TODO: go stmt
	//TODO: assert stmt

}

//TODO: try stmt

// expression stmt
func (b *builder) buildExpressionStmt(stmt *yak.ExpressionStmtContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	if s, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		b.buildExpression(s)
	}
}

// assign expression stmt
func (b *builder) buildAssignExpressionStmt(stmt *yak.AssignExpressionStmtContext) {
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
// TODO: go stmt
// return stmt
func (b *builder) buildReturnStmt(stmt *yak.ReturnStmtContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	if list, ok := stmt.ExpressionList().(*yak.ExpressionListContext); ok {
		values := b.buildExpressionList(list)
		b.emitReturn(values)
	} else {
		b.emitReturn(nil)
	}
}

// for stmt
func (b *builder) buildForStmt(stmt *yak.ForStmtContext) {
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
	enter := b.currentBlock
	header := b.newBasicBlockUnSealed("loop.header")

	body := b.newBasicBlock("loop.body")
	exit := b.newBasicBlock("loop.exit")
	latch := b.newBasicBlock("loop.latch")
	var endThird *yak.ForThirdExprContext
	endThird = nil

	var cond Value
	if e, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		// if only expression; just build expression in header;
		cond = b.buildExpression(e)
	} else if condition, ok := stmt.ForStmtCond().(*yak.ForStmtCondContext); ok {
		if first, ok := condition.ForFirstExpr().(*yak.ForFirstExprContext); ok {
			// first expression is initialization, in enter block
			b.currentBlock = enter
			recover := b.SetRange(first.BaseParserRuleContext)
			b.ForExpr(first)
			recover()
		}
		if expr, ok := condition.Expression().(*yak.ExpressionContext); ok {
			// build expression in header
			b.currentBlock = header
			cond = b.buildExpression(expr)
		} else {
			// not found expression; default is true
			cond = NewConst(true)
		}

		if third, ok := condition.ForThirdExpr().(*yak.ForThirdExprContext); ok {
			// third exprssion in latch block, when loop.body builded
			endThird = third
		}
	}
	// jump enter->header
	b.currentBlock = enter
	b.emitJump(header)
	// build if in header end; to exit or body
	b.currentBlock = header
	ifssa := b.emitIf(cond)
	ifssa.AddFalse(exit)
	ifssa.AddTrue(body)

	//  build body
	b.currentBlock = body
	if block, ok := stmt.Block().(*yak.BlockContext); ok {

		b.target = &target{
			tail:      b.target, // push
			_break:    exit,
			_continue: latch,
		}

		b.buildBlock(block)      // block can create block
		b.target = b.target.tail // pop
		// // f.currentBlock is end block in body
		// body = b.currentBlock
	}
	// jump body -> latch
	b.emitJump(latch)

	// build latch
	b.currentBlock = latch
	if endThird != nil {
		// build third expression in loop.body end
		recover := b.SetRange(endThird.BaseParserRuleContext)
		b.ForExpr(endThird)
		recover()
	}
	// jump latch -> header
	b.emitJump(header)

	// now header sealed
	header.Sealed()

	rest := b.newBasicBlock("")
	// jump exit -> rest
	b.currentBlock = exit
	b.emitJump(rest)
	// continue in rest code
	b.currentBlock = rest
}

type forExpr interface {
	Expression() yak.IExpressionContext
	AssignExpression() yak.IAssignExpressionContext
}

func (b *builder) ForExpr(stmt forExpr) {
	if ae, ok := stmt.AssignExpression().(*yak.AssignExpressionContext); ok {
		b.buildAssignExpression(ae)
	}
	if e, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		b.buildExpression(e)
	}
}

//TODO: for range stmt

// switch stmt
func (b *builder) buildSwitchStmt(stmt *yak.SwitchStmtContext) {
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
	var cond Value
	if expr, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		cond = b.buildExpression(expr)
	} else {
		// expression is nil
	}
	enter := b.currentBlock
	allcase := stmt.AllCase()
	slabel := make([]switchlabel, 0, len(allcase))
	handlers := make([]*BasicBlock, 0, len(allcase))
	done := b.newBasicBlock("switch.done")
	defaultb := b.newBasicBlock("switch.default")
	enter.AddSucc(defaultb)

	// handler label
	for i := range allcase {
		if exprlist, ok := stmt.ExpressionList(i).(*yak.ExpressionListContext); ok {
			exprs := b.buildExpressionList(exprlist)
			handler := b.newBasicBlock("switch.handler")
			enter.AddSucc(handler)
			handlers = append(handlers, handler)
			if len(exprs) == 1 {
				// only one expr
				slabel = append(slabel, switchlabel{
					exprs[0], handler,
				})

			} else {
				for _, expr := range exprs {
					slabel = append(slabel, switchlabel{
						expr, handler,
					})
				}
			}
		}
	}
	// build body
	for i := range allcase {
		if stmtlist, ok := stmt.StatementList(i).(*yak.StatementListContext); ok {
			b.target = &target{
				tail:      b.target,
				_break:    nil,
				_continue: nil,
			}
			if i == len(allcase)-1 {
				b.target._fallthrough = defaultb
			} else {
				b.target._fallthrough = handlers[i+1]
			}
			b.currentBlock = handlers[i]
			b.buildStatementList(stmtlist)
			b.emitJump(done)
			b.target = b.target.tail
		}
	}
	// default
	if stmt.Default() != nil {
		if stmtlist, ok := stmt.StatementList(len(allcase)).(*yak.StatementListContext); ok {
			b.target = &target{
				tail:         b.target,
				_break:       nil,
				_continue:    nil,
				_fallthrough: nil,
			}
			b.currentBlock = defaultb
			b.buildStatementList(stmtlist)
			b.emitJump(done)
			b.target = b.target.tail
		}
	}

	b.currentBlock = enter
	b.emitSwitch(cond, defaultb, slabel)
	rest := b.newBasicBlock("")
	b.currentBlock = done
	b.emitJump(rest)
	b.currentBlock = rest
}

// if stmt
func (b *builder) buildIfStmt(stmt *yak.IfStmtContext, done *BasicBlock) {
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
	ifssa := b.emitIf(cond)
	isOutIf := false
	if done == nil {
		done = b.newBasicBlock("if.done")
		isOutIf = true
	}

	// create true block
	trueBlock := b.newBasicBlock("if.true")
	ifssa.AddTrue(trueBlock)

	// build true block
	b.currentBlock = trueBlock
	if blockstmt, ok := stmt.Block(0).(*yak.BlockContext); ok {
		b.buildBlock(blockstmt)
	}
	// b.buildBlock(stmt.Block(0).(*yak.BlockContext))
	b.emitJump(done)

	// handler "elif"
	previf := ifssa
	// add elif block to prev-if false
	for index := range stmt.AllElif() {
		// create false block
		if previf.False == nil {
			previf.AddFalse(b.newBasicBlock("if.elif"))
		}
		// in false block
		b.currentBlock = previf.False
		// build condition
		if condstmt, ok := stmt.Expression(index + 1).(*yak.ExpressionContext); ok {
			recover := b.SetRange(condstmt.BaseParserRuleContext)
			cond := b.buildExpression(condstmt)
			// if instruction
			currentif := b.emitIf(cond)
			// create true block
			trueBlock := b.newBasicBlock("if.true")
			currentif.AddTrue(trueBlock)
			// build true block
			b.currentBlock = trueBlock
			if blockstmt, ok := stmt.Block(index + 1).(*yak.BlockContext); ok {
				b.buildBlock(blockstmt)
			}
			// jump to done
			b.emitJump(done)
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
			falseBlock := b.newBasicBlock("if.false")
			previf.AddFalse(falseBlock)

			// build false block
			b.currentBlock = falseBlock
			b.buildBlock(elseblock)
			b.emitJump(done)
		} else if elifstmt, ok := elseStmt.IfStmt().(*yak.IfStmtContext); ok {
			// "else if"
			// create elif block
			elifBlock := b.newBasicBlock("if.elif")
			previf.AddFalse(elifBlock)

			// build elif block
			b.currentBlock = elifBlock
			b.buildIfStmt(elifstmt, done)
		}
	} else {
		previf.AddFalse(done)
	}
	b.currentBlock = done
	if isOutIf {
		// in exit if; set rest block
		rest := b.newBasicBlock("")
		b.emitJump(rest)

		// continue rest code
		b.currentBlock = rest
	}
}

// block
func (b *builder) buildBlock(stmt *yak.BlockContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	if s, ok := stmt.StatementList().(*yak.StatementListContext); ok {
		b.buildStatementList(s)
	}
}

type assiglist interface {
	AssignEq() antlr.TerminalNode
	ColonAssignEq() antlr.TerminalNode
	ExpressionList() yak.IExpressionListContext
	LeftExpressionList() yak.ILeftExpressionListContext
}

func (b *builder) AssignList(stmt assiglist) {
	if op, op2 := stmt.AssignEq(), stmt.ColonAssignEq(); op != nil || op2 != nil {
		// right value
		var rvalues []Value
		if ri, ok := stmt.ExpressionList().(*yak.ExpressionListContext); ok {
			rvalues = b.buildExpressionList(ri)
		}

		// left
		var lvalues []LeftValue
		if li, ok := stmt.LeftExpressionList().(*yak.LeftExpressionListContext); ok {
			lvalues = b.buildLeftExpressionList(op2 != nil, li)
		}

		// assign
		// (n) = (n), just assign
		if len(rvalues) == len(lvalues) {
			for i := range rvalues {
				lvalues[i].Assign(rvalues[i], b.Function)
			}
		} else if len(rvalues) == 1 {
			// (n) = (1)
			// (n) = field(1, #index)
			for i, lv := range lvalues {
				field := b.emitField(rvalues[0], NewConst(i))
				lv.Assign(field, b.Function)
			}
		} else if len(lvalues) == 1 {
			// (1) = (n)
			// (1) = interface(n)
			lValueLen := NewConst(len(lvalues))
			typ := ParseInterfaceTypes(rvalues)
			_interface := b.emitInterfaceBuildWithType(typ, lValueLen, lValueLen)
			lvalues[0].Assign(_interface, b.Function)
			for i, rv := range rvalues {
				field := b.emitField(_interface, NewConst(i))
				b.emitUpdate(field, rv)
			}
		} else {
			// (n) = (m) && n!=m  faltal
			panic(fmt.Sprintf("multi-assign failed: left value length[%d] != right value length[%d]", len(lvalues), len(rvalues)))
		}
	}
}

// assign expression
func (b *builder) buildAssignExpression(stmt *yak.AssignExpressionContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	b.AssignList(stmt)

	if stmt.PlusPlus() != nil { // ++
		lvalue := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		rvalue := b.emitArith(yakvm.OpAdd, lvalue.GetValue(b.Function), NewConst(1))
		lvalue.Assign(rvalue, b.Function)
	} else if stmt.SubSub() != nil { // --
		lvalue := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
		rvalue := b.emitArith(yakvm.OpSub, lvalue.GetValue(b.Function), NewConst(1))
		lvalue.Assign(rvalue, b.Function)
	}

	if op, ok := stmt.InplaceAssignOperator().(*yak.InplaceAssignOperatorContext); ok {
		rvalue := b.buildExpression(stmt.Expression().(*yak.ExpressionContext))
		lvalue := b.buildLeftExpression(false, stmt.LeftExpression().(*yak.LeftExpressionContext))
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
		rvalue = b.emitArith(opcode, lvalue.GetValue(b.Function), rvalue)
		lvalue.Assign(rvalue, b.Function)
	}
}

// declear variable expression
func (b *builder) buildDeclearVariableExpressionStmt(stmt *yak.DeclearVariableExpressionStmtContext) {
	// recover := b.SetRange(stmt.BaseParserRuleContext)
	// defer recover()
	if s, ok := stmt.DeclearVariableExpression().(*yak.DeclearVariableExpressionContext); ok {
		b.buildDeclearVariableExpression(s)
	}
}

func (b *builder) buildDeclearVariableExpression(stmt *yak.DeclearVariableExpressionContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	if s, ok := stmt.DeclearVariableOnly().(*yak.DeclearVariableOnlyContext); ok {
		b.buildDeclearVariableOnly(s)
	}
	if s, ok := stmt.DeclearAndAssignExpression().(*yak.DeclearAndAssignExpressionContext); ok {
		b.buildDeclearAndAssignExpression(s)
	}
}

func (b *builder) buildDeclearVariableOnly(stmt *yak.DeclearVariableOnlyContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	// TODO: how handler this ?
	for _, id := range stmt.AllIdentifier() {
		b.writeVariable(id.GetText(), nil)
	}
}

func (b *builder) buildDeclearAndAssignExpression(stmt *yak.DeclearAndAssignExpressionContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	b.AssignList(stmt)
}

// left expression list
func (b *builder) buildLeftExpressionList(forceAssign bool, stmt *yak.LeftExpressionListContext) []LeftValue {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	exprs := stmt.AllLeftExpression()
	valueLen := len(exprs)
	values := make([]LeftValue, valueLen)
	for i, e := range exprs {
		if e, ok := e.(*yak.LeftExpressionContext); ok {
			values[i] = b.buildLeftExpression(forceAssign, e)
		}
	}
	return values
}

// left  expression
func (b *builder) buildLeftExpression(forceAssign bool, stmt *yak.LeftExpressionContext) LeftValue {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	if s := stmt.Identifier(); s != nil {
		if v := b.readVariable(s.GetText()); v != nil {
			// when v exist
			switch v := v.(type) {
			case *Field:
				if v.outCapture {
					return v
				}
			case *Parameter:
			default:
			}
		} else if !forceAssign && b.CanBuildFreeValue(s.GetText()) {
			field := b.parent.newField(s.GetText())
			field.outCapture = true
			b.FreeValues = append(b.FreeValues, field)
			b.SetReg(field)
			b.parent.writeVariable(s.GetText(), field)
			b.writeVariable(s.GetText(), field)
			return field
		}
		return &IdentifierLV{
			variable: s.GetText(),
		}
	}
	if s, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		expr := b.buildExpression(s)
		if expr == nil {
			panic("leftexpression expression is nil")
		}

		if s, ok := stmt.LeftSliceCall().(*yak.LeftSliceCallContext); ok {
			index := b.buildLeftSliceCall(s)
			if expr, ok := expr.(*Interface); ok {
				return b.emitField(expr, index)
			} else {
				panic("leftexprssion exprssion is not interface")
			}
		}

		//TODO: leftMemberCall
	}
	return nil
}

//TODO: left member call

// left slice call
func (b *builder) buildLeftSliceCall(stmt *yak.LeftSliceCallContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	if s, ok := stmt.Expression().(*yak.ExpressionContext); ok {
		return b.buildExpression(s)
	}
	return nil
}

// expression
func (b *builder) buildExpression(stmt *yak.ExpressionContext) Value {
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
		if ret := b.readVariable(text); ret != nil {
			return ret
		} else if b.CanBuildFreeValue(text) {
			return b.BuildFreeValue(text)
		} else {
			fmt.Printf("warn!! Expression: undefine value %v\n", s.GetText())
			return nil
		}
	}

	getValue := func(index int) Value {
		if s, ok := stmt.Expression(index).(*yak.ExpressionContext); ok {
			return b.buildExpression(s)
		}
		return nil
	}

	//TODO: member call

	// slice call
	if s, ok := stmt.SliceCall().(*yak.SliceCallContext); ok {
		expr, ok := getValue(0).(*Interface)
		if !ok {
			panic("expression slice need expression")
		}
		keys := b.buildSliceCall(s)
		if len(keys) == 1 {
			return b.emitField(expr, keys[0])
		} else if len(keys) == 2 {
			return b.emitInterfaceSlice(expr, keys[0], keys[1], nil)
		} else if len(keys) == 3 {
			return b.emitInterfaceSlice(expr, keys[0], keys[1], keys[2])
		} else {
			panic("")
		}
	}

	// function call
	if s, ok := stmt.FunctionCall().(*yak.FunctionCallContext); ok {
		return b.emitCall(b.buildFunctionCallWarp(stmt, s))
	}

	//TODO: parent expression

	// instance code
	if s, ok := stmt.InstanceCode().(*yak.InstanceCodeContext); ok {
		return b.emitCall(b.buildInstanceCode(s))
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
			panic("additive binary operator need two expression")
		}
		var opcode yakvm.OpcodeFlag
		switch op.GetText() {
		// BitBinaryOperator
		case "<<":
			opcode = yakvm.OpShl
		case ">>":
			opcode = yakvm.OpShr
		case "&":
			opcode = yakvm.OpAnd
		case "&^":
			opcode = yakvm.OpAndNot
		case "|":
			opcode = yakvm.OpOr
		case "^":
			opcode = yakvm.OpXor

		// AdditiveBinaryOperator
		case "+":
			opcode = yakvm.OpAdd
		case "-":
			opcode = yakvm.OpSub

		// MultiplicativeBinaryOperator
		case "*":
			opcode = yakvm.OpMul
		case "/":
			opcode = yakvm.OpDiv
		case "%":
			opcode = yakvm.OpMod

		// ComparisonBinaryOperator
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
		return b.emitArith(opcode, op0, op1)
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
func (b *builder) buildMakeExpression(stmt *yak.MakeExpressionContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	var typ Type
	if s, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
		typ = b.buildTypeLiteral(s)
	}
	if typ == nil {
		panic("")
	}

	var exprs []Value
	if s, ok := stmt.ExpressionListMultiline().(*yak.ExpressionListMultilineContext); ok {
		exprs = b.buildExpressionListMultiline(s)
	}
	zero := NewConst(0)
	switch typ.(type) {
	case *SliceType:
		// fmt.Printf("debug %s %v %d\n", "make slice", typ, num)
		if len(exprs) == 0 {
			return b.emitInterfaceBuildWithType(Types{typ}, zero, zero)
		} else if len(exprs) == 1 {
			return b.emitInterfaceBuildWithType(Types{typ}, exprs[0], exprs[0])
		} else if len(exprs) == 2 {
			return b.emitInterfaceBuildWithType(Types{typ}, exprs[0], exprs[1])
		} else {
			panic("make slice expression error!")
		}
	case *MapType:
		fmt.Printf("debug %v\n", "make map")
	case *ChanType:
		fmt.Printf("debug %v\n", "make chan")
	default:
		panic("make unknow type")
	}
	return nil
}

// type literal
func (b *builder) buildTypeLiteral(stmt *yak.TypeLiteralContext) Type {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	text := stmt.GetText()
	// var type name
	if b, ok := basicTypesStr[text]; ok {
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
				return NewChanType(Types{typ})
			}
		}
	}

	return nil
}

// slice type literal
func (b *builder) buildSliceTypeLiteral(stmt *yak.SliceTypeLiteralContext) Type {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	if s, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
		if eleTyp := b.buildTypeLiteral(s); eleTyp != nil {
			return NewSliceType(Types{eleTyp})
		}
	}
	return nil
}

// map type literal
func (b *builder) buildMapTypeLiteral(stmt *yak.MapTypeLiteralContext) Type {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	// key
	var keyTyp Type
	var valueTyp Type
	if s, ok := stmt.TypeLiteral(0).(*yak.TypeLiteralContext); ok {
		keyTyp = b.buildTypeLiteral(s)
	}

	// value
	if s, ok := stmt.TypeLiteral(1).(*yak.TypeLiteralContext); ok {
		valueTyp = b.buildTypeLiteral(s)
	}
	if keyTyp != nil && valueTyp != nil {
		return NewMapType(Types{keyTyp}, Types{valueTyp})
	}

	return nil
}

// instance code
func (b *builder) buildInstanceCode(stmt *yak.InstanceCodeContext) *Call {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	newfunc := b.Package.NewFunctionWithParent("", b.Function)
	b = &builder{
		Function: newfunc,
		next:     b,
	}

	if block, ok := stmt.Block().(*yak.BlockContext); ok {
		b.buildBlock(block)
	}

	b.Finish()
	b = b.next
	return b.newCall(newfunc, nil, false)
}

// anonymous function decl
func (b *builder) buildAnonymouseFunctionDecl(stmt *yak.AnonymousFunctionDeclContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	funcName := ""
	if name := stmt.FunctionNameDecl(); name != nil {
		funcName = name.GetText()
	}
	newfunc := b.Package.NewFunctionWithParent(funcName, b.Function)
	b = &builder{
		Function: newfunc,
		next:     b, // push
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
			newfunc.NewParam(stmt.Identifier().GetText())
		}
		if block, ok := stmt.Block().(*yak.BlockContext); ok {
			// build block
			b.buildBlock(block)
		} else if expression, ok := stmt.Expression().(*yak.ExpressionContext); ok {
			// hanlder expression
			v := b.buildExpression(expression)
			b.emitReturn([]Value{v})
		} else {
			panic("BUG: arrow function need expression or block at least")
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
	b = b.next // pop

	if funcName != "" {
		b.writeVariable(funcName, newfunc)
	}
	return newfunc
}

// function param decl
func (b *builder) buildFunctionParamDecl(stmt *yak.FunctionParamDeclContext) {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	ellipsis := stmt.Ellipsis() // if has "...",  use array pass this argument
	ids := stmt.AllIdentifier()

	for _, id := range ids {
		b.NewParam(id.GetText())
	}
	if ellipsis != nil {
		// handler "..." to array
		b.Param[len(b.Param)-1].typs = append(b.Param[len(b.Param)-1].typs, NewSliceType(nil))
		b.hasEllipsis = true
	}
}

func (b *builder) buildFunctionCallWarp(exprstmt *yak.ExpressionContext, stmt *yak.FunctionCallContext) *Call {
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
	panic("call target is nil")
}

// function call
func (b *builder) buildFunctionCall(stmt *yak.FunctionCallContext, v Value) *Call {
	// recover := b.SetRange(stmt.BaseParserRuleContext)
	// defer recover()
	var args []Value
	isDropErr := false
	if s, ok := stmt.OrdinaryArguments().(*yak.OrdinaryArgumentsContext); ok {
		args = b.buildOrdinaryArguments(s)
	}
	if stmt.Wavy() != nil {
		isDropErr = true
	}
	return b.newCall(v, args, isDropErr)
}

// ordinary argument
func (b *builder) buildOrdinaryArguments(stmt *yak.OrdinaryArgumentsContext) []Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	ellipsis := stmt.Ellipsis()
	allexpre := stmt.AllExpression()
	v := make([]Value, 0, len(allexpre))
	for _, expr := range allexpre {
		v = append(v, b.buildExpression(expr.(*yak.ExpressionContext)))
	}
	if ellipsis != nil {
		//handler "..." to array
		typ := v[len(v)-1].GetType()
		typ = append(typ, NewSliceType(nil))
		v[len(v)-1].SetType(typ)
	}
	return v
}

// TODO: member call

// slice call
func (b *builder) buildSliceCall(stmt *yak.SliceCallContext) []Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	exprs := stmt.AllExpression()
	values := make([]Value, len(exprs))
	if len(exprs) == 0 {
		panic("slicecall expression is zero")
	}
	if len(exprs) > 3 {
		panic("slicecall expression too much")
	}
	for i, expr := range exprs {
		if s, ok := expr.(*yak.ExpressionContext); ok {
			values[i] = b.buildExpression(s)
		}
	}
	return values
}

func (b *builder) buildLiteral(stmt *yak.LiteralContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	//TODO: template stirng literal

	// string literal
	if s, ok := stmt.StringLiteral().(*yak.StringLiteralContext); ok {
		return b.buildStringLiteral(s)
	}

	// numeric literal
	if s, ok := stmt.NumericLiteral().(*yak.NumericLiteralContext); ok {
		return b.buildNumericLiteral(s)
	}

	//TODO: bool literal
	//TODO: undefined literal
	//TODO: charater literal
	//TODO: map literal
	//TODO: slice typed literal
	//TODO: type literal
	//TODO: slice literal

	return nil
}

// numeric literal
func (b *builder) buildNumericLiteral(stmt *yak.NumericLiteralContext) Value {
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
			fmt.Printf("cannot parse `%s` as integer literal... is too large for int64:%v", originIntStr, err)
		}
		return NewConst(resultInt64)
	}

	// float literal
	if iFloat := stmt.FloatLiteral(); iFloat != nil {
		lit := iFloat.GetText()
		if strings.HasPrefix(lit, ".") {
			lit = "0" + lit
		}
		var f, _ = strconv.ParseFloat(lit, 64)
		return NewConst(f)
	}
	fmt.Printf("cannot parse num for literal: %s", stmt.GetText())
	return nil
}

// string literal
func (b *builder) buildStringLiteral(stmt *yak.StringLiteralContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	var text = stmt.GetText()
	if text == "" {
		return NewConst(text)
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
			return NewConstWithUnary(val, int(prefix))
		} else {
			val, err := strconv.Unquote(text)
			if err != nil {
				fmt.Printf("parse %v to stirng literal fieled: %s", stmt.GetText(), err.Error())
			}
			return NewConstWithUnary(val, int(prefix))
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
			return NewConstWithUnary(val, int(prefix))

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
			return NewConstWithUnary(val, int(prefix))
		}
	case '`':
		val := text[1 : len(text)-1]
		return NewConstWithUnary(val, int(prefix))
	case '0':
		switch text[1] {
		case 'h':
			text = text[2:]
			hex, err := codec.DecodeHex(text)
			if err != nil {
				fmt.Printf("parse hex string error: %v", err)
			}
			return NewConst(hex)
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
func (b *builder) buildExpressionList(stmt *yak.ExpressionListContext) []Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	exprs := stmt.AllExpression()
	valueLen := len(exprs)
	values := make([]Value, valueLen)
	for i, e := range exprs {
		if e, ok := e.(*yak.ExpressionContext); ok {
			values[i] = b.buildExpression(e)
		}
	}
	return values
}

// expression list multiline
func (b *builder) buildExpressionListMultiline(stmt *yak.ExpressionListMultilineContext) []Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	allexpr := stmt.AllExpression()
	exprs := make([]Value, 0, len(allexpr))
	for _, expr := range allexpr {
		if expr, ok := expr.(*yak.ExpressionContext); ok {
			exprs = append(exprs, b.buildExpression(expr))
		}
	}
	return exprs
}

func (pkg *Package) build() {
	main := pkg.NewFunction("yak-main")
	b := builder{
		Function: main,
		next:     nil,
		target:   nil,
	}
	b.build(pkg.ast)
	b.Finish()
}

func (pkg *Package) Build() { pkg.buildOnece.Do(pkg.build) }

func (prog *Program) Build() {
	for _, pkg := range prog.Packages {
		pkg.Build()
	}
}
