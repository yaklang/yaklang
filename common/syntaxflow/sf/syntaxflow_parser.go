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
		"", "'else'", "'type'", "'in'", "'call'", "'function'", "", "'phi'",
		"", "", "'opcode'", "'have'", "'any'", "'not'", "'for'",
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
		"Have", "HaveAny", "Not", "For", "Identifier", "IdentifierChar", "QuotedStringLiteral",
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
		"thenExpr", "elseExpr", "refVariable", "filterItemFirst", "filterItem",
		"filterExpr", "nativeCall", "useNativeCall", "useDefCalcParams", "nativeCallActualParams",
		"nativeCallActualParam", "nativeCallActualParamKey", "nativeCallActualParamValue",
		"actualParam", "actualParamFilter", "singleParam", "config", "recursiveConfigItem",
		"recursiveConfigItemValue", "sliceCallItem", "nameFilter", "chainFilter",
		"stringLiteralWithoutStarGroup", "negativeCondition", "conditionExpression",
		"numberLiteral", "stringLiteral", "stringLiteralWithoutStar", "regexpLiteral",
		"identifier", "keywords", "opcodes", "types", "boolLiteral",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 93, 696, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
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
		2, 58, 7, 58, 2, 59, 7, 59, 2, 60, 7, 60, 1, 0, 1, 0, 1, 0, 1, 1, 4, 1,
		127, 8, 1, 11, 1, 12, 1, 128, 1, 2, 1, 2, 3, 2, 133, 8, 2, 1, 2, 1, 2,
		3, 2, 137, 8, 2, 1, 2, 1, 2, 3, 2, 141, 8, 2, 1, 2, 1, 2, 3, 2, 145, 8,
		2, 1, 2, 1, 2, 3, 2, 149, 8, 2, 1, 2, 1, 2, 3, 2, 153, 8, 2, 1, 2, 3, 2,
		156, 8, 2, 1, 3, 1, 3, 1, 3, 1, 3, 3, 3, 162, 8, 3, 1, 3, 1, 3, 1, 3, 1,
		3, 3, 3, 168, 8, 3, 1, 4, 1, 4, 3, 4, 172, 8, 4, 1, 5, 1, 5, 1, 5, 3, 5,
		177, 8, 5, 1, 5, 1, 5, 1, 6, 1, 6, 3, 6, 183, 8, 6, 1, 6, 1, 6, 3, 6, 187,
		8, 6, 1, 6, 1, 6, 3, 6, 191, 8, 6, 5, 6, 193, 8, 6, 10, 6, 12, 6, 196,
		9, 6, 1, 6, 3, 6, 199, 8, 6, 1, 6, 3, 6, 202, 8, 6, 1, 7, 3, 7, 205, 8,
		7, 1, 7, 1, 7, 1, 8, 1, 8, 1, 8, 1, 9, 1, 9, 1, 10, 1, 10, 1, 10, 5, 10,
		217, 8, 10, 10, 10, 12, 10, 220, 9, 10, 1, 11, 1, 11, 5, 11, 224, 8, 11,
		10, 11, 12, 11, 227, 9, 11, 1, 11, 1, 11, 3, 11, 231, 8, 11, 1, 11, 1,
		11, 1, 11, 3, 11, 236, 8, 11, 3, 11, 238, 8, 11, 1, 12, 1, 12, 1, 13, 1,
		13, 3, 13, 244, 8, 13, 1, 14, 1, 14, 1, 15, 4, 15, 249, 8, 15, 11, 15,
		12, 15, 250, 1, 16, 1, 16, 1, 16, 3, 16, 256, 8, 16, 1, 16, 1, 16, 1, 16,
		3, 16, 261, 8, 16, 1, 16, 3, 16, 264, 8, 16, 1, 17, 3, 17, 267, 8, 17,
		1, 17, 1, 17, 1, 17, 3, 17, 272, 8, 17, 1, 17, 5, 17, 275, 8, 17, 10, 17,
		12, 17, 278, 9, 17, 1, 17, 3, 17, 281, 8, 17, 1, 17, 3, 17, 284, 8, 17,
		1, 18, 1, 18, 3, 18, 288, 8, 18, 1, 18, 1, 18, 1, 18, 1, 18, 3, 18, 294,
		8, 18, 3, 18, 296, 8, 18, 1, 19, 1, 19, 3, 19, 300, 8, 19, 1, 20, 1, 20,
		3, 20, 304, 8, 20, 1, 20, 1, 20, 1, 21, 1, 21, 3, 21, 310, 8, 21, 1, 21,
		1, 21, 1, 22, 4, 22, 315, 8, 22, 11, 22, 12, 22, 316, 1, 23, 4, 23, 320,
		8, 23, 11, 23, 12, 23, 321, 1, 24, 1, 24, 1, 24, 1, 24, 3, 24, 328, 8,
		24, 1, 25, 1, 25, 1, 25, 1, 25, 3, 25, 334, 8, 25, 1, 26, 1, 26, 1, 26,
		3, 26, 339, 8, 26, 1, 26, 3, 26, 342, 8, 26, 1, 27, 1, 27, 1, 27, 1, 28,
		1, 28, 1, 28, 1, 29, 1, 29, 1, 29, 1, 29, 1, 29, 1, 29, 3, 29, 356, 8,
		29, 1, 30, 1, 30, 1, 30, 3, 30, 361, 8, 30, 1, 30, 1, 30, 3, 30, 365, 8,
		30, 1, 31, 1, 31, 1, 31, 3, 31, 370, 8, 31, 1, 31, 1, 31, 1, 31, 3, 31,
		375, 8, 31, 1, 31, 3, 31, 378, 8, 31, 1, 31, 1, 31, 1, 31, 1, 31, 1, 31,
		1, 31, 1, 31, 1, 31, 1, 31, 1, 31, 1, 31, 1, 31, 1, 31, 1, 31, 3, 31, 394,
		8, 31, 1, 31, 1, 31, 1, 31, 1, 31, 3, 31, 400, 8, 31, 1, 31, 1, 31, 1,
		31, 1, 31, 1, 31, 1, 31, 1, 31, 3, 31, 409, 8, 31, 1, 32, 1, 32, 5, 32,
		413, 8, 32, 10, 32, 12, 32, 416, 9, 32, 1, 33, 1, 33, 1, 33, 1, 33, 1,
		34, 1, 34, 3, 34, 424, 8, 34, 1, 35, 1, 35, 3, 35, 428, 8, 35, 1, 35, 1,
		35, 1, 35, 3, 35, 433, 8, 35, 1, 35, 3, 35, 436, 8, 35, 1, 36, 3, 36, 439,
		8, 36, 1, 36, 1, 36, 1, 36, 3, 36, 444, 8, 36, 1, 36, 5, 36, 447, 8, 36,
		10, 36, 12, 36, 450, 9, 36, 1, 36, 3, 36, 453, 8, 36, 1, 36, 3, 36, 456,
		8, 36, 1, 37, 1, 37, 1, 37, 3, 37, 461, 8, 37, 1, 37, 1, 37, 1, 38, 1,
		38, 1, 39, 1, 39, 1, 39, 1, 39, 5, 39, 471, 8, 39, 10, 39, 12, 39, 474,
		9, 39, 1, 39, 1, 39, 1, 39, 1, 39, 3, 39, 480, 8, 39, 1, 40, 1, 40, 3,
		40, 484, 8, 40, 1, 40, 4, 40, 487, 8, 40, 11, 40, 12, 40, 488, 1, 40, 3,
		40, 492, 8, 40, 1, 40, 3, 40, 495, 8, 40, 3, 40, 497, 8, 40, 1, 41, 1,
		41, 1, 41, 1, 41, 3, 41, 503, 8, 41, 1, 42, 1, 42, 1, 42, 3, 42, 508, 8,
		42, 1, 42, 3, 42, 511, 8, 42, 1, 42, 1, 42, 1, 43, 1, 43, 1, 43, 5, 43,
		518, 8, 43, 10, 43, 12, 43, 521, 9, 43, 1, 43, 3, 43, 524, 8, 43, 1, 43,
		3, 43, 527, 8, 43, 1, 44, 3, 44, 530, 8, 44, 1, 44, 1, 44, 1, 44, 1, 44,
		3, 44, 536, 8, 44, 1, 45, 1, 45, 3, 45, 540, 8, 45, 1, 45, 1, 45, 1, 45,
		1, 45, 3, 45, 546, 8, 45, 1, 46, 1, 46, 3, 46, 550, 8, 46, 1, 47, 1, 47,
		1, 47, 3, 47, 555, 8, 47, 1, 48, 1, 48, 1, 48, 1, 48, 5, 48, 561, 8, 48,
		10, 48, 12, 48, 564, 9, 48, 1, 48, 3, 48, 567, 8, 48, 1, 48, 1, 48, 1,
		48, 1, 48, 1, 48, 1, 48, 1, 48, 1, 48, 1, 48, 1, 48, 1, 48, 1, 48, 5, 48,
		581, 8, 48, 10, 48, 12, 48, 584, 9, 48, 3, 48, 586, 8, 48, 1, 48, 3, 48,
		589, 8, 48, 1, 48, 3, 48, 592, 8, 48, 1, 49, 1, 49, 1, 49, 5, 49, 597,
		8, 49, 10, 49, 12, 49, 600, 9, 49, 1, 49, 3, 49, 603, 8, 49, 1, 50, 1,
		50, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51,
		1, 51, 5, 51, 618, 8, 51, 10, 51, 12, 51, 621, 9, 51, 1, 51, 3, 51, 624,
		8, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1,
		51, 1, 51, 1, 51, 1, 51, 3, 51, 639, 8, 51, 1, 51, 1, 51, 1, 51, 3, 51,
		644, 8, 51, 3, 51, 646, 8, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51, 1, 51,
		5, 51, 654, 8, 51, 10, 51, 12, 51, 657, 9, 51, 1, 52, 1, 52, 1, 53, 1,
		53, 3, 53, 663, 8, 53, 1, 54, 1, 54, 3, 54, 667, 8, 54, 1, 55, 1, 55, 1,
		56, 1, 56, 1, 56, 3, 56, 674, 8, 56, 1, 57, 1, 57, 1, 57, 1, 57, 1, 57,
		1, 57, 1, 57, 1, 57, 1, 57, 1, 57, 1, 57, 1, 57, 3, 57, 688, 8, 57, 1,
		58, 1, 58, 1, 59, 1, 59, 1, 60, 1, 60, 1, 60, 0, 1, 102, 61, 0, 2, 4, 6,
		8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30, 32, 34, 36, 38, 40, 42,
		44, 46, 48, 50, 52, 54, 56, 58, 60, 62, 64, 66, 68, 70, 72, 74, 76, 78,
		80, 82, 84, 86, 88, 90, 92, 94, 96, 98, 100, 102, 104, 106, 108, 110, 112,
		114, 116, 118, 120, 0, 8, 2, 0, 29, 29, 42, 42, 1, 0, 48, 48, 2, 0, 44,
		44, 80, 80, 5, 0, 5, 6, 9, 9, 14, 14, 25, 25, 28, 29, 1, 0, 10, 11, 1,
		0, 54, 57, 1, 0, 71, 76, 1, 0, 58, 62, 779, 0, 122, 1, 0, 0, 0, 2, 126,
		1, 0, 0, 0, 4, 155, 1, 0, 0, 0, 6, 157, 1, 0, 0, 0, 8, 171, 1, 0, 0, 0,
		10, 173, 1, 0, 0, 0, 12, 180, 1, 0, 0, 0, 14, 204, 1, 0, 0, 0, 16, 208,
		1, 0, 0, 0, 18, 211, 1, 0, 0, 0, 20, 213, 1, 0, 0, 0, 22, 237, 1, 0, 0,
		0, 24, 239, 1, 0, 0, 0, 26, 243, 1, 0, 0, 0, 28, 245, 1, 0, 0, 0, 30, 248,
		1, 0, 0, 0, 32, 263, 1, 0, 0, 0, 34, 266, 1, 0, 0, 0, 36, 295, 1, 0, 0,
		0, 38, 299, 1, 0, 0, 0, 40, 301, 1, 0, 0, 0, 42, 307, 1, 0, 0, 0, 44, 314,
		1, 0, 0, 0, 46, 319, 1, 0, 0, 0, 48, 323, 1, 0, 0, 0, 50, 329, 1, 0, 0,
		0, 52, 335, 1, 0, 0, 0, 54, 343, 1, 0, 0, 0, 56, 346, 1, 0, 0, 0, 58, 349,
		1, 0, 0, 0, 60, 364, 1, 0, 0, 0, 62, 408, 1, 0, 0, 0, 64, 410, 1, 0, 0,
		0, 66, 417, 1, 0, 0, 0, 68, 421, 1, 0, 0, 0, 70, 435, 1, 0, 0, 0, 72, 438,
		1, 0, 0, 0, 74, 460, 1, 0, 0, 0, 76, 464, 1, 0, 0, 0, 78, 479, 1, 0, 0,
		0, 80, 496, 1, 0, 0, 0, 82, 502, 1, 0, 0, 0, 84, 510, 1, 0, 0, 0, 86, 514,
		1, 0, 0, 0, 88, 529, 1, 0, 0, 0, 90, 545, 1, 0, 0, 0, 92, 549, 1, 0, 0,
		0, 94, 554, 1, 0, 0, 0, 96, 591, 1, 0, 0, 0, 98, 593, 1, 0, 0, 0, 100,
		604, 1, 0, 0, 0, 102, 645, 1, 0, 0, 0, 104, 658, 1, 0, 0, 0, 106, 662,
		1, 0, 0, 0, 108, 666, 1, 0, 0, 0, 110, 668, 1, 0, 0, 0, 112, 673, 1, 0,
		0, 0, 114, 687, 1, 0, 0, 0, 116, 689, 1, 0, 0, 0, 118, 691, 1, 0, 0, 0,
		120, 693, 1, 0, 0, 0, 122, 123, 3, 2, 1, 0, 123, 124, 5, 0, 0, 1, 124,
		1, 1, 0, 0, 0, 125, 127, 3, 4, 2, 0, 126, 125, 1, 0, 0, 0, 127, 128, 1,
		0, 0, 0, 128, 126, 1, 0, 0, 0, 128, 129, 1, 0, 0, 0, 129, 3, 1, 0, 0, 0,
		130, 132, 3, 52, 26, 0, 131, 133, 3, 26, 13, 0, 132, 131, 1, 0, 0, 0, 132,
		133, 1, 0, 0, 0, 133, 156, 1, 0, 0, 0, 134, 136, 3, 32, 16, 0, 135, 137,
		3, 26, 13, 0, 136, 135, 1, 0, 0, 0, 136, 137, 1, 0, 0, 0, 137, 156, 1,
		0, 0, 0, 138, 140, 3, 50, 25, 0, 139, 141, 3, 26, 13, 0, 140, 139, 1, 0,
		0, 0, 140, 141, 1, 0, 0, 0, 141, 156, 1, 0, 0, 0, 142, 144, 3, 22, 11,
		0, 143, 145, 3, 26, 13, 0, 144, 143, 1, 0, 0, 0, 144, 145, 1, 0, 0, 0,
		145, 156, 1, 0, 0, 0, 146, 148, 3, 6, 3, 0, 147, 149, 3, 26, 13, 0, 148,
		147, 1, 0, 0, 0, 148, 149, 1, 0, 0, 0, 149, 156, 1, 0, 0, 0, 150, 152,
		3, 24, 12, 0, 151, 153, 3, 26, 13, 0, 152, 151, 1, 0, 0, 0, 152, 153, 1,
		0, 0, 0, 153, 156, 1, 0, 0, 0, 154, 156, 3, 26, 13, 0, 155, 130, 1, 0,
		0, 0, 155, 134, 1, 0, 0, 0, 155, 138, 1, 0, 0, 0, 155, 142, 1, 0, 0, 0,
		155, 146, 1, 0, 0, 0, 155, 150, 1, 0, 0, 0, 155, 154, 1, 0, 0, 0, 156,
		5, 1, 0, 0, 0, 157, 158, 5, 15, 0, 0, 158, 159, 3, 8, 4, 0, 159, 161, 5,
		39, 0, 0, 160, 162, 3, 30, 15, 0, 161, 160, 1, 0, 0, 0, 161, 162, 1, 0,
		0, 0, 162, 163, 1, 0, 0, 0, 163, 164, 5, 26, 0, 0, 164, 167, 3, 10, 5,
		0, 165, 166, 5, 47, 0, 0, 166, 168, 3, 58, 29, 0, 167, 165, 1, 0, 0, 0,
		167, 168, 1, 0, 0, 0, 168, 7, 1, 0, 0, 0, 169, 172, 3, 20, 10, 0, 170,
		172, 3, 110, 55, 0, 171, 169, 1, 0, 0, 0, 171, 170, 1, 0, 0, 0, 172, 9,
		1, 0, 0, 0, 173, 174, 5, 82, 0, 0, 174, 176, 5, 33, 0, 0, 175, 177, 3,
		12, 6, 0, 176, 175, 1, 0, 0, 0, 176, 177, 1, 0, 0, 0, 177, 178, 1, 0, 0,
		0, 178, 179, 5, 35, 0, 0, 179, 11, 1, 0, 0, 0, 180, 182, 3, 14, 7, 0, 181,
		183, 3, 30, 15, 0, 182, 181, 1, 0, 0, 0, 182, 183, 1, 0, 0, 0, 183, 194,
		1, 0, 0, 0, 184, 186, 5, 34, 0, 0, 185, 187, 3, 30, 15, 0, 186, 185, 1,
		0, 0, 0, 186, 187, 1, 0, 0, 0, 187, 188, 1, 0, 0, 0, 188, 190, 3, 14, 7,
		0, 189, 191, 3, 30, 15, 0, 190, 189, 1, 0, 0, 0, 190, 191, 1, 0, 0, 0,
		191, 193, 1, 0, 0, 0, 192, 184, 1, 0, 0, 0, 193, 196, 1, 0, 0, 0, 194,
		192, 1, 0, 0, 0, 194, 195, 1, 0, 0, 0, 195, 198, 1, 0, 0, 0, 196, 194,
		1, 0, 0, 0, 197, 199, 5, 34, 0, 0, 198, 197, 1, 0, 0, 0, 198, 199, 1, 0,
		0, 0, 199, 201, 1, 0, 0, 0, 200, 202, 3, 30, 15, 0, 201, 200, 1, 0, 0,
		0, 201, 202, 1, 0, 0, 0, 202, 13, 1, 0, 0, 0, 203, 205, 3, 16, 8, 0, 204,
		203, 1, 0, 0, 0, 204, 205, 1, 0, 0, 0, 205, 206, 1, 0, 0, 0, 206, 207,
		3, 18, 9, 0, 207, 15, 1, 0, 0, 0, 208, 209, 5, 82, 0, 0, 209, 210, 5, 42,
		0, 0, 210, 17, 1, 0, 0, 0, 211, 212, 3, 94, 47, 0, 212, 19, 1, 0, 0, 0,
		213, 218, 3, 94, 47, 0, 214, 215, 9, 0, 0, 0, 215, 217, 3, 94, 47, 0, 216,
		214, 1, 0, 0, 0, 217, 220, 1, 0, 0, 0, 218, 216, 1, 0, 0, 0, 218, 219,
		1, 0, 0, 0, 219, 21, 1, 0, 0, 0, 220, 218, 1, 0, 0, 0, 221, 225, 3, 58,
		29, 0, 222, 224, 3, 62, 31, 0, 223, 222, 1, 0, 0, 0, 224, 227, 1, 0, 0,
		0, 225, 223, 1, 0, 0, 0, 225, 226, 1, 0, 0, 0, 226, 230, 1, 0, 0, 0, 227,
		225, 1, 0, 0, 0, 228, 229, 5, 47, 0, 0, 229, 231, 3, 58, 29, 0, 230, 228,
		1, 0, 0, 0, 230, 231, 1, 0, 0, 0, 231, 238, 1, 0, 0, 0, 232, 235, 3, 64,
		32, 0, 233, 234, 5, 47, 0, 0, 234, 236, 3, 58, 29, 0, 235, 233, 1, 0, 0,
		0, 235, 236, 1, 0, 0, 0, 236, 238, 1, 0, 0, 0, 237, 221, 1, 0, 0, 0, 237,
		232, 1, 0, 0, 0, 238, 23, 1, 0, 0, 0, 239, 240, 5, 51, 0, 0, 240, 25, 1,
		0, 0, 0, 241, 244, 5, 16, 0, 0, 242, 244, 3, 28, 14, 0, 243, 241, 1, 0,
		0, 0, 243, 242, 1, 0, 0, 0, 244, 27, 1, 0, 0, 0, 245, 246, 5, 52, 0, 0,
		246, 29, 1, 0, 0, 0, 247, 249, 3, 28, 14, 0, 248, 247, 1, 0, 0, 0, 249,
		250, 1, 0, 0, 0, 250, 248, 1, 0, 0, 0, 250, 251, 1, 0, 0, 0, 251, 31, 1,
		0, 0, 0, 252, 253, 5, 67, 0, 0, 253, 255, 5, 33, 0, 0, 254, 256, 3, 34,
		17, 0, 255, 254, 1, 0, 0, 0, 255, 256, 1, 0, 0, 0, 256, 257, 1, 0, 0, 0,
		257, 264, 5, 35, 0, 0, 258, 260, 5, 38, 0, 0, 259, 261, 3, 34, 17, 0, 260,
		259, 1, 0, 0, 0, 260, 261, 1, 0, 0, 0, 261, 262, 1, 0, 0, 0, 262, 264,
		5, 39, 0, 0, 263, 252, 1, 0, 0, 0, 263, 258, 1, 0, 0, 0, 264, 33, 1, 0,
		0, 0, 265, 267, 3, 30, 15, 0, 266, 265, 1, 0, 0, 0, 266, 267, 1, 0, 0,
		0, 267, 268, 1, 0, 0, 0, 268, 276, 3, 36, 18, 0, 269, 271, 5, 34, 0, 0,
		270, 272, 3, 30, 15, 0, 271, 270, 1, 0, 0, 0, 271, 272, 1, 0, 0, 0, 272,
		273, 1, 0, 0, 0, 273, 275, 3, 36, 18, 0, 274, 269, 1, 0, 0, 0, 275, 278,
		1, 0, 0, 0, 276, 274, 1, 0, 0, 0, 276, 277, 1, 0, 0, 0, 277, 280, 1, 0,
		0, 0, 278, 276, 1, 0, 0, 0, 279, 281, 5, 34, 0, 0, 280, 279, 1, 0, 0, 0,
		280, 281, 1, 0, 0, 0, 281, 283, 1, 0, 0, 0, 282, 284, 3, 30, 15, 0, 283,
		282, 1, 0, 0, 0, 283, 284, 1, 0, 0, 0, 284, 35, 1, 0, 0, 0, 285, 287, 3,
		106, 53, 0, 286, 288, 3, 30, 15, 0, 287, 286, 1, 0, 0, 0, 287, 288, 1,
		0, 0, 0, 288, 296, 1, 0, 0, 0, 289, 290, 3, 106, 53, 0, 290, 291, 5, 42,
		0, 0, 291, 293, 3, 38, 19, 0, 292, 294, 3, 30, 15, 0, 293, 292, 1, 0, 0,
		0, 293, 294, 1, 0, 0, 0, 294, 296, 1, 0, 0, 0, 295, 285, 1, 0, 0, 0, 295,
		289, 1, 0, 0, 0, 296, 37, 1, 0, 0, 0, 297, 300, 3, 106, 53, 0, 298, 300,
		3, 48, 24, 0, 299, 297, 1, 0, 0, 0, 299, 298, 1, 0, 0, 0, 300, 39, 1, 0,
		0, 0, 301, 303, 5, 88, 0, 0, 302, 304, 3, 44, 22, 0, 303, 302, 1, 0, 0,
		0, 303, 304, 1, 0, 0, 0, 304, 305, 1, 0, 0, 0, 305, 306, 5, 90, 0, 0, 306,
		41, 1, 0, 0, 0, 307, 309, 5, 89, 0, 0, 308, 310, 3, 46, 23, 0, 309, 308,
		1, 0, 0, 0, 309, 310, 1, 0, 0, 0, 310, 311, 1, 0, 0, 0, 311, 312, 5, 92,
		0, 0, 312, 43, 1, 0, 0, 0, 313, 315, 5, 91, 0, 0, 314, 313, 1, 0, 0, 0,
		315, 316, 1, 0, 0, 0, 316, 314, 1, 0, 0, 0, 316, 317, 1, 0, 0, 0, 317,
		45, 1, 0, 0, 0, 318, 320, 5, 93, 0, 0, 319, 318, 1, 0, 0, 0, 320, 321,
		1, 0, 0, 0, 321, 319, 1, 0, 0, 0, 321, 322, 1, 0, 0, 0, 322, 47, 1, 0,
		0, 0, 323, 324, 5, 27, 0, 0, 324, 327, 5, 87, 0, 0, 325, 328, 3, 40, 20,
		0, 326, 328, 3, 42, 21, 0, 327, 325, 1, 0, 0, 0, 327, 326, 1, 0, 0, 0,
		328, 49, 1, 0, 0, 0, 329, 330, 5, 64, 0, 0, 330, 333, 3, 58, 29, 0, 331,
		332, 5, 81, 0, 0, 332, 334, 3, 106, 53, 0, 333, 331, 1, 0, 0, 0, 333, 334,
		1, 0, 0, 0, 334, 51, 1, 0, 0, 0, 335, 336, 5, 65, 0, 0, 336, 338, 3, 58,
		29, 0, 337, 339, 3, 54, 27, 0, 338, 337, 1, 0, 0, 0, 338, 339, 1, 0, 0,
		0, 339, 341, 1, 0, 0, 0, 340, 342, 3, 56, 28, 0, 341, 340, 1, 0, 0, 0,
		341, 342, 1, 0, 0, 0, 342, 53, 1, 0, 0, 0, 343, 344, 5, 66, 0, 0, 344,
		345, 3, 106, 53, 0, 345, 55, 1, 0, 0, 0, 346, 347, 5, 68, 0, 0, 347, 348,
		3, 106, 53, 0, 348, 57, 1, 0, 0, 0, 349, 355, 5, 41, 0, 0, 350, 356, 3,
		112, 56, 0, 351, 352, 5, 33, 0, 0, 352, 353, 3, 112, 56, 0, 353, 354, 5,
		35, 0, 0, 354, 356, 1, 0, 0, 0, 355, 350, 1, 0, 0, 0, 355, 351, 1, 0, 0,
		0, 356, 59, 1, 0, 0, 0, 357, 365, 3, 94, 47, 0, 358, 360, 5, 26, 0, 0,
		359, 361, 3, 30, 15, 0, 360, 359, 1, 0, 0, 0, 360, 361, 1, 0, 0, 0, 361,
		362, 1, 0, 0, 0, 362, 365, 3, 94, 47, 0, 363, 365, 3, 66, 33, 0, 364, 357,
		1, 0, 0, 0, 364, 358, 1, 0, 0, 0, 364, 363, 1, 0, 0, 0, 365, 61, 1, 0,
		0, 0, 366, 409, 3, 60, 30, 0, 367, 369, 5, 2, 0, 0, 368, 370, 3, 30, 15,
		0, 369, 368, 1, 0, 0, 0, 369, 370, 1, 0, 0, 0, 370, 371, 1, 0, 0, 0, 371,
		409, 3, 94, 47, 0, 372, 374, 5, 33, 0, 0, 373, 375, 3, 30, 15, 0, 374,
		373, 1, 0, 0, 0, 374, 375, 1, 0, 0, 0, 375, 377, 1, 0, 0, 0, 376, 378,
		3, 80, 40, 0, 377, 376, 1, 0, 0, 0, 377, 378, 1, 0, 0, 0, 378, 379, 1,
		0, 0, 0, 379, 409, 5, 35, 0, 0, 380, 381, 5, 36, 0, 0, 381, 382, 3, 92,
		46, 0, 382, 383, 5, 37, 0, 0, 383, 409, 1, 0, 0, 0, 384, 385, 5, 17, 0,
		0, 385, 386, 3, 102, 51, 0, 386, 387, 5, 39, 0, 0, 387, 409, 1, 0, 0, 0,
		388, 409, 5, 19, 0, 0, 389, 409, 5, 23, 0, 0, 390, 409, 5, 21, 0, 0, 391,
		393, 5, 18, 0, 0, 392, 394, 3, 86, 43, 0, 393, 392, 1, 0, 0, 0, 393, 394,
		1, 0, 0, 0, 394, 395, 1, 0, 0, 0, 395, 409, 5, 20, 0, 0, 396, 409, 5, 24,
		0, 0, 397, 399, 5, 22, 0, 0, 398, 400, 3, 86, 43, 0, 399, 398, 1, 0, 0,
		0, 399, 400, 1, 0, 0, 0, 400, 401, 1, 0, 0, 0, 401, 409, 5, 20, 0, 0, 402,
		403, 5, 30, 0, 0, 403, 409, 3, 58, 29, 0, 404, 405, 5, 46, 0, 0, 405, 409,
		3, 58, 29, 0, 406, 407, 5, 31, 0, 0, 407, 409, 3, 58, 29, 0, 408, 366,
		1, 0, 0, 0, 408, 367, 1, 0, 0, 0, 408, 372, 1, 0, 0, 0, 408, 380, 1, 0,
		0, 0, 408, 384, 1, 0, 0, 0, 408, 388, 1, 0, 0, 0, 408, 389, 1, 0, 0, 0,
		408, 390, 1, 0, 0, 0, 408, 391, 1, 0, 0, 0, 408, 396, 1, 0, 0, 0, 408,
		397, 1, 0, 0, 0, 408, 402, 1, 0, 0, 0, 408, 404, 1, 0, 0, 0, 408, 406,
		1, 0, 0, 0, 409, 63, 1, 0, 0, 0, 410, 414, 3, 60, 30, 0, 411, 413, 3, 62,
		31, 0, 412, 411, 1, 0, 0, 0, 413, 416, 1, 0, 0, 0, 414, 412, 1, 0, 0, 0,
		414, 415, 1, 0, 0, 0, 415, 65, 1, 0, 0, 0, 416, 414, 1, 0, 0, 0, 417, 418,
		5, 28, 0, 0, 418, 419, 3, 68, 34, 0, 419, 420, 5, 25, 0, 0, 420, 67, 1,
		0, 0, 0, 421, 423, 3, 112, 56, 0, 422, 424, 3, 70, 35, 0, 423, 422, 1,
		0, 0, 0, 423, 424, 1, 0, 0, 0, 424, 69, 1, 0, 0, 0, 425, 427, 5, 38, 0,
		0, 426, 428, 3, 72, 36, 0, 427, 426, 1, 0, 0, 0, 427, 428, 1, 0, 0, 0,
		428, 429, 1, 0, 0, 0, 429, 436, 5, 39, 0, 0, 430, 432, 5, 33, 0, 0, 431,
		433, 3, 72, 36, 0, 432, 431, 1, 0, 0, 0, 432, 433, 1, 0, 0, 0, 433, 434,
		1, 0, 0, 0, 434, 436, 5, 35, 0, 0, 435, 425, 1, 0, 0, 0, 435, 430, 1, 0,
		0, 0, 436, 71, 1, 0, 0, 0, 437, 439, 3, 30, 15, 0, 438, 437, 1, 0, 0, 0,
		438, 439, 1, 0, 0, 0, 439, 440, 1, 0, 0, 0, 440, 448, 3, 74, 37, 0, 441,
		443, 5, 34, 0, 0, 442, 444, 3, 30, 15, 0, 443, 442, 1, 0, 0, 0, 443, 444,
		1, 0, 0, 0, 444, 445, 1, 0, 0, 0, 445, 447, 3, 74, 37, 0, 446, 441, 1,
		0, 0, 0, 447, 450, 1, 0, 0, 0, 448, 446, 1, 0, 0, 0, 448, 449, 1, 0, 0,
		0, 449, 452, 1, 0, 0, 0, 450, 448, 1, 0, 0, 0, 451, 453, 5, 34, 0, 0, 452,
		451, 1, 0, 0, 0, 452, 453, 1, 0, 0, 0, 453, 455, 1, 0, 0, 0, 454, 456,
		3, 30, 15, 0, 455, 454, 1, 0, 0, 0, 455, 456, 1, 0, 0, 0, 456, 73, 1, 0,
		0, 0, 457, 458, 3, 76, 38, 0, 458, 459, 7, 0, 0, 0, 459, 461, 1, 0, 0,
		0, 460, 457, 1, 0, 0, 0, 460, 461, 1, 0, 0, 0, 461, 462, 1, 0, 0, 0, 462,
		463, 3, 78, 39, 0, 463, 75, 1, 0, 0, 0, 464, 465, 3, 112, 56, 0, 465, 77,
		1, 0, 0, 0, 466, 480, 3, 112, 56, 0, 467, 480, 3, 104, 52, 0, 468, 472,
		5, 48, 0, 0, 469, 471, 8, 1, 0, 0, 470, 469, 1, 0, 0, 0, 471, 474, 1, 0,
		0, 0, 472, 470, 1, 0, 0, 0, 472, 473, 1, 0, 0, 0, 473, 475, 1, 0, 0, 0,
		474, 472, 1, 0, 0, 0, 475, 480, 5, 48, 0, 0, 476, 477, 5, 41, 0, 0, 477,
		480, 3, 112, 56, 0, 478, 480, 3, 48, 24, 0, 479, 466, 1, 0, 0, 0, 479,
		467, 1, 0, 0, 0, 479, 468, 1, 0, 0, 0, 479, 476, 1, 0, 0, 0, 479, 478,
		1, 0, 0, 0, 480, 79, 1, 0, 0, 0, 481, 483, 3, 84, 42, 0, 482, 484, 3, 30,
		15, 0, 483, 482, 1, 0, 0, 0, 483, 484, 1, 0, 0, 0, 484, 497, 1, 0, 0, 0,
		485, 487, 3, 82, 41, 0, 486, 485, 1, 0, 0, 0, 487, 488, 1, 0, 0, 0, 488,
		486, 1, 0, 0, 0, 488, 489, 1, 0, 0, 0, 489, 491, 1, 0, 0, 0, 490, 492,
		3, 84, 42, 0, 491, 490, 1, 0, 0, 0, 491, 492, 1, 0, 0, 0, 492, 494, 1,
		0, 0, 0, 493, 495, 3, 30, 15, 0, 494, 493, 1, 0, 0, 0, 494, 495, 1, 0,
		0, 0, 495, 497, 1, 0, 0, 0, 496, 481, 1, 0, 0, 0, 496, 486, 1, 0, 0, 0,
		497, 81, 1, 0, 0, 0, 498, 499, 3, 84, 42, 0, 499, 500, 5, 34, 0, 0, 500,
		503, 1, 0, 0, 0, 501, 503, 5, 34, 0, 0, 502, 498, 1, 0, 0, 0, 502, 501,
		1, 0, 0, 0, 503, 83, 1, 0, 0, 0, 504, 511, 5, 23, 0, 0, 505, 507, 5, 22,
		0, 0, 506, 508, 3, 86, 43, 0, 507, 506, 1, 0, 0, 0, 507, 508, 1, 0, 0,
		0, 508, 509, 1, 0, 0, 0, 509, 511, 5, 39, 0, 0, 510, 504, 1, 0, 0, 0, 510,
		505, 1, 0, 0, 0, 510, 511, 1, 0, 0, 0, 511, 512, 1, 0, 0, 0, 512, 513,
		3, 22, 11, 0, 513, 85, 1, 0, 0, 0, 514, 519, 3, 88, 44, 0, 515, 516, 5,
		34, 0, 0, 516, 518, 3, 88, 44, 0, 517, 515, 1, 0, 0, 0, 518, 521, 1, 0,
		0, 0, 519, 517, 1, 0, 0, 0, 519, 520, 1, 0, 0, 0, 520, 523, 1, 0, 0, 0,
		521, 519, 1, 0, 0, 0, 522, 524, 5, 34, 0, 0, 523, 522, 1, 0, 0, 0, 523,
		524, 1, 0, 0, 0, 524, 526, 1, 0, 0, 0, 525, 527, 3, 30, 15, 0, 526, 525,
		1, 0, 0, 0, 526, 527, 1, 0, 0, 0, 527, 87, 1, 0, 0, 0, 528, 530, 3, 30,
		15, 0, 529, 528, 1, 0, 0, 0, 529, 530, 1, 0, 0, 0, 530, 531, 1, 0, 0, 0,
		531, 532, 3, 112, 56, 0, 532, 533, 5, 42, 0, 0, 533, 535, 3, 90, 45, 0,
		534, 536, 3, 30, 15, 0, 535, 534, 1, 0, 0, 0, 535, 536, 1, 0, 0, 0, 536,
		89, 1, 0, 0, 0, 537, 540, 3, 112, 56, 0, 538, 540, 3, 104, 52, 0, 539,
		537, 1, 0, 0, 0, 539, 538, 1, 0, 0, 0, 540, 546, 1, 0, 0, 0, 541, 542,
		5, 48, 0, 0, 542, 543, 3, 22, 11, 0, 543, 544, 5, 48, 0, 0, 544, 546, 1,
		0, 0, 0, 545, 539, 1, 0, 0, 0, 545, 541, 1, 0, 0, 0, 546, 91, 1, 0, 0,
		0, 547, 550, 3, 94, 47, 0, 548, 550, 3, 104, 52, 0, 549, 547, 1, 0, 0,
		0, 549, 548, 1, 0, 0, 0, 550, 93, 1, 0, 0, 0, 551, 555, 5, 45, 0, 0, 552,
		555, 3, 112, 56, 0, 553, 555, 3, 110, 55, 0, 554, 551, 1, 0, 0, 0, 554,
		552, 1, 0, 0, 0, 554, 553, 1, 0, 0, 0, 555, 95, 1, 0, 0, 0, 556, 566, 5,
		36, 0, 0, 557, 562, 3, 2, 1, 0, 558, 559, 5, 34, 0, 0, 559, 561, 3, 2,
		1, 0, 560, 558, 1, 0, 0, 0, 561, 564, 1, 0, 0, 0, 562, 560, 1, 0, 0, 0,
		562, 563, 1, 0, 0, 0, 563, 567, 1, 0, 0, 0, 564, 562, 1, 0, 0, 0, 565,
		567, 5, 2, 0, 0, 566, 557, 1, 0, 0, 0, 566, 565, 1, 0, 0, 0, 567, 568,
		1, 0, 0, 0, 568, 592, 5, 37, 0, 0, 569, 585, 5, 38, 0, 0, 570, 571, 3,
		112, 56, 0, 571, 572, 5, 42, 0, 0, 572, 573, 1, 0, 0, 0, 573, 582, 3, 2,
		1, 0, 574, 575, 5, 16, 0, 0, 575, 576, 3, 112, 56, 0, 576, 577, 5, 42,
		0, 0, 577, 578, 1, 0, 0, 0, 578, 579, 3, 2, 1, 0, 579, 581, 1, 0, 0, 0,
		580, 574, 1, 0, 0, 0, 581, 584, 1, 0, 0, 0, 582, 580, 1, 0, 0, 0, 582,
		583, 1, 0, 0, 0, 583, 586, 1, 0, 0, 0, 584, 582, 1, 0, 0, 0, 585, 570,
		1, 0, 0, 0, 585, 586, 1, 0, 0, 0, 586, 588, 1, 0, 0, 0, 587, 589, 5, 16,
		0, 0, 588, 587, 1, 0, 0, 0, 588, 589, 1, 0, 0, 0, 589, 590, 1, 0, 0, 0,
		590, 592, 5, 39, 0, 0, 591, 556, 1, 0, 0, 0, 591, 569, 1, 0, 0, 0, 592,
		97, 1, 0, 0, 0, 593, 598, 3, 108, 54, 0, 594, 595, 5, 34, 0, 0, 595, 597,
		3, 108, 54, 0, 596, 594, 1, 0, 0, 0, 597, 600, 1, 0, 0, 0, 598, 596, 1,
		0, 0, 0, 598, 599, 1, 0, 0, 0, 599, 602, 1, 0, 0, 0, 600, 598, 1, 0, 0,
		0, 601, 603, 5, 34, 0, 0, 602, 601, 1, 0, 0, 0, 602, 603, 1, 0, 0, 0, 603,
		99, 1, 0, 0, 0, 604, 605, 7, 2, 0, 0, 605, 101, 1, 0, 0, 0, 606, 607, 6,
		51, -1, 0, 607, 608, 5, 33, 0, 0, 608, 609, 3, 102, 51, 0, 609, 610, 5,
		35, 0, 0, 610, 646, 1, 0, 0, 0, 611, 646, 3, 64, 32, 0, 612, 613, 5, 77,
		0, 0, 613, 614, 5, 42, 0, 0, 614, 619, 3, 116, 58, 0, 615, 616, 5, 34,
		0, 0, 616, 618, 3, 116, 58, 0, 617, 615, 1, 0, 0, 0, 618, 621, 1, 0, 0,
		0, 619, 617, 1, 0, 0, 0, 619, 620, 1, 0, 0, 0, 620, 623, 1, 0, 0, 0, 621,
		619, 1, 0, 0, 0, 622, 624, 5, 34, 0, 0, 623, 622, 1, 0, 0, 0, 623, 624,
		1, 0, 0, 0, 624, 646, 1, 0, 0, 0, 625, 626, 5, 78, 0, 0, 626, 627, 5, 42,
		0, 0, 627, 646, 3, 98, 49, 0, 628, 629, 5, 79, 0, 0, 629, 630, 5, 42, 0,
		0, 630, 646, 3, 98, 49, 0, 631, 632, 3, 100, 50, 0, 632, 633, 3, 102, 51,
		5, 633, 646, 1, 0, 0, 0, 634, 638, 7, 3, 0, 0, 635, 639, 3, 104, 52, 0,
		636, 639, 3, 112, 56, 0, 637, 639, 3, 120, 60, 0, 638, 635, 1, 0, 0, 0,
		638, 636, 1, 0, 0, 0, 638, 637, 1, 0, 0, 0, 639, 646, 1, 0, 0, 0, 640,
		643, 7, 4, 0, 0, 641, 644, 3, 106, 53, 0, 642, 644, 3, 110, 55, 0, 643,
		641, 1, 0, 0, 0, 643, 642, 1, 0, 0, 0, 644, 646, 1, 0, 0, 0, 645, 606,
		1, 0, 0, 0, 645, 611, 1, 0, 0, 0, 645, 612, 1, 0, 0, 0, 645, 625, 1, 0,
		0, 0, 645, 628, 1, 0, 0, 0, 645, 631, 1, 0, 0, 0, 645, 634, 1, 0, 0, 0,
		645, 640, 1, 0, 0, 0, 646, 655, 1, 0, 0, 0, 647, 648, 10, 2, 0, 0, 648,
		649, 5, 12, 0, 0, 649, 654, 3, 102, 51, 3, 650, 651, 10, 1, 0, 0, 651,
		652, 5, 13, 0, 0, 652, 654, 3, 102, 51, 2, 653, 647, 1, 0, 0, 0, 653, 650,
		1, 0, 0, 0, 654, 657, 1, 0, 0, 0, 655, 653, 1, 0, 0, 0, 655, 656, 1, 0,
		0, 0, 656, 103, 1, 0, 0, 0, 657, 655, 1, 0, 0, 0, 658, 659, 7, 5, 0, 0,
		659, 105, 1, 0, 0, 0, 660, 663, 3, 112, 56, 0, 661, 663, 5, 45, 0, 0, 662,
		660, 1, 0, 0, 0, 662, 661, 1, 0, 0, 0, 663, 107, 1, 0, 0, 0, 664, 667,
		3, 112, 56, 0, 665, 667, 3, 110, 55, 0, 666, 664, 1, 0, 0, 0, 666, 665,
		1, 0, 0, 0, 667, 109, 1, 0, 0, 0, 668, 669, 5, 85, 0, 0, 669, 111, 1, 0,
		0, 0, 670, 674, 5, 82, 0, 0, 671, 674, 3, 114, 57, 0, 672, 674, 5, 84,
		0, 0, 673, 670, 1, 0, 0, 0, 673, 671, 1, 0, 0, 0, 673, 672, 1, 0, 0, 0,
		674, 113, 1, 0, 0, 0, 675, 688, 3, 118, 59, 0, 676, 688, 3, 116, 58, 0,
		677, 688, 5, 77, 0, 0, 678, 688, 5, 65, 0, 0, 679, 688, 5, 66, 0, 0, 680,
		688, 5, 67, 0, 0, 681, 688, 5, 68, 0, 0, 682, 688, 5, 69, 0, 0, 683, 688,
		5, 70, 0, 0, 684, 688, 5, 78, 0, 0, 685, 688, 5, 79, 0, 0, 686, 688, 5,
		63, 0, 0, 687, 675, 1, 0, 0, 0, 687, 676, 1, 0, 0, 0, 687, 677, 1, 0, 0,
		0, 687, 678, 1, 0, 0, 0, 687, 679, 1, 0, 0, 0, 687, 680, 1, 0, 0, 0, 687,
		681, 1, 0, 0, 0, 687, 682, 1, 0, 0, 0, 687, 683, 1, 0, 0, 0, 687, 684,
		1, 0, 0, 0, 687, 685, 1, 0, 0, 0, 687, 686, 1, 0, 0, 0, 688, 115, 1, 0,
		0, 0, 689, 690, 7, 6, 0, 0, 690, 117, 1, 0, 0, 0, 691, 692, 7, 7, 0, 0,
		692, 119, 1, 0, 0, 0, 693, 694, 5, 63, 0, 0, 694, 121, 1, 0, 0, 0, 104,
		128, 132, 136, 140, 144, 148, 152, 155, 161, 167, 171, 176, 182, 186, 190,
		194, 198, 201, 204, 218, 225, 230, 235, 237, 243, 250, 255, 260, 263, 266,
		271, 276, 280, 283, 287, 293, 295, 299, 303, 309, 316, 321, 327, 333, 338,
		341, 355, 360, 364, 369, 374, 377, 393, 399, 408, 414, 423, 427, 432, 435,
		438, 443, 448, 452, 455, 460, 472, 479, 483, 488, 491, 494, 496, 502, 507,
		510, 519, 523, 526, 529, 535, 539, 545, 549, 554, 562, 566, 582, 585, 588,
		591, 598, 602, 619, 623, 638, 643, 645, 653, 655, 662, 666, 673, 687,
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
	SyntaxFlowParserEOF                        = antlr.TokenEOF
	SyntaxFlowParserDeepFilter                 = 1
	SyntaxFlowParserDeep                       = 2
	SyntaxFlowParserPercent                    = 3
	SyntaxFlowParserDeepDot                    = 4
	SyntaxFlowParserLtEq                       = 5
	SyntaxFlowParserGtEq                       = 6
	SyntaxFlowParserDoubleGt                   = 7
	SyntaxFlowParserFilter                     = 8
	SyntaxFlowParserEqEq                       = 9
	SyntaxFlowParserRegexpMatch                = 10
	SyntaxFlowParserNotRegexpMatch             = 11
	SyntaxFlowParserAnd                        = 12
	SyntaxFlowParserOr                         = 13
	SyntaxFlowParserNotEq                      = 14
	SyntaxFlowParserDollarBraceOpen            = 15
	SyntaxFlowParserSemicolon                  = 16
	SyntaxFlowParserConditionStart             = 17
	SyntaxFlowParserDeepNextStart              = 18
	SyntaxFlowParserUseStart                   = 19
	SyntaxFlowParserDeepNextEnd                = 20
	SyntaxFlowParserDeepNext                   = 21
	SyntaxFlowParserTopDefStart                = 22
	SyntaxFlowParserDefStart                   = 23
	SyntaxFlowParserTopDef                     = 24
	SyntaxFlowParserGt                         = 25
	SyntaxFlowParserDot                        = 26
	SyntaxFlowParserStartNowDoc                = 27
	SyntaxFlowParserLt                         = 28
	SyntaxFlowParserEq                         = 29
	SyntaxFlowParserAdd                        = 30
	SyntaxFlowParserAmp                        = 31
	SyntaxFlowParserQuestion                   = 32
	SyntaxFlowParserOpenParen                  = 33
	SyntaxFlowParserComma                      = 34
	SyntaxFlowParserCloseParen                 = 35
	SyntaxFlowParserListSelectOpen             = 36
	SyntaxFlowParserListSelectClose            = 37
	SyntaxFlowParserMapBuilderOpen             = 38
	SyntaxFlowParserMapBuilderClose            = 39
	SyntaxFlowParserListStart                  = 40
	SyntaxFlowParserDollarOutput               = 41
	SyntaxFlowParserColon                      = 42
	SyntaxFlowParserSearch                     = 43
	SyntaxFlowParserBang                       = 44
	SyntaxFlowParserStar                       = 45
	SyntaxFlowParserMinus                      = 46
	SyntaxFlowParserAs                         = 47
	SyntaxFlowParserBacktick                   = 48
	SyntaxFlowParserSingleQuote                = 49
	SyntaxFlowParserDoubleQuote                = 50
	SyntaxFlowParserLineComment                = 51
	SyntaxFlowParserBreakLine                  = 52
	SyntaxFlowParserWhiteSpace                 = 53
	SyntaxFlowParserNumber                     = 54
	SyntaxFlowParserOctalNumber                = 55
	SyntaxFlowParserBinaryNumber               = 56
	SyntaxFlowParserHexNumber                  = 57
	SyntaxFlowParserStringType                 = 58
	SyntaxFlowParserListType                   = 59
	SyntaxFlowParserDictType                   = 60
	SyntaxFlowParserNumberType                 = 61
	SyntaxFlowParserBoolType                   = 62
	SyntaxFlowParserBoolLiteral                = 63
	SyntaxFlowParserAlert                      = 64
	SyntaxFlowParserCheck                      = 65
	SyntaxFlowParserThen                       = 66
	SyntaxFlowParserDesc                       = 67
	SyntaxFlowParserElse                       = 68
	SyntaxFlowParserType                       = 69
	SyntaxFlowParserIn                         = 70
	SyntaxFlowParserCall                       = 71
	SyntaxFlowParserFunction                   = 72
	SyntaxFlowParserConstant                   = 73
	SyntaxFlowParserPhi                        = 74
	SyntaxFlowParserFormalParam                = 75
	SyntaxFlowParserReturn                     = 76
	SyntaxFlowParserOpcode                     = 77
	SyntaxFlowParserHave                       = 78
	SyntaxFlowParserHaveAny                    = 79
	SyntaxFlowParserNot                        = 80
	SyntaxFlowParserFor                        = 81
	SyntaxFlowParserIdentifier                 = 82
	SyntaxFlowParserIdentifierChar             = 83
	SyntaxFlowParserQuotedStringLiteral        = 84
	SyntaxFlowParserRegexpLiteral              = 85
	SyntaxFlowParserWS                         = 86
	SyntaxFlowParserHereDocIdentifierName      = 87
	SyntaxFlowParserCRLFHereDocIdentifierBreak = 88
	SyntaxFlowParserLFHereDocIdentifierBreak   = 89
	SyntaxFlowParserCRLFEndDoc                 = 90
	SyntaxFlowParserCRLFHereDocText            = 91
	SyntaxFlowParserLFEndDoc                   = 92
	SyntaxFlowParserLFHereDocText              = 93
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
	SyntaxFlowParserRULE_filterItem                        = 31
	SyntaxFlowParserRULE_filterExpr                        = 32
	SyntaxFlowParserRULE_nativeCall                        = 33
	SyntaxFlowParserRULE_useNativeCall                     = 34
	SyntaxFlowParserRULE_useDefCalcParams                  = 35
	SyntaxFlowParserRULE_nativeCallActualParams            = 36
	SyntaxFlowParserRULE_nativeCallActualParam             = 37
	SyntaxFlowParserRULE_nativeCallActualParamKey          = 38
	SyntaxFlowParserRULE_nativeCallActualParamValue        = 39
	SyntaxFlowParserRULE_actualParam                       = 40
	SyntaxFlowParserRULE_actualParamFilter                 = 41
	SyntaxFlowParserRULE_singleParam                       = 42
	SyntaxFlowParserRULE_config                            = 43
	SyntaxFlowParserRULE_recursiveConfigItem               = 44
	SyntaxFlowParserRULE_recursiveConfigItemValue          = 45
	SyntaxFlowParserRULE_sliceCallItem                     = 46
	SyntaxFlowParserRULE_nameFilter                        = 47
	SyntaxFlowParserRULE_chainFilter                       = 48
	SyntaxFlowParserRULE_stringLiteralWithoutStarGroup     = 49
	SyntaxFlowParserRULE_negativeCondition                 = 50
	SyntaxFlowParserRULE_conditionExpression               = 51
	SyntaxFlowParserRULE_numberLiteral                     = 52
	SyntaxFlowParserRULE_stringLiteral                     = 53
	SyntaxFlowParserRULE_stringLiteralWithoutStar          = 54
	SyntaxFlowParserRULE_regexpLiteral                     = 55
	SyntaxFlowParserRULE_identifier                        = 56
	SyntaxFlowParserRULE_keywords                          = 57
	SyntaxFlowParserRULE_opcodes                           = 58
	SyntaxFlowParserRULE_types                             = 59
	SyntaxFlowParserRULE_boolLiteral                       = 60
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
		p.SetState(122)
		p.Statements()
	}
	{
		p.SetState(123)
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
	p.SetState(126)
	p.GetErrorHandler().Sync(p)
	_alt = 1
	for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		switch _alt {
		case 1:
			{
				p.SetState(125)
				p.Statement()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

		p.SetState(128)
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

	p.SetState(155)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 7, p.GetParserRuleContext()) {
	case 1:
		localctx = NewCheckContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(130)
			p.CheckStatement()
		}
		p.SetState(132)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 1, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(131)
				p.Eos()
			}

		}

	case 2:
		localctx = NewDescriptionContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(134)
			p.DescriptionStatement()
		}
		p.SetState(136)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 2, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(135)
				p.Eos()
			}

		}

	case 3:
		localctx = NewAlertContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(138)
			p.AlertStatement()
		}
		p.SetState(140)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 3, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(139)
				p.Eos()
			}

		}

	case 4:
		localctx = NewFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(142)
			p.FilterStatement()
		}
		p.SetState(144)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 4, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(143)
				p.Eos()
			}

		}

	case 5:
		localctx = NewFileFilterContentContext(p, localctx)
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(146)
			p.FileFilterContentStatement()
		}
		p.SetState(148)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 5, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(147)
				p.Eos()
			}

		}

	case 6:
		localctx = NewCommandContext(p, localctx)
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(150)
			p.Comment()
		}
		p.SetState(152)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 6, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(151)
				p.Eos()
			}

		}

	case 7:
		localctx = NewEmptyContext(p, localctx)
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(154)
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
		p.SetState(157)
		p.Match(SyntaxFlowParserDollarBraceOpen)
	}
	{
		p.SetState(158)
		p.FileFilterContentInput()
	}
	{
		p.SetState(159)
		p.Match(SyntaxFlowParserMapBuilderClose)
	}
	p.SetState(161)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(160)
			p.Lines()
		}

	}
	{
		p.SetState(163)
		p.Match(SyntaxFlowParserDot)
	}
	{
		p.SetState(164)
		p.FileFilterContentMethod()
	}
	p.SetState(167)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserAs {
		{
			p.SetState(165)
			p.Match(SyntaxFlowParserAs)
		}
		{
			p.SetState(166)
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
	p.SetState(171)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 10, p.GetParserRuleContext()) {
	case 1:
		{
			p.SetState(169)
			p.FileName()
		}

	case 2:
		{
			p.SetState(170)
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
		p.SetState(173)
		p.Match(SyntaxFlowParserIdentifier)
	}
	{
		p.SetState(174)
		p.Match(SyntaxFlowParserOpenParen)
	}
	p.SetState(176)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if (int64((_la-45)) & ^0x3f) == 0 && ((int64(1)<<(_la-45))&1821065601025) != 0 {
		{
			p.SetState(175)
			p.FileFilterContentMethodParam()
		}

	}
	{
		p.SetState(178)
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
		p.SetState(180)
		p.FileFilterContentMethodParamItem()
	}
	p.SetState(182)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 12, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(181)
			p.Lines()
		}

	}
	p.SetState(194)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 15, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(184)
				p.Match(SyntaxFlowParserComma)
			}
			p.SetState(186)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			if _la == SyntaxFlowParserBreakLine {
				{
					p.SetState(185)
					p.Lines()
				}

			}
			{
				p.SetState(188)
				p.FileFilterContentMethodParamItem()
			}
			p.SetState(190)
			p.GetErrorHandler().Sync(p)

			if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 14, p.GetParserRuleContext()) == 1 {
				{
					p.SetState(189)
					p.Lines()
				}

			}

		}
		p.SetState(196)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 15, p.GetParserRuleContext())
	}
	p.SetState(198)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserComma {
		{
			p.SetState(197)
			p.Match(SyntaxFlowParserComma)
		}

	}
	p.SetState(201)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(200)
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
	p.SetState(204)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 18, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(203)
			p.FileFilterContentMethodParamKey()
		}

	}
	{
		p.SetState(206)
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
		p.SetState(208)
		p.Match(SyntaxFlowParserIdentifier)
	}
	{
		p.SetState(209)
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
		p.SetState(211)
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
		p.SetState(213)
		p.NameFilter()
	}
	p.SetState(218)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 19, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			p.SetState(214)
			p.MatchWildcard()

			{
				p.SetState(215)
				p.NameFilter()
			}

		}
		p.SetState(220)
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

	p.SetState(237)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDollarOutput:
		localctx = NewRefFilterExprContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(221)
			p.RefVariable()
		}
		p.SetState(225)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 20, p.GetParserRuleContext())

		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(222)
					p.FilterItem()
				}

			}
			p.SetState(227)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 20, p.GetParserRuleContext())
		}
		p.SetState(230)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserAs {
			{
				p.SetState(228)
				p.Match(SyntaxFlowParserAs)
			}
			{
				p.SetState(229)
				p.RefVariable()
			}

		}

	case SyntaxFlowParserDot, SyntaxFlowParserLt, SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		localctx = NewPureFilterExprContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(232)
			p.FilterExpr()
		}
		p.SetState(235)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserAs {
			{
				p.SetState(233)
				p.Match(SyntaxFlowParserAs)
			}
			{
				p.SetState(234)
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
		p.SetState(239)
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

	p.SetState(243)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserSemicolon:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(241)
			p.Match(SyntaxFlowParserSemicolon)
		}

	case SyntaxFlowParserBreakLine:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(242)
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
		p.SetState(245)
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
	p.SetState(248)
	p.GetErrorHandler().Sync(p)
	_alt = 1
	for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		switch _alt {
		case 1:
			{
				p.SetState(247)
				p.Line()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

		p.SetState(250)
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

	p.SetState(263)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDesc:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(252)
			p.Match(SyntaxFlowParserDesc)
		}

		{
			p.SetState(253)
			p.Match(SyntaxFlowParserOpenParen)
		}
		p.SetState(255)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-45)) & ^0x3f) == 0 && ((int64(1)<<(_la-45))&721553973377) != 0 {
			{
				p.SetState(254)
				p.DescriptionItems()
			}

		}
		{
			p.SetState(257)
			p.Match(SyntaxFlowParserCloseParen)
		}

	case SyntaxFlowParserMapBuilderOpen:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(258)
			p.Match(SyntaxFlowParserMapBuilderOpen)
		}
		p.SetState(260)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-45)) & ^0x3f) == 0 && ((int64(1)<<(_la-45))&721553973377) != 0 {
			{
				p.SetState(259)
				p.DescriptionItems()
			}

		}
		{
			p.SetState(262)
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
	p.SetState(266)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(265)
			p.Lines()
		}

	}
	{
		p.SetState(268)
		p.DescriptionItem()
	}
	p.SetState(276)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 31, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(269)
				p.Match(SyntaxFlowParserComma)
			}
			p.SetState(271)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			if _la == SyntaxFlowParserBreakLine {
				{
					p.SetState(270)
					p.Lines()
				}

			}
			{
				p.SetState(273)
				p.DescriptionItem()
			}

		}
		p.SetState(278)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 31, p.GetParserRuleContext())
	}
	p.SetState(280)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserComma {
		{
			p.SetState(279)
			p.Match(SyntaxFlowParserComma)
		}

	}
	p.SetState(283)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(282)
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

	p.SetState(295)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 36, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(285)
			p.StringLiteral()
		}
		p.SetState(287)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 34, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(286)
				p.Lines()
			}

		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(289)
			p.StringLiteral()
		}
		{
			p.SetState(290)
			p.Match(SyntaxFlowParserColon)
		}
		{
			p.SetState(291)
			p.DescriptionItemValue()
		}
		p.SetState(293)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 35, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(292)
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

	p.SetState(299)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(297)
			p.StringLiteral()
		}

	case SyntaxFlowParserStartNowDoc:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(298)
			p.HereDoc()
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
		p.SetState(301)
		p.Match(SyntaxFlowParserCRLFHereDocIdentifierBreak)
	}
	p.SetState(303)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserCRLFHereDocText {
		{
			p.SetState(302)
			p.CrlfText()
		}

	}
	{
		p.SetState(305)
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
		p.SetState(307)
		p.Match(SyntaxFlowParserLFHereDocIdentifierBreak)
	}
	p.SetState(309)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserLFHereDocText {
		{
			p.SetState(308)
			p.LfText()
		}

	}
	{
		p.SetState(311)
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
	p.SetState(314)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == SyntaxFlowParserCRLFHereDocText {
		{
			p.SetState(313)
			p.Match(SyntaxFlowParserCRLFHereDocText)
		}

		p.SetState(316)
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
	p.SetState(319)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == SyntaxFlowParserLFHereDocText {
		{
			p.SetState(318)
			p.Match(SyntaxFlowParserLFHereDocText)
		}

		p.SetState(321)
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
		p.SetState(323)
		p.Match(SyntaxFlowParserStartNowDoc)
	}
	{
		p.SetState(324)
		p.Match(SyntaxFlowParserHereDocIdentifierName)
	}
	p.SetState(327)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserCRLFHereDocIdentifierBreak:
		{
			p.SetState(325)
			p.CrlfHereDoc()
		}

	case SyntaxFlowParserLFHereDocIdentifierBreak:
		{
			p.SetState(326)
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
		p.SetState(329)
		p.Match(SyntaxFlowParserAlert)
	}
	{
		p.SetState(330)
		p.RefVariable()
	}
	p.SetState(333)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserFor {
		{
			p.SetState(331)
			p.Match(SyntaxFlowParserFor)
		}
		{
			p.SetState(332)
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
		p.SetState(335)
		p.Match(SyntaxFlowParserCheck)
	}
	{
		p.SetState(336)
		p.RefVariable()
	}
	p.SetState(338)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 44, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(337)
			p.ThenExpr()
		}

	}
	p.SetState(341)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 45, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(340)
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
		p.SetState(343)
		p.Match(SyntaxFlowParserThen)
	}
	{
		p.SetState(344)
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
		p.SetState(346)
		p.Match(SyntaxFlowParserElse)
	}
	{
		p.SetState(347)
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
		p.SetState(349)
		p.Match(SyntaxFlowParserDollarOutput)
	}
	p.SetState(355)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		{
			p.SetState(350)
			p.Identifier()
		}

	case SyntaxFlowParserOpenParen:
		{
			p.SetState(351)
			p.Match(SyntaxFlowParserOpenParen)
		}
		{
			p.SetState(352)
			p.Identifier()
		}
		{
			p.SetState(353)
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

	p.SetState(364)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		localctx = NewNamedFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(357)
			p.NameFilter()
		}

	case SyntaxFlowParserDot:
		localctx = NewFieldCallFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(358)
			p.Match(SyntaxFlowParserDot)
		}
		p.SetState(360)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserBreakLine {
			{
				p.SetState(359)
				p.Lines()
			}

		}
		{
			p.SetState(362)
			p.NameFilter()
		}

	case SyntaxFlowParserLt:
		localctx = NewNativeCallFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(363)
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
	p.EnterRule(localctx, 62, SyntaxFlowParserRULE_filterItem)
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

	p.SetState(408)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDot, SyntaxFlowParserLt, SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		localctx = NewFirstContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(366)
			p.FilterItemFirst()
		}

	case SyntaxFlowParserDeep:
		localctx = NewDeepChainFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(367)
			p.Match(SyntaxFlowParserDeep)
		}
		p.SetState(369)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserBreakLine {
			{
				p.SetState(368)
				p.Lines()
			}

		}
		{
			p.SetState(371)
			p.NameFilter()
		}

	case SyntaxFlowParserOpenParen:
		localctx = NewFunctionCallFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(372)
			p.Match(SyntaxFlowParserOpenParen)
		}
		p.SetState(374)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserBreakLine {
			{
				p.SetState(373)
				p.Lines()
			}

		}
		p.SetState(377)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-22)) & ^0x3f) == 0 && ((int64(1)<<(_la-22))&-3170538604425899949) != 0 {
			{
				p.SetState(376)
				p.ActualParam()
			}

		}
		{
			p.SetState(379)
			p.Match(SyntaxFlowParserCloseParen)
		}

	case SyntaxFlowParserListSelectOpen:
		localctx = NewFieldIndexFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(380)
			p.Match(SyntaxFlowParserListSelectOpen)
		}
		{
			p.SetState(381)
			p.SliceCallItem()
		}
		{
			p.SetState(382)
			p.Match(SyntaxFlowParserListSelectClose)
		}

	case SyntaxFlowParserConditionStart:
		localctx = NewOptionalFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(384)
			p.Match(SyntaxFlowParserConditionStart)
		}
		{
			p.SetState(385)
			p.conditionExpression(0)
		}
		{
			p.SetState(386)
			p.Match(SyntaxFlowParserMapBuilderClose)
		}

	case SyntaxFlowParserUseStart:
		localctx = NewNextFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(388)
			p.Match(SyntaxFlowParserUseStart)
		}

	case SyntaxFlowParserDefStart:
		localctx = NewDefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(389)
			p.Match(SyntaxFlowParserDefStart)
		}

	case SyntaxFlowParserDeepNext:
		localctx = NewDeepNextFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(390)
			p.Match(SyntaxFlowParserDeepNext)
		}

	case SyntaxFlowParserDeepNextStart:
		localctx = NewDeepNextConfigFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(391)
			p.Match(SyntaxFlowParserDeepNextStart)
		}
		p.SetState(393)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-52)) & ^0x3f) == 0 && ((int64(1)<<(_la-52))&5637140417) != 0 {
			{
				p.SetState(392)
				p.Config()
			}

		}
		{
			p.SetState(395)
			p.Match(SyntaxFlowParserDeepNextEnd)
		}

	case SyntaxFlowParserTopDef:
		localctx = NewTopDefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 10)
		{
			p.SetState(396)
			p.Match(SyntaxFlowParserTopDef)
		}

	case SyntaxFlowParserTopDefStart:
		localctx = NewTopDefConfigFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 11)
		{
			p.SetState(397)
			p.Match(SyntaxFlowParserTopDefStart)
		}
		p.SetState(399)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-52)) & ^0x3f) == 0 && ((int64(1)<<(_la-52))&5637140417) != 0 {
			{
				p.SetState(398)
				p.Config()
			}

		}
		{
			p.SetState(401)
			p.Match(SyntaxFlowParserDeepNextEnd)
		}

	case SyntaxFlowParserAdd:
		localctx = NewMergeRefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 12)
		{
			p.SetState(402)
			p.Match(SyntaxFlowParserAdd)
		}
		{
			p.SetState(403)
			p.RefVariable()
		}

	case SyntaxFlowParserMinus:
		localctx = NewRemoveRefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 13)
		{
			p.SetState(404)
			p.Match(SyntaxFlowParserMinus)
		}
		{
			p.SetState(405)
			p.RefVariable()
		}

	case SyntaxFlowParserAmp:
		localctx = NewIntersectionRefFilterContext(p, localctx)
		p.EnterOuterAlt(localctx, 14)
		{
			p.SetState(406)
			p.Match(SyntaxFlowParserAmp)
		}
		{
			p.SetState(407)
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
	p.EnterRule(localctx, 64, SyntaxFlowParserRULE_filterExpr)

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
		p.SetState(410)
		p.FilterItemFirst()
	}
	p.SetState(414)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 55, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(411)
				p.FilterItem()
			}

		}
		p.SetState(416)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 55, p.GetParserRuleContext())
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
	p.EnterRule(localctx, 66, SyntaxFlowParserRULE_nativeCall)

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
		p.Match(SyntaxFlowParserLt)
	}
	{
		p.SetState(418)
		p.UseNativeCall()
	}
	{
		p.SetState(419)
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
	p.EnterRule(localctx, 68, SyntaxFlowParserRULE_useNativeCall)
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
		p.Identifier()
	}
	p.SetState(423)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserOpenParen || _la == SyntaxFlowParserMapBuilderOpen {
		{
			p.SetState(422)
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
	p.EnterRule(localctx, 70, SyntaxFlowParserRULE_useDefCalcParams)
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

	p.SetState(435)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserMapBuilderOpen:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(425)
			p.Match(SyntaxFlowParserMapBuilderOpen)
		}
		p.SetState(427)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-27)) & ^0x3f) == 0 && ((int64(1)<<(_la-27))&189151046812057601) != 0 {
			{
				p.SetState(426)
				p.NativeCallActualParams()
			}

		}
		{
			p.SetState(429)
			p.Match(SyntaxFlowParserMapBuilderClose)
		}

	case SyntaxFlowParserOpenParen:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(430)
			p.Match(SyntaxFlowParserOpenParen)
		}
		p.SetState(432)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-27)) & ^0x3f) == 0 && ((int64(1)<<(_la-27))&189151046812057601) != 0 {
			{
				p.SetState(431)
				p.NativeCallActualParams()
			}

		}
		{
			p.SetState(434)
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
	p.EnterRule(localctx, 72, SyntaxFlowParserRULE_nativeCallActualParams)
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
	p.SetState(438)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(437)
			p.Lines()
		}

	}
	{
		p.SetState(440)
		p.NativeCallActualParam()
	}
	p.SetState(448)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 62, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(441)
				p.Match(SyntaxFlowParserComma)
			}
			p.SetState(443)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			if _la == SyntaxFlowParserBreakLine {
				{
					p.SetState(442)
					p.Lines()
				}

			}
			{
				p.SetState(445)
				p.NativeCallActualParam()
			}

		}
		p.SetState(450)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 62, p.GetParserRuleContext())
	}
	p.SetState(452)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserComma {
		{
			p.SetState(451)
			p.Match(SyntaxFlowParserComma)
		}

	}
	p.SetState(455)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(454)
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
	p.EnterRule(localctx, 74, SyntaxFlowParserRULE_nativeCallActualParam)
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
	p.SetState(460)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 65, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(457)
			p.NativeCallActualParamKey()
		}
		{
			p.SetState(458)
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
		p.SetState(462)
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
	p.EnterRule(localctx, 76, SyntaxFlowParserRULE_nativeCallActualParamKey)

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
		p.SetState(464)
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
	p.EnterRule(localctx, 78, SyntaxFlowParserRULE_nativeCallActualParamValue)
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

	p.SetState(479)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(466)
			p.Identifier()
		}

	case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(467)
			p.NumberLiteral()
		}

	case SyntaxFlowParserBacktick:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(468)
			p.Match(SyntaxFlowParserBacktick)
		}
		p.SetState(472)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-281474976710658) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&1073741823) != 0 {
			{
				p.SetState(469)
				_la = p.GetTokenStream().LA(1)

				if _la <= 0 || _la == SyntaxFlowParserBacktick {
					p.GetErrorHandler().RecoverInline(p)
				} else {
					p.GetErrorHandler().ReportMatch(p)
					p.Consume()
				}
			}

			p.SetState(474)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(475)
			p.Match(SyntaxFlowParserBacktick)
		}

	case SyntaxFlowParserDollarOutput:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(476)
			p.Match(SyntaxFlowParserDollarOutput)
		}
		{
			p.SetState(477)
			p.Identifier()
		}

	case SyntaxFlowParserStartNowDoc:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(478)
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
	p.EnterRule(localctx, 80, SyntaxFlowParserRULE_actualParam)
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

	p.SetState(496)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 72, p.GetParserRuleContext()) {
	case 1:
		localctx = NewAllParamContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(481)
			p.SingleParam()
		}
		p.SetState(483)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserBreakLine {
			{
				p.SetState(482)
				p.Lines()
			}

		}

	case 2:
		localctx = NewEveryParamContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		p.SetState(486)
		p.GetErrorHandler().Sync(p)
		_alt = 1
		for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			switch _alt {
			case 1:
				{
					p.SetState(485)
					p.ActualParamFilter()
				}

			default:
				panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
			}

			p.SetState(488)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 69, p.GetParserRuleContext())
		}
		p.SetState(491)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-22)) & ^0x3f) == 0 && ((int64(1)<<(_la-22))&-3170538604425904045) != 0 {
			{
				p.SetState(490)
				p.SingleParam()
			}

		}
		p.SetState(494)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserBreakLine {
			{
				p.SetState(493)
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
	p.EnterRule(localctx, 82, SyntaxFlowParserRULE_actualParamFilter)

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

	p.SetState(502)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserTopDefStart, SyntaxFlowParserDefStart, SyntaxFlowParserDot, SyntaxFlowParserLt, SyntaxFlowParserDollarOutput, SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(498)
			p.SingleParam()
		}
		{
			p.SetState(499)
			p.Match(SyntaxFlowParserComma)
		}

	case SyntaxFlowParserComma:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(501)
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
	p.EnterRule(localctx, 84, SyntaxFlowParserRULE_singleParam)
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
	p.SetState(510)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDefStart:
		{
			p.SetState(504)
			p.Match(SyntaxFlowParserDefStart)
		}

	case SyntaxFlowParserTopDefStart:
		{
			p.SetState(505)
			p.Match(SyntaxFlowParserTopDefStart)
		}
		p.SetState(507)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-52)) & ^0x3f) == 0 && ((int64(1)<<(_la-52))&5637140417) != 0 {
			{
				p.SetState(506)
				p.Config()
			}

		}
		{
			p.SetState(509)
			p.Match(SyntaxFlowParserMapBuilderClose)
		}

	case SyntaxFlowParserDot, SyntaxFlowParserLt, SyntaxFlowParserDollarOutput, SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:

	default:
	}
	{
		p.SetState(512)
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
	p.EnterRule(localctx, 86, SyntaxFlowParserRULE_config)
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
		p.SetState(514)
		p.RecursiveConfigItem()
	}
	p.SetState(519)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 76, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(515)
				p.Match(SyntaxFlowParserComma)
			}
			{
				p.SetState(516)
				p.RecursiveConfigItem()
			}

		}
		p.SetState(521)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 76, p.GetParserRuleContext())
	}
	p.SetState(523)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserComma {
		{
			p.SetState(522)
			p.Match(SyntaxFlowParserComma)
		}

	}
	p.SetState(526)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(525)
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
	p.EnterRule(localctx, 88, SyntaxFlowParserRULE_recursiveConfigItem)
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
	p.SetState(529)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserBreakLine {
		{
			p.SetState(528)
			p.Lines()
		}

	}
	{
		p.SetState(531)
		p.Identifier()
	}
	{
		p.SetState(532)
		p.Match(SyntaxFlowParserColon)
	}
	{
		p.SetState(533)
		p.RecursiveConfigItemValue()
	}
	p.SetState(535)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 80, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(534)
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
	p.EnterRule(localctx, 90, SyntaxFlowParserRULE_recursiveConfigItemValue)

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

	p.SetState(545)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		p.SetState(539)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
			{
				p.SetState(537)
				p.Identifier()
			}

		case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
			{
				p.SetState(538)
				p.NumberLiteral()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

	case SyntaxFlowParserBacktick:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(541)
			p.Match(SyntaxFlowParserBacktick)
		}
		{
			p.SetState(542)
			p.FilterStatement()
		}
		{
			p.SetState(543)
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
	p.EnterRule(localctx, 92, SyntaxFlowParserRULE_sliceCallItem)

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

	p.SetState(549)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(547)
			p.NameFilter()
		}

	case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(548)
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
	p.EnterRule(localctx, 94, SyntaxFlowParserRULE_nameFilter)

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

	p.SetState(554)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStar:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(551)
			p.Match(SyntaxFlowParserStar)
		}

	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(552)
			p.Identifier()
		}

	case SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(553)
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
	p.EnterRule(localctx, 96, SyntaxFlowParserRULE_chainFilter)
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

	p.SetState(591)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserListSelectOpen:
		localctx = NewFlatContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(556)
			p.Match(SyntaxFlowParserListSelectOpen)
		}
		p.SetState(566)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserDollarBraceOpen, SyntaxFlowParserSemicolon, SyntaxFlowParserDot, SyntaxFlowParserLt, SyntaxFlowParserMapBuilderOpen, SyntaxFlowParserDollarOutput, SyntaxFlowParserStar, SyntaxFlowParserLineComment, SyntaxFlowParserBreakLine, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserAlert, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral, SyntaxFlowParserRegexpLiteral:
			{
				p.SetState(557)
				p.Statements()
			}
			p.SetState(562)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			for _la == SyntaxFlowParserComma {
				{
					p.SetState(558)
					p.Match(SyntaxFlowParserComma)
				}
				{
					p.SetState(559)
					p.Statements()
				}

				p.SetState(564)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)
			}

		case SyntaxFlowParserDeep:
			{
				p.SetState(565)
				p.Match(SyntaxFlowParserDeep)
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}
		{
			p.SetState(568)
			p.Match(SyntaxFlowParserListSelectClose)
		}

	case SyntaxFlowParserMapBuilderOpen:
		localctx = NewBuildMapContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(569)
			p.Match(SyntaxFlowParserMapBuilderOpen)
		}
		p.SetState(585)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-58)) & ^0x3f) == 0 && ((int64(1)<<(_la-58))&88080319) != 0 {
			{
				p.SetState(570)
				p.Identifier()
			}
			{
				p.SetState(571)
				p.Match(SyntaxFlowParserColon)
			}

			{
				p.SetState(573)
				p.Statements()
			}
			p.SetState(582)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 87, p.GetParserRuleContext())

			for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
				if _alt == 1 {
					{
						p.SetState(574)
						p.Match(SyntaxFlowParserSemicolon)
					}

					{
						p.SetState(575)
						p.Identifier()
					}
					{
						p.SetState(576)
						p.Match(SyntaxFlowParserColon)
					}

					{
						p.SetState(578)
						p.Statements()
					}

				}
				p.SetState(584)
				p.GetErrorHandler().Sync(p)
				_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 87, p.GetParserRuleContext())
			}

		}
		p.SetState(588)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserSemicolon {
			{
				p.SetState(587)
				p.Match(SyntaxFlowParserSemicolon)
			}

		}
		{
			p.SetState(590)
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
	p.EnterRule(localctx, 98, SyntaxFlowParserRULE_stringLiteralWithoutStarGroup)

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
		p.SetState(593)
		p.StringLiteralWithoutStar()
	}
	p.SetState(598)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 91, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(594)
				p.Match(SyntaxFlowParserComma)
			}
			{
				p.SetState(595)
				p.StringLiteralWithoutStar()
			}

		}
		p.SetState(600)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 91, p.GetParserRuleContext())
	}
	p.SetState(602)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 92, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(601)
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
	p.EnterRule(localctx, 100, SyntaxFlowParserRULE_negativeCondition)
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
		p.SetState(604)
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
	_startState := 102
	p.EnterRecursionRule(localctx, 102, SyntaxFlowParserRULE_conditionExpression, _p)
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
	p.SetState(645)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 97, p.GetParserRuleContext()) {
	case 1:
		localctx = NewParenConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx

		{
			p.SetState(607)
			p.Match(SyntaxFlowParserOpenParen)
		}
		{
			p.SetState(608)
			p.conditionExpression(0)
		}
		{
			p.SetState(609)
			p.Match(SyntaxFlowParserCloseParen)
		}

	case 2:
		localctx = NewFilterConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(611)
			p.FilterExpr()
		}

	case 3:
		localctx = NewOpcodeTypeConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(612)
			p.Match(SyntaxFlowParserOpcode)
		}
		{
			p.SetState(613)
			p.Match(SyntaxFlowParserColon)
		}
		{
			p.SetState(614)
			p.Opcodes()
		}
		p.SetState(619)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 93, p.GetParserRuleContext())

		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(615)
					p.Match(SyntaxFlowParserComma)
				}
				{
					p.SetState(616)
					p.Opcodes()
				}

			}
			p.SetState(621)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 93, p.GetParserRuleContext())
		}
		p.SetState(623)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 94, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(622)
				p.Match(SyntaxFlowParserComma)
			}

		}

	case 4:
		localctx = NewStringContainHaveConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(625)
			p.Match(SyntaxFlowParserHave)
		}
		{
			p.SetState(626)
			p.Match(SyntaxFlowParserColon)
		}
		{
			p.SetState(627)
			p.StringLiteralWithoutStarGroup()
		}

	case 5:
		localctx = NewStringContainAnyConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(628)
			p.Match(SyntaxFlowParserHaveAny)
		}
		{
			p.SetState(629)
			p.Match(SyntaxFlowParserColon)
		}
		{
			p.SetState(630)
			p.StringLiteralWithoutStarGroup()
		}

	case 6:
		localctx = NewNotConditionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(631)
			p.NegativeCondition()
		}
		{
			p.SetState(632)
			p.conditionExpression(5)
		}

	case 7:
		localctx = NewFilterExpressionCompareContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(634)

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
		p.SetState(638)
		p.GetErrorHandler().Sync(p)
		switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 95, p.GetParserRuleContext()) {
		case 1:
			{
				p.SetState(635)
				p.NumberLiteral()
			}

		case 2:
			{
				p.SetState(636)
				p.Identifier()
			}

		case 3:
			{
				p.SetState(637)
				p.BoolLiteral()
			}

		}

	case 8:
		localctx = NewFilterExpressionRegexpMatchContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(640)

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
		p.SetState(643)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserStar, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
			{
				p.SetState(641)
				p.StringLiteral()
			}

		case SyntaxFlowParserRegexpLiteral:
			{
				p.SetState(642)
				p.RegexpLiteral()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

	}
	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(655)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 99, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			p.SetState(653)
			p.GetErrorHandler().Sync(p)
			switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 98, p.GetParserRuleContext()) {
			case 1:
				localctx = NewFilterExpressionAndContext(p, NewConditionExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_conditionExpression)
				p.SetState(647)

				if !(p.Precpred(p.GetParserRuleContext(), 2)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 2)", ""))
				}
				{
					p.SetState(648)
					p.Match(SyntaxFlowParserAnd)
				}
				{
					p.SetState(649)
					p.conditionExpression(3)
				}

			case 2:
				localctx = NewFilterExpressionOrContext(p, NewConditionExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_conditionExpression)
				p.SetState(650)

				if !(p.Precpred(p.GetParserRuleContext(), 1)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 1)", ""))
				}
				{
					p.SetState(651)
					p.Match(SyntaxFlowParserOr)
				}
				{
					p.SetState(652)
					p.conditionExpression(2)
				}

			}

		}
		p.SetState(657)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 99, p.GetParserRuleContext())
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
	p.EnterRule(localctx, 104, SyntaxFlowParserRULE_numberLiteral)
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
		p.SetState(658)
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
	p.EnterRule(localctx, 106, SyntaxFlowParserRULE_stringLiteral)

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

	p.SetState(662)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(660)
			p.Identifier()
		}

	case SyntaxFlowParserStar:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(661)
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
	p.EnterRule(localctx, 108, SyntaxFlowParserRULE_stringLiteralWithoutStar)

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

	p.SetState(666)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny, SyntaxFlowParserIdentifier, SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(664)
			p.Identifier()
		}

	case SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(665)
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
	p.EnterRule(localctx, 110, SyntaxFlowParserRULE_regexpLiteral)

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
		p.SetState(668)
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
	p.EnterRule(localctx, 112, SyntaxFlowParserRULE_identifier)

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

	p.SetState(673)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserIdentifier:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(670)
			p.Match(SyntaxFlowParserIdentifier)
		}

	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserBoolLiteral, SyntaxFlowParserCheck, SyntaxFlowParserThen, SyntaxFlowParserDesc, SyntaxFlowParserElse, SyntaxFlowParserType, SyntaxFlowParserIn, SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn, SyntaxFlowParserOpcode, SyntaxFlowParserHave, SyntaxFlowParserHaveAny:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(671)
			p.Keywords()
		}

	case SyntaxFlowParserQuotedStringLiteral:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(672)
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
	p.EnterRule(localctx, 114, SyntaxFlowParserRULE_keywords)

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

	p.SetState(687)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(675)
			p.Types()
		}

	case SyntaxFlowParserCall, SyntaxFlowParserFunction, SyntaxFlowParserConstant, SyntaxFlowParserPhi, SyntaxFlowParserFormalParam, SyntaxFlowParserReturn:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(676)
			p.Opcodes()
		}

	case SyntaxFlowParserOpcode:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(677)
			p.Match(SyntaxFlowParserOpcode)
		}

	case SyntaxFlowParserCheck:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(678)
			p.Match(SyntaxFlowParserCheck)
		}

	case SyntaxFlowParserThen:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(679)
			p.Match(SyntaxFlowParserThen)
		}

	case SyntaxFlowParserDesc:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(680)
			p.Match(SyntaxFlowParserDesc)
		}

	case SyntaxFlowParserElse:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(681)
			p.Match(SyntaxFlowParserElse)
		}

	case SyntaxFlowParserType:
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(682)
			p.Match(SyntaxFlowParserType)
		}

	case SyntaxFlowParserIn:
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(683)
			p.Match(SyntaxFlowParserIn)
		}

	case SyntaxFlowParserHave:
		p.EnterOuterAlt(localctx, 10)
		{
			p.SetState(684)
			p.Match(SyntaxFlowParserHave)
		}

	case SyntaxFlowParserHaveAny:
		p.EnterOuterAlt(localctx, 11)
		{
			p.SetState(685)
			p.Match(SyntaxFlowParserHaveAny)
		}

	case SyntaxFlowParserBoolLiteral:
		p.EnterOuterAlt(localctx, 12)
		{
			p.SetState(686)
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
	p.EnterRule(localctx, 116, SyntaxFlowParserRULE_opcodes)
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
		p.SetState(689)
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
	p.EnterRule(localctx, 118, SyntaxFlowParserRULE_types)
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
		p.SetState(691)
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
	p.EnterRule(localctx, 120, SyntaxFlowParserRULE_boolLiteral)

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
		p.SetState(693)
		p.Match(SyntaxFlowParserBoolLiteral)
	}

	return localctx
}

func (p *SyntaxFlowParser) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 51:
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
