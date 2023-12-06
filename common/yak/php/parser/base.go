package phpparser

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"reflect"
	"strings"
)

type PHPLexerBase struct {
	*antlr.BaseLexer

	Interpreter     *antlr.LexerATNSimulator
	RuleNames       []string
	LiteralNames    []string
	SymbolicNames   []string
	GrammarFileName string

	// inline
	_scriptTag         bool
	_styleTag          bool
	_phpScript         bool
	_insideString      bool
	_htmlNameText      string
	_prevTokenType     int
	_heredocIdentifier string
	_astTags           bool
}

func reflectGetInt(i any, field string) (finalRet int) {
	defer func() {
		if err := recover(); err != nil {
			finalRet = -1
		}
	}()
	v := reflect.ValueOf(i)
	if ret := v.Type().Kind(); ret != reflect.Ptr && ret != reflect.Struct {
		return
	} else {
		if ret == reflect.Ptr {
			v = v.Elem()
		}
	}

	fieldR := v.FieldByName(field)
	if fieldR.IsValid() {
		finalRet = int(fieldR.Int())
	}
	return finalRet
}

func reflectSetInt(i any, field string) (finalRet int) {
	defer func() {
		if err := recover(); err != nil {
			finalRet = -1
		}
	}()
	v := reflect.ValueOf(i)
	if ret := v.Type().Kind(); ret != reflect.Ptr && ret != reflect.Struct {
		return
	} else {
		if ret == reflect.Ptr {
			v = v.Elem()
		}
	}

	fieldR := v.FieldByName(field)
	if fieldR.IsValid() {
		finalRet = int(fieldR.Int())
	}
	return finalRet
}

func (p *PHPLexerBase) NextToken() antlr.Token {
	if p.BaseLexer.Interpreter == nil {
		p.BaseLexer.Interpreter = p.Interpreter
	}
	token := p.BaseLexer.NextToken()

	switch token.GetTokenType() {
	case PHPLexerPHPEnd, PHPLexerPHPEndSingleLineComment:
		if reflectGetInt(p.BaseLexer, "mode") == PHPLexerSingleLineCommentMode {
			// SingleLineCommentMode for such allowed syntax:
			// // <?php echo "Hello world"; // comment ?>
			p.PopMode()
		}
		p.PopMode()

		if token.GetText() == "</script>" {
			p._phpScript = false
			token.GetTokenType()
		}
	case PHPLexerHtmlName:
		p._htmlNameText = token.GetText()
	case PHPLexerHtmlDoubleQuoteString:
		if token.GetText() == "php" && p._htmlNameText == "language" {
			p._phpScript = true
		}
	default:
		mode := reflectGetInt(p.BaseLexer, "mode")
		if mode == PHPLexerHereDoc {
			if token.GetTokenType() == PHPLexerStartHereDoc || token.GetTokenType() == PHPLexerStartNowDoc {
				p._heredocIdentifier = strings.ReplaceAll(strings.TrimSpace(token.GetText()[3:]), "'", "")
			} else if token.GetTokenType() == PHPLexerHereDocText {
				p.PopMode()
				var heredocIdentifier = p.GetHeredocEnd(token.GetText())
				if strings.HasSuffix(strings.TrimSpace(token.GetText()), ";") {
					var text = heredocIdentifier + ";\n"
					token.SetTokenIndex(PHPLexerSemiColon)
					token.SetText(text)
				} else {
					token = p.BaseLexer.NextToken()
					token.SetText(heredocIdentifier + ";\n")
				}
			}
		} else if mode == PHPLexerPHP {
			if reflectGetInt(p.BaseLexer, "channel") == antlr.TokenHiddenChannel {
				p._prevTokenType = token.GetTokenType()
			}
		}

	}
	return token
}

func (p *PHPLexerBase) GetHeredocEnd(i string) string {
	return strings.TrimRight(strings.TrimSpace(i), ";")
}

func (p *PHPLexerBase) PushModeOnHtmlClose() {
	p.PopMode()
	if p._scriptTag {
		if !p._phpScript {
			p.PushMode(PHPLexerSCRIPT)
		} else {
			p.PushMode(PHPLexerPHP)
		}
		p._scriptTag = false
	} else if p._styleTag {
		p.PushMode(PHPLexerSTYLE)
		p._styleTag = false
	}
}

func (p *PHPLexerBase) PopModeOnCurlyBracketClose() {
	if p._insideString {
		p._insideString = false
		p.SetChannel(PHPLexerSkipChannel)
		p.PopMode()
	}
}

func (p *PHPLexerBase) SetInsideString() {
	p._insideString = true
}

func (p *PHPLexerBase) IsNewLineOrStart(i int) bool {
	laLeft1 := p.GetInputStream().LA(-1)
	return laLeft1 == '\n' || laLeft1 == '\r' || laLeft1 <= 0
}

func (p *PHPLexerBase) HasAspTags() bool {
	return p._astTags
}

func (p *PHPLexerBase) HasPhpScriptTag() bool {
	return p._phpScript
}

func (p *PHPLexerBase) ShouldPushHereDocMode(i int) bool {
	t := p.GetInputStream().LA(i)
	return t == '\r' || t == '\n'
}

func (p *PHPLexerBase) IsCurlyDollar(i int) bool {
	return p.GetInputStream().LA(i) == '$'
}
