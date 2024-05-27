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
		"", "'->'", "'-->'", "'-<'", "'>-'", "';'", "'==>'", "'...'", "'%%'",
		"'..'", "'<='", "'>='", "'>>'", "'=>'", "'=='", "'=~'", "'!~'", "'&&'",
		"'||'", "'!='", "'?{'", "'-{'", "'}->'", "'#{'", "'#>'", "'#->'", "'>'",
		"'.'", "'<'", "'='", "'?'", "'('", "','", "')'", "'['", "']'", "'{'",
		"'}'", "'#'", "'$'", "':'", "'%'", "'!'", "'*'", "'-'", "'as'", "'`'",
		"", "", "", "", "", "'str'", "'list'", "'dict'", "", "'bool'",
	}
	staticData.symbolicNames = []string{
		"", "", "", "", "", "", "DeepFilter", "Deep", "Percent", "DeepDot",
		"LtEq", "GtEq", "DoubleGt", "Filter", "EqEq", "RegexpMatch", "NotRegexpMatch",
		"And", "Or", "NotEq", "ConditionStart", "DeepNextStart", "DeepNextEnd",
		"TopDefStart", "DefStart", "TopDef", "Gt", "Dot", "Lt", "Eq", "Question",
		"OpenParen", "Comma", "CloseParen", "ListSelectOpen", "ListSelectClose",
		"MapBuilderOpen", "MapBuilderClose", "ListStart", "DollarOutput", "Colon",
		"Search", "Bang", "Star", "Minus", "As", "Backtick", "WhiteSpace", "Number",
		"OctalNumber", "BinaryNumber", "HexNumber", "StringType", "ListType",
		"DictType", "NumberType", "BoolType", "BoolLiteral", "Identifier", "IdentifierChar",
		"RegexpLiteral", "WS",
	}
	staticData.ruleNames = []string{
		"T__0", "T__1", "T__2", "T__3", "T__4", "DeepFilter", "Deep", "Percent",
		"DeepDot", "LtEq", "GtEq", "DoubleGt", "Filter", "EqEq", "RegexpMatch",
		"NotRegexpMatch", "And", "Or", "NotEq", "ConditionStart", "DeepNextStart",
		"DeepNextEnd", "TopDefStart", "DefStart", "TopDef", "Gt", "Dot", "Lt",
		"Eq", "Question", "OpenParen", "Comma", "CloseParen", "ListSelectOpen",
		"ListSelectClose", "MapBuilderOpen", "MapBuilderClose", "ListStart",
		"DollarOutput", "Colon", "Search", "Bang", "Star", "Minus", "As", "Backtick",
		"WhiteSpace", "Number", "OctalNumber", "BinaryNumber", "HexNumber",
		"StringType", "ListType", "DictType", "NumberType", "BoolType", "BoolLiteral",
		"Identifier", "IdentifierChar", "IdentifierCharStart", "HexDigit", "Digit",
		"OctalDigit", "RegexpLiteral", "RegexpLiteralChar", "WS",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 0, 61, 368, 6, -1, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2,
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
		7, 57, 2, 58, 7, 58, 2, 59, 7, 59, 2, 60, 7, 60, 2, 61, 7, 61, 2, 62, 7,
		62, 2, 63, 7, 63, 2, 64, 7, 64, 2, 65, 7, 65, 1, 0, 1, 0, 1, 0, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 2, 1, 3, 1, 3, 1, 3, 1, 4, 1, 4, 1, 5, 1,
		5, 1, 5, 1, 5, 1, 6, 1, 6, 1, 6, 1, 6, 1, 7, 1, 7, 1, 7, 1, 8, 1, 8, 1,
		8, 1, 9, 1, 9, 1, 9, 1, 10, 1, 10, 1, 10, 1, 11, 1, 11, 1, 11, 1, 12, 1,
		12, 1, 12, 1, 13, 1, 13, 1, 13, 1, 14, 1, 14, 1, 14, 1, 15, 1, 15, 1, 15,
		1, 16, 1, 16, 1, 16, 1, 17, 1, 17, 1, 17, 1, 18, 1, 18, 1, 18, 1, 19, 1,
		19, 1, 19, 1, 20, 1, 20, 1, 20, 1, 21, 1, 21, 1, 21, 1, 21, 1, 22, 1, 22,
		1, 22, 1, 23, 1, 23, 1, 23, 1, 24, 1, 24, 1, 24, 1, 24, 1, 25, 1, 25, 1,
		26, 1, 26, 1, 27, 1, 27, 1, 28, 1, 28, 1, 29, 1, 29, 1, 30, 1, 30, 1, 31,
		1, 31, 1, 32, 1, 32, 1, 33, 1, 33, 1, 34, 1, 34, 1, 35, 1, 35, 1, 36, 1,
		36, 1, 37, 1, 37, 1, 38, 1, 38, 1, 39, 1, 39, 1, 40, 1, 40, 1, 41, 1, 41,
		1, 42, 1, 42, 1, 43, 1, 43, 1, 44, 1, 44, 1, 44, 1, 45, 1, 45, 1, 46, 1,
		46, 1, 46, 1, 46, 1, 47, 4, 47, 261, 8, 47, 11, 47, 12, 47, 262, 1, 48,
		1, 48, 1, 48, 1, 48, 4, 48, 269, 8, 48, 11, 48, 12, 48, 270, 1, 49, 1,
		49, 1, 49, 1, 49, 4, 49, 277, 8, 49, 11, 49, 12, 49, 278, 1, 50, 1, 50,
		1, 50, 1, 50, 4, 50, 285, 8, 50, 11, 50, 12, 50, 286, 1, 51, 1, 51, 1,
		51, 1, 51, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 53, 1, 53, 1, 53, 1, 53,
		1, 53, 1, 54, 1, 54, 1, 54, 1, 54, 1, 54, 1, 54, 1, 54, 1, 54, 3, 54, 311,
		8, 54, 1, 55, 1, 55, 1, 55, 1, 55, 1, 55, 1, 56, 1, 56, 1, 56, 1, 56, 1,
		56, 1, 56, 1, 56, 1, 56, 1, 56, 3, 56, 327, 8, 56, 1, 57, 1, 57, 5, 57,
		331, 8, 57, 10, 57, 12, 57, 334, 9, 57, 1, 58, 1, 58, 3, 58, 338, 8, 58,
		1, 59, 3, 59, 341, 8, 59, 1, 60, 1, 60, 1, 61, 1, 61, 1, 62, 1, 62, 1,
		63, 1, 63, 4, 63, 351, 8, 63, 11, 63, 12, 63, 352, 1, 63, 1, 63, 1, 64,
		1, 64, 1, 64, 3, 64, 360, 8, 64, 1, 65, 4, 65, 363, 8, 65, 11, 65, 12,
		65, 364, 1, 65, 1, 65, 0, 0, 66, 1, 1, 3, 2, 5, 3, 7, 4, 9, 5, 11, 6, 13,
		7, 15, 8, 17, 9, 19, 10, 21, 11, 23, 12, 25, 13, 27, 14, 29, 15, 31, 16,
		33, 17, 35, 18, 37, 19, 39, 20, 41, 21, 43, 22, 45, 23, 47, 24, 49, 25,
		51, 26, 53, 27, 55, 28, 57, 29, 59, 30, 61, 31, 63, 32, 65, 33, 67, 34,
		69, 35, 71, 36, 73, 37, 75, 38, 77, 39, 79, 40, 81, 41, 83, 42, 85, 43,
		87, 44, 89, 45, 91, 46, 93, 47, 95, 48, 97, 49, 99, 50, 101, 51, 103, 52,
		105, 53, 107, 54, 109, 55, 111, 56, 113, 57, 115, 58, 117, 59, 119, 0,
		121, 0, 123, 0, 125, 0, 127, 60, 129, 0, 131, 61, 1, 0, 7, 3, 0, 10, 10,
		13, 13, 32, 32, 1, 0, 48, 57, 4, 0, 42, 42, 65, 90, 95, 95, 97, 122, 3,
		0, 48, 57, 65, 70, 97, 102, 1, 0, 48, 55, 1, 0, 47, 47, 3, 0, 9, 9, 13,
		13, 32, 32, 373, 0, 1, 1, 0, 0, 0, 0, 3, 1, 0, 0, 0, 0, 5, 1, 0, 0, 0,
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
		0, 0, 0, 107, 1, 0, 0, 0, 0, 109, 1, 0, 0, 0, 0, 111, 1, 0, 0, 0, 0, 113,
		1, 0, 0, 0, 0, 115, 1, 0, 0, 0, 0, 117, 1, 0, 0, 0, 0, 127, 1, 0, 0, 0,
		0, 131, 1, 0, 0, 0, 1, 133, 1, 0, 0, 0, 3, 136, 1, 0, 0, 0, 5, 140, 1,
		0, 0, 0, 7, 143, 1, 0, 0, 0, 9, 146, 1, 0, 0, 0, 11, 148, 1, 0, 0, 0, 13,
		152, 1, 0, 0, 0, 15, 156, 1, 0, 0, 0, 17, 159, 1, 0, 0, 0, 19, 162, 1,
		0, 0, 0, 21, 165, 1, 0, 0, 0, 23, 168, 1, 0, 0, 0, 25, 171, 1, 0, 0, 0,
		27, 174, 1, 0, 0, 0, 29, 177, 1, 0, 0, 0, 31, 180, 1, 0, 0, 0, 33, 183,
		1, 0, 0, 0, 35, 186, 1, 0, 0, 0, 37, 189, 1, 0, 0, 0, 39, 192, 1, 0, 0,
		0, 41, 195, 1, 0, 0, 0, 43, 198, 1, 0, 0, 0, 45, 202, 1, 0, 0, 0, 47, 205,
		1, 0, 0, 0, 49, 208, 1, 0, 0, 0, 51, 212, 1, 0, 0, 0, 53, 214, 1, 0, 0,
		0, 55, 216, 1, 0, 0, 0, 57, 218, 1, 0, 0, 0, 59, 220, 1, 0, 0, 0, 61, 222,
		1, 0, 0, 0, 63, 224, 1, 0, 0, 0, 65, 226, 1, 0, 0, 0, 67, 228, 1, 0, 0,
		0, 69, 230, 1, 0, 0, 0, 71, 232, 1, 0, 0, 0, 73, 234, 1, 0, 0, 0, 75, 236,
		1, 0, 0, 0, 77, 238, 1, 0, 0, 0, 79, 240, 1, 0, 0, 0, 81, 242, 1, 0, 0,
		0, 83, 244, 1, 0, 0, 0, 85, 246, 1, 0, 0, 0, 87, 248, 1, 0, 0, 0, 89, 250,
		1, 0, 0, 0, 91, 253, 1, 0, 0, 0, 93, 255, 1, 0, 0, 0, 95, 260, 1, 0, 0,
		0, 97, 264, 1, 0, 0, 0, 99, 272, 1, 0, 0, 0, 101, 280, 1, 0, 0, 0, 103,
		288, 1, 0, 0, 0, 105, 292, 1, 0, 0, 0, 107, 297, 1, 0, 0, 0, 109, 310,
		1, 0, 0, 0, 111, 312, 1, 0, 0, 0, 113, 326, 1, 0, 0, 0, 115, 328, 1, 0,
		0, 0, 117, 337, 1, 0, 0, 0, 119, 340, 1, 0, 0, 0, 121, 342, 1, 0, 0, 0,
		123, 344, 1, 0, 0, 0, 125, 346, 1, 0, 0, 0, 127, 348, 1, 0, 0, 0, 129,
		359, 1, 0, 0, 0, 131, 362, 1, 0, 0, 0, 133, 134, 5, 45, 0, 0, 134, 135,
		5, 62, 0, 0, 135, 2, 1, 0, 0, 0, 136, 137, 5, 45, 0, 0, 137, 138, 5, 45,
		0, 0, 138, 139, 5, 62, 0, 0, 139, 4, 1, 0, 0, 0, 140, 141, 5, 45, 0, 0,
		141, 142, 5, 60, 0, 0, 142, 6, 1, 0, 0, 0, 143, 144, 5, 62, 0, 0, 144,
		145, 5, 45, 0, 0, 145, 8, 1, 0, 0, 0, 146, 147, 5, 59, 0, 0, 147, 10, 1,
		0, 0, 0, 148, 149, 5, 61, 0, 0, 149, 150, 5, 61, 0, 0, 150, 151, 5, 62,
		0, 0, 151, 12, 1, 0, 0, 0, 152, 153, 5, 46, 0, 0, 153, 154, 5, 46, 0, 0,
		154, 155, 5, 46, 0, 0, 155, 14, 1, 0, 0, 0, 156, 157, 5, 37, 0, 0, 157,
		158, 5, 37, 0, 0, 158, 16, 1, 0, 0, 0, 159, 160, 5, 46, 0, 0, 160, 161,
		5, 46, 0, 0, 161, 18, 1, 0, 0, 0, 162, 163, 5, 60, 0, 0, 163, 164, 5, 61,
		0, 0, 164, 20, 1, 0, 0, 0, 165, 166, 5, 62, 0, 0, 166, 167, 5, 61, 0, 0,
		167, 22, 1, 0, 0, 0, 168, 169, 5, 62, 0, 0, 169, 170, 5, 62, 0, 0, 170,
		24, 1, 0, 0, 0, 171, 172, 5, 61, 0, 0, 172, 173, 5, 62, 0, 0, 173, 26,
		1, 0, 0, 0, 174, 175, 5, 61, 0, 0, 175, 176, 5, 61, 0, 0, 176, 28, 1, 0,
		0, 0, 177, 178, 5, 61, 0, 0, 178, 179, 5, 126, 0, 0, 179, 30, 1, 0, 0,
		0, 180, 181, 5, 33, 0, 0, 181, 182, 5, 126, 0, 0, 182, 32, 1, 0, 0, 0,
		183, 184, 5, 38, 0, 0, 184, 185, 5, 38, 0, 0, 185, 34, 1, 0, 0, 0, 186,
		187, 5, 124, 0, 0, 187, 188, 5, 124, 0, 0, 188, 36, 1, 0, 0, 0, 189, 190,
		5, 33, 0, 0, 190, 191, 5, 61, 0, 0, 191, 38, 1, 0, 0, 0, 192, 193, 5, 63,
		0, 0, 193, 194, 5, 123, 0, 0, 194, 40, 1, 0, 0, 0, 195, 196, 5, 45, 0,
		0, 196, 197, 5, 123, 0, 0, 197, 42, 1, 0, 0, 0, 198, 199, 5, 125, 0, 0,
		199, 200, 5, 45, 0, 0, 200, 201, 5, 62, 0, 0, 201, 44, 1, 0, 0, 0, 202,
		203, 5, 35, 0, 0, 203, 204, 5, 123, 0, 0, 204, 46, 1, 0, 0, 0, 205, 206,
		5, 35, 0, 0, 206, 207, 5, 62, 0, 0, 207, 48, 1, 0, 0, 0, 208, 209, 5, 35,
		0, 0, 209, 210, 5, 45, 0, 0, 210, 211, 5, 62, 0, 0, 211, 50, 1, 0, 0, 0,
		212, 213, 5, 62, 0, 0, 213, 52, 1, 0, 0, 0, 214, 215, 5, 46, 0, 0, 215,
		54, 1, 0, 0, 0, 216, 217, 5, 60, 0, 0, 217, 56, 1, 0, 0, 0, 218, 219, 5,
		61, 0, 0, 219, 58, 1, 0, 0, 0, 220, 221, 5, 63, 0, 0, 221, 60, 1, 0, 0,
		0, 222, 223, 5, 40, 0, 0, 223, 62, 1, 0, 0, 0, 224, 225, 5, 44, 0, 0, 225,
		64, 1, 0, 0, 0, 226, 227, 5, 41, 0, 0, 227, 66, 1, 0, 0, 0, 228, 229, 5,
		91, 0, 0, 229, 68, 1, 0, 0, 0, 230, 231, 5, 93, 0, 0, 231, 70, 1, 0, 0,
		0, 232, 233, 5, 123, 0, 0, 233, 72, 1, 0, 0, 0, 234, 235, 5, 125, 0, 0,
		235, 74, 1, 0, 0, 0, 236, 237, 5, 35, 0, 0, 237, 76, 1, 0, 0, 0, 238, 239,
		5, 36, 0, 0, 239, 78, 1, 0, 0, 0, 240, 241, 5, 58, 0, 0, 241, 80, 1, 0,
		0, 0, 242, 243, 5, 37, 0, 0, 243, 82, 1, 0, 0, 0, 244, 245, 5, 33, 0, 0,
		245, 84, 1, 0, 0, 0, 246, 247, 5, 42, 0, 0, 247, 86, 1, 0, 0, 0, 248, 249,
		5, 45, 0, 0, 249, 88, 1, 0, 0, 0, 250, 251, 5, 97, 0, 0, 251, 252, 5, 115,
		0, 0, 252, 90, 1, 0, 0, 0, 253, 254, 5, 96, 0, 0, 254, 92, 1, 0, 0, 0,
		255, 256, 7, 0, 0, 0, 256, 257, 1, 0, 0, 0, 257, 258, 6, 46, 0, 0, 258,
		94, 1, 0, 0, 0, 259, 261, 3, 123, 61, 0, 260, 259, 1, 0, 0, 0, 261, 262,
		1, 0, 0, 0, 262, 260, 1, 0, 0, 0, 262, 263, 1, 0, 0, 0, 263, 96, 1, 0,
		0, 0, 264, 265, 5, 48, 0, 0, 265, 266, 5, 111, 0, 0, 266, 268, 1, 0, 0,
		0, 267, 269, 3, 125, 62, 0, 268, 267, 1, 0, 0, 0, 269, 270, 1, 0, 0, 0,
		270, 268, 1, 0, 0, 0, 270, 271, 1, 0, 0, 0, 271, 98, 1, 0, 0, 0, 272, 273,
		5, 48, 0, 0, 273, 274, 5, 98, 0, 0, 274, 276, 1, 0, 0, 0, 275, 277, 2,
		48, 49, 0, 276, 275, 1, 0, 0, 0, 277, 278, 1, 0, 0, 0, 278, 276, 1, 0,
		0, 0, 278, 279, 1, 0, 0, 0, 279, 100, 1, 0, 0, 0, 280, 281, 5, 48, 0, 0,
		281, 282, 5, 120, 0, 0, 282, 284, 1, 0, 0, 0, 283, 285, 3, 121, 60, 0,
		284, 283, 1, 0, 0, 0, 285, 286, 1, 0, 0, 0, 286, 284, 1, 0, 0, 0, 286,
		287, 1, 0, 0, 0, 287, 102, 1, 0, 0, 0, 288, 289, 5, 115, 0, 0, 289, 290,
		5, 116, 0, 0, 290, 291, 5, 114, 0, 0, 291, 104, 1, 0, 0, 0, 292, 293, 5,
		108, 0, 0, 293, 294, 5, 105, 0, 0, 294, 295, 5, 115, 0, 0, 295, 296, 5,
		116, 0, 0, 296, 106, 1, 0, 0, 0, 297, 298, 5, 100, 0, 0, 298, 299, 5, 105,
		0, 0, 299, 300, 5, 99, 0, 0, 300, 301, 5, 116, 0, 0, 301, 108, 1, 0, 0,
		0, 302, 303, 5, 105, 0, 0, 303, 304, 5, 110, 0, 0, 304, 311, 5, 116, 0,
		0, 305, 306, 5, 102, 0, 0, 306, 307, 5, 108, 0, 0, 307, 308, 5, 111, 0,
		0, 308, 309, 5, 97, 0, 0, 309, 311, 5, 116, 0, 0, 310, 302, 1, 0, 0, 0,
		310, 305, 1, 0, 0, 0, 311, 110, 1, 0, 0, 0, 312, 313, 5, 98, 0, 0, 313,
		314, 5, 111, 0, 0, 314, 315, 5, 111, 0, 0, 315, 316, 5, 108, 0, 0, 316,
		112, 1, 0, 0, 0, 317, 318, 5, 116, 0, 0, 318, 319, 5, 114, 0, 0, 319, 320,
		5, 117, 0, 0, 320, 327, 5, 101, 0, 0, 321, 322, 5, 102, 0, 0, 322, 323,
		5, 97, 0, 0, 323, 324, 5, 108, 0, 0, 324, 325, 5, 115, 0, 0, 325, 327,
		5, 101, 0, 0, 326, 317, 1, 0, 0, 0, 326, 321, 1, 0, 0, 0, 327, 114, 1,
		0, 0, 0, 328, 332, 3, 119, 59, 0, 329, 331, 3, 117, 58, 0, 330, 329, 1,
		0, 0, 0, 331, 334, 1, 0, 0, 0, 332, 330, 1, 0, 0, 0, 332, 333, 1, 0, 0,
		0, 333, 116, 1, 0, 0, 0, 334, 332, 1, 0, 0, 0, 335, 338, 7, 1, 0, 0, 336,
		338, 3, 119, 59, 0, 337, 335, 1, 0, 0, 0, 337, 336, 1, 0, 0, 0, 338, 118,
		1, 0, 0, 0, 339, 341, 7, 2, 0, 0, 340, 339, 1, 0, 0, 0, 341, 120, 1, 0,
		0, 0, 342, 343, 7, 3, 0, 0, 343, 122, 1, 0, 0, 0, 344, 345, 7, 1, 0, 0,
		345, 124, 1, 0, 0, 0, 346, 347, 7, 4, 0, 0, 347, 126, 1, 0, 0, 0, 348,
		350, 5, 47, 0, 0, 349, 351, 3, 129, 64, 0, 350, 349, 1, 0, 0, 0, 351, 352,
		1, 0, 0, 0, 352, 350, 1, 0, 0, 0, 352, 353, 1, 0, 0, 0, 353, 354, 1, 0,
		0, 0, 354, 355, 5, 47, 0, 0, 355, 128, 1, 0, 0, 0, 356, 357, 5, 92, 0,
		0, 357, 360, 5, 47, 0, 0, 358, 360, 8, 5, 0, 0, 359, 356, 1, 0, 0, 0, 359,
		358, 1, 0, 0, 0, 360, 130, 1, 0, 0, 0, 361, 363, 7, 6, 0, 0, 362, 361,
		1, 0, 0, 0, 363, 364, 1, 0, 0, 0, 364, 362, 1, 0, 0, 0, 364, 365, 1, 0,
		0, 0, 365, 366, 1, 0, 0, 0, 366, 367, 6, 65, 0, 0, 367, 132, 1, 0, 0, 0,
		13, 0, 262, 270, 278, 286, 310, 326, 332, 337, 340, 352, 359, 364, 1, 6,
		0, 0,
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
	SyntaxFlowLexerT__3            = 4
	SyntaxFlowLexerT__4            = 5
	SyntaxFlowLexerDeepFilter      = 6
	SyntaxFlowLexerDeep            = 7
	SyntaxFlowLexerPercent         = 8
	SyntaxFlowLexerDeepDot         = 9
	SyntaxFlowLexerLtEq            = 10
	SyntaxFlowLexerGtEq            = 11
	SyntaxFlowLexerDoubleGt        = 12
	SyntaxFlowLexerFilter          = 13
	SyntaxFlowLexerEqEq            = 14
	SyntaxFlowLexerRegexpMatch     = 15
	SyntaxFlowLexerNotRegexpMatch  = 16
	SyntaxFlowLexerAnd             = 17
	SyntaxFlowLexerOr              = 18
	SyntaxFlowLexerNotEq           = 19
	SyntaxFlowLexerConditionStart  = 20
	SyntaxFlowLexerDeepNextStart   = 21
	SyntaxFlowLexerDeepNextEnd     = 22
	SyntaxFlowLexerTopDefStart     = 23
	SyntaxFlowLexerDefStart        = 24
	SyntaxFlowLexerTopDef          = 25
	SyntaxFlowLexerGt              = 26
	SyntaxFlowLexerDot             = 27
	SyntaxFlowLexerLt              = 28
	SyntaxFlowLexerEq              = 29
	SyntaxFlowLexerQuestion        = 30
	SyntaxFlowLexerOpenParen       = 31
	SyntaxFlowLexerComma           = 32
	SyntaxFlowLexerCloseParen      = 33
	SyntaxFlowLexerListSelectOpen  = 34
	SyntaxFlowLexerListSelectClose = 35
	SyntaxFlowLexerMapBuilderOpen  = 36
	SyntaxFlowLexerMapBuilderClose = 37
	SyntaxFlowLexerListStart       = 38
	SyntaxFlowLexerDollarOutput    = 39
	SyntaxFlowLexerColon           = 40
	SyntaxFlowLexerSearch          = 41
	SyntaxFlowLexerBang            = 42
	SyntaxFlowLexerStar            = 43
	SyntaxFlowLexerMinus           = 44
	SyntaxFlowLexerAs              = 45
	SyntaxFlowLexerBacktick        = 46
	SyntaxFlowLexerWhiteSpace      = 47
	SyntaxFlowLexerNumber          = 48
	SyntaxFlowLexerOctalNumber     = 49
	SyntaxFlowLexerBinaryNumber    = 50
	SyntaxFlowLexerHexNumber       = 51
	SyntaxFlowLexerStringType      = 52
	SyntaxFlowLexerListType        = 53
	SyntaxFlowLexerDictType        = 54
	SyntaxFlowLexerNumberType      = 55
	SyntaxFlowLexerBoolType        = 56
	SyntaxFlowLexerBoolLiteral     = 57
	SyntaxFlowLexerIdentifier      = 58
	SyntaxFlowLexerIdentifierChar  = 59
	SyntaxFlowLexerRegexpLiteral   = 60
	SyntaxFlowLexerWS              = 61
)
