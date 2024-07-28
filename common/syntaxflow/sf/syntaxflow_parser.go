// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package sf // SyntaxFlow
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

type SyntaxFlowParser struct {
	*antlr.BaseParser
}

var syntaxflowParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	literalNames           []string
	symbolicNames          []string
	ruleNames              []string
	predictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func syntaxflowParserInit() {
	staticData := &syntaxflowParserStaticData
	staticData.literalNames = []string{
		"", "'${'", "';'", "'\\n'", "'->'", "'-->'", "'+'", "'==>'", "'...'",
		"'%%'", "'..'", "'<='", "'>='", "'>>'", "'=>'", "'=='", "'=~'", "'!~'",
		"'&&'", "'||'", "'!='", "'?{'", "'-{'", "'}->'", "'#{'", "'#>'", "'#->'",
		"'>'", "'.'", "'<'", "'='", "'?'", "'('", "','", "')'", "'['", "']'",
		"'{'", "'}'", "'#'", "'$'", "':'", "'%'", "'!'", "'*'", "'-'", "'as'",
		"'`'", "'''", "'\"'", "", "", "", "", "", "", "'str'", "'list'", "'dict'",
		"", "'bool'", "", "'alert'", "'check'", "'then'", "", "'else'", "'type'",
		"'in'", "'call'", "'function'", "", "'phi'", "", "", "'opcode'", "'have'",
		"'any'", "'not'", "'for'",
	}
	staticData.symbolicNames = []string{
		"", "", "", "", "", "", "", "DeepFilter", "Deep", "Percent", "DeepDot",
		"LtEq", "GtEq", "DoubleGt", "Filter", "EqEq", "RegexpMatch", "NotRegexpMatch",
		"And", "Or", "NotEq", "ConditionStart", "DeepNextStart", "DeepNextEnd",
		"TopDefStart", "DefStart", "TopDef", "Gt", "Dot", "Lt", "Eq", "Question",
		"OpenParen", "Comma", "CloseParen", "ListSelectOpen", "ListSelectClose",
		"MapBuilderOpen", "MapBuilderClose", "ListStart", "DollarOutput", "Colon",
		"Search", "Bang", "Star", "Minus", "As", "Backtick", "SingleQuote",
		"DoubleQuote", "LineComment", "WhiteSpace", "Number", "OctalNumber",
		"BinaryNumber", "HexNumber", "StringType", "ListType", "DictType", "NumberType",
		"BoolType", "BoolLiteral", "Alert", "Check", "Then", "Desc", "Else",
		"Type", "In", "Call", "Function", "Constant", "Phi", "FormalParam",
		"Return", "Opcode", "Have", "HaveAny", "Not", "For", "Identifier", "IdentifierChar",
		"QuotedStringLiteral", "RegexpLiteral", "WS",
	}
	staticData.ruleNames = []string{
		"flow", "statements", "statement", "fileFilterContentStatement", "fileFilterContentInput",
		"fileFilterContentMethod", "fileFilterContentMethodParam", "fileFilterContentMethodParamItem",
		"fileFilterContentMethodParamKey", "fileFilterContentMethodParamValue",
		"fileName", "filterStatement", "comment", "eos", "line", "lines", "descriptionStatement",
		"descriptionItems", "descriptionItem", "alertStatement", "checkStatement",
		"thenExpr", "elseExpr", "refVariable", "filterItemFirst", "filterItem",
		"filterExpr", "nativeCall", "useNativeCall", "useDefCalcParams", "actualParam",
		"actualParamFilter", "singleParam", "config", "recursiveConfigItem",
		"recursiveConfigItemValue", "sliceCallItem", "nameFilter", "chainFilter",
		"stringLiteralWithoutStarGroup", "negativeCondition", "conditionExpression",
		"numberLiteral", "stringLiteral", "stringLiteralWithoutStar", "regexpLiteral",
		"identifier", "keywords", "opcodes", "types", "boolLiteral",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 84, 595, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7, 20, 2,
		21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25, 2, 26,
		7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 2, 29, 7, 29, 2, 30, 7, 30, 2, 31, 7,
		31, 2, 32, 7, 32, 2, 33, 7, 33, 2, 34, 7, 34, 2, 35, 7, 35, 2, 36, 7, 36,
		2, 37, 7, 37, 2, 38, 7, 38, 2, 39, 7, 39, 2, 40, 7, 40, 2, 41, 7, 41, 2,
		42, 7, 42, 2, 43, 7, 43, 2, 44, 7, 44, 2, 45, 7, 45, 2, 46, 7, 46, 2, 47,
		7, 47, 2, 48, 7, 48, 2, 49, 7, 49, 2, 50, 7, 50, 1, 0, 1, 0, 1, 0, 1, 1,
		4, 1, 107, 8, 1, 11, 1, 12, 1, 108, 1, 2, 1, 2, 3, 2, 113, 8, 2, 1, 2,
		1, 2, 3, 2, 117, 8, 2, 1, 2, 1, 2, 3, 2, 121, 8, 2, 1, 2, 1, 2, 3, 2, 125,
		8, 2, 1, 2, 1, 2, 3, 2, 129, 8, 2, 1, 2, 1, 2, 3, 2, 133, 8, 2, 1, 2, 3,
		2, 136, 8, 2, 1, 3, 1, 3, 1, 3, 1, 3, 3, 3, 142, 8, 3, 1, 3, 1, 3, 1, 3,
		1, 3, 3, 3, 148, 8, 3, 1, 4, 1, 4, 3, 4, 152, 8, 4, 1, 5, 1, 5, 1, 5, 3,
		5, 157, 8, 5, 1, 5, 1, 5, 1, 6, 1, 6, 3, 6, 163, 8, 6, 1, 6, 1, 6, 3, 6,
		167, 8, 6, 1, 6, 1, 6, 3, 6, 171, 8, 6, 5, 6, 173, 8, 6, 10, 6, 12, 6,
		176, 9, 6, 1, 6, 3, 6, 179, 8, 6, 1, 6, 3, 6, 182, 8, 6, 1, 7, 3, 7, 185,
		8, 7, 1, 7, 1, 7, 1, 8, 1, 8, 1, 8, 1, 9, 1, 9, 1, 10, 1, 10, 1, 10, 5,
		10, 197, 8, 10, 10, 10, 12, 10, 200, 9, 10, 1, 11, 1, 11, 5, 11, 204, 8,
		11, 10, 11, 12, 11, 207, 9, 11, 1, 11, 1, 11, 3, 11, 211, 8, 11, 1, 11,
		1, 11, 1, 11, 3, 11, 216, 8, 11, 3, 11, 218, 8, 11, 1, 12, 1, 12, 1, 13,
		1, 13, 3, 13, 224, 8, 13, 1, 14, 1, 14, 1, 15, 4, 15, 229, 8, 15, 11, 15,
		12, 15, 230, 1, 16, 1, 16, 1, 16, 3, 16, 236, 8, 16, 1, 16, 1, 16, 1, 16,
		3, 16, 241, 8, 16, 1, 16, 3, 16, 244, 8, 16, 1, 17, 3, 17, 247, 8, 17,
		1, 17, 1, 17, 1, 17, 3, 17, 252, 8, 17, 1, 17, 5, 17, 255, 8, 17, 10, 17,
		12, 17, 258, 9, 17, 1, 17, 3, 17, 261, 8, 17, 1, 17, 3, 17, 264, 8, 17,
		1, 18, 1, 18, 3, 18, 268, 8, 18, 1, 18, 1, 18, 1, 18, 1, 18, 3, 18, 274,
		8, 18, 3, 18, 276, 8, 18, 1, 19, 1, 19, 1, 19, 1, 19, 3, 19, 282, 8, 19,
		1, 20, 1, 20, 1, 20, 3, 20, 287, 8, 20, 1, 20, 3, 20, 290, 8, 20, 1, 21,
		1, 21, 1, 21, 1, 22, 1, 22, 1, 22, 1, 23, 1, 23, 1, 23, 1, 23, 1, 23, 1,
		23, 3, 23, 304, 8, 23, 1, 24, 1, 24, 1, 24, 3, 24, 309, 8, 24, 1, 24, 1,
		24, 3, 24, 313, 8, 24, 1, 25, 1, 25, 1, 25, 3, 25, 318, 8, 25, 1, 25, 1,
		25, 1, 25, 3, 25, 323, 8, 25, 1, 25, 3, 25, 326, 8, 25, 1, 25, 1, 25, 1,
		25, 1, 25, 1, 25, 1, 25, 1, 25, 1, 25, 1, 25, 1, 25, 1, 25, 1, 25, 1, 25,
		1, 25, 3, 25, 342, 8, 25, 1, 25, 1, 25, 1, 25, 1, 25, 3, 25, 348, 8, 25,
		1, 25, 1, 25, 1, 25, 1, 25, 1, 25, 3, 25, 355, 8, 25, 1, 26, 1, 26, 5,
		26, 359, 8, 26, 10, 26, 12, 26, 362, 9, 26, 1, 27, 1, 27, 1, 27, 1, 27,
		1, 28, 1, 28, 3, 28, 370, 8, 28, 1, 29, 1, 29, 3, 29, 374, 8, 29, 1, 29,
		1, 29, 1, 29, 3, 29, 379, 8, 29, 1, 29, 3, 29, 382, 8, 29, 1, 30, 1, 30,
		3, 30, 386, 8, 30, 1, 30, 4, 30, 389, 8, 30, 11, 30, 12, 30, 390, 1, 30,
		3, 30, 394, 8, 30, 1, 30, 3, 30, 397, 8, 30, 3, 30, 399, 8, 30, 1, 31,
		1, 31, 1, 31, 1, 31, 3, 31, 405, 8, 31, 1, 32, 1, 32, 1, 32, 3, 32, 410,
		8, 32, 1, 32, 3, 32, 413, 8, 32, 1, 32, 1, 32, 1, 33, 1, 33, 1, 33, 5,
		33, 420, 8, 33, 10, 33, 12, 33, 423, 9, 33, 1, 33, 3, 33, 426, 8, 33, 1,
		34, 3, 34, 429, 8, 34, 1, 34, 1, 34, 1, 34, 1, 34, 3, 34, 435, 8, 34, 1,
		35, 1, 35, 3, 35, 439, 8, 35, 1, 35, 1, 35, 1, 35, 1, 35, 3, 35, 445, 8,
		35, 1, 36, 1, 36, 3, 36, 449, 8, 36, 1, 37, 1, 37, 1, 37, 3, 37, 454, 8,
		37, 1, 38, 1, 38, 1, 38, 1, 38, 5, 38, 460, 8, 38, 10, 38, 12, 38, 463,
		9, 38, 1, 38, 3, 38, 466, 8, 38, 1, 38, 1, 38, 1, 38, 1, 38, 1, 38, 1,
		38, 1, 38, 1, 38, 1, 38, 1, 38, 1, 38, 1, 38, 5, 38, 480, 8, 38, 10, 38,
		12, 38, 483, 9, 38, 3, 38, 485, 8, 38, 1, 38, 3, 38, 488, 8, 38, 1, 38,
		3, 38, 491, 8, 38, 1, 39, 1, 39, 1, 39, 5, 39, 496, 8, 39, 10, 39, 12,
		39, 499, 9, 39, 1, 39, 3, 39, 502, 8, 39, 1, 40, 1, 40, 1, 41, 1, 41, 1,
		41, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41, 5, 41, 517,
		8, 41, 10, 41, 12, 41, 520, 9, 41, 1, 41, 3, 41, 523, 8, 41, 1, 41, 1,
		41, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41,
		1, 41, 3, 41, 538, 8, 41, 1, 41, 1, 41, 1, 41, 3, 41, 543, 8, 41, 3, 41,
		545, 8, 41, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41, 1, 41, 5, 41, 553, 8, 41,
		10, 41, 12, 41, 556, 9, 41, 1, 42, 1, 42, 1, 43, 1, 43, 3, 43, 562, 8,
		43, 1, 44, 1, 44, 3, 44, 566, 8, 44, 1, 45, 1, 45, 1, 46, 1, 46, 1, 46,
		3, 46, 573, 8, 46, 1, 47, 1, 47, 1, 47, 1, 47, 1, 47, 1, 47, 1, 47, 1,
		47, 1, 47, 1, 47, 1, 47, 1, 47, 3, 47, 587, 8, 47, 1, 48, 1, 48, 1, 49,
		1, 49, 1, 50, 1, 50, 1, 50, 0, 1, 82, 51, 0, 2, 4, 6, 8, 10, 12, 14, 16,
		18, 20, 22, 24, 26, 28, 30, 32, 34, 36, 38, 40, 42, 44, 46, 48, 50, 52,
		54, 56, 58, 60, 62, 64, 66, 68, 70, 72, 74, 76, 78, 80, 82, 84, 86, 88,
		90, 92, 94, 96, 98, 100, 0, 6, 2, 0, 43, 43, 78, 78, 5, 0, 11, 12, 15,
		15, 20, 20, 27, 27, 29, 30, 1, 0, 16, 17, 1, 0, 52, 55, 1, 0, 69, 74, 1,
		0, 56, 60, 669, 0, 102, 1, 0, 0, 0, 2, 106, 1, 0, 0, 0, 4, 135, 1, 0, 0,
		0, 6, 137, 1, 0, 0, 0, 8, 151, 1, 0, 0, 0, 10, 153, 1, 0, 0, 0, 12, 160,
		1, 0, 0, 0, 14, 184, 1, 0, 0, 0, 16, 188, 1, 0, 0, 0, 18, 191, 1, 0, 0,
		0, 20, 193, 1, 0, 0, 0, 22, 217, 1, 0, 0, 0, 24, 219, 1, 0, 0, 0, 26, 223,
		1, 0, 0, 0, 28, 225, 1, 0, 0, 0, 30, 228, 1, 0, 0, 0, 32, 243, 1, 0, 0,
		0, 34, 246, 1, 0, 0, 0, 36, 275, 1, 0, 0, 0, 38, 277, 1, 0, 0, 0, 40, 283,
		1, 0, 0, 0, 42, 291, 1, 0, 0, 0, 44, 294, 1, 0, 0, 0, 46, 297, 1, 0, 0,
		0, 48, 312, 1, 0, 0, 0, 50, 354, 1, 0, 0, 0, 52, 356, 1, 0, 0, 0, 54, 363,
		1, 0, 0, 0, 56, 367, 1, 0, 0, 0, 58, 381, 1, 0, 0, 0, 60, 398, 1, 0, 0,
		0, 62, 404, 1, 0, 0, 0, 64, 412, 1, 0, 0, 0, 66, 416, 1, 0, 0, 0, 68, 428,
		1, 0, 0, 0, 70, 444, 1, 0, 0, 0, 72, 448, 1, 0, 0, 0, 74, 453, 1, 0, 0,
		0, 76, 490, 1, 0, 0, 0, 78, 492, 1, 0, 0, 0, 80, 503, 1, 0, 0, 0, 82, 544,
		1, 0, 0, 0, 84, 557, 1, 0, 0, 0, 86, 561, 1, 0, 0, 0, 88, 565, 1, 0, 0,
		0, 90, 567, 1, 0, 0, 0, 92, 572, 1, 0, 0, 0, 94, 586, 1, 0, 0, 0, 96, 588,
		1, 0, 0, 0, 98, 590, 1, 0, 0, 0, 100, 592, 1, 0, 0, 0, 102, 103, 3, 2,
		1, 0, 103, 104, 5, 0, 0, 1, 104, 1, 1, 0, 0, 0, 105, 107, 3, 4, 2, 0, 106,
		105, 1, 0, 0, 0, 107, 108, 1, 0, 0, 0, 108, 106, 1, 0, 0, 0, 108, 109,
		1, 0, 0, 0, 109, 3, 1, 0, 0, 0, 110, 112, 3, 40, 20, 0, 111, 113, 3, 26,
		13, 0, 112, 111, 1, 0, 0, 0, 112, 113, 1, 0, 0, 0, 113, 136, 1, 0, 0, 0,
		114, 116, 3, 32, 16, 0, 115, 117, 3, 26, 13, 0, 116, 115, 1, 0, 0, 0, 116,
		117, 1, 0, 0, 0, 117, 136, 1, 0, 0, 0, 118, 120, 3, 38, 19, 0, 119, 121,
		3, 26, 13, 0, 120, 119, 1, 0, 0, 0, 120, 121, 1, 0, 0, 0, 121, 136, 1,
		0, 0, 0, 122, 124, 3, 22, 11, 0, 123, 125, 3, 26, 13, 0, 124, 123, 1, 0,
		0, 0, 124, 125, 1, 0, 0, 0, 125, 136, 1, 0, 0, 0, 126, 128, 3, 6, 3, 0,
		127, 129, 3, 26, 13, 0, 128, 127, 1, 0, 0, 0, 128, 129, 1, 0, 0, 0, 129,
		136, 1, 0, 0, 0, 130, 132, 3, 24, 12, 0, 131, 133, 3, 26, 13, 0, 132, 131,
		1, 0, 0, 0, 132, 133, 1, 0, 0, 0, 133, 136, 1, 0, 0, 0, 134, 136, 3, 26,
		13, 0, 135, 110, 1, 0, 0, 0, 135, 114, 1, 0, 0, 0, 135, 118, 1, 0, 0, 0,
		135, 122, 1, 0, 0, 0, 135, 126, 1, 0, 0, 0, 135, 130, 1, 0, 0, 0, 135,
		134, 1, 0, 0, 0, 136, 5, 1, 0, 0, 0, 137, 138, 5, 1, 0, 0, 138, 139, 3,
		8, 4, 0, 139, 141, 5, 38, 0, 0, 140, 142, 3, 30, 15, 0, 141, 140, 1, 0,
		0, 0, 141, 142, 1, 0, 0, 0, 142, 143, 1, 0, 0, 0, 143, 144, 5, 28, 0, 0,
		144, 147, 3, 10, 5, 0, 145, 146, 5, 46, 0, 0, 146, 148, 3, 46, 23, 0, 147,
		145, 1, 0, 0, 0, 147, 148, 1, 0, 0, 0, 148, 7, 1, 0, 0, 0, 149, 152, 3,
		20, 10, 0, 150, 152, 3, 90, 45, 0, 151, 149, 1, 0, 0, 0, 151, 150, 1, 0,
		0, 0, 152, 9, 1, 0, 0, 0, 153, 154, 5, 80, 0, 0, 154, 156, 5, 32, 0, 0,
		155, 157, 3, 12, 6, 0, 156, 155, 1, 0, 0, 0, 156, 157, 1, 0, 0, 0, 157,
		158, 1, 0, 0, 0, 158, 159, 5, 34, 0, 0, 159, 11, 1, 0, 0, 0, 160, 162,
		3, 14, 7, 0, 161, 163, 3, 30, 15, 0, 162, 161, 1, 0, 0, 0, 162, 163, 1,
		0, 0, 0, 163, 174, 1, 0, 0, 0, 164, 166, 5, 33, 0, 0, 165, 167, 3, 30,
		15, 0, 166, 165, 1, 0, 0, 0, 166, 167, 1, 0, 0, 0, 167, 168, 1, 0, 0, 0,
		168, 170, 3, 14, 7, 0, 169, 171, 3, 30, 15, 0, 170, 169, 1, 0, 0, 0, 170,
		171, 1, 0, 0, 0, 171, 173, 1, 0, 0, 0, 172, 164, 1, 0, 0, 0, 173, 176,
		1, 0, 0, 0, 174, 172, 1, 0, 0, 0, 174, 175, 1, 0, 0, 0, 175, 178, 1, 0,
		0, 0, 176, 174, 1, 0, 0, 0, 177, 179, 5, 33, 0, 0, 178, 177, 1, 0, 0, 0,
		178, 179, 1, 0, 0, 0, 179, 181, 1, 0, 0, 0, 180, 182, 3, 30, 15, 0, 181,
		180, 1, 0, 0, 0, 181, 182, 1, 0, 0, 0, 182, 13, 1, 0, 0, 0, 183, 185, 3,
		16, 8, 0, 184, 183, 1, 0, 0, 0, 184, 185, 1, 0, 0, 0, 185, 186, 1, 0, 0,
		0, 186, 187, 3, 18, 9, 0, 187, 15, 1, 0, 0, 0, 188, 189, 5, 80, 0, 0, 189,
		190, 5, 41, 0, 0, 190, 17, 1, 0, 0, 0, 191, 192, 3, 74, 37, 0, 192, 19,
		1, 0, 0, 0, 193, 198, 3, 74, 37, 0, 194, 195, 9, 0, 0, 0, 195, 197, 3,
		74, 37, 0, 196, 194, 1, 0, 0, 0, 197, 200, 1, 0, 0, 0, 198, 196, 1, 0,
		0, 0, 198, 199, 1, 0, 0, 0, 199, 21, 1, 0, 0, 0, 200, 198, 1, 0, 0, 0,
		201, 205, 3, 46, 23, 0, 202, 204, 3, 50, 25, 0, 203, 202, 1, 0, 0, 0, 204,
		207, 1, 0, 0, 0, 205, 203, 1, 0, 0, 0, 205, 206, 1, 0, 0, 0, 206, 210,
		1, 0, 0, 0, 207, 205, 1, 0, 0, 0, 208, 209, 5, 46, 0, 0, 209, 211, 3, 46,
		23, 0, 210, 208, 1, 0, 0, 0, 210, 211, 1, 0, 0, 0, 211, 218, 1, 0, 0, 0,
		212, 215, 3, 52, 26, 0, 213, 214, 5, 46, 0, 0, 214, 216, 3, 46, 23, 0,
		215, 213, 1, 0, 0, 0, 215, 216, 1, 0, 0, 0, 216, 218, 1, 0, 0, 0, 217,
		201, 1, 0, 0, 0, 217, 212, 1, 0, 0, 0, 218, 23, 1, 0, 0, 0, 219, 220, 5,
		50, 0, 0, 220, 25, 1, 0, 0, 0, 221, 224, 5, 2, 0, 0, 222, 224, 3, 28, 14,
		0, 223, 221, 1, 0, 0, 0, 223, 222, 1, 0, 0, 0, 224, 27, 1, 0, 0, 0, 225,
		226, 5, 3, 0, 0, 226, 29, 1, 0, 0, 0, 227, 229, 3, 28, 14, 0, 228, 227,
		1, 0, 0, 0, 229, 230, 1, 0, 0, 0, 230, 228, 1, 0, 0, 0, 230, 231, 1, 0,
		0, 0, 231, 31, 1, 0, 0, 0, 232, 233, 5, 65, 0, 0, 233, 235, 5, 32, 0, 0,
		234, 236, 3, 34, 17, 0, 235, 234, 1, 0, 0, 0, 235, 236, 1, 0, 0, 0, 236,
		237, 1, 0, 0, 0, 237, 244, 5, 34, 0, 0, 238, 240, 5, 37, 0, 0, 239, 241,
		3, 34, 17, 0, 240, 239, 1, 0, 0, 0, 240, 241, 1, 0, 0, 0, 241, 242, 1,
		0, 0, 0, 242, 244, 5, 38, 0, 0, 243, 232, 1, 0, 0, 0, 243, 238, 1, 0, 0,
		0, 244, 33, 1, 0, 0, 0, 245, 247, 3, 30, 15, 0, 246, 245, 1, 0, 0, 0, 246,
		247, 1, 0, 0, 0, 247, 248, 1, 0, 0, 0, 248, 256, 3, 36, 18, 0, 249, 251,
		5, 33, 0, 0, 250, 252, 3, 30, 15, 0, 251, 250, 1, 0, 0, 0, 251, 252, 1,
		0, 0, 0, 252, 253, 1, 0, 0, 0, 253, 255, 3, 36, 18, 0, 254, 249, 1, 0,
		0, 0, 255, 258, 1, 0, 0, 0, 256, 254, 1, 0, 0, 0, 256, 257, 1, 0, 0, 0,
		257, 260, 1, 0, 0, 0, 258, 256, 1, 0, 0, 0, 259, 261, 5, 33, 0, 0, 260,
		259, 1, 0, 0, 0, 260, 261, 1, 0, 0, 0, 261, 263, 1, 0, 0, 0, 262, 264,
		3, 30, 15, 0, 263, 262, 1, 0, 0, 0, 263, 264, 1, 0, 0, 0, 264, 35, 1, 0,
		0, 0, 265, 267, 3, 86, 43, 0, 266, 268, 3, 30, 15, 0, 267, 266, 1, 0, 0,
		0, 267, 268, 1, 0, 0, 0, 268, 276, 1, 0, 0, 0, 269, 270, 3, 86, 43, 0,
		270, 271, 5, 41, 0, 0, 271, 273, 3, 86, 43, 0, 272, 274, 3, 30, 15, 0,
		273, 272, 1, 0, 0, 0, 273, 274, 1, 0, 0, 0, 274, 276, 1, 0, 0, 0, 275,
		265, 1, 0, 0, 0, 275, 269, 1, 0, 0, 0, 276, 37, 1, 0, 0, 0, 277, 278, 5,
		62, 0, 0, 278, 281, 3, 46, 23, 0, 279, 280, 5, 79, 0, 0, 280, 282, 3, 86,
		43, 0, 281, 279, 1, 0, 0, 0, 281, 282, 1, 0, 0, 0, 282, 39, 1, 0, 0, 0,
		283, 284, 5, 63, 0, 0, 284, 286, 3, 46, 23, 0, 285, 287, 3, 42, 21, 0,
		286, 285, 1, 0, 0, 0, 286, 287, 1, 0, 0, 0, 287, 289, 1, 0, 0, 0, 288,
		290, 3, 44, 22, 0, 289, 288, 1, 0, 0, 0, 289, 290, 1, 0, 0, 0, 290, 41,
		1, 0, 0, 0, 291, 292, 5, 64, 0, 0, 292, 293, 3, 86, 43, 0, 293, 43, 1,
		0, 0, 0, 294, 295, 5, 66, 0, 0, 295, 296, 3, 86, 43, 0, 296, 45, 1, 0,
		0, 0, 297, 303, 5, 40, 0, 0, 298, 304, 3, 92, 46, 0, 299, 300, 5, 32, 0,
		0, 300, 301, 3, 92, 46, 0, 301, 302, 5, 34, 0, 0, 302, 304, 1, 0, 0, 0,
		303, 298, 1, 0, 0, 0, 303, 299, 1, 0, 0, 0, 304, 47, 1, 0, 0, 0, 305, 313,
		3, 74, 37, 0, 306, 308, 5, 28, 0, 0, 307, 309, 3, 30, 15, 0, 308, 307,
		1, 0, 0, 0, 308, 309, 1, 0, 0, 0, 309, 310, 1, 0, 0, 0, 310, 313, 3, 74,
		37, 0, 311, 313, 3, 54, 27, 0, 312, 305, 1, 0, 0, 0, 312, 306, 1, 0, 0,
		0, 312, 311, 1, 0, 0, 0, 313, 49, 1, 0, 0, 0, 314, 355, 3, 48, 24, 0, 315,
		317, 5, 8, 0, 0, 316, 318, 3, 30, 15, 0, 317, 316, 1, 0, 0, 0, 317, 318,
		1, 0, 0, 0, 318, 319, 1, 0, 0, 0, 319, 355, 3, 74, 37, 0, 320, 322, 5,
		32, 0, 0, 321, 323, 3, 30, 15, 0, 322, 321, 1, 0, 0, 0, 322, 323, 1, 0,
		0, 0, 323, 325, 1, 0, 0, 0, 324, 326, 3, 60, 30, 0, 325, 324, 1, 0, 0,
		0, 325, 326, 1, 0, 0, 0, 326, 327, 1, 0, 0, 0, 327, 355, 5, 34, 0, 0, 328,
		329, 5, 35, 0, 0, 329, 330, 3, 72, 36, 0, 330, 331, 5, 36, 0, 0, 331, 355,
		1, 0, 0, 0, 332, 333, 5, 21, 0, 0, 333, 334, 3, 82, 41, 0, 334, 335, 5,
		38, 0, 0, 335, 355, 1, 0, 0, 0, 336, 355, 5, 4, 0, 0, 337, 355, 5, 25,
		0, 0, 338, 355, 5, 5, 0, 0, 339, 341, 5, 22, 0, 0, 340, 342, 3, 66, 33,
		0, 341, 340, 1, 0, 0, 0, 341, 342, 1, 0, 0, 0, 342, 343, 1, 0, 0, 0, 343,
		355, 5, 23, 0, 0, 344, 355, 5, 26, 0, 0, 345, 347, 5, 24, 0, 0, 346, 348,
		3, 66, 33, 0, 347, 346, 1, 0, 0, 0, 347, 348, 1, 0, 0, 0, 348, 349, 1,
		0, 0, 0, 349, 355, 5, 23, 0, 0, 350, 351, 5, 6, 0, 0, 351, 355, 3, 46,
		23, 0, 352, 353, 5, 45, 0, 0, 353, 355, 3, 46, 23, 0, 354, 314, 1, 0, 0,
		0, 354, 315, 1, 0, 0, 0, 354, 320, 1, 0, 0, 0, 354, 328, 1, 0, 0, 0, 354,
		332, 1, 0, 0, 0, 354, 336, 1, 0, 0, 0, 354, 337, 1, 0, 0, 0, 354, 338,
		1, 0, 0, 0, 354, 339, 1, 0, 0, 0, 354, 344, 1, 0, 0, 0, 354, 345, 1, 0,
		0, 0, 354, 350, 1, 0, 0, 0, 354, 352, 1, 0, 0, 0, 355, 51, 1, 0, 0, 0,
		356, 360, 3, 48, 24, 0, 357, 359, 3, 50, 25, 0, 358, 357, 1, 0, 0, 0, 359,
		362, 1, 0, 0, 0, 360, 358, 1, 0, 0, 0, 360, 361, 1, 0, 0, 0, 361, 53, 1,
		0, 0, 0, 362, 360, 1, 0, 0, 0, 363, 364, 5, 29, 0, 0, 364, 365, 3, 56,
		28, 0, 365, 366, 5, 27, 0, 0, 366, 55, 1, 0, 0, 0, 367, 369, 3, 92, 46,
		0, 368, 370, 3, 58, 29, 0, 369, 368, 1, 0, 0, 0, 369, 370, 1, 0, 0, 0,
		370, 57, 1, 0, 0, 0, 371, 373, 5, 37, 0, 0, 372, 374, 3, 66, 33, 0, 373,
		372, 1, 0, 0, 0, 373, 374, 1, 0, 0, 0, 374, 375, 1, 0, 0, 0, 375, 382,
		5, 38, 0, 0, 376, 378, 5, 32, 0, 0, 377, 379, 3, 66, 33, 0, 378, 377, 1,
		0, 0, 0, 378, 379, 1, 0, 0, 0, 379, 380, 1, 0, 0, 0, 380, 382, 5, 34, 0,
		0, 381, 371, 1, 0, 0, 0, 381, 376, 1, 0, 0, 0, 382, 59, 1, 0, 0, 0, 383,
		385, 3, 64, 32, 0, 384, 386, 3, 30, 15, 0, 385, 384, 1, 0, 0, 0, 385, 386,
		1, 0, 0, 0, 386, 399, 1, 0, 0, 0, 387, 389, 3, 62, 31, 0, 388, 387, 1,
		0, 0, 0, 389, 390, 1, 0, 0, 0, 390, 388, 1, 0, 0, 0, 390, 391, 1, 0, 0,
		0, 391, 393, 1, 0, 0, 0, 392, 394, 3, 64, 32, 0, 393, 392, 1, 0, 0, 0,
		393, 394, 1, 0, 0, 0, 394, 396, 1, 0, 0, 0, 395, 397, 3, 30, 15, 0, 396,
		395, 1, 0, 0, 0, 396, 397, 1, 0, 0, 0, 397, 399, 1, 0, 0, 0, 398, 383,
		1, 0, 0, 0, 398, 388, 1, 0, 0, 0, 399, 61, 1, 0, 0, 0, 400, 401, 3, 64,
		32, 0, 401, 402, 5, 33, 0, 0, 402, 405, 1, 0, 0, 0, 403, 405, 5, 33, 0,
		0, 404, 400, 1, 0, 0, 0, 404, 403, 1, 0, 0, 0, 405, 63, 1, 0, 0, 0, 406,
		413, 5, 25, 0, 0, 407, 409, 5, 24, 0, 0, 408, 410, 3, 66, 33, 0, 409, 408,
		1, 0, 0, 0, 409, 410, 1, 0, 0, 0, 410, 411, 1, 0, 0, 0, 411, 413, 5, 38,
		0, 0, 412, 406, 1, 0, 0, 0, 412, 407, 1, 0, 0, 0, 412, 413, 1, 0, 0, 0,
		413, 414, 1, 0, 0, 0, 414, 415, 3, 22, 11, 0, 415, 65, 1, 0, 0, 0, 416,
		421, 3, 68, 34, 0, 417, 418, 5, 33, 0, 0, 418, 420, 3, 68, 34, 0, 419,
		417, 1, 0, 0, 0, 420, 423, 1, 0, 0, 0, 421, 419, 1, 0, 0, 0, 421, 422,
		1, 0, 0, 0, 422, 425, 1, 0, 0, 0, 423, 421, 1, 0, 0, 0, 424, 426, 5, 33,
		0, 0, 425, 424, 1, 0, 0, 0, 425, 426, 1, 0, 0, 0, 426, 67, 1, 0, 0, 0,
		427, 429, 3, 28, 14, 0, 428, 427, 1, 0, 0, 0, 428, 429, 1, 0, 0, 0, 429,
		430, 1, 0, 0, 0, 430, 431, 3, 92, 46, 0, 431, 432, 5, 41, 0, 0, 432, 434,
		3, 70, 35, 0, 433, 435, 3, 30, 15, 0, 434, 433, 1, 0, 0, 0, 434, 435, 1,
		0, 0, 0, 435, 69, 1, 0, 0, 0, 436, 439, 3, 92, 46, 0, 437, 439, 3, 84,
		42, 0, 438, 436, 1, 0, 0, 0, 438, 437, 1, 0, 0, 0, 439, 445, 1, 0, 0, 0,
		440, 441, 5, 47, 0, 0, 441, 442, 3, 22, 11, 0, 442, 443, 5, 47, 0, 0, 443,
		445, 1, 0, 0, 0, 444, 438, 1, 0, 0, 0, 444, 440, 1, 0, 0, 0, 445, 71, 1,
		0, 0, 0, 446, 449, 3, 74, 37, 0, 447, 449, 3, 84, 42, 0, 448, 446, 1, 0,
		0, 0, 448, 447, 1, 0, 0, 0, 449, 73, 1, 0, 0, 0, 450, 454, 5, 44, 0, 0,
		451, 454, 3, 92, 46, 0, 452, 454, 3, 90, 45, 0, 453, 450, 1, 0, 0, 0, 453,
		451, 1, 0, 0, 0, 453, 452, 1, 0, 0, 0, 454, 75, 1, 0, 0, 0, 455, 465, 5,
		35, 0, 0, 456, 461, 3, 2, 1, 0, 457, 458, 5, 33, 0, 0, 458, 460, 3, 2,
		1, 0, 459, 457, 1, 0, 0, 0, 460, 463, 1, 0, 0, 0, 461, 459, 1, 0, 0, 0,
		461, 462, 1, 0, 0, 0, 462, 466, 1, 0, 0, 0, 463, 461, 1, 0, 0, 0, 464,
		466, 5, 8, 0, 0, 465, 456, 1, 0, 0, 0, 465, 464, 1, 0, 0, 0, 466, 467,
		1, 0, 0, 0, 467, 491, 5, 36, 0, 0, 468, 484, 5, 37, 0, 0, 469, 470, 3,
		92, 46, 0, 470, 471, 5, 41, 0, 0, 471, 472, 1, 0, 0, 0, 472, 481, 3, 2,
		1, 0, 473, 474, 5, 2, 0, 0, 474, 475, 3, 92, 46, 0, 475, 476, 5, 41, 0,
		0, 476, 477, 1, 0, 0, 0, 477, 478, 3, 2, 1, 0, 478, 480, 1, 0, 0, 0, 479,
		473, 1, 0, 0, 0, 480, 483, 1, 0, 0, 0, 481, 479, 1, 0, 0, 0, 481, 482,
		1, 0, 0, 0, 482, 485, 1, 0, 0, 0, 483, 481, 1, 0, 0, 0, 484, 469, 1, 0,
		0, 0, 484, 485, 1, 0, 0, 0, 485, 487, 1, 0, 0, 0, 486, 488, 5, 2, 0, 0,
		487, 486, 1, 0, 0, 0, 487, 488, 1, 0, 0, 0, 488, 489, 1, 0, 0, 0, 489,
		491, 5, 38, 0, 0, 490, 455, 1, 0, 0, 0, 490, 468, 1, 0, 0, 0, 491, 77,
		1, 0, 0, 0, 492, 497, 3, 88, 44, 0, 493, 494, 5, 33, 0, 0, 494, 496, 3,
		88, 44, 0, 495, 493, 1, 0, 0, 0, 496, 499, 1, 0, 0, 0, 497, 495, 1, 0,
		0, 0, 497, 498, 1, 0, 0, 0, 498, 501, 1, 0, 0, 0, 499, 497, 1, 0, 0, 0,
		500, 502, 5, 33, 0, 0, 501, 500, 1, 0, 0, 0, 501, 502, 1, 0, 0, 0, 502,
		79, 1, 0, 0, 0, 503, 504, 7, 0, 0, 0, 504, 81, 1, 0, 0, 0, 505, 506, 6,
		41, -1, 0, 506, 507, 5, 32, 0, 0, 507, 508, 3, 82, 41, 0, 508, 509, 5,
		34, 0, 0, 509, 545, 1, 0, 0, 0, 510, 545, 3, 52, 26, 0, 511, 512, 5, 75,
		0, 0, 512, 513, 5, 41, 0, 0, 513, 518, 3, 96, 48, 0, 514, 515, 5, 33, 0,
		0, 515, 517, 3, 96, 48, 0, 516, 514, 1, 0, 0, 0, 517, 520, 1, 0, 0, 0,
		518, 516, 1, 0, 0, 0, 518, 519, 1, 0, 0, 0, 519, 522, 1, 0, 0, 0, 520,
		518, 1, 0, 0, 0, 521, 523, 5, 33, 0, 0, 522, 521, 1, 0, 0, 0, 522, 523,
		1, 0, 0, 0, 523, 545, 1, 0, 0, 0, 524, 525, 5, 76, 0, 0, 525, 526, 5, 41,
		0, 0, 526, 545, 3, 78, 39, 0, 527, 528, 5, 77, 0, 0, 528, 529, 5, 41, 0,
		0, 529, 545, 3, 78, 39, 0, 530, 531, 3, 80, 40, 0, 531, 532, 3, 82, 41,
		5, 532, 545, 1, 0, 0, 0, 533, 537, 7, 1, 0, 0, 534, 538, 3, 84, 42, 0,
		535, 538, 3, 92, 46, 0, 536, 538, 3, 100, 50, 0, 537, 534, 1, 0, 0, 0,
		537, 535, 1, 0, 0, 0, 537, 536, 1, 0, 0, 0, 538, 545, 1, 0, 0, 0, 539,
		542, 7, 2, 0, 0, 540, 543, 3, 86, 43, 0, 541, 543, 3, 90, 45, 0, 542, 540,
		1, 0, 0, 0, 542, 541, 1, 0, 0, 0, 543, 545, 1, 0, 0, 0, 544, 505, 1, 0,
		0, 0, 544, 510, 1, 0, 0, 0, 544, 511, 1, 0, 0, 0, 544, 524, 1, 0, 0, 0,
		544, 527, 1, 0, 0, 0, 544, 530, 1, 0, 0, 0, 544, 533, 1, 0, 0, 0, 544,
		539, 1, 0, 0, 0, 545, 554, 1, 0, 0, 0, 546, 547, 10, 2, 0, 0, 547, 548,
		5, 18, 0, 0, 548, 553, 3, 82, 41, 3, 549, 550, 10, 1, 0, 0, 550, 551, 5,
		19, 0, 0, 551, 553, 3, 82, 41, 2, 552, 546, 1, 0, 0, 0, 552, 549, 1, 0,
		0, 0, 553, 556, 1, 0, 0, 0, 554, 552, 1, 0, 0, 0, 554, 555, 1, 0, 0, 0,
		555, 83, 1, 0, 0, 0, 556, 554, 1, 0, 0, 0, 557, 558, 7, 3, 0, 0, 558, 85,
		1, 0, 0, 0, 559, 562, 3, 92, 46, 0, 560, 562, 5, 44, 0, 0, 561, 559, 1,
		0, 0, 0, 561, 560, 1, 0, 0, 0, 562, 87, 1, 0, 0, 0, 563, 566, 3, 92, 46,
		0, 564, 566, 3, 90, 45, 0, 565, 563, 1, 0, 0, 0, 565, 564, 1, 0, 0, 0,
		566, 89, 1, 0, 0, 0, 567, 568, 5, 83, 0, 0, 568, 91, 1, 0, 0, 0, 569, 573,
		5, 80, 0, 0, 570, 573, 3, 94, 47, 0, 571, 573, 5, 82, 0, 0, 572, 569, 1,
		0, 0, 0, 572, 570, 1, 0, 0, 0, 572, 571, 1, 0, 0, 0, 573, 93, 1, 0, 0,
		0, 574, 587, 3, 98, 49, 0, 575, 587, 3, 96, 48, 0, 576, 587, 5, 75, 0,
		0, 577, 587, 5, 63, 0, 0, 578, 587, 5, 64, 0, 0, 579, 587, 5, 65, 0, 0,
		580, 587, 5, 66, 0, 0, 581, 587, 5, 67, 0, 0, 582, 587, 5, 68, 0, 0, 583,
		587, 5, 76, 0, 0, 584, 587, 5, 77, 0, 0, 585, 587, 5, 61, 0, 0, 586, 574,
		1, 0, 0, 0, 586, 575, 1, 0, 0, 0, 586, 576, 1, 0, 0, 0, 586, 577, 1, 0,
		0, 0, 586, 578, 1, 0, 0, 0, 586, 579, 1, 0, 0, 0, 586, 580, 1, 0, 0, 0,
		586, 581, 1, 0, 0, 0, 586, 582, 1, 0, 0, 0, 586, 583, 1, 0, 0, 0, 586,
		584, 1, 0, 0, 0, 586, 585, 1, 0, 0, 0, 587, 95, 1, 0, 0, 0, 588, 589, 7,
		4, 0, 0, 589, 97, 1, 0, 0, 0, 590, 591, 7, 5, 0, 0, 591, 99, 1, 0, 0, 0,
		592, 593, 5, 61, 0, 0, 593, 101, 1, 0, 0, 0, 89, 108, 112, 116, 120, 124,
		128, 132, 135, 141, 147, 151, 156, 162, 166, 170, 174, 178, 181, 184, 198,
		205, 210, 215, 217, 223, 230, 235, 240, 243, 246, 251, 256, 260, 263, 267,
		273, 275, 281, 286, 289, 303, 308, 312, 317, 322, 325, 341, 347, 354, 360,
		369, 373, 378, 381, 385, 390, 393, 396, 398, 404, 409, 412, 421, 425, 428,
		434, 438, 444, 448, 453, 461, 465, 481, 484, 487, 490, 497, 501, 518, 522,
		537, 542, 544, 552, 554, 561, 565, 572, 586,
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

// SyntaxFlowParserInit initializes any static state used to implement SyntaxFlowParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewSyntaxFlowParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func SyntaxFlowParserInit() {
	staticData := &syntaxflowParserStaticData
	staticData.once.Do(syntaxflowParserInit)
}

// NewSyntaxFlowParser produces a new parser instance for the optional input antlr.TokenStream.
func NewSyntaxFlowParser(input antlr.TokenStream) *SyntaxFlowParser {
	SyntaxFlowParserInit()
	this := new(SyntaxFlowParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &syntaxflowParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.predictionContextCache)
	this.RuleNames = staticData.ruleNames
	this.LiteralNames = staticData.literalNames
	this.SymbolicNames = staticData.symbolicNames
	this.GrammarFileName = "java-escape"

	return this
}

// SyntaxFlowParser tokens.
const (
	SyntaxFlowParserEOF                 = antlr.TokenEOF
	SyntaxFlowParserT__0                = 1
	SyntaxFlowParserT__1                = 2
	SyntaxFlowParserT__2                = 3
	SyntaxFlowParserT__3                = 4
	SyntaxFlowParserT__4                = 5
	SyntaxFlowParserT__5                = 6
	SyntaxFlowParserDeepFilter          = 7
	SyntaxFlowParserDeep                = 8
	SyntaxFlowParserPercent             = 9
	SyntaxFlowParserDeepDot             = 10
	SyntaxFlowParserLtEq                = 11
	SyntaxFlowParserGtEq                = 12
	SyntaxFlowParserDoubleGt            = 13
	SyntaxFlowParserFilter              = 14
	SyntaxFlowParserEqEq                = 15
	SyntaxFlowParserRegexpMatch         = 16
	SyntaxFlowParserNotRegexpMatch      = 17
	SyntaxFlowParserAnd                 = 18
	SyntaxFlowParserOr                  = 19
	SyntaxFlowParserNotEq               = 20
	SyntaxFlowParserConditionStart      = 21
	SyntaxFlowParserDeepNextStart       = 22
	SyntaxFlowParserDeepNextEnd         = 23
	SyntaxFlowParserTopDefStart         = 24
	SyntaxFlowParserDefStart            = 25
	SyntaxFlowParserTopDef              = 26
	SyntaxFlowParserGt                  = 27
	SyntaxFlowParserDot                 = 28
	SyntaxFlowParserLt                  = 29
	SyntaxFlowParserEq                  = 30
	SyntaxFlowParserQuestion            = 31
	SyntaxFlowParserOpenParen           = 32
	SyntaxFlowParserComma               = 33
	SyntaxFlowParserCloseParen          = 34
	SyntaxFlowParserListSelectOpen      = 35
	SyntaxFlowParserListSelectClose     = 36
	SyntaxFlowParserMapBuilderOpen      = 37
	SyntaxFlowParserMapBuilderClose     = 38
	SyntaxFlowParserListStart           = 39
	SyntaxFlowParserDollarOutput        = 40
	SyntaxFlowParserColon               = 41
	SyntaxFlowParserSearch              = 42
	SyntaxFlowParserBang                = 43
	SyntaxFlowParserStar                = 44
	SyntaxFlowParserMinus               = 45
	SyntaxFlowParserAs                  = 46
	SyntaxFlowParserBacktick            = 47
	SyntaxFlowParserSingleQuote         = 48
	SyntaxFlowParserDoubleQuote         = 49
	SyntaxFlowParserLineComment         = 50
	SyntaxFlowParserWhiteSpace          = 51
	SyntaxFlowParserNumber              = 52
	SyntaxFlowParserOctalNumber         = 53
	SyntaxFlowParserBinaryNumber        = 54
	SyntaxFlowParserHexNumber           = 55
	SyntaxFlowParserStringType          = 56
	SyntaxFlowParserListType            = 57
	SyntaxFlowParserDictType            = 58
	SyntaxFlowParserNumberType          = 59
	SyntaxFlowParserBoolType            = 60
	SyntaxFlowParserBoolLiteral         = 61
	SyntaxFlowParserAlert               = 62
	SyntaxFlowParserCheck               = 63
	SyntaxFlowParserThen                = 64
	SyntaxFlowParserDesc                = 65
	SyntaxFlowParserElse                = 66
	SyntaxFlowParserType                = 67
	SyntaxFlowParserIn                  = 68
	SyntaxFlowParserCall                = 69
	SyntaxFlowParserFunction            = 70
	SyntaxFlowParserConstant            = 71
	SyntaxFlowParserPhi                 = 72
	SyntaxFlowParserFormalParam         = 73
	SyntaxFlowParserReturn              = 74
	SyntaxFlowParserOpcode              = 75
	SyntaxFlowParserHave                = 76
	SyntaxFlowParserHaveAny             = 77
	SyntaxFlowParserNot                 = 78
	SyntaxFlowParserFor                 = 79
	SyntaxFlowParserIdentifier          = 80
	SyntaxFlowParserIdentifierChar      = 81
	SyntaxFlowParserQuotedStringLiteral = 82
	SyntaxFlowParserRegexpLiteral       = 83
	SyntaxFlowParserWS                  = 84
)

// SyntaxFlowParser rules.
const (
	SyntaxFlowParserRULE_flow                              = 0
	SyntaxFlowParserRULE_statements                        = 1
	SyntaxFlowParserRULE_statement                         = 2
	SyntaxFlowParserRULE_fileFilterContentStatement        = 3
	SyntaxFlowParserRULE_fileFilterContentInput            = 4
	SyntaxFlowParserRULE_fileFilterContentMethod           = 5
	SyntaxFlowParserRULE_fileFilterContentMethodParam      = 6
	SyntaxFlowParserRULE_fileFilterContentMethodParamItem  = 7
	SyntaxFlowParserRULE_fileFilterContentMethodParamKey   = 8
	SyntaxFlowParserRULE_fileFilterContentMethodParamValue = 9
	SyntaxFlowParserRULE_fileName                          = 10
	SyntaxFlowParserRULE_filterStatement                   = 11
	SyntaxFlowParserRULE_comment                           = 12
	SyntaxFlowParserRULE_eos                               = 13
	SyntaxFlowParserRULE_line                              = 14
	SyntaxFlowParserRULE_lines                             = 15
	SyntaxFlowParserRULE_descriptionStatement              = 16
	SyntaxFlowParserRULE_descriptionItems                  = 17
	SyntaxFlowParserRULE_descriptionItem                   = 18
	SyntaxFlowParserRULE_alertStatement                    = 19
	SyntaxFlowParserRULE_checkStatement                    = 20
	SyntaxFlowParserRULE_thenExpr                          = 21
	SyntaxFlowParserRULE_elseExpr                          = 22
	SyntaxFlowParserRULE_refVariable                       = 23
	SyntaxFlowParserRULE_filterItemFirst                   = 24
	SyntaxFlowParserRULE_filterItem                        = 25
	SyntaxFlowParserRULE_filterExpr                        = 26
	SyntaxFlowParserRULE_nativeCall                        = 27
	SyntaxFlowParserRULE_useNativeCall                     = 28
	SyntaxFlowParserRULE_useDefCalcParams                  = 29
	SyntaxFlowParserRULE_actualParam                       = 30
	SyntaxFlowParserRULE_actualParamFilter                 = 31
	SyntaxFlowParserRULE_singleParam                       = 32
	SyntaxFlowParserRULE_config                            = 33
	SyntaxFlowParserRULE_recursiveConfigItem               = 34
	SyntaxFlowParserRULE_recursiveConfigItemValue          = 35
	SyntaxFlowParserRULE_sliceCallItem                     = 36
	SyntaxFlowParserRULE_nameFilter                        = 37
	SyntaxFlowParserRULE_chainFilter                       = 38
	SyntaxFlowParserRULE_stringLiteralWithoutStarGroup     = 39
	SyntaxFlowParserRULE_negativeCondition                 = 40
	SyntaxFlowParserRULE_conditionExpression               = 41
	SyntaxFlowParserRULE_numberLiteral                     = 42
	SyntaxFlowParserRULE_stringLiteral                     = 43
	SyntaxFlowParserRULE_stringLiteralWithoutStar          = 44
	SyntaxFlowParserRULE_regexpLiteral                     = 45
	SyntaxFlowParserRULE_identifier                        = 46
	SyntaxFlowParserRULE_keywords                          = 47
	SyntaxFlowParserRULE_opcodes                           = 48
	SyntaxFlowParserRULE_types                             = 49
	SyntaxFlowParserRULE_boolLiteral                       = 50
)

// IFlowContext is an interface to support dynamic dispatch.
type IFlowContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFlowContext differentiates from other interfaces.
	IsFlowContext()
}

type FlowContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFlowContext() *FlowContext {
	var p = new(FlowContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_flow
	return p
}

func (*FlowContext) IsFlowContext() {}

func NewFlowContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FlowContext {
	var p = new(FlowContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_flow

	return p
}

func (s *FlowContext) GetParser() antlr.Parser { return s.parser }

func (s *FlowContext) Statements() IStatementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatementsContext)
}

func (s *FlowContext) EOF() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserEOF, 0)
}

func (s *FlowContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FlowContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FlowContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFlow(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Flow() (localctx IFlowContext) {
	this := p
	_ = this

	localctx = NewFlowContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, SyntaxFlowParserRULE_flow)

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
		p.SetState(102)
		p.Statements()
	}
	{
		p.SetState(103)
		p.Match(SyntaxFlowParserEOF)
	}

	return localctx
}

// IStatementsContext is an interface to support dynamic dispatch.
type IStatementsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStatementsContext differentiates from other interfaces.
	IsStatementsContext()
}

type StatementsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStatementsContext() *StatementsContext {
	var p = new(StatementsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_statements
	return p
}

func (*StatementsContext) IsStatementsContext() {}

func NewStatementsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StatementsContext {
	var p = new(StatementsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_statements

	return p
}

func (s *StatementsContext) GetParser() antlr.Parser { return s.parser }

func (s *StatementsContext) AllStatement() []IStatementContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStatementContext); ok {
			len++
		}
	}

	tst := make([]IStatementContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStatementContext); ok {
			tst[i] = t.(IStatementContext)
			i++
		}
	}

	return tst
}

