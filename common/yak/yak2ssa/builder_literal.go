package yak2ssa

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func (b *astbuilder) buildLiteral(stmt *yak.LiteralContext) (v ssa.Value) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	// template string literal
	if s, ok := stmt.TemplateStringLiteral().(*yak.TemplateStringLiteralContext); ok {
		return b.buildTemplateStringLiteral(s)
	}

	// string literal
	if s, ok := stmt.StringLiteral().(*yak.StringLiteralContext); ok {
		return b.buildStringLiteral(s)
	} else if s, ok := stmt.NumericLiteral().(*yak.NumericLiteralContext); ok {
		return b.buildNumericLiteral(s)
	} else if s, ok := stmt.BoolLiteral().(*yak.BoolLiteralContext); ok {
		boolLit, err := strconv.ParseBool(s.GetText())
		if err != nil {
			b.NewError(ssa.Error, TAG, "Unhandled bool literal")
		}
		return b.EmitConstInst(boolLit)
	} else if stmt.UndefinedLiteral() != nil {
		// TODO: this ok??
		return ssa.NewParam("undefined", false, b.FunctionBuilder)
	} else if stmt.NilLiteral() != nil {
		return b.EmitConstInst(nil)
	} else if stmt.CharacterLiteral() != nil {
		lit := stmt.CharacterLiteral().GetText()
		var s string
		var err error
		if lit == "'\\''" {
			s = "'"
		} else {
			lit = strings.ReplaceAll(lit, `"`, `\"`)
			s, err = strconv.Unquote(fmt.Sprintf("\"%s\"", lit[1:len(lit)-1]))
			if err != nil {
				b.NewError(ssa.Error, TAG, "unquote error %s", err)
				return nil
			}
		}
		runeChar := []rune(s)[0]
		if runeChar < 256 {
			return b.EmitConstInst(byte(runeChar))
		} else {
			// unbelievable
			// log.Warnf("Character literal is rune: %s", stmt.CharacterLiteral().GetText())
			return b.EmitConstInst(runeChar)
		}
	} else if s := stmt.MapLiteral(); s != nil {
		if s, ok := s.(*yak.MapLiteralContext); ok {
			return b.buildMapLiteral(s)
		} else {
			b.NewError(ssa.Error, TAG, "Unhandled Map(Object) Literal: "+stmt.MapLiteral().GetText())
		}
	} else if s := stmt.SliceLiteral(); s != nil {
		if s, ok := s.(*yak.SliceLiteralContext); ok {
			return b.buildSliceLiteral(s)
		} else {
			b.NewError(ssa.Error, TAG, "Unhandled Slice Literal: "+stmt.SliceLiteral().GetText())
		}
	} else if s := stmt.SliceTypedLiteral(); s != nil {
		if s, ok := s.(*yak.SliceTypedLiteralContext); ok {
			return b.buildSliceTypedLiteral(s)
		} else {
			b.NewError(ssa.Error, TAG, "unhandled Slice Typed Literal: "+stmt.SliceTypedLiteral().GetText())
		}
	}

	// slice typed literal
	if s, ok := stmt.SliceTypedLiteral().(*yak.SliceTypedLiteralContext); ok {
		return b.buildSliceTypedLiteral(s)
	}

	// TODO: type literal
	if s, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
		return b.EmitTypeValue(b.buildTypeLiteral(s))
	}

	// mixed

	return nil
}

// type literal
func (b *astbuilder) buildTypeLiteral(stmt *yak.TypeLiteralContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	text := stmt.GetText()
	// var type name
	if b := ssa.GetTypeByStr(text); b != nil {
		return b
	}

	// slice type literal
	if s, ok := stmt.SliceTypeLiteral().(*yak.SliceTypeLiteralContext); ok {
		return b.buildSliceTypeLiteral(s)
	}

	// map type literal
	if strings.HasPrefix(text, "map") {
		if s, ok := stmt.MapTypeLiteral().(*yak.MapTypeLiteralContext); ok {
			return b.buildMapTypeLiteral(s)
		}
	}

	// chan type literal
	if strings.HasPrefix(text, "chan") {
		if s, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
			if typ := b.buildTypeLiteral(s); typ != nil {
				return ssa.NewChanType(typ)
			}
		}
	}

	return nil
}

// slice type literal
func (b *astbuilder) buildSliceTypeLiteral(stmt *yak.SliceTypeLiteralContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	// fmt.Println(stmt.GetText())
	if stmt.GetText() == "[]byte" || stmt.GetText() == "[]uint8" {
		return ssa.CreateBytesType()
	}
	if s, ok := stmt.TypeLiteral().(*yak.TypeLiteralContext); ok {
		if eleTyp := b.buildTypeLiteral(s); eleTyp != nil {
			return ssa.NewSliceType(eleTyp)
		}
	}
	return nil
}

// map type literal
func (b *astbuilder) buildMapTypeLiteral(stmt *yak.MapTypeLiteralContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	// key
	var keyTyp ssa.Type
	var valueTyp ssa.Type
	if s, ok := stmt.TypeLiteral(0).(*yak.TypeLiteralContext); ok {
		keyTyp = b.buildTypeLiteral(s)
	}

	// value
	if s, ok := stmt.TypeLiteral(1).(*yak.TypeLiteralContext); ok {
		valueTyp = b.buildTypeLiteral(s)
	}
	if keyTyp != nil && valueTyp != nil {
		return ssa.NewMapType(keyTyp, valueTyp)
	}

	return nil
}

