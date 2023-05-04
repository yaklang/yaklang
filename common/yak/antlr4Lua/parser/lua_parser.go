// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package parser // LuaParser

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

// Suppress unused import errors
var _ = fmt.Printf
var _ = strconv.Itoa
var _ = sync.Once{}

type LuaParser struct {
	*antlr.BaseParser
}

var luaparserParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	literalNames           []string
	symbolicNames          []string
	ruleNames              []string
	predictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func luaparserParserInit() {
	staticData := &luaparserParserStaticData
	staticData.literalNames = []string{
		"", "'function'", "'nil'", "'false'", "'true'", "'return'", "'break'",
		"'continue'", "'goto'", "'repeat'", "'until'", "'while'", "'do'", "'end'",
		"'if'", "'then'", "'else'", "'elseif'", "'for'", "'in'", "'local'",
		"'not'", "'or'", "'and'", "'*'", "'/'", "'//'", "'%'", "'&'", "'|'",
		"'<<'", "'>>'", "'+'", "'-'", "'^'", "'<'", "'>'", "'=='", "'<='", "'>='",
		"'~='", "':'", "'::'", "'['", "']'", "'('", "')'", "'{'", "'}'", "','",
		"'#'", "'='", "'++'", "'--'", "'+='", "'-='", "'*='", "'/='", "'%='",
		"';'", "'...'", "'~'", "'.'", "'..'",
	}
	staticData.symbolicNames = []string{
		"", "Function", "Nil", "False", "True", "Return", "Break", "Continue",
		"Goto", "Repeat", "Until", "While", "Do", "End", "If", "Then", "Else",
		"ElseIf", "For", "In", "Local", "Not", "Or", "And", "Mul", "Div", "IntegralDiv",
		"Mod", "Amp", "Xand", "LtLt", "GtGt", "Plus", "Sub", "Power", "Lt",
		"Gt", "Eq", "LtEq", "GtEq", "Neq", "Colon", "DoubleColon", "LBracket",
		"RBracket", "LParen", "RParen", "LBrace", "RBrace", "Comma", "Pound",
		"AssignEq", "PlusPlus", "SubSub", "PlusEq", "MinusEq", "MulEq", "DivEq",
		"ModEq", "SemiColon", "Ellipsis", "NotSymbol", "Dot", "Strcat", "NAME",
		"NORMALSTRING", "CHARSTRING", "LONGSTRING", "INT", "HEX", "FLOAT", "HEX_FLOAT",
		"COMMENT", "LINE_COMMENT", "WS", "SHEBANG",
	}
	staticData.ruleNames = []string{
		"chunk", "block", "stat", "attnamelist", "attrib", "laststat", "label",
		"funcname", "varlist", "namelist", "explist", "exp", "prefixexp", "functioncall",
		"varOrExp", "var", "varSuffix", "nameAndArgs", "args", "functiondef",
		"funcbody", "parlist", "tableconstructor", "fieldlist", "field", "fieldsep",
		"operatorOr", "operatorAnd", "operatorComparison", "operatorStrcat",
		"operatorAddSub", "operatorMulDivMod", "operatorBitwise", "operatorUnary",
		"operatorPower", "number", "string",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 75, 426, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7, 20, 2,
		21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25, 2, 26,
		7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 2, 29, 7, 29, 2, 30, 7, 30, 2, 31, 7,
		31, 2, 32, 7, 32, 2, 33, 7, 33, 2, 34, 7, 34, 2, 35, 7, 35, 2, 36, 7, 36,
		1, 0, 1, 0, 1, 0, 1, 1, 5, 1, 79, 8, 1, 10, 1, 12, 1, 82, 9, 1, 1, 1, 3,
		1, 85, 8, 1, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2,
		1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2,
		1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2,
		5, 2, 121, 8, 2, 10, 2, 12, 2, 124, 9, 2, 1, 2, 1, 2, 3, 2, 128, 8, 2,
		1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 3, 2, 140,
		8, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2,
		1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2,
		1, 2, 3, 2, 166, 8, 2, 3, 2, 168, 8, 2, 1, 3, 1, 3, 1, 3, 1, 3, 1, 3, 5,
		3, 175, 8, 3, 10, 3, 12, 3, 178, 9, 3, 1, 4, 1, 4, 1, 4, 3, 4, 183, 8,
		4, 1, 5, 1, 5, 3, 5, 187, 8, 5, 1, 5, 1, 5, 1, 5, 3, 5, 192, 8, 5, 3, 5,
		194, 8, 5, 1, 6, 1, 6, 1, 6, 1, 6, 1, 7, 1, 7, 1, 7, 5, 7, 203, 8, 7, 10,
		7, 12, 7, 206, 9, 7, 1, 7, 1, 7, 3, 7, 210, 8, 7, 1, 8, 1, 8, 1, 8, 5,
		8, 215, 8, 8, 10, 8, 12, 8, 218, 9, 8, 1, 9, 1, 9, 1, 9, 5, 9, 223, 8,
		9, 10, 9, 12, 9, 226, 9, 9, 1, 10, 1, 10, 1, 10, 5, 10, 231, 8, 10, 10,
		10, 12, 10, 234, 9, 10, 1, 10, 1, 10, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11,
		1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 3, 11, 251, 8,
		11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11,
		1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1,
		11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11,
		1, 11, 5, 11, 285, 8, 11, 10, 11, 12, 11, 288, 9, 11, 1, 12, 1, 12, 5,
		12, 292, 8, 12, 10, 12, 12, 12, 295, 9, 12, 1, 13, 1, 13, 4, 13, 299, 8,
		13, 11, 13, 12, 13, 300, 1, 14, 1, 14, 1, 14, 1, 14, 1, 14, 3, 14, 308,
		8, 14, 1, 15, 1, 15, 1, 15, 1, 15, 1, 15, 1, 15, 3, 15, 316, 8, 15, 1,
		15, 5, 15, 319, 8, 15, 10, 15, 12, 15, 322, 9, 15, 1, 16, 5, 16, 325, 8,
		16, 10, 16, 12, 16, 328, 9, 16, 1, 16, 1, 16, 1, 16, 1, 16, 1, 16, 1, 16,
		3, 16, 336, 8, 16, 1, 17, 1, 17, 3, 17, 340, 8, 17, 1, 17, 1, 17, 1, 18,
		1, 18, 3, 18, 346, 8, 18, 1, 18, 1, 18, 1, 18, 3, 18, 351, 8, 18, 1, 19,
		1, 19, 1, 19, 1, 20, 1, 20, 3, 20, 358, 8, 20, 1, 20, 1, 20, 1, 20, 1,
		20, 1, 21, 1, 21, 1, 21, 3, 21, 367, 8, 21, 1, 21, 3, 21, 370, 8, 21, 1,
		22, 1, 22, 3, 22, 374, 8, 22, 1, 22, 1, 22, 1, 23, 1, 23, 1, 23, 1, 23,
		5, 23, 382, 8, 23, 10, 23, 12, 23, 385, 9, 23, 1, 23, 3, 23, 388, 8, 23,
		1, 24, 1, 24, 1, 24, 1, 24, 1, 24, 1, 24, 1, 24, 1, 24, 1, 24, 1, 24, 3,
		24, 400, 8, 24, 1, 25, 1, 25, 1, 26, 1, 26, 1, 27, 1, 27, 1, 28, 1, 28,
		1, 29, 1, 29, 1, 30, 1, 30, 1, 31, 1, 31, 1, 32, 1, 32, 1, 33, 1, 33, 1,
		34, 1, 34, 1, 35, 1, 35, 1, 36, 1, 36, 1, 36, 0, 1, 22, 37, 0, 2, 4, 6,
		8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30, 32, 34, 36, 38, 40, 42,
		44, 46, 48, 50, 52, 54, 56, 58, 60, 62, 64, 66, 68, 70, 72, 0, 8, 2, 0,
		49, 49, 59, 59, 1, 0, 35, 40, 1, 0, 32, 33, 1, 0, 24, 27, 2, 0, 28, 31,
		61, 61, 4, 0, 21, 21, 33, 33, 50, 50, 61, 61, 1, 0, 68, 71, 1, 0, 65, 67,
		455, 0, 74, 1, 0, 0, 0, 2, 80, 1, 0, 0, 0, 4, 167, 1, 0, 0, 0, 6, 169,
		1, 0, 0, 0, 8, 182, 1, 0, 0, 0, 10, 193, 1, 0, 0, 0, 12, 195, 1, 0, 0,
		0, 14, 199, 1, 0, 0, 0, 16, 211, 1, 0, 0, 0, 18, 219, 1, 0, 0, 0, 20, 232,
		1, 0, 0, 0, 22, 250, 1, 0, 0, 0, 24, 289, 1, 0, 0, 0, 26, 296, 1, 0, 0,
		0, 28, 307, 1, 0, 0, 0, 30, 315, 1, 0, 0, 0, 32, 326, 1, 0, 0, 0, 34, 339,
		1, 0, 0, 0, 36, 350, 1, 0, 0, 0, 38, 352, 1, 0, 0, 0, 40, 355, 1, 0, 0,
		0, 42, 369, 1, 0, 0, 0, 44, 371, 1, 0, 0, 0, 46, 377, 1, 0, 0, 0, 48, 399,
		1, 0, 0, 0, 50, 401, 1, 0, 0, 0, 52, 403, 1, 0, 0, 0, 54, 405, 1, 0, 0,
		0, 56, 407, 1, 0, 0, 0, 58, 409, 1, 0, 0, 0, 60, 411, 1, 0, 0, 0, 62, 413,
		1, 0, 0, 0, 64, 415, 1, 0, 0, 0, 66, 417, 1, 0, 0, 0, 68, 419, 1, 0, 0,
		0, 70, 421, 1, 0, 0, 0, 72, 423, 1, 0, 0, 0, 74, 75, 3, 2, 1, 0, 75, 76,
		5, 0, 0, 1, 76, 1, 1, 0, 0, 0, 77, 79, 3, 4, 2, 0, 78, 77, 1, 0, 0, 0,
		79, 82, 1, 0, 0, 0, 80, 78, 1, 0, 0, 0, 80, 81, 1, 0, 0, 0, 81, 84, 1,
		0, 0, 0, 82, 80, 1, 0, 0, 0, 83, 85, 3, 10, 5, 0, 84, 83, 1, 0, 0, 0, 84,
		85, 1, 0, 0, 0, 85, 3, 1, 0, 0, 0, 86, 168, 5, 59, 0, 0, 87, 88, 3, 16,
		8, 0, 88, 89, 5, 51, 0, 0, 89, 90, 3, 20, 10, 0, 90, 168, 1, 0, 0, 0, 91,
		168, 3, 26, 13, 0, 92, 168, 3, 12, 6, 0, 93, 168, 5, 6, 0, 0, 94, 95, 5,
		8, 0, 0, 95, 168, 5, 64, 0, 0, 96, 97, 5, 12, 0, 0, 97, 98, 3, 2, 1, 0,
		98, 99, 5, 13, 0, 0, 99, 168, 1, 0, 0, 0, 100, 101, 5, 11, 0, 0, 101, 102,
		3, 22, 11, 0, 102, 103, 5, 12, 0, 0, 103, 104, 3, 2, 1, 0, 104, 105, 5,
		13, 0, 0, 105, 168, 1, 0, 0, 0, 106, 107, 5, 9, 0, 0, 107, 108, 3, 2, 1,
		0, 108, 109, 5, 10, 0, 0, 109, 110, 3, 22, 11, 0, 110, 168, 1, 0, 0, 0,
		111, 112, 5, 14, 0, 0, 112, 113, 3, 22, 11, 0, 113, 114, 5, 15, 0, 0, 114,
		122, 3, 2, 1, 0, 115, 116, 5, 17, 0, 0, 116, 117, 3, 22, 11, 0, 117, 118,
		5, 15, 0, 0, 118, 119, 3, 2, 1, 0, 119, 121, 1, 0, 0, 0, 120, 115, 1, 0,
		0, 0, 121, 124, 1, 0, 0, 0, 122, 120, 1, 0, 0, 0, 122, 123, 1, 0, 0, 0,
		123, 127, 1, 0, 0, 0, 124, 122, 1, 0, 0, 0, 125, 126, 5, 16, 0, 0, 126,
		128, 3, 2, 1, 0, 127, 125, 1, 0, 0, 0, 127, 128, 1, 0, 0, 0, 128, 129,
		1, 0, 0, 0, 129, 130, 5, 13, 0, 0, 130, 168, 1, 0, 0, 0, 131, 132, 5, 18,
		0, 0, 132, 133, 5, 64, 0, 0, 133, 134, 5, 51, 0, 0, 134, 135, 3, 22, 11,
		0, 135, 136, 5, 49, 0, 0, 136, 139, 3, 22, 11, 0, 137, 138, 5, 49, 0, 0,
		138, 140, 3, 22, 11, 0, 139, 137, 1, 0, 0, 0, 139, 140, 1, 0, 0, 0, 140,
		141, 1, 0, 0, 0, 141, 142, 5, 12, 0, 0, 142, 143, 3, 2, 1, 0, 143, 144,
		5, 13, 0, 0, 144, 168, 1, 0, 0, 0, 145, 146, 5, 18, 0, 0, 146, 147, 3,
		18, 9, 0, 147, 148, 5, 19, 0, 0, 148, 149, 3, 20, 10, 0, 149, 150, 5, 12,
		0, 0, 150, 151, 3, 2, 1, 0, 151, 152, 5, 13, 0, 0, 152, 168, 1, 0, 0, 0,
		153, 154, 5, 1, 0, 0, 154, 155, 3, 14, 7, 0, 155, 156, 3, 40, 20, 0, 156,
		168, 1, 0, 0, 0, 157, 158, 5, 20, 0, 0, 158, 159, 5, 1, 0, 0, 159, 160,
		5, 64, 0, 0, 160, 168, 3, 40, 20, 0, 161, 162, 5, 20, 0, 0, 162, 165, 3,
		6, 3, 0, 163, 164, 5, 51, 0, 0, 164, 166, 3, 20, 10, 0, 165, 163, 1, 0,
		0, 0, 165, 166, 1, 0, 0, 0, 166, 168, 1, 0, 0, 0, 167, 86, 1, 0, 0, 0,
		167, 87, 1, 0, 0, 0, 167, 91, 1, 0, 0, 0, 167, 92, 1, 0, 0, 0, 167, 93,
		1, 0, 0, 0, 167, 94, 1, 0, 0, 0, 167, 96, 1, 0, 0, 0, 167, 100, 1, 0, 0,
		0, 167, 106, 1, 0, 0, 0, 167, 111, 1, 0, 0, 0, 167, 131, 1, 0, 0, 0, 167,
		145, 1, 0, 0, 0, 167, 153, 1, 0, 0, 0, 167, 157, 1, 0, 0, 0, 167, 161,
		1, 0, 0, 0, 168, 5, 1, 0, 0, 0, 169, 170, 5, 64, 0, 0, 170, 176, 3, 8,
		4, 0, 171, 172, 5, 49, 0, 0, 172, 173, 5, 64, 0, 0, 173, 175, 3, 8, 4,
		0, 174, 171, 1, 0, 0, 0, 175, 178, 1, 0, 0, 0, 176, 174, 1, 0, 0, 0, 176,
		177, 1, 0, 0, 0, 177, 7, 1, 0, 0, 0, 178, 176, 1, 0, 0, 0, 179, 180, 5,
		35, 0, 0, 180, 181, 5, 64, 0, 0, 181, 183, 5, 36, 0, 0, 182, 179, 1, 0,
		0, 0, 182, 183, 1, 0, 0, 0, 183, 9, 1, 0, 0, 0, 184, 186, 5, 5, 0, 0, 185,
		187, 3, 20, 10, 0, 186, 185, 1, 0, 0, 0, 186, 187, 1, 0, 0, 0, 187, 194,
		1, 0, 0, 0, 188, 194, 5, 6, 0, 0, 189, 191, 5, 7, 0, 0, 190, 192, 5, 59,
		0, 0, 191, 190, 1, 0, 0, 0, 191, 192, 1, 0, 0, 0, 192, 194, 1, 0, 0, 0,
		193, 184, 1, 0, 0, 0, 193, 188, 1, 0, 0, 0, 193, 189, 1, 0, 0, 0, 194,
		11, 1, 0, 0, 0, 195, 196, 5, 42, 0, 0, 196, 197, 5, 64, 0, 0, 197, 198,
		5, 42, 0, 0, 198, 13, 1, 0, 0, 0, 199, 204, 5, 64, 0, 0, 200, 201, 5, 62,
		0, 0, 201, 203, 5, 64, 0, 0, 202, 200, 1, 0, 0, 0, 203, 206, 1, 0, 0, 0,
		204, 202, 1, 0, 0, 0, 204, 205, 1, 0, 0, 0, 205, 209, 1, 0, 0, 0, 206,
		204, 1, 0, 0, 0, 207, 208, 5, 41, 0, 0, 208, 210, 5, 64, 0, 0, 209, 207,
		1, 0, 0, 0, 209, 210, 1, 0, 0, 0, 210, 15, 1, 0, 0, 0, 211, 216, 3, 30,
		15, 0, 212, 213, 5, 49, 0, 0, 213, 215, 3, 30, 15, 0, 214, 212, 1, 0, 0,
		0, 215, 218, 1, 0, 0, 0, 216, 214, 1, 0, 0, 0, 216, 217, 1, 0, 0, 0, 217,
		17, 1, 0, 0, 0, 218, 216, 1, 0, 0, 0, 219, 224, 5, 64, 0, 0, 220, 221,
		5, 49, 0, 0, 221, 223, 5, 64, 0, 0, 222, 220, 1, 0, 0, 0, 223, 226, 1,
		0, 0, 0, 224, 222, 1, 0, 0, 0, 224, 225, 1, 0, 0, 0, 225, 19, 1, 0, 0,
		0, 226, 224, 1, 0, 0, 0, 227, 228, 3, 22, 11, 0, 228, 229, 5, 49, 0, 0,
		229, 231, 1, 0, 0, 0, 230, 227, 1, 0, 0, 0, 231, 234, 1, 0, 0, 0, 232,
		230, 1, 0, 0, 0, 232, 233, 1, 0, 0, 0, 233, 235, 1, 0, 0, 0, 234, 232,
		1, 0, 0, 0, 235, 236, 3, 22, 11, 0, 236, 21, 1, 0, 0, 0, 237, 238, 6, 11,
		-1, 0, 238, 251, 5, 2, 0, 0, 239, 251, 5, 3, 0, 0, 240, 251, 5, 4, 0, 0,
		241, 251, 3, 70, 35, 0, 242, 251, 3, 72, 36, 0, 243, 251, 5, 60, 0, 0,
		244, 251, 3, 38, 19, 0, 245, 251, 3, 24, 12, 0, 246, 251, 3, 44, 22, 0,
		247, 248, 3, 66, 33, 0, 248, 249, 3, 22, 11, 8, 249, 251, 1, 0, 0, 0, 250,
		237, 1, 0, 0, 0, 250, 239, 1, 0, 0, 0, 250, 240, 1, 0, 0, 0, 250, 241,
		1, 0, 0, 0, 250, 242, 1, 0, 0, 0, 250, 243, 1, 0, 0, 0, 250, 244, 1, 0,
		0, 0, 250, 245, 1, 0, 0, 0, 250, 246, 1, 0, 0, 0, 250, 247, 1, 0, 0, 0,
		251, 286, 1, 0, 0, 0, 252, 253, 10, 9, 0, 0, 253, 254, 3, 68, 34, 0, 254,
		255, 3, 22, 11, 9, 255, 285, 1, 0, 0, 0, 256, 257, 10, 7, 0, 0, 257, 258,
		3, 62, 31, 0, 258, 259, 3, 22, 11, 8, 259, 285, 1, 0, 0, 0, 260, 261, 10,
		6, 0, 0, 261, 262, 3, 60, 30, 0, 262, 263, 3, 22, 11, 7, 263, 285, 1, 0,
		0, 0, 264, 265, 10, 5, 0, 0, 265, 266, 3, 58, 29, 0, 266, 267, 3, 22, 11,
		5, 267, 285, 1, 0, 0, 0, 268, 269, 10, 4, 0, 0, 269, 270, 3, 56, 28, 0,
		270, 271, 3, 22, 11, 5, 271, 285, 1, 0, 0, 0, 272, 273, 10, 3, 0, 0, 273,
		274, 3, 54, 27, 0, 274, 275, 3, 22, 11, 4, 275, 285, 1, 0, 0, 0, 276, 277,
		10, 2, 0, 0, 277, 278, 3, 52, 26, 0, 278, 279, 3, 22, 11, 3, 279, 285,
		1, 0, 0, 0, 280, 281, 10, 1, 0, 0, 281, 282, 3, 64, 32, 0, 282, 283, 3,
		22, 11, 2, 283, 285, 1, 0, 0, 0, 284, 252, 1, 0, 0, 0, 284, 256, 1, 0,
		0, 0, 284, 260, 1, 0, 0, 0, 284, 264, 1, 0, 0, 0, 284, 268, 1, 0, 0, 0,
		284, 272, 1, 0, 0, 0, 284, 276, 1, 0, 0, 0, 284, 280, 1, 0, 0, 0, 285,
		288, 1, 0, 0, 0, 286, 284, 1, 0, 0, 0, 286, 287, 1, 0, 0, 0, 287, 23, 1,
		0, 0, 0, 288, 286, 1, 0, 0, 0, 289, 293, 3, 28, 14, 0, 290, 292, 3, 34,
		17, 0, 291, 290, 1, 0, 0, 0, 292, 295, 1, 0, 0, 0, 293, 291, 1, 0, 0, 0,
		293, 294, 1, 0, 0, 0, 294, 25, 1, 0, 0, 0, 295, 293, 1, 0, 0, 0, 296, 298,
		3, 28, 14, 0, 297, 299, 3, 34, 17, 0, 298, 297, 1, 0, 0, 0, 299, 300, 1,
		0, 0, 0, 300, 298, 1, 0, 0, 0, 300, 301, 1, 0, 0, 0, 301, 27, 1, 0, 0,
		0, 302, 308, 3, 30, 15, 0, 303, 304, 5, 45, 0, 0, 304, 305, 3, 22, 11,
		0, 305, 306, 5, 46, 0, 0, 306, 308, 1, 0, 0, 0, 307, 302, 1, 0, 0, 0, 307,
		303, 1, 0, 0, 0, 308, 29, 1, 0, 0, 0, 309, 316, 5, 64, 0, 0, 310, 311,
		5, 45, 0, 0, 311, 312, 3, 22, 11, 0, 312, 313, 5, 46, 0, 0, 313, 314, 3,
		32, 16, 0, 314, 316, 1, 0, 0, 0, 315, 309, 1, 0, 0, 0, 315, 310, 1, 0,
		0, 0, 316, 320, 1, 0, 0, 0, 317, 319, 3, 32, 16, 0, 318, 317, 1, 0, 0,
		0, 319, 322, 1, 0, 0, 0, 320, 318, 1, 0, 0, 0, 320, 321, 1, 0, 0, 0, 321,
		31, 1, 0, 0, 0, 322, 320, 1, 0, 0, 0, 323, 325, 3, 34, 17, 0, 324, 323,
		1, 0, 0, 0, 325, 328, 1, 0, 0, 0, 326, 324, 1, 0, 0, 0, 326, 327, 1, 0,
		0, 0, 327, 335, 1, 0, 0, 0, 328, 326, 1, 0, 0, 0, 329, 330, 5, 43, 0, 0,
		330, 331, 3, 22, 11, 0, 331, 332, 5, 44, 0, 0, 332, 336, 1, 0, 0, 0, 333,
		334, 5, 62, 0, 0, 334, 336, 5, 64, 0, 0, 335, 329, 1, 0, 0, 0, 335, 333,
		1, 0, 0, 0, 336, 33, 1, 0, 0, 0, 337, 338, 5, 41, 0, 0, 338, 340, 5, 64,
		0, 0, 339, 337, 1, 0, 0, 0, 339, 340, 1, 0, 0, 0, 340, 341, 1, 0, 0, 0,
		341, 342, 3, 36, 18, 0, 342, 35, 1, 0, 0, 0, 343, 345, 5, 45, 0, 0, 344,
		346, 3, 20, 10, 0, 345, 344, 1, 0, 0, 0, 345, 346, 1, 0, 0, 0, 346, 347,
		1, 0, 0, 0, 347, 351, 5, 46, 0, 0, 348, 351, 3, 44, 22, 0, 349, 351, 3,
		72, 36, 0, 350, 343, 1, 0, 0, 0, 350, 348, 1, 0, 0, 0, 350, 349, 1, 0,
		0, 0, 351, 37, 1, 0, 0, 0, 352, 353, 5, 1, 0, 0, 353, 354, 3, 40, 20, 0,
		354, 39, 1, 0, 0, 0, 355, 357, 5, 45, 0, 0, 356, 358, 3, 42, 21, 0, 357,
		356, 1, 0, 0, 0, 357, 358, 1, 0, 0, 0, 358, 359, 1, 0, 0, 0, 359, 360,
		5, 46, 0, 0, 360, 361, 3, 2, 1, 0, 361, 362, 5, 13, 0, 0, 362, 41, 1, 0,
		0, 0, 363, 366, 3, 18, 9, 0, 364, 365, 5, 49, 0, 0, 365, 367, 5, 60, 0,
		0, 366, 364, 1, 0, 0, 0, 366, 367, 1, 0, 0, 0, 367, 370, 1, 0, 0, 0, 368,
		370, 5, 60, 0, 0, 369, 363, 1, 0, 0, 0, 369, 368, 1, 0, 0, 0, 370, 43,
		1, 0, 0, 0, 371, 373, 5, 47, 0, 0, 372, 374, 3, 46, 23, 0, 373, 372, 1,
		0, 0, 0, 373, 374, 1, 0, 0, 0, 374, 375, 1, 0, 0, 0, 375, 376, 5, 48, 0,
		0, 376, 45, 1, 0, 0, 0, 377, 383, 3, 48, 24, 0, 378, 379, 3, 50, 25, 0,
		379, 380, 3, 48, 24, 0, 380, 382, 1, 0, 0, 0, 381, 378, 1, 0, 0, 0, 382,
		385, 1, 0, 0, 0, 383, 381, 1, 0, 0, 0, 383, 384, 1, 0, 0, 0, 384, 387,
		1, 0, 0, 0, 385, 383, 1, 0, 0, 0, 386, 388, 3, 50, 25, 0, 387, 386, 1,
		0, 0, 0, 387, 388, 1, 0, 0, 0, 388, 47, 1, 0, 0, 0, 389, 390, 5, 43, 0,
		0, 390, 391, 3, 22, 11, 0, 391, 392, 5, 44, 0, 0, 392, 393, 5, 51, 0, 0,
		393, 394, 3, 22, 11, 0, 394, 400, 1, 0, 0, 0, 395, 396, 5, 64, 0, 0, 396,
		397, 5, 51, 0, 0, 397, 400, 3, 22, 11, 0, 398, 400, 3, 22, 11, 0, 399,
		389, 1, 0, 0, 0, 399, 395, 1, 0, 0, 0, 399, 398, 1, 0, 0, 0, 400, 49, 1,
		0, 0, 0, 401, 402, 7, 0, 0, 0, 402, 51, 1, 0, 0, 0, 403, 404, 5, 22, 0,
		0, 404, 53, 1, 0, 0, 0, 405, 406, 5, 23, 0, 0, 406, 55, 1, 0, 0, 0, 407,
		408, 7, 1, 0, 0, 408, 57, 1, 0, 0, 0, 409, 410, 5, 63, 0, 0, 410, 59, 1,
		0, 0, 0, 411, 412, 7, 2, 0, 0, 412, 61, 1, 0, 0, 0, 413, 414, 7, 3, 0,
		0, 414, 63, 1, 0, 0, 0, 415, 416, 7, 4, 0, 0, 416, 65, 1, 0, 0, 0, 417,
		418, 7, 5, 0, 0, 418, 67, 1, 0, 0, 0, 419, 420, 5, 34, 0, 0, 420, 69, 1,
		0, 0, 0, 421, 422, 7, 6, 0, 0, 422, 71, 1, 0, 0, 0, 423, 424, 7, 7, 0,
		0, 424, 73, 1, 0, 0, 0, 37, 80, 84, 122, 127, 139, 165, 167, 176, 182,
		186, 191, 193, 204, 209, 216, 224, 232, 250, 284, 286, 293, 300, 307, 315,
		320, 326, 335, 339, 345, 350, 357, 366, 369, 373, 383, 387, 399,
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

// LuaParserInit initializes any static state used to implement LuaParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewLuaParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func LuaParserInit() {
	staticData := &luaparserParserStaticData
	staticData.once.Do(luaparserParserInit)
}

// NewLuaParser produces a new parser instance for the optional input antlr.TokenStream.
func NewLuaParser(input antlr.TokenStream) *LuaParser {
	LuaParserInit()
	this := new(LuaParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &luaparserParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.predictionContextCache)
	this.RuleNames = staticData.ruleNames
	this.LiteralNames = staticData.literalNames
	this.SymbolicNames = staticData.symbolicNames
	this.GrammarFileName = "java-escape"

	return this
}

// LuaParser tokens.
const (
	LuaParserEOF          = antlr.TokenEOF
	LuaParserFunction     = 1
	LuaParserNil          = 2
	LuaParserFalse        = 3
	LuaParserTrue         = 4
	LuaParserReturn       = 5
	LuaParserBreak        = 6
	LuaParserContinue     = 7
	LuaParserGoto         = 8
	LuaParserRepeat       = 9
	LuaParserUntil        = 10
	LuaParserWhile        = 11
	LuaParserDo           = 12
	LuaParserEnd          = 13
	LuaParserIf           = 14
	LuaParserThen         = 15
	LuaParserElse         = 16
	LuaParserElseIf       = 17
	LuaParserFor          = 18
	LuaParserIn           = 19
	LuaParserLocal        = 20
	LuaParserNot          = 21
	LuaParserOr           = 22
	LuaParserAnd          = 23
	LuaParserMul          = 24
	LuaParserDiv          = 25
	LuaParserIntegralDiv  = 26
	LuaParserMod          = 27
	LuaParserAmp          = 28
	LuaParserXand         = 29
	LuaParserLtLt         = 30
	LuaParserGtGt         = 31
	LuaParserPlus         = 32
	LuaParserSub          = 33
	LuaParserPower        = 34
	LuaParserLt           = 35
	LuaParserGt           = 36
	LuaParserEq           = 37
	LuaParserLtEq         = 38
	LuaParserGtEq         = 39
	LuaParserNeq          = 40
	LuaParserColon        = 41
	LuaParserDoubleColon  = 42
	LuaParserLBracket     = 43
	LuaParserRBracket     = 44
	LuaParserLParen       = 45
	LuaParserRParen       = 46
	LuaParserLBrace       = 47
	LuaParserRBrace       = 48
	LuaParserComma        = 49
	LuaParserPound        = 50
	LuaParserAssignEq     = 51
	LuaParserPlusPlus     = 52
	LuaParserSubSub       = 53
	LuaParserPlusEq       = 54
	LuaParserMinusEq      = 55
	LuaParserMulEq        = 56
	LuaParserDivEq        = 57
	LuaParserModEq        = 58
	LuaParserSemiColon    = 59
	LuaParserEllipsis     = 60
	LuaParserNotSymbol    = 61
	LuaParserDot          = 62
	LuaParserStrcat       = 63
	LuaParserNAME         = 64
	LuaParserNORMALSTRING = 65
	LuaParserCHARSTRING   = 66
	LuaParserLONGSTRING   = 67
	LuaParserINT          = 68
	LuaParserHEX          = 69
	LuaParserFLOAT        = 70
	LuaParserHEX_FLOAT    = 71
	LuaParserCOMMENT      = 72
	LuaParserLINE_COMMENT = 73
	LuaParserWS           = 74
	LuaParserSHEBANG      = 75
)

// LuaParser rules.
const (
	LuaParserRULE_chunk              = 0
	LuaParserRULE_block              = 1
	LuaParserRULE_stat               = 2
	LuaParserRULE_attnamelist        = 3
	LuaParserRULE_attrib             = 4
	LuaParserRULE_laststat           = 5
	LuaParserRULE_label              = 6
	LuaParserRULE_funcname           = 7
	LuaParserRULE_varlist            = 8
	LuaParserRULE_namelist           = 9
	LuaParserRULE_explist            = 10
	LuaParserRULE_exp                = 11
	LuaParserRULE_prefixexp          = 12
	LuaParserRULE_functioncall       = 13
	LuaParserRULE_varOrExp           = 14
	LuaParserRULE_var                = 15
	LuaParserRULE_varSuffix          = 16
	LuaParserRULE_nameAndArgs        = 17
	LuaParserRULE_args               = 18
	LuaParserRULE_functiondef        = 19
	LuaParserRULE_funcbody           = 20
	LuaParserRULE_parlist            = 21
	LuaParserRULE_tableconstructor   = 22
	LuaParserRULE_fieldlist          = 23
	LuaParserRULE_field              = 24
	LuaParserRULE_fieldsep           = 25
	LuaParserRULE_operatorOr         = 26
	LuaParserRULE_operatorAnd        = 27
	LuaParserRULE_operatorComparison = 28
	LuaParserRULE_operatorStrcat     = 29
	LuaParserRULE_operatorAddSub     = 30
	LuaParserRULE_operatorMulDivMod  = 31
	LuaParserRULE_operatorBitwise    = 32
	LuaParserRULE_operatorUnary      = 33
	LuaParserRULE_operatorPower      = 34
	LuaParserRULE_number             = 35
	LuaParserRULE_string             = 36
)

// IChunkContext is an interface to support dynamic dispatch.
type IChunkContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsChunkContext differentiates from other interfaces.
	IsChunkContext()
}

type ChunkContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyChunkContext() *ChunkContext {
	var p = new(ChunkContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_chunk
	return p
}

func (*ChunkContext) IsChunkContext() {}

func NewChunkContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ChunkContext {
	var p = new(ChunkContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_chunk

	return p
}

func (s *ChunkContext) GetParser() antlr.Parser { return s.parser }

func (s *ChunkContext) Block() IBlockContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBlockContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBlockContext)
}

func (s *ChunkContext) EOF() antlr.TerminalNode {
	return s.GetToken(LuaParserEOF, 0)
}

func (s *ChunkContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ChunkContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ChunkContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitChunk(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Chunk() (localctx IChunkContext) {
	this := p
	_ = this

	localctx = NewChunkContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, LuaParserRULE_chunk)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(74)
		p.Block()
	}
	{
		p.SetState(75)
		p.Match(LuaParserEOF)
	}

	return localctx
}

// IBlockContext is an interface to support dynamic dispatch.
type IBlockContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsBlockContext differentiates from other interfaces.
	IsBlockContext()
}

type BlockContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyBlockContext() *BlockContext {
	var p = new(BlockContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_block
	return p
}

func (*BlockContext) IsBlockContext() {}

func NewBlockContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *BlockContext {
	var p = new(BlockContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_block

	return p
}

func (s *BlockContext) GetParser() antlr.Parser { return s.parser }

func (s *BlockContext) AllStat() []IStatContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStatContext); ok {
			len++
		}
	}

	tst := make([]IStatContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStatContext); ok {
			tst[i] = t.(IStatContext)
			i++
		}
	}

	return tst
}

