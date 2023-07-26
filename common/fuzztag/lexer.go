package fuzztag

import (
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

type FuzzTagLexer struct {
	Origin      []byte
	tokens      *utils.Stack[any]
	tokensCache *[]*token
}

func NewFuzzTagLexer(i interface{}) *FuzzTagLexer {

	lexer := &FuzzTagLexer{
		Origin: utils.InterfaceToBytes(i),
		tokens: utils.NewStack[any](),
	}
	lexer.parse()
	return lexer
}
func (f *FuzzTagLexer) Tokens() []*token {
	if f.tokensCache != nil {
		return *f.tokensCache
	}
	tokens := make([]*token, f.tokens.Size())
	for i := f.tokens.Size(); i > 0; i-- {
		tokens[i-1] = f.tokens.Pop().(*token)
	}
	f.tokensCache = &tokens
	return tokens
}
func (f *FuzzTagLexer) parse() {
	bs := f.Origin
	var escape bool
	start := 0
	for i := 0; i < len(bs); i++ {
		b := bs[i]
		switch b {
		case '{':
			if i+1 < len(bs) && bs[i+1] == '{' {
				if i >= 0 && start < i {
					f.pushToken(bs[start:i], "DATA", TokenType_DATA)
				}
				i++
				start = i + 1
				f.pushToken([]byte("{{"), TAG_OPEN_VERBOSE, TokenType_TAG_OPEN)
			}
		case '(':
			if escape {
				break
			}
			fname := bs[start:i]
			lastToken := f.tokens.Peek()
			if lastToken != nil && lastToken.(*token).Type == TokenType_TAG_OPEN && !isIdentifyString(strings.TrimSpace(string(fname))) {
				f.tokens.Pop() // pop掉左标签
				start--        // 回退一个字符
				i = start - 1
				f.pushToken([]byte("{"), "DATA", TokenType_DATA) // 补一个
				continue

			}
			f.pushToken(fname, "DATA", TokenType_DATA)
			f.pushToken([]byte("("), LEFT_PAREN_VERBOSE, TokenType_LEFT_PAREN)
			start = i + 1
		case ')':
			if escape {
				break
			}
			f.pushToken(bs[start:i], "DATA", TokenType_DATA)
			f.pushToken([]byte(")"), RIGHT_PAREN_VERBOSE, TokenType_RIGHT_PAREN)
			start = i + 1
		case '}':
			if i+1 < len(bs) && bs[i+1] == '}' {
				if i >= 0 && start < i {
					f.pushToken(bs[start:i], "DATA", TokenType_DATA)
				}
				i++
				start = i + 1
				f.pushToken([]byte("}}"), TAG_CLOSE_VERBOSE, TokenType_TAG_CLOSE)
			}
		}
		if b == '\\' {
			escape = true
		} else {
			escape = false
		}
	}
	if start < len(bs) {
		f.pushToken(bs[start:], "", TokenType_DATA)
	}
}
func (f *FuzzTagLexer) pushToken(raw []byte, verbose string, typ TokenType) {
	if len(raw) == 0 {
		return
	}
	cp := make([]byte, len(raw))
	copy(cp, raw)
	f.tokens.Push(&token{
		Raw:     cp,
		Verbose: verbose,
		Type:    typ,
	})
}
func (f *FuzzTagLexer) ShowTokens() {
	s := []string{}
	for !f.tokens.IsEmpty() {
		t := f.tokens.Pop().(*token)
		s = append(s, string(t.Raw))
	}
	println(strings.Join(s, ","))
}