func (s *StatementsContext) Statement(i int) IStatementContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementContext); ok {
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

	return t.(IStatementContext)
}

func (s *StatementsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StatementsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StatementsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitStatements(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Statements() (localctx IStatementsContext) {
	this := p
	_ = this

	localctx = NewStatementsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, SyntaxFlowParserRULE_statements)

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
	p.SetState(106)
	p.GetErrorHandler().Sync(p)
	_alt = 1
	for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		switch _alt {
		case 1:
			{
				p.SetState(105)
				p.Statement()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

		p.SetState(108)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 0, p.GetParserRuleContext())
	}

	return localctx
}

// IStatementContext is an interface to support dynamic dispatch.
type IStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStatementContext differentiates from other interfaces.
	IsStatementContext()
}

type StatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStatementContext() *StatementContext {
	var p = new(StatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_statement
	return p
}

func (*StatementContext) IsStatementContext() {}

func NewStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StatementContext {
	var p = new(StatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_statement

	return p
}

func (s *StatementContext) GetParser() antlr.Parser { return s.parser }

func (s *StatementContext) CopyFrom(ctx *StatementContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *StatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type EmptyContext struct {
	*StatementContext
}

func NewEmptyContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *EmptyContext {
	var p = new(EmptyContext)

	p.StatementContext = NewEmptyStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*StatementContext))

	return p
}

func (s *EmptyContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *EmptyContext) Eos() IEosContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IEosContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IEosContext)
}

func (s *EmptyContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitEmpty(s)

	default:
		return t.VisitChildren(s)
	}
}

type DescriptionContext struct {
	*StatementContext
}

func NewDescriptionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *DescriptionContext {
	var p = new(DescriptionContext)

	p.StatementContext = NewEmptyStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*StatementContext))

	return p
}

func (s *DescriptionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DescriptionContext) DescriptionStatement() IDescriptionStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDescriptionStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDescriptionStatementContext)
}

func (s *DescriptionContext) Eos() IEosContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IEosContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IEosContext)
}

func (s *DescriptionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitDescription(s)

	default:
		return t.VisitChildren(s)
	}
}

type FilterContext struct {
	*StatementContext
}

func NewFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FilterContext {
	var p = new(FilterContext)

	p.StatementContext = NewEmptyStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*StatementContext))

	return p
}

func (s *FilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterContext) FilterStatement() IFilterStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFilterStatementContext)
}

func (s *FilterContext) Eos() IEosContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IEosContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IEosContext)
}

func (s *FilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type FileFilterContentContext struct {
	*StatementContext
}

func NewFileFilterContentContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FileFilterContentContext {
	var p = new(FileFilterContentContext)

	p.StatementContext = NewEmptyStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*StatementContext))

	return p
}

func (s *FileFilterContentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FileFilterContentContext) FileFilterContentStatement() IFileFilterContentStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFileFilterContentStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFileFilterContentStatementContext)
}

func (s *FileFilterContentContext) Eos() IEosContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IEosContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IEosContext)
}

func (s *FileFilterContentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFileFilterContent(s)

	default:
		return t.VisitChildren(s)
	}
}

type CommandContext struct {
	*StatementContext
}

func NewCommandContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *CommandContext {
	var p = new(CommandContext)

	p.StatementContext = NewEmptyStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*StatementContext))

	return p
}

func (s *CommandContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *CommandContext) Comment() ICommentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ICommentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ICommentContext)
}

func (s *CommandContext) Eos() IEosContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IEosContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IEosContext)
}

func (s *CommandContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitCommand(s)

	default:
		return t.VisitChildren(s)
	}
}

type CheckContext struct {
	*StatementContext
}

func NewCheckContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *CheckContext {
	var p = new(CheckContext)

	p.StatementContext = NewEmptyStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*StatementContext))

	return p
}

func (s *CheckContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *CheckContext) CheckStatement() ICheckStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ICheckStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ICheckStatementContext)
}

func (s *CheckContext) Eos() IEosContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IEosContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IEosContext)
}

func (s *CheckContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitCheck(s)

	default:
		return t.VisitChildren(s)
	}
}

type AlertContext struct {
	*StatementContext
}

func NewAlertContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *AlertContext {
	var p = new(AlertContext)

	p.StatementContext = NewEmptyStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*StatementContext))

	return p
}

func (s *AlertContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AlertContext) AlertStatement() IAlertStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAlertStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAlertStatementContext)
}

func (s *AlertContext) Eos() IEosContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IEosContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IEosContext)
}

func (s *AlertContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitAlert(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Statement() (localctx IStatementContext) {
	this := p
	_ = this

	localctx = NewStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, SyntaxFlowParserRULE_statement)

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

	p.SetState(135)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 7, p.GetParserRuleContext()) {
	case 1:
		localctx = NewCheckContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(110)
			p.CheckStatement()
		}
		p.SetState(112)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 1, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(111)
				p.Eos()
			}

		}

	case 2:
		localctx = NewDescriptionContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(114)
			p.DescriptionStatement()
		}
		p.SetState(116)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 2, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(115)
				p.Eos()
			}

		}

	case 3:
		localctx = NewAlertContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(118)
			p.AlertStatement()
		}
		p.SetState(120)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 3, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(119)
				p.Eos()
			}

		}

	case 4:
		localctx = NewFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(122)
			p.FilterStatement()
		}
		p.SetState(124)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 4, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(123)
				p.Eos()
			}

		}

	case 5:
		localctx = NewFileFilterContentContext(p, localctx)
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(126)
			p.FileFilterContentStatement()
		}
		p.SetState(128)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 5, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(127)
				p.Eos()
			}

		}

	case 6:
		localctx = NewCommandContext(p, localctx)
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(130)
			p.Comment()
		}
		p.SetState(132)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 6, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(131)
				p.Eos()
			}

		}

	case 7:
		localctx = NewEmptyContext(p, localctx)
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(134)
			p.Eos()
		}

	}

	return localctx
}

// IFileFilterContentStatementContext is an interface to support dynamic dispatch.
type IFileFilterContentStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFileFilterContentStatementContext differentiates from other interfaces.
	IsFileFilterContentStatementContext()
}

type FileFilterContentStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFileFilterContentStatementContext() *FileFilterContentStatementContext {
	var p = new(FileFilterContentStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentStatement
	return p
}

func (*FileFilterContentStatementContext) IsFileFilterContentStatementContext() {}

func NewFileFilterContentStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FileFilterContentStatementContext {
	var p = new(FileFilterContentStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentStatement

	return p
}

func (s *FileFilterContentStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *FileFilterContentStatementContext) FileFilterContentInput() IFileFilterContentInputContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFileFilterContentInputContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFileFilterContentInputContext)
}

func (s *FileFilterContentStatementContext) MapBuilderClose() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserMapBuilderClose, 0)
}

func (s *FileFilterContentStatementContext) Dot() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDot, 0)
}

func (s *FileFilterContentStatementContext) FileFilterContentMethod() IFileFilterContentMethodContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFileFilterContentMethodContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFileFilterContentMethodContext)
}

func (s *FileFilterContentStatementContext) Lines() ILinesContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILinesContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILinesContext)
}

func (s *FileFilterContentStatementContext) As() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserAs, 0)
}

func (s *FileFilterContentStatementContext) RefVariable() IRefVariableContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRefVariableContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRefVariableContext)
}

func (s *FileFilterContentStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FileFilterContentStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FileFilterContentStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFileFilterContentStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FileFilterContentStatement() (localctx IFileFilterContentStatementContext) {
	this := p
	_ = this

	localctx = NewFileFilterContentStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, SyntaxFlowParserRULE_fileFilterContentStatement)
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
		p.SetState(137)
		p.Match(SyntaxFlowParserT__0)
	}
	{
		p.SetState(138)
		p.FileFilterContentInput()
	}
	{
		p.SetState(139)
		p.Match(SyntaxFlowParserMapBuilderClose)
	}
	p.SetState(141)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserT__2 {
		{
			p.SetState(140)
			p.Lines()
		}

	}
	{
		p.SetState(143)
		p.Match(SyntaxFlowParserDot)
	}
	{
		p.SetState(144)
		p.FileFilterContentMethod()
	}
	p.SetState(147)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserAs {
		{
			p.SetState(145)
			p.Match(SyntaxFlowParserAs)
		}
		{
			p.SetState(146)
			p.RefVariable()
		}

	}

	return localctx
}

// IFileFilterContentInputContext is an interface to support dynamic dispatch.
type IFileFilterContentInputContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFileFilterContentInputContext differentiates from other interfaces.
	IsFileFilterContentInputContext()
}

type FileFilterContentInputContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFileFilterContentInputContext() *FileFilterContentInputContext {
	var p = new(FileFilterContentInputContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentInput
	return p
}

func (*FileFilterContentInputContext) IsFileFilterContentInputContext() {}

func NewFileFilterContentInputContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FileFilterContentInputContext {
	var p = new(FileFilterContentInputContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentInput

	return p
}

func (s *FileFilterContentInputContext) GetParser() antlr.Parser { return s.parser }

func (s *FileFilterContentInputContext) FileName() IFileNameContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFileNameContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFileNameContext)
}

func (s *FileFilterContentInputContext) RegexpLiteral() IRegexpLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRegexpLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRegexpLiteralContext)
}

func (s *FileFilterContentInputContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FileFilterContentInputContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FileFilterContentInputContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFileFilterContentInput(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FileFilterContentInput() (localctx IFileFilterContentInputContext) {
	this := p
	_ = this

	localctx = NewFileFilterContentInputContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, SyntaxFlowParserRULE_fileFilterContentInput)

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
	p.SetState(151)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 10, p.GetParserRuleContext()) {
	case 1:
		{
			p.SetState(149)
			p.FileName()
		}

	case 2:
		{
			p.SetState(150)
			p.RegexpLiteral()
		}

	}

	return localctx
}

