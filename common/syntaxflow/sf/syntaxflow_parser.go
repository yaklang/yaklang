// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package sf // SyntaxFlowParser
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

var syntaxflowparserParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	literalNames           []string
	symbolicNames          []string
	ruleNames              []string
	predictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func syntaxflowparserParserInit() {
	staticData := &syntaxflowparserParserStaticData
	staticData.literalNames = []string{
		"", "'==>'", "'...'", "'%%'", "'..'", "'<='", "'>='", "'>>'", "'=>'",
		"'=='", "'=~'", "'!~'", "'&&'", "'||'", "'!='", "'${'", "';'", "'?{'",
		"'-{'", "'->'", "'}->'", "'-->'", "'#{'", "'#>'", "'#->'", "'>'", "'.'",
		"'<<<'", "'<'", "'='", "'+'", "'&'", "'?'", "'('", "','", "')'", "'['",
		"']'", "'{'", "'}'", "'#'", "'$'", "':'", "'%'", "'!'", "'*'", "'-'",
		"'as'", "'`'", "'''", "'\"'", "", "'\\n'", "", "", "", "", "", "'str'",
		"'list'", "'dict'", "", "'bool'", "", "'alert'", "'check'", "'then'",
		"", "'else'", "'type'", "'in'", "'call'", "", "", "'phi'", "", "", "'opcode'",
		"'have'", "'any'", "'not'", "'for'", "'r'", "'g'", "'e'",
	}
	staticData.symbolicNames = []string{
		"", "DeepFilter", "Deep", "Percent", "DeepDot", "LtEq", "GtEq", "DoubleGt",
		"Filter", "EqEq", "RegexpMatch", "NotRegexpMatch", "And", "Or", "NotEq",
		"DollarBraceOpen", "Semicolon", "ConditionStart", "DeepNextStart", "UseStart",
		"DeepNextEnd", "DeepNext", "TopDefStart", "DefStart", "TopDef", "Gt",
		"Dot", "StartNowDoc", "Lt", "Eq", "Add", "Amp", "Question", "OpenParen",
		"Comma", "CloseParen", "ListSelectOpen", "ListSelectClose", "MapBuilderOpen",
		"MapBuilderClose", "ListStart", "DollarOutput", "Colon", "Search", "Bang",
		"Star", "Minus", "As", "Backtick", "SingleQuote", "DoubleQuote", "LineComment",
		"BreakLine", "WhiteSpace", "Number", "OctalNumber", "BinaryNumber",
		"HexNumber", "StringType", "ListType", "DictType", "NumberType", "BoolType",
		"BoolLiteral", "Alert", "Check", "Then", "Desc", "Else", "Type", "In",
		"Call", "Function", "Constant", "Phi", "FormalParam", "Return", "Opcode",
		"Have", "HaveAny", "Not", "For", "ConstSearchModePrefixRegexp", "ConstSearchModePrefixGlob",
		"ConstSearchModePrefixExact", "Identifier", "IdentifierChar", "QuotedStringLiteral",
		"RegexpLiteral", "WS", "HereDocIdentifierName", "CRLFHereDocIdentifierBreak",
		"LFHereDocIdentifierBreak", "CRLFEndDoc", "CRLFHereDocText", "LFEndDoc",
		"LFHereDocText",
	}
	staticData.ruleNames = []string{
		"flow", "statements", "statement", "fileFilterContentStatement", "fileFilterContentInput",
		"fileFilterContentMethod", "fileFilterContentMethodParam", "fileFilterContentMethodParamItem",
		"fileFilterContentMethodParamKey", "fileFilterContentMethodParamValue",
		"fileName", "filterStatement", "comment", "eos", "line", "lines", "descriptionStatement",
		"descriptionItems", "descriptionItem", "descriptionItemValue", "crlfHereDoc",
		"lfHereDoc", "crlfText", "lfText", "hereDoc", "alertStatement", "checkStatement",
		"thenExpr", "elseExpr", "refVariable", "filterItemFirst", "constSearchPrefix",
		"filterItem", "filterExpr", "nativeCall", "useNativeCall", "useDefCalcParams",
		"nativeCallActualParams", "nativeCallActualParam", "nativeCallActualParamKey",
		"nativeCallActualParamValue", "actualParam", "actualParamFilter", "singleParam",
		"config", "recursiveConfigItem", "recursiveConfigItemValue", "sliceCallItem",
		"nameFilter", "chainFilter", "stringLiteralWithoutStarGroup", "negativeCondition",
		"conditionExpression", "opcodesCondition", "numberLiteral", "stringLiteral",
		"stringLiteralWithoutStar", "regexpLiteral", "identifier", "keywords",
		"opcodes", "types", "boolLiteral",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 96, 715, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7, 20, 2,
		21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25, 2, 26,
		7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 2, 29, 7, 29, 2, 30, 7, 30, 2, 31, 7,
		31, 2, 32, 7, 32, 2, 33, 7, 33, 2, 34, 7, 34, 2, 35, 7, 35, 2, 36, 7, 36,
		2, 37, 7, 37, 2, 38, 7, 38, 2, 39, 7, 39, 2, 40, 7, 40, 2, 41, 7, 41, 2,
		42, 7, 42, 2, 43, 7, 43, 2, 44, 7, 44, 2, 45, 7, 45, 2, 46, 7, 46, 2, 47,
		7, 47, 2, 48, 7, 48, 2, 49, 7, 49, 2, 50, 7, 50, 2, 51, 7, 51, 2, 52, 7,
		52, 2, 53, 7, 53, 2, 54, 7, 54, 2, 55, 7, 55, 2, 56, 7, 56, 2, 57, 7, 57,
		2, 58, 7, 58, 2, 59, 7, 59, 2, 60, 7, 60, 2, 61, 7, 61, 2, 62, 7, 62, 1,
		0, 1, 0, 1, 0, 1, 1, 4, 1, 131, 8, 1, 11, 1, 12, 1, 132, 1, 2, 1, 2, 3,
		2, 137, 8, 2, 1, 2, 1, 2, 3, 2, 141, 8, 2, 1, 2, 1, 2, 3, 2, 145, 8, 2,
		1, 2, 1, 2, 3, 2, 149, 8, 2, 1, 2, 1, 2, 3, 2, 153, 8, 2, 1, 2, 1, 2, 3,
		2, 157, 8, 2, 1, 2, 3, 2, 160, 8, 2, 1, 3, 1, 3, 1, 3, 1, 3, 3, 3, 166,
		8, 3, 1, 3, 1, 3, 1, 3, 1, 3, 3, 3, 172, 8, 3, 1, 4, 1, 4, 3, 4, 176, 8,
		4, 1, 5, 1, 5, 1, 5, 3, 5, 181, 8, 5, 1, 5, 1, 5, 1, 6, 1, 6, 3, 6, 187,
		8, 6, 1, 6, 1, 6, 3, 6, 191, 8, 6, 1, 6, 1, 6, 3, 6, 195, 8, 6, 5, 6, 197,
		8, 6, 10, 6, 12, 6, 200, 9, 6, 1, 6, 3, 6, 203, 8, 6, 1, 6, 3, 6, 206,
		8, 6, 1, 7, 3, 7, 209, 8, 7, 1, 7, 1, 7, 1, 8, 1, 8, 1, 8, 1, 9, 1, 9,
		1, 10, 1, 10, 1, 10, 5, 10, 221, 8, 10, 10, 10, 12, 10, 224, 9, 10, 1,
		11, 1, 11, 5, 11, 228, 8, 11, 10, 11, 12, 11, 231, 9, 11, 1, 11, 1, 11,
		3, 11, 235, 8, 11, 1, 11, 1, 11, 1, 11, 3, 11, 240, 8, 11, 3, 11, 242,
		8, 11, 1, 12, 1, 12, 1, 13, 1, 13, 3, 13, 248, 8, 13, 1, 14, 1, 14, 1,
		15, 4, 15, 253, 8, 15, 11, 15, 12, 15, 254, 1, 16, 1, 16, 1, 16, 3, 16,
		260, 8, 16, 1, 16, 1, 16, 1, 16, 3, 16, 265, 8, 16, 1, 16, 3, 16, 268,
		8, 16, 1, 17, 3, 17, 271, 8, 17, 1, 17, 1, 17, 1, 17, 3, 17, 276, 8, 17,
		1, 17, 5, 17, 279, 8, 17, 10, 17, 12, 17, 282, 9, 17, 1, 17, 3, 17, 285,
		8, 17, 1, 17, 3, 17, 288, 8, 17, 1, 18, 1, 18, 3, 18, 292, 8, 18, 1, 18,
		1, 18, 1, 18, 1, 18, 3, 18, 298, 8, 18, 3, 18, 300, 8, 18, 1, 19, 1, 19,
		1, 19, 3, 19, 305, 8, 19, 1, 20, 1, 20, 3, 20, 309, 8, 20, 1, 20, 1, 20,
		1, 21, 1, 21, 3, 21, 315, 8, 21, 1, 21, 1, 21, 1, 22, 4, 22, 320, 8, 22,
		11, 22, 12, 22, 321, 1, 23, 4, 23, 325, 8, 23, 11, 23, 12, 23, 326, 1,
		24, 1, 24, 1, 24, 1, 24, 3, 24, 333, 8, 24, 1, 25, 1, 25, 1, 25, 1, 25,
		3, 25, 339, 8, 25, 1, 26, 1, 26, 1, 26, 3, 26, 344, 8, 26, 1, 26, 3, 26,
		347, 8, 26, 1, 27, 1, 27, 1, 27, 1, 28, 1, 28, 1, 28, 1, 29, 1, 29, 1,
		29, 1, 29, 1, 29, 1, 29, 3, 29, 361, 8, 29, 1, 30, 3, 30, 364, 8, 30, 1,
		30, 1, 30, 3, 30, 368, 8, 30, 1, 30, 1, 30, 1, 30, 3, 30, 373, 8, 30, 1,
		30, 1, 30, 3, 30, 377, 8, 30, 1, 31, 1, 31, 1, 32, 1, 32, 1, 32, 3, 32,
		384, 8, 32, 1, 32, 1, 32, 1, 32, 3, 32, 389, 8, 32, 1, 32, 3, 32, 392,
		8, 32, 1, 32, 1, 32, 1, 32, 1, 32, 1, 32, 1, 32, 1, 32, 1, 32, 1, 32, 1,
		32, 1, 32, 1, 32, 1, 32, 1, 32, 3, 32, 408, 8, 32, 1, 32, 1, 32, 1, 32,
		1, 32, 3, 32, 414, 8, 32, 1, 32, 1, 32, 1, 32, 1, 32, 1, 32, 1, 32, 1,
		32, 3, 32, 423, 8, 32, 1, 33, 1, 33, 5, 33, 427, 8, 33, 10, 33, 12, 33,
		430, 9, 33, 1, 34, 1, 34, 1, 34, 1, 34, 1, 35, 1, 35, 3, 35, 438, 8, 35,
		1, 36, 1, 36, 3, 36, 442, 8, 36, 1, 36, 1, 36, 1, 36, 3, 36, 447, 8, 36,
		1, 36, 3, 36, 450, 8, 36, 1, 37, 3, 37, 453, 8, 37, 1, 37, 1, 37, 1, 37,
		3, 37, 458, 8, 37, 1, 37, 5, 37, 461, 8, 37, 10, 37, 12, 37, 464, 9, 37,
		1, 37, 3, 37, 467, 8, 37, 1, 37, 3, 37, 470, 8, 37, 1, 38, 1, 38, 1, 38,
		3, 38, 475, 8, 38, 1, 38, 1, 38, 1, 39, 1, 39, 1, 40, 1, 40, 1, 40, 1,
		40, 5, 40, 485, 8, 40, 10, 40, 12, 40, 488, 9, 40, 1, 40, 1, 40, 1, 40,
		1, 40, 3, 40, 494, 8, 40, 1, 41, 1, 41, 3, 41, 498, 8, 41, 1, 41, 4, 41,
		501, 8, 41, 11, 41, 12, 41, 502, 1, 41, 3, 41, 506, 8, 41, 1, 41, 3, 41,
		509, 8, 41, 3, 41, 511, 8, 41, 1, 42, 1, 42, 1, 42, 1, 42, 3, 42, 517,
		8, 42, 1, 43, 1, 43, 1, 43, 3, 43, 522, 8, 43, 1, 43, 3, 43, 525, 8, 43,
		1, 43, 1, 43, 1, 44, 1, 44, 1, 44, 5, 44, 532, 8, 44, 10, 44, 12, 44, 535,
		9, 44, 1, 44, 3, 44, 538, 8, 44, 1, 44, 3, 44, 541, 8, 44, 1, 45, 3, 45,
		544, 8, 45, 1, 45, 1, 45, 1, 45, 1, 45, 3, 45, 550, 8, 45, 1, 46, 1, 46,
		3, 46, 554, 8, 46, 1, 46, 1, 46, 1, 46, 1, 46, 3, 46, 560, 8, 46, 1, 47,
		1, 47, 3, 47, 564, 8, 47, 1, 48, 1, 48, 1, 48, 3, 48, 569, 8, 48, 1, 49,
		1, 49, 1, 49, 1, 49, 5, 49, 575, 8, 49, 10, 49, 12, 49, 578, 9, 49, 1,
		49, 3, 49, 581, 8, 49, 1, 49, 1, 49, 1, 49, 1, 49, 1, 49, 1, 49, 1, 49,
		1, 49, 1, 49, 1, 49, 1, 49, 1, 49, 5, 49, 595, 8, 49, 10, 49, 12, 49, 598,
		9, 49, 3, 49, 600, 8, 49, 1, 49, 3, 49, 603, 8, 49, 1, 49, 3, 49, 606,
		8, 49, 1, 50, 1, 50, 1, 50, 5, 50, 611, 8, 50, 10, 50, 12, 50, 614, 9,
		50, 1, 50, 3, 50, 617, 8, 50, 1, 51, 1, 51, 1, 52, 1, 52, 1, 52, 1, 52,
		1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 5, 52, 632, 8, 52, 10,
		52, 12, 52, 635, 9, 52, 1, 52, 3, 52, 638, 8, 52, 1, 52, 1, 52, 1, 52,
		1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 3,
		52, 653, 8, 52, 1, 52, 1, 52, 1, 52, 3, 52, 658, 8, 52, 3, 52, 660, 8,
		52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 1, 52, 5, 52, 668, 8, 52, 10, 52,
		12, 52, 671, 9, 52, 1, 53, 1, 53, 3, 53, 675, 8, 53, 1, 54, 1, 54, 1, 55,
		1, 55, 3, 55, 681, 8, 55, 1, 56, 1, 56, 3, 56, 685, 8, 56, 1, 57, 1, 57,
		1, 58, 1, 58, 1, 58, 3, 58, 692, 8, 58, 1, 59, 1, 59, 1, 59, 1, 59, 1,
		59, 1, 59, 1, 59, 1, 59, 1, 59, 1, 59, 1, 59, 1, 59, 1, 59, 3, 59, 707,
		8, 59, 1, 60, 1, 60, 1, 61, 1, 61, 1, 62, 1, 62, 1, 62, 0, 1, 104, 63,
		0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30, 32, 34, 36,
		38, 40, 42, 44, 46, 48, 50, 52, 54, 56, 58, 60, 62, 64, 66, 68, 70, 72,
		74, 76, 78, 80, 82, 84, 86, 88, 90, 92, 94, 96, 98, 100, 102, 104, 106,
		108, 110, 112, 114, 116, 118, 120, 122, 124, 0, 9, 1, 0, 82, 84, 2, 0,
		29, 29, 42, 42, 1, 0, 48, 48, 2, 0, 44, 44, 80, 80, 5, 0, 5, 6, 9, 9, 14,
		14, 25, 25, 28, 29, 1, 0, 10, 11, 1, 0, 54, 57, 1, 0, 71, 76, 1, 0, 58,
		62, 802, 0, 126, 1, 0, 0, 0, 2, 130, 1, 0, 0, 0, 4, 159, 1, 0, 0, 0, 6,
		161, 1, 0, 0, 0, 8, 175, 1, 0, 0, 0, 10, 177, 1, 0, 0, 0, 12, 184, 1, 0,
		0, 0, 14, 208, 1, 0, 0, 0, 16, 212, 1, 0, 0, 0, 18, 215, 1, 0, 0, 0, 20,
		217, 1, 0, 0, 0, 22, 241, 1, 0, 0, 0, 24, 243, 1, 0, 0, 0, 26, 247, 1,
		0, 0, 0, 28, 249, 1, 0, 0, 0, 30, 252, 1, 0, 0, 0, 32, 267, 1, 0, 0, 0,
		34, 270, 1, 0, 0, 0, 36, 299, 1, 0, 0, 0, 38, 304, 1, 0, 0, 0, 40, 306,
		1, 0, 0, 0, 42, 312, 1, 0, 0, 0, 44, 319, 1, 0, 0, 0, 46, 324, 1, 0, 0,
		0, 48, 328, 1, 0, 0, 0, 50, 334, 1, 0, 0, 0, 52, 340, 1, 0, 0, 0, 54, 348,
		1, 0, 0, 0, 56, 351, 1, 0, 0, 0, 58, 354, 1, 0, 0, 0, 60, 376, 1, 0, 0,
		0, 62, 378, 1, 0, 0, 0, 64, 422, 1, 0, 0, 0, 66, 424, 1, 0, 0, 0, 68, 431,
		1, 0, 0, 0, 70, 435, 1, 0, 0, 0, 72, 449, 1, 0, 0, 0, 74, 452, 1, 0, 0,
		0, 76, 474, 1, 0, 0, 0, 78, 478, 1, 0, 0, 0, 80, 493, 1, 0, 0, 0, 82, 510,
		1, 0, 0, 0, 84, 516, 1, 0, 0, 0, 86, 524, 1, 0, 0, 0, 88, 528, 1, 0, 0,
		0, 90, 543, 1, 0, 0, 0, 92, 559, 1, 0, 0, 0, 94, 563, 1, 0, 0, 0, 96, 568,
		1, 0, 0, 0, 98, 605, 1, 0, 0, 0, 100, 607, 1, 0, 0, 0, 102, 618, 1, 0,
		0, 0, 104, 659, 1, 0, 0, 0, 106, 674, 1, 0, 0, 0, 108, 676, 1, 0, 0, 0,
		110, 680, 1, 0, 0, 0, 112, 684, 1, 0, 0, 0, 114, 686, 1, 0, 0, 0, 116,
		691, 1, 0, 0, 0, 118, 706, 1, 0, 0, 0, 120, 708, 1, 0, 0, 0, 122, 710,
		1, 0, 0, 0, 124, 712, 1, 0, 0, 0, 126, 127, 3, 2, 1, 0, 127, 128, 5, 0,
		0, 1, 128, 1, 1, 0, 0, 0, 129, 131, 3, 4, 2, 0, 130, 129, 1, 0, 0, 0, 131,
		132, 1, 0, 0, 0, 132, 130, 1, 0, 0, 0, 132, 133, 1, 0, 0, 0, 133, 3, 1,
		0, 0, 0, 134, 136, 3, 52, 26, 0, 135, 137, 3, 26, 13, 0, 136, 135, 1, 0,
		0, 0, 136, 137, 1, 0, 0, 0, 137, 160, 1, 0, 0, 0, 138, 140, 3, 32, 16,
		0, 139, 141, 3, 26, 13, 0, 140, 139, 1, 0, 0, 0, 140, 141, 1, 0, 0, 0,
		141, 160, 1, 0, 0, 0, 142, 144, 3, 50, 25, 0, 143, 145, 3, 26, 13, 0, 144,
		143, 1, 0, 0, 0, 144, 145, 1, 0, 0, 0, 145, 160, 1, 0, 0, 0, 146, 148,
		3, 22, 11, 0, 147, 149, 3, 26, 13, 0, 148, 147, 1, 0, 0, 0, 148, 149, 1,
		0, 0, 0, 149, 160, 1, 0, 0, 0, 150, 152, 3, 6, 3, 0, 151, 153, 3, 26, 13,
		0, 152, 151, 1, 0, 0, 0, 152, 153, 1, 0, 0, 0, 153, 160, 1, 0, 0, 0, 154,
		156, 3, 24, 12, 0, 155, 157, 3, 26, 13, 0, 156, 155, 1, 0, 0, 0, 156, 157,
		1, 0, 0, 0, 157, 160, 1, 0, 0, 0, 158, 160, 3, 26, 13, 0, 159, 134, 1,
		0, 0, 0, 159, 138, 1, 0, 0, 0, 159, 142, 1, 0, 0, 0, 159, 146, 1, 0, 0,
		0, 159, 150, 1, 0, 0, 0, 159, 154, 1, 0, 0, 0, 159, 158, 1, 0, 0, 0, 160,
		5, 1, 0, 0, 0, 161, 162, 5, 15, 0, 0, 162, 163, 3, 8, 4, 0, 163, 165, 5,
		39, 0, 0, 164, 166, 3, 30, 15, 0, 165, 164, 1, 0, 0, 0, 165, 166, 1, 0,
		0, 0, 166, 167, 1, 0, 0, 0, 167, 168, 5, 26, 0, 0, 168, 171, 3, 10, 5,
		0, 169, 170, 5, 47, 0, 0, 170, 172, 3, 58, 29, 0, 171, 169, 1, 0, 0, 0,
		171, 172, 1, 0, 0, 0, 172, 7, 1, 0, 0, 0, 173, 176, 3, 20, 10, 0, 174,
		176, 3, 114, 57, 0, 175, 173, 1, 0, 0, 0, 175, 174, 1, 0, 0, 0, 176, 9,
		1, 0, 0, 0, 177, 178, 5, 85, 0, 0, 178, 180, 5, 33, 0, 0, 179, 181, 3,
		12, 6, 0, 180, 179, 1, 0, 0, 0, 180, 181, 1, 0, 0, 0, 181, 182, 1, 0, 0,
		0, 182, 183, 5, 35, 0, 0, 183, 11, 1, 0, 0, 0, 184, 186, 3, 14, 7, 0, 185,
		187, 3, 30, 15, 0, 186, 185, 1, 0, 0, 0, 186, 187, 1, 0, 0, 0, 187, 198,
		1, 0, 0, 0, 188, 190, 5, 34, 0, 0, 189, 191, 3, 30, 15, 0, 190, 189, 1,
		0, 0, 0, 190, 191, 1, 0, 0, 0, 191, 192, 1, 0, 0, 0, 192, 194, 3, 14, 7,
		0, 193, 195, 3, 30, 15, 0, 194, 193, 1, 0, 0, 0, 194, 195, 1, 0, 0, 0,
		195, 197, 1, 0, 0, 0, 196, 188, 1, 0, 0, 0, 197, 200, 1, 0, 0, 0, 198,
		196, 1, 0, 0, 0, 198, 199, 1, 0, 0, 0, 199, 202, 1, 0, 0, 0, 200, 198,
		1, 0, 0, 0, 201, 203, 5, 34, 0, 0, 202, 201, 1, 0, 0, 0, 202, 203, 1, 0,
		0, 0, 203, 205, 1, 0, 0, 0, 204, 206, 3, 30, 15, 0, 205, 204, 1, 0, 0,
		0, 205, 206, 1, 0, 0, 0, 206, 13, 1, 0, 0, 0, 207, 209, 3, 16, 8, 0, 208,
		207, 1, 0, 0, 0, 208, 209, 1, 0, 0, 0, 209, 210, 1, 0, 0, 0, 210, 211,
		3, 18, 9, 0, 211, 15, 1, 0, 0, 0, 212, 213, 5, 85, 0, 0, 213, 214, 5, 42,
		0, 0, 214, 17, 1, 0, 0, 0, 215, 216, 3, 96, 48, 0, 216, 19, 1, 0, 0, 0,
		217, 222, 3, 96, 48, 0, 218, 219, 9, 0, 0, 0, 219, 221, 3, 96, 48, 0, 220,
		218, 1, 0, 0, 0, 221, 224, 1, 0, 0, 0, 222, 220, 1, 0, 0, 0, 222, 223,
		1, 0, 0, 0, 223, 21, 1, 0, 0, 0, 224, 222, 1, 0, 0, 0, 225, 229, 3, 58,
		29, 0, 226, 228, 3, 64, 32, 0, 227, 226, 1, 0, 0, 0, 228, 231, 1, 0, 0,
		0, 229, 227, 1, 0, 0, 0, 229, 230, 1, 0, 0, 0, 230, 234, 1, 0, 0, 0, 231,
		229, 1, 0, 0, 0, 232, 233, 5, 47, 0, 0, 233, 235, 3, 58, 29, 0, 234, 232,
		1, 0, 0, 0, 234, 235, 1, 0, 0, 0, 235, 242, 1, 0, 0, 0, 236, 239, 3, 66,
		33, 0, 237, 238, 5, 47, 0, 0, 238, 240, 3, 58, 29, 0, 239, 237, 1, 0, 0,
		0, 239, 240, 1, 0, 0, 0, 240, 242, 1, 0, 0, 0, 241, 225, 1, 0, 0, 0, 241,
		236, 1, 0, 0, 0, 242, 23, 1, 0, 0, 0, 243, 244, 5, 51, 0, 0, 244, 25, 1,
		0, 0, 0, 245, 248, 5, 16, 0, 0, 246, 248, 3, 28, 14, 0, 247, 245, 1, 0,
		0, 0, 247, 246, 1, 0, 0, 0, 248, 27, 1, 0, 0, 0, 249, 250, 5, 52, 0, 0,
		250, 29, 1, 0, 0, 0, 251, 253, 3, 28, 14, 0, 252, 251, 1, 0, 0, 0, 253,
		254, 1, 0, 0, 0, 254, 252, 1, 0, 0, 0, 254, 255, 1, 0, 0, 0, 255, 31, 1,
		0, 0, 0, 256, 257, 5, 67, 0, 0, 257, 259, 5, 33, 0, 0, 258, 260, 3, 34,
		17, 0, 259, 258, 1, 0, 0, 0, 259, 260, 1, 0, 0, 0, 260, 261, 1, 0, 0, 0,
		261, 268, 5, 35, 0, 0, 262, 264, 5, 38, 0, 0, 263, 265, 3, 34, 17, 0, 264,
		263, 1, 0, 0, 0, 264, 265, 1, 0, 0, 0, 265, 266, 1, 0, 0, 0, 266, 268,
		5, 39, 0, 0, 267, 256, 1, 0, 0, 0, 267, 262, 1, 0, 0, 0, 268, 33, 1, 0,
		0, 0, 269, 271, 3, 30, 15, 0, 270, 269, 1, 0, 0, 0, 270, 271, 1, 0, 0,
		0, 271, 272, 1, 0, 0, 0, 272, 280, 3, 36, 18, 0, 273, 275, 5, 34, 0, 0,
		274, 276, 3, 30, 15, 0, 275, 274, 1, 0, 0, 0, 275, 276, 1, 0, 0, 0, 276,
		277, 1, 0, 0, 0, 277, 279, 3, 36, 18, 0, 278, 273, 1, 0, 0, 0, 279, 282,
		1, 0, 0, 0, 280, 278, 1, 0, 0, 0, 280, 281, 1, 0, 0, 0, 281, 284, 1, 0,
		0, 0, 282, 280, 1, 0, 0, 0, 283, 285, 5, 34, 0, 0, 284, 283, 1, 0, 0, 0,
		284, 285, 1, 0, 0, 0, 285, 287, 1, 0, 0, 0, 286, 288, 3, 30, 15, 0, 287,
		286, 1, 0, 0, 0, 287, 288, 1, 0, 0, 0, 288, 35, 1, 0, 0, 0, 289, 291, 3,
		110, 55, 0, 290, 292, 3, 30, 15, 0, 291, 290, 1, 0, 0, 0, 291, 292, 1,
		0, 0, 0, 292, 300, 1, 0, 0, 0, 293, 294, 3, 110, 55, 0, 294, 295, 5, 42,
		0, 0, 295, 297, 3, 38, 19, 0, 296, 298, 3, 30, 15, 0, 297, 296, 1, 0, 0,
		0, 297, 298, 1, 0, 0, 0, 298, 300, 1, 0, 0, 0, 299, 289, 1, 0, 0, 0, 299,
		293, 1, 0, 0, 0, 300, 37, 1, 0, 0, 0, 301, 305, 3, 110, 55, 0, 302, 305,
		3, 48, 24, 0, 303, 305, 3, 108, 54, 0, 304, 301, 1, 0, 0, 0, 304, 302,
		1, 0, 0, 0, 304, 303, 1, 0, 0, 0, 305, 39, 1, 0, 0, 0, 306, 308, 5, 91,
		0, 0, 307, 309, 3, 44, 22, 0, 308, 307, 1, 0, 0, 0, 308, 309, 1, 0, 0,
		0, 309, 310, 1, 0, 0, 0, 310, 311, 5, 93, 0, 0, 311, 41, 1, 0, 0, 0, 312,
		314, 5, 92, 0, 0, 313, 315, 3, 46, 23, 0, 314, 313, 1, 0, 0, 0, 314, 315,
		1, 0, 0, 0, 315, 316, 1, 0, 0, 0, 316, 317, 5, 95, 0, 0, 317, 43, 1, 0,
		0, 0, 318, 320, 5, 94, 0, 0, 319, 318, 1, 0, 0, 0, 320, 321, 1, 0, 0, 0,
		321, 319, 1, 0, 0, 0, 321, 322, 1, 0, 0, 0, 322, 45, 1, 0, 0, 0, 323, 325,
		5, 96, 0, 0, 324, 323, 1, 0, 0, 0, 325, 326, 1, 0, 0, 0, 326, 324, 1, 0,
		0, 0, 326, 327, 1, 0, 0, 0, 327, 47, 1, 0, 0, 0, 328, 329, 5, 27, 0, 0,
		329, 332, 5, 90, 0, 0, 330, 333, 3, 40, 20, 0, 331, 333, 3, 42, 21, 0,
		332, 330, 1, 0, 0, 0, 332, 331, 1, 0, 0, 0, 333, 49, 1, 0, 0, 0, 334, 335,
		5, 64, 0, 0, 335, 338, 3, 58, 29, 0, 336, 337, 5, 81, 0, 0, 337, 339, 3,
		110, 55, 0, 338, 336, 1, 0, 0, 0, 338, 339, 1, 0, 0, 0, 339, 51, 1, 0,
		0, 0, 340, 341, 5, 65, 0, 0, 341, 343, 3, 58, 29, 0, 342, 344, 3, 54, 27,
		0, 343, 342, 1, 0, 0, 0, 343, 344, 1, 0, 0, 0, 344, 346, 1, 0, 0, 0, 345,
		347, 3, 56, 28, 0, 346, 345, 1, 0, 0, 0, 346, 347, 1, 0, 0, 0, 347, 53,
		1, 0, 0, 0, 348, 349, 5, 66, 0, 0, 349, 350, 3, 110, 55, 0, 350, 55, 1,
		0, 0, 0, 351, 352, 5, 68, 0, 0, 352, 353, 3, 110, 55, 0, 353, 57, 1, 0,
		0, 0, 354, 360, 5, 41, 0, 0, 355, 361, 3, 116, 58, 0, 356, 357, 5, 33,
		0, 0, 357, 358, 3, 116, 58, 0, 358, 359, 5, 35, 0, 0, 359, 361, 1, 0, 0,
		0, 360, 355, 1, 0, 0, 0, 360, 356, 1, 0, 0, 0, 361, 59, 1, 0, 0, 0, 362,
		364, 3, 62, 31, 0, 363, 362, 1, 0, 0, 0, 363, 364, 1, 0, 0, 0, 364, 367,
		1, 0, 0, 0, 365, 368, 5, 87, 0, 0, 366, 368, 3, 48, 24, 0, 367, 365, 1,
		0, 0, 0, 367, 366, 1, 0, 0, 0, 368, 377, 1, 0, 0, 0, 369, 377, 3, 96, 48,
		0, 370, 372, 5, 26, 0, 0, 371, 373, 3, 30, 15, 0, 372, 371, 1, 0, 0, 0,
		372, 373, 1, 0, 0, 0, 373, 374, 1, 0, 0, 0, 374, 377, 3, 96, 48, 0, 375,
		377, 3, 68, 34, 0, 376, 363, 1, 0, 0, 0, 376, 369, 1, 0, 0, 0, 376, 370,
		1, 0, 0, 0, 376, 375, 1, 0, 0, 0, 377, 61, 1, 0, 0, 0, 378, 379, 7, 0,
		0, 0, 379, 63, 1, 0, 0, 0, 380, 423, 3, 60, 30, 0, 381, 383, 5, 2, 0, 0,
		382, 384, 3, 30, 15, 0, 383, 382, 1, 0, 0, 0, 383, 384, 1, 0, 0, 0, 384,
		385, 1, 0, 0, 0, 385, 423, 3, 96, 48, 0, 386, 388, 5, 33, 0, 0, 387, 389,
		3, 30, 15, 0, 388, 387, 1, 0, 0, 0, 388, 389, 1, 0, 0, 0, 389, 391, 1,
		0, 0, 0, 390, 392, 3, 82, 41, 0, 391, 390, 1, 0, 0, 0, 391, 392, 1, 0,
		0, 0, 392, 393, 1, 0, 0, 0, 393, 423, 5, 35, 0, 0, 394, 395, 5, 36, 0,
		0, 395, 396, 3, 94, 47, 0, 396, 397, 5, 37, 0, 0, 397, 423, 1, 0, 0, 0,
		398, 399, 5, 17, 0, 0, 399, 400, 3, 104, 52, 0, 400, 401, 5, 39, 0, 0,
		401, 423, 1, 0, 0, 0, 402, 423, 5, 19, 0, 0, 403, 423, 5, 23, 0, 0, 404,
		423, 5, 21, 0, 0, 405, 407, 5, 18, 0, 0, 406, 408, 3, 88, 44, 0, 407, 406,
		1, 0, 0, 0, 407, 408, 1, 0, 0, 0, 408, 409, 1, 0, 0, 0, 409, 423, 5, 20,
		0, 0, 410, 423, 5, 24, 0, 0, 411, 413, 5, 22, 0, 0, 412, 414, 3, 88, 44,
		0, 413, 412, 1, 0, 0, 0, 413, 414, 1, 0, 0, 0, 414, 415, 1, 0, 0, 0, 415,
		423, 5, 20, 0, 0, 416, 417, 5, 30, 0, 0, 417, 423, 3, 58, 29, 0, 418, 419,
		5, 46, 0, 0, 419, 423, 3, 58, 29, 0, 420, 421, 5, 31, 0, 0, 421, 423, 3,
		58, 29, 0, 422, 380, 1, 0, 0, 0, 422, 381, 1, 0, 0, 0, 422, 386, 1, 0,
		0, 0, 422, 394, 1, 0, 0, 0, 422, 398, 1, 0, 0, 0, 422, 402, 1, 0, 0, 0,
		422, 403, 1, 0, 0, 0, 422, 404, 1, 0, 0, 0, 422, 405, 1, 0, 0, 0, 422,
		410, 1, 0, 0, 0, 422, 411, 1, 0, 0, 0, 422, 416, 1, 0, 0, 0, 422, 418,
		1, 0, 0, 0, 422, 420, 1, 0, 0, 0, 423, 65, 1, 0, 0, 0, 424, 428, 3, 60,
		30, 0, 425, 427, 3, 64, 32, 0, 426, 425, 1, 0, 0, 0, 427, 430, 1, 0, 0,
		0, 428, 426, 1, 0, 0, 0, 428, 429, 1, 0, 0, 0, 429, 67, 1, 0, 0, 0, 430,
		428, 1, 0, 0, 0, 431, 432, 5, 28, 0, 0, 432, 433, 3, 70, 35, 0, 433, 434,
		5, 25, 0, 0, 434, 69, 1, 0, 0, 0, 435, 437, 3, 116, 58, 0, 436, 438, 3,
		72, 36, 0, 437, 436, 1, 0, 0, 0, 437, 438, 1, 0, 0, 0, 438, 71, 1, 0, 0,
		0, 439, 441, 5, 38, 0, 0, 440, 442, 3, 74, 37, 0, 441, 440, 1, 0, 0, 0,
		441, 442, 1, 0, 0, 0, 442, 443, 1, 0, 0, 0, 443, 450, 5, 39, 0, 0, 444,
		446, 5, 33, 0, 0, 445, 447, 3, 74, 37, 0, 446, 445, 1, 0, 0, 0, 446, 447,
		1, 0, 0, 0, 447, 448, 1, 0, 0, 0, 448, 450, 5, 35, 0, 0, 449, 439, 1, 0,
		0, 0, 449, 444, 1, 0, 0, 0, 450, 73, 1, 0, 0, 0, 451, 453, 3, 30, 15, 0,
		452, 451, 1, 0, 0, 0, 452, 453, 1, 0, 0, 0, 453, 454, 1, 0, 0, 0, 454,
		462, 3, 76, 38, 0, 455, 457, 5, 34, 0, 0, 456, 458, 3, 30, 15, 0, 457,
		456, 1, 0, 0, 0, 457, 458, 1, 0, 0, 0, 458, 459, 1, 0, 0, 0, 459, 461,
		3, 76, 38, 0, 460, 455, 1, 0, 0, 0, 461, 464, 1, 0, 0, 0, 462, 460, 1,
		0, 0, 0, 462, 463, 1, 0, 0, 0, 463, 466, 1, 0, 0, 0, 464, 462, 1, 0, 0,
		0, 465, 467, 5, 34, 0, 0, 466, 465, 1, 0, 0, 0, 466, 467, 1, 0, 0, 0, 467,
		469, 1, 0, 0, 0, 468, 470, 3, 30, 15, 0, 469, 468, 1, 0, 0, 0, 469, 470,
		1, 0, 0, 0, 470, 75, 1, 0, 0, 0, 471, 472, 3, 78, 39, 0, 472, 473, 7, 1,
		0, 0, 473, 475, 1, 0, 0, 0, 474, 471, 1, 0, 0, 0, 474, 475, 1, 0, 0, 0,
		475, 476, 1, 0, 0, 0, 476, 477, 3, 80, 40, 0, 477, 77, 1, 0, 0, 0, 478,
		479, 3, 116, 58, 0, 479, 79, 1, 0, 0, 0, 480, 494, 3, 116, 58, 0, 481,
		494, 3, 108, 54, 0, 482, 486, 5, 48, 0, 0, 483, 485, 8, 2, 0, 0, 484, 483,
		1, 0, 0, 0, 485, 488, 1, 0, 0, 0, 486, 484, 1, 0, 0, 0, 486, 487, 1, 0,
		0, 0, 487, 489, 1, 0, 0, 0, 488, 486, 1, 0, 0, 0, 489, 494, 5, 48, 0, 0,
		490, 491, 5, 41, 0, 0, 491, 494, 3, 116, 58, 0, 492, 494, 3, 48, 24, 0,
		493, 480, 1, 0, 0, 0, 493, 481, 1, 0, 0, 0, 493, 482, 1, 0, 0, 0, 493,
		490, 1, 0, 0, 0, 493, 492, 1, 0, 0, 0, 494, 81, 1, 0, 0, 0, 495, 497, 3,
		86, 43, 0, 496, 498, 3, 30, 15, 0, 497, 496, 1, 0, 0, 0, 497, 498, 1, 0,
		0, 0, 498, 511, 1, 0, 0, 0, 499, 501, 3, 84, 42, 0, 500, 499, 1, 0, 0,
		0, 501, 502, 1, 0, 0, 0, 502, 500, 1, 0, 0, 0, 502, 503, 1, 0, 0, 0, 503,
		505, 1, 0, 0, 0, 504, 506, 3, 86, 43, 0, 505, 504, 1, 0, 0, 0, 505, 506,
		1, 0, 0, 0, 506, 508, 1, 0, 0, 0, 507, 509, 3, 30, 15, 0, 508, 507, 1,
		0, 0, 0, 508, 509, 1, 0, 0, 0, 509, 511, 1, 0, 0, 0, 510, 495, 1, 0, 0,
		0, 510, 500, 1, 0, 0, 0, 511, 83, 1, 0, 0, 0, 512, 513, 3, 86, 43, 0, 513,
		514, 5, 34, 0, 0, 514, 517, 1, 0, 0, 0, 515, 517, 5, 34, 0, 0, 516, 512,
		1, 0, 0, 0, 516, 515, 1, 0, 0, 0, 517, 85, 1, 0, 0, 0, 518, 525, 5, 23,
		0, 0, 519, 521, 5, 22, 0, 0, 520, 522, 3, 88, 44, 0, 521, 520, 1, 0, 0,
		0, 521, 522, 1, 0, 0, 0, 522, 523, 1, 0, 0, 0, 523, 525, 5, 39, 0, 0, 524,
		518, 1, 0, 0, 0, 524, 519, 1, 0, 0, 0, 524, 525, 1, 0, 0, 0, 525, 526,
		1, 0, 0, 0, 526, 527, 3, 22, 11, 0, 527, 87, 1, 0, 0, 0, 528, 533, 3, 90,
		45, 0, 529, 530, 5, 34, 0, 0, 530, 532, 3, 90, 45, 0, 531, 529, 1, 0, 0,
		0, 532, 535, 1, 0, 0, 0, 533, 531, 1, 0, 0, 0, 533, 534, 1, 0, 0, 0, 534,
		537, 1, 0, 0, 0, 535, 533, 1, 0, 0, 0, 536, 538, 5, 34, 0, 0, 537, 536,
		1, 0, 0, 0, 537, 538, 1, 0, 0, 0, 538, 540, 1, 0, 0, 0, 539, 541, 3, 30,
		15, 0, 540, 539, 1, 0, 0, 0, 540, 541, 1, 0, 0, 0, 541, 89, 1, 0, 0, 0,
		542, 544, 3, 30, 15, 0, 543, 542, 1, 0, 0, 0, 543, 544, 1, 0, 0, 0, 544,
		545, 1, 0, 0, 0, 545, 546, 3, 116, 58, 0, 546, 547, 5, 42, 0, 0, 547, 549,
		3, 92, 46, 0, 548, 550, 3, 30, 15, 0, 549, 548, 1, 0, 0, 0, 549, 550, 1,
		0, 0, 0, 550, 91, 1, 0, 0, 0, 551, 554, 3, 116, 58, 0, 552, 554, 3, 108,
		54, 0, 553, 551, 1, 0, 0, 0, 553, 552, 1, 0, 0, 0, 554, 560, 1, 0, 0, 0,
		555, 556, 5, 48, 0, 0, 556, 557, 3, 22, 11, 0, 557, 558, 5, 48, 0, 0, 558,
		560, 1, 0, 0, 0, 559, 553, 1, 0, 0, 0, 559, 555, 1, 0, 0, 0, 560, 93, 1,
		0, 0, 0, 561, 564, 3, 96, 48, 0, 562, 564, 3, 108, 54, 0, 563, 561, 1,
		0, 0, 0, 563, 562, 1, 0, 0, 0, 564, 95, 1, 0, 0, 0, 565, 569, 5, 45, 0,
		0, 566, 569, 3, 116, 58, 0, 567, 569, 3, 114, 57, 0, 568, 565, 1, 0, 0,
		0, 568, 566, 1, 0, 0, 0, 568, 567, 1, 0, 0, 0, 569, 97, 1, 0, 0, 0, 570,
		580, 5, 36, 0, 0, 571, 576, 3, 2, 1, 0, 572, 573, 5, 34, 0, 0, 573, 575,
		3, 2, 1, 0, 574, 572, 1, 0, 0, 0, 575, 578, 1, 0, 0, 0, 576, 574, 1, 0,
		0, 0, 576, 577, 1, 0, 0, 0, 577, 581, 1, 0, 0, 0, 578, 576, 1, 0, 0, 0,
		579, 581, 5, 2, 0, 0, 580, 571, 1, 0, 0, 0, 580, 579, 1, 0, 0, 0, 581,
		582, 1, 0, 0, 0, 582, 606, 5, 37, 0, 0, 583, 599, 5, 38, 0, 0, 584, 585,
		3, 116, 58, 0, 585, 586, 5, 42, 0, 0, 586, 587, 1, 0, 0, 0, 587, 596, 3,
		2, 1, 0, 588, 589, 5, 16, 0, 0, 589, 590, 3, 116, 58, 0, 590, 591, 5, 42,
		0, 0, 591, 592, 1, 0, 0, 0, 592, 593, 3, 2, 1, 0, 593, 595, 1, 0, 0, 0,
		594, 588, 1, 0, 0, 0, 595, 598, 1, 0, 0, 0, 596, 594, 1, 0, 0, 0, 596,
		597, 1, 0, 0, 0, 597, 600, 1, 0, 0, 0, 598, 596, 1, 0, 0, 0, 599, 584,
		1, 0, 0, 0, 599, 600, 1, 0, 0, 0, 600, 602, 1, 0, 0, 0, 601, 603, 5, 16,
		0, 0, 602, 601, 1, 0, 0, 0, 602, 603, 1, 0, 0, 0, 603, 604, 1, 0, 0, 0,
		604, 606, 5, 39, 0, 0, 605, 570, 1, 0, 0, 0, 605, 583, 1, 0, 0, 0, 606,
		99, 1, 0, 0, 0, 607, 612, 3, 112, 56, 0, 608, 609, 5, 34, 0, 0, 609, 611,
		3, 112, 56, 0, 610, 608, 1, 0, 0, 0, 611, 614, 1, 0, 0, 0, 612, 610, 1,
		0, 0, 0, 612, 613, 1, 0, 0, 0, 613, 616, 1, 0, 0, 0, 614, 612, 1, 0, 0,
		0, 615, 617, 5, 34, 0, 0, 616, 615, 1, 0, 0, 0, 616, 617, 1, 0, 0, 0, 617,
		101, 1, 0, 0, 0, 618, 619, 7, 3, 0, 0, 619, 103, 1, 0, 0, 0, 620, 621,
		6, 52, -1, 0, 621, 622, 5, 33, 0, 0, 622, 623, 3, 104, 52, 0, 623, 624,
		5, 35, 0, 0, 624, 660, 1, 0, 0, 0, 625, 660, 3, 66, 33, 0, 626, 627, 5,
		77, 0, 0, 627, 628, 5, 42, 0, 0, 628, 633, 3, 106, 53, 0, 629, 630, 5,
		34, 0, 0, 630, 632, 3, 106, 53, 0, 631, 629, 1, 0, 0, 0, 632, 635, 1, 0,
		0, 0, 633, 631, 1, 0, 0, 0, 633, 634, 1, 0, 0, 0, 634, 637, 1, 0, 0, 0,
		635, 633, 1, 0, 0, 0, 636, 638, 5, 34, 0, 0, 637, 636, 1, 0, 0, 0, 637,
		638, 1, 0, 0, 0, 638, 660, 1, 0, 0, 0, 639, 640, 5, 78, 0, 0, 640, 641,
		5, 42, 0, 0, 641, 660, 3, 100, 50, 0, 642, 643, 5, 79, 0, 0, 643, 644,
		5, 42, 0, 0, 644, 660, 3, 100, 50, 0, 645, 646, 3, 102, 51, 0, 646, 647,
		3, 104, 52, 5, 647, 660, 1, 0, 0, 0, 648, 652, 7, 4, 0, 0, 649, 653, 3,
		108, 54, 0, 650, 653, 3, 116, 58, 0, 651, 653, 3, 124, 62, 0, 652, 649,
		1, 0, 0, 0, 652, 650, 1, 0, 0, 0, 652, 651, 1, 0, 0, 0, 653, 660, 1, 0,
		0, 0, 654, 657, 7, 5, 0, 0, 655, 658, 3, 110, 55, 0, 656, 658, 3, 114,
		57, 0, 657, 655, 1, 0, 0, 0, 657, 656, 1, 0, 0, 0, 658, 660, 1, 0, 0, 0,
		659, 620, 1, 0, 0, 0, 659, 625, 1, 0, 0, 0, 659, 626, 1, 0, 0, 0, 659,
		639, 1, 0, 0, 0, 659, 642, 1, 0, 0, 0, 659, 645, 1, 0, 0, 0, 659, 648,
		1, 0, 0, 0, 659, 654, 1, 0, 0, 0, 660, 669, 1, 0, 0, 0, 661, 662, 10, 2,
		0, 0, 662, 663, 5, 12, 0, 0, 663, 668, 3, 104, 52, 3, 664, 665, 10, 1,
		0, 0, 665, 666, 5, 13, 0, 0, 666, 668, 3, 104, 52, 2, 667, 661, 1, 0, 0,
		0, 667, 664, 1, 0, 0, 0, 668, 671, 1, 0, 0, 0, 669, 667, 1, 0, 0, 0, 669,
		670, 1, 0, 0, 0, 670, 105, 1, 0, 0, 0, 671, 669, 1, 0, 0, 0, 672, 675,
		3, 120, 60, 0, 673, 675, 3, 116, 58, 0, 674, 672, 1, 0, 0, 0, 674, 673,
		1, 0, 0, 0, 675, 107, 1, 0, 0, 0, 676, 677, 7, 6, 0, 0, 677, 109, 1, 0,
		0, 0, 678, 681, 3, 116, 58, 0, 679, 681, 5, 45, 0, 0, 680, 678, 1, 0, 0,
		0, 680, 679, 1, 0, 0, 0, 681, 111, 1, 0, 0, 0, 682, 685, 3, 116, 58, 0,
		683, 685, 3, 114, 57, 0, 684, 682, 1, 0, 0, 0, 684, 683, 1, 0, 0, 0, 685,
		113, 1, 0, 0, 0, 686, 687, 5, 88, 0, 0, 687, 115, 1, 0, 0, 0, 688, 692,
		5, 85, 0, 0, 689, 692, 3, 118, 59, 0, 690, 692, 5, 87, 0, 0, 691, 688,
		1, 0, 0, 0, 691, 689, 1, 0, 0, 0, 691, 690, 1, 0, 0, 0, 692, 117, 1, 0,
		0, 0, 693, 707, 3, 122, 61, 0, 694, 707, 3, 120, 60, 0, 695, 707, 5, 77,
		0, 0, 696, 707, 5, 65, 0, 0, 697, 707, 5, 66, 0, 0, 698, 707, 5, 67, 0,
		0, 699, 707, 5, 68, 0, 0, 700, 707, 5, 69, 0, 0, 701, 707, 5, 70, 0, 0,
		702, 707, 5, 78, 0, 0, 703, 707, 5, 79, 0, 0, 704, 707, 5, 63, 0, 0, 705,
		707, 3, 62, 31, 0, 706, 693, 1, 0, 0, 0, 706, 694, 1, 0, 0, 0, 706, 695,
		1, 0, 0, 0, 706, 696, 1, 0, 0, 0, 706, 697, 1, 0, 0, 0, 706, 698, 1, 0,
		0, 0, 706, 699, 1, 0, 0, 0, 706, 700, 1, 0, 0, 0, 706, 701, 1, 0, 0, 0,
		706, 702, 1, 0, 0, 0, 706, 703, 1, 0, 0, 0, 706, 704, 1, 0, 0, 0, 706,
		705, 1, 0, 0, 0, 707, 119, 1, 0, 0, 0, 708, 709, 7, 7, 0, 0, 709, 121,
		1, 0, 0, 0, 710, 711, 7, 8, 0, 0, 711, 123, 1, 0, 0, 0, 712, 713, 5, 63,
		0, 0, 713, 125, 1, 0, 0, 0, 107, 132, 136, 140, 144, 148, 152, 156, 159,
		165, 171, 175, 180, 186, 190, 194, 198, 202, 205, 208, 222, 229, 234, 239,
		241, 247, 254, 259, 264, 267, 270, 275, 280, 284, 287, 291, 297, 299, 304,
		308, 314, 321, 326, 332, 338, 343, 346, 360, 363, 367, 372, 376, 383, 388,
		391, 407, 413, 422, 428, 437, 441, 446, 449, 452, 457, 462, 466, 469, 474,
		486, 493, 497, 502, 505, 508, 510, 516, 521, 524, 533, 537, 540, 543, 549,
		553, 559, 563, 568, 576, 580, 596, 599, 602, 605, 612, 616, 633, 637, 652,
		657, 659, 667, 669, 674, 680, 684, 691, 706,
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
	staticData := &syntaxflowparserParserStaticData
	staticData.once.Do(syntaxflowparserParserInit)
}

// NewSyntaxFlowParser produces a new parser instance for the optional input antlr.TokenStream.
func NewSyntaxFlowParser(input antlr.TokenStream) *SyntaxFlowParser {
	SyntaxFlowParserInit()
	this := new(SyntaxFlowParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &syntaxflowparserParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.predictionContextCache)
	this.RuleNames = staticData.ruleNames
	this.LiteralNames = staticData.literalNames
	this.SymbolicNames = staticData.symbolicNames
	this.GrammarFileName = "java-escape"

	return this
}

// SyntaxFlowParser tokens.
const (
	SyntaxFlowParserEOF                         = antlr.TokenEOF
	SyntaxFlowParserDeepFilter                  = 1
	SyntaxFlowParserDeep                        = 2
	SyntaxFlowParserPercent                     = 3
	SyntaxFlowParserDeepDot                     = 4
	SyntaxFlowParserLtEq                        = 5
	SyntaxFlowParserGtEq                        = 6
	SyntaxFlowParserDoubleGt                    = 7
	SyntaxFlowParserFilter                      = 8
	SyntaxFlowParserEqEq                        = 9
	SyntaxFlowParserRegexpMatch                 = 10
	SyntaxFlowParserNotRegexpMatch              = 11
	SyntaxFlowParserAnd                         = 12
	SyntaxFlowParserOr                          = 13
	SyntaxFlowParserNotEq                       = 14
	SyntaxFlowParserDollarBraceOpen             = 15
	SyntaxFlowParserSemicolon                   = 16
	SyntaxFlowParserConditionStart              = 17
	SyntaxFlowParserDeepNextStart               = 18
	SyntaxFlowParserUseStart                    = 19
	SyntaxFlowParserDeepNextEnd                 = 20
	SyntaxFlowParserDeepNext                    = 21
	SyntaxFlowParserTopDefStart                 = 22
	SyntaxFlowParserDefStart                    = 23
	SyntaxFlowParserTopDef                      = 24
	SyntaxFlowParserGt                          = 25
	SyntaxFlowParserDot                         = 26
	SyntaxFlowParserStartNowDoc                 = 27
	SyntaxFlowParserLt                          = 28
	SyntaxFlowParserEq                          = 29
	SyntaxFlowParserAdd                         = 30
	SyntaxFlowParserAmp                         = 31
	SyntaxFlowParserQuestion                    = 32
	SyntaxFlowParserOpenParen                   = 33
	SyntaxFlowParserComma                       = 34
	SyntaxFlowParserCloseParen                  = 35
	SyntaxFlowParserListSelectOpen              = 36
	SyntaxFlowParserListSelectClose             = 37
	SyntaxFlowParserMapBuilderOpen              = 38
	SyntaxFlowParserMapBuilderClose             = 39
	SyntaxFlowParserListStart                   = 40
	SyntaxFlowParserDollarOutput                = 41
	SyntaxFlowParserColon                       = 42
	SyntaxFlowParserSearch                      = 43
	SyntaxFlowParserBang                        = 44
	SyntaxFlowParserStar                        = 45
	SyntaxFlowParserMinus                       = 46
	SyntaxFlowParserAs                          = 47
	SyntaxFlowParserBacktick                    = 48
	SyntaxFlowParserSingleQuote                 = 49
	SyntaxFlowParserDoubleQuote                 = 50
	SyntaxFlowParserLineComment                 = 51
	SyntaxFlowParserBreakLine                   = 52
	SyntaxFlowParserWhiteSpace                  = 53
	SyntaxFlowParserNumber                      = 54
	SyntaxFlowParserOctalNumber                 = 55
	SyntaxFlowParserBinaryNumber                = 56
	SyntaxFlowParserHexNumber                   = 57
	SyntaxFlowParserStringType                  = 58
	SyntaxFlowParserListType                    = 59
	SyntaxFlowParserDictType                    = 60
	SyntaxFlowParserNumberType                  = 61
	SyntaxFlowParserBoolType                    = 62
	SyntaxFlowParserBoolLiteral                 = 63
	SyntaxFlowParserAlert                       = 64
	SyntaxFlowParserCheck                       = 65
	SyntaxFlowParserThen                        = 66
	SyntaxFlowParserDesc                        = 67
	SyntaxFlowParserElse                        = 68
	SyntaxFlowParserType                        = 69
	SyntaxFlowParserIn                          = 70
	SyntaxFlowParserCall                        = 71
	SyntaxFlowParserFunction                    = 72
	SyntaxFlowParserConstant                    = 73
	SyntaxFlowParserPhi                         = 74
	SyntaxFlowParserFormalParam                 = 75
	SyntaxFlowParserReturn                      = 76
	SyntaxFlowParserOpcode                      = 77
	SyntaxFlowParserHave                        = 78
	SyntaxFlowParserHaveAny                     = 79
	SyntaxFlowParserNot                         = 80
	SyntaxFlowParserFor                         = 81
	SyntaxFlowParserConstSearchModePrefixRegexp = 82
	SyntaxFlowParserConstSearchModePrefixGlob   = 83
	SyntaxFlowParserConstSearchModePrefixExact  = 84
	SyntaxFlowParserIdentifier                  = 85
	SyntaxFlowParserIdentifierChar              = 86
	SyntaxFlowParserQuotedStringLiteral         = 87
	SyntaxFlowParserRegexpLiteral               = 88
	SyntaxFlowParserWS                          = 89
	SyntaxFlowParserHereDocIdentifierName       = 90
	SyntaxFlowParserCRLFHereDocIdentifierBreak  = 91
	SyntaxFlowParserLFHereDocIdentifierBreak    = 92
	SyntaxFlowParserCRLFEndDoc                  = 93
	SyntaxFlowParserCRLFHereDocText             = 94
	SyntaxFlowParserLFEndDoc                    = 95
	SyntaxFlowParserLFHereDocText               = 96
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
	SyntaxFlowParserRULE_descriptionItemValue              = 19
	SyntaxFlowParserRULE_crlfHereDoc                       = 20
	SyntaxFlowParserRULE_lfHereDoc                         = 21
	SyntaxFlowParserRULE_crlfText                          = 22
	SyntaxFlowParserRULE_lfText                            = 23
	SyntaxFlowParserRULE_hereDoc                           = 24
	SyntaxFlowParserRULE_alertStatement                    = 25
	SyntaxFlowParserRULE_checkStatement                    = 26
	SyntaxFlowParserRULE_thenExpr                          = 27
	SyntaxFlowParserRULE_elseExpr                          = 28
	SyntaxFlowParserRULE_refVariable                       = 29
	SyntaxFlowParserRULE_filterItemFirst                   = 30
	SyntaxFlowParserRULE_constSearchPrefix                 = 31
	SyntaxFlowParserRULE_filterItem                        = 32
	SyntaxFlowParserRULE_filterExpr                        = 33
	SyntaxFlowParserRULE_nativeCall                        = 34
	SyntaxFlowParserRULE_useNativeCall                     = 35
	SyntaxFlowParserRULE_useDefCalcParams                  = 36
	SyntaxFlowParserRULE_nativeCallActualParams            = 37
	SyntaxFlowParserRULE_nativeCallActualParam             = 38
	SyntaxFlowParserRULE_nativeCallActualParamKey          = 39
	SyntaxFlowParserRULE_nativeCallActualParamValue        = 40
	SyntaxFlowParserRULE_actualParam                       = 41
	SyntaxFlowParserRULE_actualParamFilter                 = 42
	SyntaxFlowParserRULE_singleParam                       = 43
	SyntaxFlowParserRULE_config                            = 44
	SyntaxFlowParserRULE_recursiveConfigItem               = 45
	SyntaxFlowParserRULE_recursiveConfigItemValue          = 46
	SyntaxFlowParserRULE_sliceCallItem                     = 47
	SyntaxFlowParserRULE_nameFilter                        = 48
	SyntaxFlowParserRULE_chainFilter                       = 49
	SyntaxFlowParserRULE_stringLiteralWithoutStarGroup     = 50
	SyntaxFlowParserRULE_negativeCondition                 = 51
	SyntaxFlowParserRULE_conditionExpression               = 52
	SyntaxFlowParserRULE_opcodesCondition                  = 53
	SyntaxFlowParserRULE_numberLiteral                     = 54
	SyntaxFlowParserRULE_stringLiteral                     = 55
	SyntaxFlowParserRULE_stringLiteralWithoutStar          = 56
	SyntaxFlowParserRULE_regexpLiteral                     = 57
	SyntaxFlowParserRULE_identifier                        = 58
	SyntaxFlowParserRULE_keywords                          = 59
	SyntaxFlowParserRULE_opcodes                           = 60
	SyntaxFlowParserRULE_types                             = 61
	SyntaxFlowParserRULE_boolLiteral                       = 62
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
	case SyntaxFlowParserVisitor:
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
		p.SetState(126)
		p.Statements()
	}
	{
		p.SetState(127)
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
	case SyntaxFlowParserVisitor:
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
	p.SetState(130)
	p.GetErrorHandler().Sync(p)
	_alt = 1
	for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		switch _alt {
		case 1:
			{
				p.SetState(129)
				p.Statement()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

		p.SetState(132)
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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

	p.SetState(159)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 7, p.GetParserRuleContext()) {
	case 1:
		localctx = NewCheckContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(134)
			p.CheckStatement()
		}
		p.SetState(136)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 1, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(135)
				p.Eos()
			}

		}

	case 2:
		localctx = NewDescriptionContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(138)
			p.DescriptionStatement()
		}
		p.SetState(140)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 2, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(139)
				p.Eos()
			}

		}

	case 3:
		localctx = NewAlertContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(142)
			p.AlertStatement()
		}
		p.SetState(144)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 3, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(143)
				p.Eos()
			}

		}

	case 4:
		localctx = NewFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(146)
			p.FilterStatement()
		}
		p.SetState(148)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 4, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(147)
				p.Eos()
			}

		}

	case 5:
		localctx = NewFileFilterContentContext(p, localctx)
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(150)
			p.FileFilterContentStatement()
		}
		p.SetState(152)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 5, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(151)
				p.Eos()
			}

		}

	case 6:
		localctx = NewCommandContext(p, localctx)
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(154)
			p.Comment()
		}
		p.SetState(156)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 6, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(155)
				p.Eos()
			}

		}

	case 7:
		localctx = NewEmptyContext(p, localctx)
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(158)
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

