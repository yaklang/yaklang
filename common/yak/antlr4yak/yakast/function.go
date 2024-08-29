package yakast

import (
	"unicode/utf8"

	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func (y *YakCompiler) VisitFunctionCall(raw yak.IFunctionCallContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.FunctionCallContext)
	if i == nil {
		return nil
	}

	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("(")

	// 函数调用需要先把参数压栈
	// 调用的时候，call n 表示要取多少数出来
	argCount := 0
	if i.OrdinaryArguments() != nil {
		argCount, _ = y.VisitOrdinaryArguments(i.OrdinaryArguments())
	}
	y.writeString(")")

	if i.Wavy() != nil {
		y.writeString("~")
		y.pushCallWithWavy(argCount)
	} else {
		y.pushCall(argCount)
	}
	return nil
}

func (y *YakCompiler) VisitOrdinaryArguments(raw yak.IOrdinaryArgumentsContext) (int, bool) {
	if y == nil || raw == nil {
		return 0, false
	}

	i, _ := raw.(*yak.OrdinaryArgumentsContext)
	if i == nil {
		return 0, false
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	ellipsis := i.Ellipsis()
	allExpressions := i.AllExpression()
	lenOfAllExpressions := len(allExpressions)

	expressionTokenLengths := make([]int, lenOfAllExpressions)
	// lineLength := i.BaseParserRuleContext.GetStart().GetColumn()
	lineLength := y.currentLineLength
	eachParamOneLine := false
	// 计算每个表达式的长度，如果某个表达式过长或者总体过长，则每个参数一行
	if lenOfAllExpressions > 1 {
		for i, e := range allExpressions {
			expressionTokenLengths[i] = utf8.RuneCountInString(e.GetText())
			lineLength += expressionTokenLengths[i]
			if !eachParamOneLine && (expressionTokenLengths[i] > FORMATTER_RECOMMEND_PARAM_LENGTH || lineLength > FORMATTER_MAXWIDTH) {
				eachParamOneLine = true
			}
		}
	}

	hadIncIndent := false

	for index, expr := range allExpressions {
		lineLength += expressionTokenLengths[index]

		if eachParamOneLine {
			y.writeNewLine()
			if !hadIncIndent {
				y.incIndent()
				hadIncIndent = true
			}
			y.writeIndent()
			lineLength = y.indent*4 + expressionTokenLengths[index]
		}

		y.VisitExpression(expr)

		// 如果是最后一个参数且有...，就要加...
		if index == lenOfAllExpressions-1 {
			if ellipsis != nil {
				y.pushEllipsis(lenOfAllExpressions)
				y.writeString("...")
			}
		}
		// 如果不是最后一个参数或者每个参数一行就要加,
		if index != lenOfAllExpressions-1 || eachParamOneLine {
			y.writeString(", ")
			lineLength += 2
		}
		// 如果是最后一个参数且每个参数一行，就要换行
		if index == lenOfAllExpressions-1 && eachParamOneLine {
			y.writeNewLine()
			if hadIncIndent {
				y.decIndent()
			}
			y.writeIndent()
		}
	}

	return len(i.AllExpression()), ellipsis != nil
}