// IFileFilterContentMethodContext is an interface to support dynamic dispatch.
type IFileFilterContentMethodContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFileFilterContentMethodContext differentiates from other interfaces.
	IsFileFilterContentMethodContext()
}

type FileFilterContentMethodContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFileFilterContentMethodContext() *FileFilterContentMethodContext {
	var p = new(FileFilterContentMethodContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentMethod
	return p
}

func (*FileFilterContentMethodContext) IsFileFilterContentMethodContext() {}

func NewFileFilterContentMethodContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FileFilterContentMethodContext {
	var p = new(FileFilterContentMethodContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentMethod

	return p
}

func (s *FileFilterContentMethodContext) GetParser() antlr.Parser { return s.parser }

func (s *FileFilterContentMethodContext) Identifier() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserIdentifier, 0)
}

func (s *FileFilterContentMethodContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserOpenParen, 0)
}

func (s *FileFilterContentMethodContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCloseParen, 0)
}

func (s *FileFilterContentMethodContext) FileFilterContentMethodParam() IFileFilterContentMethodParamContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFileFilterContentMethodParamContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFileFilterContentMethodParamContext)
}

func (s *FileFilterContentMethodContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FileFilterContentMethodContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FileFilterContentMethodContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFileFilterContentMethod(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FileFilterContentMethod() (localctx IFileFilterContentMethodContext) {
	this := p
	_ = this

	localctx = NewFileFilterContentMethodContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, SyntaxFlowParserRULE_fileFilterContentMethod)
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
		p.SetState(153)
		p.Match(SyntaxFlowParserIdentifier)
	}
	{
		p.SetState(154)
		p.Match(SyntaxFlowParserOpenParen)
	}
	p.SetState(156)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if (int64((_la-44)) & ^0x3f) == 0 && ((int64(1)<<(_la-44))&910532800513) != 0 {
		{
			p.SetState(155)
			p.FileFilterContentMethodParam()
		}

	}
	{
		p.SetState(158)
		p.Match(SyntaxFlowParserCloseParen)
	}

	return localctx
}

// IFileFilterContentMethodParamContext is an interface to support dynamic dispatch.
type IFileFilterContentMethodParamContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFileFilterContentMethodParamContext differentiates from other interfaces.
	IsFileFilterContentMethodParamContext()
}

type FileFilterContentMethodParamContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFileFilterContentMethodParamContext() *FileFilterContentMethodParamContext {
	var p = new(FileFilterContentMethodParamContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentMethodParam
	return p
}

func (*FileFilterContentMethodParamContext) IsFileFilterContentMethodParamContext() {}

func NewFileFilterContentMethodParamContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FileFilterContentMethodParamContext {
	var p = new(FileFilterContentMethodParamContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentMethodParam

	return p
}

func (s *FileFilterContentMethodParamContext) GetParser() antlr.Parser { return s.parser }

func (s *FileFilterContentMethodParamContext) AllFileFilterContentMethodParamItem() []IFileFilterContentMethodParamItemContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFileFilterContentMethodParamItemContext); ok {
			len++
		}
	}

	tst := make([]IFileFilterContentMethodParamItemContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFileFilterContentMethodParamItemContext); ok {
			tst[i] = t.(IFileFilterContentMethodParamItemContext)
			i++
		}
	}

	return tst
}

func (s *FileFilterContentMethodParamContext) FileFilterContentMethodParamItem(i int) IFileFilterContentMethodParamItemContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFileFilterContentMethodParamItemContext); ok {
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

	return t.(IFileFilterContentMethodParamItemContext)
}

func (s *FileFilterContentMethodParamContext) AllLines() []ILinesContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ILinesContext); ok {
			len++
		}
	}

	tst := make([]ILinesContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ILinesContext); ok {
			tst[i] = t.(ILinesContext)
			i++
		}
	}

	return tst
}

func (s *FileFilterContentMethodParamContext) Lines(i int) ILinesContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILinesContext); ok {
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

	return t.(ILinesContext)
}

func (s *FileFilterContentMethodParamContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserComma)
}

func (s *FileFilterContentMethodParamContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserComma, i)
}

func (s *FileFilterContentMethodParamContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FileFilterContentMethodParamContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FileFilterContentMethodParamContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFileFilterContentMethodParam(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FileFilterContentMethodParam() (localctx IFileFilterContentMethodParamContext) {
	this := p
	_ = this

	localctx = NewFileFilterContentMethodParamContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, SyntaxFlowParserRULE_fileFilterContentMethodParam)
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
		p.SetState(160)
		p.FileFilterContentMethodParamItem()
	}
	p.SetState(162)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 12, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(161)
			p.Lines()
		}

	}
	p.SetState(174)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 15, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(164)
				p.Match(SyntaxFlowParserComma)
			}
			p.SetState(166)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			if _la == SyntaxFlowParserT__2 {
				{
					p.SetState(165)
					p.Lines()
				}

			}
			{
				p.SetState(168)
				p.FileFilterContentMethodParamItem()
			}
			p.SetState(170)
			p.GetErrorHandler().Sync(p)

			if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 14, p.GetParserRuleContext()) == 1 {
				{
					p.SetState(169)
					p.Lines()
				}

			}

		}
		p.SetState(176)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 15, p.GetParserRuleContext())
	}
	p.SetState(178)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserComma {
		{
			p.SetState(177)
			p.Match(SyntaxFlowParserComma)
		}

	}
	p.SetState(181)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserT__2 {
		{
			p.SetState(180)
			p.Lines()
		}

	}

	return localctx
}

// IFileFilterContentMethodParamItemContext is an interface to support dynamic dispatch.
type IFileFilterContentMethodParamItemContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFileFilterContentMethodParamItemContext differentiates from other interfaces.
	IsFileFilterContentMethodParamItemContext()
}

type FileFilterContentMethodParamItemContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFileFilterContentMethodParamItemContext() *FileFilterContentMethodParamItemContext {
	var p = new(FileFilterContentMethodParamItemContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentMethodParamItem
	return p
}

func (*FileFilterContentMethodParamItemContext) IsFileFilterContentMethodParamItemContext() {}

func NewFileFilterContentMethodParamItemContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FileFilterContentMethodParamItemContext {
	var p = new(FileFilterContentMethodParamItemContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentMethodParamItem

	return p
}

func (s *FileFilterContentMethodParamItemContext) GetParser() antlr.Parser { return s.parser }

func (s *FileFilterContentMethodParamItemContext) FileFilterContentMethodParamValue() IFileFilterContentMethodParamValueContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFileFilterContentMethodParamValueContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFileFilterContentMethodParamValueContext)
}

func (s *FileFilterContentMethodParamItemContext) FileFilterContentMethodParamKey() IFileFilterContentMethodParamKeyContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFileFilterContentMethodParamKeyContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFileFilterContentMethodParamKeyContext)
}

func (s *FileFilterContentMethodParamItemContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FileFilterContentMethodParamItemContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FileFilterContentMethodParamItemContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFileFilterContentMethodParamItem(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FileFilterContentMethodParamItem() (localctx IFileFilterContentMethodParamItemContext) {
	this := p
	_ = this

	localctx = NewFileFilterContentMethodParamItemContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 14, SyntaxFlowParserRULE_fileFilterContentMethodParamItem)

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
	p.SetState(184)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 18, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(183)
			p.FileFilterContentMethodParamKey()
		}

	}
	{
		p.SetState(186)
		p.FileFilterContentMethodParamValue()
	}

	return localctx
}

// IFileFilterContentMethodParamKeyContext is an interface to support dynamic dispatch.
type IFileFilterContentMethodParamKeyContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFileFilterContentMethodParamKeyContext differentiates from other interfaces.
	IsFileFilterContentMethodParamKeyContext()
}

type FileFilterContentMethodParamKeyContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFileFilterContentMethodParamKeyContext() *FileFilterContentMethodParamKeyContext {
	var p = new(FileFilterContentMethodParamKeyContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentMethodParamKey
	return p
}

func (*FileFilterContentMethodParamKeyContext) IsFileFilterContentMethodParamKeyContext() {}

func NewFileFilterContentMethodParamKeyContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FileFilterContentMethodParamKeyContext {
	var p = new(FileFilterContentMethodParamKeyContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentMethodParamKey

	return p
}

func (s *FileFilterContentMethodParamKeyContext) GetParser() antlr.Parser { return s.parser }

func (s *FileFilterContentMethodParamKeyContext) Identifier() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserIdentifier, 0)
}

func (s *FileFilterContentMethodParamKeyContext) Colon() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserColon, 0)
}

func (s *FileFilterContentMethodParamKeyContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FileFilterContentMethodParamKeyContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FileFilterContentMethodParamKeyContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFileFilterContentMethodParamKey(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FileFilterContentMethodParamKey() (localctx IFileFilterContentMethodParamKeyContext) {
	this := p
	_ = this

	localctx = NewFileFilterContentMethodParamKeyContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 16, SyntaxFlowParserRULE_fileFilterContentMethodParamKey)

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
		p.SetState(188)
		p.Match(SyntaxFlowParserIdentifier)
	}
	{
		p.SetState(189)
		p.Match(SyntaxFlowParserColon)
	}

	return localctx
}

// IFileFilterContentMethodParamValueContext is an interface to support dynamic dispatch.
type IFileFilterContentMethodParamValueContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFileFilterContentMethodParamValueContext differentiates from other interfaces.
	IsFileFilterContentMethodParamValueContext()
}

type FileFilterContentMethodParamValueContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFileFilterContentMethodParamValueContext() *FileFilterContentMethodParamValueContext {
	var p = new(FileFilterContentMethodParamValueContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentMethodParamValue
	return p
}

func (*FileFilterContentMethodParamValueContext) IsFileFilterContentMethodParamValueContext() {}

func NewFileFilterContentMethodParamValueContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FileFilterContentMethodParamValueContext {
	var p = new(FileFilterContentMethodParamValueContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_fileFilterContentMethodParamValue

	return p
}

func (s *FileFilterContentMethodParamValueContext) GetParser() antlr.Parser { return s.parser }

func (s *FileFilterContentMethodParamValueContext) NameFilter() INameFilterContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INameFilterContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INameFilterContext)
}

func (s *FileFilterContentMethodParamValueContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FileFilterContentMethodParamValueContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FileFilterContentMethodParamValueContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFileFilterContentMethodParamValue(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FileFilterContentMethodParamValue() (localctx IFileFilterContentMethodParamValueContext) {
	this := p
	_ = this

	localctx = NewFileFilterContentMethodParamValueContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 18, SyntaxFlowParserRULE_fileFilterContentMethodParamValue)

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
		p.SetState(191)
		p.NameFilter()
	}

	return localctx
}

// IFileNameContext is an interface to support dynamic dispatch.
type IFileNameContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFileNameContext differentiates from other interfaces.
	IsFileNameContext()
}

type FileNameContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFileNameContext() *FileNameContext {
	var p = new(FileNameContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_fileName
	return p
}

func (*FileNameContext) IsFileNameContext() {}

func NewFileNameContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FileNameContext {
	var p = new(FileNameContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_fileName

	return p
}

func (s *FileNameContext) GetParser() antlr.Parser { return s.parser }

func (s *FileNameContext) AllNameFilter() []INameFilterContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(INameFilterContext); ok {
			len++
		}
	}

	tst := make([]INameFilterContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(INameFilterContext); ok {
			tst[i] = t.(INameFilterContext)
			i++
		}
	}

	return tst
}

func (s *FileNameContext) NameFilter(i int) INameFilterContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INameFilterContext); ok {
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

	return t.(INameFilterContext)
}

func (s *FileNameContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FileNameContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FileNameContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFileName(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FileName() (localctx IFileNameContext) {
	this := p
	_ = this

	localctx = NewFileNameContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 20, SyntaxFlowParserRULE_fileName)

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
		p.SetState(193)
		p.NameFilter()
	}
	p.SetState(198)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 19, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			p.SetState(194)
			p.MatchWildcard()

			{
				p.SetState(195)
				p.NameFilter()
			}

		}
		p.SetState(200)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 19, p.GetParserRuleContext())
	}

	return localctx
}

// IFilterStatementContext is an interface to support dynamic dispatch.
type IFilterStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFilterStatementContext differentiates from other interfaces.
	IsFilterStatementContext()
}

type FilterStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFilterStatementContext() *FilterStatementContext {
	var p = new(FilterStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_filterStatement
	return p
}

func (*FilterStatementContext) IsFilterStatementContext() {}

func NewFilterStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FilterStatementContext {
	var p = new(FilterStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_filterStatement

	return p
}

func (s *FilterStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *FilterStatementContext) CopyFrom(ctx *FilterStatementContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *FilterStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type PureFilterExprContext struct {
	*FilterStatementContext
}

func NewPureFilterExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *PureFilterExprContext {
	var p = new(PureFilterExprContext)

	p.FilterStatementContext = NewEmptyFilterStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterStatementContext))

	return p
}

func (s *PureFilterExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PureFilterExprContext) FilterExpr() IFilterExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFilterExprContext)
}

func (s *PureFilterExprContext) As() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserAs, 0)
}

func (s *PureFilterExprContext) RefVariable() IRefVariableContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRefVariableContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRefVariableContext)
}

func (s *PureFilterExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitPureFilterExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

type RefFilterExprContext struct {
	*FilterStatementContext
}

func NewRefFilterExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *RefFilterExprContext {
	var p = new(RefFilterExprContext)

	p.FilterStatementContext = NewEmptyFilterStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterStatementContext))

	return p
}

func (s *RefFilterExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RefFilterExprContext) AllRefVariable() []IRefVariableContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IRefVariableContext); ok {
			len++
		}
	}

	tst := make([]IRefVariableContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IRefVariableContext); ok {
			tst[i] = t.(IRefVariableContext)
			i++
		}
	}

	return tst
}

func (s *RefFilterExprContext) RefVariable(i int) IRefVariableContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRefVariableContext); ok {
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

	return t.(IRefVariableContext)
}

func (s *RefFilterExprContext) AllFilterItem() []IFilterItemContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFilterItemContext); ok {
			len++
		}
	}

	tst := make([]IFilterItemContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFilterItemContext); ok {
			tst[i] = t.(IFilterItemContext)
			i++
		}
	}

	return tst
}

func (s *RefFilterExprContext) FilterItem(i int) IFilterItemContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterItemContext); ok {
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

	return t.(IFilterItemContext)
}

func (s *RefFilterExprContext) As() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserAs, 0)
}

func (s *RefFilterExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitRefFilterExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FilterStatement() (localctx IFilterStatementContext) {
	this := p
	_ = this

	localctx = NewFilterStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 22, SyntaxFlowParserRULE_filterStatement)
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

	p.SetState(217)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDollarOutput:
		localctx = NewRefFilterExprContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(201)
			p.RefVariable()
		}
		p.SetState(205)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 20, p.GetParserRuleContext())

		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(202)
					p.FilterItem()
				}

			}
			p.SetState(207)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 20, p.GetParserRuleContext())
		}
		p.SetState(210)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserAs {
			{
				p.SetState(208)
				p.Match(SyntaxFlowParserAs)
			}
			{
				p.SetState(209)
				p.RefVariable()
			}

		}

	case SyntaxFlowParserDot, SyntaxFlowParserLt, SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		localctx = NewPureFilterExprContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(212)
			p.FilterExpr()
		}
		p.SetState(215)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserAs {
			{
				p.SetState(213)
				p.Match(SyntaxFlowParserAs)
			}
			{
				p.SetState(214)
				p.RefVariable()
			}

		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// ICommentContext is an interface to support dynamic dispatch.
type ICommentContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsCommentContext differentiates from other interfaces.
	IsCommentContext()
}

type CommentContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyCommentContext() *CommentContext {
	var p = new(CommentContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_comment
	return p
}

func (*CommentContext) IsCommentContext() {}

func NewCommentContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *CommentContext {
	var p = new(CommentContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_comment

	return p
}

func (s *CommentContext) GetParser() antlr.Parser { return s.parser }

func (s *CommentContext) LineComment() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserLineComment, 0)
}

func (s *CommentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *CommentContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *CommentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitComment(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Comment() (localctx ICommentContext) {
	this := p
	_ = this

	localctx = NewCommentContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, SyntaxFlowParserRULE_comment)

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
		p.SetState(219)
		p.Match(SyntaxFlowParserLineComment)
	}

	return localctx
}

// IEosContext is an interface to support dynamic dispatch.
type IEosContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsEosContext differentiates from other interfaces.
	IsEosContext()
}

type EosContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyEosContext() *EosContext {
	var p = new(EosContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_eos
	return p
}

func (*EosContext) IsEosContext() {}

func NewEosContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *EosContext {
	var p = new(EosContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_eos

	return p
}

func (s *EosContext) GetParser() antlr.Parser { return s.parser }

func (s *EosContext) Line() ILineContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILineContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILineContext)
}

func (s *EosContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *EosContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *EosContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitEos(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Eos() (localctx IEosContext) {
	this := p
	_ = this

	localctx = NewEosContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, SyntaxFlowParserRULE_eos)

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

	p.SetState(223)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserT__1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(221)
			p.Match(SyntaxFlowParserT__1)
		}

	case SyntaxFlowParserT__2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(222)
			p.Line()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// ILineContext is an interface to support dynamic dispatch.
type ILineContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsLineContext differentiates from other interfaces.
	IsLineContext()
}

type LineContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyLineContext() *LineContext {
	var p = new(LineContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_line
	return p
}

func (*LineContext) IsLineContext() {}

func NewLineContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *LineContext {
	var p = new(LineContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_line

	return p
}

func (s *LineContext) GetParser() antlr.Parser { return s.parser }
func (s *LineContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LineContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *LineContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitLine(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Line() (localctx ILineContext) {
	this := p
	_ = this

	localctx = NewLineContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 28, SyntaxFlowParserRULE_line)

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
		p.SetState(225)
		p.Match(SyntaxFlowParserT__2)
	}

	return localctx
}

// ILinesContext is an interface to support dynamic dispatch.
type ILinesContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsLinesContext differentiates from other interfaces.
	IsLinesContext()
}

type LinesContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyLinesContext() *LinesContext {
	var p = new(LinesContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_lines
	return p
}

func (*LinesContext) IsLinesContext() {}

func NewLinesContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *LinesContext {
	var p = new(LinesContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_lines

	return p
}

func (s *LinesContext) GetParser() antlr.Parser { return s.parser }

func (s *LinesContext) AllLine() []ILineContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ILineContext); ok {
			len++
		}
	}

	tst := make([]ILineContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ILineContext); ok {
			tst[i] = t.(ILineContext)
			i++
		}
	}

	return tst
}

func (s *LinesContext) Line(i int) ILineContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILineContext); ok {
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

	return t.(ILineContext)
}

func (s *LinesContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LinesContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *LinesContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitLines(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Lines() (localctx ILinesContext) {
	this := p
	_ = this

	localctx = NewLinesContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 30, SyntaxFlowParserRULE_lines)

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
	p.SetState(228)
	p.GetErrorHandler().Sync(p)
	_alt = 1
	for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		switch _alt {
		case 1:
			{
				p.SetState(227)
				p.Line()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

		p.SetState(230)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 25, p.GetParserRuleContext())
	}

	return localctx
}

// IDescriptionStatementContext is an interface to support dynamic dispatch.
type IDescriptionStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDescriptionStatementContext differentiates from other interfaces.
	IsDescriptionStatementContext()
}

type DescriptionStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDescriptionStatementContext() *DescriptionStatementContext {
	var p = new(DescriptionStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_descriptionStatement
	return p
}

func (*DescriptionStatementContext) IsDescriptionStatementContext() {}

func NewDescriptionStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DescriptionStatementContext {
	var p = new(DescriptionStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_descriptionStatement

	return p
}

func (s *DescriptionStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *DescriptionStatementContext) Desc() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDesc, 0)
}

func (s *DescriptionStatementContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserOpenParen, 0)
}

func (s *DescriptionStatementContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCloseParen, 0)
}

func (s *DescriptionStatementContext) DescriptionItems() IDescriptionItemsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDescriptionItemsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDescriptionItemsContext)
}

func (s *DescriptionStatementContext) MapBuilderOpen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserMapBuilderOpen, 0)
}

func (s *DescriptionStatementContext) MapBuilderClose() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserMapBuilderClose, 0)
}

