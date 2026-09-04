package csharp2ssa

import (
	"strconv"
	"strings"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) VisitLiteral(raw csharpparser.ILiteralContext) ssa.Value {
	if b == nil || raw == nil {
		return nil
	}
	i, ok := raw.(*csharpparser.LiteralContext)
	if !ok || i == nil {
		return b.EmitUndefined("literal")
	}
	switch {
	case i.Boolean_literal() != nil:
		bl, _ := i.Boolean_literal().(*csharpparser.Boolean_literalContext)
		return b.EmitConstInst(bl != nil && bl.TRUE() != nil)
	case i.Integer_Literal() != nil:
		return b.EmitConstInst(parseCSharpInteger(i.Integer_Literal().GetText()))
	case i.Real_Literal() != nil:
		return b.EmitConstInst(parseCSharpReal(i.Real_Literal().GetText()))
	case i.String_Literal() != nil:
		return b.EmitConstInst(unquoteCSharpString(i.String_Literal().GetText()))
	case i.Character_Literal() != nil:
		return b.EmitConstInst(unquoteCSharpChar(i.Character_Literal().GetText()))
	case i.Null_literal() != nil:
		return b.EmitConstInstNil()
	}
	return b.EmitUndefined(i.GetText())
}

// parseCSharpInteger handles decimal / 0x / 0b literals with `_` separators and u/l suffixes.
func parseCSharpInteger(text string) any {
	raw := text
	normalized := strings.ReplaceAll(text, "_", "")
	digits := strings.TrimRight(normalized, "uUlL")
	suffix := normalized[len(digits):]
	base := 10
	payload := digits
	if len(digits) >= 2 {
		switch strings.ToLower(digits[:2]) {
		case "0x":
			base, payload = 16, digits[2:]
		case "0b":
			base, payload = 2, digits[2:]
		}
	}
	if payload == "" {
		return raw
	}
	// A U suffix is an explicit unsigned type selection even when the value
	// also fits in int64. Unsuffixed/L-suffixed values use int64 when possible,
	// then retain uint64 for the legal upper half of C#'s ulong range.
	if strings.ContainsAny(suffix, "uU") {
		if v, err := strconv.ParseUint(payload, base, 64); err == nil {
			return v
		}
		return raw
	}
	if v, err := strconv.ParseInt(payload, base, 64); err == nil {
		return v
	}
	if v, err := strconv.ParseUint(payload, base, 64); err == nil {
		return v
	}
	return raw
}

func parseCSharpReal(text string) any {
	raw := text
	text = strings.ReplaceAll(text, "_", "")
	text = strings.TrimRight(text, "fFdDmM")
	if v, err := strconv.ParseFloat(text, 64); err == nil {
		return v
	}
	return raw
}

func unquoteCSharpString(s string) string {
	if strings.HasPrefix(s, "@\"") && strings.HasSuffix(s, "\"") && len(s) >= 3 {
		return strings.ReplaceAll(s[2:len(s)-1], `""`, `"`)
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		return unescapeCSharpLoose(s[1 : len(s)-1])
	}
	return s
}

func unquoteCSharpChar(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		inner := s[1 : len(s)-1]
		if u, err := strconv.Unquote(`"` + strings.ReplaceAll(inner, `"`, `\"`) + `"`); err == nil {
			return u
		}
		return unescapeCSharpLoose(inner)
	}
	return s
}

// unescapeCSharpLoose handles escapes strconv.Unquote rejects (\0, \x with 1-4 digits, lone quotes).
func unescapeCSharpLoose(s string) string {
	var sb strings.Builder
	for idx := 0; idx < len(s); idx++ {
		c := s[idx]
		if c != '\\' || idx+1 >= len(s) {
			sb.WriteByte(c)
			continue
		}
		idx++
		switch s[idx] {
		case 'n':
			sb.WriteByte('\n')
		case 'r':
			sb.WriteByte('\r')
		case 't':
			sb.WriteByte('\t')
		case '0':
			sb.WriteByte(0)
		case 'a':
			sb.WriteByte('\a')
		case 'b':
			sb.WriteByte('\b')
		case 'f':
			sb.WriteByte('\f')
		case 'v':
			sb.WriteByte('\v')
		case '\\', '"', '\'':
			sb.WriteByte(s[idx])
		case 'x', 'u', 'U':
			end := idx + 1
			for end < len(s) && end-idx-1 < 8 && isHexDigit(s[end]) {
				end++
			}
			if v, err := strconv.ParseUint(s[idx+1:end], 16, 32); err == nil {
				sb.WriteRune(rune(v))
			}
			idx = end - 1
		default:
			sb.WriteByte('\\')
			sb.WriteByte(s[idx])
		}
	}
	return sb.String()
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// ---------------------------------------------------------------- interpolated strings

// VisitInterpolatedString compiles $"a{x}b" into `"a" + x + "b"` so data flow through
// string building is preserved.
func (b *singleFileBuilder) VisitInterpolatedString(raw csharpparser.IInterpolated_string_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Interpolated_string_expressionContext)
	if !ok || i == nil {
		return nil
	}
	var node antlr.ParserRuleContext
	verbatim := false
	if r, _ := i.Interpolated_regular_string_expression().(*csharpparser.Interpolated_regular_string_expressionContext); r != nil {
		node = r
	} else if v, _ := i.Interpolated_verbatim_string_expression().(*csharpparser.Interpolated_verbatim_string_expressionContext); v != nil {
		node = v
		verbatim = true
	}
	if node == nil {
		return b.EmitConstInst(i.GetText())
	}

	var result ssa.Value
	appendPart := func(v ssa.Value) {
		if utils.IsNil(v) {
			return
		}
		if result == nil {
			result = v
			return
		}
		result = b.EmitBinOp(ssa.OpAdd, result, v)
	}
	for _, child := range node.GetChildren() {
		switch c := child.(type) {
		case *csharpparser.Regular_interpolationContext:
			appendPart(b.VisitExpression(c.Expression()))
		case *csharpparser.Verbatim_interpolationContext:
			appendPart(b.VisitExpression(c.Expression()))
		case antlr.TerminalNode:
			tt := c.GetSymbol().GetTokenType()
			if tt == csharpparser.CSharpLexerInterpolated_Regular_String_Mid || tt == csharpparser.CSharpLexerInterpolated_Verbatim_String_Mid {
				appendPart(b.EmitConstInst(unescapeInterpolatedMid(c.GetText(), verbatim)))
			}
		}
	}
	if result == nil {
		return b.EmitConstInst("")
	}
	if result.GetType() == nil || result.GetType().GetTypeKind() != ssa.StringTypeKind {
		result.SetType(ssa.CreateStringType())
	}
	return result
}

func unescapeInterpolatedMid(text string, verbatim bool) string {
	// CSharpLexerBase.WrapToken protects interpolation tokens from ANTLR's
	// mode handling by surrounding their text with U+3014/U+3015 and doubling
	// literal closing markers. Restore the source text before C# unescaping.
	if strings.HasPrefix(text, "〔") && strings.HasSuffix(text, "〕") {
		text = strings.TrimSuffix(strings.TrimPrefix(text, "〔"), "〕")
		text = strings.ReplaceAll(text, "〕〕", "〕")
	}
	text = strings.ReplaceAll(text, "{{", "{")
	text = strings.ReplaceAll(text, "}}", "}")
	if verbatim {
		return strings.ReplaceAll(text, `""`, `"`)
	}
	return unescapeCSharpLoose(text)
}