func (s *FileFilterContentStatementContext) DollarBraceOpen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDollarBraceOpen, 0)
}

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
	case SyntaxFlowParserVisitor:
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
		p.SetState(161)
		p.Match(SyntaxFlowParserDollarBraceOpen)
	}
	{
		p.SetState(162)
		p.FileFilterContentInput()
	}
	{
		p.SetState(163)
		p.Match(SyntaxFlowParserMapBuilderClose)
	}
	p.SetState(165)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(164)
			p.Lines()
		}

	}
	{
		p.SetState(167)
		p.Match(SyntaxFlowParserDot)
	}
	{
		p.SetState(168)
		p.FileFilterContentMethod()
	}
	p.SetState(171)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserAs {
		{
			p.SetState(169)
			p.Match(SyntaxFlowParserAs)
		}
		{
			p.SetState(170)
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
	case SyntaxFlowParserVisitor:
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
	p.SetState(175)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 10, p.GetParserRuleContext()) {
	case 1:
		{
			p.SetState(173)
			p.FileName()
		}

	case 2:
		{
			p.SetState(174)
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
	case SyntaxFlowParserVisitor:
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
		p.SetState(177)
		p.Match(SyntaxFlowParserIdentifier)
	}
	{
		p.SetState(178)
		p.Match(SyntaxFlowParserOpenParen)
	}
	p.SetState(180)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if (int64((_la-45)) & ^0x3f) == 0 && ((int64(1)<<(_la-45))&15290083041281) != 0 {
		{
			p.SetState(179)
			p.FileFilterContentMethodParam()
		}

	}
	{
		p.SetState(182)
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
	case SyntaxFlowParserVisitor:
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
		p.SetState(184)
		p.FileFilterContentMethodParamItem()
	}
	p.SetState(186)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 12, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(185)
			p.Lines()
		}

	}
	p.SetState(198)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 15, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(188)
				p.Match(SyntaxFlowParserComma)
			}
			p.SetState(190)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			if _la == SyntaxFlowParserBreakLine {
				{
					p.SetState(189)
					p.Lines()
				}

			}
			{
				p.SetState(192)
				p.FileFilterContentMethodParamItem()
			}
			p.SetState(194)
			p.GetErrorHandler().Sync(p)

			if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 14, p.GetParserRuleContext()) == 1 {
				{
					p.SetState(193)
					p.Lines()
				}

			}

		}
		p.SetState(200)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 15, p.GetParserRuleContext())
	}
	p.SetState(202)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserComma {
		{
			p.SetState(201)
			p.Match(SyntaxFlowParserComma)
		}

	}
	p.SetState(205)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(204)
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
	case SyntaxFlowParserVisitor:
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
	p.SetState(208)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 18, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(207)
			p.FileFilterContentMethodParamKey()
		}

	}
	{
		p.SetState(210)
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
	case SyntaxFlowParserVisitor:
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
		p.SetState(212)
		p.Match(SyntaxFlowParserIdentifier)
	}
	{
		p.SetState(213)
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
	case SyntaxFlowParserVisitor:
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
		p.SetState(215)
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
	case SyntaxFlowParserVisitor:
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
		p.SetState(217)
		p.NameFilter()
	}
	p.SetState(222)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 19, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			p.SetState(218)
			p.MatchWildcard()

			{
				p.SetState(219)
				p.NameFilter()
			}

		}
		p.SetState(224)
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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

	p.SetState(241)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDollarOutput:
		localctx = NewRefFilterExprContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(225)
			p.RefVariable()
		}
		p.SetState(229)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 20, p.GetParserRuleContext())

		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(226)
					p.FilterItem()
				}

			}
			p.SetState(231)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 20, p.GetParserRuleContext())
		}
		p.SetState(234)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserAs {
			{
				p.SetState(232)
				p.Match(SyntaxFlowParserAs)
			}
			{
				p.SetState(233)
				p.RefVariable()
			}

		}

	case SyntaxFlowParserDot, SyntaxFlowParserStartNowDoc, SyntaxFlowParserLt, SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		localctx = NewPureFilterExprContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(236)
			p.FilterExpr()
		}
		p.SetState(239)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserAs {
			{
				p.SetState(237)
				p.Match(SyntaxFlowParserAs)
			}
			{
				p.SetState(238)
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
	case SyntaxFlowParserVisitor:
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
		p.SetState(243)
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

func (s *EosContext) Semicolon() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserSemicolon, 0)
}

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
	case SyntaxFlowParserVisitor:
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

	p.SetState(247)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserSemicolon:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(245)
			p.Match(SyntaxFlowParserSemicolon)
		}

	case SyntaxFlowParserBreakLine:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(246)
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