func (s *BlockContext) Stat(i int) IStatContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatContext)
}

func (s *BlockContext) Laststat() ILaststatContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILaststatContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILaststatContext)
}

func (s *BlockContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BlockContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *BlockContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitBlock(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Block() (localctx IBlockContext) {
	this := p
	_ = this

	localctx = NewBlockContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, LuaParserRULE_block)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(80)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 0, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(77)
				p.Stat()
			}

		}
		p.SetState(82)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 0, p.GetParserRuleContext())
	}
	p.SetState(84)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&224) != 0 {
		{
			p.SetState(83)
			p.Laststat()
		}

	}

	return localctx
}

// IStatContext is an interface to support dynamic dispatch.
type IStatContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStatContext differentiates from other interfaces.
	IsStatContext()
}

type StatContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStatContext() *StatContext {
	var p = new(StatContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_stat
	return p
}

func (*StatContext) IsStatContext() {}

func NewStatContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StatContext {
	var p = new(StatContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_stat

	return p
}

func (s *StatContext) GetParser() antlr.Parser { return s.parser }

func (s *StatContext) SemiColon() antlr.TerminalNode {
	return s.GetToken(LuaParserSemiColon, 0)
}

func (s *StatContext) Varlist() IVarlistContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IVarlistContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IVarlistContext)
}

func (s *StatContext) AssignEq() antlr.TerminalNode {
	return s.GetToken(LuaParserAssignEq, 0)
}

func (s *StatContext) Explist() IExplistContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExplistContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExplistContext)
}

func (s *StatContext) Functioncall() IFunctioncallContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFunctioncallContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFunctioncallContext)
}

func (s *StatContext) Label() ILabelContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILabelContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILabelContext)
}

func (s *StatContext) Break() antlr.TerminalNode {
	return s.GetToken(LuaParserBreak, 0)
}

func (s *StatContext) Goto() antlr.TerminalNode {
	return s.GetToken(LuaParserGoto, 0)
}

func (s *StatContext) NAME() antlr.TerminalNode {
	return s.GetToken(LuaParserNAME, 0)
}

func (s *StatContext) Do() antlr.TerminalNode {
	return s.GetToken(LuaParserDo, 0)
}

func (s *StatContext) AllBlock() []IBlockContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IBlockContext); ok {
			len++
		}
	}

	tst := make([]IBlockContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IBlockContext); ok {
			tst[i] = t.(IBlockContext)
			i++
		}
	}

	return tst
}

func (s *StatContext) Block(i int) IBlockContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBlockContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBlockContext)
}

func (s *StatContext) End() antlr.TerminalNode {
	return s.GetToken(LuaParserEnd, 0)
}

func (s *StatContext) While() antlr.TerminalNode {
	return s.GetToken(LuaParserWhile, 0)
}

func (s *StatContext) AllExp() []IExpContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExpContext); ok {
			len++
		}
	}

	tst := make([]IExpContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExpContext); ok {
			tst[i] = t.(IExpContext)
			i++
		}
	}

	return tst
}

func (s *StatContext) Exp(i int) IExpContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExpContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExpContext)
}

func (s *StatContext) Repeat() antlr.TerminalNode {
	return s.GetToken(LuaParserRepeat, 0)
}

func (s *StatContext) Until() antlr.TerminalNode {
	return s.GetToken(LuaParserUntil, 0)
}

func (s *StatContext) If() antlr.TerminalNode {
	return s.GetToken(LuaParserIf, 0)
}

func (s *StatContext) AllThen() []antlr.TerminalNode {
	return s.GetTokens(LuaParserThen)
}

func (s *StatContext) Then(i int) antlr.TerminalNode {
	return s.GetToken(LuaParserThen, i)
}

func (s *StatContext) AllElseIf() []antlr.TerminalNode {
	return s.GetTokens(LuaParserElseIf)
}

func (s *StatContext) ElseIf(i int) antlr.TerminalNode {
	return s.GetToken(LuaParserElseIf, i)
}

func (s *StatContext) Else() antlr.TerminalNode {
	return s.GetToken(LuaParserElse, 0)
}

func (s *StatContext) For() antlr.TerminalNode {
	return s.GetToken(LuaParserFor, 0)
}

func (s *StatContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(LuaParserComma)
}

func (s *StatContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(LuaParserComma, i)
}

func (s *StatContext) Namelist() INamelistContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INamelistContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INamelistContext)
}

func (s *StatContext) In() antlr.TerminalNode {
	return s.GetToken(LuaParserIn, 0)
}