func (s *DescriptionStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DescriptionStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DescriptionStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitDescriptionStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) DescriptionStatement() (localctx IDescriptionStatementContext) {
	this := p
	_ = this

	localctx = NewDescriptionStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 32, SyntaxFlowParserRULE_descriptionStatement)
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

	p.SetState(243)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDesc:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(232)
			p.Match(SyntaxFlowParserDesc)
		}

		{
			p.SetState(233)
			p.Match(SyntaxFlowParserOpenParen)
		}
		p.SetState(235)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-4683726020279271416) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&344063) != 0 {
			{
				p.SetState(234)
				p.DescriptionItems()
			}

		}
		{
			p.SetState(237)
			p.Match(SyntaxFlowParserCloseParen)
		}

	case SyntaxFlowParserMapBuilderOpen:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(238)
			p.Match(SyntaxFlowParserMapBuilderOpen)
		}
		p.SetState(240)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-4683726020279271416) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&344063) != 0 {
			{
				p.SetState(239)
				p.DescriptionItems()
			}

		}
		{
			p.SetState(242)
			p.Match(SyntaxFlowParserMapBuilderClose)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IDescriptionItemsContext is an interface to support dynamic dispatch.
type IDescriptionItemsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDescriptionItemsContext differentiates from other interfaces.
	IsDescriptionItemsContext()
}

type DescriptionItemsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDescriptionItemsContext() *DescriptionItemsContext {
	var p = new(DescriptionItemsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_descriptionItems
	return p
}

func (*DescriptionItemsContext) IsDescriptionItemsContext() {}

func NewDescriptionItemsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DescriptionItemsContext {
	var p = new(DescriptionItemsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_descriptionItems

	return p
}

func (s *DescriptionItemsContext) GetParser() antlr.Parser { return s.parser }

func (s *DescriptionItemsContext) AllDescriptionItem() []IDescriptionItemContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IDescriptionItemContext); ok {
			len++
		}
	}

	tst := make([]IDescriptionItemContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IDescriptionItemContext); ok {
			tst[i] = t.(IDescriptionItemContext)
			i++
		}
	}

	return tst
}

func (s *DescriptionItemsContext) DescriptionItem(i int) IDescriptionItemContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDescriptionItemContext); ok {
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

	return t.(IDescriptionItemContext)
}

func (s *DescriptionItemsContext) AllLines() []ILinesContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ILinesContext); ok {
			len++
		}
	}

	tst := make([]ILinesContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ILinesContext); ok {
			tst[i] = t.(ILinesContext)
			i++
		}
	}

	return tst
}

func (s *DescriptionItemsContext) Lines(i int) ILinesContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILinesContext); ok {
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

	return t.(ILinesContext)
}

func (s *DescriptionItemsContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserComma)
}

func (s *DescriptionItemsContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserComma, i)
}

func (s *DescriptionItemsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DescriptionItemsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DescriptionItemsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitDescriptionItems(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) DescriptionItems() (localctx IDescriptionItemsContext) {
	this := p
	_ = this

	localctx = NewDescriptionItemsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 34, SyntaxFlowParserRULE_descriptionItems)
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
	p.SetState(246)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserT__2 {
		{
			p.SetState(245)
			p.Lines()
		}

	}
	{
		p.SetState(248)
		p.DescriptionItem()
	}
	p.SetState(256)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 31, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(249)
				p.Match(SyntaxFlowParserComma)
			}
			p.SetState(251)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			if _la == SyntaxFlowParserT__2 {
				{
					p.SetState(250)
					p.Lines()
				}

			}
			{
				p.SetState(253)
				p.DescriptionItem()
			}

		}
		p.SetState(258)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 31, p.GetParserRuleContext())
	}
	p.SetState(260)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserComma {
		{
			p.SetState(259)
			p.Match(SyntaxFlowParserComma)
		}

	}
	p.SetState(263)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserT__2 {
		{
			p.SetState(262)
			p.Lines()
		}

	}

	return localctx
}

// IDescriptionItemContext is an interface to support dynamic dispatch.
type IDescriptionItemContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDescriptionItemContext differentiates from other interfaces.
	IsDescriptionItemContext()
}

type DescriptionItemContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDescriptionItemContext() *DescriptionItemContext {
	var p = new(DescriptionItemContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_descriptionItem
	return p
}

func (*DescriptionItemContext) IsDescriptionItemContext() {}

func NewDescriptionItemContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DescriptionItemContext {
	var p = new(DescriptionItemContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_descriptionItem

	return p
}

func (s *DescriptionItemContext) GetParser() antlr.Parser { return s.parser }

func (s *DescriptionItemContext) AllStringLiteral() []IStringLiteralContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStringLiteralContext); ok {
			len++
		}
	}

	tst := make([]IStringLiteralContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStringLiteralContext); ok {
			tst[i] = t.(IStringLiteralContext)
			i++
		}
	}

	return tst
}

func (s *DescriptionItemContext) StringLiteral(i int) IStringLiteralContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStringLiteralContext); ok {
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

	return t.(IStringLiteralContext)
}

func (s *DescriptionItemContext) Lines() ILinesContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILinesContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILinesContext)
}

func (s *DescriptionItemContext) Colon() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserColon, 0)
}

func (s *DescriptionItemContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DescriptionItemContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DescriptionItemContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitDescriptionItem(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) DescriptionItem() (localctx IDescriptionItemContext) {
	this := p
	_ = this

	localctx = NewDescriptionItemContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 36, SyntaxFlowParserRULE_descriptionItem)

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

	p.SetState(275)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 36, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(265)
			p.StringLiteral()
		}
		p.SetState(267)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 34, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(266)
				p.Lines()
			}

		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(269)
			p.StringLiteral()
		}
		{
			p.SetState(270)
			p.Match(SyntaxFlowParserColon)
		}
		{
			p.SetState(271)
			p.StringLiteral()
		}
		p.SetState(273)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 35, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(272)
				p.Lines()
			}

		}

	}

	return localctx
}

// IAlertStatementContext is an interface to support dynamic dispatch.
type IAlertStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsAlertStatementContext differentiates from other interfaces.
	IsAlertStatementContext()
}

type AlertStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAlertStatementContext() *AlertStatementContext {
	var p = new(AlertStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_alertStatement
	return p
}

func (*AlertStatementContext) IsAlertStatementContext() {}

func NewAlertStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AlertStatementContext {
	var p = new(AlertStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_alertStatement

	return p
}

func (s *AlertStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *AlertStatementContext) Alert() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserAlert, 0)
}

func (s *AlertStatementContext) RefVariable() IRefVariableContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRefVariableContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRefVariableContext)
}

func (s *AlertStatementContext) For() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserFor, 0)
}

func (s *AlertStatementContext) StringLiteral() IStringLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStringLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStringLiteralContext)
}

func (s *AlertStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AlertStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AlertStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitAlertStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) AlertStatement() (localctx IAlertStatementContext) {
	this := p
	_ = this

	localctx = NewAlertStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 38, SyntaxFlowParserRULE_alertStatement)
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
		p.SetState(277)
		p.Match(SyntaxFlowParserAlert)
	}
	{
		p.SetState(278)
		p.RefVariable()
	}
	p.SetState(281)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserFor {
		{
			p.SetState(279)
			p.Match(SyntaxFlowParserFor)
		}
		{
			p.SetState(280)
			p.StringLiteral()
		}

	}

	return localctx
}

// ICheckStatementContext is an interface to support dynamic dispatch.
type ICheckStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsCheckStatementContext differentiates from other interfaces.
	IsCheckStatementContext()
}

type CheckStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyCheckStatementContext() *CheckStatementContext {
	var p = new(CheckStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_checkStatement
	return p
}

func (*CheckStatementContext) IsCheckStatementContext() {}

func NewCheckStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *CheckStatementContext {
	var p = new(CheckStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_checkStatement

	return p
}

func (s *CheckStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *CheckStatementContext) Check() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCheck, 0)
}

func (s *CheckStatementContext) RefVariable() IRefVariableContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRefVariableContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRefVariableContext)
}

func (s *CheckStatementContext) ThenExpr() IThenExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IThenExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IThenExprContext)
}

func (s *CheckStatementContext) ElseExpr() IElseExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElseExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElseExprContext)
}

func (s *CheckStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *CheckStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *CheckStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitCheckStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) CheckStatement() (localctx ICheckStatementContext) {
	this := p
	_ = this

	localctx = NewCheckStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 40, SyntaxFlowParserRULE_checkStatement)

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
		p.SetState(283)
		p.Match(SyntaxFlowParserCheck)
	}
	{
		p.SetState(284)
		p.RefVariable()
	}
	p.SetState(286)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 38, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(285)
			p.ThenExpr()
		}

	}
	p.SetState(289)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 39, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(288)
			p.ElseExpr()
		}

	}

	return localctx
}

// IThenExprContext is an interface to support dynamic dispatch.
type IThenExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsThenExprContext differentiates from other interfaces.
	IsThenExprContext()
}

type ThenExprContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyThenExprContext() *ThenExprContext {
	var p = new(ThenExprContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_thenExpr
	return p
}

func (*ThenExprContext) IsThenExprContext() {}

func NewThenExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ThenExprContext {
	var p = new(ThenExprContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_thenExpr

	return p
}

func (s *ThenExprContext) GetParser() antlr.Parser { return s.parser }

func (s *ThenExprContext) Then() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserThen, 0)
}

func (s *ThenExprContext) StringLiteral() IStringLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStringLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStringLiteralContext)
}

func (s *ThenExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ThenExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ThenExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitThenExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) ThenExpr() (localctx IThenExprContext) {
	this := p
	_ = this

	localctx = NewThenExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 42, SyntaxFlowParserRULE_thenExpr)

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
		p.SetState(291)
		p.Match(SyntaxFlowParserThen)
	}
	{
		p.SetState(292)
		p.StringLiteral()
	}

	return localctx
}

// IElseExprContext is an interface to support dynamic dispatch.
type IElseExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsElseExprContext differentiates from other interfaces.
	IsElseExprContext()
}

type ElseExprContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyElseExprContext() *ElseExprContext {
	var p = new(ElseExprContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_elseExpr
	return p
}

func (*ElseExprContext) IsElseExprContext() {}

func NewElseExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ElseExprContext {
	var p = new(ElseExprContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_elseExpr

	return p
}

func (s *ElseExprContext) GetParser() antlr.Parser { return s.parser }

func (s *ElseExprContext) Else() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserElse, 0)
}

func (s *ElseExprContext) StringLiteral() IStringLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStringLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStringLiteralContext)
}

func (s *ElseExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ElseExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ElseExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitElseExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) ElseExpr() (localctx IElseExprContext) {
	this := p
	_ = this

	localctx = NewElseExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 44, SyntaxFlowParserRULE_elseExpr)

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
		p.SetState(294)
		p.Match(SyntaxFlowParserElse)
	}
	{
		p.SetState(295)
		p.StringLiteral()
	}

	return localctx
}

// IRefVariableContext is an interface to support dynamic dispatch.
type IRefVariableContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsRefVariableContext differentiates from other interfaces.
	IsRefVariableContext()
}

type RefVariableContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyRefVariableContext() *RefVariableContext {
	var p = new(RefVariableContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_refVariable
	return p
}

func (*RefVariableContext) IsRefVariableContext() {}

func NewRefVariableContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *RefVariableContext {
	var p = new(RefVariableContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_refVariable

	return p
}

func (s *RefVariableContext) GetParser() antlr.Parser { return s.parser }

func (s *RefVariableContext) DollarOutput() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDollarOutput, 0)
}

func (s *RefVariableContext) Identifier() IIdentifierContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIdentifierContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIdentifierContext)
}

func (s *RefVariableContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserOpenParen, 0)
}

func (s *RefVariableContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCloseParen, 0)
}

func (s *RefVariableContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RefVariableContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *RefVariableContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitRefVariable(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) RefVariable() (localctx IRefVariableContext) {
	this := p
	_ = this

	localctx = NewRefVariableContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 46, SyntaxFlowParserRULE_refVariable)

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
		p.SetState(297)
		p.Match(SyntaxFlowParserDollarOutput)
	}
	p.SetState(303)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		{
			p.SetState(298)
			p.Identifier()
		}

	case SyntaxFlowParserOpenParen:
		{
			p.SetState(299)
			p.Match(SyntaxFlowParserOpenParen)
		}
		{
			p.SetState(300)
			p.Identifier()
		}
		{
			p.SetState(301)
			p.Match(SyntaxFlowParserCloseParen)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IFilterItemFirstContext is an interface to support dynamic dispatch.
type IFilterItemFirstContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFilterItemFirstContext differentiates from other interfaces.
	IsFilterItemFirstContext()
}

type FilterItemFirstContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFilterItemFirstContext() *FilterItemFirstContext {
	var p = new(FilterItemFirstContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_filterItemFirst
	return p
}

func (*FilterItemFirstContext) IsFilterItemFirstContext() {}

func NewFilterItemFirstContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FilterItemFirstContext {
	var p = new(FilterItemFirstContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_filterItemFirst

	return p
}

func (s *FilterItemFirstContext) GetParser() antlr.Parser { return s.parser }

func (s *FilterItemFirstContext) CopyFrom(ctx *FilterItemFirstContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *FilterItemFirstContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterItemFirstContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type FieldCallFilterContext struct {
	*FilterItemFirstContext
}

func NewFieldCallFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FieldCallFilterContext {
	var p = new(FieldCallFilterContext)

	p.FilterItemFirstContext = NewEmptyFilterItemFirstContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemFirstContext))

	return p
}

func (s *FieldCallFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FieldCallFilterContext) Dot() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDot, 0)
}

func (s *FieldCallFilterContext) NameFilter() INameFilterContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INameFilterContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INameFilterContext)
}

func (s *FieldCallFilterContext) Lines() ILinesContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILinesContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILinesContext)
}

func (s *FieldCallFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFieldCallFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type NamedFilterContext struct {
	*FilterItemFirstContext
}

func NewNamedFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *NamedFilterContext {
	var p = new(NamedFilterContext)

	p.FilterItemFirstContext = NewEmptyFilterItemFirstContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemFirstContext))

	return p
}

func (s *NamedFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NamedFilterContext) NameFilter() INameFilterContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INameFilterContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INameFilterContext)
}

func (s *NamedFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitNamedFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type NativeCallFilterContext struct {
	*FilterItemFirstContext
}

func NewNativeCallFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *NativeCallFilterContext {
	var p = new(NativeCallFilterContext)

	p.FilterItemFirstContext = NewEmptyFilterItemFirstContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemFirstContext))

	return p
}

func (s *NativeCallFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NativeCallFilterContext) NativeCall() INativeCallContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INativeCallContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INativeCallContext)
}

func (s *NativeCallFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitNativeCallFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FilterItemFirst() (localctx IFilterItemFirstContext) {
	this := p
	_ = this

	localctx = NewFilterItemFirstContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 48, SyntaxFlowParserRULE_filterItemFirst)
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

	p.SetState(312)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		localctx = NewNamedFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(305)
			p.NameFilter()
		}

	case SyntaxFlowParserDot:
		localctx = NewFieldCallFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(306)
			p.Match(SyntaxFlowParserDot)
		}
		p.SetState(308)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserT__2 {
			{
				p.SetState(307)
				p.Lines()
			}

		}
		{
			p.SetState(310)
			p.NameFilter()
		}

	case SyntaxFlowParserLt:
		localctx = NewNativeCallFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(311)
			p.NativeCall()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IFilterItemContext is an interface to support dynamic dispatch.
type IFilterItemContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFilterItemContext differentiates from other interfaces.
	IsFilterItemContext()
}

type FilterItemContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFilterItemContext() *FilterItemContext {
	var p = new(FilterItemContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_filterItem
	return p
}

func (*FilterItemContext) IsFilterItemContext() {}

func NewFilterItemContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FilterItemContext {
	var p = new(FilterItemContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_filterItem

	return p
}

func (s *FilterItemContext) GetParser() antlr.Parser { return s.parser }

func (s *FilterItemContext) CopyFrom(ctx *FilterItemContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *FilterItemContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterItemContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type FunctionCallFilterContext struct {
	*FilterItemContext
}

func NewFunctionCallFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FunctionCallFilterContext {
	var p = new(FunctionCallFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *FunctionCallFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FunctionCallFilterContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserOpenParen, 0)
}

func (s *FunctionCallFilterContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCloseParen, 0)
}

func (s *FunctionCallFilterContext) Lines() ILinesContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILinesContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILinesContext)
}

func (s *FunctionCallFilterContext) ActualParam() IActualParamContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IActualParamContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IActualParamContext)
}

func (s *FunctionCallFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFunctionCallFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type DeepChainFilterContext struct {
	*FilterItemContext
}

func NewDeepChainFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *DeepChainFilterContext {
	var p = new(DeepChainFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *DeepChainFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DeepChainFilterContext) Deep() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDeep, 0)
}

func (s *DeepChainFilterContext) NameFilter() INameFilterContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INameFilterContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INameFilterContext)
}

func (s *DeepChainFilterContext) Lines() ILinesContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILinesContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILinesContext)
}

func (s *DeepChainFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitDeepChainFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type NextFilterContext struct {
	*FilterItemContext
}

func NewNextFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *NextFilterContext {
	var p = new(NextFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *NextFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NextFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitNextFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type OptionalFilterContext struct {
	*FilterItemContext
}

func NewOptionalFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *OptionalFilterContext {
	var p = new(OptionalFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *OptionalFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OptionalFilterContext) ConditionStart() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserConditionStart, 0)
}

func (s *OptionalFilterContext) ConditionExpression() IConditionExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConditionExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConditionExpressionContext)
}

func (s *OptionalFilterContext) MapBuilderClose() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserMapBuilderClose, 0)
}

func (s *OptionalFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitOptionalFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type TopDefFilterContext struct {
	*FilterItemContext
}

func NewTopDefFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *TopDefFilterContext {
	var p = new(TopDefFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *TopDefFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TopDefFilterContext) TopDef() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserTopDef, 0)
}

func (s *TopDefFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitTopDefFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type DeepNextConfigFilterContext struct {
	*FilterItemContext
}

func NewDeepNextConfigFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *DeepNextConfigFilterContext {
	var p = new(DeepNextConfigFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *DeepNextConfigFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DeepNextConfigFilterContext) DeepNextStart() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDeepNextStart, 0)
}

func (s *DeepNextConfigFilterContext) DeepNextEnd() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDeepNextEnd, 0)
}

func (s *DeepNextConfigFilterContext) Config() IConfigContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConfigContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConfigContext)
}

func (s *DeepNextConfigFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitDeepNextConfigFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type FieldIndexFilterContext struct {
	*FilterItemContext
}

func NewFieldIndexFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FieldIndexFilterContext {
	var p = new(FieldIndexFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *FieldIndexFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FieldIndexFilterContext) ListSelectOpen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserListSelectOpen, 0)
}

func (s *FieldIndexFilterContext) SliceCallItem() ISliceCallItemContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISliceCallItemContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISliceCallItemContext)
}

func (s *FieldIndexFilterContext) ListSelectClose() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserListSelectClose, 0)
}

func (s *FieldIndexFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFieldIndexFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type RemoveRefFilterContext struct {
	*FilterItemContext
}

func NewRemoveRefFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *RemoveRefFilterContext {
	var p = new(RemoveRefFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *RemoveRefFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RemoveRefFilterContext) Minus() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserMinus, 0)
}

func (s *RemoveRefFilterContext) RefVariable() IRefVariableContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRefVariableContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRefVariableContext)
}

func (s *RemoveRefFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitRemoveRefFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type DefFilterContext struct {
	*FilterItemContext
}

func NewDefFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *DefFilterContext {
	var p = new(DefFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *DefFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DefFilterContext) DefStart() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDefStart, 0)
}

func (s *DefFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitDefFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type TopDefConfigFilterContext struct {
	*FilterItemContext
}

func NewTopDefConfigFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *TopDefConfigFilterContext {
	var p = new(TopDefConfigFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *TopDefConfigFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TopDefConfigFilterContext) TopDefStart() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserTopDefStart, 0)
}

func (s *TopDefConfigFilterContext) DeepNextEnd() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDeepNextEnd, 0)
}

func (s *TopDefConfigFilterContext) Config() IConfigContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConfigContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConfigContext)
}

func (s *TopDefConfigFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitTopDefConfigFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type MergeRefFilterContext struct {
	*FilterItemContext
}

func NewMergeRefFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *MergeRefFilterContext {
	var p = new(MergeRefFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *MergeRefFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *MergeRefFilterContext) RefVariable() IRefVariableContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRefVariableContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRefVariableContext)
}

