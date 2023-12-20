package JS

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
)

// var AtomicNCost int64 = 0

// JavaScriptParserBase implementation.
type JavaScriptParserBase struct {
	*antlr.BaseParser

	braceDepth int64
}

// Short for p.prev(str string)
func (p *JavaScriptParserBase) p(str string) bool {
	return p.prev(str)
}

// Whether the previous token value equals to str.
func (p *JavaScriptParserBase) prev(str string) bool {
	return p.GetTokenStream().LT(-1).GetText() == str
}

// Short for p.next(str string)
func (p *JavaScriptParserBase) n(str string) bool {
	return p.next(str)
}

// var count int

// func (p *JavaScriptParserBase) log(i string) {
// 	count++
// 	log.Infof("match syntax log [%v]: %v", count, i)
// }

// Whether the next token value equals to str.
func (p *JavaScriptParserBase) next(str string) bool {
	return p.GetTokenStream().LT(1).GetText() == str
}

func (p *JavaScriptParserBase) notLineTerminator() bool {
	b := !p.here(JavaScriptParserLineTerminator)
	return b
}

var count = 0

func (p *JavaScriptParserBase) log(i any) bool {
	count++
	log.Infof("match syntax log[%v]: %v", count, i)
	return true
}

func (p *JavaScriptParserBase) notMatchField() bool {
	// start := time.Now()
	// defer func() {
	// 	atomic.AddInt64(&AtomicNCost, int64(time.Since(start)))
	// 	fmt.Println("notMatchField cost time: ", time.Duration(AtomicNCost).String())
	// }()
	str := p.GetTokenStream().LT(1).GetText()
	// fmt.Println("token1：", str)
	if str == "?" && p.GetTokenStream().LT(2).GetText() == "." {
		ret := p.GetTokenStream().LT(3).GetText()
		switch ret {
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			return false
		case "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z":
			return true
		default:
			return false
		}
	}

	return true
}

var EOSCOUNT = 0

func (p *JavaScriptParserBase) notOpenBraceAndNotFunction() bool {
	nextTokenType := p.GetTokenStream().LT(1).GetTokenType()
	return nextTokenType != JavaScriptParserOpenBrace && nextTokenType != JavaScriptParserFunction_
}

func (p *JavaScriptParserBase) closeBrace() bool {
	EOSCOUNT++
	//log.Infof("closeBrace EOS: %v", EOSCOUNT)
	return p.GetTokenStream().LT(1).GetTokenType() == JavaScriptParserCloseBrace
}

// Returns true if on the current index of the parser's
// token stream a token of the given type exists on the
// Hidden channel.
func (p *JavaScriptParserBase) here(_type int) bool {
	// Get the token ahead of the current index.
	possibleIndexEosToken := p.GetCurrentToken().GetTokenIndex() - 1
	ahead := p.GetTokenStream().Get(possibleIndexEosToken)

	// Check if the token resides on the HIDDEN channel and if it's of the
	// provided type.
	return ahead.GetChannel() == antlr.LexerHidden && ahead.GetTokenType() == _type
}

// Returns true if on the current index of the parser's
// token stream a token exists on the Hidden channel which
// either is a line terminator, or is a multi line comment that
// contains a line terminator.
func (p *JavaScriptParserBase) isEOS() bool {
	afterToken := p.GetTokenStream().LT(1)
	if afterToken.GetTokenType() == JavaScriptParserCloseBrace {
		return true
	}
	possibleIndexEosToken := p.GetCurrentToken().GetTokenIndex() - 1
	if possibleIndexEosToken < 0 {
		return true
	}
	ahead := p.GetTokenStream().Get(possibleIndexEosToken)
	switch ahead.GetTokenType() {
	case JavaScriptParserMultiLineComment:
		return true
	case JavaScriptLexerLineTerminator:
		return true
	default:
		return false
	}
}
