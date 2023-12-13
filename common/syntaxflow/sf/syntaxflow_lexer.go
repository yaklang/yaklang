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
		"'>>'", "'=>'", "'=='", "'~='", "'&&'", "'||'", "'>'", "'.'", "'<'",
		"'='", "'('", "','", "')'", "'['", "']'", "'{'", "'}'", "'#'", "'$'",
		"':'", "'%'", "", "", "", "", "", "", "'str'", "'list'", "'dict'",
	}
	staticData.symbolicNames = []string{
		"", "", "DeepFilter", "Deep", "Percent", "DeepDot", "LtEq", "GtEq",
		"DoubleLt", "DoubleGt", "Filter", "EqEq", "RegexpMatch", "And", "Or",
		"Gt", "Dot", "Lt", "Eq", "OpenParen", "Comma", "CloseParen", "ListSelectOpen",
		"ListSelectClose", "MapBuilderOpen", "MapBuilderClose", "ListStart",
		"DollarOutput", "Colon", "Search", "WhiteSpace", "Number", "OctalNumber",
		"BinaryNumber", "HexNumber", "StringLiteral", "StringType", "ListType",
		"DictType", "NumberType", "BoolType", "Identifier",
	}
	staticData.ruleNames = []string{
		"T__0", "DeepFilter", "Deep", "Percent", "DeepDot", "LtEq", "GtEq",
		"DoubleLt", "DoubleGt", "Filter", "EqEq", "RegexpMatch", "And", "Or",
		"Gt", "Dot", "Lt", "Eq", "OpenParen", "Comma", "CloseParen", "ListSelectOpen",
		"ListSelectClose", "MapBuilderOpen", "MapBuilderClose", "ListStart",
		"DollarOutput", "Colon", "Search", "WhiteSpace", "Number", "OctalNumber",
		"BinaryNumber", "HexNumber", "StringLiteral", "StringType", "ListType",
		"DictType", "NumberType", "BoolType", "Identifier", "IdentifierCharStart",
		"IdentifierChar", "HexDigit", "Digit", "OctalDigit",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 0, 41, 265, 6, -1, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2,
		4, 7, 4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2,
		10, 7, 10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15,
		7, 15, 2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7,
		20, 2, 21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25,
		2, 26, 7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 2, 29, 7, 29, 2, 30, 7, 30, 2,
		31, 7, 31, 2, 32, 7, 32, 2, 33, 7, 33, 2, 34, 7, 34, 2, 35, 7, 35, 2, 36,
		7, 36, 2, 37, 7, 37, 2, 38, 7, 38, 2, 39, 7, 39, 2, 40, 7, 40, 2, 41, 7,
		41, 2, 42, 7, 42, 2, 43, 7, 43, 2, 44, 7, 44, 2, 45, 7, 45, 1, 0, 1, 0,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 2, 1, 2, 1, 3, 1, 3, 1, 3, 1, 4,
		1, 4, 1, 4, 1, 5, 1, 5, 1, 5, 1, 6, 1, 6, 1, 6, 1, 7, 1, 7, 1, 7, 1, 8,
		1, 8, 1, 8, 1, 9, 1, 9, 1, 9, 1, 10, 1, 10, 1, 10, 1, 11, 1, 11, 1, 11,
		1, 12, 1, 12, 1, 12, 1, 13, 1, 13, 1, 13, 1, 14, 1, 14, 1, 15, 1, 15, 1,
		16, 1, 16, 1, 17, 1, 17, 1, 18, 1, 18, 1, 19, 1, 19, 1, 20, 1, 20, 1, 21,
		1, 21, 1, 22, 1, 22, 1, 23, 1, 23, 1, 24, 1, 24, 1, 25, 1, 25, 1, 26, 1,
		26, 1, 27, 1, 27, 1, 28, 1, 28, 1, 29, 1, 29, 1, 29, 1, 29, 1, 30, 4, 30,
		172, 8, 30, 11, 30, 12, 30, 173, 1, 31, 1, 31, 1, 31, 1, 31, 4, 31, 180,
		8, 31, 11, 31, 12, 31, 181, 1, 32, 1, 32, 1, 32, 1, 32, 4, 32, 188, 8,
		32, 11, 32, 12, 32, 189, 1, 33, 1, 33, 1, 33, 1, 33, 4, 33, 196, 8, 33,
		11, 33, 12, 33, 197, 1, 34, 1, 34, 5, 34, 202, 8, 34, 10, 34, 12, 34, 205,
		9, 34, 1, 34, 1, 34, 1, 35, 1, 35, 1, 35, 1, 35, 1, 36, 1, 36, 1, 36, 1,
		36, 1, 36, 1, 37, 1, 37, 1, 37, 1, 37, 1, 37, 1, 38, 1, 38, 1, 38, 1, 38,
		1, 38, 1, 38, 1, 38, 1, 38, 3, 38, 231, 8, 38, 1, 39, 1, 39, 1, 39, 1,
		39, 1, 39, 1, 39, 1, 39, 1, 39, 1, 39, 3, 39, 242, 8, 39, 1, 40, 1, 40,
		5, 40, 246, 8, 40, 10, 40, 12, 40, 249, 9, 40, 1, 41, 1, 41, 1, 41, 3,
		41, 254, 8, 41, 1, 42, 1, 42, 3, 42, 258, 8, 42, 1, 43, 1, 43, 1, 44, 1,
		44, 1, 45, 1, 45, 0, 0, 46, 1, 1, 3, 2, 5, 3, 7, 4, 9, 5, 11, 6, 13, 7,
		15, 8, 17, 9, 19, 10, 21, 11, 23, 12, 25, 13, 27, 14, 29, 15, 31, 16, 33,
		17, 35, 18, 37, 19, 39, 20, 41, 21, 43, 22, 45, 23, 47, 24, 49, 25, 51,
		26, 53, 27, 55, 28, 57, 29, 59, 30, 61, 31, 63, 32, 65, 33, 67, 34, 69,
		35, 71, 36, 73, 37, 75, 38, 77, 39, 79, 40, 81, 41, 83, 0, 85, 0, 87, 0,
		89, 0, 91, 0, 1, 0, 6, 3, 0, 10, 10, 13, 13, 32, 32, 1, 0, 96, 96, 4, 0,
		37, 37, 65, 90, 95, 95, 97, 122, 1, 0, 48, 57, 3, 0, 48, 57, 65, 70, 97,
		102, 1, 0, 48, 55, 269, 0, 1, 1, 0, 0, 0, 0, 3, 1, 0, 0, 0, 0, 5, 1, 0,
		0, 0, 0, 7, 1, 0, 0, 0, 0, 9, 1, 0, 0, 0, 0, 11, 1, 0, 0, 0, 0, 13, 1,
		0, 0, 0, 0, 15, 1, 0, 0, 0, 0, 17, 1, 0, 0, 0, 0, 19, 1, 0, 0, 0, 0, 21,
		1, 0, 0, 0, 0, 23, 1, 0, 0, 0, 0, 25, 1, 0, 0, 0, 0, 27, 1, 0, 0, 0, 0,
		29, 1, 0, 0, 0, 0, 31, 1, 0, 0, 0, 0, 33, 1, 0, 0, 0, 0, 35, 1, 0, 0, 0,
		0, 37, 1, 0, 0, 0, 0, 39, 1, 0, 0, 0, 0, 41, 1, 0, 0, 0, 0, 43, 1, 0, 0,
		0, 0, 45, 1, 0, 0, 0, 0, 47, 1, 0, 0, 0, 0, 49, 1, 0, 0, 0, 0, 51, 1, 0,
		0, 0, 0, 53, 1, 0, 0, 0, 0, 55, 1, 0, 0, 0, 0, 57, 1, 0, 0, 0, 0, 59, 1,
		0, 0, 0, 0, 61, 1, 0, 0, 0, 0, 63, 1, 0, 0, 0, 0, 65, 1, 0, 0, 0, 0, 67,
		1, 0, 0, 0, 0, 69, 1, 0, 0, 0, 0, 71, 1, 0, 0, 0, 0, 73, 1, 0, 0, 0, 0,
		75, 1, 0, 0, 0, 0, 77, 1, 0, 0, 0, 0, 79, 1, 0, 0, 0, 0, 81, 1, 0, 0, 0,
		1, 93, 1, 0, 0, 0, 3, 95, 1, 0, 0, 0, 5, 99, 1, 0, 0, 0, 7, 103, 1, 0,
		0, 0, 9, 106, 1, 0, 0, 0, 11, 109, 1, 0, 0, 0, 13, 112, 1, 0, 0, 0, 15,
		115, 1, 0, 0, 0, 17, 118, 1, 0, 0, 0, 19, 121, 1, 0, 0, 0, 21, 124, 1,
		0, 0, 0, 23, 127, 1, 0, 0, 0, 25, 130, 1, 0, 0, 0, 27, 133, 1, 0, 0, 0,
		29, 136, 1, 0, 0, 0, 31, 138, 1, 0, 0, 0, 33, 140, 1, 0, 0, 0, 35, 142,
		1, 0, 0, 0, 37, 144, 1, 0, 0, 0, 39, 146, 1, 0, 0, 0, 41, 148, 1, 0, 0,
		0, 43, 150, 1, 0, 0, 0, 45, 152, 1, 0, 0, 0, 47, 154, 1, 0, 0, 0, 49, 156,
		1, 0, 0, 0, 51, 158, 1, 0, 0, 0, 53, 160, 1, 0, 0, 0, 55, 162, 1, 0, 0,
		0, 57, 164, 1, 0, 0, 0, 59, 166, 1, 0, 0, 0, 61, 171, 1, 0, 0, 0, 63, 175,
		1, 0, 0, 0, 65, 183, 1, 0, 0, 0, 67, 191, 1, 0, 0, 0, 69, 199, 1, 0, 0,
		0, 71, 208, 1, 0, 0, 0, 73, 212, 1, 0, 0, 0, 75, 217, 1, 0, 0, 0, 77, 230,
		1, 0, 0, 0, 79, 241, 1, 0, 0, 0, 81, 243, 1, 0, 0, 0, 83, 253, 1, 0, 0,
		0, 85, 257, 1, 0, 0, 0, 87, 259, 1, 0, 0, 0, 89, 261, 1, 0, 0, 0, 91, 263,
		1, 0, 0, 0, 93, 94, 5, 59, 0, 0, 94, 2, 1, 0, 0, 0, 95, 96, 5, 61, 0, 0,
		96, 97, 5, 61, 0, 0, 97, 98, 5, 62, 0, 0, 98, 4, 1, 0, 0, 0, 99, 100, 5,
		46, 0, 0, 100, 101, 5, 46, 0, 0, 101, 102, 5, 46, 0, 0, 102, 6, 1, 0, 0,
		0, 103, 104, 5, 37, 0, 0, 104, 105, 5, 37, 0, 0, 105, 8, 1, 0, 0, 0, 106,
		107, 5, 46, 0, 0, 107, 108, 5, 46, 0, 0, 108, 10, 1, 0, 0, 0, 109, 110,
		5, 60, 0, 0, 110, 111, 5, 61, 0, 0, 111, 12, 1, 0, 0, 0, 112, 113, 5, 62,
		0, 0, 113, 114, 5, 61, 0, 0, 114, 14, 1, 0, 0, 0, 115, 116, 5, 60, 0, 0,
		116, 117, 5, 60, 0, 0, 117, 16, 1, 0, 0, 0, 118, 119, 5, 62, 0, 0, 119,
		120, 5, 62, 0, 0, 120, 18, 1, 0, 0, 0, 121, 122, 5, 61, 0, 0, 122, 123,
		5, 62, 0, 0, 123, 20, 1, 0, 0, 0, 124, 125, 5, 61, 0, 0, 125, 126, 5, 61,
		0, 0, 126, 22, 1, 0, 0, 0, 127, 128, 5, 126, 0, 0, 128, 129, 5, 61, 0,
		0, 129, 24, 1, 0, 0, 0, 130, 131, 5, 38, 0, 0, 131, 132, 5, 38, 0, 0, 132,
		26, 1, 0, 0, 0, 133, 134, 5, 124, 0, 0, 134, 135, 5, 124, 0, 0, 135, 28,
		1, 0, 0, 0, 136, 137, 5, 62, 0, 0, 137, 30, 1, 0, 0, 0, 138, 139, 5, 46,
		0, 0, 139, 32, 1, 0, 0, 0, 140, 141, 5, 60, 0, 0, 141, 34, 1, 0, 0, 0,
		142, 143, 5, 61, 0, 0, 143, 36, 1, 0, 0, 0, 144, 145, 5, 40, 0, 0, 145,
		38, 1, 0, 0, 0, 146, 147, 5, 44, 0, 0, 147, 40, 1, 0, 0, 0, 148, 149, 5,
		41, 0, 0, 149, 42, 1, 0, 0, 0, 150, 151, 5, 91, 0, 0, 151, 44, 1, 0, 0,
		0, 152, 153, 5, 93, 0, 0, 153, 46, 1, 0, 0, 0, 154, 155, 5, 123, 0, 0,
		155, 48, 1, 0, 0, 0, 156, 157, 5, 125, 0, 0, 157, 50, 1, 0, 0, 0, 158,
		159, 5, 35, 0, 0, 159, 52, 1, 0, 0, 0, 160, 161, 5, 36, 0, 0, 161, 54,
		1, 0, 0, 0, 162, 163, 5, 58, 0, 0, 163, 56, 1, 0, 0, 0, 164, 165, 5, 37,
		0, 0, 165, 58, 1, 0, 0, 0, 166, 167, 7, 0, 0, 0, 167, 168, 1, 0, 0, 0,
		168, 169, 6, 29, 0, 0, 169, 60, 1, 0, 0, 0, 170, 172, 3, 89, 44, 0, 171,
		170, 1, 0, 0, 0, 172, 173, 1, 0, 0, 0, 173, 171, 1, 0, 0, 0, 173, 174,
		1, 0, 0, 0, 174, 62, 1, 0, 0, 0, 175, 176, 5, 48, 0, 0, 176, 177, 5, 111,
		0, 0, 177, 179, 1, 0, 0, 0, 178, 180, 3, 91, 45, 0, 179, 178, 1, 0, 0,
		0, 180, 181, 1, 0, 0, 0, 181, 179, 1, 0, 0, 0, 181, 182, 1, 0, 0, 0, 182,
		64, 1, 0, 0, 0, 183, 184, 5, 48, 0, 0, 184, 185, 5, 98, 0, 0, 185, 187,
		1, 0, 0, 0, 186, 188, 2, 48, 49, 0, 187, 186, 1, 0, 0, 0, 188, 189, 1,
		0, 0, 0, 189, 187, 1, 0, 0, 0, 189, 190, 1, 0, 0, 0, 190, 66, 1, 0, 0,
		0, 191, 192, 5, 48, 0, 0, 192, 193, 5, 120, 0, 0, 193, 195, 1, 0, 0, 0,
		194, 196, 3, 87, 43, 0, 195, 194, 1, 0, 0, 0, 196, 197, 1, 0, 0, 0, 197,
		195, 1, 0, 0, 0, 197, 198, 1, 0, 0, 0, 198, 68, 1, 0, 0, 0, 199, 203, 5,
		96, 0, 0, 200, 202, 8, 1, 0, 0, 201, 200, 1, 0, 0, 0, 202, 205, 1, 0, 0,
		0, 203, 201, 1, 0, 0, 0, 203, 204, 1, 0, 0, 0, 204, 206, 1, 0, 0, 0, 205,
		203, 1, 0, 0, 0, 206, 207, 5, 96, 0, 0, 207, 70, 1, 0, 0, 0, 208, 209,
		5, 115, 0, 0, 209, 210, 5, 116, 0, 0, 210, 211, 5, 114, 0, 0, 211, 72,
		1, 0, 0, 0, 212, 213, 5, 108, 0, 0, 213, 214, 5, 105, 0, 0, 214, 215, 5,
		115, 0, 0, 215, 216, 5, 116, 0, 0, 216, 74, 1, 0, 0, 0, 217, 218, 5, 100,
		0, 0, 218, 219, 5, 105, 0, 0, 219, 220, 5, 99, 0, 0, 220, 221, 5, 116,
		0, 0, 221, 76, 1, 0, 0, 0, 222, 223, 5, 105, 0, 0, 223, 224, 5, 110, 0,
		0, 224, 231, 5, 116, 0, 0, 225, 226, 5, 102, 0, 0, 226, 227, 5, 108, 0,
		0, 227, 228, 5, 111, 0, 0, 228, 229, 5, 97, 0, 0, 229, 231, 5, 116, 0,
		0, 230, 222, 1, 0, 0, 0, 230, 225, 1, 0, 0, 0, 231, 78, 1, 0, 0, 0, 232,
		233, 5, 116, 0, 0, 233, 234, 5, 114, 0, 0, 234, 235, 5, 117, 0, 0, 235,
		242, 5, 101, 0, 0, 236, 237, 5, 102, 0, 0, 237, 238, 5, 97, 0, 0, 238,
		239, 5, 108, 0, 0, 239, 240, 5, 115, 0, 0, 240, 242, 5, 101, 0, 0, 241,
		232, 1, 0, 0, 0, 241, 236, 1, 0, 0, 0, 242, 80, 1, 0, 0, 0, 243, 247, 3,
		83, 41, 0, 244, 246, 3, 85, 42, 0, 245, 244, 1, 0, 0, 0, 246, 249, 1, 0,
		0, 0, 247, 245, 1, 0, 0, 0, 247, 248, 1, 0, 0, 0, 248, 82, 1, 0, 0, 0,
		249, 247, 1, 0, 0, 0, 250, 254, 7, 2, 0, 0, 251, 252, 5, 37, 0, 0, 252,
		254, 5, 37, 0, 0, 253, 250, 1, 0, 0, 0, 253, 251, 1, 0, 0, 0, 254, 84,
		1, 0, 0, 0, 255, 258, 7, 3, 0, 0, 256, 258, 3, 83, 41, 0, 257, 255, 1,
		0, 0, 0, 257, 256, 1, 0, 0, 0, 258, 86, 1, 0, 0, 0, 259, 260, 7, 4, 0,
		0, 260, 88, 1, 0, 0, 0, 261, 262, 7, 3, 0, 0, 262, 90, 1, 0, 0, 0, 263,
		264, 7, 5, 0, 0, 264, 92, 1, 0, 0, 0, 11, 0, 173, 181, 189, 197, 203, 230,
		241, 247, 253, 257, 1, 6, 0, 0,
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
	SyntaxFlowLexerAnd             = 13
	SyntaxFlowLexerOr              = 14
	SyntaxFlowLexerGt              = 15
	SyntaxFlowLexerDot             = 16
	SyntaxFlowLexerLt              = 17
	SyntaxFlowLexerEq              = 18
	SyntaxFlowLexerOpenParen       = 19
	SyntaxFlowLexerComma           = 20
	SyntaxFlowLexerCloseParen      = 21
	SyntaxFlowLexerListSelectOpen  = 22
	SyntaxFlowLexerListSelectClose = 23
	SyntaxFlowLexerMapBuilderOpen  = 24
	SyntaxFlowLexerMapBuilderClose = 25
	SyntaxFlowLexerListStart       = 26
	SyntaxFlowLexerDollarOutput    = 27
	SyntaxFlowLexerColon           = 28
	SyntaxFlowLexerSearch          = 29
	SyntaxFlowLexerWhiteSpace      = 30
	SyntaxFlowLexerNumber          = 31
	SyntaxFlowLexerOctalNumber     = 32
	SyntaxFlowLexerBinaryNumber    = 33
	SyntaxFlowLexerHexNumber       = 34
	SyntaxFlowLexerStringLiteral   = 35
	SyntaxFlowLexerStringType      = 36
	SyntaxFlowLexerListType        = 37
	SyntaxFlowLexerDictType        = 38
	SyntaxFlowLexerNumberType      = 39
	SyntaxFlowLexerBoolType        = 40
	SyntaxFlowLexerIdentifier      = 41
)