func (s *LineContext) BreakLine() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserBreakLine, 0)
}

func (s *LineContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LineContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *LineContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
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
		p.SetState(249)
		p.Match(SyntaxFlowParserBreakLine)
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
	case SyntaxFlowParserVisitor:
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
	p.SetState(252)
	p.GetErrorHandler().Sync(p)
	_alt = 1
	for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		switch _alt {
		case 1:
			{
				p.SetState(251)
				p.Line()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

		p.SetState(254)
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
	case SyntaxFlowParserVisitor:
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

	p.SetState(267)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDesc:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(256)
			p.Match(SyntaxFlowParserDesc)
		}

		{
			p.SetState(257)
			p.Match(SyntaxFlowParserOpenParen)
		}
		p.SetState(259)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-45)) & ^0x3f) == 0 && ((int64(1)<<(_la-45))&6493990019201) != 0 {
			{
				p.SetState(258)
				p.DescriptionItems()
			}

		}
		{
			p.SetState(261)
			p.Match(SyntaxFlowParserCloseParen)
		}

	case SyntaxFlowParserMapBuilderOpen:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(262)
			p.Match(SyntaxFlowParserMapBuilderOpen)
		}
		p.SetState(264)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-45)) & ^0x3f) == 0 && ((int64(1)<<(_la-45))&6493990019201) != 0 {
			{
				p.SetState(263)
				p.DescriptionItems()
			}

		}
		{
			p.SetState(266)
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
	case SyntaxFlowParserVisitor:
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
	p.SetState(270)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(269)
			p.Lines()
		}

	}
	{
		p.SetState(272)
		p.DescriptionItem()
	}
	p.SetState(280)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 31, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(273)
				p.Match(SyntaxFlowParserComma)
			}
			p.SetState(275)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			if _la == SyntaxFlowParserBreakLine {
				{
					p.SetState(274)
					p.Lines()
				}

			}
			{
				p.SetState(277)
				p.DescriptionItem()
			}

		}
		p.SetState(282)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 31, p.GetParserRuleContext())
	}
	p.SetState(284)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserComma {
		{
			p.SetState(283)
			p.Match(SyntaxFlowParserComma)
		}

	}
	p.SetState(287)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(286)
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

