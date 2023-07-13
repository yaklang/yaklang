// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package parser

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

type SuricataRuleLexer struct {
	*antlr.BaseLexer
	channelNames []string
	modeNames    []string
	// TODO: EOF string
}

var suricatarulelexerLexerStaticData struct {
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

func suricatarulelexerLexerInit() {
	staticData := &suricatarulelexerLexerStaticData
	staticData.channelNames = []string{
		"DEFAULT_TOKEN_CHANNEL", "HIDDEN",
	}
	staticData.modeNames = []string{
		"DEFAULT_MODE", "PARAM_MODE",
	}
	staticData.literalNames = []string{
		"", "'any'", "'!'", "'$'", "'->'", "'<>'", "'*'", "'/'", "'%'", "'&'",
		"'+'", "'-'", "'^'", "'<'", "'>'", "'<='", "'>='", "':'", "'::'", "'['",
		"']'", "'('", "'{'", "'}'", "','", "'='", "'~'", "'.'", "", "", "",
		"", "", "", "", "", "", "';'", "", "')'",
	}
	staticData.symbolicNames = []string{
		"", "Any", "Negative", "Dollar", "Arrow", "BothDirect", "Mul", "Div",
		"Mod", "Amp", "Plus", "Sub", "Power", "Lt", "Gt", "LtEq", "GtEq", "Colon",
		"DoubleColon", "LBracket", "RBracket", "ParamStart", "LBrace", "RBrace",
		"Comma", "Eq", "NotSymbol", "Dot", "LINE_COMMENT", "ID", "NORMALSTRING",
		"INT", "HEX", "WS", "NonSemiColon", "SHEBANG", "ParamQuotedString",
		"ParamSep", "ParamValue", "ParamEnd",
	}
	staticData.ruleNames = []string{
		"Any", "Negative", "Dollar", "Arrow", "BothDirect", "Mul", "Div", "Mod",
		"Amp", "Plus", "Sub", "Power", "Lt", "Gt", "LtEq", "GtEq", "Colon",
		"DoubleColon", "LBracket", "RBracket", "ParamStart", "LBrace", "RBrace",
		"Comma", "Eq", "NotSymbol", "Dot", "LINE_COMMENT", "ID", "NORMALSTRING",
		"INT", "HEX", "ExponentPart", "HexExponentPart", "EscapeSequence", "DecimalEscape",
		"HexEscape", "UtfEscape", "Digit", "HexDigit", "SingleLineInputCharacter",
		"WS", "NonSemiColon", "SHEBANG", "Quote", "CharInQuotedString", "ParamQuotedString",
		"ParamSep", "ParamValue", "ParamEnd", "FreeValueAnyChar",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 0, 39, 320, 6, -1, 6, -1, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3,
		7, 3, 2, 4, 7, 4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9,
		7, 9, 2, 10, 7, 10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7,
		14, 2, 15, 7, 15, 2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19,
		2, 20, 7, 20, 2, 21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2,
		25, 7, 25, 2, 26, 7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 2, 29, 7, 29, 2, 30,
		7, 30, 2, 31, 7, 31, 2, 32, 7, 32, 2, 33, 7, 33, 2, 34, 7, 34, 2, 35, 7,
		35, 2, 36, 7, 36, 2, 37, 7, 37, 2, 38, 7, 38, 2, 39, 7, 39, 2, 40, 7, 40,
		2, 41, 7, 41, 2, 42, 7, 42, 2, 43, 7, 43, 2, 44, 7, 44, 2, 45, 7, 45, 2,
		46, 7, 46, 2, 47, 7, 47, 2, 48, 7, 48, 2, 49, 7, 49, 2, 50, 7, 50, 1, 0,
		1, 0, 1, 0, 1, 0, 1, 1, 1, 1, 1, 2, 1, 2, 1, 3, 1, 3, 1, 3, 1, 4, 1, 4,
		1, 4, 1, 5, 1, 5, 1, 6, 1, 6, 1, 7, 1, 7, 1, 8, 1, 8, 1, 9, 1, 9, 1, 10,
		1, 10, 1, 11, 1, 11, 1, 12, 1, 12, 1, 13, 1, 13, 1, 14, 1, 14, 1, 14, 1,
		15, 1, 15, 1, 15, 1, 16, 1, 16, 1, 17, 1, 17, 1, 17, 1, 18, 1, 18, 1, 19,
		1, 19, 1, 20, 1, 20, 1, 20, 1, 20, 1, 21, 1, 21, 1, 22, 1, 22, 1, 23, 1,
		23, 1, 24, 1, 24, 1, 25, 1, 25, 1, 26, 1, 26, 1, 27, 1, 27, 1, 27, 3, 27,
		171, 8, 27, 1, 27, 5, 27, 174, 8, 27, 10, 27, 12, 27, 177, 9, 27, 1, 27,
		1, 27, 1, 28, 1, 28, 5, 28, 183, 8, 28, 10, 28, 12, 28, 186, 9, 28, 1,
		29, 1, 29, 1, 29, 5, 29, 191, 8, 29, 10, 29, 12, 29, 194, 9, 29, 1, 29,
		1, 29, 1, 30, 4, 30, 199, 8, 30, 11, 30, 12, 30, 200, 1, 31, 4, 31, 204,
		8, 31, 11, 31, 12, 31, 205, 1, 32, 1, 32, 3, 32, 210, 8, 32, 1, 32, 4,
		32, 213, 8, 32, 11, 32, 12, 32, 214, 1, 33, 1, 33, 3, 33, 219, 8, 33, 1,
		33, 4, 33, 222, 8, 33, 11, 33, 12, 33, 223, 1, 34, 1, 34, 1, 34, 1, 34,
		3, 34, 230, 8, 34, 1, 34, 1, 34, 1, 34, 1, 34, 3, 34, 236, 8, 34, 1, 35,
		1, 35, 1, 35, 1, 35, 1, 35, 1, 35, 1, 35, 1, 35, 1, 35, 1, 35, 1, 35, 3,
		35, 249, 8, 35, 1, 36, 1, 36, 1, 36, 1, 36, 1, 36, 1, 37, 1, 37, 1, 37,
		1, 37, 1, 37, 4, 37, 261, 8, 37, 11, 37, 12, 37, 262, 1, 37, 1, 37, 1,
		38, 1, 38, 1, 39, 1, 39, 1, 40, 1, 40, 1, 41, 4, 41, 274, 8, 41, 11, 41,
		12, 41, 275, 1, 41, 1, 41, 1, 42, 4, 42, 281, 8, 42, 11, 42, 12, 42, 282,
		1, 43, 1, 43, 1, 43, 5, 43, 288, 8, 43, 10, 43, 12, 43, 291, 9, 43, 1,
		43, 1, 43, 1, 44, 1, 44, 1, 45, 1, 45, 1, 46, 1, 46, 5, 46, 301, 8, 46,
		10, 46, 12, 46, 304, 9, 46, 1, 46, 1, 46, 1, 47, 1, 47, 1, 48, 4, 48, 311,
		8, 48, 11, 48, 12, 48, 312, 1, 49, 1, 49, 1, 49, 1, 49, 1, 50, 1, 50, 0,
		0, 51, 2, 1, 4, 2, 6, 3, 8, 4, 10, 5, 12, 6, 14, 7, 16, 8, 18, 9, 20, 10,
		22, 11, 24, 12, 26, 13, 28, 14, 30, 15, 32, 16, 34, 17, 36, 18, 38, 19,
		40, 20, 42, 21, 44, 22, 46, 23, 48, 24, 50, 25, 52, 26, 54, 27, 56, 28,
		58, 29, 60, 30, 62, 31, 64, 32, 66, 0, 68, 0, 70, 0, 72, 0, 74, 0, 76,
		0, 78, 0, 80, 0, 82, 0, 84, 33, 86, 34, 88, 35, 90, 0, 92, 0, 94, 36, 96,
		37, 98, 38, 100, 39, 102, 0, 2, 0, 1, 15, 3, 0, 65, 90, 95, 95, 97, 122,
		4, 0, 48, 57, 65, 90, 95, 95, 97, 122, 2, 0, 34, 34, 92, 92, 2, 0, 69,
		69, 101, 101, 2, 0, 43, 43, 45, 45, 2, 0, 80, 80, 112, 112, 11, 0, 34,
		36, 39, 39, 92, 92, 97, 98, 102, 102, 110, 110, 114, 114, 116, 116, 118,
		118, 122, 122, 124, 124, 1, 0, 48, 50, 1, 0, 48, 57, 3, 0, 48, 57, 65,
		70, 97, 102, 4, 0, 10, 10, 13, 13, 133, 133, 8232, 8233, 3, 0, 9, 10, 12,
		13, 32, 32, 2, 0, 59, 59, 94, 94, 1, 0, 34, 34, 3, 0, 34, 34, 41, 41, 59,
		59, 330, 0, 2, 1, 0, 0, 0, 0, 4, 1, 0, 0, 0, 0, 6, 1, 0, 0, 0, 0, 8, 1,
		0, 0, 0, 0, 10, 1, 0, 0, 0, 0, 12, 1, 0, 0, 0, 0, 14, 1, 0, 0, 0, 0, 16,
		1, 0, 0, 0, 0, 18, 1, 0, 0, 0, 0, 20, 1, 0, 0, 0, 0, 22, 1, 0, 0, 0, 0,
		24, 1, 0, 0, 0, 0, 26, 1, 0, 0, 0, 0, 28, 1, 0, 0, 0, 0, 30, 1, 0, 0, 0,
		0, 32, 1, 0, 0, 0, 0, 34, 1, 0, 0, 0, 0, 36, 1, 0, 0, 0, 0, 38, 1, 0, 0,
		0, 0, 40, 1, 0, 0, 0, 0, 42, 1, 0, 0, 0, 0, 44, 1, 0, 0, 0, 0, 46, 1, 0,
		0, 0, 0, 48, 1, 0, 0, 0, 0, 50, 1, 0, 0, 0, 0, 52, 1, 0, 0, 0, 0, 54, 1,
		0, 0, 0, 0, 56, 1, 0, 0, 0, 0, 58, 1, 0, 0, 0, 0, 60, 1, 0, 0, 0, 0, 62,
		1, 0, 0, 0, 0, 64, 1, 0, 0, 0, 0, 84, 1, 0, 0, 0, 0, 86, 1, 0, 0, 0, 0,
		88, 1, 0, 0, 0, 1, 94, 1, 0, 0, 0, 1, 96, 1, 0, 0, 0, 1, 98, 1, 0, 0, 0,
		1, 100, 1, 0, 0, 0, 2, 104, 1, 0, 0, 0, 4, 108, 1, 0, 0, 0, 6, 110, 1,
		0, 0, 0, 8, 112, 1, 0, 0, 0, 10, 115, 1, 0, 0, 0, 12, 118, 1, 0, 0, 0,
		14, 120, 1, 0, 0, 0, 16, 122, 1, 0, 0, 0, 18, 124, 1, 0, 0, 0, 20, 126,
		1, 0, 0, 0, 22, 128, 1, 0, 0, 0, 24, 130, 1, 0, 0, 0, 26, 132, 1, 0, 0,
		0, 28, 134, 1, 0, 0, 0, 30, 136, 1, 0, 0, 0, 32, 139, 1, 0, 0, 0, 34, 142,
		1, 0, 0, 0, 36, 144, 1, 0, 0, 0, 38, 147, 1, 0, 0, 0, 40, 149, 1, 0, 0,
		0, 42, 151, 1, 0, 0, 0, 44, 155, 1, 0, 0, 0, 46, 157, 1, 0, 0, 0, 48, 159,
		1, 0, 0, 0, 50, 161, 1, 0, 0, 0, 52, 163, 1, 0, 0, 0, 54, 165, 1, 0, 0,
		0, 56, 170, 1, 0, 0, 0, 58, 180, 1, 0, 0, 0, 60, 187, 1, 0, 0, 0, 62, 198,
		1, 0, 0, 0, 64, 203, 1, 0, 0, 0, 66, 207, 1, 0, 0, 0, 68, 216, 1, 0, 0,
		0, 70, 235, 1, 0, 0, 0, 72, 248, 1, 0, 0, 0, 74, 250, 1, 0, 0, 0, 76, 255,
		1, 0, 0, 0, 78, 266, 1, 0, 0, 0, 80, 268, 1, 0, 0, 0, 82, 270, 1, 0, 0,
		0, 84, 273, 1, 0, 0, 0, 86, 280, 1, 0, 0, 0, 88, 284, 1, 0, 0, 0, 90, 294,
		1, 0, 0, 0, 92, 296, 1, 0, 0, 0, 94, 298, 1, 0, 0, 0, 96, 307, 1, 0, 0,
		0, 98, 310, 1, 0, 0, 0, 100, 314, 1, 0, 0, 0, 102, 318, 1, 0, 0, 0, 104,
		105, 5, 97, 0, 0, 105, 106, 5, 110, 0, 0, 106, 107, 5, 121, 0, 0, 107,
		3, 1, 0, 0, 0, 108, 109, 5, 33, 0, 0, 109, 5, 1, 0, 0, 0, 110, 111, 5,
		36, 0, 0, 111, 7, 1, 0, 0, 0, 112, 113, 5, 45, 0, 0, 113, 114, 5, 62, 0,
		0, 114, 9, 1, 0, 0, 0, 115, 116, 5, 60, 0, 0, 116, 117, 5, 62, 0, 0, 117,
		11, 1, 0, 0, 0, 118, 119, 5, 42, 0, 0, 119, 13, 1, 0, 0, 0, 120, 121, 5,
		47, 0, 0, 121, 15, 1, 0, 0, 0, 122, 123, 5, 37, 0, 0, 123, 17, 1, 0, 0,
		0, 124, 125, 5, 38, 0, 0, 125, 19, 1, 0, 0, 0, 126, 127, 5, 43, 0, 0, 127,
		21, 1, 0, 0, 0, 128, 129, 5, 45, 0, 0, 129, 23, 1, 0, 0, 0, 130, 131, 5,
		94, 0, 0, 131, 25, 1, 0, 0, 0, 132, 133, 5, 60, 0, 0, 133, 27, 1, 0, 0,
		0, 134, 135, 5, 62, 0, 0, 135, 29, 1, 0, 0, 0, 136, 137, 5, 60, 0, 0, 137,
		138, 5, 61, 0, 0, 138, 31, 1, 0, 0, 0, 139, 140, 5, 62, 0, 0, 140, 141,
		5, 61, 0, 0, 141, 33, 1, 0, 0, 0, 142, 143, 5, 58, 0, 0, 143, 35, 1, 0,
		0, 0, 144, 145, 5, 58, 0, 0, 145, 146, 5, 58, 0, 0, 146, 37, 1, 0, 0, 0,
		147, 148, 5, 91, 0, 0, 148, 39, 1, 0, 0, 0, 149, 150, 5, 93, 0, 0, 150,
		41, 1, 0, 0, 0, 151, 152, 5, 40, 0, 0, 152, 153, 1, 0, 0, 0, 153, 154,
		6, 20, 0, 0, 154, 43, 1, 0, 0, 0, 155, 156, 5, 123, 0, 0, 156, 45, 1, 0,
		0, 0, 157, 158, 5, 125, 0, 0, 158, 47, 1, 0, 0, 0, 159, 160, 5, 44, 0,
		0, 160, 49, 1, 0, 0, 0, 161, 162, 5, 61, 0, 0, 162, 51, 1, 0, 0, 0, 163,
		164, 5, 126, 0, 0, 164, 53, 1, 0, 0, 0, 165, 166, 5, 46, 0, 0, 166, 55,
		1, 0, 0, 0, 167, 171, 5, 35, 0, 0, 168, 169, 5, 47, 0, 0, 169, 171, 5,
		47, 0, 0, 170, 167, 1, 0, 0, 0, 170, 168, 1, 0, 0, 0, 171, 175, 1, 0, 0,
		0, 172, 174, 3, 82, 40, 0, 173, 172, 1, 0, 0, 0, 174, 177, 1, 0, 0, 0,
		175, 173, 1, 0, 0, 0, 175, 176, 1, 0, 0, 0, 176, 178, 1, 0, 0, 0, 177,
		175, 1, 0, 0, 0, 178, 179, 6, 27, 1, 0, 179, 57, 1, 0, 0, 0, 180, 184,
		7, 0, 0, 0, 181, 183, 7, 1, 0, 0, 182, 181, 1, 0, 0, 0, 183, 186, 1, 0,
		0, 0, 184, 182, 1, 0, 0, 0, 184, 185, 1, 0, 0, 0, 185, 59, 1, 0, 0, 0,
		186, 184, 1, 0, 0, 0, 187, 192, 5, 34, 0, 0, 188, 191, 3, 70, 34, 0, 189,
		191, 8, 2, 0, 0, 190, 188, 1, 0, 0, 0, 190, 189, 1, 0, 0, 0, 191, 194,
		1, 0, 0, 0, 192, 190, 1, 0, 0, 0, 192, 193, 1, 0, 0, 0, 193, 195, 1, 0,
		0, 0, 194, 192, 1, 0, 0, 0, 195, 196, 5, 34, 0, 0, 196, 61, 1, 0, 0, 0,
		197, 199, 3, 78, 38, 0, 198, 197, 1, 0, 0, 0, 199, 200, 1, 0, 0, 0, 200,
		198, 1, 0, 0, 0, 200, 201, 1, 0, 0, 0, 201, 63, 1, 0, 0, 0, 202, 204, 3,
		80, 39, 0, 203, 202, 1, 0, 0, 0, 204, 205, 1, 0, 0, 0, 205, 203, 1, 0,
		0, 0, 205, 206, 1, 0, 0, 0, 206, 65, 1, 0, 0, 0, 207, 209, 7, 3, 0, 0,
		208, 210, 7, 4, 0, 0, 209, 208, 1, 0, 0, 0, 209, 210, 1, 0, 0, 0, 210,
		212, 1, 0, 0, 0, 211, 213, 3, 78, 38, 0, 212, 211, 1, 0, 0, 0, 213, 214,
		1, 0, 0, 0, 214, 212, 1, 0, 0, 0, 214, 215, 1, 0, 0, 0, 215, 67, 1, 0,
		0, 0, 216, 218, 7, 5, 0, 0, 217, 219, 7, 4, 0, 0, 218, 217, 1, 0, 0, 0,
		218, 219, 1, 0, 0, 0, 219, 221, 1, 0, 0, 0, 220, 222, 3, 78, 38, 0, 221,
		220, 1, 0, 0, 0, 222, 223, 1, 0, 0, 0, 223, 221, 1, 0, 0, 0, 223, 224,
		1, 0, 0, 0, 224, 69, 1, 0, 0, 0, 225, 226, 5, 92, 0, 0, 226, 236, 7, 6,
		0, 0, 227, 229, 5, 92, 0, 0, 228, 230, 5, 13, 0, 0, 229, 228, 1, 0, 0,
		0, 229, 230, 1, 0, 0, 0, 230, 231, 1, 0, 0, 0, 231, 236, 5, 10, 0, 0, 232,
		236, 3, 72, 35, 0, 233, 236, 3, 74, 36, 0, 234, 236, 3, 76, 37, 0, 235,
		225, 1, 0, 0, 0, 235, 227, 1, 0, 0, 0, 235, 232, 1, 0, 0, 0, 235, 233,
		1, 0, 0, 0, 235, 234, 1, 0, 0, 0, 236, 71, 1, 0, 0, 0, 237, 238, 5, 92,
		0, 0, 238, 249, 3, 78, 38, 0, 239, 240, 5, 92, 0, 0, 240, 241, 3, 78, 38,
		0, 241, 242, 3, 78, 38, 0, 242, 249, 1, 0, 0, 0, 243, 244, 5, 92, 0, 0,
		244, 245, 7, 7, 0, 0, 245, 246, 3, 78, 38, 0, 246, 247, 3, 78, 38, 0, 247,
		249, 1, 0, 0, 0, 248, 237, 1, 0, 0, 0, 248, 239, 1, 0, 0, 0, 248, 243,
		1, 0, 0, 0, 249, 73, 1, 0, 0, 0, 250, 251, 5, 92, 0, 0, 251, 252, 5, 120,
		0, 0, 252, 253, 3, 80, 39, 0, 253, 254, 3, 80, 39, 0, 254, 75, 1, 0, 0,
		0, 255, 256, 5, 92, 0, 0, 256, 257, 5, 117, 0, 0, 257, 258, 5, 123, 0,
		0, 258, 260, 1, 0, 0, 0, 259, 261, 3, 80, 39, 0, 260, 259, 1, 0, 0, 0,
		261, 262, 1, 0, 0, 0, 262, 260, 1, 0, 0, 0, 262, 263, 1, 0, 0, 0, 263,
		264, 1, 0, 0, 0, 264, 265, 5, 125, 0, 0, 265, 77, 1, 0, 0, 0, 266, 267,
		7, 8, 0, 0, 267, 79, 1, 0, 0, 0, 268, 269, 7, 9, 0, 0, 269, 81, 1, 0, 0,
		0, 270, 271, 8, 10, 0, 0, 271, 83, 1, 0, 0, 0, 272, 274, 7, 11, 0, 0, 273,
		272, 1, 0, 0, 0, 274, 275, 1, 0, 0, 0, 275, 273, 1, 0, 0, 0, 275, 276,
		1, 0, 0, 0, 276, 277, 1, 0, 0, 0, 277, 278, 6, 41, 1, 0, 278, 85, 1, 0,
		0, 0, 279, 281, 7, 12, 0, 0, 280, 279, 1, 0, 0, 0, 281, 282, 1, 0, 0, 0,
		282, 280, 1, 0, 0, 0, 282, 283, 1, 0, 0, 0, 283, 87, 1, 0, 0, 0, 284, 285,
		5, 35, 0, 0, 285, 289, 5, 33, 0, 0, 286, 288, 3, 82, 40, 0, 287, 286, 1,
		0, 0, 0, 288, 291, 1, 0, 0, 0, 289, 287, 1, 0, 0, 0, 289, 290, 1, 0, 0,
		0, 290, 292, 1, 0, 0, 0, 291, 289, 1, 0, 0, 0, 292, 293, 6, 43, 2, 0, 293,
		89, 1, 0, 0, 0, 294, 295, 5, 34, 0, 0, 295, 91, 1, 0, 0, 0, 296, 297, 8,
		13, 0, 0, 297, 93, 1, 0, 0, 0, 298, 302, 3, 90, 44, 0, 299, 301, 3, 92,
		45, 0, 300, 299, 1, 0, 0, 0, 301, 304, 1, 0, 0, 0, 302, 300, 1, 0, 0, 0,
		302, 303, 1, 0, 0, 0, 303, 305, 1, 0, 0, 0, 304, 302, 1, 0, 0, 0, 305,
		306, 3, 90, 44, 0, 306, 95, 1, 0, 0, 0, 307, 308, 5, 59, 0, 0, 308, 97,
		1, 0, 0, 0, 309, 311, 3, 102, 50, 0, 310, 309, 1, 0, 0, 0, 311, 312, 1,
		0, 0, 0, 312, 310, 1, 0, 0, 0, 312, 313, 1, 0, 0, 0, 313, 99, 1, 0, 0,
		0, 314, 315, 5, 41, 0, 0, 315, 316, 1, 0, 0, 0, 316, 317, 6, 49, 3, 0,
		317, 101, 1, 0, 0, 0, 318, 319, 8, 14, 0, 0, 319, 103, 1, 0, 0, 0, 22,
		0, 1, 170, 175, 184, 190, 192, 200, 205, 209, 214, 218, 223, 229, 235,
		248, 262, 275, 282, 289, 302, 312, 4, 5, 1, 0, 6, 0, 0, 0, 1, 0, 4, 0,
		0,
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

// SuricataRuleLexerInit initializes any static state used to implement SuricataRuleLexer. By default the
// static state used to implement the lexer is lazily initialized during the first call to
// NewSuricataRuleLexer(). You can call this function if you wish to initialize the static state ahead
// of time.
func SuricataRuleLexerInit() {
	staticData := &suricatarulelexerLexerStaticData
	staticData.once.Do(suricatarulelexerLexerInit)
}

// NewSuricataRuleLexer produces a new lexer instance for the optional input antlr.CharStream.
func NewSuricataRuleLexer(input antlr.CharStream) *SuricataRuleLexer {
	SuricataRuleLexerInit()
	l := new(SuricataRuleLexer)
	l.BaseLexer = antlr.NewBaseLexer(input)
	staticData := &suricatarulelexerLexerStaticData
	l.Interpreter = antlr.NewLexerATNSimulator(l, staticData.atn, staticData.decisionToDFA, staticData.predictionContextCache)
	l.channelNames = staticData.channelNames
	l.modeNames = staticData.modeNames
	l.RuleNames = staticData.ruleNames
	l.LiteralNames = staticData.literalNames
	l.SymbolicNames = staticData.symbolicNames
	l.GrammarFileName = "SuricataRuleLexer.g4"
	// TODO: l.EOF = antlr.TokenEOF

	return l
}

// SuricataRuleLexer tokens.
const (
	SuricataRuleLexerAny               = 1
	SuricataRuleLexerNegative          = 2
	SuricataRuleLexerDollar            = 3
	SuricataRuleLexerArrow             = 4
	SuricataRuleLexerBothDirect        = 5
	SuricataRuleLexerMul               = 6
	SuricataRuleLexerDiv               = 7
	SuricataRuleLexerMod               = 8
	SuricataRuleLexerAmp               = 9
	SuricataRuleLexerPlus              = 10
	SuricataRuleLexerSub               = 11
	SuricataRuleLexerPower             = 12
	SuricataRuleLexerLt                = 13
	SuricataRuleLexerGt                = 14
	SuricataRuleLexerLtEq              = 15
	SuricataRuleLexerGtEq              = 16
	SuricataRuleLexerColon             = 17
	SuricataRuleLexerDoubleColon       = 18
	SuricataRuleLexerLBracket          = 19
	SuricataRuleLexerRBracket          = 20
	SuricataRuleLexerParamStart        = 21
	SuricataRuleLexerLBrace            = 22
	SuricataRuleLexerRBrace            = 23
	SuricataRuleLexerComma             = 24
	SuricataRuleLexerEq                = 25
	SuricataRuleLexerNotSymbol         = 26
	SuricataRuleLexerDot               = 27
	SuricataRuleLexerLINE_COMMENT      = 28
	SuricataRuleLexerID                = 29
	SuricataRuleLexerNORMALSTRING      = 30
	SuricataRuleLexerINT               = 31
	SuricataRuleLexerHEX               = 32
	SuricataRuleLexerWS                = 33
	SuricataRuleLexerNonSemiColon      = 34
	SuricataRuleLexerSHEBANG           = 35
	SuricataRuleLexerParamQuotedString = 36
	SuricataRuleLexerParamSep          = 37
	SuricataRuleLexerParamValue        = 38
	SuricataRuleLexerParamEnd          = 39
)

// SuricataRuleLexerPARAM_MODE is the SuricataRuleLexer mode.
const SuricataRuleLexerPARAM_MODE = 1