func (s *StatContext) Function() antlr.TerminalNode {
	return s.GetToken(LuaParserFunction, 0)
}

func (s *StatContext) Funcname() IFuncnameContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFuncnameContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFuncnameContext)
}

func (s *StatContext) Funcbody() IFuncbodyContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFuncbodyContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFuncbodyContext)
}

func (s *StatContext) Local() antlr.TerminalNode {
	return s.GetToken(LuaParserLocal, 0)
}

func (s *StatContext) Attnamelist() IAttnamelistContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAttnamelistContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAttnamelistContext)
}

func (s *StatContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StatContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StatContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitStat(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Stat() (localctx IStatContext) {
	this := p
	_ = this

	localctx = NewStatContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, LuaParserRULE_stat)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(167)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 6, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(86)
			p.Match(LuaParserSemiColon)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(87)
			p.Varlist()
		}
		{
			p.SetState(88)
			p.Match(LuaParserAssignEq)
		}
		{
			p.SetState(89)
			p.Explist()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(91)
			p.Functioncall()
		}

	case 4:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(92)
			p.Label()
		}

	case 5:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(93)
			p.Match(LuaParserBreak)
		}

	case 6:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(94)
			p.Match(LuaParserGoto)
		}
		{
			p.SetState(95)
			p.Match(LuaParserNAME)
		}

	case 7:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(96)
			p.Match(LuaParserDo)
		}
		{
			p.SetState(97)
			p.Block()
		}
		{
			p.SetState(98)
			p.Match(LuaParserEnd)
		}

	case 8:
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(100)
			p.Match(LuaParserWhile)
		}
		{
			p.SetState(101)
			p.exp(0)
		}
		{
			p.SetState(102)
			p.Match(LuaParserDo)
		}
		{
			p.SetState(103)
			p.Block()
		}
		{
			p.SetState(104)
			p.Match(LuaParserEnd)
		}

	case 9:
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(106)
			p.Match(LuaParserRepeat)
		}
		{
			p.SetState(107)
			p.Block()
		}
		{
			p.SetState(108)
			p.Match(LuaParserUntil)
		}
		{
			p.SetState(109)
			p.exp(0)
		}

	case 10:
		p.EnterOuterAlt(localctx, 10)
		{
			p.SetState(111)
			p.Match(LuaParserIf)
		}
		{
			p.SetState(112)
			p.exp(0)
		}
		{
			p.SetState(113)
			p.Match(LuaParserThen)
		}
		{
			p.SetState(114)
			p.Block()
		}
		p.SetState(122)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == LuaParserElseIf {
			{
				p.SetState(115)
				p.Match(LuaParserElseIf)
			}
			{
				p.SetState(116)
				p.exp(0)
			}
			{
				p.SetState(117)
				p.Match(LuaParserThen)
			}
			{
				p.SetState(118)
				p.Block()
			}

			p.SetState(124)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		p.SetState(127)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == LuaParserElse {
			{
				p.SetState(125)
				p.Match(LuaParserElse)
			}
			{
				p.SetState(126)
				p.Block()
			}

		}
		{
			p.SetState(129)
			p.Match(LuaParserEnd)
		}

	case 11:
		p.EnterOuterAlt(localctx, 11)
		{
			p.SetState(131)
			p.Match(LuaParserFor)
		}
		{
			p.SetState(132)
			p.Match(LuaParserNAME)
		}
		{
			p.SetState(133)
			p.Match(LuaParserAssignEq)
		}
		{
			p.SetState(134)
			p.exp(0)
		}
		{
			p.SetState(135)
			p.Match(LuaParserComma)
		}
		{
			p.SetState(136)
			p.exp(0)
		}
		p.SetState(139)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == LuaParserComma {
			{
				p.SetState(137)
				p.Match(LuaParserComma)
			}
			{
				p.SetState(138)
				p.exp(0)
			}

		}
		{
			p.SetState(141)
			p.Match(LuaParserDo)
		}
		{
			p.SetState(142)
			p.Block()
		}
		{
			p.SetState(143)
			p.Match(LuaParserEnd)
		}

	case 12:
		p.EnterOuterAlt(localctx, 12)
		{
			p.SetState(145)
			p.Match(LuaParserFor)
		}
		{
			p.SetState(146)
			p.Namelist()
		}
		{
			p.SetState(147)
			p.Match(LuaParserIn)
		}
		{
			p.SetState(148)
			p.Explist()
		}
		{
			p.SetState(149)
			p.Match(LuaParserDo)
		}
		{
			p.SetState(150)
			p.Block()
		}
		{
			p.SetState(151)
			p.Match(LuaParserEnd)
		}

	case 13:
		p.EnterOuterAlt(localctx, 13)
		{
			p.SetState(153)
			p.Match(LuaParserFunction)
		}
		{
			p.SetState(154)
			p.Funcname()
		}
		{
			p.SetState(155)
			p.Funcbody()
		}

	case 14:
		p.EnterOuterAlt(localctx, 14)
		{
			p.SetState(157)
			p.Match(LuaParserLocal)
		}
		{
			p.SetState(158)
			p.Match(LuaParserFunction)
		}
		{
			p.SetState(159)
			p.Match(LuaParserNAME)
		}
		{
			p.SetState(160)
			p.Funcbody()
		}

	case 15:
		p.EnterOuterAlt(localctx, 15)
		{
			p.SetState(161)
			p.Match(LuaParserLocal)
		}
		{
			p.SetState(162)
			p.Attnamelist()
		}
		p.SetState(165)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == LuaParserAssignEq {
			{
				p.SetState(163)
				p.Match(LuaParserAssignEq)
			}
			{
				p.SetState(164)
				p.Explist()
			}

		}

	}

	return localctx
}

// IAttnamelistContext is an interface to support dynamic dispatch.
type IAttnamelistContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsAttnamelistContext differentiates from other interfaces.
	IsAttnamelistContext()
}

type AttnamelistContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAttnamelistContext() *AttnamelistContext {
	var p = new(AttnamelistContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_attnamelist
	return p
}

func (*AttnamelistContext) IsAttnamelistContext() {}

func NewAttnamelistContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AttnamelistContext {
	var p = new(AttnamelistContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_attnamelist

	return p
}

func (s *AttnamelistContext) GetParser() antlr.Parser { return s.parser }

func (s *AttnamelistContext) AllNAME() []antlr.TerminalNode {
	return s.GetTokens(LuaParserNAME)
}

func (s *AttnamelistContext) NAME(i int) antlr.TerminalNode {
	return s.GetToken(LuaParserNAME, i)
}

func (s *AttnamelistContext) AllAttrib() []IAttribContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IAttribContext); ok {
			len++
		}
	}

	tst := make([]IAttribContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IAttribContext); ok {
			tst[i] = t.(IAttribContext)
			i++
		}
	}

	return tst
}

func (s *AttnamelistContext) Attrib(i int) IAttribContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAttribContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAttribContext)
}

func (s *AttnamelistContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(LuaParserComma)
}

func (s *AttnamelistContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(LuaParserComma, i)
}

func (s *AttnamelistContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AttnamelistContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AttnamelistContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitAttnamelist(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Attnamelist() (localctx IAttnamelistContext) {
	this := p
	_ = this

	localctx = NewAttnamelistContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, LuaParserRULE_attnamelist)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(169)
		p.Match(LuaParserNAME)
	}
	{
		p.SetState(170)
		p.Attrib()
	}
	p.SetState(176)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == LuaParserComma {
		{
			p.SetState(171)
			p.Match(LuaParserComma)
		}
		{
			p.SetState(172)
			p.Match(LuaParserNAME)
		}
		{
			p.SetState(173)
			p.Attrib()
		}

		p.SetState(178)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// IAttribContext is an interface to support dynamic dispatch.
type IAttribContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsAttribContext differentiates from other interfaces.
	IsAttribContext()
}

type AttribContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAttribContext() *AttribContext {
	var p = new(AttribContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_attrib
	return p
}

func (*AttribContext) IsAttribContext() {}

func NewAttribContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AttribContext {
	var p = new(AttribContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_attrib

	return p
}

func (s *AttribContext) GetParser() antlr.Parser { return s.parser }

func (s *AttribContext) Lt() antlr.TerminalNode {
	return s.GetToken(LuaParserLt, 0)
}

func (s *AttribContext) NAME() antlr.TerminalNode {
	return s.GetToken(LuaParserNAME, 0)
}

func (s *AttribContext) Gt() antlr.TerminalNode {
	return s.GetToken(LuaParserGt, 0)
}

func (s *AttribContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AttribContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AttribContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitAttrib(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Attrib() (localctx IAttribContext) {
	this := p
	_ = this

	localctx = NewAttribContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, LuaParserRULE_attrib)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	p.SetState(182)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == LuaParserLt {
		{
			p.SetState(179)
			p.Match(LuaParserLt)
		}
		{
			p.SetState(180)
			p.Match(LuaParserNAME)
		}
		{
			p.SetState(181)
			p.Match(LuaParserGt)
		}

	}

	return localctx
}

// ILaststatContext is an interface to support dynamic dispatch.
type ILaststatContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsLaststatContext differentiates from other interfaces.
	IsLaststatContext()
}

type LaststatContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyLaststatContext() *LaststatContext {
	var p = new(LaststatContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_laststat
	return p
}

func (*LaststatContext) IsLaststatContext() {}

func NewLaststatContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *LaststatContext {
	var p = new(LaststatContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_laststat

	return p
}

func (s *LaststatContext) GetParser() antlr.Parser { return s.parser }

func (s *LaststatContext) Return() antlr.TerminalNode {
	return s.GetToken(LuaParserReturn, 0)
}

func (s *LaststatContext) Explist() IExplistContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExplistContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExplistContext)
}

func (s *LaststatContext) Break() antlr.TerminalNode {
	return s.GetToken(LuaParserBreak, 0)
}

func (s *LaststatContext) Continue() antlr.TerminalNode {
	return s.GetToken(LuaParserContinue, 0)
}

func (s *LaststatContext) SemiColon() antlr.TerminalNode {
	return s.GetToken(LuaParserSemiColon, 0)
}

func (s *LaststatContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LaststatContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *LaststatContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitLaststat(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Laststat() (localctx ILaststatContext) {
	this := p
	_ = this

	localctx = NewLaststatContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, LuaParserRULE_laststat)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(193)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case LuaParserReturn:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(184)
			p.Match(LuaParserReturn)
		}
		p.SetState(186)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&3460066344179859486) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&255) != 0 {
			{
				p.SetState(185)
				p.Explist()
			}

		}

	case LuaParserBreak:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(188)
			p.Match(LuaParserBreak)
		}

	case LuaParserContinue:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(189)
			p.Match(LuaParserContinue)
		}
		p.SetState(191)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == LuaParserSemiColon {
			{
				p.SetState(190)
				p.Match(LuaParserSemiColon)
			}

		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// ILabelContext is an interface to support dynamic dispatch.
type ILabelContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsLabelContext differentiates from other interfaces.
	IsLabelContext()
}

type LabelContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyLabelContext() *LabelContext {
	var p = new(LabelContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_label
	return p
}

func (*LabelContext) IsLabelContext() {}

func NewLabelContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *LabelContext {
	var p = new(LabelContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_label

	return p
}

func (s *LabelContext) GetParser() antlr.Parser { return s.parser }

func (s *LabelContext) AllDoubleColon() []antlr.TerminalNode {
	return s.GetTokens(LuaParserDoubleColon)
}

func (s *LabelContext) DoubleColon(i int) antlr.TerminalNode {
	return s.GetToken(LuaParserDoubleColon, i)
}

func (s *LabelContext) NAME() antlr.TerminalNode {
	return s.GetToken(LuaParserNAME, 0)
}

func (s *LabelContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LabelContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *LabelContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitLabel(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Label() (localctx ILabelContext) {
	this := p
	_ = this

	localctx = NewLabelContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, LuaParserRULE_label)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(195)
		p.Match(LuaParserDoubleColon)
	}
	{
		p.SetState(196)
		p.Match(LuaParserNAME)
	}
	{
		p.SetState(197)
		p.Match(LuaParserDoubleColon)
	}

	return localctx
}

// IFuncnameContext is an interface to support dynamic dispatch.
type IFuncnameContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFuncnameContext differentiates from other interfaces.
	IsFuncnameContext()
}

type FuncnameContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFuncnameContext() *FuncnameContext {
	var p = new(FuncnameContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_funcname
	return p
}

func (*FuncnameContext) IsFuncnameContext() {}

func NewFuncnameContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FuncnameContext {
	var p = new(FuncnameContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_funcname

	return p
}

func (s *FuncnameContext) GetParser() antlr.Parser { return s.parser }

func (s *FuncnameContext) AllNAME() []antlr.TerminalNode {
	return s.GetTokens(LuaParserNAME)
}

func (s *FuncnameContext) NAME(i int) antlr.TerminalNode {
	return s.GetToken(LuaParserNAME, i)
}

func (s *FuncnameContext) AllDot() []antlr.TerminalNode {
	return s.GetTokens(LuaParserDot)
}

func (s *FuncnameContext) Dot(i int) antlr.TerminalNode {
	return s.GetToken(LuaParserDot, i)
}

func (s *FuncnameContext) Colon() antlr.TerminalNode {
	return s.GetToken(LuaParserColon, 0)
}

func (s *FuncnameContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FuncnameContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FuncnameContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitFuncname(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Funcname() (localctx IFuncnameContext) {
	this := p
	_ = this

	localctx = NewFuncnameContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 14, LuaParserRULE_funcname)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(199)
		p.Match(LuaParserNAME)
	}
	p.SetState(204)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == LuaParserDot {
		{
			p.SetState(200)
			p.Match(LuaParserDot)
		}
		{
			p.SetState(201)
			p.Match(LuaParserNAME)
		}

		p.SetState(206)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	p.SetState(209)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == LuaParserColon {
		{
			p.SetState(207)
			p.Match(LuaParserColon)
		}
		{
			p.SetState(208)
			p.Match(LuaParserNAME)
		}

	}

	return localctx
}

// IVarlistContext is an interface to support dynamic dispatch.
type IVarlistContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsVarlistContext differentiates from other interfaces.
	IsVarlistContext()
}

type VarlistContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyVarlistContext() *VarlistContext {
	var p = new(VarlistContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_varlist
	return p
}

func (*VarlistContext) IsVarlistContext() {}

func NewVarlistContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *VarlistContext {
	var p = new(VarlistContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_varlist

	return p
}

func (s *VarlistContext) GetParser() antlr.Parser { return s.parser }

func (s *VarlistContext) AllVar_() []IVarContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IVarContext); ok {
			len++
		}
	}

	tst := make([]IVarContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IVarContext); ok {
			tst[i] = t.(IVarContext)
			i++
		}
	}

	return tst
}

func (s *VarlistContext) Var_(i int) IVarContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IVarContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IVarContext)
}