func (s *DescriptionItemContext) StringLiteral() IStringLiteralContext {
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

func (s *DescriptionItemContext) DescriptionItemValue() IDescriptionItemValueContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDescriptionItemValueContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDescriptionItemValueContext)
}

func (s *DescriptionItemContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DescriptionItemContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DescriptionItemContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
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

	p.SetState(299)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 36, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(289)
			p.StringLiteral()
		}
		p.SetState(291)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 34, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(290)
				p.Lines()
			}

		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(293)
			p.StringLiteral()
		}
		{
			p.SetState(294)
			p.Match(SyntaxFlowParserColon)
		}
		{
			p.SetState(295)
			p.DescriptionItemValue()
		}
		p.SetState(297)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 35, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(296)
				p.Lines()
			}

		}

	}

	return localctx
}

// IDescriptionItemValueContext is an interface to support dynamic dispatch.
type IDescriptionItemValueContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDescriptionItemValueContext differentiates from other interfaces.
	IsDescriptionItemValueContext()
}

type DescriptionItemValueContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDescriptionItemValueContext() *DescriptionItemValueContext {
	var p = new(DescriptionItemValueContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_descriptionItemValue
	return p
}

func (*DescriptionItemValueContext) IsDescriptionItemValueContext() {}

func NewDescriptionItemValueContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DescriptionItemValueContext {
	var p = new(DescriptionItemValueContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_descriptionItemValue

	return p
}

func (s *DescriptionItemValueContext) GetParser() antlr.Parser { return s.parser }

func (s *DescriptionItemValueContext) StringLiteral() IStringLiteralContext {
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

func (s *DescriptionItemValueContext) HereDoc() IHereDocContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHereDocContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHereDocContext)
}

func (s *DescriptionItemValueContext) NumberLiteral() INumberLiteralContext {
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

func (s *DescriptionItemValueContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DescriptionItemValueContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DescriptionItemValueContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitDescriptionItemValue(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) DescriptionItemValue() (localctx IDescriptionItemValueContext) {
	this := p
	_ = this

	localctx = NewDescriptionItemValueContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 38, SyntaxFlowParserRULE_descriptionItemValue)

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

	p.SetState(304)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(301)
			p.StringLiteral()
		}

	case SyntaxFlowParserStartNowDoc:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(302)
			p.HereDoc()
		}

	case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(303)
			p.NumberLiteral()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// ICrlfHereDocContext is an interface to support dynamic dispatch.
type ICrlfHereDocContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsCrlfHereDocContext differentiates from other interfaces.
	IsCrlfHereDocContext()
}

type CrlfHereDocContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyCrlfHereDocContext() *CrlfHereDocContext {
	var p = new(CrlfHereDocContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_crlfHereDoc
	return p
}

func (*CrlfHereDocContext) IsCrlfHereDocContext() {}

func NewCrlfHereDocContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *CrlfHereDocContext {
	var p = new(CrlfHereDocContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_crlfHereDoc

	return p
}

func (s *CrlfHereDocContext) GetParser() antlr.Parser { return s.parser }

func (s *CrlfHereDocContext) CRLFHereDocIdentifierBreak() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCRLFHereDocIdentifierBreak, 0)
}

func (s *CrlfHereDocContext) CRLFEndDoc() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCRLFEndDoc, 0)
}

func (s *CrlfHereDocContext) CrlfText() ICrlfTextContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ICrlfTextContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ICrlfTextContext)
}

func (s *CrlfHereDocContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *CrlfHereDocContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *CrlfHereDocContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitCrlfHereDoc(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) CrlfHereDoc() (localctx ICrlfHereDocContext) {
	this := p
	_ = this

	localctx = NewCrlfHereDocContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 40, SyntaxFlowParserRULE_crlfHereDoc)
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
		p.SetState(306)
		p.Match(SyntaxFlowParserCRLFHereDocIdentifierBreak)
	}
	p.SetState(308)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserCRLFHereDocText {
		{
			p.SetState(307)
			p.CrlfText()
		}

	}
	{
		p.SetState(310)
		p.Match(SyntaxFlowParserCRLFEndDoc)
	}

	return localctx
}

// ILfHereDocContext is an interface to support dynamic dispatch.
type ILfHereDocContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsLfHereDocContext differentiates from other interfaces.
	IsLfHereDocContext()
}

type LfHereDocContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyLfHereDocContext() *LfHereDocContext {
	var p = new(LfHereDocContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_lfHereDoc
	return p
}

func (*LfHereDocContext) IsLfHereDocContext() {}

func NewLfHereDocContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *LfHereDocContext {
	var p = new(LfHereDocContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_lfHereDoc

	return p
}

func (s *LfHereDocContext) GetParser() antlr.Parser { return s.parser }

func (s *LfHereDocContext) LFHereDocIdentifierBreak() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserLFHereDocIdentifierBreak, 0)
}

func (s *LfHereDocContext) LFEndDoc() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserLFEndDoc, 0)
}

func (s *LfHereDocContext) LfText() ILfTextContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILfTextContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILfTextContext)
}

func (s *LfHereDocContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LfHereDocContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *LfHereDocContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitLfHereDoc(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) LfHereDoc() (localctx ILfHereDocContext) {
	this := p
	_ = this

	localctx = NewLfHereDocContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 42, SyntaxFlowParserRULE_lfHereDoc)
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
		p.SetState(312)
		p.Match(SyntaxFlowParserLFHereDocIdentifierBreak)
	}
	p.SetState(314)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserLFHereDocText {
		{
			p.SetState(313)
			p.LfText()
		}

	}
	{
		p.SetState(316)
		p.Match(SyntaxFlowParserLFEndDoc)
	}

	return localctx
}

// ICrlfTextContext is an interface to support dynamic dispatch.
type ICrlfTextContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsCrlfTextContext differentiates from other interfaces.
	IsCrlfTextContext()
}

type CrlfTextContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyCrlfTextContext() *CrlfTextContext {
	var p = new(CrlfTextContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_crlfText
	return p
}

func (*CrlfTextContext) IsCrlfTextContext() {}

func NewCrlfTextContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *CrlfTextContext {
	var p = new(CrlfTextContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_crlfText

	return p
}

func (s *CrlfTextContext) GetParser() antlr.Parser { return s.parser }

func (s *CrlfTextContext) AllCRLFHereDocText() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserCRLFHereDocText)
}

func (s *CrlfTextContext) CRLFHereDocText(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCRLFHereDocText, i)
}

func (s *CrlfTextContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *CrlfTextContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *CrlfTextContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitCrlfText(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) CrlfText() (localctx ICrlfTextContext) {
	this := p
	_ = this

	localctx = NewCrlfTextContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 44, SyntaxFlowParserRULE_crlfText)
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
	p.SetState(319)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == SyntaxFlowParserCRLFHereDocText {
		{
			p.SetState(318)
			p.Match(SyntaxFlowParserCRLFHereDocText)
		}

		p.SetState(321)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// ILfTextContext is an interface to support dynamic dispatch.
type ILfTextContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsLfTextContext differentiates from other interfaces.
	IsLfTextContext()
}

type LfTextContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyLfTextContext() *LfTextContext {
	var p = new(LfTextContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_lfText
	return p
}

func (*LfTextContext) IsLfTextContext() {}

func NewLfTextContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *LfTextContext {
	var p = new(LfTextContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_lfText

	return p
}

func (s *LfTextContext) GetParser() antlr.Parser { return s.parser }

func (s *LfTextContext) AllLFHereDocText() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserLFHereDocText)
}

func (s *LfTextContext) LFHereDocText(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserLFHereDocText, i)
}

