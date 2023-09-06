package yakast

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func (y *YakCompiler) VisitNumericLiteral(raw yak.INumericLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.NumericLiteralContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString(raw.GetText())

	var err error

	if iLit := i.IntegerLiteral(); iLit != nil {
		// 0x7fffffffffffffff
		var originIntStr = iLit.GetText()
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
			y.panicCompilerError(integerIsTooLarge, originIntStr)
		}
		if resultInt64 > math.MaxInt {
			y.pushInt64(resultInt64, originIntStr)
		} else {
			y.pushInteger(int(resultInt64), originIntStr)
		}
		return nil
	}

	if iFloat := i.FloatLiteral(); iFloat != nil {
		lit := iFloat.GetText()
		if strings.HasPrefix(lit, ".") {
			lit = "0" + lit
		}
		var f, _ = strconv.ParseFloat(lit, 64)
		y.pushFloat(f, iFloat.GetText())
		return nil
	}
	y.panicCompilerError(contParseNumber, i.GetText())

	return nil
}

func (y *YakCompiler) VisitBoolLiteral(raw yak.IBoolLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.BoolLiteralContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString(raw.GetText())

	b, _ := strconv.ParseBool(i.GetText())
	y.pushBool(b)
	return nil
}

func (y *YakCompiler) VisitLiteral(raw yak.ILiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.LiteralContext)
	if i == nil {
		return nil
	}

	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	if n := i.StringLiteral(); n != nil {
		y.VisitStringLiteral(n)
		return nil
	}
	if n := i.TemplateStringLiteral(); n != nil {
		y.VisitTemplateStringLiteral(n)
		return nil
	}
	if n := i.NumericLiteral(); n != nil {
		y.VisitNumericLiteral(n)
		return nil
	}

	if b := i.BoolLiteral(); b != nil {
		y.VisitBoolLiteral(b)
		return nil
	}

	if i.UndefinedLiteral() != nil || i.NilLiteral() != nil {
		y.writeString(i.GetText())
		y.pushUndefined()
		return nil
	}

	if b := i.CharaterLiteral(); b != nil {
		y.VisitCharaterLiteral(b)
		return nil
	}

	if m := i.MapLiteral(); m != nil {
		y.VisitMapLiteral(m)
		return nil
	}

	if l := i.SliceTypedLiteral(); l != nil {
		y.VisitSliceTypedLiteral(l)
		return nil
	}

	if m := i.TypeLiteral(); m != nil {
		y.VisitTypeLiteral(m)
	}

	if m := i.SliceLiteral(); m != nil {
		y.VisitSliceLiteral(m)
		return nil
	}

	return nil
}

func (y *YakCompiler) VisitSliceLiteral(raw yak.ISliceLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.SliceLiteralContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	// [ ... ] 语法
	if i.LBracket() != nil && i.RBracket() != nil {
		y.writeString("[")
		unary := y.VisitExpressionListMultiline(i.ExpressionListMultiline())
		y.writeString("]")
		y.pushNewSlice(unary)
		return nil
	}

	y.panicCompilerError(notImplemented, i.GetText())
	return nil
}

func (y *YakCompiler) VisitSliceTypedLiteral(raw yak.ISliceTypedLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.SliceTypedLiteralContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	// 先创建一个类型
	y.VisitSliceTypeLiteral(i.SliceTypeLiteral())
	y.writeString("{")
	y.pushTypedSlice(y.VisitExpressionListMultiline(i.ExpressionListMultiline()))
	y.writeString("}")
	return nil
}

func (y *YakCompiler) VisitSliceTypeLiteral(raw yak.ISliceTypeLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.SliceTypeLiteralContext)
	if i == nil {
		return nil
	}
	y.writeString("[]")
	y.VisitTypeLiteral(i.TypeLiteral())
	y.pushType("slice")
	return nil
}