func (s *MergeRefFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitMergeRefFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type DeepNextFilterContext struct {
	*FilterItemContext
}

func NewDeepNextFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *DeepNextFilterContext {
	var p = new(DeepNextFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *DeepNextFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DeepNextFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitDeepNextFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type FirstContext struct {
	*FilterItemContext
}

func NewFirstContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FirstContext {
	var p = new(FirstContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *FirstContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FirstContext) FilterItemFirst() IFilterItemFirstContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterItemFirstContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFilterItemFirstContext)
}

func (s *FirstContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFirst(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FilterItem() (localctx IFilterItemContext) {
	this := p
	_ = this

	localctx = NewFilterItemContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 50, SyntaxFlowParserRULE_filterItem)
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

	p.SetState(354)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDot, SyntaxFlowParserLt, SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		localctx = NewFirstContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(314)
			p.FilterItemFirst()
		}

	case SyntaxFlowParserDeep:
		localctx = NewDeepChainFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(315)
			p.Match(SyntaxFlowParserDeep)
		}
		p.SetState(317)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserT__2 {
			{
				p.SetState(316)
				p.Lines()
			}

		}
		{
			p.SetState(319)
			p.NameFilter()
		}

	case SyntaxFlowParserOpenParen:
		localctx = NewFunctionCallFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(320)
			p.Match(SyntaxFlowParserOpenParen)
		}
		p.SetState(322)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserT__2 {
			{
				p.SetState(321)
				p.Lines()
			}

		}
		p.SetState(325)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-24)) & ^0x3f) == 0 && ((int64(1)<<(_la-24))&954762841830785587) != 0 {
			{
				p.SetState(324)
				p.ActualParam()
			}

		}
		{
			p.SetState(327)
			p.Match(SyntaxFlowParserCloseParen)
		}

	case SyntaxFlowParserListSelectOpen:
		localctx = NewFieldIndexFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(328)
			p.Match(SyntaxFlowParserListSelectOpen)
		}
		{
			p.SetState(329)
			p.SliceCallItem()
		}
		{
			p.SetState(330)
			p.Match(SyntaxFlowParserListSelectClose)
		}

	case SyntaxFlowParserConditionStart:
		localctx = NewOptionalFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(332)
			p.Match(SyntaxFlowParserConditionStart)
		}
		{
			p.SetState(333)
			p.conditionExpression(0)
		}
		{
			p.SetState(334)
			p.Match(SyntaxFlowParserMapBuilderClose)
		}

	case SyntaxFlowParserT__3:
		localctx = NewNextFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(336)
			p.Match(SyntaxFlowParserT__3)
		}

	case SyntaxFlowParserDefStart:
		localctx = NewDefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(337)
			p.Match(SyntaxFlowParserDefStart)
		}

	case SyntaxFlowParserT__4:
		localctx = NewDeepNextFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(338)
			p.Match(SyntaxFlowParserT__4)
		}

	case SyntaxFlowParserDeepNextStart:
		localctx = NewDeepNextConfigFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(339)
			p.Match(SyntaxFlowParserDeepNextStart)
		}
		p.SetState(341)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-4683743612465315832) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&344063) != 0 {
			{
				p.SetState(340)
				p.Config()
			}

		}
		{
			p.SetState(343)
			p.Match(SyntaxFlowParserDeepNextEnd)
		}

	case SyntaxFlowParserTopDef:
		localctx = NewTopDefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 10)
		{
			p.SetState(344)
			p.Match(SyntaxFlowParserTopDef)
		}

	case SyntaxFlowParserTopDefStart:
		localctx = NewTopDefConfigFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 11)
		{
			p.SetState(345)
			p.Match(SyntaxFlowParserTopDefStart)
		}
		p.SetState(347)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-4683743612465315832) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&344063) != 0 {
			{
				p.SetState(346)
				p.Config()
			}

		}
		{
			p.SetState(349)
			p.Match(SyntaxFlowParserDeepNextEnd)
		}

	case SyntaxFlowParserT__5:
		localctx = NewMergeRefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 12)
		{
			p.SetState(350)
			p.Match(SyntaxFlowParserT__5)
		}
		{
			p.SetState(351)
			p.RefVariable()
		}

	case SyntaxFlowParserMinus:
		localctx = NewRemoveRefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 13)
		{
			p.SetState(352)
			p.Match(SyntaxFlowParserMinus)
		}
		{
			p.SetState(353)
			p.RefVariable()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IFilterExprContext is an interface to support dynamic dispatch.
type IFilterExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFilterExprContext differentiates from other interfaces.
	IsFilterExprContext()
}

type FilterExprContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFilterExprContext() *FilterExprContext {
	var p = new(FilterExprContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_filterExpr
	return p
}

func (*FilterExprContext) IsFilterExprContext() {}

func NewFilterExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FilterExprContext {
	var p = new(FilterExprContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_filterExpr

	return p
}

func (s *FilterExprContext) GetParser() antlr.Parser { return s.parser }

func (s *FilterExprContext) FilterItemFirst() IFilterItemFirstContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterItemFirstContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFilterItemFirstContext)
}

func (s *FilterExprContext) AllFilterItem() []IFilterItemContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFilterItemContext); ok {
			len++
		}
	}

	tst := make([]IFilterItemContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFilterItemContext); ok {
			tst[i] = t.(IFilterItemContext)
			i++
		}
	}

	return tst
}

func (s *FilterExprContext) FilterItem(i int) IFilterItemContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterItemContext); ok {
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

	return t.(IFilterItemContext)
}

func (s *FilterExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FilterExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilterExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FilterExpr() (localctx IFilterExprContext) {
	this := p
	_ = this

	localctx = NewFilterExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 52, SyntaxFlowParserRULE_filterExpr)

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
		p.SetState(356)
		p.FilterItemFirst()
	}
	p.SetState(360)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 49, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(357)
				p.FilterItem()
			}

		}
		p.SetState(362)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 49, p.GetParserRuleContext())
	}

	return localctx
}

// INativeCallContext is an interface to support dynamic dispatch.
type INativeCallContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNativeCallContext differentiates from other interfaces.
	IsNativeCallContext()
}

type NativeCallContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNativeCallContext() *NativeCallContext {
	var p = new(NativeCallContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_nativeCall
	return p
}

func (*NativeCallContext) IsNativeCallContext() {}

func NewNativeCallContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NativeCallContext {
	var p = new(NativeCallContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_nativeCall

	return p
}

func (s *NativeCallContext) GetParser() antlr.Parser { return s.parser }

func (s *NativeCallContext) Lt() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserLt, 0)
}

func (s *NativeCallContext) UseNativeCall() IUseNativeCallContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IUseNativeCallContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IUseNativeCallContext)
}

func (s *NativeCallContext) Gt() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserGt, 0)
}

func (s *NativeCallContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NativeCallContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NativeCallContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitNativeCall(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) NativeCall() (localctx INativeCallContext) {
	this := p
	_ = this

	localctx = NewNativeCallContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 54, SyntaxFlowParserRULE_nativeCall)

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
		p.SetState(363)
		p.Match(SyntaxFlowParserLt)
	}
	{
		p.SetState(364)
		p.UseNativeCall()
	}
	{
		p.SetState(365)
		p.Match(SyntaxFlowParserGt)
	}

	return localctx
}

// IUseNativeCallContext is an interface to support dynamic dispatch.
type IUseNativeCallContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsUseNativeCallContext differentiates from other interfaces.
	IsUseNativeCallContext()
}

type UseNativeCallContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyUseNativeCallContext() *UseNativeCallContext {
	var p = new(UseNativeCallContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_useNativeCall
	return p
}

func (*UseNativeCallContext) IsUseNativeCallContext() {}

func NewUseNativeCallContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *UseNativeCallContext {
	var p = new(UseNativeCallContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_useNativeCall

	return p
}

func (s *UseNativeCallContext) GetParser() antlr.Parser { return s.parser }

func (s *UseNativeCallContext) Identifier() IIdentifierContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIdentifierContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIdentifierContext)
}

func (s *UseNativeCallContext) UseDefCalcParams() IUseDefCalcParamsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IUseDefCalcParamsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IUseDefCalcParamsContext)
}

func (s *UseNativeCallContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *UseNativeCallContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *UseNativeCallContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitUseNativeCall(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) UseNativeCall() (localctx IUseNativeCallContext) {
	this := p
	_ = this

	localctx = NewUseNativeCallContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 56, SyntaxFlowParserRULE_useNativeCall)
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
		p.SetState(367)
		p.Identifier()
	}
	p.SetState(369)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserOpenParen || _la == SyntaxFlowParserMapBuilderOpen {
		{
			p.SetState(368)
			p.UseDefCalcParams()
		}

	}

	return localctx
}

// IUseDefCalcParamsContext is an interface to support dynamic dispatch.
type IUseDefCalcParamsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsUseDefCalcParamsContext differentiates from other interfaces.
	IsUseDefCalcParamsContext()
}

type UseDefCalcParamsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyUseDefCalcParamsContext() *UseDefCalcParamsContext {
	var p = new(UseDefCalcParamsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_useDefCalcParams
	return p
}

func (*UseDefCalcParamsContext) IsUseDefCalcParamsContext() {}

func NewUseDefCalcParamsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *UseDefCalcParamsContext {
	var p = new(UseDefCalcParamsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_useDefCalcParams

	return p
}

func (s *UseDefCalcParamsContext) GetParser() antlr.Parser { return s.parser }

func (s *UseDefCalcParamsContext) MapBuilderOpen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserMapBuilderOpen, 0)
}

func (s *UseDefCalcParamsContext) MapBuilderClose() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserMapBuilderClose, 0)
}

func (s *UseDefCalcParamsContext) Config() IConfigContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConfigContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConfigContext)
}

func (s *UseDefCalcParamsContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserOpenParen, 0)
}

func (s *UseDefCalcParamsContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCloseParen, 0)
}

func (s *UseDefCalcParamsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *UseDefCalcParamsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *UseDefCalcParamsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitUseDefCalcParams(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) UseDefCalcParams() (localctx IUseDefCalcParamsContext) {
	this := p
	_ = this

	localctx = NewUseDefCalcParamsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 58, SyntaxFlowParserRULE_useDefCalcParams)
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

	p.SetState(381)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserMapBuilderOpen:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(371)
			p.Match(SyntaxFlowParserMapBuilderOpen)
		}
		p.SetState(373)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-4683743612465315832) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&344063) != 0 {
			{
				p.SetState(372)
				p.Config()
			}

		}
		{
			p.SetState(375)
			p.Match(SyntaxFlowParserMapBuilderClose)
		}

	case SyntaxFlowParserOpenParen:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(376)
			p.Match(SyntaxFlowParserOpenParen)
		}
		p.SetState(378)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-4683743612465315832) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&344063) != 0 {
			{
				p.SetState(377)
				p.Config()
			}

		}
		{
			p.SetState(380)
			p.Match(SyntaxFlowParserCloseParen)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IActualParamContext is an interface to support dynamic dispatch.
type IActualParamContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsActualParamContext differentiates from other interfaces.
	IsActualParamContext()
}

type ActualParamContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyActualParamContext() *ActualParamContext {
	var p = new(ActualParamContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_actualParam
	return p
}

func (*ActualParamContext) IsActualParamContext() {}

func NewActualParamContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ActualParamContext {
	var p = new(ActualParamContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_actualParam

	return p
}

func (s *ActualParamContext) GetParser() antlr.Parser { return s.parser }

func (s *ActualParamContext) CopyFrom(ctx *ActualParamContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *ActualParamContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ActualParamContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type AllParamContext struct {
	*ActualParamContext
}

func NewAllParamContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *AllParamContext {
	var p = new(AllParamContext)

	p.ActualParamContext = NewEmptyActualParamContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ActualParamContext))

	return p
}

func (s *AllParamContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AllParamContext) SingleParam() ISingleParamContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleParamContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleParamContext)
}

func (s *AllParamContext) Lines() ILinesContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILinesContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILinesContext)
}

func (s *AllParamContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitAllParam(s)

	default:
		return t.VisitChildren(s)
	}
}

type EveryParamContext struct {
	*ActualParamContext
}

func NewEveryParamContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *EveryParamContext {
	var p = new(EveryParamContext)

	p.ActualParamContext = NewEmptyActualParamContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ActualParamContext))

	return p
}

func (s *EveryParamContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *EveryParamContext) AllActualParamFilter() []IActualParamFilterContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IActualParamFilterContext); ok {
			len++
		}
	}

	tst := make([]IActualParamFilterContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IActualParamFilterContext); ok {
			tst[i] = t.(IActualParamFilterContext)
			i++
		}
	}

	return tst
}

func (s *EveryParamContext) ActualParamFilter(i int) IActualParamFilterContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IActualParamFilterContext); ok {
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

	return t.(IActualParamFilterContext)
}

func (s *EveryParamContext) SingleParam() ISingleParamContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleParamContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleParamContext)
}

func (s *EveryParamContext) Lines() ILinesContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILinesContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILinesContext)
}

func (s *EveryParamContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitEveryParam(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) ActualParam() (localctx IActualParamContext) {
	this := p
	_ = this

	localctx = NewActualParamContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 60, SyntaxFlowParserRULE_actualParam)
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

	p.SetState(398)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 58, p.GetParserRuleContext()) {
	case 1:
		localctx = NewAllParamContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(383)
			p.SingleParam()
		}
		p.SetState(385)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserT__2 {
			{
				p.SetState(384)
				p.Lines()
			}

		}

	case 2:
		localctx = NewEveryParamContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		p.SetState(388)
		p.GetErrorHandler().Sync(p)
		_alt = 1
		for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			switch _alt {
			case 1:
				{
					p.SetState(387)
					p.ActualParamFilter()
				}

			default:
				panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
			}

			p.SetState(390)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 55, p.GetParserRuleContext())
		}
		p.SetState(393)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-24)) & ^0x3f) == 0 && ((int64(1)<<(_la-24))&954762841830785075) != 0 {
			{
				p.SetState(392)
				p.SingleParam()
			}

		}
		p.SetState(396)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserT__2 {
			{
				p.SetState(395)
				p.Lines()
			}

		}

	}

	return localctx
}

// IActualParamFilterContext is an interface to support dynamic dispatch.
type IActualParamFilterContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsActualParamFilterContext differentiates from other interfaces.
	IsActualParamFilterContext()
}

type ActualParamFilterContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyActualParamFilterContext() *ActualParamFilterContext {
	var p = new(ActualParamFilterContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_actualParamFilter
	return p
}

func (*ActualParamFilterContext) IsActualParamFilterContext() {}

func NewActualParamFilterContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ActualParamFilterContext {
	var p = new(ActualParamFilterContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_actualParamFilter

	return p
}

func (s *ActualParamFilterContext) GetParser() antlr.Parser { return s.parser }

func (s *ActualParamFilterContext) SingleParam() ISingleParamContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleParamContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleParamContext)
}

func (s *ActualParamFilterContext) Comma() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserComma, 0)
}

func (s *ActualParamFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ActualParamFilterContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ActualParamFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitActualParamFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) ActualParamFilter() (localctx IActualParamFilterContext) {
	this := p
	_ = this

	localctx = NewActualParamFilterContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 62, SyntaxFlowParserRULE_actualParamFilter)

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

	p.SetState(404)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserTopDefStart, SyntaxFlowParserDefStart, SyntaxFlowParserDot, SyntaxFlowParserLt, SyntaxFlowParserDollarOutput, SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(400)
			p.SingleParam()
		}
		{
			p.SetState(401)
			p.Match(SyntaxFlowParserComma)
		}

	case SyntaxFlowParserComma:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(403)
			p.Match(SyntaxFlowParserComma)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// ISingleParamContext is an interface to support dynamic dispatch.
type ISingleParamContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsSingleParamContext differentiates from other interfaces.
	IsSingleParamContext()
}

type SingleParamContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySingleParamContext() *SingleParamContext {
	var p = new(SingleParamContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_singleParam
	return p
}

func (*SingleParamContext) IsSingleParamContext() {}

func NewSingleParamContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SingleParamContext {
	var p = new(SingleParamContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_singleParam

	return p
}

func (s *SingleParamContext) GetParser() antlr.Parser { return s.parser }

func (s *SingleParamContext) FilterStatement() IFilterStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFilterStatementContext)
}

func (s *SingleParamContext) DefStart() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDefStart, 0)
}

func (s *SingleParamContext) TopDefStart() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserTopDefStart, 0)
}

func (s *SingleParamContext) MapBuilderClose() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserMapBuilderClose, 0)
}

func (s *SingleParamContext) Config() IConfigContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConfigContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConfigContext)
}

func (s *SingleParamContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SingleParamContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *SingleParamContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitSingleParam(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) SingleParam() (localctx ISingleParamContext) {
	this := p
	_ = this

	localctx = NewSingleParamContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 64, SyntaxFlowParserRULE_singleParam)
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
	p.SetState(412)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDefStart:
		{
			p.SetState(406)
			p.Match(SyntaxFlowParserDefStart)
		}

	case SyntaxFlowParserTopDefStart:
		{
			p.SetState(407)
			p.Match(SyntaxFlowParserTopDefStart)
		}
		p.SetState(409)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-4683743612465315832) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&344063) != 0 {
			{
				p.SetState(408)
				p.Config()
			}

		}
		{
			p.SetState(411)
			p.Match(SyntaxFlowParserMapBuilderClose)
		}

	case SyntaxFlowParserDot, SyntaxFlowParserLt, SyntaxFlowParserDollarOutput, SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:

	default:
	}
	{
		p.SetState(414)
		p.FilterStatement()
	}

	return localctx
}

// IConfigContext is an interface to support dynamic dispatch.
type IConfigContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsConfigContext differentiates from other interfaces.
	IsConfigContext()
}

type ConfigContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyConfigContext() *ConfigContext {
	var p = new(ConfigContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_config
	return p
}

func (*ConfigContext) IsConfigContext() {}

func NewConfigContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ConfigContext {
	var p = new(ConfigContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_config

	return p
}

func (s *ConfigContext) GetParser() antlr.Parser { return s.parser }

func (s *ConfigContext) AllRecursiveConfigItem() []IRecursiveConfigItemContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IRecursiveConfigItemContext); ok {
			len++
		}
	}

	tst := make([]IRecursiveConfigItemContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IRecursiveConfigItemContext); ok {
			tst[i] = t.(IRecursiveConfigItemContext)
			i++
		}
	}

	return tst
}

func (s *ConfigContext) RecursiveConfigItem(i int) IRecursiveConfigItemContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRecursiveConfigItemContext); ok {
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

	return t.(IRecursiveConfigItemContext)
}

func (s *ConfigContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserComma)
}

func (s *ConfigContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserComma, i)
}

func (s *ConfigContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ConfigContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ConfigContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitConfig(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Config() (localctx IConfigContext) {
	this := p
	_ = this

	localctx = NewConfigContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 66, SyntaxFlowParserRULE_config)
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
		p.SetState(416)
		p.RecursiveConfigItem()
	}
	p.SetState(421)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 62, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(417)
				p.Match(SyntaxFlowParserComma)
			}
			{
				p.SetState(418)
				p.RecursiveConfigItem()
			}

		}
		p.SetState(423)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 62, p.GetParserRuleContext())
	}
	p.SetState(425)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserComma {
		{
			p.SetState(424)
			p.Match(SyntaxFlowParserComma)
		}

	}

	return localctx
}

// IRecursiveConfigItemContext is an interface to support dynamic dispatch.
type IRecursiveConfigItemContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsRecursiveConfigItemContext differentiates from other interfaces.
	IsRecursiveConfigItemContext()
}

type RecursiveConfigItemContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyRecursiveConfigItemContext() *RecursiveConfigItemContext {
	var p = new(RecursiveConfigItemContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_recursiveConfigItem
	return p
}

func (*RecursiveConfigItemContext) IsRecursiveConfigItemContext() {}

func NewRecursiveConfigItemContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *RecursiveConfigItemContext {
	var p = new(RecursiveConfigItemContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_recursiveConfigItem

	return p
}

func (s *RecursiveConfigItemContext) GetParser() antlr.Parser { return s.parser }

func (s *RecursiveConfigItemContext) Identifier() IIdentifierContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIdentifierContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIdentifierContext)
}

func (s *RecursiveConfigItemContext) Colon() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserColon, 0)
}

func (s *RecursiveConfigItemContext) RecursiveConfigItemValue() IRecursiveConfigItemValueContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRecursiveConfigItemValueContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRecursiveConfigItemValueContext)
}

func (s *RecursiveConfigItemContext) Line() ILineContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILineContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILineContext)
}

func (s *RecursiveConfigItemContext) Lines() ILinesContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILinesContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILinesContext)
}

func (s *RecursiveConfigItemContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RecursiveConfigItemContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *RecursiveConfigItemContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitRecursiveConfigItem(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) RecursiveConfigItem() (localctx IRecursiveConfigItemContext) {
	this := p
	_ = this

	localctx = NewRecursiveConfigItemContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 68, SyntaxFlowParserRULE_recursiveConfigItem)
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
	p.SetState(428)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserT__2 {
		{
			p.SetState(427)
			p.Line()
		}

	}
	{
		p.SetState(430)
		p.Identifier()
	}
	{
		p.SetState(431)
		p.Match(SyntaxFlowParserColon)
	}
	{
		p.SetState(432)
		p.RecursiveConfigItemValue()
	}
	p.SetState(434)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserT__2 {
		{
			p.SetState(433)
			p.Lines()
		}

	}

	return localctx
}

