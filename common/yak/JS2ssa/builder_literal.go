package js2ssa

import (
	"math"
	"strconv"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	// "github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *astbuilder) buildLiteralExpression(stmt *JS.LiteralExpressionContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.Literal().(*JS.LiteralContext); ok {
		return b.buildLiteral(s)
	}
	return nil
}

func (b *astbuilder) buildLiteral(stmt *JS.LiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.TemplateStringLiteral().(*JS.TemplateStringLiteralContext); ok {
		return b.buildTemplateStringLiteral(s)
	}

	if s, ok := stmt.NumericLiteral().(*JS.NumericLiteralContext); ok {
		return b.buildNumericLiteral(s)
	}

	if s, ok := stmt.BigintLiteral().(*JS.BigintLiteralContext); ok {
		return b.buildBigintLiteral(s)
	}

	if stmt.StringLiteral() != nil {
		s := stmt.StringLiteral()
		return b.buildStringLiteral(s)

	} else if stmt.BooleanLiteral() != nil {
		bo := stmt.GetText()
		return b.buildBooleanLiteral(bo)

	} else if stmt.NullLiteral() != nil {
		return b.buildNullLiteral()
	} else if stmt.RegularExpressionLiteral() != nil {
		// return ssa.emitconst(stmt.GetText())
		return b.EmitConstInst(stmt.GetText())
		// TODO
	}

	return nil
}

func (b *astbuilder) buildTemplateStringLiteral(stmt *JS.TemplateStringLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	type StringAtom interface {
		GetText() string
		SingleExpression() JS.ISingleExpressionContext
	}
	var tmpStr string
	var value ssa.Value
	value = b.EmitConstInst("")

	templateQuoteEscapeChar := func(quote byte, s string) string {
		s = strings.Replace(s, "\\$", "$", -1)
		if quote == '`' {
			s = strings.Replace(s, "\\n", "\\\\n", -1)
			s = strings.Replace(s, "\\r", "\\\\r", -1)
		}
		escapeString, err := yakunquote.UnquoteInner(s, quote)
		if err != nil {
			b.NewError(ssa.Error, TAG, "const parse %s as template string literal escape char: %v", s, err)
			return ""
		}
		return escapeString
	}

	parseStringAtom := func(prefix byte, atom StringAtom) {
		expr := atom.SingleExpression()
		if expr != nil {
			s := templateQuoteEscapeChar(prefix, tmpStr)
			tmpStr = ""
			value = b.EmitBinOp(ssa.OpAdd, value, b.EmitConstInst(s))

			v, _ := b.buildSingleExpression(expr, false)
			t := b.EmitTypeCast(v, ssa.CreateStringType())
			value = b.EmitBinOp(ssa.OpAdd, value, t)
		} else {
			tmpStr += atom.GetText()
		}
	}

	handlerTemplate := func(prefix byte, atoms []StringAtom) {
		for _, item := range atoms {
			parseStringAtom(prefix, item)
		}

		if tmpStr != "" {
			s := templateQuoteEscapeChar(prefix, tmpStr)
			value = b.EmitBinOp(ssa.OpAdd, value, b.EmitConstInst(s))
		}
	}

	handlerTemplate('`', lo.FilterMap(
		stmt.AllTemplateStringAtom(),
		func(atom JS.ITemplateStringAtomContext, _ int) (StringAtom, bool) {
			item, ok := atom.(*JS.TemplateStringAtomContext)
			return item, ok
		}))

	return value
}

func (b *astbuilder) buildNumericLiteral(stmt *JS.NumericLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	lit := stmt.GetText()
	// fmt.Println(lit)
	if find := strings.Contains(lit, "."); find {
		var f, _ = strconv.ParseFloat(lit, 64)
		return b.EmitConstInst(f)

	} else {

		var err error
		var originStr = stmt.GetText()
		var intStr = strings.ToLower(originStr)
		var resultInt64 int64

		// fmt.Println(originStr)

		if num := stmt.DecimalLiteral(); num != nil { // 十进制
			if strings.Contains(stmt.GetText(), "e") {
				var f, _ = strconv.ParseFloat(intStr, 64)
				return b.EmitConstInst(f)
			}
			resultInt64, err = strconv.ParseInt(intStr, 10, 64)
		} else if num := stmt.HexIntegerLiteral(); num != nil { // 十六进制
			resultInt64, err = strconv.ParseInt(intStr[2:], 16, 64)
		} else if num := stmt.BinaryIntegerLiteral(); num != nil { // 二进制
			resultInt64, err = strconv.ParseInt(intStr[2:], 2, 64)
		} else if num := stmt.OctalIntegerLiteral(); num != nil { // 八进制 017
			resultInt64, err = strconv.ParseInt(intStr[1:], 8, 64)
		} else if num := stmt.OctalIntegerLiteral2(); num != nil { // 八进制 0oxx
			resultInt64, err = strconv.ParseInt(intStr[2:], 8, 64)
		} else {
			b.NewError(ssa.Error, TAG, "cannot parse num for literal: %s", stmt.GetText())
			return nil
		}

		if err != nil {
			b.NewError(ssa.Error, TAG, "const parse %s as integer literal... is to large for int64: %v", originStr, err)
			return nil
		}

		if resultInt64 > math.MaxInt {
			return b.EmitConstInst(int64(resultInt64))
		} else {
			return b.EmitConstInst(int64(resultInt64))
		}
	}
}

func (b *astbuilder) buildBigintLiteral(stmt *JS.BigintLiteralContext) ssa.Value {
	// TODO:unfinished
	return nil
}

func (b *astbuilder) buildStringLiteral(stmt antlr.TerminalNode) ssa.Value {
	// TODO:unfinished

	var text = stmt.GetText()
	if text == "" {
		return b.EmitConstInst(text)
	}

	switch text[0] {
	case '"':
		val, err := strconv.Unquote(text)
		// fmt.Println(val)
		if err != nil {
			b.NewError(ssa.Error, TAG, "cannot parse string literal: %s failed: %s", stmt.GetText(), err.Error())
		}
		return b.EmitConstInstWithUnary(val, 0)
	case '\'':
		if lit := stmt.GetText(); len(lit) >= 2 {
			text = lit[1 : len(lit)-1]
		} else {
			text = lit
		}
		text = strings.Replace(text, "\\'", "'", -1)
		text = strings.Replace(text, `"`, `\"`, -1)
		val, err := strconv.Unquote(`"` + text + `"`)
		if err != nil {
			b.NewError(ssa.Error, TAG, "cannot parse string literal: %s failed: %s", stmt.GetText(), err.Error())
		}
		return b.EmitConstInstWithUnary(val, 0)
	}

	return nil
}

func (b *astbuilder) buildBooleanLiteral(bo string) ssa.Value {
	boolLit, err := strconv.ParseBool(bo)
	if err != nil {
		b.NewError(ssa.Error, TAG, "Unhandled bool literal")
	}
	return b.EmitConstInst(boolLit)
}

func (b *astbuilder) buildNullLiteral() ssa.Value {
	return b.EmitConstInst(nil)
}