func (y *YakCompiler) VisitExpressionListMultiline(raw yak.IExpressionListMultilineContext) int {
	if y == nil || raw == nil {
		return 0
	}

	i, _ := raw.(*yak.ExpressionListMultilineContext)
	if i == nil {
		return 0
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	allExpression := i.AllExpression()
	lenOfAllExpression := len(allExpression)
	for index, e := range allExpression {
		y.VisitExpression(e)
		if index != lenOfAllExpression-1 {
			y.writeString(", ")
		}
	}
	return lenOfAllExpression
}

func (y *YakCompiler) VisitMapLiteral(raw yak.IMapLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.MapLiteralContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	if l := i.MapTypedLiteral(); l != nil {
		y.VisitMapTypedLiteral(l)
		return nil
	}

	y.writeString("{")
	defer y.writeString("}")

	pairs := i.MapPairs()
	if pairs == nil {
		y.pushNewMap(0)
		return nil
	}

	allPair := pairs.(*yak.MapPairsContext).AllMapPair()
	lenOfAllPair := len(allPair)
	for index, p := range allPair {
		y.VisitExpression(p.(*yak.MapPairContext).Expression(0))
		y.writeString(": ")
		y.VisitExpression(p.(*yak.MapPairContext).Expression(1))
		if index < lenOfAllPair-1 {
			y.writeString(", ")
		}
	}

	y.pushNewMap(lenOfAllPair)

	return nil
}

func (y *YakCompiler) VisitMapTypedLiteral(raw yak.IMapTypedLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.MapTypedLiteralContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	// 先创建一个类型
	y.VisitMapTypeLiteral(i.MapTypeLiteral())
	y.writeString("{")
	defer y.writeString("}")

	pairs := i.MapPairs()
	if pairs == nil {
		// 转成opmake
		y.pushMake(0)
		return nil
	}

	allPair := pairs.(*yak.MapPairsContext).AllMapPair()
	lenOfAllPair := len(allPair)
	for index, p := range allPair {
		y.VisitExpression(p.(*yak.MapPairContext).Expression(0))
		y.writeString(": ")
		y.VisitExpression(p.(*yak.MapPairContext).Expression(1))
		if index < lenOfAllPair-1 {
			y.writeString(", ")
		}
	}

	y.pushTypedMap(lenOfAllPair)
	return nil
}

func (y *YakCompiler) VisitMapTypeLiteral(raw yak.IMapTypeLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.MapTypeLiteralContext)
	if i == nil {
		return nil
	}

	y.writeString("map[")
	y.VisitTypeLiteral(i.TypeLiteral(0))
	y.writeString("]")
	y.VisitTypeLiteral(i.TypeLiteral(1))
	y.pushType("map")
	return nil
}

