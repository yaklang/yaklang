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
		"", "'->'", "'-->'", "';'", "'==>'", "'...'", "'%%'", "'..'", "'<='",
		"'>='", "'>>'", "'=>'", "'=='", "'=~'", "'!~'", "'&&'", "'||'", "'!='",
		"'?{'", "'-{'", "'}->'", "'#{'", "'>'", "'.'", "'<'", "'='", "'?'",
		"'('", "','", "')'", "'['", "']'", "'{'", "'}'", "'#'", "'$'", "':'",
		"'%'", "'!'", "'*'", "'-'", "'as'", "", "", "", "", "", "", "'str'",
		"'list'", "'dict'", "", "'bool'",
	}
	staticData.symbolicNames = []string{
		"", "", "", "", "DeepFilter", "Deep", "Percent", "DeepDot", "LtEq",
		"GtEq", "DoubleGt", "Filter", "EqEq", "RegexpMatch", "NotRegexpMatch",
		"And", "Or", "NotEq", "ConditionStart", "DeepNextStart", "DeepNextEnd",
		"TopDefStart", "Gt", "Dot", "Lt", "Eq", "Question", "OpenParen", "Comma",
		"CloseParen", "ListSelectOpen", "ListSelectClose", "MapBuilderOpen",
		"MapBuilderClose", "ListStart", "DollarOutput", "Colon", "Search", "Bang",
		"Star", "Minus", "As", "WhiteSpace", "Number", "OctalNumber", "BinaryNumber",
		"HexNumber", "StringLiteral", "StringType", "ListType", "DictType",
		"NumberType", "BoolType", "BoolLiteral", "Identifier", "IdentifierChar",
		"RegexpLiteral",
	}
	staticData.ruleNames = []string{
		"T__0", "T__1", "T__2", "DeepFilter", "Deep", "Percent", "DeepDot",
		"LtEq", "GtEq", "DoubleGt", "Filter", "EqEq", "RegexpMatch", "NotRegexpMatch",
		"And", "Or", "NotEq", "ConditionStart", "DeepNextStart", "DeepNextEnd",
		"TopDefStart", "Gt", "Dot", "Lt", "Eq", "Question", "OpenParen", "Comma",
		"CloseParen", "ListSelectOpen", "ListSelectClose", "MapBuilderOpen",
		"MapBuilderClose", "ListStart", "DollarOutput", "Colon", "Search", "Bang",
		"Star", "Minus", "As", "WhiteSpace", "Number", "OctalNumber", "BinaryNumber",
		"HexNumber", "StringLiteral", "StringType", "ListType", "DictType",
		"NumberType", "BoolType", "BoolLiteral", "Identifier", "IdentifierChar",
		"IdentifierCharStart", "HexDigit", "Digit", "OctalDigit", "RegexpLiteral",
		"RegexpLiteralChar",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 0, 56, 345, 6, -1, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2,
		4, 7, 4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2,
		10, 7, 10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15,
		7, 15, 2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7,
		20, 2, 21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25,
		2, 26, 7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 2, 29, 7, 29, 2, 30, 7, 30, 2,
		31, 7, 31, 2, 32, 7, 32, 2, 33, 7, 33, 2, 34, 7, 34, 2, 35, 7, 35, 2, 36,
		7, 36, 2, 37, 7, 37, 2, 38, 7, 38, 2, 39, 7, 39, 2, 40, 7, 40, 2, 41, 7,
		41, 2, 42, 7, 42, 2, 43, 7, 43, 2, 44, 7, 44, 2, 45, 7, 45, 2, 46, 7, 46,
		2, 47, 7, 47, 2, 48, 7, 48, 2, 49, 7, 49, 2, 50, 7, 50, 2, 51, 7, 51, 2,
		52, 7, 52, 2, 53, 7, 53, 2, 54, 7, 54, 2, 55, 7, 55, 2, 56, 7, 56, 2, 57,
		7, 57, 2, 58, 7, 58, 2, 59, 7, 59, 2, 60, 7, 60, 1, 0, 1, 0, 1, 0, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 3, 1, 3, 1, 3, 1, 3, 1, 4, 1, 4, 1, 4,
		1, 4, 1, 5, 1, 5, 1, 5, 1, 6, 1, 6, 1, 6, 1, 7, 1, 7, 1, 7, 1, 8, 1, 8,
		1, 8, 1, 9, 1, 9, 1, 9, 1, 10, 1, 10, 1, 10, 1, 11, 1, 11, 1, 11, 1, 12,
		1, 12, 1, 12, 1, 13, 1, 13, 1, 13, 1, 14, 1, 14, 1, 14, 1, 15, 1, 15, 1,
		15, 1, 16, 1, 16, 1, 16, 1, 17, 1, 17, 1, 17, 1, 18, 1, 18, 1, 18, 1, 19,
		1, 19, 1, 19, 1, 19, 1, 20, 1, 20, 1, 20, 1, 21, 1, 21, 1, 22, 1, 22, 1,
		23, 1, 23, 1, 24, 1, 24, 1, 25, 1, 25, 1, 26, 1, 26, 1, 27, 1, 27, 1, 28,
		1, 28, 1, 29, 1, 29, 1, 30, 1, 30, 1, 31, 1, 31, 1, 32, 1, 32, 1, 33, 1,
		33, 1, 34, 1, 34, 1, 35, 1, 35, 1, 36, 1, 36, 1, 37, 1, 37, 1, 38, 1, 38,
		1, 39, 1, 39, 1, 40, 1, 40, 1, 40, 1, 41, 1, 41, 1, 41, 1, 41, 1, 42, 4,
		42, 236, 8, 42, 11, 42, 12, 42, 237, 1, 43, 1, 43, 1, 43, 1, 43, 4, 43,
		244, 8, 43, 11, 43, 12, 43, 245, 1, 44, 1, 44, 1, 44, 1, 44, 4, 44, 252,
		8, 44, 11, 44, 12, 44, 253, 1, 45, 1, 45, 1, 45, 1, 45, 4, 45, 260, 8,
		45, 11, 45, 12, 45, 261, 1, 46, 1, 46, 5, 46, 266, 8, 46, 10, 46, 12, 46,
		269, 9, 46, 1, 46, 1, 46, 1, 47, 1, 47, 1, 47, 1, 47, 1, 48, 1, 48, 1,
		48, 1, 48, 1, 48, 1, 49, 1, 49, 1, 49, 1, 49, 1, 49, 1, 50, 1, 50, 1, 50,
		1, 50, 1, 50, 1, 50, 1, 50, 1, 50, 3, 50, 295, 8, 50, 1, 51, 1, 51, 1,
		51, 1, 51, 1, 51, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52,
		1, 52, 3, 52, 311, 8, 52, 1, 53, 1, 53, 5, 53, 315, 8, 53, 10, 53, 12,
		53, 318, 9, 53, 1, 54, 1, 54, 3, 54, 322, 8, 54, 1, 55, 3, 55, 325, 8,
		55, 1, 56, 1, 56, 1, 57, 1, 57, 1, 58, 1, 58, 1, 59, 1, 59, 4, 59, 335,
		8, 59, 11, 59, 12, 59, 336, 1, 59, 1, 59, 1, 60, 1, 60, 1, 60, 3, 60, 344,
		8, 60, 0, 0, 61, 1, 1, 3, 2, 5, 3, 7, 4, 9, 5, 11, 6, 13, 7, 15, 8, 17,
		9, 19, 10, 21, 11, 23, 12, 25, 13, 27, 14, 29, 15, 31, 16, 33, 17, 35,
		18, 37, 19, 39, 20, 41, 21, 43, 22, 45, 23, 47, 24, 49, 25, 51, 26, 53,
		27, 55, 28, 57, 29, 59, 30, 61, 31, 63, 32, 65, 33, 67, 34, 69, 35, 71,
		36, 73, 37, 75, 38, 77, 39, 79, 40, 81, 41, 83, 42, 85, 43, 87, 44, 89,
		45, 91, 46, 93, 47, 95, 48, 97, 49, 99, 50, 101, 51, 103, 52, 105, 53,
		107, 54, 109, 55, 111, 0, 113, 0, 115, 0, 117, 0, 119, 56, 121, 0, 1, 0,
		7, 3, 0, 10, 10, 13, 13, 32, 32, 1, 0, 96, 96, 1, 0, 48, 57, 4, 0, 42,
		42, 65, 90, 95, 95, 97, 122, 3, 0, 48, 57, 65, 70, 97, 102, 1, 0, 48, 55,
		1, 0, 47, 47, 350, 0, 1, 1, 0, 0, 0, 0, 3, 1, 0, 0, 0, 0, 5, 1, 0, 0, 0,
		0, 7, 1, 0, 0, 0, 0, 9, 1, 0, 0, 0, 0, 11, 1, 0, 0, 0, 0, 13, 1, 0, 0,
		0, 0, 15, 1, 0, 0, 0, 0, 17, 1, 0, 0, 0, 0, 19, 1, 0, 0, 0, 0, 21, 1, 0,
		0, 0, 0, 23, 1, 0, 0, 0, 0, 25, 1, 0, 0, 0, 0, 27, 1, 0, 0, 0, 0, 29, 1,
		0, 0, 0, 0, 31, 1, 0, 0, 0, 0, 33, 1, 0, 0, 0, 0, 35, 1, 0, 0, 0, 0, 37,
		1, 0, 0, 0, 0, 39, 1, 0, 0, 0, 0, 41, 1, 0, 0, 0, 0, 43, 1, 0, 0, 0, 0,
		45, 1, 0, 0, 0, 0, 47, 1, 0, 0, 0, 0, 49, 1, 0, 0, 0, 0, 51, 1, 0, 0, 0,
		0, 53, 1, 0, 0, 0, 0, 55, 1, 0, 0, 0, 0, 57, 1, 0, 0, 0, 0, 59, 1, 0, 0,
		0, 0, 61, 1, 0, 0, 0, 0, 63, 1, 0, 0, 0, 0, 65, 1, 0, 0, 0, 0, 67, 1, 0,
		0, 0, 0, 69, 1, 0, 0, 0, 0, 71, 1, 0, 0, 0, 0, 73, 1, 0, 0, 0, 0, 75, 1,
		0, 0, 0, 0, 77, 1, 0, 0, 0, 0, 79, 1, 0, 0, 0, 0, 81, 1, 0, 0, 0, 0, 83,
		1, 0, 0, 0, 0, 85, 1, 0, 0, 0, 0, 87, 1, 0, 0, 0, 0, 89, 1, 0, 0, 0, 0,
		91, 1, 0, 0, 0, 0, 93, 1, 0, 0, 0, 0, 95, 1, 0, 0, 0, 0, 97, 1, 0, 0, 0,
		0, 99, 1, 0, 0, 0, 0, 101, 1, 0, 0, 0, 0, 103, 1, 0, 0, 0, 0, 105, 1, 0,
		0, 0, 0, 107, 1, 0, 0, 0, 0, 109, 1, 0, 0, 0, 0, 119, 1, 0, 0, 0, 1, 123,
		1, 0, 0, 0, 3, 126, 1, 0, 0, 0, 5, 130, 1, 0, 0, 0, 7, 132, 1, 0, 0, 0,
		9, 136, 1, 0, 0, 0, 11, 140, 1, 0, 0, 0, 13, 143, 1, 0, 0, 0, 15, 146,
		1, 0, 0, 0, 17, 149, 1, 0, 0, 0, 19, 152, 1, 0, 0, 0, 21, 155, 1, 0, 0,
		0, 23, 158, 1, 0, 0, 0, 25, 161, 1, 0, 0, 0, 27, 164, 1, 0, 0, 0, 29, 167,
		1, 0, 0, 0, 31, 170, 1, 0, 0, 0, 33, 173, 1, 0, 0, 0, 35, 176, 1, 0, 0,
		0, 37, 179, 1, 0, 0, 0, 39, 182, 1, 0, 0, 0, 41, 186, 1, 0, 0, 0, 43, 189,
		1, 0, 0, 0, 45, 191, 1, 0, 0, 0, 47, 193, 1, 0, 0, 0, 49, 195, 1, 0, 0,
		0, 51, 197, 1, 0, 0, 0, 53, 199, 1, 0, 0, 0, 55, 201, 1, 0, 0, 0, 57, 203,
		1, 0, 0, 0, 59, 205, 1, 0, 0, 0, 61, 207, 1, 0, 0, 0, 63, 209, 1, 0, 0,
		0, 65, 211, 1, 0, 0, 0, 67, 213, 1, 0, 0, 0, 69, 215, 1, 0, 0, 0, 71, 217,
		1, 0, 0, 0, 73, 219, 1, 0, 0, 0, 75, 221, 1, 0, 0, 0, 77, 223, 1, 0, 0,
		0, 79, 225, 1, 0, 0, 0, 81, 227, 1, 0, 0, 0, 83, 230, 1, 0, 0, 0, 85, 235,
		1, 0, 0, 0, 87, 239, 1, 0, 0, 0, 89, 247, 1, 0, 0, 0, 91, 255, 1, 0, 0,
		0, 93, 263, 1, 0, 0, 0, 95, 272, 1, 0, 0, 0, 97, 276, 1, 0, 0, 0, 99, 281,
		1, 0, 0, 0, 101, 294, 1, 0, 0, 0, 103, 296, 1, 0, 0, 0, 105, 310, 1, 0,
		0, 0, 107, 312, 1, 0, 0, 0, 109, 321, 1, 0, 0, 0, 111, 324, 1, 0, 0, 0,
		113, 326, 1, 0, 0, 0, 115, 328, 1, 0, 0, 0, 117, 330, 1, 0, 0, 0, 119,
		332, 1, 0, 0, 0, 121, 343, 1, 0, 0, 0, 123, 124, 5, 45, 0, 0, 124, 125,
		5, 62, 0, 0, 125, 2, 1, 0, 0, 0, 126, 127, 5, 45, 0, 0, 127, 128, 5, 45,
		0, 0, 128, 129, 5, 62, 0, 0, 129, 4, 1, 0, 0, 0, 130, 131, 5, 59, 0, 0,
		131, 6, 1, 0, 0, 0, 132, 133, 5, 61, 0, 0, 133, 134, 5, 61, 0, 0, 134,
		135, 5, 62, 0, 0, 135, 8, 1, 0, 0, 0, 136, 137, 5, 46, 0, 0, 137, 138,
		5, 46, 0, 0, 138, 139, 5, 46, 0, 0, 139, 10, 1, 0, 0, 0, 140, 141, 5, 37,
		0, 0, 141, 142, 5, 37, 0, 0, 142, 12, 1, 0, 0, 0, 143, 144, 5, 46, 0, 0,
		144, 145, 5, 46, 0, 0, 145, 14, 1, 0, 0, 0, 146, 147, 5, 60, 0, 0, 147,
		148, 5, 61, 0, 0, 148, 16, 1, 0, 0, 0, 149, 150, 5, 62, 0, 0, 150, 151,
		5, 61, 0, 0, 151, 18, 1, 0, 0, 0, 152, 153, 5, 62, 0, 0, 153, 154, 5, 62,
		0, 0, 154, 20, 1, 0, 0, 0, 155, 156, 5, 61, 0, 0, 156, 157, 5, 62, 0, 0,
		157, 22, 1, 0, 0, 0, 158, 159, 5, 61, 0, 0, 159, 160, 5, 61, 0, 0, 160,
		24, 1, 0, 0, 0, 161, 162, 5, 61, 0, 0, 162, 163, 5, 126, 0, 0, 163, 26,
		1, 0, 0, 0, 164, 165, 5, 33, 0, 0, 165, 166, 5, 126, 0, 0, 166, 28, 1,
		0, 0, 0, 167, 168, 5, 38, 0, 0, 168, 169, 5, 38, 0, 0, 169, 30, 1, 0, 0,
		0, 170, 171, 5, 124, 0, 0, 171, 172, 5, 124, 0, 0, 172, 32, 1, 0, 0, 0,
		173, 174, 5, 33, 0, 0, 174, 175, 5, 61, 0, 0, 175, 34, 1, 0, 0, 0, 176,
		177, 5, 63, 0, 0, 177, 178, 5, 123, 0, 0, 178, 36, 1, 0, 0, 0, 179, 180,
		5, 45, 0, 0, 180, 181, 5, 123, 0, 0, 181, 38, 1, 0, 0, 0, 182, 183, 5,
		125, 0, 0, 183, 184, 5, 45, 0, 0, 184, 185, 5, 62, 0, 0, 185, 40, 1, 0,
		0, 0, 186, 187, 5, 35, 0, 0, 187, 188, 5, 123, 0, 0, 188, 42, 1, 0, 0,
		0, 189, 190, 5, 62, 0, 0, 190, 44, 1, 0, 0, 0, 191, 192, 5, 46, 0, 0, 192,
		46, 1, 0, 0, 0, 193, 194, 5, 60, 0, 0, 194, 48, 1, 0, 0, 0, 195, 196, 5,
		61, 0, 0, 196, 50, 1, 0, 0, 0, 197, 198, 5, 63, 0, 0, 198, 52, 1, 0, 0,
		0, 199, 200, 5, 40, 0, 0, 200, 54, 1, 0, 0, 0, 201, 202, 5, 44, 0, 0, 202,
		56, 1, 0, 0, 0, 203, 204, 5, 41, 0, 0, 204, 58, 1, 0, 0, 0, 205, 206, 5,
		91, 0, 0, 206, 60, 1, 0, 0, 0, 207, 208, 5, 93, 0, 0, 208, 62, 1, 0, 0,
		0, 209, 210, 5, 123, 0, 0, 210, 64, 1, 0, 0, 0, 211, 212, 5, 125, 0, 0,
		212, 66, 1, 0, 0, 0, 213, 214, 5, 35, 0, 0, 214, 68, 1, 0, 0, 0, 215, 216,
		5, 36, 0, 0, 216, 70, 1, 0, 0, 0, 217, 218, 5, 58, 0, 0, 218, 72, 1, 0,
		0, 0, 219, 220, 5, 37, 0, 0, 220, 74, 1, 0, 0, 0, 221, 222, 5, 33, 0, 0,
		222, 76, 1, 0, 0, 0, 223, 224, 5, 42, 0, 0, 224, 78, 1, 0, 0, 0, 225, 226,
		5, 45, 0, 0, 226, 80, 1, 0, 0, 0, 227, 228, 5, 97, 0, 0, 228, 229, 5, 115,
		0, 0, 229, 82, 1, 0, 0, 0, 230, 231, 7, 0, 0, 0, 231, 232, 1, 0, 0, 0,
		232, 233, 6, 41, 0, 0, 233, 84, 1, 0, 0, 0, 234, 236, 3, 115, 57, 0, 235,
		234, 1, 0, 0, 0, 236, 237, 1, 0, 0, 0, 237, 235, 1, 0, 0, 0, 237, 238,
		1, 0, 0, 0, 238, 86, 1, 0, 0, 0, 239, 240, 5, 48, 0, 0, 240, 241, 5, 111,
		0, 0, 241, 243, 1, 0, 0, 0, 242, 244, 3, 117, 58, 0, 243, 242, 1, 0, 0,
		0, 244, 245, 1, 0, 0, 0, 245, 243, 1, 0, 0, 0, 245, 246, 1, 0, 0, 0, 246,
		88, 1, 0, 0, 0, 247, 248, 5, 48, 0, 0, 248, 249, 5, 98, 0, 0, 249, 251,
		1, 0, 0, 0, 250, 252, 2, 48, 49, 0, 251, 250, 1, 0, 0, 0, 252, 253, 1,
		0, 0, 0, 253, 251, 1, 0, 0, 0, 253, 254, 1, 0, 0, 0, 254, 90, 1, 0, 0,
		0, 255, 256, 5, 48, 0, 0, 256, 257, 5, 120, 0, 0, 257, 259, 1, 0, 0, 0,
		258, 260, 3, 113, 56, 0, 259, 258, 1, 0, 0, 0, 260, 261, 1, 0, 0, 0, 261,
		259, 1, 0, 0, 0, 261, 262, 1, 0, 0, 0, 262, 92, 1, 0, 0, 0, 263, 267, 5,
		96, 0, 0, 264, 266, 8, 1, 0, 0, 265, 264, 1, 0, 0, 0, 266, 269, 1, 0, 0,
		0, 267, 265, 1, 0, 0, 0, 267, 268, 1, 0, 0, 0, 268, 270, 1, 0, 0, 0, 269,
		267, 1, 0, 0, 0, 270, 271, 5, 96, 0, 0, 271, 94, 1, 0, 0, 0, 272, 273,
		5, 115, 0, 0, 273, 274, 5, 116, 0, 0, 274, 275, 5, 114, 0, 0, 275, 96,
		1, 0, 0, 0, 276, 277, 5, 108, 0, 0, 277, 278, 5, 105, 0, 0, 278, 279, 5,
		115, 0, 0, 279, 280, 5, 116, 0, 0, 280, 98, 1, 0, 0, 0, 281, 282, 5, 100,
		0, 0, 282, 283, 5, 105, 0, 0, 283, 284, 5, 99, 0, 0, 284, 285, 5, 116,
		0, 0, 285, 100, 1, 0, 0, 0, 286, 287, 5, 105, 0, 0, 287, 288, 5, 110, 0,
		0, 288, 295, 5, 116, 0, 0, 289, 290, 5, 102, 0, 0, 290, 291, 5, 108, 0,
		0, 291, 292, 5, 111, 0, 0, 292, 293, 5, 97, 0, 0, 293, 295, 5, 116, 0,
		0, 294, 286, 1, 0, 0, 0, 294, 289, 1, 0, 0, 0, 295, 102, 1, 0, 0, 0, 296,
		297, 5, 98, 0, 0, 297, 298, 5, 111, 0, 0, 298, 299, 5, 111, 0, 0, 299,
		300, 5, 108, 0, 0, 300, 104, 1, 0, 0, 0, 301, 302, 5, 116, 0, 0, 302, 303,
		5, 114, 0, 0, 303, 304, 5, 117, 0, 0, 304, 311, 5, 101, 0, 0, 305, 306,
		5, 102, 0, 0, 306, 307, 5, 97, 0, 0, 307, 308, 5, 108, 0, 0, 308, 309,
		5, 115, 0, 0, 309, 311, 5, 101, 0, 0, 310, 301, 1, 0, 0, 0, 310, 305, 1,
		0, 0, 0, 311, 106, 1, 0, 0, 0, 312, 316, 3, 111, 55, 0, 313, 315, 3, 109,
		54, 0, 314, 313, 1, 0, 0, 0, 315, 318, 1, 0, 0, 0, 316, 314, 1, 0, 0, 0,
		316, 317, 1, 0, 0, 0, 317, 108, 1, 0, 0, 0, 318, 316, 1, 0, 0, 0, 319,
		322, 7, 2, 0, 0, 320, 322, 3, 111, 55, 0, 321, 319, 1, 0, 0, 0, 321, 320,
		1, 0, 0, 0, 322, 110, 1, 0, 0, 0, 323, 325, 7, 3, 0, 0, 324, 323, 1, 0,
		0, 0, 325, 112, 1, 0, 0, 0, 326, 327, 7, 4, 0, 0, 327, 114, 1, 0, 0, 0,
		328, 329, 7, 2, 0, 0, 329, 116, 1, 0, 0, 0, 330, 331, 7, 5, 0, 0, 331,
		118, 1, 0, 0, 0, 332, 334, 5, 47, 0, 0, 333, 335, 3, 121, 60, 0, 334, 333,
		1, 0, 0, 0, 335, 336, 1, 0, 0, 0, 336, 334, 1, 0, 0, 0, 336, 337, 1, 0,
		0, 0, 337, 338, 1, 0, 0, 0, 338, 339, 5, 47, 0, 0, 339, 120, 1, 0, 0, 0,
		340, 341, 5, 92, 0, 0, 341, 344, 5, 47, 0, 0, 342, 344, 8, 6, 0, 0, 343,
		340, 1, 0, 0, 0, 343, 342, 1, 0, 0, 0, 344, 122, 1, 0, 0, 0, 13, 0, 237,
		245, 253, 261, 267, 294, 310, 316, 321, 324, 336, 343, 1, 6, 0, 0,
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
	SyntaxFlowLexerT__1            = 2
	SyntaxFlowLexerT__2            = 3
	SyntaxFlowLexerDeepFilter      = 4
	SyntaxFlowLexerDeep            = 5
	SyntaxFlowLexerPercent         = 6
	SyntaxFlowLexerDeepDot         = 7
	SyntaxFlowLexerLtEq            = 8
	SyntaxFlowLexerGtEq            = 9
	SyntaxFlowLexerDoubleGt        = 10
	SyntaxFlowLexerFilter          = 11
	SyntaxFlowLexerEqEq            = 12
	SyntaxFlowLexerRegexpMatch     = 13
	SyntaxFlowLexerNotRegexpMatch  = 14
	SyntaxFlowLexerAnd             = 15
	SyntaxFlowLexerOr              = 16
	SyntaxFlowLexerNotEq           = 17
	SyntaxFlowLexerConditionStart  = 18
	SyntaxFlowLexerDeepNextStart   = 19
	SyntaxFlowLexerDeepNextEnd     = 20
	SyntaxFlowLexerTopDefStart     = 21
	SyntaxFlowLexerGt              = 22
	SyntaxFlowLexerDot             = 23
	SyntaxFlowLexerLt              = 24
	SyntaxFlowLexerEq              = 25
	SyntaxFlowLexerQuestion        = 26
	SyntaxFlowLexerOpenParen       = 27
	SyntaxFlowLexerComma           = 28
	SyntaxFlowLexerCloseParen      = 29
	SyntaxFlowLexerListSelectOpen  = 30
	SyntaxFlowLexerListSelectClose = 31
	SyntaxFlowLexerMapBuilderOpen  = 32
	SyntaxFlowLexerMapBuilderClose = 33
	SyntaxFlowLexerListStart       = 34
	SyntaxFlowLexerDollarOutput    = 35
	SyntaxFlowLexerColon           = 36
	SyntaxFlowLexerSearch          = 37
	SyntaxFlowLexerBang            = 38
	SyntaxFlowLexerStar            = 39
	SyntaxFlowLexerMinus           = 40
	SyntaxFlowLexerAs              = 41
	SyntaxFlowLexerWhiteSpace      = 42
	SyntaxFlowLexerNumber          = 43
	SyntaxFlowLexerOctalNumber     = 44
	SyntaxFlowLexerBinaryNumber    = 45
	SyntaxFlowLexerHexNumber       = 46
	SyntaxFlowLexerStringLiteral   = 47
	SyntaxFlowLexerStringType      = 48
	SyntaxFlowLexerListType        = 49
	SyntaxFlowLexerDictType        = 50
	SyntaxFlowLexerNumberType      = 51
	SyntaxFlowLexerBoolType        = 52
	SyntaxFlowLexerBoolLiteral     = 53
	SyntaxFlowLexerIdentifier      = 54
	SyntaxFlowLexerIdentifierChar  = 55
	SyntaxFlowLexerRegexpLiteral   = 56
)
