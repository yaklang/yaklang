package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

	"github.com/google/uuid"
)

func (y *YakCompiler) VisitIfStmt(raw yak.IIfStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.IfStmtContext)
	if i == nil {
		return nil
	}

	ifBlock := i.Block(0)
	if ifBlock == nil {
		y.panicCompilerError(compileError, "no if code block")
	}
	recoverRange := y.SetRange(ifBlock)
	defer recoverRange()
	y.writeString("if ")

	// if 初始化语句：if <init>; <cond> { ... }
	// 初始化语句声明的变量需要在整个 if/elif/else 链中可见，但不能泄漏到外层作用域，
	// 因此在这里创建一个外层作用域包裹初始化语句以及后续所有分支。
	// 关键词: if 初始化语句, if init statement, Go 风格 if
	var initScopeRecover func()
	if initStmt := i.IfStmtInit(); initStmt != nil {
		initScopeRecover = y.SwitchSymbolTableInNewScope("if-init", uuid.New().String())
		y.VisitIfStmtInit(initStmt)
		y.writeString("; ")
	}
	if initScopeRecover != nil {
		defer initScopeRecover()
	}

	tableRecover := y.SwitchSymbolTableInNewScope("if", uuid.New().String())

	ifCond := i.Expression(0)
	if ifCond == nil {
		y.panicCompilerError(compileError, "no if condition")
	}
	y.VisitExpression(ifCond)
	y.writeString(" ")

	// if 条件为真，执行 if 语句块
	// 使用 jmpf 来实现，如果 pop stack 之后的值被认为是 false，则跳转
	var jmpfCode = y.pushJmpIfFalse()

	// 编译 block

	y.VisitBlock(ifBlock)

	var jmpToEnd []*yakvm.Code
	jmpToEnd = append(jmpToEnd, y.pushJmp())

	// 为 jmpf 实现操作
	jmpfCode.Unary = y.GetNextCodeIndex()
	tableRecover()
	for index := range i.AllElif() {
		tableRecover = y.SwitchSymbolTableInNewScope("elif", uuid.New().String())
		// elif 和 if 逻辑是一摸一样的，读一个 expression
		// 然后使用 jmpf 跳转
		// 但是 elif 有一个特殊的地方，就是需要在 if 语句块执行完毕之后
		// 跳过 elif 语句块，所以需要在 elif 语句块的最后添加一个 jmp 指令
		y.writeStringWithWhitespace("elif")
		y.VisitExpression(i.Expression(index + 1))
		y.writeString(" ")
		var jmpfCode = y.pushJmpIfFalse()
		y.VisitBlock(i.Block(index + 1))
		jmpToEnd = append(jmpToEnd, y.pushJmp())

		jmpfCode.Unary = y.GetNextCodeIndex()
		tableRecover()
	}

	// 为 else 设置好结尾符
	if ielseBlock := i.ElseBlock(); ielseBlock != nil {
		elseBlock := ielseBlock.(*yak.ElseBlockContext)
		y.writeStringWithWhitespace("else")
		block := elseBlock.Block()
		elseIf := elseBlock.IfStmt()
		if block != nil {
			tableRecover = y.SwitchSymbolTableInNewScope("else", uuid.New().String())
			y.VisitBlock(block)
			tableRecover()
		} else if elseIf != nil {
			y.VisitIfStmt(elseIf)
		}

	}

	endCode := y.GetCodeIndex()
	for _, jmp := range jmpToEnd {
		jmp.Unary = endCode
	}

	return nil
}

// VisitIfStmtInit 编译 if 初始化语句（if <init>; <cond> { ... }）。
// 初始化语句可以是赋值语句、变量声明或普通表达式。
// 关键词: if 初始化语句, if init statement
func (y *YakCompiler) VisitIfStmtInit(raw yak.IIfStmtInitContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.IfStmtInitContext)
	if i == nil {
		return nil
	}

	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	// 变量声明：if var a = f(); a != nil { ... }
	if s := i.DeclareVariableExpression(); s != nil {
		y.VisitDeclareVariableExpression(s)
		return nil
	}

	// 赋值语句：if err = f(); err != nil { ... }
	if s := i.AssignExpression(); s != nil {
		y.VisitAssignExpression(s)
		return nil
	}

	// 普通表达式：if f(); cond { ... }
	// 表达式求值后会压栈，需要 pop 保持栈平衡
	if s := i.Expression(); s != nil {
		y.VisitExpression(s)
		y.pushOpPop()
		return nil
	}

	return nil
}