func (y *YakCompiler) VisitTemplateStringLiteral(raw yak.ITemplateStringLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	y.writeString(raw.GetText())

	// 在当前函数中禁用！
	recoverFormatter := y.switchFormatBuffer()
	defer recoverFormatter()

	type StringAtom interface {
		GetText() string
		Expression() yak.IExpressionContext
	}
	var tempString string
	var quote byte
	yakTemplateQuoteEscapeChar := func(s string) string {
		s = strings.Replace(s, "\\$", "$", -1)
		if quote == '`' {
			s = strings.Replace(s, "\\n", "\\\\n", -1)
			s = strings.Replace(s, "\\r", "\\\\r", -1)
		}
		escapeString, err := yakunquote.UnquoteInner(s, quote)
		if err != nil {
			y.panicCompilerError(compileError, err)
		}
		return escapeString
	}

	pushStringAtom := func(atom StringAtom) {
		exp := atom.Expression()
		if exp != nil {
			tempString = yakTemplateQuoteEscapeChar(tempString)
			y.pushString(tempString, tempString)

			tempString = ""
			y.pushOperator(yakvm.OpAdd)
			y.VisitExpression(exp)
			y.pushType("string")
			y.pushOperator(yakvm.OpTypeCast)
			y.pushOperator(yakvm.OpAdd)
		} else {
			tempString += atom.GetText()
		}
	}

	handleTemplate := func(prefix byte, getAtoms func() []StringAtom) {
		quote = prefix
		y.pushString("", "")
		atoms := getAtoms()
		for _, item := range atoms {
			pushStringAtom(item)
		}
		if tempString != "" {
			tempString = yakTemplateQuoteEscapeChar(tempString)
			y.pushString(tempString, tempString)
			tempString = ""
			y.pushOperator(yakvm.OpAdd)
		}
	}

	i, _ := raw.(*yak.TemplateStringLiteralContext)
	if i == nil {
		return nil
	}
	if ilit := i.TemplateDoubleQuoteStringLiteral(); ilit != nil {
		if lit, _ := ilit.(*yak.TemplateDoubleQuoteStringLiteralContext); lit != nil {
			handleTemplate('"', func() []StringAtom {
				return lo.FilterMap(lit.AllTemplateDoubleQupteStringAtom(), func(atom yak.ITemplateDoubleQupteStringAtomContext, _ int) (StringAtom, bool) {
					item, ok := atom.(*yak.TemplateDoubleQupteStringAtomContext)
					return item, ok
				})
			})
		}
	} else if ilit := i.TemplateBackTickStringLiteral(); ilit != nil {
		if lit, _ := ilit.(*yak.TemplateBackTickStringLiteralContext); lit != nil {
			handleTemplate('`', func() []StringAtom {
				return lo.FilterMap(lit.AllTemplateBackTickStringAtom(), func(atom yak.ITemplateBackTickStringAtomContext, _ int) (StringAtom, bool) {
					item, ok := atom.(*yak.TemplateBackTickStringAtomContext)
					return item, ok
				})
			})
		}
	} else if ilit := i.TemplateSingleQuoteStringLiteral(); ilit != nil {
		if lit, _ := ilit.(*yak.TemplateSingleQuoteStringLiteralContext); lit != nil {
			handleTemplate('\'', func() []StringAtom {
				return lo.FilterMap(lit.AllTemplateSingleQupteStringAtom(), func(atom yak.ITemplateSingleQupteStringAtomContext, _ int) (StringAtom, bool) {
					item, ok := atom.(*yak.TemplateSingleQupteStringAtomContext)
					return item, ok
				})
			})
		}
	} else {
		y.panicCompilerError(compileError, "parse template string literal error")
	}

	//y.pushString(i.GetText())
	return nil
}

func (y *YakCompiler) VisitStringLiteral(raw yak.IStringLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.StringLiteralContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString(raw.GetText())

	var text = i.GetText()
	if text == "" {
		y.pushString(text, text)
		return nil
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
			y.pushPrefixString(prefix, val, i.GetText())
		} else {
			val, err := yakunquote.Unquote(text)
			if err != nil {
				y.panicCompilerError(compileError, utils.Errorf("parse %v to string literal failed: %s", i.GetText(), err.Error()))
			}
			y.pushPrefixString(prefix, val, i.GetText())
		}
	case '\'':
		if prefix == 'r' {
			var val string
			if lit := i.GetText(); len(lit) >= 2 {
				val = lit[1 : len(lit)-1]
			} else {
				val = lit
			}
			prefix = 0
			y.pushPrefixString(prefix, val, i.GetText())
		} else {
			val, err := yakunquote.Unquote(text)
			if err != nil {
				y.panicCompilerError(compileError, utils.Errorf("parse %v to string literal failed: %s", i.GetText(), err.Error()))
			}
			y.pushPrefixString(prefix, val, i.GetText())
		}
	case '`':
		val := text[1 : len(text)-1]
		y.pushPrefixString(prefix, val, i.GetText())
	case '0':
		switch text[1] {
		case 'h':
			text = text[2:]
			hex, err := codec.DecodeHex(text)
			if err != nil {
				y.panicCompilerError(compileError, fmt.Sprintf("parse hex string error: %v", err))
			}
			y.pushBytes(hex, text)
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
			y.panicCompilerError(stringLiteralError, i.GetText())
		}
	}
	return nil
}

func (y *YakCompiler) VisitCharaterLiteral(raw yak.ICharaterLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.CharaterLiteralContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString(raw.GetText())

	lit := i.GetText()
	var s string
	var err error
	if lit == "'\\''" {
		s = "'"
	} else {
		lit = strings.ReplaceAll(lit, `"`, `\"`)
		s, err = strconv.Unquote(fmt.Sprintf("\"%s\"", lit[1:len(lit)-1]))
		if err != nil {
			y.panicCompilerError(compileError, fmt.Sprintf("unquote error: %s", err))
		}
	}

	runeLit := []rune(s)
	v := runeLit[0]
	y.pushChar(v, lit)
	return nil
}
