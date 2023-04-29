package fuzztag

import (
	"yaklang/common/utils"
)

type token struct {
	Raw     []byte
	Verbose string
	Type    TokenType
}

type TokenType string

const (
	TokenType_TAG_OPEN    = "TAG_OPEN"
	TokenType_TAG_CLOSE   = "TAG_CLOSE"
	TokenType_DATA        = "DATA"
	TokenType_METHOND     = "Method"
	TokenType_LEFT_PAREN  = "LEFT_PAREN"
	TokenType_RIGHT_PAREN = "RIGHT_PAREN"
)

const (
	TAG_OPEN            = "{{"
	TAG_OPEN_VERBOSE    = "TAG_OPEN"
	TAG_CLOSE           = "}}"
	TAG_CLOSE_VERBOSE   = "TAG_CLOSE"
	LEFT_PAREN          = '('
	LEFT_PAREN_VERBOSE  = "LEFT_PAREN"
	RIGHT_PAREN         = ')'
	RIGHT_PAREN_VERBOSE = "RIGHT_PAREN"
)

func isIdentifyFirstByte(b byte) bool {
	if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_' {
		return true
	}
	return false
}

var blankByteTable = make(map[byte]byte)

func isBlank(b byte) bool {
	_, ok := blankByteTable[b]
	return ok
}

var identifyRestByteTable = make(map[byte]byte)

func init() {
	for _, b := range "abcdefghijklmnopqrstuvwzxy_1234567890ABCDEFGHJIKLMNOPQRSTUVWXYZ:-" {
		identifyRestByteTable[byte(b)] = byte(b)
	}

	for _, b := range " \n\r\v\t" {
		blankByteTable[byte(b)] = byte(b)
	}
}
func isIdentifyString(s string) bool {
	return utils.MatchAllOfRegexp(s, "^[a-zA-Z_][a-zA-Z0-9_:-]*$")
}

func isIdentifyRestByte(b byte) bool {
	_, ok := identifyRestByteTable[b]
	return ok
}
