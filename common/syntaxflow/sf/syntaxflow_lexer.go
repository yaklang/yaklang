// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package sf

import (
	"fmt"
	"sync"
	"unicode"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

// Suppress unused import error
var _ = fmt.Printf
var _ = sync.Once{}
var _ = unicode.IsLetter

type SyntaxFlowLexer struct {
	*antlr.BaseLexer
	channelNames []string
	modeNames    []string
	// TODO: EOF string
}

var syntaxflowlexerLexerStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	channelNames           []string
	modeNames              []string
	literalNames           []string
	symbolicNames          []string
	ruleNames              []string
	predictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func syntaxflowlexerLexerInit() {
	staticData := &syntaxflowlexerLexerStaticData
	staticData.channelNames = []string{
		"DEFAULT_TOKEN_CHANNEL", "HIDDEN",
	}
	staticData.modeNames = []string{
		"DEFAULT_MODE",
	}
	staticData.literalNames = []string{
		"", "';'", "'==>'", "'...'", "'%%'", "'..'", "'<='", "'>='", "'<<'",
		"'>>'", "'=>'", "'=='", "'=~'", "'!~'", "'&&'", "'||'", "'!='", "'>'",
		"'.'", "'<'", "'='", "'('", "','", "')'", "'['", "']'", "'{'", "'}'",
		"'#'", "'$'", "':'", "'%'", "'!'", "", "", "", "", "", "", "'str'",
		"'list'", "'dict'", "", "'bool'",
	}
	staticData.symbolicNames = []string{
		"", "", "DeepFilter", "Deep", "Percent", "DeepDot", "LtEq", "GtEq",
		"DoubleLt", "DoubleGt", "Filter", "EqEq", "RegexpMatch", "NotRegexpMatch",
		"And", "Or", "NotEq", "Gt", "Dot", "Lt", "Eq", "OpenParen", "Comma",
		"CloseParen", "ListSelectOpen", "ListSelectClose", "MapBuilderOpen",
		"MapBuilderClose", "ListStart", "DollarOutput", "Colon", "Search", "Bang",
		"WhiteSpace", "Number", "OctalNumber", "BinaryNumber", "HexNumber",
		"StringLiteral", "StringType", "ListType", "DictType", "NumberType",
		"BoolType", "BoolLiteral", "Identifier", "RegexpLiteral",
	}
	staticData.ruleNames = []string{
		"T__0", "DeepFilter", "Deep", "Percent", "DeepDot", "LtEq", "GtEq",
		"DoubleLt", "DoubleGt", "Filter", "EqEq", "RegexpMatch", "NotRegexpMatch",
		"And", "Or", "NotEq", "Gt", "Dot", "Lt", "Eq", "OpenParen", "Comma",
		"CloseParen", "ListSelectOpen", "ListSelectClose", "MapBuilderOpen",
		"MapBuilderClose", "ListStart", "DollarOutput", "Colon", "Search", "Bang",
		"WhiteSpace", "Number", "OctalNumber", "BinaryNumber", "HexNumber",
		"StringLiteral", "StringType", "ListType", "DictType", "NumberType",
		"BoolType", "BoolLiteral", "Identifier", "IdentifierCharStart", "IdentifierChar",
		"HexDigit", "Digit", "OctalDigit", "RegexpLiteral", "RegexpLiteralChar",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 0, 46, 303, 6, -1, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2,
		4, 7, 4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2,
		10, 7, 10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15,
		7, 15, 2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7,
		20, 2, 21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25,
		2, 26, 7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 2, 29, 7, 29, 2, 30, 7, 30, 2,
		31, 7, 31, 2, 32, 7, 32, 2, 33, 7, 33, 2, 34, 7, 34, 2, 35, 7, 35, 2, 36,
		7, 36, 2, 37, 7, 37, 2, 38, 7, 38, 2, 39, 7, 39, 2, 40, 7, 40, 2, 41, 7,
		41, 2, 42, 7, 42, 2, 43, 7, 43, 2, 44, 7, 44, 2, 45, 7, 45, 2, 46, 7, 46,
		2, 47, 7, 47, 2, 48, 7, 48, 2, 49, 7, 49, 2, 50, 7, 50, 2, 51, 7, 51, 1,
		0, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 2, 1, 2, 1, 3, 1, 3, 1,
		3, 1, 4, 1, 4, 1, 4, 1, 5, 1, 5, 1, 5, 1, 6, 1, 6, 1, 6, 1, 7, 1, 7, 1,
		7, 1, 8, 1, 8, 1, 8, 1, 9, 1, 9, 1, 9, 1, 10, 1, 10, 1, 10, 1, 11, 1, 11,
		1, 11, 1, 12, 1, 12, 1, 12, 1, 13, 1, 13, 1, 13, 1, 14, 1, 14, 1, 14, 1,
		15, 1, 15, 1, 15, 1, 16, 1, 16, 1, 17, 1, 17, 1, 18, 1, 18, 1, 19, 1, 19,
		1, 20, 1, 20, 1, 21, 1, 21, 1, 22, 1, 22, 1, 23, 1, 23, 1, 24, 1, 24, 1,
		25, 1, 25, 1, 26, 1, 26, 1, 27, 1, 27, 1, 28, 1, 28, 1, 29, 1, 29, 1, 30,
		1, 30, 1, 31, 1, 31, 1, 32, 1, 32, 1, 32, 1, 32, 1, 33, 4, 33, 192, 8,
		33, 11, 33, 12, 33, 193, 1, 34, 1, 34, 1, 34, 1, 34, 4, 34, 200, 8, 34,
		11, 34, 12, 34, 201, 1, 35, 1, 35, 1, 35, 1, 35, 4, 35, 208, 8, 35, 11,
		35, 12, 35, 209, 1, 36, 1, 36, 1, 36, 1, 36, 4, 36, 216, 8, 36, 11, 36,
		12, 36, 217, 1, 37, 1, 37, 5, 37, 222, 8, 37, 10, 37, 12, 37, 225, 9, 37,
		1, 37, 1, 37, 1, 38, 1, 38, 1, 38, 1, 38, 1, 39, 1, 39, 1, 39, 1, 39, 1,
		39, 1, 40, 1, 40, 1, 40, 1, 40, 1, 40, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41,
		1, 41, 1, 41, 1, 41, 3, 41, 251, 8, 41, 1, 42, 1, 42, 1, 42, 1, 42, 1,
		42, 1, 43, 1, 43, 1, 43, 1, 43, 1, 43, 1, 43, 1, 43, 1, 43, 1, 43, 3, 43,
		267, 8, 43, 1, 44, 1, 44, 5, 44, 271, 8, 44, 10, 44, 12, 44, 274, 9, 44,
		1, 45, 1, 45, 1, 45, 3, 45, 279, 8, 45, 1, 46, 1, 46, 3, 46, 283, 8, 46,
		1, 47, 1, 47, 1, 48, 1, 48, 1, 49, 1, 49, 1, 50, 1, 50, 4, 50, 293, 8,
		50, 11, 50, 12, 50, 294, 1, 50, 1, 50, 1, 51, 1, 51, 1, 51, 3, 51, 302,
		8, 51, 0, 0, 52, 1, 1, 3, 2, 5, 3, 7, 4, 9, 5, 11, 6, 13, 7, 15, 8, 17,
		9, 19, 10, 21, 11, 23, 12, 25, 13, 27, 14, 29, 15, 31, 16, 33, 17, 35,
		18, 37, 19, 39, 20, 41, 21, 43, 22, 45, 23, 47, 24, 49, 25, 51, 26, 53,
		27, 55, 28, 57, 29, 59, 30, 61, 31, 63, 32, 65, 33, 67, 34, 69, 35, 71,
		36, 73, 37, 75, 38, 77, 39, 79, 40, 81, 41, 83, 42, 85, 43, 87, 44, 89,
		45, 91, 0, 93, 0, 95, 0, 97, 0, 99, 0, 101, 46, 103, 0, 1, 0, 7, 3, 0,
		10, 10, 13, 13, 32, 32, 1, 0, 96, 96, 4, 0, 37, 37, 65, 90, 95, 95, 97,
		122, 1, 0, 48, 57, 3, 0, 48, 57, 65, 70, 97, 102, 1, 0, 48, 55, 1, 0, 47,
		47, 308, 0, 1, 1, 0, 0, 0, 0, 3, 1, 0, 0, 0, 0, 5, 1, 0, 0, 0, 0, 7, 1,
		0, 0, 0, 0, 9, 1, 0, 0, 0, 0, 11, 1, 0, 0, 0, 0, 13, 1, 0, 0, 0, 0, 15,
		1, 0, 0, 0, 0, 17, 1, 0, 0, 0, 0, 19, 1, 0, 0, 0, 0, 21, 1, 0, 0, 0, 0,
		23, 1, 0, 0, 0, 0, 25, 1, 0, 0, 0, 0, 27, 1, 0, 0, 0, 0, 29, 1, 0, 0, 0,
		0, 31, 1, 0, 0, 0, 0, 33, 1, 0, 0, 0, 0, 35, 1, 0, 0, 0, 0, 37, 1, 0, 0,
		0, 0, 39, 1, 0, 0, 0, 0, 41, 1, 0, 0, 0, 0, 43, 1, 0, 0, 0, 0, 45, 1, 0,
		0, 0, 0, 47, 1, 0, 0, 0, 0, 49, 1, 0, 0, 0, 0, 51, 1, 0, 0, 0, 0, 53, 1,
		0, 0, 0, 0, 55, 1, 0, 0, 0, 0, 57, 1, 0, 0, 0, 0, 59, 1, 0, 0, 0, 0, 61,
		1, 0, 0, 0, 0, 63, 1, 0, 0, 0, 0, 65, 1, 0, 0, 0, 0, 67, 1, 0, 0, 0, 0,
		69, 1, 0, 0, 0, 0, 71, 1, 0, 0, 0, 0, 73, 1, 0, 0, 0, 0, 75, 1, 0, 0, 0,
		0, 77, 1, 0, 0, 0, 0, 79, 1, 0, 0, 0, 0, 81, 1, 0, 0, 0, 0, 83, 1, 0, 0,
		0, 0, 85, 1, 0, 0, 0, 0, 87, 1, 0, 0, 0, 0, 89, 1, 0, 0, 0, 0, 101, 1,
		0, 0, 0, 1, 105, 1, 0, 0, 0, 3, 107, 1, 0, 0, 0, 5, 111, 1, 0, 0, 0, 7,
		115, 1, 0, 0, 0, 9, 118, 1, 0, 0, 0, 11, 121, 1, 0, 0, 0, 13, 124, 1, 0,
		0, 0, 15, 127, 1, 0, 0, 0, 17, 130, 1, 0, 0, 0, 19, 133, 1, 0, 0, 0, 21,
		136, 1, 0, 0, 0, 23, 139, 1, 0, 0, 0, 25, 142, 1, 0, 0, 0, 27, 145, 1,
		0, 0, 0, 29, 148, 1, 0, 0, 0, 31, 151, 1, 0, 0, 0, 33, 154, 1, 0, 0, 0,
		35, 156, 1, 0, 0, 0, 37, 158, 1, 0, 0, 0, 39, 160, 1, 0, 0, 0, 41, 162,
		1, 0, 0, 0, 43, 164, 1, 0, 0, 0, 45, 166, 1, 0, 0, 0, 47, 168, 1, 0, 0,
		0, 49, 170, 1, 0, 0, 0, 51, 172, 1, 0, 0, 0, 53, 174, 1, 0, 0, 0, 55, 176,
		1, 0, 0, 0, 57, 178, 1, 0, 0, 0, 59, 180, 1, 0, 0, 0, 61, 182, 1, 0, 0,
		0, 63, 184, 1, 0, 0, 0, 65, 186, 1, 0, 0, 0, 67, 191, 1, 0, 0, 0, 69, 195,
		1, 0, 0, 0, 71, 203, 1, 0, 0, 0, 73, 211, 1, 0, 0, 0, 75, 219, 1, 0, 0,
		0, 77, 228, 1, 0, 0, 0, 79, 232, 1, 0, 0, 0, 81, 237, 1, 0, 0, 0, 83, 250,
		1, 0, 0, 0, 85, 252, 1, 0, 0, 0, 87, 266, 1, 0, 0, 0, 89, 268, 1, 0, 0,
		0, 91, 278, 1, 0, 0, 0, 93, 282, 1, 0, 0, 0, 95, 284, 1, 0, 0, 0, 97, 286,
		1, 0, 0, 0, 99, 288, 1, 0, 0, 0, 101, 290, 1, 0, 0, 0, 103, 301, 1, 0,
		0, 0, 105, 106, 5, 59, 0, 0, 106, 2, 1, 0, 0, 0, 107, 108, 5, 61, 0, 0,
		108, 109, 5, 61, 0, 0, 109, 110, 5, 62, 0, 0, 110, 4, 1, 0, 0, 0, 111,
		112, 5, 46, 0, 0, 112, 113, 5, 46, 0, 0, 113, 114, 5, 46, 0, 0, 114, 6,
		1, 0, 0, 0, 115, 116, 5, 37, 0, 0, 116, 117, 5, 37, 0, 0, 117, 8, 1, 0,
		0, 0, 118, 119, 5, 46, 0, 0, 119, 120, 5, 46, 0, 0, 120, 10, 1, 0, 0, 0,
		121, 122, 5, 60, 0, 0, 122, 123, 5, 61, 0, 0, 123, 12, 1, 0, 0, 0, 124,
		125, 5, 62, 0, 0, 125, 126, 5, 61, 0, 0, 126, 14, 1, 0, 0, 0, 127, 128,
		5, 60, 0, 0, 128, 129, 5, 60, 0, 0, 129, 16, 1, 0, 0, 0, 130, 131, 5, 62,
		0, 0, 131, 132, 5, 62, 0, 0, 132, 18, 1, 0, 0, 0, 133, 134, 5, 61, 0, 0,
		134, 135, 5, 62, 0, 0, 135, 20, 1, 0, 0, 0, 136, 137, 5, 61, 0, 0, 137,
		138, 5, 61, 0, 0, 138, 22, 1, 0, 0, 0, 139, 140, 5, 61, 0, 0, 140, 141,
		5, 126, 0, 0, 141, 24, 1, 0, 0, 0, 142, 143, 5, 33, 0, 0, 143, 144, 5,
		126, 0, 0, 144, 26, 1, 0, 0, 0, 145, 146, 5, 38, 0, 0, 146, 147, 5, 38,
		0, 0, 147, 28, 1, 0, 0, 0, 148, 149, 5, 124, 0, 0, 149, 150, 5, 124, 0,
		0, 150, 30, 1, 0, 0, 0, 151, 152, 5, 33, 0, 0, 152, 153, 5, 61, 0, 0, 153,
		32, 1, 0, 0, 0, 154, 155, 5, 62, 0, 0, 155, 34, 1, 0, 0, 0, 156, 157, 5,
		46, 0, 0, 157, 36, 1, 0, 0, 0, 158, 159, 5, 60, 0, 0, 159, 38, 1, 0, 0,
		0, 160, 161, 5, 61, 0, 0, 161, 40, 1, 0, 0, 0, 162, 163, 5, 40, 0, 0, 163,
		42, 1, 0, 0, 0, 164, 165, 5, 44, 0, 0, 165, 44, 1, 0, 0, 0, 166, 167, 5,
		41, 0, 0, 167, 46, 1, 0, 0, 0, 168, 169, 5, 91, 0, 0, 169, 48, 1, 0, 0,
		0, 170, 171, 5, 93, 0, 0, 171, 50, 1, 0, 0, 0, 172, 173, 5, 123, 0, 0,
		173, 52, 1, 0, 0, 0, 174, 175, 5, 125, 0, 0, 175, 54, 1, 0, 0, 0, 176,
		177, 5, 35, 0, 0, 177, 56, 1, 0, 0, 0, 178, 179, 5, 36, 0, 0, 179, 58,
		1, 0, 0, 0, 180, 181, 5, 58, 0, 0, 181, 60, 1, 0, 0, 0, 182, 183, 5, 37,
		0, 0, 183, 62, 1, 0, 0, 0, 184, 185, 5, 33, 0, 0, 185, 64, 1, 0, 0, 0,
		186, 187, 7, 0, 0, 0, 187, 188, 1, 0, 0, 0, 188, 189, 6, 32, 0, 0, 189,
		66, 1, 0, 0, 0, 190, 192, 3, 97, 48, 0, 191, 190, 1, 0, 0, 0, 192, 193,
		1, 0, 0, 0, 193, 191, 1, 0, 0, 0, 193, 194, 1, 0, 0, 0, 194, 68, 1, 0,
		0, 0, 195, 196, 5, 48, 0, 0, 196, 197, 5, 111, 0, 0, 197, 199, 1, 0, 0,
		0, 198, 200, 3, 99, 49, 0, 199, 198, 1, 0, 0, 0, 200, 201, 1, 0, 0, 0,
		201, 199, 1, 0, 0, 0, 201, 202, 1, 0, 0, 0, 202, 70, 1, 0, 0, 0, 203, 204,
		5, 48, 0, 0, 204, 205, 5, 98, 0, 0, 205, 207, 1, 0, 0, 0, 206, 208, 2,
		48, 49, 0, 207, 206, 1, 0, 0, 0, 208, 209, 1, 0, 0, 0, 209, 207, 1, 0,
		0, 0, 209, 210, 1, 0, 0, 0, 210, 72, 1, 0, 0, 0, 211, 212, 5, 48, 0, 0,
		212, 213, 5, 120, 0, 0, 213, 215, 1, 0, 0, 0, 214, 216, 3, 95, 47, 0, 215,
		214, 1, 0, 0, 0, 216, 217, 1, 0, 0, 0, 217, 215, 1, 0, 0, 0, 217, 218,
		1, 0, 0, 0, 218, 74, 1, 0, 0, 0, 219, 223, 5, 96, 0, 0, 220, 222, 8, 1,
		0, 0, 221, 220, 1, 0, 0, 0, 222, 225, 1, 0, 0, 0, 223, 221, 1, 0, 0, 0,
		223, 224, 1, 0, 0, 0, 224, 226, 1, 0, 0, 0, 225, 223, 1, 0, 0, 0, 226,
		227, 5, 96, 0, 0, 227, 76, 1, 0, 0, 0, 228, 229, 5, 115, 0, 0, 229, 230,
		5, 116, 0, 0, 230, 231, 5, 114, 0, 0, 231, 78, 1, 0, 0, 0, 232, 233, 5,
		108, 0, 0, 233, 234, 5, 105, 0, 0, 234, 235, 5, 115, 0, 0, 235, 236, 5,
		116, 0, 0, 236, 80, 1, 0, 0, 0, 237, 238, 5, 100, 0, 0, 238, 239, 5, 105,
		0, 0, 239, 240, 5, 99, 0, 0, 240, 241, 5, 116, 0, 0, 241, 82, 1, 0, 0,
		0, 242, 243, 5, 105, 0, 0, 243, 244, 5, 110, 0, 0, 244, 251, 5, 116, 0,
		0, 245, 246, 5, 102, 0, 0, 246, 247, 5, 108, 0, 0, 247, 248, 5, 111, 0,
		0, 248, 249, 5, 97, 0, 0, 249, 251, 5, 116, 0, 0, 250, 242, 1, 0, 0, 0,
		250, 245, 1, 0, 0, 0, 251, 84, 1, 0, 0, 0, 252, 253, 5, 98, 0, 0, 253,
		254, 5, 111, 0, 0, 254, 255, 5, 111, 0, 0, 255, 256, 5, 108, 0, 0, 256,
		86, 1, 0, 0, 0, 257, 258, 5, 116, 0, 0, 258, 259, 5, 114, 0, 0, 259, 260,
		5, 117, 0, 0, 260, 267, 5, 101, 0, 0, 261, 262, 5, 102, 0, 0, 262, 263,
		5, 97, 0, 0, 263, 264, 5, 108, 0, 0, 264, 265, 5, 115, 0, 0, 265, 267,
		5, 101, 0, 0, 266, 257, 1, 0, 0, 0, 266, 261, 1, 0, 0, 0, 267, 88, 1, 0,
		0, 0, 268, 272, 3, 91, 45, 0, 269, 271, 3, 93, 46, 0, 270, 269, 1, 0, 0,
		0, 271, 274, 1, 0, 0, 0, 272, 270, 1, 0, 0, 0, 272, 273, 1, 0, 0, 0, 273,
		90, 1, 0, 0, 0, 274, 272, 1, 0, 0, 0, 275, 279, 7, 2, 0, 0, 276, 277, 5,
		37, 0, 0, 277, 279, 5, 37, 0, 0, 278, 275, 1, 0, 0, 0, 278, 276, 1, 0,
		0, 0, 279, 92, 1, 0, 0, 0, 280, 283, 7, 3, 0, 0, 281, 283, 3, 91, 45, 0,
		282, 280, 1, 0, 0, 0, 282, 281, 1, 0, 0, 0, 283, 94, 1, 0, 0, 0, 284, 285,
		7, 4, 0, 0, 285, 96, 1, 0, 0, 0, 286, 287, 7, 3, 0, 0, 287, 98, 1, 0, 0,
		0, 288, 289, 7, 5, 0, 0, 289, 100, 1, 0, 0, 0, 290, 292, 5, 47, 0, 0, 291,
		293, 3, 103, 51, 0, 292, 291, 1, 0, 0, 0, 293, 294, 1, 0, 0, 0, 294, 292,
		1, 0, 0, 0, 294, 295, 1, 0, 0, 0, 295, 296, 1, 0, 0, 0, 296, 297, 5, 47,
		0, 0, 297, 102, 1, 0, 0, 0, 298, 299, 5, 92, 0, 0, 299, 302, 5, 47, 0,
		0, 300, 302, 8, 6, 0, 0, 301, 298, 1, 0, 0, 0, 301, 300, 1, 0, 0, 0, 302,
		104, 1, 0, 0, 0, 13, 0, 193, 201, 209, 217, 223, 250, 266, 272, 278, 282,
		294, 301, 1, 6, 0, 0,
	}
	deserializer := antlr.NewATNDeserializer(nil)
	staticData.atn = deserializer.Deserialize(staticData.serializedATN)
	atn := staticData.atn
	staticData.decisionToDFA = make([]*antlr.DFA, len(atn.DecisionToState))
	decisionToDFA := staticData.decisionToDFA
	for index, state := range atn.DecisionToState {
		decisionToDFA[index] = antlr.NewDFA(state, index)
	}
}

// SyntaxFlowLexerInit initializes any static state used to implement SyntaxFlowLexer. By default the
// static state used to implement the lexer is lazily initialized during the first call to
// NewSyntaxFlowLexer(). You can call this function if you wish to initialize the static state ahead
// of time.
func SyntaxFlowLexerInit() {
	staticData := &syntaxflowlexerLexerStaticData
	staticData.once.Do(syntaxflowlexerLexerInit)
}

// NewSyntaxFlowLexer produces a new lexer instance for the optional input antlr.CharStream.
func NewSyntaxFlowLexer(input antlr.CharStream) *SyntaxFlowLexer {
	SyntaxFlowLexerInit()
	l := new(SyntaxFlowLexer)
	l.BaseLexer = antlr.NewBaseLexer(input)
	staticData := &syntaxflowlexerLexerStaticData
	l.Interpreter = antlr.NewLexerATNSimulator(l, staticData.atn, staticData.decisionToDFA, staticData.predictionContextCache)
	l.channelNames = staticData.channelNames
	l.modeNames = staticData.modeNames
	l.RuleNames = staticData.ruleNames
	l.LiteralNames = staticData.literalNames
	l.SymbolicNames = staticData.symbolicNames
	l.GrammarFileName = "SyntaxFlow.g4"
	// TODO: l.EOF = antlr.TokenEOF

	return l
}

// SyntaxFlowLexer tokens.
const (
	SyntaxFlowLexerT__0            = 1
	SyntaxFlowLexerDeepFilter      = 2
	SyntaxFlowLexerDeep            = 3
	SyntaxFlowLexerPercent         = 4
	SyntaxFlowLexerDeepDot         = 5
	SyntaxFlowLexerLtEq            = 6
	SyntaxFlowLexerGtEq            = 7
	SyntaxFlowLexerDoubleLt        = 8
	SyntaxFlowLexerDoubleGt        = 9
	SyntaxFlowLexerFilter          = 10
	SyntaxFlowLexerEqEq            = 11
	SyntaxFlowLexerRegexpMatch     = 12
	SyntaxFlowLexerNotRegexpMatch  = 13
	SyntaxFlowLexerAnd             = 14
	SyntaxFlowLexerOr              = 15
	SyntaxFlowLexerNotEq           = 16
	SyntaxFlowLexerGt              = 17
	SyntaxFlowLexerDot             = 18
	SyntaxFlowLexerLt              = 19
	SyntaxFlowLexerEq              = 20
	SyntaxFlowLexerOpenParen       = 21
	SyntaxFlowLexerComma           = 22
	SyntaxFlowLexerCloseParen      = 23
	SyntaxFlowLexerListSelectOpen  = 24
	SyntaxFlowLexerListSelectClose = 25
	SyntaxFlowLexerMapBuilderOpen  = 26
	SyntaxFlowLexerMapBuilderClose = 27
	SyntaxFlowLexerListStart       = 28
	SyntaxFlowLexerDollarOutput    = 29
	SyntaxFlowLexerColon           = 30
	SyntaxFlowLexerSearch          = 31
	SyntaxFlowLexerBang            = 32
	SyntaxFlowLexerWhiteSpace      = 33
	SyntaxFlowLexerNumber          = 34
	SyntaxFlowLexerOctalNumber     = 35
	SyntaxFlowLexerBinaryNumber    = 36
	SyntaxFlowLexerHexNumber       = 37
	SyntaxFlowLexerStringLiteral   = 38
	SyntaxFlowLexerStringType      = 39
	SyntaxFlowLexerListType        = 40
	SyntaxFlowLexerDictType        = 41
	SyntaxFlowLexerNumberType      = 42
	SyntaxFlowLexerBoolType        = 43
	SyntaxFlowLexerBoolLiteral     = 44
	SyntaxFlowLexerIdentifier      = 45
	SyntaxFlowLexerRegexpLiteral   = 46
)