func (s *VarlistContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(LuaParserComma)
}

func (s *VarlistContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(LuaParserComma, i)
}

func (s *VarlistContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *VarlistContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *VarlistContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitVarlist(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Varlist() (localctx IVarlistContext) {
	this := p
	_ = this

	localctx = NewVarlistContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 16, LuaParserRULE_varlist)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(211)
		p.Var_()
	}
	p.SetState(216)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == LuaParserComma {
		{
			p.SetState(212)
			p.Match(LuaParserComma)
		}
		{
			p.SetState(213)
			p.Var_()
		}

		p.SetState(218)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// INamelistContext is an interface to support dynamic dispatch.
type INamelistContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNamelistContext differentiates from other interfaces.
	IsNamelistContext()
}

type NamelistContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNamelistContext() *NamelistContext {
	var p = new(NamelistContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_namelist
	return p
}

func (*NamelistContext) IsNamelistContext() {}

func NewNamelistContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NamelistContext {
	var p = new(NamelistContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_namelist

	return p
}

func (s *NamelistContext) GetParser() antlr.Parser { return s.parser }

func (s *NamelistContext) AllNAME() []antlr.TerminalNode {
	return s.GetTokens(LuaParserNAME)
}

func (s *NamelistContext) NAME(i int) antlr.TerminalNode {
	return s.GetToken(LuaParserNAME, i)
}

func (s *NamelistContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(LuaParserComma)
}

func (s *NamelistContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(LuaParserComma, i)
}

func (s *NamelistContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NamelistContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NamelistContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitNamelist(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Namelist() (localctx INamelistContext) {
	this := p
	_ = this

	localctx = NewNamelistContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 18, LuaParserRULE_namelist)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(219)
		p.Match(LuaParserNAME)
	}
	p.SetState(224)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 15, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(220)
				p.Match(LuaParserComma)
			}
			{
				p.SetState(221)
				p.Match(LuaParserNAME)
			}

		}
		p.SetState(226)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 15, p.GetParserRuleContext())
	}

	return localctx
}

// IExplistContext is an interface to support dynamic dispatch.
type IExplistContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsExplistContext differentiates from other interfaces.
	IsExplistContext()
}

type ExplistContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyExplistContext() *ExplistContext {
	var p = new(ExplistContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_explist
	return p
}

func (*ExplistContext) IsExplistContext() {}

func NewExplistContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ExplistContext {
	var p = new(ExplistContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_explist

	return p
}

func (s *ExplistContext) GetParser() antlr.Parser { return s.parser }

func (s *ExplistContext) AllExp() []IExpContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExpContext); ok {
			len++
		}
	}

	tst := make([]IExpContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExpContext); ok {
			tst[i] = t.(IExpContext)
			i++
		}
	}

	return tst
}

func (s *ExplistContext) Exp(i int) IExpContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExpContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExpContext)
}

func (s *ExplistContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(LuaParserComma)
}

func (s *ExplistContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(LuaParserComma, i)
}

func (s *ExplistContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExplistContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ExplistContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitExplist(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Explist() (localctx IExplistContext) {
	this := p
	_ = this

	localctx = NewExplistContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 20, LuaParserRULE_explist)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(232)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 16, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(227)
				p.exp(0)
			}
			{
				p.SetState(228)
				p.Match(LuaParserComma)
			}

		}
		p.SetState(234)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 16, p.GetParserRuleContext())
	}
	{
		p.SetState(235)
		p.exp(0)
	}

	return localctx
}

// IExpContext is an interface to support dynamic dispatch.
type IExpContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsExpContext differentiates from other interfaces.
	IsExpContext()
}

type ExpContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyExpContext() *ExpContext {
	var p = new(ExpContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_exp
	return p
}

func (*ExpContext) IsExpContext() {}

func NewExpContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ExpContext {
	var p = new(ExpContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_exp

	return p
}

func (s *ExpContext) GetParser() antlr.Parser { return s.parser }

func (s *ExpContext) Nil() antlr.TerminalNode {
	return s.GetToken(LuaParserNil, 0)
}

func (s *ExpContext) False() antlr.TerminalNode {
	return s.GetToken(LuaParserFalse, 0)
}

func (s *ExpContext) True() antlr.TerminalNode {
	return s.GetToken(LuaParserTrue, 0)
}

func (s *ExpContext) Number() INumberContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INumberContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INumberContext)
}

func (s *ExpContext) String_() IStringContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStringContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStringContext)
}

func (s *ExpContext) Ellipsis() antlr.TerminalNode {
	return s.GetToken(LuaParserEllipsis, 0)
}

func (s *ExpContext) Functiondef() IFunctiondefContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFunctiondefContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFunctiondefContext)
}

func (s *ExpContext) Prefixexp() IPrefixexpContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IPrefixexpContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IPrefixexpContext)
}

func (s *ExpContext) Tableconstructor() ITableconstructorContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITableconstructorContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ITableconstructorContext)
}

func (s *ExpContext) OperatorUnary() IOperatorUnaryContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperatorUnaryContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperatorUnaryContext)
}

func (s *ExpContext) AllExp() []IExpContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExpContext); ok {
			len++
		}
	}

	tst := make([]IExpContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExpContext); ok {
			tst[i] = t.(IExpContext)
			i++
		}
	}

	return tst
}

func (s *ExpContext) Exp(i int) IExpContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExpContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExpContext)
}

func (s *ExpContext) OperatorPower() IOperatorPowerContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperatorPowerContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperatorPowerContext)
}

func (s *ExpContext) OperatorMulDivMod() IOperatorMulDivModContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperatorMulDivModContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperatorMulDivModContext)
}

func (s *ExpContext) OperatorAddSub() IOperatorAddSubContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperatorAddSubContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperatorAddSubContext)
}

func (s *ExpContext) OperatorStrcat() IOperatorStrcatContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperatorStrcatContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperatorStrcatContext)
}

func (s *ExpContext) OperatorComparison() IOperatorComparisonContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperatorComparisonContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperatorComparisonContext)
}

func (s *ExpContext) OperatorAnd() IOperatorAndContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperatorAndContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperatorAndContext)
}

func (s *ExpContext) OperatorOr() IOperatorOrContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperatorOrContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperatorOrContext)
}

func (s *ExpContext) OperatorBitwise() IOperatorBitwiseContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperatorBitwiseContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperatorBitwiseContext)
}

func (s *ExpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExpContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ExpContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitExp(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Exp() (localctx IExpContext) {
	return p.exp(0)
}

func (p *LuaParser) exp(_p int) (localctx IExpContext) {
	this := p
	_ = this

	var _parentctx antlr.ParserRuleContext = p.GetParserRuleContext()
	_parentState := p.GetState()
	localctx = NewExpContext(p, p.GetParserRuleContext(), _parentState)
	var _prevctx IExpContext = localctx
	var _ antlr.ParserRuleContext = _prevctx // TODO: To prevent unused variable warning.
	_startState := 22
	p.EnterRecursionRule(localctx, 22, LuaParserRULE_exp, _p)

	defer func() {
		p.UnrollRecursionContexts(_parentctx)
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(250)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case LuaParserNil:
		{
			p.SetState(238)
			p.Match(LuaParserNil)
		}

	case LuaParserFalse:
		{
			p.SetState(239)
			p.Match(LuaParserFalse)
		}

	case LuaParserTrue:
		{
			p.SetState(240)
			p.Match(LuaParserTrue)
		}

	case LuaParserINT, LuaParserHEX, LuaParserFLOAT, LuaParserHEX_FLOAT:
		{
			p.SetState(241)
			p.Number()
		}

	case LuaParserNORMALSTRING, LuaParserCHARSTRING, LuaParserLONGSTRING:
		{
			p.SetState(242)
			p.String_()
		}

	case LuaParserEllipsis:
		{
			p.SetState(243)
			p.Match(LuaParserEllipsis)
		}

	case LuaParserFunction:
		{
			p.SetState(244)
			p.Functiondef()
		}

	case LuaParserLParen, LuaParserNAME:
		{
			p.SetState(245)
			p.Prefixexp()
		}

	case LuaParserLBrace:
		{
			p.SetState(246)
			p.Tableconstructor()
		}

	case LuaParserNot, LuaParserSub, LuaParserPound, LuaParserNotSymbol:
		{
			p.SetState(247)
			p.OperatorUnary()
		}
		{
			p.SetState(248)
			p.exp(8)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}
	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(286)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 19, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			p.SetState(284)
			p.GetErrorHandler().Sync(p)
			switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 18, p.GetParserRuleContext()) {
			case 1:
				localctx = NewExpContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, LuaParserRULE_exp)
				p.SetState(252)

				if !(p.Precpred(p.GetParserRuleContext(), 9)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 9)", ""))
				}
				{
					p.SetState(253)
					p.OperatorPower()
				}
				{
					p.SetState(254)
					p.exp(9)
				}

			case 2:
				localctx = NewExpContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, LuaParserRULE_exp)
				p.SetState(256)

				if !(p.Precpred(p.GetParserRuleContext(), 7)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 7)", ""))
				}
				{
					p.SetState(257)
					p.OperatorMulDivMod()
				}
				{
					p.SetState(258)
					p.exp(8)
				}

			case 3:
				localctx = NewExpContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, LuaParserRULE_exp)
				p.SetState(260)

				if !(p.Precpred(p.GetParserRuleContext(), 6)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 6)", ""))
				}
				{
					p.SetState(261)
					p.OperatorAddSub()
				}
				{
					p.SetState(262)
					p.exp(7)
				}

			case 4:
				localctx = NewExpContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, LuaParserRULE_exp)
				p.SetState(264)

				if !(p.Precpred(p.GetParserRuleContext(), 5)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 5)", ""))
				}
				{
					p.SetState(265)
					p.OperatorStrcat()
				}
				{
					p.SetState(266)
					p.exp(5)
				}

			case 5:
				localctx = NewExpContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, LuaParserRULE_exp)
				p.SetState(268)

				if !(p.Precpred(p.GetParserRuleContext(), 4)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 4)", ""))
				}
				{
					p.SetState(269)
					p.OperatorComparison()
				}
				{
					p.SetState(270)
					p.exp(5)
				}

			case 6:
				localctx = NewExpContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, LuaParserRULE_exp)
				p.SetState(272)

				if !(p.Precpred(p.GetParserRuleContext(), 3)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 3)", ""))
				}
				{
					p.SetState(273)
					p.OperatorAnd()
				}
				{
					p.SetState(274)
					p.exp(4)
				}

			case 7:
				localctx = NewExpContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, LuaParserRULE_exp)
				p.SetState(276)

				if !(p.Precpred(p.GetParserRuleContext(), 2)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 2)", ""))
				}
				{
					p.SetState(277)
					p.OperatorOr()
				}
				{
					p.SetState(278)
					p.exp(3)
				}

			case 8:
				localctx = NewExpContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, LuaParserRULE_exp)
				p.SetState(280)

				if !(p.Precpred(p.GetParserRuleContext(), 1)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 1)", ""))
				}
				{
					p.SetState(281)
					p.OperatorBitwise()
				}
				{
					p.SetState(282)
					p.exp(2)
				}

			}

		}
		p.SetState(288)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 19, p.GetParserRuleContext())
	}

	return localctx
}

// IPrefixexpContext is an interface to support dynamic dispatch.
type IPrefixexpContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsPrefixexpContext differentiates from other interfaces.
	IsPrefixexpContext()
}

type PrefixexpContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyPrefixexpContext() *PrefixexpContext {
	var p = new(PrefixexpContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_prefixexp
	return p
}

func (*PrefixexpContext) IsPrefixexpContext() {}

func NewPrefixexpContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *PrefixexpContext {
	var p = new(PrefixexpContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_prefixexp

	return p
}

func (s *PrefixexpContext) GetParser() antlr.Parser { return s.parser }

func (s *PrefixexpContext) VarOrExp() IVarOrExpContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IVarOrExpContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IVarOrExpContext)
}

func (s *PrefixexpContext) AllNameAndArgs() []INameAndArgsContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(INameAndArgsContext); ok {
			len++
		}
	}

	tst := make([]INameAndArgsContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(INameAndArgsContext); ok {
			tst[i] = t.(INameAndArgsContext)
			i++
		}
	}

	return tst
}

func (s *PrefixexpContext) NameAndArgs(i int) INameAndArgsContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INameAndArgsContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(INameAndArgsContext)
}

func (s *PrefixexpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PrefixexpContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *PrefixexpContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitPrefixexp(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Prefixexp() (localctx IPrefixexpContext) {
	this := p
	_ = this

	localctx = NewPrefixexpContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, LuaParserRULE_prefixexp)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(289)
		p.VarOrExp()
	}
	p.SetState(293)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 20, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(290)
				p.NameAndArgs()
			}

		}
		p.SetState(295)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 20, p.GetParserRuleContext())
	}

	return localctx
}

// IFunctioncallContext is an interface to support dynamic dispatch.
type IFunctioncallContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFunctioncallContext differentiates from other interfaces.
	IsFunctioncallContext()
}

type FunctioncallContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFunctioncallContext() *FunctioncallContext {
	var p = new(FunctioncallContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_functioncall
	return p
}

func (*FunctioncallContext) IsFunctioncallContext() {}

func NewFunctioncallContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FunctioncallContext {
	var p = new(FunctioncallContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_functioncall

	return p
}

func (s *FunctioncallContext) GetParser() antlr.Parser { return s.parser }

func (s *FunctioncallContext) VarOrExp() IVarOrExpContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IVarOrExpContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IVarOrExpContext)
}

func (s *FunctioncallContext) AllNameAndArgs() []INameAndArgsContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(INameAndArgsContext); ok {
			len++
		}
	}

	tst := make([]INameAndArgsContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(INameAndArgsContext); ok {
			tst[i] = t.(INameAndArgsContext)
			i++
		}
	}

	return tst
}

func (s *FunctioncallContext) NameAndArgs(i int) INameAndArgsContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INameAndArgsContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(INameAndArgsContext)
}

func (s *FunctioncallContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FunctioncallContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FunctioncallContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitFunctioncall(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Functioncall() (localctx IFunctioncallContext) {
	this := p
	_ = this

	localctx = NewFunctioncallContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, LuaParserRULE_functioncall)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(296)
		p.VarOrExp()
	}
	p.SetState(298)
	p.GetErrorHandler().Sync(p)
	_alt = 1
	for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		switch _alt {
		case 1:
			{
				p.SetState(297)
				p.NameAndArgs()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

		p.SetState(300)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 21, p.GetParserRuleContext())
	}

	return localctx
}

// IVarOrExpContext is an interface to support dynamic dispatch.
type IVarOrExpContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsVarOrExpContext differentiates from other interfaces.
	IsVarOrExpContext()
}

type VarOrExpContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyVarOrExpContext() *VarOrExpContext {
	var p = new(VarOrExpContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_varOrExp
	return p
}

func (*VarOrExpContext) IsVarOrExpContext() {}

func NewVarOrExpContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *VarOrExpContext {
	var p = new(VarOrExpContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_varOrExp

	return p
}

func (s *VarOrExpContext) GetParser() antlr.Parser { return s.parser }

func (s *VarOrExpContext) Var_() IVarContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IVarContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IVarContext)
}

func (s *VarOrExpContext) LParen() antlr.TerminalNode {
	return s.GetToken(LuaParserLParen, 0)
}

func (s *VarOrExpContext) Exp() IExpContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExpContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExpContext)
}

func (s *VarOrExpContext) RParen() antlr.TerminalNode {
	return s.GetToken(LuaParserRParen, 0)
}

func (s *VarOrExpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *VarOrExpContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *VarOrExpContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitVarOrExp(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) VarOrExp() (localctx IVarOrExpContext) {
	this := p
	_ = this

	localctx = NewVarOrExpContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 28, LuaParserRULE_varOrExp)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(307)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 22, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(302)
			p.Var_()
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(303)
			p.Match(LuaParserLParen)
		}
		{
			p.SetState(304)
			p.exp(0)
		}
		{
			p.SetState(305)
			p.Match(LuaParserRParen)
		}

	}

	return localctx
}

// IVarContext is an interface to support dynamic dispatch.
type IVarContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsVarContext differentiates from other interfaces.
	IsVarContext()
}

type VarContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyVarContext() *VarContext {
	var p = new(VarContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_var
	return p
}

func (*VarContext) IsVarContext() {}

func NewVarContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *VarContext {
	var p = new(VarContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_var

	return p
}

func (s *VarContext) GetParser() antlr.Parser { return s.parser }

func (s *VarContext) NAME() antlr.TerminalNode {
	return s.GetToken(LuaParserNAME, 0)
}

func (s *VarContext) LParen() antlr.TerminalNode {
	return s.GetToken(LuaParserLParen, 0)
}

func (s *VarContext) Exp() IExpContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExpContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExpContext)
}

func (s *VarContext) RParen() antlr.TerminalNode {
	return s.GetToken(LuaParserRParen, 0)
}

func (s *VarContext) AllVarSuffix() []IVarSuffixContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IVarSuffixContext); ok {
			len++
		}
	}

	tst := make([]IVarSuffixContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IVarSuffixContext); ok {
			tst[i] = t.(IVarSuffixContext)
			i++
		}
	}

	return tst
}

func (s *VarContext) VarSuffix(i int) IVarSuffixContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IVarSuffixContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IVarSuffixContext)
}

func (s *VarContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *VarContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *VarContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitVar(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Var_() (localctx IVarContext) {
	this := p
	_ = this

	localctx = NewVarContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 30, LuaParserRULE_var)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(315)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case LuaParserNAME:
		{
			p.SetState(309)
			p.Match(LuaParserNAME)
		}

	case LuaParserLParen:
		{
			p.SetState(310)
			p.Match(LuaParserLParen)
		}
		{
			p.SetState(311)
			p.exp(0)
		}
		{
			p.SetState(312)
			p.Match(LuaParserRParen)
		}
		{
			p.SetState(313)
			p.VarSuffix()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}
	p.SetState(320)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 24, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(317)
				p.VarSuffix()
			}

		}
		p.SetState(322)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 24, p.GetParserRuleContext())
	}

	return localctx
}

// IVarSuffixContext is an interface to support dynamic dispatch.
type IVarSuffixContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsVarSuffixContext differentiates from other interfaces.
	IsVarSuffixContext()
}

type VarSuffixContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyVarSuffixContext() *VarSuffixContext {
	var p = new(VarSuffixContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_varSuffix
	return p
}

func (*VarSuffixContext) IsVarSuffixContext() {}

func NewVarSuffixContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *VarSuffixContext {
	var p = new(VarSuffixContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_varSuffix

	return p
}

func (s *VarSuffixContext) GetParser() antlr.Parser { return s.parser }

func (s *VarSuffixContext) LBracket() antlr.TerminalNode {
	return s.GetToken(LuaParserLBracket, 0)
}

func (s *VarSuffixContext) Exp() IExpContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExpContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExpContext)
}

func (s *VarSuffixContext) RBracket() antlr.TerminalNode {
	return s.GetToken(LuaParserRBracket, 0)
}

func (s *VarSuffixContext) Dot() antlr.TerminalNode {
	return s.GetToken(LuaParserDot, 0)
}

func (s *VarSuffixContext) NAME() antlr.TerminalNode {
	return s.GetToken(LuaParserNAME, 0)
}

func (s *VarSuffixContext) AllNameAndArgs() []INameAndArgsContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(INameAndArgsContext); ok {
			len++
		}
	}

	tst := make([]INameAndArgsContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(INameAndArgsContext); ok {
			tst[i] = t.(INameAndArgsContext)
			i++
		}
	}

	return tst
}

func (s *VarSuffixContext) NameAndArgs(i int) INameAndArgsContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INameAndArgsContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(INameAndArgsContext)
}

func (s *VarSuffixContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *VarSuffixContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *VarSuffixContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitVarSuffix(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) VarSuffix() (localctx IVarSuffixContext) {
	this := p
	_ = this

	localctx = NewVarSuffixContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 32, LuaParserRULE_varSuffix)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	p.SetState(326)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for (int64((_la-41)) & ^0x3f) == 0 && ((int64(1)<<(_la-41))&117440593) != 0 {
		{
			p.SetState(323)
			p.NameAndArgs()
		}

		p.SetState(328)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	p.SetState(335)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case LuaParserLBracket:
		{
			p.SetState(329)
			p.Match(LuaParserLBracket)
		}
		{
			p.SetState(330)
			p.exp(0)
		}
		{
			p.SetState(331)
			p.Match(LuaParserRBracket)
		}

	case LuaParserDot:
		{
			p.SetState(333)
			p.Match(LuaParserDot)
		}
		{
			p.SetState(334)
			p.Match(LuaParserNAME)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// INameAndArgsContext is an interface to support dynamic dispatch.
type INameAndArgsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNameAndArgsContext differentiates from other interfaces.
	IsNameAndArgsContext()
}

type NameAndArgsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNameAndArgsContext() *NameAndArgsContext {
	var p = new(NameAndArgsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_nameAndArgs
	return p
}

func (*NameAndArgsContext) IsNameAndArgsContext() {}

func NewNameAndArgsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NameAndArgsContext {
	var p = new(NameAndArgsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_nameAndArgs

	return p
}

func (s *NameAndArgsContext) GetParser() antlr.Parser { return s.parser }

func (s *NameAndArgsContext) Args() IArgsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IArgsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IArgsContext)
}

func (s *NameAndArgsContext) Colon() antlr.TerminalNode {
	return s.GetToken(LuaParserColon, 0)
}

func (s *NameAndArgsContext) NAME() antlr.TerminalNode {
	return s.GetToken(LuaParserNAME, 0)
}

func (s *NameAndArgsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NameAndArgsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NameAndArgsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitNameAndArgs(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) NameAndArgs() (localctx INameAndArgsContext) {
	this := p
	_ = this

	localctx = NewNameAndArgsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 34, LuaParserRULE_nameAndArgs)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	p.SetState(339)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == LuaParserColon {
		{
			p.SetState(337)
			p.Match(LuaParserColon)
		}
		{
			p.SetState(338)
			p.Match(LuaParserNAME)
		}

	}
	{
		p.SetState(341)
		p.Args()
	}

	return localctx
}

// IArgsContext is an interface to support dynamic dispatch.
type IArgsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsArgsContext differentiates from other interfaces.
	IsArgsContext()
}

type ArgsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyArgsContext() *ArgsContext {
	var p = new(ArgsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_args
	return p
}

func (*ArgsContext) IsArgsContext() {}

func NewArgsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ArgsContext {
	var p = new(ArgsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_args

	return p
}

func (s *ArgsContext) GetParser() antlr.Parser { return s.parser }

func (s *ArgsContext) LParen() antlr.TerminalNode {
	return s.GetToken(LuaParserLParen, 0)
}

func (s *ArgsContext) RParen() antlr.TerminalNode {
	return s.GetToken(LuaParserRParen, 0)
}

func (s *ArgsContext) Explist() IExplistContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExplistContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExplistContext)
}

func (s *ArgsContext) Tableconstructor() ITableconstructorContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITableconstructorContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ITableconstructorContext)
}

func (s *ArgsContext) String_() IStringContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStringContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStringContext)
}

func (s *ArgsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ArgsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ArgsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitArgs(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Args() (localctx IArgsContext) {
	this := p
	_ = this

	localctx = NewArgsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 36, LuaParserRULE_args)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(350)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case LuaParserLParen:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(343)
			p.Match(LuaParserLParen)
		}
		p.SetState(345)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&3460066344179859486) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&255) != 0 {
			{
				p.SetState(344)
				p.Explist()
			}

		}
		{
			p.SetState(347)
			p.Match(LuaParserRParen)
		}

	case LuaParserLBrace:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(348)
			p.Tableconstructor()
		}

	case LuaParserNORMALSTRING, LuaParserCHARSTRING, LuaParserLONGSTRING:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(349)
			p.String_()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IFunctiondefContext is an interface to support dynamic dispatch.
type IFunctiondefContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFunctiondefContext differentiates from other interfaces.
	IsFunctiondefContext()
}

type FunctiondefContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFunctiondefContext() *FunctiondefContext {
	var p = new(FunctiondefContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_functiondef
	return p
}

func (*FunctiondefContext) IsFunctiondefContext() {}

func NewFunctiondefContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FunctiondefContext {
	var p = new(FunctiondefContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_functiondef

	return p
}

func (s *FunctiondefContext) GetParser() antlr.Parser { return s.parser }

func (s *FunctiondefContext) Function() antlr.TerminalNode {
	return s.GetToken(LuaParserFunction, 0)
}

func (s *FunctiondefContext) Funcbody() IFuncbodyContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFuncbodyContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFuncbodyContext)
}