func (s *LfTextContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LfTextContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *LfTextContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitLfText(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) LfText() (localctx ILfTextContext) {
	this := p
	_ = this

	localctx = NewLfTextContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 46, SyntaxFlowParserRULE_lfText)
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
	p.SetState(324)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == SyntaxFlowParserLFHereDocText {
		{
			p.SetState(323)
			p.Match(SyntaxFlowParserLFHereDocText)
		}

		p.SetState(326)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// IHereDocContext is an interface to support dynamic dispatch.
type IHereDocContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHereDocContext differentiates from other interfaces.
	IsHereDocContext()
}

type HereDocContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHereDocContext() *HereDocContext {
	var p = new(HereDocContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_hereDoc
	return p
}

func (*HereDocContext) IsHereDocContext() {}

func NewHereDocContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HereDocContext {
	var p = new(HereDocContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_hereDoc

	return p
}

func (s *HereDocContext) GetParser() antlr.Parser { return s.parser }

func (s *HereDocContext) StartNowDoc() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserStartNowDoc, 0)
}

func (s *HereDocContext) HereDocIdentifierName() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserHereDocIdentifierName, 0)
}

func (s *HereDocContext) CrlfHereDoc() ICrlfHereDocContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ICrlfHereDocContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ICrlfHereDocContext)
}

func (s *HereDocContext) LfHereDoc() ILfHereDocContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILfHereDocContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILfHereDocContext)
}

func (s *HereDocContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HereDocContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HereDocContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitHereDoc(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) HereDoc() (localctx IHereDocContext) {
	this := p
	_ = this

	localctx = NewHereDocContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 48, SyntaxFlowParserRULE_hereDoc)

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
		p.SetState(328)
		p.Match(SyntaxFlowParserStartNowDoc)
	}
	{
		p.SetState(329)
		p.Match(SyntaxFlowParserHereDocIdentifierName)
	}
	p.SetState(332)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserCRLFHereDocIdentifierBreak:
		{
			p.SetState(330)
			p.CrlfHereDoc()
		}

	case SyntaxFlowParserLFHereDocIdentifierBreak:
		{
			p.SetState(331)
			p.LfHereDoc()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
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
	case SyntaxFlowParserVisitor:
		return t.VisitAlertStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) AlertStatement() (localctx IAlertStatementContext) {
	this := p
	_ = this

	localctx = NewAlertStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 50, SyntaxFlowParserRULE_alertStatement)
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
		p.SetState(334)
		p.Match(SyntaxFlowParserAlert)
	}
	{
		p.SetState(335)
		p.RefVariable()
	}
	p.SetState(338)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserFor {
		{
			p.SetState(336)
			p.Match(SyntaxFlowParserFor)
		}
		{
			p.SetState(337)
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
	case SyntaxFlowParserVisitor:
		return t.VisitCheckStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) CheckStatement() (localctx ICheckStatementContext) {
	this := p
	_ = this

	localctx = NewCheckStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 52, SyntaxFlowParserRULE_checkStatement)

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
		p.SetState(340)
		p.Match(SyntaxFlowParserCheck)
	}
	{
		p.SetState(341)
		p.RefVariable()
	}
	p.SetState(343)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 44, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(342)
			p.ThenExpr()
		}

	}
	p.SetState(346)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 45, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(345)
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
	case SyntaxFlowParserVisitor:
		return t.VisitThenExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) ThenExpr() (localctx IThenExprContext) {
	this := p
	_ = this

	localctx = NewThenExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 54, SyntaxFlowParserRULE_thenExpr)

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
		p.SetState(348)
		p.Match(SyntaxFlowParserThen)
	}
	{
		p.SetState(349)
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
	case SyntaxFlowParserVisitor:
		return t.VisitElseExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) ElseExpr() (localctx IElseExprContext) {
	this := p
	_ = this

	localctx = NewElseExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 56, SyntaxFlowParserRULE_elseExpr)

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
		p.SetState(351)
		p.Match(SyntaxFlowParserElse)
	}
	{
		p.SetState(352)
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
	case SyntaxFlowParserVisitor:
		return t.VisitRefVariable(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) RefVariable() (localctx IRefVariableContext) {
	this := p
	_ = this

	localctx = NewRefVariableContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 58, SyntaxFlowParserRULE_refVariable)

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
		p.SetState(354)
		p.Match(SyntaxFlowParserDollarOutput)
	}
	p.SetState(360)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		{
			p.SetState(355)
			p.Identifier()
		}

	case SyntaxFlowParserOpenParen:
		{
			p.SetState(356)
			p.Match(SyntaxFlowParserOpenParen)
		}
		{
			p.SetState(357)
			p.Identifier()
		}
		{
			p.SetState(358)
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
		return t.VisitNamedFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type ConstFilterContext struct {
	*FilterItemFirstContext
}

func NewConstFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ConstFilterContext {
	var p = new(ConstFilterContext)

	p.FilterItemFirstContext = NewEmptyFilterItemFirstContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemFirstContext))

	return p
}

func (s *ConstFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ConstFilterContext) QuotedStringLiteral() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserQuotedStringLiteral, 0)
}

func (s *ConstFilterContext) HereDoc() IHereDocContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHereDocContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHereDocContext)
}

func (s *ConstFilterContext) ConstSearchPrefix() IConstSearchPrefixContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConstSearchPrefixContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConstSearchPrefixContext)
}

func (s *ConstFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitConstFilter(s)

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
	case SyntaxFlowParserVisitor:
		return t.VisitNativeCallFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FilterItemFirst() (localctx IFilterItemFirstContext) {
	this := p
	_ = this

	localctx = NewFilterItemFirstContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 60, SyntaxFlowParserRULE_filterItemFirst)
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

	p.SetState(376)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 50, p.GetParserRuleContext()) {
	case 1:
		localctx = NewConstFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		p.SetState(363)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-82)) & ^0x3f) == 0 && ((int64(1)<<(_la-82))&7) != 0 {
			{
				p.SetState(362)
				p.ConstSearchPrefix()
			}

		}
		p.SetState(367)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserQuotedStringLiteral:
			{
				p.SetState(365)
				p.Match(SyntaxFlowParserQuotedStringLiteral)
			}

		case SyntaxFlowParserStartNowDoc:
			{
				p.SetState(366)
				p.HereDoc()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

	case 2:
		localctx = NewNamedFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(369)
			p.NameFilter()
		}

	case 3:
		localctx = NewFieldCallFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(370)
			p.Match(SyntaxFlowParserDot)
		}
		p.SetState(372)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserBreakLine {
			{
				p.SetState(371)
				p.Lines()
			}

		}
		{
			p.SetState(374)
			p.NameFilter()
		}

	case 4:
		localctx = NewNativeCallFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(375)
			p.NativeCall()
		}

	}

	return localctx
}

// IConstSearchPrefixContext is an interface to support dynamic dispatch.
type IConstSearchPrefixContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsConstSearchPrefixContext differentiates from other interfaces.
	IsConstSearchPrefixContext()
}

type ConstSearchPrefixContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyConstSearchPrefixContext() *ConstSearchPrefixContext {
	var p = new(ConstSearchPrefixContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_constSearchPrefix
	return p
}

func (*ConstSearchPrefixContext) IsConstSearchPrefixContext() {}

func NewConstSearchPrefixContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ConstSearchPrefixContext {
	var p = new(ConstSearchPrefixContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_constSearchPrefix

	return p
}

func (s *ConstSearchPrefixContext) GetParser() antlr.Parser { return s.parser }

func (s *ConstSearchPrefixContext) ConstSearchModePrefixRegexp() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserConstSearchModePrefixRegexp, 0)
}

func (s *ConstSearchPrefixContext) ConstSearchModePrefixGlob() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserConstSearchModePrefixGlob, 0)
}

func (s *ConstSearchPrefixContext) ConstSearchModePrefixExact() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserConstSearchModePrefixExact, 0)
}

func (s *ConstSearchPrefixContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ConstSearchPrefixContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ConstSearchPrefixContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitConstSearchPrefix(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) ConstSearchPrefix() (localctx IConstSearchPrefixContext) {
	this := p
	_ = this

	localctx = NewConstSearchPrefixContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 62, SyntaxFlowParserRULE_constSearchPrefix)
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
		p.SetState(378)
		_la = p.GetTokenStream().LA(1)

		if !((int64((_la-82)) & ^0x3f) == 0 && ((int64(1)<<(_la-82))&7) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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

func (s *NextFilterContext) UseStart() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserUseStart, 0)
}

func (s *NextFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
		return t.VisitOptionalFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type IntersectionRefFilterContext struct {
	*FilterItemContext
}

func NewIntersectionRefFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *IntersectionRefFilterContext {
	var p = new(IntersectionRefFilterContext)

	p.FilterItemContext = NewEmptyFilterItemContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterItemContext))

	return p
}

func (s *IntersectionRefFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *IntersectionRefFilterContext) Amp() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserAmp, 0)
}

func (s *IntersectionRefFilterContext) RefVariable() IRefVariableContext {
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

func (s *IntersectionRefFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitIntersectionRefFilter(s)

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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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

func (s *MergeRefFilterContext) Add() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserAdd, 0)
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
	case SyntaxFlowParserVisitor:
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

func (s *DeepNextFilterContext) DeepNext() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDeepNext, 0)
}

func (s *DeepNextFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
		return t.VisitFirst(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FilterItem() (localctx IFilterItemContext) {
	this := p
	_ = this

	localctx = NewFilterItemContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 64, SyntaxFlowParserRULE_filterItem)
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

	p.SetState(422)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDot, SyntaxFlowParserStartNowDoc, SyntaxFlowParserLt, SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		localctx = NewFirstContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(380)
			p.FilterItemFirst()
		}

	case SyntaxFlowParserDeep:
		localctx = NewDeepChainFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(381)
			p.Match(SyntaxFlowParserDeep)
		}
		p.SetState(383)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserBreakLine {
			{
				p.SetState(382)
				p.Lines()
			}

		}
		{
			p.SetState(385)
			p.NameFilter()
		}

	case SyntaxFlowParserOpenParen:
		localctx = NewFunctionCallFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(386)
			p.Match(SyntaxFlowParserOpenParen)
		}
		p.SetState(388)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserBreakLine {
			{
				p.SetState(387)
				p.Lines()
			}

		}
		p.SetState(391)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-288192975094153216) != 0 || (int64((_la-65)) & ^0x3f) == 0 && ((int64(1)<<(_la-65))&14581759) != 0 {
			{
				p.SetState(390)
				p.ActualParam()
			}

		}
		{
			p.SetState(393)
			p.Match(SyntaxFlowParserCloseParen)
		}

	case SyntaxFlowParserListSelectOpen:
		localctx = NewFieldIndexFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(394)
			p.Match(SyntaxFlowParserListSelectOpen)
		}
		{
			p.SetState(395)
			p.SliceCallItem()
		}
		{
			p.SetState(396)
			p.Match(SyntaxFlowParserListSelectClose)
		}

	case SyntaxFlowParserConditionStart:
		localctx = NewOptionalFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(398)
			p.Match(SyntaxFlowParserConditionStart)
		}
		{
			p.SetState(399)
			p.conditionExpression(0)
		}
		{
			p.SetState(400)
			p.Match(SyntaxFlowParserMapBuilderClose)
		}

	case SyntaxFlowParserUseStart:
		localctx = NewNextFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(402)
			p.Match(SyntaxFlowParserUseStart)
		}

	case SyntaxFlowParserDefStart:
		localctx = NewDefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(403)
			p.Match(SyntaxFlowParserDefStart)
		}

	case SyntaxFlowParserDeepNext:
		localctx = NewDeepNextFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(404)
			p.Match(SyntaxFlowParserDeepNext)
		}

	case SyntaxFlowParserDeepNextStart:
		localctx = NewDeepNextConfigFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(405)
			p.Match(SyntaxFlowParserDeepNextStart)
		}
		p.SetState(407)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-52)) & ^0x3f) == 0 && ((int64(1)<<(_la-52))&50734297025) != 0 {
			{
				p.SetState(406)
				p.Config()
			}

		}
		{
			p.SetState(409)
			p.Match(SyntaxFlowParserDeepNextEnd)
		}

	case SyntaxFlowParserTopDef:
		localctx = NewTopDefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 10)
		{
			p.SetState(410)
			p.Match(SyntaxFlowParserTopDef)
		}

	case SyntaxFlowParserTopDefStart:
		localctx = NewTopDefConfigFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 11)
		{
			p.SetState(411)
			p.Match(SyntaxFlowParserTopDefStart)
		}
		p.SetState(413)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-52)) & ^0x3f) == 0 && ((int64(1)<<(_la-52))&50734297025) != 0 {
			{
				p.SetState(412)
				p.Config()
			}

		}
		{
			p.SetState(415)
			p.Match(SyntaxFlowParserDeepNextEnd)
		}

	case SyntaxFlowParserAdd:
		localctx = NewMergeRefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 12)
		{
			p.SetState(416)
			p.Match(SyntaxFlowParserAdd)
		}
		{
			p.SetState(417)
			p.RefVariable()
		}

	case SyntaxFlowParserMinus:
		localctx = NewRemoveRefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 13)
		{
			p.SetState(418)
			p.Match(SyntaxFlowParserMinus)
		}
		{
			p.SetState(419)
			p.RefVariable()
		}

	case SyntaxFlowParserAmp:
		localctx = NewIntersectionRefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 14)
		{
			p.SetState(420)
			p.Match(SyntaxFlowParserAmp)
		}
		{
			p.SetState(421)
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
	case SyntaxFlowParserVisitor:
		return t.VisitFilterExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FilterExpr() (localctx IFilterExprContext) {
	this := p
	_ = this

	localctx = NewFilterExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 66, SyntaxFlowParserRULE_filterExpr)

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
		p.SetState(424)
		p.FilterItemFirst()
	}
	p.SetState(428)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 57, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(425)
				p.FilterItem()
			}

		}
		p.SetState(430)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 57, p.GetParserRuleContext())
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
	case SyntaxFlowParserVisitor:
		return t.VisitNativeCall(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) NativeCall() (localctx INativeCallContext) {
	this := p
	_ = this

	localctx = NewNativeCallContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 68, SyntaxFlowParserRULE_nativeCall)

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
		p.SetState(431)
		p.Match(SyntaxFlowParserLt)
	}
	{
		p.SetState(432)
		p.UseNativeCall()
	}
	{
		p.SetState(433)
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
	case SyntaxFlowParserVisitor:
		return t.VisitUseNativeCall(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) UseNativeCall() (localctx IUseNativeCallContext) {
	this := p
	_ = this

	localctx = NewUseNativeCallContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 70, SyntaxFlowParserRULE_useNativeCall)
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
		p.SetState(435)
		p.Identifier()
	}
	p.SetState(437)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserOpenParen || _la == SyntaxFlowParserMapBuilderOpen {
		{
			p.SetState(436)
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

func (s *UseDefCalcParamsContext) NativeCallActualParams() INativeCallActualParamsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INativeCallActualParamsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INativeCallActualParamsContext)
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
	case SyntaxFlowParserVisitor:
		return t.VisitUseDefCalcParams(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) UseDefCalcParams() (localctx IUseDefCalcParamsContext) {
	this := p
	_ = this

	localctx = NewUseDefCalcParamsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 72, SyntaxFlowParserRULE_useDefCalcParams)
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

	p.SetState(449)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserMapBuilderOpen:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(439)
			p.Match(SyntaxFlowParserMapBuilderOpen)
		}
		p.SetState(441)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-27)) & ^0x3f) == 0 && ((int64(1)<<(_la-27))&1702360521608544257) != 0 {
			{
				p.SetState(440)
				p.NativeCallActualParams()
			}

		}
		{
			p.SetState(443)
			p.Match(SyntaxFlowParserMapBuilderClose)
		}

	case SyntaxFlowParserOpenParen:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(444)
			p.Match(SyntaxFlowParserOpenParen)
		}
		p.SetState(446)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-27)) & ^0x3f) == 0 && ((int64(1)<<(_la-27))&1702360521608544257) != 0 {
			{
				p.SetState(445)
				p.NativeCallActualParams()
			}

		}
		{
			p.SetState(448)
			p.Match(SyntaxFlowParserCloseParen)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// INativeCallActualParamsContext is an interface to support dynamic dispatch.
type INativeCallActualParamsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNativeCallActualParamsContext differentiates from other interfaces.
	IsNativeCallActualParamsContext()
}

type NativeCallActualParamsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNativeCallActualParamsContext() *NativeCallActualParamsContext {
	var p = new(NativeCallActualParamsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_nativeCallActualParams
	return p
}

func (*NativeCallActualParamsContext) IsNativeCallActualParamsContext() {}

func NewNativeCallActualParamsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NativeCallActualParamsContext {
	var p = new(NativeCallActualParamsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_nativeCallActualParams

	return p
}

func (s *NativeCallActualParamsContext) GetParser() antlr.Parser { return s.parser }

func (s *NativeCallActualParamsContext) AllNativeCallActualParam() []INativeCallActualParamContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(INativeCallActualParamContext); ok {
			len++
		}
	}

	tst := make([]INativeCallActualParamContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(INativeCallActualParamContext); ok {
			tst[i] = t.(INativeCallActualParamContext)
			i++
		}
	}

	return tst
}

func (s *NativeCallActualParamsContext) NativeCallActualParam(i int) INativeCallActualParamContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INativeCallActualParamContext); ok {
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

	return t.(INativeCallActualParamContext)
}

func (s *NativeCallActualParamsContext) AllLines() []ILinesContext {
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

func (s *NativeCallActualParamsContext) Lines(i int) ILinesContext {
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

func (s *NativeCallActualParamsContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserComma)
}

func (s *NativeCallActualParamsContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserComma, i)
}

func (s *NativeCallActualParamsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NativeCallActualParamsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NativeCallActualParamsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitNativeCallActualParams(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) NativeCallActualParams() (localctx INativeCallActualParamsContext) {
	this := p
	_ = this

	localctx = NewNativeCallActualParamsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 74, SyntaxFlowParserRULE_nativeCallActualParams)
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
	p.SetState(452)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(451)
			p.Lines()
		}

	}
	{
		p.SetState(454)
		p.NativeCallActualParam()
	}
	p.SetState(462)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 64, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(455)
				p.Match(SyntaxFlowParserComma)
			}
			p.SetState(457)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			if _la == SyntaxFlowParserBreakLine {
				{
					p.SetState(456)
					p.Lines()
				}

			}
			{
				p.SetState(459)
				p.NativeCallActualParam()
			}

		}
		p.SetState(464)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 64, p.GetParserRuleContext())
	}
	p.SetState(466)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserComma {
		{
			p.SetState(465)
			p.Match(SyntaxFlowParserComma)
		}

	}
	p.SetState(469)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(468)
			p.Lines()
		}

	}

	return localctx
}

// INativeCallActualParamContext is an interface to support dynamic dispatch.
type INativeCallActualParamContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNativeCallActualParamContext differentiates from other interfaces.
	IsNativeCallActualParamContext()
}

type NativeCallActualParamContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNativeCallActualParamContext() *NativeCallActualParamContext {
	var p = new(NativeCallActualParamContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_nativeCallActualParam
	return p
}

func (*NativeCallActualParamContext) IsNativeCallActualParamContext() {}

func NewNativeCallActualParamContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NativeCallActualParamContext {
	var p = new(NativeCallActualParamContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_nativeCallActualParam

	return p
}

func (s *NativeCallActualParamContext) GetParser() antlr.Parser { return s.parser }

func (s *NativeCallActualParamContext) NativeCallActualParamValue() INativeCallActualParamValueContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INativeCallActualParamValueContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INativeCallActualParamValueContext)
}

func (s *NativeCallActualParamContext) NativeCallActualParamKey() INativeCallActualParamKeyContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INativeCallActualParamKeyContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INativeCallActualParamKeyContext)
}

func (s *NativeCallActualParamContext) Colon() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserColon, 0)
}

func (s *NativeCallActualParamContext) Eq() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserEq, 0)
}

func (s *NativeCallActualParamContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NativeCallActualParamContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NativeCallActualParamContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitNativeCallActualParam(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) NativeCallActualParam() (localctx INativeCallActualParamContext) {
	this := p
	_ = this

	localctx = NewNativeCallActualParamContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 76, SyntaxFlowParserRULE_nativeCallActualParam)
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
	p.SetState(474)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 67, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(471)
			p.NativeCallActualParamKey()
		}
		{
			p.SetState(472)
			_la = p.GetTokenStream().LA(1)

			if !(_la == SyntaxFlowParserEq || _la == SyntaxFlowParserColon) {
				p.GetErrorHandler().RecoverInline(p)
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}

	}
	{
		p.SetState(476)
		p.NativeCallActualParamValue()
	}

	return localctx
}

// INativeCallActualParamKeyContext is an interface to support dynamic dispatch.
type INativeCallActualParamKeyContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNativeCallActualParamKeyContext differentiates from other interfaces.
	IsNativeCallActualParamKeyContext()
}

type NativeCallActualParamKeyContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNativeCallActualParamKeyContext() *NativeCallActualParamKeyContext {
	var p = new(NativeCallActualParamKeyContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_nativeCallActualParamKey
	return p
}

func (*NativeCallActualParamKeyContext) IsNativeCallActualParamKeyContext() {}

func NewNativeCallActualParamKeyContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NativeCallActualParamKeyContext {
	var p = new(NativeCallActualParamKeyContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_nativeCallActualParamKey

	return p
}

func (s *NativeCallActualParamKeyContext) GetParser() antlr.Parser { return s.parser }

func (s *NativeCallActualParamKeyContext) Identifier() IIdentifierContext {
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

func (s *NativeCallActualParamKeyContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NativeCallActualParamKeyContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NativeCallActualParamKeyContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitNativeCallActualParamKey(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) NativeCallActualParamKey() (localctx INativeCallActualParamKeyContext) {
	this := p
	_ = this

	localctx = NewNativeCallActualParamKeyContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 78, SyntaxFlowParserRULE_nativeCallActualParamKey)

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
		p.SetState(478)
		p.Identifier()
	}

	return localctx
}

// INativeCallActualParamValueContext is an interface to support dynamic dispatch.
type INativeCallActualParamValueContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNativeCallActualParamValueContext differentiates from other interfaces.
	IsNativeCallActualParamValueContext()
}

type NativeCallActualParamValueContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNativeCallActualParamValueContext() *NativeCallActualParamValueContext {
	var p = new(NativeCallActualParamValueContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_nativeCallActualParamValue
	return p
}

func (*NativeCallActualParamValueContext) IsNativeCallActualParamValueContext() {}

func NewNativeCallActualParamValueContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NativeCallActualParamValueContext {
	var p = new(NativeCallActualParamValueContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_nativeCallActualParamValue

	return p
}

func (s *NativeCallActualParamValueContext) GetParser() antlr.Parser { return s.parser }

func (s *NativeCallActualParamValueContext) Identifier() IIdentifierContext {
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

func (s *NativeCallActualParamValueContext) NumberLiteral() INumberLiteralContext {
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

func (s *NativeCallActualParamValueContext) AllBacktick() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserBacktick)
}

func (s *NativeCallActualParamValueContext) Backtick(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserBacktick, i)
}

func (s *NativeCallActualParamValueContext) DollarOutput() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDollarOutput, 0)
}

func (s *NativeCallActualParamValueContext) HereDoc() IHereDocContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHereDocContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHereDocContext)
}

func (s *NativeCallActualParamValueContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NativeCallActualParamValueContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NativeCallActualParamValueContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitNativeCallActualParamValue(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) NativeCallActualParamValue() (localctx INativeCallActualParamValueContext) {
	this := p
	_ = this

	localctx = NewNativeCallActualParamValueContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 80, SyntaxFlowParserRULE_nativeCallActualParamValue)
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

	p.SetState(493)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(480)
			p.Identifier()
		}

	case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(481)
			p.NumberLiteral()
		}

	case SyntaxFlowParserBacktick:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(482)
			p.Match(SyntaxFlowParserBacktick)
		}
		p.SetState(486)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-281474976710658) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&8589934591) != 0 {
			{
				p.SetState(483)
				_la = p.GetTokenStream().LA(1)

				if _la <= 0 || _la == SyntaxFlowParserBacktick {
					p.GetErrorHandler().RecoverInline(p)
				} else {
					p.GetErrorHandler().ReportMatch(p)
					p.Consume()
				}
			}

			p.SetState(488)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(489)
			p.Match(SyntaxFlowParserBacktick)
		}

	case SyntaxFlowParserDollarOutput:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(490)
			p.Match(SyntaxFlowParserDollarOutput)
		}
		{
			p.SetState(491)
			p.Identifier()
		}

	case SyntaxFlowParserStartNowDoc:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(492)
			p.HereDoc()
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
		return t.VisitEveryParam(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) ActualParam() (localctx IActualParamContext) {
	this := p
	_ = this

	localctx = NewActualParamContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 82, SyntaxFlowParserRULE_actualParam)
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

	p.SetState(510)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 74, p.GetParserRuleContext()) {
	case 1:
		localctx = NewAllParamContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(495)
			p.SingleParam()
		}
		p.SetState(497)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserBreakLine {
			{
				p.SetState(496)
				p.Lines()
			}

		}

	case 2:
		localctx = NewEveryParamContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		p.SetState(500)
		p.GetErrorHandler().Sync(p)
		_alt = 1
		for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			switch _alt {
			case 1:
				{
					p.SetState(499)
					p.ActualParamFilter()
				}

			default:
				panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
			}

			p.SetState(502)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 71, p.GetParserRuleContext())
		}
		p.SetState(505)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-288192992274022400) != 0 || (int64((_la-65)) & ^0x3f) == 0 && ((int64(1)<<(_la-65))&14581759) != 0 {
			{
				p.SetState(504)
				p.SingleParam()
			}

		}
		p.SetState(508)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserBreakLine {
			{
				p.SetState(507)
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
	case SyntaxFlowParserVisitor:
		return t.VisitActualParamFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) ActualParamFilter() (localctx IActualParamFilterContext) {
	this := p
	_ = this

	localctx = NewActualParamFilterContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 84, SyntaxFlowParserRULE_actualParamFilter)

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

	p.SetState(516)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserTopDefStart, SyntaxFlowParserDefStart, SyntaxFlowParserDot, SyntaxFlowParserStartNowDoc, SyntaxFlowParserLt, SyntaxFlowParserDollarOutput, SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(512)
			p.SingleParam()
		}
		{
			p.SetState(513)
			p.Match(SyntaxFlowParserComma)
		}

	case SyntaxFlowParserComma:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(515)
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
	case SyntaxFlowParserVisitor:
		return t.VisitSingleParam(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) SingleParam() (localctx ISingleParamContext) {
	this := p
	_ = this

	localctx = NewSingleParamContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 86, SyntaxFlowParserRULE_singleParam)
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
	p.SetState(524)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDefStart:
		{
			p.SetState(518)
			p.Match(SyntaxFlowParserDefStart)
		}

	case SyntaxFlowParserTopDefStart:
		{
			p.SetState(519)
			p.Match(SyntaxFlowParserTopDefStart)
		}
		p.SetState(521)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-52)) & ^0x3f) == 0 && ((int64(1)<<(_la-52))&50734297025) != 0 {
			{
				p.SetState(520)
				p.Config()
			}

		}
		{
			p.SetState(523)
			p.Match(SyntaxFlowParserMapBuilderClose)
		}

	case SyntaxFlowParserDot, SyntaxFlowParserStartNowDoc, SyntaxFlowParserLt, SyntaxFlowParserDollarOutput, SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:

	default:
	}
	{
		p.SetState(526)
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

func (s *ConfigContext) Lines() ILinesContext {
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

func (s *ConfigContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ConfigContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ConfigContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitConfig(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Config() (localctx IConfigContext) {
	this := p
	_ = this

	localctx = NewConfigContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 88, SyntaxFlowParserRULE_config)
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
		p.SetState(528)
		p.RecursiveConfigItem()
	}
	p.SetState(533)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 78, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(529)
				p.Match(SyntaxFlowParserComma)
			}
			{
				p.SetState(530)
				p.RecursiveConfigItem()
			}

		}
		p.SetState(535)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 78, p.GetParserRuleContext())
	}
	p.SetState(537)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserComma {
		{
			p.SetState(536)
			p.Match(SyntaxFlowParserComma)
		}

	}
	p.SetState(540)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(539)
			p.Lines()
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

func (s *RecursiveConfigItemContext) AllLines() []ILinesContext {
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

func (s *RecursiveConfigItemContext) Lines(i int) ILinesContext {
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

func (s *RecursiveConfigItemContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RecursiveConfigItemContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *RecursiveConfigItemContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitRecursiveConfigItem(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) RecursiveConfigItem() (localctx IRecursiveConfigItemContext) {
	this := p
	_ = this

	localctx = NewRecursiveConfigItemContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 90, SyntaxFlowParserRULE_recursiveConfigItem)
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
	p.SetState(543)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(542)
			p.Lines()
		}

	}
	{
		p.SetState(545)
		p.Identifier()
	}
	{
		p.SetState(546)
		p.Match(SyntaxFlowParserColon)
	}
	{
		p.SetState(547)
		p.RecursiveConfigItemValue()
	}
	p.SetState(549)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 82, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(548)
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
	case SyntaxFlowParserVisitor:
		return t.VisitRecursiveConfigItemValue(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) RecursiveConfigItemValue() (localctx IRecursiveConfigItemValueContext) {
	this := p
	_ = this

	localctx = NewRecursiveConfigItemValueContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 92, SyntaxFlowParserRULE_recursiveConfigItemValue)

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

	p.SetState(559)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		p.SetState(553)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
			{
				p.SetState(551)
				p.Identifier()
			}

		case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
			{
				p.SetState(552)
				p.NumberLiteral()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

	case SyntaxFlowParserBacktick:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(555)
			p.Match(SyntaxFlowParserBacktick)
		}
		{
			p.SetState(556)
			p.FilterStatement()
		}
		{
			p.SetState(557)
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
	case SyntaxFlowParserVisitor:
		return t.VisitSliceCallItem(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) SliceCallItem() (localctx ISliceCallItemContext) {
	this := p
	_ = this

	localctx = NewSliceCallItemContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 94, SyntaxFlowParserRULE_sliceCallItem)

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

	p.SetState(563)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(561)
			p.NameFilter()
		}

	case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(562)
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
	case SyntaxFlowParserVisitor:
		return t.VisitNameFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) NameFilter() (localctx INameFilterContext) {
	this := p
	_ = this

	localctx = NewNameFilterContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 96, SyntaxFlowParserRULE_nameFilter)

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

	p.SetState(568)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStar:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(565)
			p.Match(SyntaxFlowParserStar)
		}

	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(566)
			p.Identifier()
		}

	case SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(567)
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
	case SyntaxFlowParserVisitor:
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

