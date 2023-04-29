package yakast

import (
	"github.com/google/uuid"
	yak "yaklang/common/yak/antlr4yak/parser"
	"yaklang/common/yak/antlr4yak/yakvm"
)

func (y *YakCompiler) VisitTryStmt(raw yak.ITryStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i, _ := raw.(*yak.TryStmtContext)
	if i == nil {
		return nil
	}
	y.writeString("try ")
	recoverFormatBufferFunc := y.switchFormatBuffer()
	// Assign 错误
	recoverSymbolTableAndScope := y.SwitchSymbolTableInNewScope("try-catch-finally", uuid.New().String())

	var id = -1
	var text string
	if identifier := i.Identifier(); identifier != nil {
		text = identifier.GetText()
		id1, err := y.currentSymtbl.NewSymbolWithReturn(text)
		if err != nil {
			y.panicCompilerError(CreateSymbolError, text)
		}
		id = id1
	}

	// 捕获 try block 中可能出现的的异常
	catchErrorOpCode := y.pushOperator(yakvm.OpCatchError) //开始捕获error
	y.tryDepthStack.Push(y.GetNextCodeIndex())
	y.VisitBlock(i.Block(0))
	y.tryDepthStack.Pop()
	y.pushOperator(yakvm.OpStopCatchError) // 结束捕获error
	jmp1 := y.pushJmp()                    // 执行 try block 后跳转到 finally block
	y.writeString(recoverFormatBufferFunc())
	y.writeString(" catch ")
	if text != "" {
		y.writeString(text + " ")
	}
	recoverFormatBufferFunc = y.switchFormatBuffer()
	catchErrorOpCode.Op1 = yakvm.NewAutoValue(y.GetCodeIndex()) // 捕获到异常后跳转到 catch block
	catchErrorOpCode.Op2 = yakvm.NewAutoValue(id)
	y.VisitBlock(i.Block(1)) // catch block
	y.writeString(recoverFormatBufferFunc())
	jmp1.Unary = y.GetNextCodeIndex()
	if finallyBlock := i.Block(2); finallyBlock != nil {
		y.writeString("finally")
		recoverFormatBufferFunc = y.switchFormatBuffer()
		y.VisitBlock(finallyBlock)
		y.writeString(recoverFormatBufferFunc())
	}
	recoverSymbolTableAndScope()
	return nil
}
