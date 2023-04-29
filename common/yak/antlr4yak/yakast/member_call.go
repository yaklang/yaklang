package yakast

import (
	"yaklang/common/yak/antlr4yak/parser"
	"yaklang/common/yak/antlr4yak/yakvm"
)

func (y *YakCompiler) VisitMemberCall(raw parser.IMemberCallContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	memberCallContext, ok := raw.(*parser.MemberCallContext)
	if !ok {
		return nil
	}
	i := memberCallContext
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	y.writeString(".")

	if identifier := memberCallContext.Identifier(); identifier != nil {
		idText := identifier.GetText()
		y.writeString(idText)
		y.pushString(idText, identifier.GetText())
		y.pushOperator(yakvm.OpMemberCall)
	} else if identifierWithDollar := memberCallContext.IdentifierWithDollar(); identifierWithDollar != nil {
		idText := identifierWithDollar.GetText()
		y.writeString(idText)
		if sym, ok := y.currentSymtbl.GetSymbolByVariableName(idText[1:]); ok {
			y.pushRef(sym)
			y.pushOperator(yakvm.OpMemberCall)
		} else {
			y.panicCompilerError(notFoundDollarVariable, idText[1:])
		}
	}
	return nil
}
