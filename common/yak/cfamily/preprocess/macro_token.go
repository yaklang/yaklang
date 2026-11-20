package preprocess

import "unicode"

// macroTokenKind classifies preprocessor tokens used during function-macro expansion.
type macroTokenKind int

const (
	macroTokIdent macroTokenKind = iota
	macroTokNumber
	macroTokString
	macroTokChar
	macroTokWhitespace
	macroTokNewline
	macroTokPunct
	macroTokComment
)

type macroToken struct {
	kind macroTokenKind
	text string
}

func (t macroToken) isIdent(name string) bool {
	return t.kind == macroTokIdent && t.text == name
}

func isMacroIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isMacroIdentPart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// multiCharPunct lists punctuation sequences to recognize (longest first).
var multiCharPunct = []string{
	"<<=", ">>=", "...", "##", "<<", ">>", "<=", ">=", "==", "!=", "&&", "||", "->", "++", "--",
	"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=",
}

func matchMultiCharPunct(src string, i int) (string, int) {
	for _, p := range multiCharPunct {
		if len(p) > 0 && i+len(p) <= len(src) && src[i:i+len(p)] == p {
			return p, len(p)
		}
	}
	return "", 0
}