func (s *FunctiondefContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FunctiondefContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FunctiondefContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitFunctiondef(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Functiondef() (localctx IFunctiondefContext) {
	this := p
	_ = this

	localctx = NewFunctiondefContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 38, LuaParserRULE_functiondef)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(352)
		p.Match(LuaParserFunction)
	}
	{
		p.SetState(353)
		p.Funcbody()
	}

	return localctx
}

// IFuncbodyContext is an interface to support dynamic dispatch.
type IFuncbodyContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFuncbodyContext differentiates from other interfaces.
	IsFuncbodyContext()
}

type FuncbodyContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFuncbodyContext() *FuncbodyContext {
	var p = new(FuncbodyContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_funcbody
	return p
}

func (*FuncbodyContext) IsFuncbodyContext() {}

func NewFuncbodyContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FuncbodyContext {
	var p = new(FuncbodyContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_funcbody

	return p
}

func (s *FuncbodyContext) GetParser() antlr.Parser { return s.parser }

func (s *FuncbodyContext) LParen() antlr.TerminalNode {
	return s.GetToken(LuaParserLParen, 0)
}

func (s *FuncbodyContext) RParen() antlr.TerminalNode {
	return s.GetToken(LuaParserRParen, 0)
}

func (s *FuncbodyContext) Block() IBlockContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBlockContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBlockContext)
}

func (s *FuncbodyContext) End() antlr.TerminalNode {
	return s.GetToken(LuaParserEnd, 0)
}

func (s *FuncbodyContext) Parlist() IParlistContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IParlistContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IParlistContext)
}

func (s *FuncbodyContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FuncbodyContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FuncbodyContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitFuncbody(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Funcbody() (localctx IFuncbodyContext) {
	this := p
	_ = this

	localctx = NewFuncbodyContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 40, LuaParserRULE_funcbody)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(355)
		p.Match(LuaParserLParen)
	}
	p.SetState(357)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == LuaParserEllipsis || _la == LuaParserNAME {
		{
			p.SetState(356)
			p.Parlist()
		}

	}
	{
		p.SetState(359)
		p.Match(LuaParserRParen)
	}
	{
		p.SetState(360)
		p.Block()
	}
	{
		p.SetState(361)
		p.Match(LuaParserEnd)
	}

	return localctx
}

// IParlistContext is an interface to support dynamic dispatch.
type IParlistContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsParlistContext differentiates from other interfaces.
	IsParlistContext()
}

type ParlistContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyParlistContext() *ParlistContext {
	var p = new(ParlistContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_parlist
	return p
}

func (*ParlistContext) IsParlistContext() {}

func NewParlistContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ParlistContext {
	var p = new(ParlistContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_parlist

	return p
}

func (s *ParlistContext) GetParser() antlr.Parser { return s.parser }

func (s *ParlistContext) Namelist() INamelistContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INamelistContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INamelistContext)
}

func (s *ParlistContext) Comma() antlr.TerminalNode {
	return s.GetToken(LuaParserComma, 0)
}

func (s *ParlistContext) Ellipsis() antlr.TerminalNode {
	return s.GetToken(LuaParserEllipsis, 0)
}

func (s *ParlistContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ParlistContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ParlistContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitParlist(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Parlist() (localctx IParlistContext) {
	this := p
	_ = this

	localctx = NewParlistContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 42, LuaParserRULE_parlist)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(369)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case LuaParserNAME:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(363)
			p.Namelist()
		}
		p.SetState(366)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == LuaParserComma {
			{
				p.SetState(364)
				p.Match(LuaParserComma)
			}
			{
				p.SetState(365)
				p.Match(LuaParserEllipsis)
			}

		}

	case LuaParserEllipsis:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(368)
			p.Match(LuaParserEllipsis)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// ITableconstructorContext is an interface to support dynamic dispatch.
type ITableconstructorContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsTableconstructorContext differentiates from other interfaces.
	IsTableconstructorContext()
}

type TableconstructorContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyTableconstructorContext() *TableconstructorContext {
	var p = new(TableconstructorContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_tableconstructor
	return p
}

func (*TableconstructorContext) IsTableconstructorContext() {}

func NewTableconstructorContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *TableconstructorContext {
	var p = new(TableconstructorContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_tableconstructor

	return p
}

func (s *TableconstructorContext) GetParser() antlr.Parser { return s.parser }

func (s *TableconstructorContext) LBrace() antlr.TerminalNode {
	return s.GetToken(LuaParserLBrace, 0)
}

func (s *TableconstructorContext) RBrace() antlr.TerminalNode {
	return s.GetToken(LuaParserRBrace, 0)
}

func (s *TableconstructorContext) Fieldlist() IFieldlistContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFieldlistContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFieldlistContext)
}

func (s *TableconstructorContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TableconstructorContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *TableconstructorContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitTableconstructor(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Tableconstructor() (localctx ITableconstructorContext) {
	this := p
	_ = this

	localctx = NewTableconstructorContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 44, LuaParserRULE_tableconstructor)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(371)
		p.Match(LuaParserLBrace)
	}
	p.SetState(373)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&3460075140272881694) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&255) != 0 {
		{
			p.SetState(372)
			p.Fieldlist()
		}

	}
	{
		p.SetState(375)
		p.Match(LuaParserRBrace)
	}

	return localctx
}

// IFieldlistContext is an interface to support dynamic dispatch.
type IFieldlistContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFieldlistContext differentiates from other interfaces.
	IsFieldlistContext()
}

type FieldlistContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFieldlistContext() *FieldlistContext {
	var p = new(FieldlistContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_fieldlist
	return p
}

func (*FieldlistContext) IsFieldlistContext() {}

func NewFieldlistContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FieldlistContext {
	var p = new(FieldlistContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_fieldlist

	return p
}

func (s *FieldlistContext) GetParser() antlr.Parser { return s.parser }

func (s *FieldlistContext) AllField() []IFieldContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFieldContext); ok {
			len++
		}
	}

	tst := make([]IFieldContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFieldContext); ok {
			tst[i] = t.(IFieldContext)
			i++
		}
	}

	return tst
}

func (s *FieldlistContext) Field(i int) IFieldContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFieldContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFieldContext)
}

func (s *FieldlistContext) AllFieldsep() []IFieldsepContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFieldsepContext); ok {
			len++
		}
	}

	tst := make([]IFieldsepContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFieldsepContext); ok {
			tst[i] = t.(IFieldsepContext)
			i++
		}
	}

	return tst
}

func (s *FieldlistContext) Fieldsep(i int) IFieldsepContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFieldsepContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFieldsepContext)
}

func (s *FieldlistContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FieldlistContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FieldlistContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitFieldlist(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Fieldlist() (localctx IFieldlistContext) {
	this := p
	_ = this

	localctx = NewFieldlistContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 46, LuaParserRULE_fieldlist)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(377)
		p.Field()
	}
	p.SetState(383)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 34, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(378)
				p.Fieldsep()
			}
			{
				p.SetState(379)
				p.Field()
			}

		}
		p.SetState(385)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 34, p.GetParserRuleContext())
	}
	p.SetState(387)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == LuaParserComma || _la == LuaParserSemiColon {
		{
			p.SetState(386)
			p.Fieldsep()
		}

	}

	return localctx
}

// IFieldContext is an interface to support dynamic dispatch.
type IFieldContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFieldContext differentiates from other interfaces.
	IsFieldContext()
}

type FieldContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFieldContext() *FieldContext {
	var p = new(FieldContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_field
	return p
}

func (*FieldContext) IsFieldContext() {}

func NewFieldContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FieldContext {
	var p = new(FieldContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_field

	return p
}

func (s *FieldContext) GetParser() antlr.Parser { return s.parser }

func (s *FieldContext) LBracket() antlr.TerminalNode {
	return s.GetToken(LuaParserLBracket, 0)
}

func (s *FieldContext) AllExp() []IExpContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExpContext); ok {
			len++
		}
	}

	tst := make([]IExpContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExpContext); ok {
			tst[i] = t.(IExpContext)
			i++
		}
	}

	return tst
}

func (s *FieldContext) Exp(i int) IExpContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExpContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExpContext)
}

func (s *FieldContext) RBracket() antlr.TerminalNode {
	return s.GetToken(LuaParserRBracket, 0)
}

func (s *FieldContext) AssignEq() antlr.TerminalNode {
	return s.GetToken(LuaParserAssignEq, 0)
}

func (s *FieldContext) NAME() antlr.TerminalNode {
	return s.GetToken(LuaParserNAME, 0)
}

func (s *FieldContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FieldContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FieldContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitField(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Field() (localctx IFieldContext) {
	this := p
	_ = this

	localctx = NewFieldContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 48, LuaParserRULE_field)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(399)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 36, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(389)
			p.Match(LuaParserLBracket)
		}
		{
			p.SetState(390)
			p.exp(0)
		}
		{
			p.SetState(391)
			p.Match(LuaParserRBracket)
		}
		{
			p.SetState(392)
			p.Match(LuaParserAssignEq)
		}
		{
			p.SetState(393)
			p.exp(0)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(395)
			p.Match(LuaParserNAME)
		}
		{
			p.SetState(396)
			p.Match(LuaParserAssignEq)
		}
		{
			p.SetState(397)
			p.exp(0)
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(398)
			p.exp(0)
		}

	}

	return localctx
}

// IFieldsepContext is an interface to support dynamic dispatch.
type IFieldsepContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFieldsepContext differentiates from other interfaces.
	IsFieldsepContext()
}

type FieldsepContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFieldsepContext() *FieldsepContext {
	var p = new(FieldsepContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_fieldsep
	return p
}

func (*FieldsepContext) IsFieldsepContext() {}

func NewFieldsepContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FieldsepContext {
	var p = new(FieldsepContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_fieldsep

	return p
}

func (s *FieldsepContext) GetParser() antlr.Parser { return s.parser }

func (s *FieldsepContext) Comma() antlr.TerminalNode {
	return s.GetToken(LuaParserComma, 0)
}

func (s *FieldsepContext) SemiColon() antlr.TerminalNode {
	return s.GetToken(LuaParserSemiColon, 0)
}

func (s *FieldsepContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FieldsepContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FieldsepContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitFieldsep(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Fieldsep() (localctx IFieldsepContext) {
	this := p
	_ = this

	localctx = NewFieldsepContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 50, LuaParserRULE_fieldsep)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(401)
		_la = p.GetTokenStream().LA(1)

		if !(_la == LuaParserComma || _la == LuaParserSemiColon) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IOperatorOrContext is an interface to support dynamic dispatch.
type IOperatorOrContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsOperatorOrContext differentiates from other interfaces.
	IsOperatorOrContext()
}

type OperatorOrContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOperatorOrContext() *OperatorOrContext {
	var p = new(OperatorOrContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_operatorOr
	return p
}

func (*OperatorOrContext) IsOperatorOrContext() {}

func NewOperatorOrContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OperatorOrContext {
	var p = new(OperatorOrContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_operatorOr

	return p
}

func (s *OperatorOrContext) GetParser() antlr.Parser { return s.parser }

func (s *OperatorOrContext) Or() antlr.TerminalNode {
	return s.GetToken(LuaParserOr, 0)
}

func (s *OperatorOrContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperatorOrContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OperatorOrContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitOperatorOr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) OperatorOr() (localctx IOperatorOrContext) {
	this := p
	_ = this

	localctx = NewOperatorOrContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 52, LuaParserRULE_operatorOr)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(403)
		p.Match(LuaParserOr)
	}

	return localctx
}

// IOperatorAndContext is an interface to support dynamic dispatch.
type IOperatorAndContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsOperatorAndContext differentiates from other interfaces.
	IsOperatorAndContext()
}

type OperatorAndContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOperatorAndContext() *OperatorAndContext {
	var p = new(OperatorAndContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_operatorAnd
	return p
}

func (*OperatorAndContext) IsOperatorAndContext() {}

func NewOperatorAndContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OperatorAndContext {
	var p = new(OperatorAndContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_operatorAnd

	return p
}

func (s *OperatorAndContext) GetParser() antlr.Parser { return s.parser }

func (s *OperatorAndContext) And() antlr.TerminalNode {
	return s.GetToken(LuaParserAnd, 0)
}

func (s *OperatorAndContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperatorAndContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OperatorAndContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitOperatorAnd(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) OperatorAnd() (localctx IOperatorAndContext) {
	this := p
	_ = this

	localctx = NewOperatorAndContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 54, LuaParserRULE_operatorAnd)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(405)
		p.Match(LuaParserAnd)
	}

	return localctx
}

// IOperatorComparisonContext is an interface to support dynamic dispatch.
type IOperatorComparisonContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsOperatorComparisonContext differentiates from other interfaces.
	IsOperatorComparisonContext()
}

type OperatorComparisonContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOperatorComparisonContext() *OperatorComparisonContext {
	var p = new(OperatorComparisonContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_operatorComparison
	return p
}

func (*OperatorComparisonContext) IsOperatorComparisonContext() {}

func NewOperatorComparisonContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OperatorComparisonContext {
	var p = new(OperatorComparisonContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_operatorComparison

	return p
}

func (s *OperatorComparisonContext) GetParser() antlr.Parser { return s.parser }

func (s *OperatorComparisonContext) Lt() antlr.TerminalNode {
	return s.GetToken(LuaParserLt, 0)
}

func (s *OperatorComparisonContext) Gt() antlr.TerminalNode {
	return s.GetToken(LuaParserGt, 0)
}

func (s *OperatorComparisonContext) LtEq() antlr.TerminalNode {
	return s.GetToken(LuaParserLtEq, 0)
}

func (s *OperatorComparisonContext) GtEq() antlr.TerminalNode {
	return s.GetToken(LuaParserGtEq, 0)
}

func (s *OperatorComparisonContext) Neq() antlr.TerminalNode {
	return s.GetToken(LuaParserNeq, 0)
}

func (s *OperatorComparisonContext) Eq() antlr.TerminalNode {
	return s.GetToken(LuaParserEq, 0)
}

func (s *OperatorComparisonContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperatorComparisonContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OperatorComparisonContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitOperatorComparison(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) OperatorComparison() (localctx IOperatorComparisonContext) {
	this := p
	_ = this

	localctx = NewOperatorComparisonContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 56, LuaParserRULE_operatorComparison)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(407)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&2164663517184) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IOperatorStrcatContext is an interface to support dynamic dispatch.
type IOperatorStrcatContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsOperatorStrcatContext differentiates from other interfaces.
	IsOperatorStrcatContext()
}

type OperatorStrcatContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOperatorStrcatContext() *OperatorStrcatContext {
	var p = new(OperatorStrcatContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_operatorStrcat
	return p
}

func (*OperatorStrcatContext) IsOperatorStrcatContext() {}

func NewOperatorStrcatContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OperatorStrcatContext {
	var p = new(OperatorStrcatContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_operatorStrcat

	return p
}

func (s *OperatorStrcatContext) GetParser() antlr.Parser { return s.parser }

func (s *OperatorStrcatContext) Strcat() antlr.TerminalNode {
	return s.GetToken(LuaParserStrcat, 0)
}

func (s *OperatorStrcatContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperatorStrcatContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OperatorStrcatContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitOperatorStrcat(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) OperatorStrcat() (localctx IOperatorStrcatContext) {
	this := p
	_ = this

	localctx = NewOperatorStrcatContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 58, LuaParserRULE_operatorStrcat)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(409)
		p.Match(LuaParserStrcat)
	}

	return localctx
}

// IOperatorAddSubContext is an interface to support dynamic dispatch.
type IOperatorAddSubContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsOperatorAddSubContext differentiates from other interfaces.
	IsOperatorAddSubContext()
}

type OperatorAddSubContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOperatorAddSubContext() *OperatorAddSubContext {
	var p = new(OperatorAddSubContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_operatorAddSub
	return p
}

func (*OperatorAddSubContext) IsOperatorAddSubContext() {}

func NewOperatorAddSubContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OperatorAddSubContext {
	var p = new(OperatorAddSubContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_operatorAddSub

	return p
}

func (s *OperatorAddSubContext) GetParser() antlr.Parser { return s.parser }

func (s *OperatorAddSubContext) Plus() antlr.TerminalNode {
	return s.GetToken(LuaParserPlus, 0)
}

func (s *OperatorAddSubContext) Sub() antlr.TerminalNode {
	return s.GetToken(LuaParserSub, 0)
}

func (s *OperatorAddSubContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperatorAddSubContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OperatorAddSubContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitOperatorAddSub(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) OperatorAddSub() (localctx IOperatorAddSubContext) {
	this := p
	_ = this

	localctx = NewOperatorAddSubContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 60, LuaParserRULE_operatorAddSub)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(411)
		_la = p.GetTokenStream().LA(1)

		if !(_la == LuaParserPlus || _la == LuaParserSub) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IOperatorMulDivModContext is an interface to support dynamic dispatch.
type IOperatorMulDivModContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsOperatorMulDivModContext differentiates from other interfaces.
	IsOperatorMulDivModContext()
}

type OperatorMulDivModContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOperatorMulDivModContext() *OperatorMulDivModContext {
	var p = new(OperatorMulDivModContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_operatorMulDivMod
	return p
}

func (*OperatorMulDivModContext) IsOperatorMulDivModContext() {}

func NewOperatorMulDivModContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OperatorMulDivModContext {
	var p = new(OperatorMulDivModContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_operatorMulDivMod

	return p
}

func (s *OperatorMulDivModContext) GetParser() antlr.Parser { return s.parser }

func (s *OperatorMulDivModContext) Mul() antlr.TerminalNode {
	return s.GetToken(LuaParserMul, 0)
}

func (s *OperatorMulDivModContext) Div() antlr.TerminalNode {
	return s.GetToken(LuaParserDiv, 0)
}

func (s *OperatorMulDivModContext) Mod() antlr.TerminalNode {
	return s.GetToken(LuaParserMod, 0)
}

func (s *OperatorMulDivModContext) IntegralDiv() antlr.TerminalNode {
	return s.GetToken(LuaParserIntegralDiv, 0)
}

func (s *OperatorMulDivModContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperatorMulDivModContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OperatorMulDivModContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitOperatorMulDivMod(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) OperatorMulDivMod() (localctx IOperatorMulDivModContext) {
	this := p
	_ = this

	localctx = NewOperatorMulDivModContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 62, LuaParserRULE_operatorMulDivMod)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(413)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&251658240) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IOperatorBitwiseContext is an interface to support dynamic dispatch.
type IOperatorBitwiseContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsOperatorBitwiseContext differentiates from other interfaces.
	IsOperatorBitwiseContext()
}

type OperatorBitwiseContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOperatorBitwiseContext() *OperatorBitwiseContext {
	var p = new(OperatorBitwiseContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_operatorBitwise
	return p
}

func (*OperatorBitwiseContext) IsOperatorBitwiseContext() {}

func NewOperatorBitwiseContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OperatorBitwiseContext {
	var p = new(OperatorBitwiseContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_operatorBitwise

	return p
}

func (s *OperatorBitwiseContext) GetParser() antlr.Parser { return s.parser }

func (s *OperatorBitwiseContext) Amp() antlr.TerminalNode {
	return s.GetToken(LuaParserAmp, 0)
}

func (s *OperatorBitwiseContext) Xand() antlr.TerminalNode {
	return s.GetToken(LuaParserXand, 0)
}

func (s *OperatorBitwiseContext) NotSymbol() antlr.TerminalNode {
	return s.GetToken(LuaParserNotSymbol, 0)
}

func (s *OperatorBitwiseContext) LtLt() antlr.TerminalNode {
	return s.GetToken(LuaParserLtLt, 0)
}

func (s *OperatorBitwiseContext) GtGt() antlr.TerminalNode {
	return s.GetToken(LuaParserGtGt, 0)
}

func (s *OperatorBitwiseContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperatorBitwiseContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OperatorBitwiseContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitOperatorBitwise(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) OperatorBitwise() (localctx IOperatorBitwiseContext) {
	this := p
	_ = this

	localctx = NewOperatorBitwiseContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 64, LuaParserRULE_operatorBitwise)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(415)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&2305843013240225792) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IOperatorUnaryContext is an interface to support dynamic dispatch.
type IOperatorUnaryContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsOperatorUnaryContext differentiates from other interfaces.
	IsOperatorUnaryContext()
}

type OperatorUnaryContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOperatorUnaryContext() *OperatorUnaryContext {
	var p = new(OperatorUnaryContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_operatorUnary
	return p
}

func (*OperatorUnaryContext) IsOperatorUnaryContext() {}

func NewOperatorUnaryContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OperatorUnaryContext {
	var p = new(OperatorUnaryContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_operatorUnary

	return p
}

func (s *OperatorUnaryContext) GetParser() antlr.Parser { return s.parser }

func (s *OperatorUnaryContext) Not() antlr.TerminalNode {
	return s.GetToken(LuaParserNot, 0)
}

func (s *OperatorUnaryContext) Pound() antlr.TerminalNode {
	return s.GetToken(LuaParserPound, 0)
}

func (s *OperatorUnaryContext) Sub() antlr.TerminalNode {
	return s.GetToken(LuaParserSub, 0)
}

func (s *OperatorUnaryContext) NotSymbol() antlr.TerminalNode {
	return s.GetToken(LuaParserNotSymbol, 0)
}

func (s *OperatorUnaryContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperatorUnaryContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OperatorUnaryContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitOperatorUnary(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) OperatorUnary() (localctx IOperatorUnaryContext) {
	this := p
	_ = this

	localctx = NewOperatorUnaryContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 66, LuaParserRULE_operatorUnary)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(417)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&2306968917712568320) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IOperatorPowerContext is an interface to support dynamic dispatch.
type IOperatorPowerContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsOperatorPowerContext differentiates from other interfaces.
	IsOperatorPowerContext()
}

type OperatorPowerContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOperatorPowerContext() *OperatorPowerContext {
	var p = new(OperatorPowerContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_operatorPower
	return p
}

func (*OperatorPowerContext) IsOperatorPowerContext() {}

func NewOperatorPowerContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OperatorPowerContext {
	var p = new(OperatorPowerContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_operatorPower

	return p
}

func (s *OperatorPowerContext) GetParser() antlr.Parser { return s.parser }

func (s *OperatorPowerContext) Power() antlr.TerminalNode {
	return s.GetToken(LuaParserPower, 0)
}

func (s *OperatorPowerContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperatorPowerContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OperatorPowerContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitOperatorPower(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) OperatorPower() (localctx IOperatorPowerContext) {
	this := p
	_ = this

	localctx = NewOperatorPowerContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 68, LuaParserRULE_operatorPower)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(419)
		p.Match(LuaParserPower)
	}

	return localctx
}

// INumberContext is an interface to support dynamic dispatch.
type INumberContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNumberContext differentiates from other interfaces.
	IsNumberContext()
}

type NumberContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNumberContext() *NumberContext {
	var p = new(NumberContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_number
	return p
}

func (*NumberContext) IsNumberContext() {}

func NewNumberContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NumberContext {
	var p = new(NumberContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_number

	return p
}

func (s *NumberContext) GetParser() antlr.Parser { return s.parser }

func (s *NumberContext) INT() antlr.TerminalNode {
	return s.GetToken(LuaParserINT, 0)
}

func (s *NumberContext) HEX() antlr.TerminalNode {
	return s.GetToken(LuaParserHEX, 0)
}

func (s *NumberContext) FLOAT() antlr.TerminalNode {
	return s.GetToken(LuaParserFLOAT, 0)
}

func (s *NumberContext) HEX_FLOAT() antlr.TerminalNode {
	return s.GetToken(LuaParserHEX_FLOAT, 0)
}

func (s *NumberContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NumberContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NumberContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitNumber(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) Number() (localctx INumberContext) {
	this := p
	_ = this

	localctx = NewNumberContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 70, LuaParserRULE_number)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(421)
		_la = p.GetTokenStream().LA(1)

		if !((int64((_la-68)) & ^0x3f) == 0 && ((int64(1)<<(_la-68))&15) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IStringContext is an interface to support dynamic dispatch.
type IStringContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStringContext differentiates from other interfaces.
	IsStringContext()
}

type StringContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStringContext() *StringContext {
	var p = new(StringContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = LuaParserRULE_string
	return p
}

func (*StringContext) IsStringContext() {}

func NewStringContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StringContext {
	var p = new(StringContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = LuaParserRULE_string

	return p
}

func (s *StringContext) GetParser() antlr.Parser { return s.parser }

func (s *StringContext) NORMALSTRING() antlr.TerminalNode {
	return s.GetToken(LuaParserNORMALSTRING, 0)
}

func (s *StringContext) CHARSTRING() antlr.TerminalNode {
	return s.GetToken(LuaParserCHARSTRING, 0)
}

func (s *StringContext) LONGSTRING() antlr.TerminalNode {
	return s.GetToken(LuaParserLONGSTRING, 0)
}

func (s *StringContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StringContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StringContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case LuaParserVisitor:
		return t.VisitString(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *LuaParser) String_() (localctx IStringContext) {
	this := p
	_ = this

	localctx = NewStringContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 72, LuaParserRULE_string)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(423)
		_la = p.GetTokenStream().LA(1)

		if !((int64((_la-65)) & ^0x3f) == 0 && ((int64(1)<<(_la-65))&7) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

func (p *LuaParser) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 11:
		var t *ExpContext = nil
		if localctx != nil {
			t = localctx.(*ExpContext)
		}
		return p.Exp_Sempred(t, predIndex)

	default:
		panic("No predicate with index: " + fmt.Sprint(ruleIndex))
	}
}

func (p *LuaParser) Exp_Sempred(localctx antlr.RuleContext, predIndex int) bool {
	this := p
	_ = this

	switch predIndex {
	case 0:
		return p.Precpred(p.GetParserRuleContext(), 9)

	case 1:
		return p.Precpred(p.GetParserRuleContext(), 7)

	case 2:
		return p.Precpred(p.GetParserRuleContext(), 6)

	case 3:
		return p.Precpred(p.GetParserRuleContext(), 5)

	case 4:
		return p.Precpred(p.GetParserRuleContext(), 4)

	case 5:
		return p.Precpred(p.GetParserRuleContext(), 3)

	case 6:
		return p.Precpred(p.GetParserRuleContext(), 2)

	case 7:
		return p.Precpred(p.GetParserRuleContext(), 1)

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}
