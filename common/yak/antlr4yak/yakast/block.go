package yakast

import (
	yak "yaklang/common/yak/antlr4yak/parser"
	"strings"

	"github.com/google/uuid"
)

func (y *YakCompiler) PreviewProgram(raw yak.IProgramContext) (int, *yak.StatementContext) {
	if y == nil || raw == nil {
		return -1, nil
	}

	i, _ := raw.(*yak.ProgramContext)
	if i == nil {
		return -1, nil
	}

	return y.PreviewStatementList(i.StatementList())
}

func (y *YakCompiler) VisitBlockWithCallback(raw yak.IBlockContext, callback func(*YakCompiler), inlineOpt ...bool) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.BlockContext)
	if i == nil {
		return nil
	}
	inline := false
	if len(inlineOpt) > 0 {
		inline = inlineOpt[0]
	}

	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	lines, firstStmt := y.PreviewStatementList(i.StatementList())
	var firstStmtIsBlock = false
	if firstStmt != nil {
		firstStmtIsBlock = firstStmt.Block() != nil
	}

	if lines <= 0 {
		inline = true
	}

	y.writeString("{")

	var text string = ""
	if firstStmt != nil {
		text = firstStmt.GetText()
	}
	if lines > 1 || firstStmtIsBlock || len(text) > FORMATTER_RECOMMEND_LINE_LENGTH || strings.Contains(text, "\n") {
		inline = false
	}

	if !inline {
		y.incIndent()
		y.writeNewLine()
	} else {
		y.writeString(" ")
	}
	recoverSymbolTableAndScope := y.SwitchSymbolTableInNewScope("block", uuid.New().String())
	recoverFormatBufferFunc := y.switchFormatBuffer()

	if callback != nil {
		callback(y)
	}
	//y.PreviewProgram()
	//y.VisitProgram()
	y.VisitStatementList(i.StatementList(), inline)
	buf := recoverFormatBufferFunc()
	recoverSymbolTableAndScope()
	buf = strings.Trim(buf, "\n")
	y.writeString(buf)
	if !inline {
		y.decIndent()
		y.writeNewLine()
		y.writeIndent()
	} else {
		y.writeString(" ")
	}
	y.writeString("}")

	return nil
}

func (y *YakCompiler) VisitBlock(raw yak.IBlockContext, inlineOpt ...bool) interface{} {
	return y.VisitBlockWithCallback(raw, nil, inlineOpt...)
}