// IRecursiveConfigItemValueContext is an interface to support dynamic dispatch.
type IRecursiveConfigItemValueContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsRecursiveConfigItemValueContext differentiates from other interfaces.
	IsRecursiveConfigItemValueContext()
}

type RecursiveConfigItemValueContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyRecursiveConfigItemValueContext() *RecursiveConfigItemValueContext {
	var p = new(RecursiveConfigItemValueContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_recursiveConfigItemValue
	return p
}

func (*RecursiveConfigItemValueContext) IsRecursiveConfigItemValueContext() {}

func NewRecursiveConfigItemValueContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *RecursiveConfigItemValueContext {
	var p = new(RecursiveConfigItemValueContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_recursiveConfigItemValue

	return p
}

func (s *RecursiveConfigItemValueContext) GetParser() antlr.Parser { return s.parser }

func (s *RecursiveConfigItemValueContext) Identifier() IIdentifierContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIdentifierContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIdentifierContext)
}

func (s *RecursiveConfigItemValueContext) NumberLiteral() INumberLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INumberLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INumberLiteralContext)
}

func (s *RecursiveConfigItemValueContext) AllBacktick() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserBacktick)
}

func (s *RecursiveConfigItemValueContext) Backtick(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserBacktick, i)
}

func (s *RecursiveConfigItemValueContext) FilterStatement() IFilterStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFilterStatementContext)
}

func (s *RecursiveConfigItemValueContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RecursiveConfigItemValueContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *RecursiveConfigItemValueContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitRecursiveConfigItemValue(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) RecursiveConfigItemValue() (localctx IRecursiveConfigItemValueContext) {
	this := p
	_ = this

	localctx = NewRecursiveConfigItemValueContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 70, SyntaxFlowParserRULE_recursiveConfigItemValue)

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

	p.SetState(444)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		p.SetState(438)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
			{
				p.SetState(436)
				p.Identifier()
			}

		case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
			{
				p.SetState(437)
				p.NumberLiteral()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

	case SyntaxFlowParserBacktick:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(440)
			p.Match(SyntaxFlowParserBacktick)
		}
		{
			p.SetState(441)
			p.FilterStatement()
		}
		{
			p.SetState(442)
			p.Match(SyntaxFlowParserBacktick)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// ISliceCallItemContext is an interface to support dynamic dispatch.
type ISliceCallItemContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsSliceCallItemContext differentiates from other interfaces.
	IsSliceCallItemContext()
}

type SliceCallItemContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySliceCallItemContext() *SliceCallItemContext {
	var p = new(SliceCallItemContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_sliceCallItem
	return p
}

func (*SliceCallItemContext) IsSliceCallItemContext() {}

func NewSliceCallItemContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SliceCallItemContext {
	var p = new(SliceCallItemContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_sliceCallItem

	return p
}

func (s *SliceCallItemContext) GetParser() antlr.Parser { return s.parser }

func (s *SliceCallItemContext) NameFilter() INameFilterContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INameFilterContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INameFilterContext)
}

func (s *SliceCallItemContext) NumberLiteral() INumberLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INumberLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INumberLiteralContext)
}

func (s *SliceCallItemContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SliceCallItemContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *SliceCallItemContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitSliceCallItem(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) SliceCallItem() (localctx ISliceCallItemContext) {
	this := p
	_ = this

	localctx = NewSliceCallItemContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 72, SyntaxFlowParserRULE_sliceCallItem)

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

	p.SetState(448)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(446)
			p.NameFilter()
		}

	case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(447)
			p.NumberLiteral()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// INameFilterContext is an interface to support dynamic dispatch.
type INameFilterContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNameFilterContext differentiates from other interfaces.
	IsNameFilterContext()
}

type NameFilterContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNameFilterContext() *NameFilterContext {
	var p = new(NameFilterContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_nameFilter
	return p
}

func (*NameFilterContext) IsNameFilterContext() {}

func NewNameFilterContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NameFilterContext {
	var p = new(NameFilterContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_nameFilter

	return p
}

func (s *NameFilterContext) GetParser() antlr.Parser { return s.parser }

func (s *NameFilterContext) Star() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserStar, 0)
}

func (s *NameFilterContext) Identifier() IIdentifierContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIdentifierContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIdentifierContext)
}

func (s *NameFilterContext) RegexpLiteral() IRegexpLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRegexpLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRegexpLiteralContext)
}

func (s *NameFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NameFilterContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NameFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitNameFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) NameFilter() (localctx INameFilterContext) {
	this := p
	_ = this

	localctx = NewNameFilterContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 74, SyntaxFlowParserRULE_nameFilter)

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

	p.SetState(453)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStar:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(450)
			p.Match(SyntaxFlowParserStar)
		}

	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(451)
			p.Identifier()
		}

	case SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(452)
			p.RegexpLiteral()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IChainFilterContext is an interface to support dynamic dispatch.
type IChainFilterContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsChainFilterContext differentiates from other interfaces.
	IsChainFilterContext()
}

type ChainFilterContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyChainFilterContext() *ChainFilterContext {
	var p = new(ChainFilterContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_chainFilter
	return p
}

func (*ChainFilterContext) IsChainFilterContext() {}

func NewChainFilterContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ChainFilterContext {
	var p = new(ChainFilterContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_chainFilter

	return p
}

func (s *ChainFilterContext) GetParser() antlr.Parser { return s.parser }

func (s *ChainFilterContext) CopyFrom(ctx *ChainFilterContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *ChainFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ChainFilterContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type FlatContext struct {
	*ChainFilterContext
}

func NewFlatContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FlatContext {
	var p = new(FlatContext)

	p.ChainFilterContext = NewEmptyChainFilterContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ChainFilterContext))

	return p
}

func (s *FlatContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FlatContext) ListSelectOpen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserListSelectOpen, 0)
}

func (s *FlatContext) ListSelectClose() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserListSelectClose, 0)
}

func (s *FlatContext) Deep() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDeep, 0)
}

func (s *FlatContext) AllStatements() []IStatementsContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStatementsContext); ok {
			len++
		}
	}

	tst := make([]IStatementsContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStatementsContext); ok {
			tst[i] = t.(IStatementsContext)
			i++
		}
	}

	return tst
}

func (s *FlatContext) Statements(i int) IStatementsContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementsContext); ok {
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

	return t.(IStatementsContext)
}

func (s *FlatContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserComma)
}

func (s *FlatContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserComma, i)
}

func (s *FlatContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFlat(s)

	default:
		return t.VisitChildren(s)
	}
}

type BuildMapContext struct {
	*ChainFilterContext
}

func NewBuildMapContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BuildMapContext {
	var p = new(BuildMapContext)

	p.ChainFilterContext = NewEmptyChainFilterContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ChainFilterContext))

	return p
}

func (s *BuildMapContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BuildMapContext) MapBuilderOpen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserMapBuilderOpen, 0)
}

func (s *BuildMapContext) MapBuilderClose() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserMapBuilderClose, 0)
}

func (s *BuildMapContext) AllStatements() []IStatementsContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStatementsContext); ok {
			len++
		}
	}

	tst := make([]IStatementsContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStatementsContext); ok {
			tst[i] = t.(IStatementsContext)
			i++
		}
	}

	return tst
}

func (s *BuildMapContext) Statements(i int) IStatementsContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementsContext); ok {
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

	return t.(IStatementsContext)
}

func (s *BuildMapContext) AllIdentifier() []IIdentifierContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IIdentifierContext); ok {
			len++
		}
	}

	tst := make([]IIdentifierContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IIdentifierContext); ok {
			tst[i] = t.(IIdentifierContext)
			i++
		}
	}

	return tst
}

func (s *BuildMapContext) Identifier(i int) IIdentifierContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIdentifierContext); ok {
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

	return t.(IIdentifierContext)
}

func (s *BuildMapContext) AllColon() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserColon)
}

func (s *BuildMapContext) Colon(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserColon, i)
}

func (s *BuildMapContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitBuildMap(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) ChainFilter() (localctx IChainFilterContext) {
	this := p
	_ = this

	localctx = NewChainFilterContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 76, SyntaxFlowParserRULE_chainFilter)
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

	p.SetState(490)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserListSelectOpen:
		localctx = NewFlatContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(455)
			p.Match(SyntaxFlowParserListSelectOpen)
		}
		p.SetState(465)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserT__0, SyntaxFlowParserT__1, SyntaxFlowParserT__2, SyntaxFlowParserDot, SyntaxFlowParserLt, SyntaxFlowParserMapBuilderOpen, SyntaxFlowParserDollarOutput, SyntaxFlowParserStar, SyntaxFlowParserLineComment, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserAlert, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
			{
				p.SetState(456)
				p.Statements()
			}
			p.SetState(461)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			for _la == SyntaxFlowParserComma {
				{
					p.SetState(457)
					p.Match(SyntaxFlowParserComma)
				}
				{
					p.SetState(458)
					p.Statements()
				}

				p.SetState(463)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)
			}

		case SyntaxFlowParserDeep:
			{
				p.SetState(464)
				p.Match(SyntaxFlowParserDeep)
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}
		{
			p.SetState(467)
			p.Match(SyntaxFlowParserListSelectClose)
		}

	case SyntaxFlowParserMapBuilderOpen:
		localctx = NewBuildMapContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(468)
			p.Match(SyntaxFlowParserMapBuilderOpen)
		}
		p.SetState(484)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-56)) & ^0x3f) == 0 && ((int64(1)<<(_la-56))&88080319) != 0 {
			{
				p.SetState(469)
				p.Identifier()
			}
			{
				p.SetState(470)
				p.Match(SyntaxFlowParserColon)
			}

			{
				p.SetState(472)
				p.Statements()
			}
			p.SetState(481)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 72, p.GetParserRuleContext())

			for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
				if _alt == 1 {
					{
						p.SetState(473)
						p.Match(SyntaxFlowParserT__1)
					}

					{
						p.SetState(474)
						p.Identifier()
					}
					{
						p.SetState(475)
						p.Match(SyntaxFlowParserColon)
					}

					{
						p.SetState(477)
						p.Statements()
					}

				}
				p.SetState(483)
				p.GetErrorHandler().Sync(p)
				_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 72, p.GetParserRuleContext())
			}

		}
		p.SetState(487)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserT__1 {
			{
				p.SetState(486)
				p.Match(SyntaxFlowParserT__1)
			}

		}
		{
			p.SetState(489)
			p.Match(SyntaxFlowParserMapBuilderClose)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IStringLiteralWithoutStarGroupContext is an interface to support dynamic dispatch.
type IStringLiteralWithoutStarGroupContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStringLiteralWithoutStarGroupContext differentiates from other interfaces.
	IsStringLiteralWithoutStarGroupContext()
}

type StringLiteralWithoutStarGroupContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStringLiteralWithoutStarGroupContext() *StringLiteralWithoutStarGroupContext {
	var p = new(StringLiteralWithoutStarGroupContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_stringLiteralWithoutStarGroup
	return p
}

func (*StringLiteralWithoutStarGroupContext) IsStringLiteralWithoutStarGroupContext() {}

func NewStringLiteralWithoutStarGroupContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StringLiteralWithoutStarGroupContext {
	var p = new(StringLiteralWithoutStarGroupContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_stringLiteralWithoutStarGroup

	return p
}

func (s *StringLiteralWithoutStarGroupContext) GetParser() antlr.Parser { return s.parser }

func (s *StringLiteralWithoutStarGroupContext) AllStringLiteralWithoutStar() []IStringLiteralWithoutStarContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStringLiteralWithoutStarContext); ok {
			len++
		}
	}

	tst := make([]IStringLiteralWithoutStarContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStringLiteralWithoutStarContext); ok {
			tst[i] = t.(IStringLiteralWithoutStarContext)
			i++
		}
	}

	return tst
}

func (s *StringLiteralWithoutStarGroupContext) StringLiteralWithoutStar(i int) IStringLiteralWithoutStarContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStringLiteralWithoutStarContext); ok {
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

	return t.(IStringLiteralWithoutStarContext)
}

func (s *StringLiteralWithoutStarGroupContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserComma)
}

func (s *StringLiteralWithoutStarGroupContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserComma, i)
}

func (s *StringLiteralWithoutStarGroupContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StringLiteralWithoutStarGroupContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StringLiteralWithoutStarGroupContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitStringLiteralWithoutStarGroup(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) StringLiteralWithoutStarGroup() (localctx IStringLiteralWithoutStarGroupContext) {
	this := p
	_ = this

	localctx = NewStringLiteralWithoutStarGroupContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 78, SyntaxFlowParserRULE_stringLiteralWithoutStarGroup)

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
		p.SetState(492)
		p.StringLiteralWithoutStar()
	}
	p.SetState(497)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 76, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(493)
				p.Match(SyntaxFlowParserComma)
			}
			{
				p.SetState(494)
				p.StringLiteralWithoutStar()
			}

		}
		p.SetState(499)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 76, p.GetParserRuleContext())
	}
	p.SetState(501)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 77, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(500)
			p.Match(SyntaxFlowParserComma)
		}

	}

	return localctx
}

// INegativeConditionContext is an interface to support dynamic dispatch.
type INegativeConditionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNegativeConditionContext differentiates from other interfaces.
	IsNegativeConditionContext()
}

type NegativeConditionContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNegativeConditionContext() *NegativeConditionContext {
	var p = new(NegativeConditionContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_negativeCondition
	return p
}

func (*NegativeConditionContext) IsNegativeConditionContext() {}

func NewNegativeConditionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NegativeConditionContext {
	var p = new(NegativeConditionContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_negativeCondition

	return p
}

func (s *NegativeConditionContext) GetParser() antlr.Parser { return s.parser }

func (s *NegativeConditionContext) Not() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserNot, 0)
}

func (s *NegativeConditionContext) Bang() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserBang, 0)
}

func (s *NegativeConditionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NegativeConditionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NegativeConditionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitNegativeCondition(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) NegativeCondition() (localctx INegativeConditionContext) {
	this := p
	_ = this

	localctx = NewNegativeConditionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 80, SyntaxFlowParserRULE_negativeCondition)
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
		p.SetState(503)
		_la = p.GetTokenStream().LA(1)

		if !(_la == SyntaxFlowParserBang || _la == SyntaxFlowParserNot) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IConditionExpressionContext is an interface to support dynamic dispatch.
type IConditionExpressionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsConditionExpressionContext differentiates from other interfaces.
	IsConditionExpressionContext()
}

type ConditionExpressionContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyConditionExpressionContext() *ConditionExpressionContext {
	var p = new(ConditionExpressionContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_conditionExpression
	return p
}

func (*ConditionExpressionContext) IsConditionExpressionContext() {}

func NewConditionExpressionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ConditionExpressionContext {
	var p = new(ConditionExpressionContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_conditionExpression

	return p
}

func (s *ConditionExpressionContext) GetParser() antlr.Parser { return s.parser }

func (s *ConditionExpressionContext) CopyFrom(ctx *ConditionExpressionContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *ConditionExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ConditionExpressionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type NotConditionContext struct {
	*ConditionExpressionContext
}

func NewNotConditionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *NotConditionContext {
	var p = new(NotConditionContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *NotConditionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NotConditionContext) NegativeCondition() INegativeConditionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INegativeConditionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INegativeConditionContext)
}

func (s *NotConditionContext) ConditionExpression() IConditionExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConditionExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConditionExpressionContext)
}

func (s *NotConditionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitNotCondition(s)

	default:
		return t.VisitChildren(s)
	}
}

type ParenConditionContext struct {
	*ConditionExpressionContext
}

func NewParenConditionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ParenConditionContext {
	var p = new(ParenConditionContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *ParenConditionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ParenConditionContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserOpenParen, 0)
}

func (s *ParenConditionContext) ConditionExpression() IConditionExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConditionExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConditionExpressionContext)
}

func (s *ParenConditionContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCloseParen, 0)
}

func (s *ParenConditionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitParenCondition(s)

	default:
		return t.VisitChildren(s)
	}
}

type FilterConditionContext struct {
	*ConditionExpressionContext
}

func NewFilterConditionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FilterConditionContext {
	var p = new(FilterConditionContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *FilterConditionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterConditionContext) FilterExpr() IFilterExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFilterExprContext)
}

func (s *FilterConditionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilterCondition(s)

	default:
		return t.VisitChildren(s)
	}
}

type OpcodeTypeConditionContext struct {
	*ConditionExpressionContext
}

func NewOpcodeTypeConditionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *OpcodeTypeConditionContext {
	var p = new(OpcodeTypeConditionContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *OpcodeTypeConditionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OpcodeTypeConditionContext) Opcode() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserOpcode, 0)
}

func (s *OpcodeTypeConditionContext) Colon() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserColon, 0)
}

func (s *OpcodeTypeConditionContext) AllOpcodes() []IOpcodesContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IOpcodesContext); ok {
			len++
		}
	}

	tst := make([]IOpcodesContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IOpcodesContext); ok {
			tst[i] = t.(IOpcodesContext)
			i++
		}
	}

	return tst
}

func (s *OpcodeTypeConditionContext) Opcodes(i int) IOpcodesContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOpcodesContext); ok {
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

	return t.(IOpcodesContext)
}

func (s *OpcodeTypeConditionContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserComma)
}

func (s *OpcodeTypeConditionContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserComma, i)
}

func (s *OpcodeTypeConditionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitOpcodeTypeCondition(s)

	default:
		return t.VisitChildren(s)
	}
}

type FilterExpressionOrContext struct {
	*ConditionExpressionContext
}

func NewFilterExpressionOrContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FilterExpressionOrContext {
	var p = new(FilterExpressionOrContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *FilterExpressionOrContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterExpressionOrContext) AllConditionExpression() []IConditionExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IConditionExpressionContext); ok {
			len++
		}
	}

	tst := make([]IConditionExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IConditionExpressionContext); ok {
			tst[i] = t.(IConditionExpressionContext)
			i++
		}
	}

	return tst
}

func (s *FilterExpressionOrContext) ConditionExpression(i int) IConditionExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConditionExpressionContext); ok {
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

	return t.(IConditionExpressionContext)
}

func (s *FilterExpressionOrContext) Or() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserOr, 0)
}

func (s *FilterExpressionOrContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilterExpressionOr(s)

	default:
		return t.VisitChildren(s)
	}
}

type FilterExpressionAndContext struct {
	*ConditionExpressionContext
}

func NewFilterExpressionAndContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FilterExpressionAndContext {
	var p = new(FilterExpressionAndContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *FilterExpressionAndContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterExpressionAndContext) AllConditionExpression() []IConditionExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IConditionExpressionContext); ok {
			len++
		}
	}

	tst := make([]IConditionExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IConditionExpressionContext); ok {
			tst[i] = t.(IConditionExpressionContext)
			i++
		}
	}

	return tst
}

func (s *FilterExpressionAndContext) ConditionExpression(i int) IConditionExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConditionExpressionContext); ok {
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

	return t.(IConditionExpressionContext)
}

func (s *FilterExpressionAndContext) And() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserAnd, 0)
}

func (s *FilterExpressionAndContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilterExpressionAnd(s)

	default:
		return t.VisitChildren(s)
	}
}

type FilterExpressionCompareContext struct {
	*ConditionExpressionContext
	op antlr.Token
}

func NewFilterExpressionCompareContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FilterExpressionCompareContext {
	var p = new(FilterExpressionCompareContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *FilterExpressionCompareContext) GetOp() antlr.Token { return s.op }

func (s *FilterExpressionCompareContext) SetOp(v antlr.Token) { s.op = v }

func (s *FilterExpressionCompareContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterExpressionCompareContext) Gt() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserGt, 0)
}

func (s *FilterExpressionCompareContext) Lt() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserLt, 0)
}

func (s *FilterExpressionCompareContext) Eq() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserEq, 0)
}

func (s *FilterExpressionCompareContext) EqEq() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserEqEq, 0)
}

func (s *FilterExpressionCompareContext) GtEq() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserGtEq, 0)
}

func (s *FilterExpressionCompareContext) LtEq() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserLtEq, 0)
}

func (s *FilterExpressionCompareContext) NotEq() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserNotEq, 0)
}

func (s *FilterExpressionCompareContext) NumberLiteral() INumberLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INumberLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INumberLiteralContext)
}

func (s *FilterExpressionCompareContext) Identifier() IIdentifierContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIdentifierContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIdentifierContext)
}

func (s *FilterExpressionCompareContext) BoolLiteral() IBoolLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBoolLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBoolLiteralContext)
}

func (s *FilterExpressionCompareContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilterExpressionCompare(s)

	default:
		return t.VisitChildren(s)
	}
}

type FilterExpressionRegexpMatchContext struct {
	*ConditionExpressionContext
	op antlr.Token
}

func NewFilterExpressionRegexpMatchContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FilterExpressionRegexpMatchContext {
	var p = new(FilterExpressionRegexpMatchContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *FilterExpressionRegexpMatchContext) GetOp() antlr.Token { return s.op }

func (s *FilterExpressionRegexpMatchContext) SetOp(v antlr.Token) { s.op = v }

func (s *FilterExpressionRegexpMatchContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterExpressionRegexpMatchContext) RegexpMatch() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserRegexpMatch, 0)
}

