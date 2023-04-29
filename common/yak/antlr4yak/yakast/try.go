package yakast

import (
	yak "yaklang/common/yak/antlr4yak/parser"
	"yaklang/common/yak/antlr4yak/yakvm"

	"github.com/google/uuid"
)

func (y *YakCompiler) _VisitTryStmt(raw yak.ITryStmtContext) interface{} {

	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.TryStmtContext)
	if i == nil {
		return nil
	}
	var (
		jmpIfFalse                               *yakvm.Code
		catchFormattedCode, finallyFormattedCode string
		idName                                   = "__recover_value__"
		identifier                               = i.Identifier()
	)

	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	tableRecover := y.SwitchSymbolTable("instanceCode", uuid.New().String())
	defer tableRecover()
	recoverCodeFunc := y.SwitchCodes()

	y.writeString("try ")

	// catch block
	recoverFormatBufferFunc := y.switchFormatBuffer()
	recoverCatchCodeFunc := y.SwitchCodes()

	y.VisitBlockWithCallback(i.Block(1), func(y *YakCompiler) {
		if identifier != nil {
			idName = identifier.GetText()
		}

		id, err := y.currentSymtbl.NewSymbolWithReturn(idName)
		if err != nil {
			y.panicCompilerError(forceCreateSymbolFailed, idName)
		}
		y.pushLeftRef(id)
		y.pushOperator(yakvm.OpRecover)

		y.pushOperator(yakvm.OpFastAssign)
		jmpIfFalse = y.pushJmpIfFalse()
	}, false)
	jmpIfFalse.Unary = y.GetNextCodeIndex()
	y.pushOperator(yakvm.OpReturn)
	catchCode := make([]*yakvm.Code, len(y.codes))
	copy(catchCode, y.codes)
	recoverCatchCodeFunc()
	catchFormattedCode = recoverFormatBufferFunc()

	finallyBlock := i.Block(2)
	if finallyBlock != nil {
		recoverFormatBufferFunc = y.switchFormatBuffer()
		recoverFinallyCodeFunc := y.SwitchCodes()
		y.VisitBlock(i.Block(2), false)
		y.pushOperator(yakvm.OpReturn)
		finallyCode := make([]*yakvm.Code, len(y.codes))
		copy(finallyCode, y.codes)
		recoverFinallyCodeFunc()
		finallyFormattedCode = recoverFormatBufferFunc()
		y.pushDefer(finallyCode)
	}

	y.pushDefer(catchCode)
	y.VisitBlock(i.Block(0), false)
	y.pushOperator(yakvm.OpReturn)
	y.writeString(" catch ")
	if identifier != nil {
		y.writeString(idName)
		y.writeString(" ")
	}
	y.writeString(catchFormattedCode)

	if finallyBlock != nil {
		y.writeString(" finally ")
		y.writeString(finallyFormattedCode)
	}

	funcCode := make([]*yakvm.Code, len(y.codes))
	copy(funcCode, y.codes)
	recoverCodeFunc()

	yakFn := yakvm.NewFunction(funcCode, y.currentSymtbl)

	if yakFn == nil {
		y.panicCompilerError(compileError, "cannot create yak function from compiler")
	}

	// 配置函数
	y.pushValue(&yakvm.Value{
		TypeVerbose: "anonymous-function",
		Value:       yakFn,
	})
	y.pushCall(0)
	y.pushOpPop()
	return nil
}
