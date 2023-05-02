package yakast

import (
	yak "yaklang.io/yaklang/common/yak/antlr4yak/parser"
	"yaklang.io/yaklang/common/yak/antlr4yak/yakvm"

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
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("if ")

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
	ifBlock := i.Block(0)
	if ifBlock == nil {
		y.panicCompilerError(compileError, "no if code block")
	}
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
