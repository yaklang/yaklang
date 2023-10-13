package js2ssa

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	// "github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *astbuilder) buildLiteralExpression(stmt *JS.LiteralExpressionContext) ssa.Value {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	// s := stmt.Literal()
	// fmt.Println(s)

	if s, ok := stmt.Literal().(*JS.LiteralContext); ok {
		return b.buildLiteral(s)
	}
	return nil
}

func (b *astbuilder) buildLiteral(stmt *JS.LiteralContext) ssa.Value {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
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
		boolLit, err := strconv.ParseBool(bo)
		if err != nil {
			b.NewError(ssa.Error, TAG, "Unhandled bool literal")
		}
		return ssa.NewConst(boolLit)
	
	} else if stmt.NullLiteral() != nil {
		return ssa.NewConst(nil)
	
	} else if stmt.RegularExpressionLiteral() != nil {
		// TODO
	}

	return nil
}

func (b *astbuilder) buildTemplateStringLiteral(stmt *JS.TemplateStringLiteralContext) ssa.Value {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	//TODO:unfinshed

	value := ssa.NewConst(1)
	return value
}

func (b *astbuilder) buildNumericLiteral(stmt *JS.NumericLiteralContext) ssa.Value {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	lit := stmt.GetText()
	// fmt.Println(lit)
	if find := strings.Contains(lit, "."); find {
		var f, _ = strconv.ParseFloat(lit, 64)
		return ssa.NewConst(f)

	} else {

		var err error
		var originStr = stmt.GetText()
		var intStr = strings.ToLower(originStr)
		var resultInt64 int64

		fmt.Println(originStr)

		if num := stmt.DecimalLiteral(); num != nil {	// 十进制
			resultInt64, err = strconv.ParseInt(intStr, 10, 64)
		} else if num := stmt.HexIntegerLiteral(); num != nil {	// 十六进制 
			resultInt64, err = strconv.ParseInt(intStr[2:], 16, 64)
		} else if num := stmt.BinaryIntegerLiteral(); num != nil { // 二进制
			resultInt64, err = strconv.ParseInt(intStr[2:], 2, 64)
		} else if num := stmt.OctalIntegerLiteral(); num != nil {	// 八进制 017
			resultInt64, err = strconv.ParseInt(intStr[1:], 8, 64)
		} else if num := stmt.OctalIntegerLiteral2(); num != nil {	// 八进制 0oxx
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
			return ssa.NewConst(int64(resultInt64))
		} else {
			return ssa.NewConst(int(resultInt64))
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
		return ssa.NewConst(text)
	}


	switch text[0] {
	case '"':
		val, err := strconv.Unquote(text)
		fmt.Println(val)
		if err != nil {
			fmt.Printf("parse %v to string literal failed: %s", stmt.GetText(), err.Error())
		}
		return ssa.NewConstWithUnary(val, 0)
	case '\'':
		if lit := stmt.GetText(); len(lit) >= 2{
			text = lit[1: len(lit)-1]
		} else {
			text = lit
		}
		text = strings.Replace(text, "\\'", "'", -1)
		text = strings.Replace(text, `"`, `\"`, -1)
		val, err := strconv.Unquote(`"` + text + `"`)
		if err != nil {
			fmt.Printf("pars %v to string literal field: %s", stmt.GetText(), err.Error())
		}
		return ssa.NewConstWithUnary(val, 0)
	}
	

	return nil
}