// numeric literal
func (b *astbuilder) buildNumericLiteral(stmt *yak.NumericLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	// integer literal
	if ilit := stmt.IntegerLiteral(); ilit != nil {
		var err error
		originIntStr := ilit.GetText()
		intStr := strings.ToLower(originIntStr)
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
			b.NewError(ssa.Error, TAG, "const parse %s as integer literal... is to large for int64: %v", originIntStr, err)
			return nil
		}
		if resultInt64 > math.MaxInt {
			return b.EmitConstInst(int64(resultInt64))
		} else {
			return b.EmitConstInst(int(resultInt64))
		}
	}

	// float literal
	if iFloat := stmt.FloatLiteral(); iFloat != nil {
		lit := iFloat.GetText()
		if strings.HasPrefix(lit, ".") {
			lit = "0" + lit
		}
		f, _ := strconv.ParseFloat(lit, 64)
		return b.EmitConstInst(f)
	}
	b.NewError(ssa.Error, TAG, "cannot parse num for literal: %s", stmt.GetText())
	return nil
}

func (b *astbuilder) buildTemplateStringLiteral(stmt *yak.TemplateStringLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	type StringAtom interface {
		GetText() string
		Expression() yak.IExpressionContext
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
		expr := atom.Expression()
		if expr != nil {
			s := templateQuoteEscapeChar(prefix, tmpStr)
			tmpStr = ""
			value = b.EmitBinOp(ssa.OpAdd, value, b.EmitConstInst(s))

			v := b.buildExpression(expr.(*yak.ExpressionContext))
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

	if s, ok := stmt.TemplateDoubleQuoteStringLiteral().(*yak.TemplateDoubleQuoteStringLiteralContext); ok {
		handlerTemplate('"', lo.FilterMap(
			s.AllTemplateDoubleQuoteStringAtom(),
			func(atom yak.ITemplateDoubleQuoteStringAtomContext, _ int) (StringAtom, bool) {
				item, ok := atom.(*yak.TemplateDoubleQuoteStringAtomContext)
				return item, ok
			}))
	} else if s, ok := stmt.TemplateBackTickStringLiteral().(*yak.TemplateBackTickStringLiteralContext); ok {
		handlerTemplate('`', lo.FilterMap(
			s.AllTemplateBackTickStringAtom(),
			func(atom yak.ITemplateBackTickStringAtomContext, _ int) (StringAtom, bool) {
				item, ok := atom.(*yak.TemplateBackTickStringAtomContext)
				return item, ok
			}))
	} else if s, ok := stmt.TemplateSingleQuoteStringLiteral().(*yak.TemplateSingleQuoteStringLiteralContext); ok {
		handlerTemplate('\'', lo.FilterMap(
			s.AllTemplateSingleQuoteStringAtom(),
			func(atom yak.ITemplateSingleQuoteStringAtomContext, _ int) (StringAtom, bool) {
				item, ok := atom.(*yak.TemplateSingleQuoteStringAtomContext)
				return item, ok
			}))
	} else {
		b.NewError(ssa.Error, TAG, "parse template string literal error")
	}

	return value
}

// string literal
func (b *astbuilder) buildStringLiteral(stmt *yak.StringLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if stmt.StartNowDoc() != nil {
		var text string
		if node := stmt.CrlfHereDoc(); node != nil {
			text = node.GetText()
		} else if node := stmt.LfHereDoc(); node != nil {
			text = node.GetText()
		}
		return b.EmitConstInst(text)
	}
	text := stmt.GetText()
	if text == "" {
		return b.EmitConstInst(text)
	}

	var prefix byte
	hasPrefix := false
	supportPrefix := []byte{'x', 'b', 'r'}
	var ret *ssa.ConstInst
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
			ret = b.EmitConstInstWithUnary(val, int(prefix))
		} else {
			val, err := strconv.Unquote(text)
			if err != nil {
				fmt.Printf("parse %v to string literal failed: %s", stmt.GetText(), err.Error())
			}
			ret = b.EmitConstInstWithUnary(val, int(prefix))
		}
	case '\'':
		if prefix == 'r' {
			var val string
			if lit := stmt.GetText(); len(lit) >= 2 {
				val = lit[1 : len(lit)-1]
			} else {
				val = lit
			}
			prefix = 0
			ret = b.EmitConstInstWithUnary(val, int(prefix))
		} else {
			if lit := stmt.GetText(); len(lit) >= 2 {
				text = lit[1 : len(lit)-1]
			} else {
				text = lit
			}
			text = strings.Replace(text, "\\'", "'", -1)
			text = strings.Replace(text, `"`, `\"`, -1)
			val, err := strconv.Unquote(`"` + text + `"`)
			if err != nil {
				fmt.Printf("pars %v to string literal field: %s", stmt.GetText(), err.Error())
			}
			ret = b.EmitConstInstWithUnary(val, int(prefix))
		}
	case '`':
		val := text[1 : len(text)-1]
		ret = b.EmitConstInstWithUnary(val, int(prefix))
	case '0':
		switch text[1] {
		case 'h':
			text = text[2:]
			hex, err := codec.DecodeHex(text)
			if err != nil {
				fmt.Printf("parse hex string error: %v", err)
			}
			ret = b.EmitConstInst(hex)
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
			fmt.Printf("invalid string literal: %s", stmt.GetText())
		}
	}

	if prefix == 'b' {
		ret.SetType(ssa.CreateBytesType())
	}

	return ret
}
