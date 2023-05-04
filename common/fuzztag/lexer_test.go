package fuzztag

import "testing"

func TestNewFuzzTagLexer(t *testing.T) {
	lexer := NewFuzzTagLexer(`asdfasdjdsfasdfa{{

int(1-2{{

port(1111-222{{abc}} {{aaaaa}} 2)}}3)

}}
abas
sdf`)
	for _, t := range lexer.Tokens() {
		println(t.Verbose)
	}
}

func TestNewFuzzTagLexer2(t *testing.T) {
	lexer := NewFuzzTagLexer(`asdfasdjdsfasdfa{{x(1-23)}}
abas
sdf`)
	for _, t := range lexer.Tokens() {
		println(t.Verbose)
	}
}
