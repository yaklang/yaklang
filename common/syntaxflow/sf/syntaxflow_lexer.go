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
		"'.'", "'<'", "'='", "'?'", "'('", "','", "')'", "'['", "']'", "'{'",
		"'}'", "'#'", "'$'", "':'", "'%'", "'!'", "", "", "", "", "", "", "'str'",
		"'list'", "'dict'", "", "'bool'",
	}
	staticData.symbolicNames = []string{
		"", "", "DeepFilter", "Deep", "Percent", "DeepDot", "LtEq", "GtEq",
		"DoubleLt", "DoubleGt", "Filter", "EqEq", "RegexpMatch", "NotRegexpMatch",
		"And", "Or", "NotEq", "Gt", "Dot", "Lt", "Eq", "Question", "OpenParen",
		"Comma", "CloseParen", "ListSelectOpen", "ListSelectClose", "MapBuilderOpen",
		"MapBuilderClose", "ListStart", "DollarOutput", "Colon", "Search", "Bang",
		"WhiteSpace", "Number", "OctalNumber", "BinaryNumber", "HexNumber",
		"StringLiteral", "StringType", "ListType", "DictType", "NumberType",
		"BoolType", "BoolLiteral", "Identifier", "RegexpLiteral",
	}
	staticData.ruleNames = []string{
		"T__0", "DeepFilter", "Deep", "Percent", "DeepDot", "LtEq", "GtEq",
		"DoubleLt", "DoubleGt", "Filter", "EqEq", "RegexpMatch", "NotRegexpMatch",
		"And", "Or", "NotEq", "Gt", "Dot", "Lt", "Eq", "Question", "OpenParen",
		"Comma", "CloseParen", "ListSelectOpen", "ListSelectClose", "MapBuilderOpen",
		"MapBuilderClose", "ListStart", "DollarOutput", "Colon", "Search", "Bang",
		"WhiteSpace", "Number", "OctalNumber", "BinaryNumber", "HexNumber",
		"StringLiteral", "StringType", "ListType", "DictType", "NumberType",
		"BoolType", "BoolLiteral", "Identifier", "IdentifierCharStart", "IdentifierChar",
		"HexDigit", "Digit", "OctalDigit", "RegexpLiteral", "RegexpLiteralChar",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 0, 47, 307, 6, -1, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2,
		4, 7, 4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2,
		10, 7, 10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15,
		7, 15, 2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7,
		20, 2, 21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25,
		2, 26, 7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 2, 29, 7, 29, 2, 30, 7, 30, 2,
		31, 7, 31, 2, 32, 7, 32, 2, 33, 7, 33, 2, 34, 7, 34, 2, 35, 7, 35, 2, 36,
		7, 36, 2, 37, 7, 37, 2, 38, 7, 38, 2, 39, 7, 39, 2, 40, 7, 40, 2, 41, 7,
		41, 2, 42, 7, 42, 2, 43, 7, 43, 2, 44, 7, 44, 2, 45, 7, 45, 2, 46, 7, 46,
		2, 47, 7, 47, 2, 48, 7, 48, 2, 49, 7, 49, 2, 50, 7, 50, 2, 51, 7, 51, 2,
		52, 7, 52, 1, 0, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 2, 1, 2,
		1, 3, 1, 3, 1, 3, 1, 4, 1, 4, 1, 4, 1, 5, 1, 5, 1, 5, 1, 6, 1, 6, 1, 6,
		1, 7, 1, 7, 1, 7, 1, 8, 1, 8, 1, 8, 1, 9, 1, 9, 1, 9, 1, 10, 1, 10, 1,
		10, 1, 11, 1, 11, 1, 11, 1, 12, 1, 12, 1, 12, 1, 13, 1, 13, 1, 13, 1, 14,
		1, 14, 1, 14, 1, 15, 1, 15, 1, 15, 1, 16, 1, 16, 1, 17, 1, 17, 1, 18, 1,
		18, 1, 19, 1, 19, 1, 20, 1, 20, 1, 21, 1, 21, 1, 22, 1, 22, 1, 23, 1, 23,
		1, 24, 1, 24, 1, 25, 1, 25, 1, 26, 1, 26, 1, 27, 1, 27, 1, 28, 1, 28, 1,
		29, 1, 29, 1, 30, 1, 30, 1, 31, 1, 31, 1, 32, 1, 32, 1, 33, 1, 33, 1, 33,
		1, 33, 1, 34, 4, 34, 196, 8, 34, 11, 34, 12, 34, 197, 1, 35, 1, 35, 1,
		35, 1, 35, 4, 35, 204, 8, 35, 11, 35, 12, 35, 205, 1, 36, 1, 36, 1, 36,
		1, 36, 4, 36, 212, 8, 36, 11, 36, 12, 36, 213, 1, 37, 1, 37, 1, 37, 1,
		37, 4, 37, 220, 8, 37, 11, 37, 12, 37, 221, 1, 38, 1, 38, 5, 38, 226, 8,
		38, 10, 38, 12, 38, 229, 9, 38, 1, 38, 1, 38, 1, 39, 1, 39, 1, 39, 1, 39,
		1, 40, 1, 40, 1, 40, 1, 40, 1, 40, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41, 1,
		42, 1, 42, 1, 42, 1, 42, 1, 42, 1, 42, 1, 42, 1, 42, 3, 42, 255, 8, 42,
		1, 43, 1, 43, 1, 43, 1, 43, 1, 43, 1, 44, 1, 44, 1, 44, 1, 44, 1, 44, 1,
		44, 1, 44, 1, 44, 1, 44, 3, 44, 271, 8, 44, 1, 45, 1, 45, 5, 45, 275, 8,
		45, 10, 45, 12, 45, 278, 9, 45, 1, 46, 1, 46, 1, 46, 3, 46, 283, 8, 46,
		1, 47, 1, 47, 3, 47, 287, 8, 47, 1, 48, 1, 48, 1, 49, 1, 49, 1, 50, 1,
		50, 1, 51, 1, 51, 4, 51, 297, 8, 51, 11, 51, 12, 51, 298, 1, 51, 1, 51,
		1, 52, 1, 52, 1, 52, 3, 52, 306, 8, 52, 0, 0, 53, 1, 1, 3, 2, 5, 3, 7,
		4, 9, 5, 11, 6, 13, 7, 15, 8, 17, 9, 19, 10, 21, 11, 23, 12, 25, 13, 27,
		14, 29, 15, 31, 16, 33, 17, 35, 18, 37, 19, 39, 20, 41, 21, 43, 22, 45,
		23, 47, 24, 49, 25, 51, 26, 53, 27, 55, 28, 57, 29, 59, 30, 61, 31, 63,
		32, 65, 33, 67, 34, 69, 35, 71, 36, 73, 37, 75, 38, 77, 39, 79, 40, 81,
		41, 83, 42, 85, 43, 87, 44, 89, 45, 91, 46, 93, 0, 95, 0, 97, 0, 99, 0,
		101, 0, 103, 47, 105, 0, 1, 0, 7, 3, 0, 10, 10, 13, 13, 32, 32, 1, 0, 96,
		96, 4, 0, 37, 37, 65, 90, 95, 95, 97, 122, 1, 0, 48, 57, 3, 0, 48, 57,
		65, 70, 97, 102, 1, 0, 48, 55, 1, 0, 47, 47, 312, 0, 1, 1, 0, 0, 0, 0,
		3, 1, 0, 0, 0, 0, 5, 1, 0, 0, 0, 0, 7, 1, 0, 0, 0, 0, 9, 1, 0, 0, 0, 0,
		11, 1, 0, 0, 0, 0, 13, 1, 0, 0, 0, 0, 15, 1, 0, 0, 0, 0, 17, 1, 0, 0, 0,
		0, 19, 1, 0, 0, 0, 0, 21, 1, 0, 0, 0, 0, 23, 1, 0, 0, 0, 0, 25, 1, 0, 0,
		0, 0, 27, 1, 0, 0, 0, 0, 29, 1, 0, 0, 0, 0, 31, 1, 0, 0, 0, 0, 33, 1, 0,
		0, 0, 0, 35, 1, 0, 0, 0, 0, 37, 1, 0, 0, 0, 0, 39, 1, 0, 0, 0, 0, 41, 1,
		0, 0, 0, 0, 43, 1, 0, 0, 0, 0, 45, 1, 0, 0, 0, 0, 47, 1, 0, 0, 0, 0, 49,
		1, 0, 0, 0, 0, 51, 1, 0, 0, 0, 0, 53, 1, 0, 0, 0, 0, 55, 1, 0, 0, 0, 0,
		57, 1, 0, 0, 0, 0, 59, 1, 0, 0, 0, 0, 61, 1, 0, 0, 0, 0, 63, 1, 0, 0, 0,
		0, 65, 1, 0, 0, 0, 0, 67, 1, 0, 0, 0, 0, 69, 1, 0, 0, 0, 0, 71, 1, 0, 0,
		0, 0, 73, 1, 0, 0, 0, 0, 75, 1, 0, 0, 0, 0, 77, 1, 0, 0, 0, 0, 79, 1, 0,
		0, 0, 0, 81, 1, 0, 0, 0, 0, 83, 1, 0, 0, 0, 0, 85, 1, 0, 0, 0, 0, 87, 1,
		0, 0, 0, 0, 89, 1, 0, 0, 0, 0, 91, 1, 0, 0, 0, 0, 103, 1, 0, 0, 0, 1, 107,
		1, 0, 0, 0, 3, 109, 1, 0, 0, 0, 5, 113, 1, 0, 0, 0, 7, 117, 1, 0, 0, 0,
		9, 120, 1, 0, 0, 0, 11, 123, 1, 0, 0, 0, 13, 126, 1, 0, 0, 0, 15, 129,
		1, 0, 0, 0, 17, 132, 1, 0, 0, 0, 19, 135, 1, 0, 0, 0, 21, 138, 1, 0, 0,
		0, 23, 141, 1, 0, 0, 0, 25, 144, 1, 0, 0, 0, 27, 147, 1, 0, 0, 0, 29, 150,
		1, 0, 0, 0, 31, 153, 1, 0, 0, 0, 33, 156, 1, 0, 0, 0, 35, 158, 1, 0, 0,
		0, 37, 160, 1, 0, 0, 0, 39, 162, 1, 0, 0, 0, 41, 164, 1, 0, 0, 0, 43, 166,
		1, 0, 0, 0, 45, 168, 1, 0, 0, 0, 47, 170, 1, 0, 0, 0, 49, 172, 1, 0, 0,
		0, 51, 174, 1, 0, 0, 0, 53, 176, 1, 0, 0, 0, 55, 178, 1, 0, 0, 0, 57, 180,
		1, 0, 0, 0, 59, 182, 1, 0, 0, 0, 61, 184, 1, 0, 0, 0, 63, 186, 1, 0, 0,
		0, 65, 188, 1, 0, 0, 0, 67, 190, 1, 0, 0, 0, 69, 195, 1, 0, 0, 0, 71, 199,
		1, 0, 0, 0, 73, 207, 1, 0, 0, 0, 75, 215, 1, 0, 0, 0, 77, 223, 1, 0, 0,
		0, 79, 232, 1, 0, 0, 0, 81, 236, 1, 0, 0, 0, 83, 241, 1, 0, 0, 0, 85, 254,
		1, 0, 0, 0, 87, 256, 1, 0, 0, 0, 89, 270, 1, 0, 0, 0, 91, 272, 1, 0, 0,
		0, 93, 282, 1, 0, 0, 0, 95, 286, 1, 0, 0, 0, 97, 288, 1, 0, 0, 0, 99, 290,
		1, 0, 0, 0, 101, 292, 1, 0, 0, 0, 103, 294, 1, 0, 0, 0, 105, 305, 1, 0,
		0, 0, 107, 108, 5, 59, 0, 0, 108, 2, 1, 0, 0, 0, 109, 110, 5, 61, 0, 0,
		110, 111, 5, 61, 0, 0, 111, 112, 5, 62, 0, 0, 112, 4, 1, 0, 0, 0, 113,
		114, 5, 46, 0, 0, 114, 115, 5, 46, 0, 0, 115, 116, 5, 46, 0, 0, 116, 6,
		1, 0, 0, 0, 117, 118, 5, 37, 0, 0, 118, 119, 5, 37, 0, 0, 119, 8, 1, 0,
		0, 0, 120, 121, 5, 46, 0, 0, 121, 122, 5, 46, 0, 0, 122, 10, 1, 0, 0, 0,
		123, 124, 5, 60, 0, 0, 124, 125, 5, 61, 0, 0, 125, 12, 1, 0, 0, 0, 126,
		127, 5, 62, 0, 0, 127, 128, 5, 61, 0, 0, 128, 14, 1, 0, 0, 0, 129, 130,
		5, 60, 0, 0, 130, 131, 5, 60, 0, 0, 131, 16, 1, 0, 0, 0, 132, 133, 5, 62,
		0, 0, 133, 134, 5, 62, 0, 0, 134, 18, 1, 0, 0, 0, 135, 136, 5, 61, 0, 0,
		136, 137, 5, 62, 0, 0, 137, 20, 1, 0, 0, 0, 138, 139, 5, 61, 0, 0, 139,
		140, 5, 61, 0, 0, 140, 22, 1, 0, 0, 0, 141, 142, 5, 61, 0, 0, 142, 143,
		5, 126, 0, 0, 143, 24, 1, 0, 0, 0, 144, 145, 5, 33, 0, 0, 145, 146, 5,
		126, 0, 0, 146, 26, 1, 0, 0, 0, 147, 148, 5, 38, 0, 0, 148, 149, 5, 38,
		0, 0, 149, 28, 1, 0, 0, 0, 150, 151, 5, 124, 0, 0, 151, 152, 5, 124, 0,
		0, 152, 30, 1, 0, 0, 0, 153, 154, 5, 33, 0, 0, 154, 155, 5, 61, 0, 0, 155,
		32, 1, 0, 0, 0, 156, 157, 5, 62, 0, 0, 157, 34, 1, 0, 0, 0, 158, 159, 5,
		46, 0, 0, 159, 36, 1, 0, 0, 0, 160, 161, 5, 60, 0, 0, 161, 38, 1, 0, 0,
		0, 162, 163, 5, 61, 0, 0, 163, 40, 1, 0, 0, 0, 164, 165, 5, 63, 0, 0, 165,
		42, 1, 0, 0, 0, 166, 167, 5, 40, 0, 0, 167, 44, 1, 0, 0, 0, 168, 169, 5,
		44, 0, 0, 169, 46, 1, 0, 0, 0, 170, 171, 5, 41, 0, 0, 171, 48, 1, 0, 0,
		0, 172, 173, 5, 91, 0, 0, 173, 50, 1, 0, 0, 0, 174, 175, 5, 93, 0, 0, 175,
		52, 1, 0, 0, 0, 176, 177, 5, 123, 0, 0, 177, 54, 1, 0, 0, 0, 178, 179,
		5, 125, 0, 0, 179, 56, 1, 0, 0, 0, 180, 181, 5, 35, 0, 0, 181, 58, 1, 0,
		0, 0, 182, 183, 5, 36, 0, 0, 183, 60, 1, 0, 0, 0, 184, 185, 5, 58, 0, 0,
		185, 62, 1, 0, 0, 0, 186, 187, 5, 37, 0, 0, 187, 64, 1, 0, 0, 0, 188, 189,
		5, 33, 0, 0, 189, 66, 1, 0, 0, 0, 190, 191, 7, 0, 0, 0, 191, 192, 1, 0,
		0, 0, 192, 193, 6, 33, 0, 0, 193, 68, 1, 0, 0, 0, 194, 196, 3, 99, 49,
		0, 195, 194, 1, 0, 0, 0, 196, 197, 1, 0, 0, 0, 197, 195, 1, 0, 0, 0, 197,
		198, 1, 0, 0, 0, 198, 70, 1, 0, 0, 0, 199, 200, 5, 48, 0, 0, 200, 201,
		5, 111, 0, 0, 201, 203, 1, 0, 0, 0, 202, 204, 3, 101, 50, 0, 203, 202,
		1, 0, 0, 0, 204, 205, 1, 0, 0, 0, 205, 203, 1, 0, 0, 0, 205, 206, 1, 0,
		0, 0, 206, 72, 1, 0, 0, 0, 207, 208, 5, 48, 0, 0, 208, 209, 5, 98, 0, 0,
		209, 211, 1, 0, 0, 0, 210, 212, 2, 48, 49, 0, 211, 210, 1, 0, 0, 0, 212,
		213, 1, 0, 0, 0, 213, 211, 1, 0, 0, 0, 213, 214, 1, 0, 0, 0, 214, 74, 1,
		0, 0, 0, 215, 216, 5, 48, 0, 0, 216, 217, 5, 120, 0, 0, 217, 219, 1, 0,
		0, 0, 218, 220, 3, 97, 48, 0, 219, 218, 1, 0, 0, 0, 220, 221, 1, 0, 0,
		0, 221, 219, 1, 0, 0, 0, 221, 222, 1, 0, 0, 0, 222, 76, 1, 0, 0, 0, 223,
		227, 5, 96, 0, 0, 224, 226, 8, 1, 0, 0, 225, 224, 1, 0, 0, 0, 226, 229,
		1, 0, 0, 0, 227, 225, 1, 0, 0, 0, 227, 228, 1, 0, 0, 0, 228, 230, 1, 0,
		0, 0, 229, 227, 1, 0, 0, 0, 230, 231, 5, 96, 0, 0, 231, 78, 1, 0, 0, 0,
		232, 233, 5, 115, 0, 0, 233, 234, 5, 116, 0, 0, 234, 235, 5, 114, 0, 0,
		235, 80, 1, 0, 0, 0, 236, 237, 5, 108, 0, 0, 237, 238, 5, 105, 0, 0, 238,
		239, 5, 115, 0, 0, 239, 240, 5, 116, 0, 0, 240, 82, 1, 0, 0, 0, 241, 242,
		5, 100, 0, 0, 242, 243, 5, 105, 0, 0, 243, 244, 5, 99, 0, 0, 244, 245,
		5, 116, 0, 0, 245, 84, 1, 0, 0, 0, 246, 247, 5, 105, 0, 0, 247, 248, 5,
		110, 0, 0, 248, 255, 5, 116, 0, 0, 249, 250, 5, 102, 0, 0, 250, 251, 5,
		108, 0, 0, 251, 252, 5, 111, 0, 0, 252, 253, 5, 97, 0, 0, 253, 255, 5,
		116, 0, 0, 254, 246, 1, 0, 0, 0, 254, 249, 1, 0, 0, 0, 255, 86, 1, 0, 0,
		0, 256, 257, 5, 98, 0, 0, 257, 258, 5, 111, 0, 0, 258, 259, 5, 111, 0,
		0, 259, 260, 5, 108, 0, 0, 260, 88, 1, 0, 0, 0, 261, 262, 5, 116, 0, 0,
		262, 263, 5, 114, 0, 0, 263, 264, 5, 117, 0, 0, 264, 271, 5, 101, 0, 0,
		265, 266, 5, 102, 0, 0, 266, 267, 5, 97, 0, 0, 267, 268, 5, 108, 0, 0,
		268, 269, 5, 115, 0, 0, 269, 271, 5, 101, 0, 0, 270, 261, 1, 0, 0, 0, 270,
		265, 1, 0, 0, 0, 271, 90, 1, 0, 0, 0, 272, 276, 3, 93, 46, 0, 273, 275,
		3, 95, 47, 0, 274, 273, 1, 0, 0, 0, 275, 278, 1, 0, 0, 0, 276, 274, 1,
		0, 0, 0, 276, 277, 1, 0, 0, 0, 277, 92, 1, 0, 0, 0, 278, 276, 1, 0, 0,
		0, 279, 283, 7, 2, 0, 0, 280, 281, 5, 37, 0, 0, 281, 283, 5, 37, 0, 0,
		282, 279, 1, 0, 0, 0, 282, 280, 1, 0, 0, 0, 283, 94, 1, 0, 0, 0, 284, 287,
		7, 3, 0, 0, 285, 287, 3, 93, 46, 0, 286, 284, 1, 0, 0, 0, 286, 285, 1,
		0, 0, 0, 287, 96, 1, 0, 0, 0, 288, 289, 7, 4, 0, 0, 289, 98, 1, 0, 0, 0,
		290, 291, 7, 3, 0, 0, 291, 100, 1, 0, 0, 0, 292, 293, 7, 5, 0, 0, 293,
		102, 1, 0, 0, 0, 294, 296, 5, 47, 0, 0, 295, 297, 3, 105, 52, 0, 296, 295,
		1, 0, 0, 0, 297, 298, 1, 0, 0, 0, 298, 296, 1, 0, 0, 0, 298, 299, 1, 0,
		0, 0, 299, 300, 1, 0, 0, 0, 300, 301, 5, 47, 0, 0, 301, 104, 1, 0, 0, 0,
		302, 303, 5, 92, 0, 0, 303, 306, 5, 47, 0, 0, 304, 306, 8, 6, 0, 0, 305,
		302, 1, 0, 0, 0, 305, 304, 1, 0, 0, 0, 306, 106, 1, 0, 0, 0, 13, 0, 197,
		205, 213, 221, 227, 254, 270, 276, 282, 286, 298, 305, 1, 6, 0, 0,
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
	SyntaxFlowLexerQuestion        = 21
	SyntaxFlowLexerOpenParen       = 22
	SyntaxFlowLexerComma           = 23
	SyntaxFlowLexerCloseParen      = 24
	SyntaxFlowLexerListSelectOpen  = 25
	SyntaxFlowLexerListSelectClose = 26
	SyntaxFlowLexerMapBuilderOpen  = 27
	SyntaxFlowLexerMapBuilderClose = 28
	SyntaxFlowLexerListStart       = 29
	SyntaxFlowLexerDollarOutput    = 30
	SyntaxFlowLexerColon           = 31
	SyntaxFlowLexerSearch          = 32
	SyntaxFlowLexerBang            = 33
	SyntaxFlowLexerWhiteSpace      = 34
	SyntaxFlowLexerNumber          = 35
	SyntaxFlowLexerOctalNumber     = 36
	SyntaxFlowLexerBinaryNumber    = 37
	SyntaxFlowLexerHexNumber       = 38
	SyntaxFlowLexerStringLiteral   = 39
	SyntaxFlowLexerStringType      = 40
	SyntaxFlowLexerListType        = 41
	SyntaxFlowLexerDictType        = 42
	SyntaxFlowLexerNumberType      = 43
	SyntaxFlowLexerBoolType        = 44
	SyntaxFlowLexerBoolLiteral     = 45
	SyntaxFlowLexerIdentifier      = 46
	SyntaxFlowLexerRegexpLiteral   = 47
)