func (s *FilterExpressionRegexpMatchContext) NotRegexpMatch() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserNotRegexpMatch, 0)
}

func (s *FilterExpressionRegexpMatchContext) StringLiteral() IStringLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStringLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStringLiteralContext)
}

func (s *FilterExpressionRegexpMatchContext) RegexpLiteral() IRegexpLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRegexpLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRegexpLiteralContext)
}

func (s *FilterExpressionRegexpMatchContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilterExpressionRegexpMatch(s)

	default:
		return t.VisitChildren(s)
	}
}

type StringContainAnyConditionContext struct {
	*ConditionExpressionContext
}

func NewStringContainAnyConditionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *StringContainAnyConditionContext {
	var p = new(StringContainAnyConditionContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *StringContainAnyConditionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StringContainAnyConditionContext) HaveAny() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserHaveAny, 0)
}

func (s *StringContainAnyConditionContext) Colon() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserColon, 0)
}

func (s *StringContainAnyConditionContext) StringLiteralWithoutStarGroup() IStringLiteralWithoutStarGroupContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStringLiteralWithoutStarGroupContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStringLiteralWithoutStarGroupContext)
}

func (s *StringContainAnyConditionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitStringContainAnyCondition(s)

	default:
		return t.VisitChildren(s)
	}
}

type StringContainHaveConditionContext struct {
	*ConditionExpressionContext
}

func NewStringContainHaveConditionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *StringContainHaveConditionContext {
	var p = new(StringContainHaveConditionContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *StringContainHaveConditionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StringContainHaveConditionContext) Have() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserHave, 0)
}

func (s *StringContainHaveConditionContext) Colon() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserColon, 0)
}

func (s *StringContainHaveConditionContext) StringLiteralWithoutStarGroup() IStringLiteralWithoutStarGroupContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStringLiteralWithoutStarGroupContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStringLiteralWithoutStarGroupContext)
}

func (s *StringContainHaveConditionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitStringContainHaveCondition(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) ConditionExpression() (localctx IConditionExpressionContext) {
	return p.conditionExpression(0)
}

func (p *SyntaxFlowParser) conditionExpression(_p int) (localctx IConditionExpressionContext) {
	this := p
	_ = this

	var _parentctx antlr.ParserRuleContext = p.GetParserRuleContext()
	_parentState := p.GetState()
	localctx = NewConditionExpressionContext(p, p.GetParserRuleContext(), _parentState)
	var _prevctx IConditionExpressionContext = localctx
	var _ antlr.ParserRuleContext = _prevctx // TODO: To prevent unused variable warning.
	_startState := 82
	p.EnterRecursionRule(localctx, 82, SyntaxFlowParserRULE_conditionExpression, _p)
	var _la int

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
	p.SetState(544)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 82, p.GetParserRuleContext()) {
	case 1:
		localctx = NewParenConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx

		{
			p.SetState(506)
			p.Match(SyntaxFlowParserOpenParen)
		}
		{
			p.SetState(507)
			p.conditionExpression(0)
		}
		{
			p.SetState(508)
			p.Match(SyntaxFlowParserCloseParen)
		}

	case 2:
		localctx = NewFilterConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(510)
			p.FilterExpr()
		}

	case 3:
		localctx = NewOpcodeTypeConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(511)
			p.Match(SyntaxFlowParserOpcode)
		}
		{
			p.SetState(512)
			p.Match(SyntaxFlowParserColon)
		}
		{
			p.SetState(513)
			p.Opcodes()
		}
		p.SetState(518)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 78, p.GetParserRuleContext())

		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(514)
					p.Match(SyntaxFlowParserComma)
				}
				{
					p.SetState(515)
					p.Opcodes()
				}

			}
			p.SetState(520)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 78, p.GetParserRuleContext())
		}
		p.SetState(522)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 79, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(521)
				p.Match(SyntaxFlowParserComma)
			}

		}

	case 4:
		localctx = NewStringContainHaveConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(524)
			p.Match(SyntaxFlowParserHave)
		}
		{
			p.SetState(525)
			p.Match(SyntaxFlowParserColon)
		}
		{
			p.SetState(526)
			p.StringLiteralWithoutStarGroup()
		}

	case 5:
		localctx = NewStringContainAnyConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(527)
			p.Match(SyntaxFlowParserHaveAny)
		}
		{
			p.SetState(528)
			p.Match(SyntaxFlowParserColon)
		}
		{
			p.SetState(529)
			p.StringLiteralWithoutStarGroup()
		}

	case 6:
		localctx = NewNotConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(530)
			p.NegativeCondition()
		}
		{
			p.SetState(531)
			p.conditionExpression(5)
		}

	case 7:
		localctx = NewFilterExpressionCompareContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(533)

			var _lt = p.GetTokenStream().LT(1)

			localctx.(*FilterExpressionCompareContext).op = _lt

			_la = p.GetTokenStream().LA(1)

			if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&1745917952) != 0) {
				var _ri = p.GetErrorHandler().RecoverInline(p)

				localctx.(*FilterExpressionCompareContext).op = _ri
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}
		p.SetState(537)
		p.GetErrorHandler().Sync(p)
		switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 80, p.GetParserRuleContext()) {
		case 1:
			{
				p.SetState(534)
				p.NumberLiteral()
			}

		case 2:
			{
				p.SetState(535)
				p.Identifier()
			}

		case 3:
			{
				p.SetState(536)
				p.BoolLiteral()
			}

		}

	case 8:
		localctx = NewFilterExpressionRegexpMatchContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(539)

			var _lt = p.GetTokenStream().LT(1)

			localctx.(*FilterExpressionRegexpMatchContext).op = _lt

			_la = p.GetTokenStream().LA(1)

			if !(_la == SyntaxFlowParserRegexpMatch || _la == SyntaxFlowParserNotRegexpMatch) {
				var _ri = p.GetErrorHandler().RecoverInline(p)

				localctx.(*FilterExpressionRegexpMatchContext).op = _ri
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}
		p.SetState(542)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
			{
				p.SetState(540)
				p.StringLiteral()
			}

		case SyntaxFlowParserRegexpLiteral:
			{
				p.SetState(541)
				p.RegexpLiteral()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

	}
	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(554)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 84, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			p.SetState(552)
			p.GetErrorHandler().Sync(p)
			switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 83, p.GetParserRuleContext()) {
			case 1:
				localctx = NewFilterExpressionAndContext(p, NewConditionExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_conditionExpression)
				p.SetState(546)

				if !(p.Precpred(p.GetParserRuleContext(), 2)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 2)", ""))
				}
				{
					p.SetState(547)
					p.Match(SyntaxFlowParserAnd)
				}
				{
					p.SetState(548)
					p.conditionExpression(3)
				}

			case 2:
				localctx = NewFilterExpressionOrContext(p, NewConditionExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_conditionExpression)
				p.SetState(549)

				if !(p.Precpred(p.GetParserRuleContext(), 1)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 1)", ""))
				}
				{
					p.SetState(550)
					p.Match(SyntaxFlowParserOr)
				}
				{
					p.SetState(551)
					p.conditionExpression(2)
				}

			}

		}
		p.SetState(556)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 84, p.GetParserRuleContext())
	}

	return localctx
}

// INumberLiteralContext is an interface to support dynamic dispatch.
type INumberLiteralContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNumberLiteralContext differentiates from other interfaces.
	IsNumberLiteralContext()
}

type NumberLiteralContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNumberLiteralContext() *NumberLiteralContext {
	var p = new(NumberLiteralContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_numberLiteral
	return p
}

func (*NumberLiteralContext) IsNumberLiteralContext() {}

func NewNumberLiteralContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NumberLiteralContext {
	var p = new(NumberLiteralContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_numberLiteral

	return p
}

func (s *NumberLiteralContext) GetParser() antlr.Parser { return s.parser }

func (s *NumberLiteralContext) Number() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserNumber, 0)
}

func (s *NumberLiteralContext) OctalNumber() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserOctalNumber, 0)
}

func (s *NumberLiteralContext) BinaryNumber() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserBinaryNumber, 0)
}

func (s *NumberLiteralContext) HexNumber() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserHexNumber, 0)
}

func (s *NumberLiteralContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NumberLiteralContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NumberLiteralContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitNumberLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) NumberLiteral() (localctx INumberLiteralContext) {
	this := p
	_ = this

	localctx = NewNumberLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 84, SyntaxFlowParserRULE_numberLiteral)
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
		p.SetState(557)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&67553994410557440) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IStringLiteralContext is an interface to support dynamic dispatch.
type IStringLiteralContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStringLiteralContext differentiates from other interfaces.
	IsStringLiteralContext()
}

type StringLiteralContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStringLiteralContext() *StringLiteralContext {
	var p = new(StringLiteralContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_stringLiteral
	return p
}

func (*StringLiteralContext) IsStringLiteralContext() {}

func NewStringLiteralContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StringLiteralContext {
	var p = new(StringLiteralContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_stringLiteral

	return p
}

func (s *StringLiteralContext) GetParser() antlr.Parser { return s.parser }

func (s *StringLiteralContext) Identifier() IIdentifierContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIdentifierContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIdentifierContext)
}

func (s *StringLiteralContext) Star() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserStar, 0)
}

func (s *StringLiteralContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StringLiteralContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StringLiteralContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitStringLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) StringLiteral() (localctx IStringLiteralContext) {
	this := p
	_ = this

	localctx = NewStringLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 86, SyntaxFlowParserRULE_stringLiteral)

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

	p.SetState(561)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(559)
			p.Identifier()
		}

	case SyntaxFlowParserStar:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(560)
			p.Match(SyntaxFlowParserStar)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IStringLiteralWithoutStarContext is an interface to support dynamic dispatch.
type IStringLiteralWithoutStarContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStringLiteralWithoutStarContext differentiates from other interfaces.
	IsStringLiteralWithoutStarContext()
}

type StringLiteralWithoutStarContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStringLiteralWithoutStarContext() *StringLiteralWithoutStarContext {
	var p = new(StringLiteralWithoutStarContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_stringLiteralWithoutStar
	return p
}

func (*StringLiteralWithoutStarContext) IsStringLiteralWithoutStarContext() {}

func NewStringLiteralWithoutStarContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StringLiteralWithoutStarContext {
	var p = new(StringLiteralWithoutStarContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_stringLiteralWithoutStar

	return p
}

func (s *StringLiteralWithoutStarContext) GetParser() antlr.Parser { return s.parser }

func (s *StringLiteralWithoutStarContext) Identifier() IIdentifierContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIdentifierContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIdentifierContext)
}

func (s *StringLiteralWithoutStarContext) RegexpLiteral() IRegexpLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRegexpLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRegexpLiteralContext)
}

func (s *StringLiteralWithoutStarContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StringLiteralWithoutStarContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StringLiteralWithoutStarContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitStringLiteralWithoutStar(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) StringLiteralWithoutStar() (localctx IStringLiteralWithoutStarContext) {
	this := p
	_ = this

	localctx = NewStringLiteralWithoutStarContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 88, SyntaxFlowParserRULE_stringLiteralWithoutStar)

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

	p.SetState(565)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(563)
			p.Identifier()
		}

	case SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(564)
			p.RegexpLiteral()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IRegexpLiteralContext is an interface to support dynamic dispatch.
type IRegexpLiteralContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsRegexpLiteralContext differentiates from other interfaces.
	IsRegexpLiteralContext()
}

type RegexpLiteralContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyRegexpLiteralContext() *RegexpLiteralContext {
	var p = new(RegexpLiteralContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_regexpLiteral
	return p
}

func (*RegexpLiteralContext) IsRegexpLiteralContext() {}

func NewRegexpLiteralContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *RegexpLiteralContext {
	var p = new(RegexpLiteralContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_regexpLiteral

	return p
}

func (s *RegexpLiteralContext) GetParser() antlr.Parser { return s.parser }

func (s *RegexpLiteralContext) RegexpLiteral() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserRegexpLiteral, 0)
}

func (s *RegexpLiteralContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RegexpLiteralContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *RegexpLiteralContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitRegexpLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) RegexpLiteral() (localctx IRegexpLiteralContext) {
	this := p
	_ = this

	localctx = NewRegexpLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 90, SyntaxFlowParserRULE_regexpLiteral)

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
		p.SetState(567)
		p.Match(SyntaxFlowParserRegexpLiteral)
	}

	return localctx
}

// IIdentifierContext is an interface to support dynamic dispatch.
type IIdentifierContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsIdentifierContext differentiates from other interfaces.
	IsIdentifierContext()
}

type IdentifierContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIdentifierContext() *IdentifierContext {
	var p = new(IdentifierContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_identifier
	return p
}

func (*IdentifierContext) IsIdentifierContext() {}

func NewIdentifierContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *IdentifierContext {
	var p = new(IdentifierContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_identifier

	return p
}

func (s *IdentifierContext) GetParser() antlr.Parser { return s.parser }

func (s *IdentifierContext) Identifier() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserIdentifier, 0)
}

func (s *IdentifierContext) Keywords() IKeywordsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IKeywordsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IKeywordsContext)
}

func (s *IdentifierContext) QuotedStringLiteral() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserQuotedStringLiteral, 0)
}

func (s *IdentifierContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *IdentifierContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *IdentifierContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitIdentifier(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Identifier() (localctx IIdentifierContext) {
	this := p
	_ = this

	localctx = NewIdentifierContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 92, SyntaxFlowParserRULE_identifier)

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

	p.SetState(572)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserIdentifier:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(569)
			p.Match(SyntaxFlowParserIdentifier)
		}

	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(570)
			p.Keywords()
		}

	case SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(571)
			p.Match(SyntaxFlowParserQuotedStringLiteral)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IKeywordsContext is an interface to support dynamic dispatch.
type IKeywordsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsKeywordsContext differentiates from other interfaces.
	IsKeywordsContext()
}

type KeywordsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyKeywordsContext() *KeywordsContext {
	var p = new(KeywordsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_keywords
	return p
}

func (*KeywordsContext) IsKeywordsContext() {}

func NewKeywordsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *KeywordsContext {
	var p = new(KeywordsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_keywords

	return p
}

func (s *KeywordsContext) GetParser() antlr.Parser { return s.parser }

func (s *KeywordsContext) Types() ITypesContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITypesContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ITypesContext)
}

func (s *KeywordsContext) Opcodes() IOpcodesContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOpcodesContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOpcodesContext)
}

func (s *KeywordsContext) Opcode() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserOpcode, 0)
}

func (s *KeywordsContext) Check() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCheck, 0)
}

func (s *KeywordsContext) Then() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserThen, 0)
}

func (s *KeywordsContext) Desc() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDesc, 0)
}

func (s *KeywordsContext) Else() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserElse, 0)
}

func (s *KeywordsContext) Type() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserType, 0)
}

func (s *KeywordsContext) In() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserIn, 0)
}

func (s *KeywordsContext) Have() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserHave, 0)
}

func (s *KeywordsContext) HaveAny() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserHaveAny, 0)
}

func (s *KeywordsContext) BoolLiteral() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserBoolLiteral, 0)
}

func (s *KeywordsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *KeywordsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *KeywordsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitKeywords(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Keywords() (localctx IKeywordsContext) {
	this := p
	_ = this

	localctx = NewKeywordsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 94, SyntaxFlowParserRULE_keywords)

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

	p.SetState(586)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(574)
			p.Types()
		}

	case SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(575)
			p.Opcodes()
		}

	case SyntaxFlowParserOpcode:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(576)
			p.Match(SyntaxFlowParserOpcode)
		}

	case SyntaxFlowParserCheck:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(577)
			p.Match(SyntaxFlowParserCheck)
		}

	case SyntaxFlowParserThen:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(578)
			p.Match(SyntaxFlowParserThen)
		}

	case SyntaxFlowParserDesc:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(579)
			p.Match(SyntaxFlowParserDesc)
		}

	case SyntaxFlowParserElse:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(580)
			p.Match(SyntaxFlowParserElse)
		}

	case SyntaxFlowParserType:
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(581)
			p.Match(SyntaxFlowParserType)
		}

	case SyntaxFlowParserIn:
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(582)
			p.Match(SyntaxFlowParserIn)
		}

	case SyntaxFlowParserHave:
		p.EnterOuterAlt(localctx, 10)
		{
			p.SetState(583)
			p.Match(SyntaxFlowParserHave)
		}

	case SyntaxFlowParserHaveAny:
		p.EnterOuterAlt(localctx, 11)
		{
			p.SetState(584)
			p.Match(SyntaxFlowParserHaveAny)
		}

	case SyntaxFlowParserBoolLiteral:
		p.EnterOuterAlt(localctx, 12)
		{
			p.SetState(585)
			p.Match(SyntaxFlowParserBoolLiteral)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IOpcodesContext is an interface to support dynamic dispatch.
type IOpcodesContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsOpcodesContext differentiates from other interfaces.
	IsOpcodesContext()
}

type OpcodesContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOpcodesContext() *OpcodesContext {
	var p = new(OpcodesContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_opcodes
	return p
}

func (*OpcodesContext) IsOpcodesContext() {}

func NewOpcodesContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OpcodesContext {
	var p = new(OpcodesContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_opcodes

	return p
}

func (s *OpcodesContext) GetParser() antlr.Parser { return s.parser }

func (s *OpcodesContext) Call() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCall, 0)
}

func (s *OpcodesContext) Constant() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserConstant, 0)
}

func (s *OpcodesContext) Phi() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserPhi, 0)
}

func (s *OpcodesContext) FormalParam() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserFormalParam, 0)
}

func (s *OpcodesContext) Return() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserReturn, 0)
}

func (s *OpcodesContext) Function() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserFunction, 0)
}

func (s *OpcodesContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OpcodesContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OpcodesContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitOpcodes(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Opcodes() (localctx IOpcodesContext) {
	this := p
	_ = this

	localctx = NewOpcodesContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 96, SyntaxFlowParserRULE_opcodes)
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
		p.SetState(588)
		_la = p.GetTokenStream().LA(1)

		if !((int64((_la-69)) & ^0x3f) == 0 && ((int64(1)<<(_la-69))&63) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// ITypesContext is an interface to support dynamic dispatch.
type ITypesContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsTypesContext differentiates from other interfaces.
	IsTypesContext()
}

type TypesContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyTypesContext() *TypesContext {
	var p = new(TypesContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_types
	return p
}

func (*TypesContext) IsTypesContext() {}

func NewTypesContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *TypesContext {
	var p = new(TypesContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_types

	return p
}

func (s *TypesContext) GetParser() antlr.Parser { return s.parser }

func (s *TypesContext) StringType() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserStringType, 0)
}

func (s *TypesContext) NumberType() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserNumberType, 0)
}

func (s *TypesContext) ListType() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserListType, 0)
}

func (s *TypesContext) DictType() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDictType, 0)
}

func (s *TypesContext) BoolType() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserBoolType, 0)
}

func (s *TypesContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TypesContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *TypesContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitTypes(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Types() (localctx ITypesContext) {
	this := p
	_ = this

	localctx = NewTypesContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 98, SyntaxFlowParserRULE_types)
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
		p.SetState(590)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&2233785415175766016) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IBoolLiteralContext is an interface to support dynamic dispatch.
type IBoolLiteralContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsBoolLiteralContext differentiates from other interfaces.
	IsBoolLiteralContext()
}

type BoolLiteralContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyBoolLiteralContext() *BoolLiteralContext {
	var p = new(BoolLiteralContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_boolLiteral
	return p
}

func (*BoolLiteralContext) IsBoolLiteralContext() {}

func NewBoolLiteralContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *BoolLiteralContext {
	var p = new(BoolLiteralContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_boolLiteral

	return p
}

func (s *BoolLiteralContext) GetParser() antlr.Parser { return s.parser }

func (s *BoolLiteralContext) BoolLiteral() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserBoolLiteral, 0)
}

func (s *BoolLiteralContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BoolLiteralContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *BoolLiteralContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitBoolLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) BoolLiteral() (localctx IBoolLiteralContext) {
	this := p
	_ = this

	localctx = NewBoolLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 100, SyntaxFlowParserRULE_boolLiteral)

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
		p.SetState(592)
		p.Match(SyntaxFlowParserBoolLiteral)
	}

	return localctx
}

func (p *SyntaxFlowParser) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 41:
		var t *ConditionExpressionContext = nil
		if localctx != nil {
			t = localctx.(*ConditionExpressionContext)
		}
		return p.ConditionExpression_Sempred(t, predIndex)

	default:
		panic("No predicate with index: " + fmt.Sprint(ruleIndex))
	}
}

func (p *SyntaxFlowParser) ConditionExpression_Sempred(localctx antlr.RuleContext, predIndex int) bool {
	this := p
	_ = this

	switch predIndex {
	case 0:
		return p.Precpred(p.GetParserRuleContext(), 2)

	case 1:
		return p.Precpred(p.GetParserRuleContext(), 1)

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}
