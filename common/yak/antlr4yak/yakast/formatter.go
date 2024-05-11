package yakast

import (
	"bytes"
	"strings"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

const (
	FORMATTER_MAXWIDTH               = 121
	FORMATTER_RECOMMEND_PARAM_LENGTH = 40
	FORMATTER_RECOMMEND_LINE_LENGTH  = 75
)

func clearWsComment(text string) string {
	if strings.Contains(text, "//") || strings.Contains(text, "/*") {
		text = text[2:]
	}

	if strings.Contains(text, "*/") {
		text = text[:len(text)-2]
	}

	text = strings.TrimSpace(text)
	return text
}

func getIdentifersSurroundComments(tokenStream antlr.TokenStream, startToken, endToken antlr.Token, lenOfIds int) []string {
	comments := make([]string, lenOfIds)
	start, stop := startToken.GetTokenIndex(), endToken.GetTokenIndex()
	for index, idIndex := start, 0; index <= stop && idIndex < lenOfIds; index++ {
		token := tokenStream.Get(index)
		tokenType := token.GetTokenType()
		if tokenType == parser.YaklangLexerIdentifier {
		} else if tokenType == parser.YaklangLexerComma {
			idIndex++
		} else if tokenType == parser.YaklangParserCOMMENT || tokenType == parser.YaklangParserLINE_COMMENT {
			text := clearWsComment(token.GetText())
			if text == "" {
				continue
			}
			comments[idIndex] += text + "; "
		}
	}

	for index := range comments {
		comments[index] = strings.TrimRight(comments[index], "; ")
	}

	return comments
}

func (y *YakCompiler) switchIsOMap(isOmap bool) func() {
	origin := y.isOMap
	y.isOMap = isOmap
	return func() {
		y.isOMap = origin
	}
}

func (y *YakCompiler) switchFormatBuffer() func() string {
	origin := y.formatted
	y.formatted = &bytes.Buffer{}
	return func() string {
		buf := y.formatted.String()
		y.formatted = origin
		return buf
	}
}

func (y *YakCompiler) TrimEos(s string) int {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, " ", "")
	if strings.Contains(s, "\n\n") {
		return 2
	} else if strings.Contains(s, "\n") || s == "" || s == ";" {
		return 1
	}
	return 0
}

func (y *YakCompiler) writeStringWithWhitespace(i string) {
	y.formatted.WriteByte(' ')
	y.formatted.WriteString(i)
	y.formatted.WriteByte(' ')
}

func (y *YakCompiler) writeWhiteSpace(i int) {
	y.formatted.WriteString(strings.Repeat(" ", i))
}

func (y *YakCompiler) writeString(i string) {
	y.formatted.WriteString(i)
}

func (y *YakCompiler) writeNewLine() {
	y.formatted.WriteString("\n")
}

func (y *YakCompiler) writeAllWS(raw []yak.IWsContext) {
	if y == nil || raw == nil {
		return
	}
	for _, i := range raw {
		if i == nil {
			continue
		}
		y.writeEosWithText(i.GetText())
	}
}

func (y *YakCompiler) writeEOS(raw yak.IEosContext) {
	if y == nil || raw == nil {
		return
	}
	i, _ := raw.(*yak.EosContext)
	if i == nil {
		return
	}

	y.writeEosWithText(i.GetText())
}

func (y *YakCompiler) writeEosWithText(s string) {
	trimedLeftStr := strings.TrimLeft(s, "\r\n")
	if strings.HasPrefix(trimedLeftStr, "//") || strings.HasPrefix(trimedLeftStr, "/*") {
		y.writeString(s)
		return
	}
	for j := 0; j < y.TrimEos(s); j++ {
		y.writeNewLine()
	}
}

func (y *YakCompiler) writeIndent() {
	y.writeWhiteSpace(4 * y.indent)
}

func (y *YakCompiler) incIndent() {
	y.indent++
}

func (y *YakCompiler) decIndent() {
	y.indent--
}

func (y *YakCompiler) GetFormattedCode() string {
	return strings.TrimSpace(y.formatted.String())
}

type parserGetter interface {
	GetParser() antlr.Parser
	GetStop() antlr.Token
	GetStart() antlr.Token
}

func (y *YakCompiler) keepCommentLine(stmts []parser.IStatementContext, index int) {
	// ts := nowToken.GetParser().GetTokenStream()
	// commentRaw := ts.GetTextFromInterval(&antlr.Interval{
	// 	Start: startColumn,
	// 	Stop:  nowToken.GetStart().GetTokenIndex() - 1,
	// })
	// commentLine := strings.TrimSpace(commentRaw)

	// if len(lines) <= 1 || index <= 0 {
	// 	return
	// }
	// last := stmts[index-1]
	// now := stmts[index]
	// last.GetStart().GetLine()

	// if strings.HasPrefix(commentLine, "//") ||
	// 	strings.HasPrefix(commentLine, "#") {
	// 	y.writeString(commentLine)
	// } else if strings.HasPrefix(commentLine, "/*") && strings.HasSuffix(commentLine, "*/") {
	// 	y.writeString(commentLine)
	// }
}