func (s *BuildMapContext) AllSemicolon() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserSemicolon)
}

func (s *BuildMapContext) Semicolon(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserSemicolon, i)
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
	case SyntaxFlowParserVisitor:
		return t.VisitBuildMap(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) ChainFilter() (localctx IChainFilterContext) {
	this := p
	_ = this

	localctx = NewChainFilterContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 98, SyntaxFlowParserRULE_chainFilter)
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

	p.SetState(605)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserListSelectOpen:
		localctx = NewFlatContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(570)
			p.Match(SyntaxFlowParserListSelectOpen)
		}
		p.SetState(580)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserDollarBraceOpen, SyntaxFlowParserSemicolon, SyntaxFlowParserDot, SyntaxFlowParserStartNowDoc, SyntaxFlowParserLt, SyntaxFlowParserMapBuilderOpen, SyntaxFlowParserDollarOutput, SyntaxFlowParserStar, SyntaxFlowParserLineComment, SyntaxFlowParserBreakLine, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserAlert, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
			{
				p.SetState(571)
				p.Statements()
			}
			p.SetState(576)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			for _la == SyntaxFlowParserComma {
				{
					p.SetState(572)
					p.Match(SyntaxFlowParserComma)
				}
				{
					p.SetState(573)
					p.Statements()
				}

				p.SetState(578)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)
			}

		case SyntaxFlowParserDeep:
			{
				p.SetState(579)
				p.Match(SyntaxFlowParserDeep)
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}
		{
			p.SetState(582)
			p.Match(SyntaxFlowParserListSelectClose)
		}

	case SyntaxFlowParserMapBuilderOpen:
		localctx = NewBuildMapContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(583)
			p.Match(SyntaxFlowParserMapBuilderOpen)
		}
		p.SetState(599)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-58)) & ^0x3f) == 0 && ((int64(1)<<(_la-58))&792723391) != 0 {
			{
				p.SetState(584)
				p.Identifier()
			}
			{
				p.SetState(585)
				p.Match(SyntaxFlowParserColon)
			}

			{
				p.SetState(587)
				p.Statements()
			}
			p.SetState(596)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 89, p.GetParserRuleContext())

			for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
				if _alt == 1 {
					{
						p.SetState(588)
						p.Match(SyntaxFlowParserSemicolon)
					}

					{
						p.SetState(589)
						p.Identifier()
					}
					{
						p.SetState(590)
						p.Match(SyntaxFlowParserColon)
					}

					{
						p.SetState(592)
						p.Statements()
					}

				}
				p.SetState(598)
				p.GetErrorHandler().Sync(p)
				_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 89, p.GetParserRuleContext())
			}

		}
		p.SetState(602)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserSemicolon {
			{
				p.SetState(601)
				p.Match(SyntaxFlowParserSemicolon)
			}

		}
		{
			p.SetState(604)
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
	case SyntaxFlowParserVisitor:
		return t.VisitStringLiteralWithoutStarGroup(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) StringLiteralWithoutStarGroup() (localctx IStringLiteralWithoutStarGroupContext) {
	this := p
	_ = this

	localctx = NewStringLiteralWithoutStarGroupContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 100, SyntaxFlowParserRULE_stringLiteralWithoutStarGroup)

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
		p.SetState(607)
		p.StringLiteralWithoutStar()
	}
	p.SetState(612)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 93, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(608)
				p.Match(SyntaxFlowParserComma)
			}
			{
				p.SetState(609)
				p.StringLiteralWithoutStar()
			}

		}
		p.SetState(614)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 93, p.GetParserRuleContext())
	}
	p.SetState(616)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 94, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(615)
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
	case SyntaxFlowParserVisitor:
		return t.VisitNegativeCondition(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) NegativeCondition() (localctx INegativeConditionContext) {
	this := p
	_ = this

	localctx = NewNegativeConditionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 102, SyntaxFlowParserRULE_negativeCondition)
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
		p.SetState(618)
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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

func (s *OpcodeTypeConditionContext) AllOpcodesCondition() []IOpcodesConditionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IOpcodesConditionContext); ok {
			len++
		}
	}

	tst := make([]IOpcodesConditionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IOpcodesConditionContext); ok {
			tst[i] = t.(IOpcodesConditionContext)
			i++
		}
	}

	return tst
}

func (s *OpcodeTypeConditionContext) OpcodesCondition(i int) IOpcodesConditionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOpcodesConditionContext); ok {
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

	return t.(IOpcodesConditionContext)
}

func (s *OpcodeTypeConditionContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserComma)
}

func (s *OpcodeTypeConditionContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserComma, i)
}

func (s *OpcodeTypeConditionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	case SyntaxFlowParserVisitor:
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
	_startState := 104
	p.EnterRecursionRule(localctx, 104, SyntaxFlowParserRULE_conditionExpression, _p)
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
	p.SetState(659)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 99, p.GetParserRuleContext()) {
	case 1:
		localctx = NewParenConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx

		{
			p.SetState(621)
			p.Match(SyntaxFlowParserOpenParen)
		}
		{
			p.SetState(622)
			p.conditionExpression(0)
		}
		{
			p.SetState(623)
			p.Match(SyntaxFlowParserCloseParen)
		}

	case 2:
		localctx = NewFilterConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(625)
			p.FilterExpr()
		}

	case 3:
		localctx = NewOpcodeTypeConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(626)
			p.Match(SyntaxFlowParserOpcode)
		}
		{
			p.SetState(627)
			p.Match(SyntaxFlowParserColon)
		}
		{
			p.SetState(628)
			p.OpcodesCondition()
		}
		p.SetState(633)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 95, p.GetParserRuleContext())

		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(629)
					p.Match(SyntaxFlowParserComma)
				}
				{
					p.SetState(630)
					p.OpcodesCondition()
				}

			}
			p.SetState(635)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 95, p.GetParserRuleContext())
		}
		p.SetState(637)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 96, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(636)
				p.Match(SyntaxFlowParserComma)
			}

		}

	case 4:
		localctx = NewStringContainHaveConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(639)
			p.Match(SyntaxFlowParserHave)
		}
		{
			p.SetState(640)
			p.Match(SyntaxFlowParserColon)
		}
		{
			p.SetState(641)
			p.StringLiteralWithoutStarGroup()
		}

	case 5:
		localctx = NewStringContainAnyConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(642)
			p.Match(SyntaxFlowParserHaveAny)
		}
		{
			p.SetState(643)
			p.Match(SyntaxFlowParserColon)
		}
		{
			p.SetState(644)
			p.StringLiteralWithoutStarGroup()
		}

	case 6:
		localctx = NewNotConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(645)
			p.NegativeCondition()
		}
		{
			p.SetState(646)
			p.conditionExpression(5)
		}

	case 7:
		localctx = NewFilterExpressionCompareContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(648)

			var _lt = p.GetTokenStream().LT(1)

			localctx.(*FilterExpressionCompareContext).op = _lt

			_la = p.GetTokenStream().LA(1)

			if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&838877792) != 0) {
				var _ri = p.GetErrorHandler().RecoverInline(p)

				localctx.(*FilterExpressionCompareContext).op = _ri
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}
		p.SetState(652)
		p.GetErrorHandler().Sync(p)
		switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 97, p.GetParserRuleContext()) {
		case 1:
			{
				p.SetState(649)
				p.NumberLiteral()
			}

		case 2:
			{
				p.SetState(650)
				p.Identifier()
			}

		case 3:
			{
				p.SetState(651)
				p.BoolLiteral()
			}

		}

	case 8:
		localctx = NewFilterExpressionRegexpMatchContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(654)

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
		p.SetState(657)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
			{
				p.SetState(655)
				p.StringLiteral()
			}

		case SyntaxFlowParserRegexpLiteral:
			{
				p.SetState(656)
				p.RegexpLiteral()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

	}
	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(669)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 101, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			p.SetState(667)
			p.GetErrorHandler().Sync(p)
			switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 100, p.GetParserRuleContext()) {
			case 1:
				localctx = NewFilterExpressionAndContext(p, NewConditionExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_conditionExpression)
				p.SetState(661)

				if !(p.Precpred(p.GetParserRuleContext(), 2)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 2)", ""))
				}
				{
					p.SetState(662)
					p.Match(SyntaxFlowParserAnd)
				}
				{
					p.SetState(663)
					p.conditionExpression(3)
				}

			case 2:
				localctx = NewFilterExpressionOrContext(p, NewConditionExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_conditionExpression)
				p.SetState(664)

				if !(p.Precpred(p.GetParserRuleContext(), 1)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 1)", ""))
				}
				{
					p.SetState(665)
					p.Match(SyntaxFlowParserOr)
				}
				{
					p.SetState(666)
					p.conditionExpression(2)
				}

			}

		}
		p.SetState(671)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 101, p.GetParserRuleContext())
	}

	return localctx
}

// IOpcodesConditionContext is an interface to support dynamic dispatch.
type IOpcodesConditionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsOpcodesConditionContext differentiates from other interfaces.
	IsOpcodesConditionContext()
}

type OpcodesConditionContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOpcodesConditionContext() *OpcodesConditionContext {
	var p = new(OpcodesConditionContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_opcodesCondition
	return p
}

func (*OpcodesConditionContext) IsOpcodesConditionContext() {}

func NewOpcodesConditionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OpcodesConditionContext {
	var p = new(OpcodesConditionContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_opcodesCondition

	return p
}

func (s *OpcodesConditionContext) GetParser() antlr.Parser { return s.parser }

func (s *OpcodesConditionContext) Opcodes() IOpcodesContext {
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

func (s *OpcodesConditionContext) Identifier() IIdentifierContext {
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

func (s *OpcodesConditionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OpcodesConditionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OpcodesConditionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitOpcodesCondition(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) OpcodesCondition() (localctx IOpcodesConditionContext) {
	this := p
	_ = this

	localctx = NewOpcodesConditionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 106, SyntaxFlowParserRULE_opcodesCondition)

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

	p.SetState(674)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 102, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(672)
			p.Opcodes()
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(673)
			p.Identifier()
		}

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
	case SyntaxFlowParserVisitor:
		return t.VisitNumberLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) NumberLiteral() (localctx INumberLiteralContext) {
	this := p
	_ = this

	localctx = NewNumberLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 108, SyntaxFlowParserRULE_numberLiteral)
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
		p.SetState(676)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&270215977642229760) != 0) {
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
	case SyntaxFlowParserVisitor:
		return t.VisitStringLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) StringLiteral() (localctx IStringLiteralContext) {
	this := p
	_ = this

	localctx = NewStringLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 110, SyntaxFlowParserRULE_stringLiteral)

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

	p.SetState(680)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(678)
			p.Identifier()
		}

	case SyntaxFlowParserStar:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(679)
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
	case SyntaxFlowParserVisitor:
		return t.VisitStringLiteralWithoutStar(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) StringLiteralWithoutStar() (localctx IStringLiteralWithoutStarContext) {
	this := p
	_ = this

	localctx = NewStringLiteralWithoutStarContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 112, SyntaxFlowParserRULE_stringLiteralWithoutStar)

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

	p.SetState(684)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(682)
			p.Identifier()
		}

	case SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(683)
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
	case SyntaxFlowParserVisitor:
		return t.VisitRegexpLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) RegexpLiteral() (localctx IRegexpLiteralContext) {
	this := p
	_ = this

	localctx = NewRegexpLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 114, SyntaxFlowParserRULE_regexpLiteral)

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
		p.SetState(686)
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
	case SyntaxFlowParserVisitor:
		return t.VisitIdentifier(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Identifier() (localctx IIdentifierContext) {
	this := p
	_ = this

	localctx = NewIdentifierContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 116, SyntaxFlowParserRULE_identifier)

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

	p.SetState(691)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserIdentifier:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(688)
			p.Match(SyntaxFlowParserIdentifier)
		}

	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(689)
			p.Keywords()
		}

	case SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(690)
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

func (s *KeywordsContext) ConstSearchPrefix() IConstSearchPrefixContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConstSearchPrefixContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConstSearchPrefixContext)
}

func (s *KeywordsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *KeywordsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *KeywordsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowParserVisitor:
		return t.VisitKeywords(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Keywords() (localctx IKeywordsContext) {
	this := p
	_ = this

	localctx = NewKeywordsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 118, SyntaxFlowParserRULE_keywords)

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

	p.SetState(706)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(693)
			p.Types()
		}

	case SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(694)
			p.Opcodes()
		}

	case SyntaxFlowParserOpcode:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(695)
			p.Match(SyntaxFlowParserOpcode)
		}

	case SyntaxFlowParserCheck:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(696)
			p.Match(SyntaxFlowParserCheck)
		}

	case SyntaxFlowParserThen:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(697)
			p.Match(SyntaxFlowParserThen)
		}

	case SyntaxFlowParserDesc:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(698)
			p.Match(SyntaxFlowParserDesc)
		}

	case SyntaxFlowParserElse:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(699)
			p.Match(SyntaxFlowParserElse)
		}

	case SyntaxFlowParserType:
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(700)
			p.Match(SyntaxFlowParserType)
		}

	case SyntaxFlowParserIn:
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(701)
			p.Match(SyntaxFlowParserIn)
		}

	case SyntaxFlowParserHave:
		p.EnterOuterAlt(localctx, 10)
		{
			p.SetState(702)
			p.Match(SyntaxFlowParserHave)
		}

	case SyntaxFlowParserHaveAny:
		p.EnterOuterAlt(localctx, 11)
		{
			p.SetState(703)
			p.Match(SyntaxFlowParserHaveAny)
		}

	case SyntaxFlowParserBoolLiteral:
		p.EnterOuterAlt(localctx, 12)
		{
			p.SetState(704)
			p.Match(SyntaxFlowParserBoolLiteral)
		}

	case SyntaxFlowParserConstSearchModePrefixRegexp, SyntaxFlowParserConstSearchModePrefixGlob, SyntaxFlowParserConstSearchModePrefixExact:
		p.EnterOuterAlt(localctx, 13)
		{
			p.SetState(705)
			p.ConstSearchPrefix()
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
	case SyntaxFlowParserVisitor:
		return t.VisitOpcodes(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Opcodes() (localctx IOpcodesContext) {
	this := p
	_ = this

	localctx = NewOpcodesContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 120, SyntaxFlowParserRULE_opcodes)
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
		p.SetState(708)
		_la = p.GetTokenStream().LA(1)

		if !((int64((_la-71)) & ^0x3f) == 0 && ((int64(1)<<(_la-71))&63) != 0) {
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
	case SyntaxFlowParserVisitor:
		return t.VisitTypes(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Types() (localctx ITypesContext) {
	this := p
	_ = this

	localctx = NewTypesContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 122, SyntaxFlowParserRULE_types)
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
		p.SetState(710)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&8935141660703064064) != 0) {
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
	case SyntaxFlowParserVisitor:
		return t.VisitBoolLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) BoolLiteral() (localctx IBoolLiteralContext) {
	this := p
	_ = this

	localctx = NewBoolLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 124, SyntaxFlowParserRULE_boolLiteral)

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
		p.SetState(712)
		p.Match(SyntaxFlowParserBoolLiteral)
	}

	return localctx
}

func (p *SyntaxFlowParser) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 52:
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
