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
		"", "'->'", "'-->'", "';'", "'==>'", "'...'", "'%%'", "'..'", "'<='",
		"'>='", "'>>'", "'=>'", "'=='", "'=~'", "'!~'", "'&&'", "'||'", "'!='",
		"'?{'", "'-{'", "'}->'", "'#{'", "'#>'", "'#->'", "'>'", "'.'", "'<'",
		"'='", "'?'", "'('", "','", "')'", "'['", "']'", "'{'", "'}'", "'#'",
		"'$'", "':'", "'%'", "'!'", "'*'", "'-'", "'as'", "", "", "", "", "",
		"", "'str'", "'list'", "'dict'", "", "'bool'",
	}
	staticData.symbolicNames = []string{
		"", "", "", "", "DeepFilter", "Deep", "Percent", "DeepDot", "LtEq",
		"GtEq", "DoubleGt", "Filter", "EqEq", "RegexpMatch", "NotRegexpMatch",
		"And", "Or", "NotEq", "ConditionStart", "DeepNextStart", "DeepNextEnd",
		"TopDefStart", "DefStart", "TopDef", "Gt", "Dot", "Lt", "Eq", "Question",
		"OpenParen", "Comma", "CloseParen", "ListSelectOpen", "ListSelectClose",
		"MapBuilderOpen", "MapBuilderClose", "ListStart", "DollarOutput", "Colon",
		"Search", "Bang", "Star", "Minus", "As", "WhiteSpace", "Number", "OctalNumber",
		"BinaryNumber", "HexNumber", "StringLiteral", "StringType", "ListType",
		"DictType", "NumberType", "BoolType", "BoolLiteral", "Identifier", "IdentifierChar",
		"RegexpLiteral", "WS",
	}
	staticData.ruleNames = []string{
		"flow", "filters", "filterStatement", "refVariable", "filterExpr", "acutalParamFilter",
		"recursiveConfig", "recursiveConfigItem", "recursiveConfigItemValue",
		"sliceCallItem", "nameFilter", "chainFilter", "conditionExpression",
		"numberLiteral", "stringLiteral", "regexpLiteral", "identifier", "types",
		"boolLiteral",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 59, 259, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 1, 0, 1, 0, 1, 0, 1, 1, 4, 1,
		43, 8, 1, 11, 1, 12, 1, 44, 1, 2, 1, 2, 1, 2, 3, 2, 50, 8, 2, 1, 3, 1,
		3, 1, 3, 1, 3, 1, 3, 1, 3, 3, 3, 58, 8, 3, 1, 4, 1, 4, 1, 4, 3, 4, 63,
		8, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 3, 4, 70, 8, 4, 1, 4, 1, 4, 1, 4, 1,
		4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 3,
		4, 87, 8, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 3, 4, 94, 8, 4, 1, 4, 1, 4,
		1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 5, 4, 104, 8, 4, 10, 4, 12, 4, 107,
		9, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4,
		5, 4, 120, 8, 4, 10, 4, 12, 4, 123, 9, 4, 1, 5, 1, 5, 1, 5, 3, 5, 128,
		8, 5, 1, 5, 3, 5, 131, 8, 5, 1, 5, 1, 5, 3, 5, 135, 8, 5, 1, 5, 3, 5, 138,
		8, 5, 1, 6, 1, 6, 1, 6, 5, 6, 143, 8, 6, 10, 6, 12, 6, 146, 9, 6, 1, 6,
		3, 6, 149, 8, 6, 1, 7, 1, 7, 1, 7, 1, 7, 1, 8, 1, 8, 3, 8, 157, 8, 8, 1,
		8, 1, 8, 3, 8, 161, 8, 8, 1, 9, 1, 9, 3, 9, 165, 8, 9, 1, 10, 1, 10, 3,
		10, 169, 8, 10, 1, 11, 1, 11, 1, 11, 1, 11, 5, 11, 175, 8, 11, 10, 11,
		12, 11, 178, 9, 11, 1, 11, 3, 11, 181, 8, 11, 1, 11, 1, 11, 1, 11, 1, 11,
		1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 5, 11, 195, 8,
		11, 10, 11, 12, 11, 198, 9, 11, 3, 11, 200, 8, 11, 1, 11, 3, 11, 203, 8,
		11, 1, 11, 3, 11, 206, 8, 11, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12,
		1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 3, 12, 222, 8,
		12, 1, 12, 1, 12, 1, 12, 3, 12, 227, 8, 12, 3, 12, 229, 8, 12, 1, 12, 1,
		12, 1, 12, 1, 12, 1, 12, 1, 12, 5, 12, 237, 8, 12, 10, 12, 12, 12, 240,
		9, 12, 1, 13, 1, 13, 1, 14, 1, 14, 3, 14, 246, 8, 14, 1, 15, 1, 15, 1,
		16, 1, 16, 1, 16, 3, 16, 253, 8, 16, 1, 17, 1, 17, 1, 18, 1, 18, 1, 18,
		0, 2, 8, 24, 19, 0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28,
		30, 32, 34, 36, 0, 4, 5, 0, 8, 9, 12, 12, 17, 17, 24, 24, 26, 27, 1, 0,
		13, 14, 1, 0, 45, 48, 1, 0, 50, 54, 291, 0, 38, 1, 0, 0, 0, 2, 42, 1, 0,
		0, 0, 4, 46, 1, 0, 0, 0, 6, 51, 1, 0, 0, 0, 8, 69, 1, 0, 0, 0, 10, 137,
		1, 0, 0, 0, 12, 139, 1, 0, 0, 0, 14, 150, 1, 0, 0, 0, 16, 160, 1, 0, 0,
		0, 18, 164, 1, 0, 0, 0, 20, 168, 1, 0, 0, 0, 22, 205, 1, 0, 0, 0, 24, 228,
		1, 0, 0, 0, 26, 241, 1, 0, 0, 0, 28, 245, 1, 0, 0, 0, 30, 247, 1, 0, 0,
		0, 32, 252, 1, 0, 0, 0, 34, 254, 1, 0, 0, 0, 36, 256, 1, 0, 0, 0, 38, 39,
		3, 2, 1, 0, 39, 40, 5, 0, 0, 1, 40, 1, 1, 0, 0, 0, 41, 43, 3, 4, 2, 0,
		42, 41, 1, 0, 0, 0, 43, 44, 1, 0, 0, 0, 44, 42, 1, 0, 0, 0, 44, 45, 1,
		0, 0, 0, 45, 3, 1, 0, 0, 0, 46, 49, 3, 8, 4, 0, 47, 48, 5, 43, 0, 0, 48,
		50, 3, 6, 3, 0, 49, 47, 1, 0, 0, 0, 49, 50, 1, 0, 0, 0, 50, 5, 1, 0, 0,
		0, 51, 57, 5, 37, 0, 0, 52, 58, 3, 32, 16, 0, 53, 54, 5, 29, 0, 0, 54,
		55, 3, 32, 16, 0, 55, 56, 5, 31, 0, 0, 56, 58, 1, 0, 0, 0, 57, 52, 1, 0,
		0, 0, 57, 53, 1, 0, 0, 0, 58, 7, 1, 0, 0, 0, 59, 60, 6, 4, -1, 0, 60, 62,
		5, 37, 0, 0, 61, 63, 3, 32, 16, 0, 62, 61, 1, 0, 0, 0, 62, 63, 1, 0, 0,
		0, 63, 70, 1, 0, 0, 0, 64, 70, 5, 41, 0, 0, 65, 70, 3, 32, 16, 0, 66, 70,
		3, 30, 15, 0, 67, 68, 5, 25, 0, 0, 68, 70, 3, 20, 10, 0, 69, 59, 1, 0,
		0, 0, 69, 64, 1, 0, 0, 0, 69, 65, 1, 0, 0, 0, 69, 66, 1, 0, 0, 0, 69, 67,
		1, 0, 0, 0, 70, 121, 1, 0, 0, 0, 71, 72, 10, 6, 0, 0, 72, 73, 5, 1, 0,
		0, 73, 120, 3, 8, 4, 7, 74, 75, 10, 5, 0, 0, 75, 76, 5, 22, 0, 0, 76, 120,
		3, 8, 4, 6, 77, 78, 10, 4, 0, 0, 78, 79, 5, 2, 0, 0, 79, 120, 3, 8, 4,
		5, 80, 81, 10, 3, 0, 0, 81, 82, 5, 23, 0, 0, 82, 120, 3, 8, 4, 4, 83, 84,
		10, 2, 0, 0, 84, 86, 5, 19, 0, 0, 85, 87, 3, 12, 6, 0, 86, 85, 1, 0, 0,
		0, 86, 87, 1, 0, 0, 0, 87, 88, 1, 0, 0, 0, 88, 89, 5, 20, 0, 0, 89, 120,
		3, 8, 4, 3, 90, 91, 10, 1, 0, 0, 91, 93, 5, 21, 0, 0, 92, 94, 3, 12, 6,
		0, 93, 92, 1, 0, 0, 0, 93, 94, 1, 0, 0, 0, 94, 95, 1, 0, 0, 0, 95, 96,
		5, 20, 0, 0, 96, 120, 3, 8, 4, 2, 97, 98, 10, 10, 0, 0, 98, 99, 5, 25,
		0, 0, 99, 120, 3, 20, 10, 0, 100, 101, 10, 9, 0, 0, 101, 105, 5, 29, 0,
		0, 102, 104, 3, 10, 5, 0, 103, 102, 1, 0, 0, 0, 104, 107, 1, 0, 0, 0, 105,
		103, 1, 0, 0, 0, 105, 106, 1, 0, 0, 0, 106, 108, 1, 0, 0, 0, 107, 105,
		1, 0, 0, 0, 108, 120, 5, 31, 0, 0, 109, 110, 10, 8, 0, 0, 110, 111, 5,
		32, 0, 0, 111, 112, 3, 18, 9, 0, 112, 113, 5, 33, 0, 0, 113, 120, 1, 0,
		0, 0, 114, 115, 10, 7, 0, 0, 115, 116, 5, 18, 0, 0, 116, 117, 3, 24, 12,
		0, 117, 118, 5, 35, 0, 0, 118, 120, 1, 0, 0, 0, 119, 71, 1, 0, 0, 0, 119,
		74, 1, 0, 0, 0, 119, 77, 1, 0, 0, 0, 119, 80, 1, 0, 0, 0, 119, 83, 1, 0,
		0, 0, 119, 90, 1, 0, 0, 0, 119, 97, 1, 0, 0, 0, 119, 100, 1, 0, 0, 0, 119,
		109, 1, 0, 0, 0, 119, 114, 1, 0, 0, 0, 120, 123, 1, 0, 0, 0, 121, 119,
		1, 0, 0, 0, 121, 122, 1, 0, 0, 0, 122, 9, 1, 0, 0, 0, 123, 121, 1, 0, 0,
		0, 124, 131, 5, 22, 0, 0, 125, 127, 5, 21, 0, 0, 126, 128, 3, 12, 6, 0,
		127, 126, 1, 0, 0, 0, 127, 128, 1, 0, 0, 0, 128, 129, 1, 0, 0, 0, 129,
		131, 5, 35, 0, 0, 130, 124, 1, 0, 0, 0, 130, 125, 1, 0, 0, 0, 130, 131,
		1, 0, 0, 0, 131, 132, 1, 0, 0, 0, 132, 134, 3, 4, 2, 0, 133, 135, 5, 30,
		0, 0, 134, 133, 1, 0, 0, 0, 134, 135, 1, 0, 0, 0, 135, 138, 1, 0, 0, 0,
		136, 138, 5, 30, 0, 0, 137, 130, 1, 0, 0, 0, 137, 136, 1, 0, 0, 0, 138,
		11, 1, 0, 0, 0, 139, 144, 3, 14, 7, 0, 140, 141, 5, 30, 0, 0, 141, 143,
		3, 14, 7, 0, 142, 140, 1, 0, 0, 0, 143, 146, 1, 0, 0, 0, 144, 142, 1, 0,
		0, 0, 144, 145, 1, 0, 0, 0, 145, 148, 1, 0, 0, 0, 146, 144, 1, 0, 0, 0,
		147, 149, 5, 30, 0, 0, 148, 147, 1, 0, 0, 0, 148, 149, 1, 0, 0, 0, 149,
		13, 1, 0, 0, 0, 150, 151, 3, 32, 16, 0, 151, 152, 5, 38, 0, 0, 152, 153,
		3, 16, 8, 0, 153, 15, 1, 0, 0, 0, 154, 157, 3, 32, 16, 0, 155, 157, 3,
		26, 13, 0, 156, 154, 1, 0, 0, 0, 156, 155, 1, 0, 0, 0, 157, 161, 1, 0,
		0, 0, 158, 159, 5, 39, 0, 0, 159, 161, 3, 4, 2, 0, 160, 156, 1, 0, 0, 0,
		160, 158, 1, 0, 0, 0, 161, 17, 1, 0, 0, 0, 162, 165, 3, 20, 10, 0, 163,
		165, 3, 26, 13, 0, 164, 162, 1, 0, 0, 0, 164, 163, 1, 0, 0, 0, 165, 19,
		1, 0, 0, 0, 166, 169, 3, 32, 16, 0, 167, 169, 3, 30, 15, 0, 168, 166, 1,
		0, 0, 0, 168, 167, 1, 0, 0, 0, 169, 21, 1, 0, 0, 0, 170, 180, 5, 32, 0,
		0, 171, 176, 3, 2, 1, 0, 172, 173, 5, 30, 0, 0, 173, 175, 3, 2, 1, 0, 174,
		172, 1, 0, 0, 0, 175, 178, 1, 0, 0, 0, 176, 174, 1, 0, 0, 0, 176, 177,
		1, 0, 0, 0, 177, 181, 1, 0, 0, 0, 178, 176, 1, 0, 0, 0, 179, 181, 5, 5,
		0, 0, 180, 171, 1, 0, 0, 0, 180, 179, 1, 0, 0, 0, 181, 182, 1, 0, 0, 0,
		182, 206, 5, 33, 0, 0, 183, 199, 5, 34, 0, 0, 184, 185, 3, 32, 16, 0, 185,
		186, 5, 38, 0, 0, 186, 187, 1, 0, 0, 0, 187, 196, 3, 2, 1, 0, 188, 189,
		5, 3, 0, 0, 189, 190, 3, 32, 16, 0, 190, 191, 5, 38, 0, 0, 191, 192, 1,
		0, 0, 0, 192, 193, 3, 2, 1, 0, 193, 195, 1, 0, 0, 0, 194, 188, 1, 0, 0,
		0, 195, 198, 1, 0, 0, 0, 196, 194, 1, 0, 0, 0, 196, 197, 1, 0, 0, 0, 197,
		200, 1, 0, 0, 0, 198, 196, 1, 0, 0, 0, 199, 184, 1, 0, 0, 0, 199, 200,
		1, 0, 0, 0, 200, 202, 1, 0, 0, 0, 201, 203, 5, 3, 0, 0, 202, 201, 1, 0,
		0, 0, 202, 203, 1, 0, 0, 0, 203, 204, 1, 0, 0, 0, 204, 206, 5, 35, 0, 0,
		205, 170, 1, 0, 0, 0, 205, 183, 1, 0, 0, 0, 206, 23, 1, 0, 0, 0, 207, 208,
		6, 12, -1, 0, 208, 229, 3, 26, 13, 0, 209, 229, 3, 28, 14, 0, 210, 229,
		3, 30, 15, 0, 211, 212, 5, 29, 0, 0, 212, 213, 3, 24, 12, 0, 213, 214,
		5, 31, 0, 0, 214, 229, 1, 0, 0, 0, 215, 216, 5, 40, 0, 0, 216, 229, 3,
		24, 12, 5, 217, 221, 7, 0, 0, 0, 218, 222, 3, 26, 13, 0, 219, 222, 3, 32,
		16, 0, 220, 222, 3, 36, 18, 0, 221, 218, 1, 0, 0, 0, 221, 219, 1, 0, 0,
		0, 221, 220, 1, 0, 0, 0, 222, 229, 1, 0, 0, 0, 223, 226, 7, 1, 0, 0, 224,
		227, 3, 28, 14, 0, 225, 227, 3, 30, 15, 0, 226, 224, 1, 0, 0, 0, 226, 225,
		1, 0, 0, 0, 227, 229, 1, 0, 0, 0, 228, 207, 1, 0, 0, 0, 228, 209, 1, 0,
		0, 0, 228, 210, 1, 0, 0, 0, 228, 211, 1, 0, 0, 0, 228, 215, 1, 0, 0, 0,
		228, 217, 1, 0, 0, 0, 228, 223, 1, 0, 0, 0, 229, 238, 1, 0, 0, 0, 230,
		231, 10, 2, 0, 0, 231, 232, 5, 15, 0, 0, 232, 237, 3, 24, 12, 3, 233, 234,
		10, 1, 0, 0, 234, 235, 5, 16, 0, 0, 235, 237, 3, 24, 12, 2, 236, 230, 1,
		0, 0, 0, 236, 233, 1, 0, 0, 0, 237, 240, 1, 0, 0, 0, 238, 236, 1, 0, 0,
		0, 238, 239, 1, 0, 0, 0, 239, 25, 1, 0, 0, 0, 240, 238, 1, 0, 0, 0, 241,
		242, 7, 2, 0, 0, 242, 27, 1, 0, 0, 0, 243, 246, 3, 32, 16, 0, 244, 246,
		5, 41, 0, 0, 245, 243, 1, 0, 0, 0, 245, 244, 1, 0, 0, 0, 246, 29, 1, 0,
		0, 0, 247, 248, 5, 58, 0, 0, 248, 31, 1, 0, 0, 0, 249, 253, 5, 56, 0, 0,
		250, 253, 3, 34, 17, 0, 251, 253, 5, 43, 0, 0, 252, 249, 1, 0, 0, 0, 252,
		250, 1, 0, 0, 0, 252, 251, 1, 0, 0, 0, 253, 33, 1, 0, 0, 0, 254, 255, 7,
		3, 0, 0, 255, 35, 1, 0, 0, 0, 256, 257, 5, 55, 0, 0, 257, 37, 1, 0, 0,
		0, 33, 44, 49, 57, 62, 69, 86, 93, 105, 119, 121, 127, 130, 134, 137, 144,
		148, 156, 160, 164, 168, 176, 180, 196, 199, 202, 205, 221, 226, 228, 236,
		238, 245, 252,
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
	SyntaxFlowParserEOF             = antlr.TokenEOF
	SyntaxFlowParserT__0            = 1
	SyntaxFlowParserT__1            = 2
	SyntaxFlowParserT__2            = 3
	SyntaxFlowParserDeepFilter      = 4
	SyntaxFlowParserDeep            = 5
	SyntaxFlowParserPercent         = 6
	SyntaxFlowParserDeepDot         = 7
	SyntaxFlowParserLtEq            = 8
	SyntaxFlowParserGtEq            = 9
	SyntaxFlowParserDoubleGt        = 10
	SyntaxFlowParserFilter          = 11
	SyntaxFlowParserEqEq            = 12
	SyntaxFlowParserRegexpMatch     = 13
	SyntaxFlowParserNotRegexpMatch  = 14
	SyntaxFlowParserAnd             = 15
	SyntaxFlowParserOr              = 16
	SyntaxFlowParserNotEq           = 17
	SyntaxFlowParserConditionStart  = 18
	SyntaxFlowParserDeepNextStart   = 19
	SyntaxFlowParserDeepNextEnd     = 20
	SyntaxFlowParserTopDefStart     = 21
	SyntaxFlowParserDefStart        = 22
	SyntaxFlowParserTopDef          = 23
	SyntaxFlowParserGt              = 24
	SyntaxFlowParserDot             = 25
	SyntaxFlowParserLt              = 26
	SyntaxFlowParserEq              = 27
	SyntaxFlowParserQuestion        = 28
	SyntaxFlowParserOpenParen       = 29
	SyntaxFlowParserComma           = 30
	SyntaxFlowParserCloseParen      = 31
	SyntaxFlowParserListSelectOpen  = 32
	SyntaxFlowParserListSelectClose = 33
	SyntaxFlowParserMapBuilderOpen  = 34
	SyntaxFlowParserMapBuilderClose = 35
	SyntaxFlowParserListStart       = 36
	SyntaxFlowParserDollarOutput    = 37
	SyntaxFlowParserColon           = 38
	SyntaxFlowParserSearch          = 39
	SyntaxFlowParserBang            = 40
	SyntaxFlowParserStar            = 41
	SyntaxFlowParserMinus           = 42
	SyntaxFlowParserAs              = 43
	SyntaxFlowParserWhiteSpace      = 44
	SyntaxFlowParserNumber          = 45
	SyntaxFlowParserOctalNumber     = 46
	SyntaxFlowParserBinaryNumber    = 47
	SyntaxFlowParserHexNumber       = 48
	SyntaxFlowParserStringLiteral   = 49
	SyntaxFlowParserStringType      = 50
	SyntaxFlowParserListType        = 51
	SyntaxFlowParserDictType        = 52
	SyntaxFlowParserNumberType      = 53
	SyntaxFlowParserBoolType        = 54
	SyntaxFlowParserBoolLiteral     = 55
	SyntaxFlowParserIdentifier      = 56
	SyntaxFlowParserIdentifierChar  = 57
	SyntaxFlowParserRegexpLiteral   = 58
	SyntaxFlowParserWS              = 59
)

// SyntaxFlowParser rules.
const (
	SyntaxFlowParserRULE_flow                     = 0
	SyntaxFlowParserRULE_filters                  = 1
	SyntaxFlowParserRULE_filterStatement          = 2
	SyntaxFlowParserRULE_refVariable              = 3
	SyntaxFlowParserRULE_filterExpr               = 4
	SyntaxFlowParserRULE_acutalParamFilter        = 5
	SyntaxFlowParserRULE_recursiveConfig          = 6
	SyntaxFlowParserRULE_recursiveConfigItem      = 7
	SyntaxFlowParserRULE_recursiveConfigItemValue = 8
	SyntaxFlowParserRULE_sliceCallItem            = 9
	SyntaxFlowParserRULE_nameFilter               = 10
	SyntaxFlowParserRULE_chainFilter              = 11
	SyntaxFlowParserRULE_conditionExpression      = 12
	SyntaxFlowParserRULE_numberLiteral            = 13
	SyntaxFlowParserRULE_stringLiteral            = 14
	SyntaxFlowParserRULE_regexpLiteral            = 15
	SyntaxFlowParserRULE_identifier               = 16
	SyntaxFlowParserRULE_types                    = 17
	SyntaxFlowParserRULE_boolLiteral              = 18
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

func (s *FlowContext) Filters() IFiltersContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFiltersContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFiltersContext)
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
		p.SetState(38)
		p.Filters()
	}
	{
		p.SetState(39)
		p.Match(SyntaxFlowParserEOF)
	}

	return localctx
}

// IFiltersContext is an interface to support dynamic dispatch.
type IFiltersContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFiltersContext differentiates from other interfaces.
	IsFiltersContext()
}

type FiltersContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFiltersContext() *FiltersContext {
	var p = new(FiltersContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_filters
	return p
}

func (*FiltersContext) IsFiltersContext() {}

func NewFiltersContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FiltersContext {
	var p = new(FiltersContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_filters

	return p
}

func (s *FiltersContext) GetParser() antlr.Parser { return s.parser }

func (s *FiltersContext) AllFilterStatement() []IFilterStatementContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFilterStatementContext); ok {
			len++
		}
	}

	tst := make([]IFilterStatementContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFilterStatementContext); ok {
			tst[i] = t.(IFilterStatementContext)
			i++
		}
	}

	return tst
}

func (s *FiltersContext) FilterStatement(i int) IFilterStatementContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterStatementContext); ok {
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

	return t.(IFilterStatementContext)
}

func (s *FiltersContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FiltersContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FiltersContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilters(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) Filters() (localctx IFiltersContext) {
	this := p
	_ = this

	localctx = NewFiltersContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, SyntaxFlowParserRULE_filters)
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
	p.SetState(42)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&395201999890546688) != 0 {
		{
			p.SetState(41)
			p.FilterStatement()
		}

		p.SetState(44)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
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

func (s *FilterStatementContext) FilterExpr() IFilterExprContext {
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

func (s *FilterStatementContext) As() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserAs, 0)
}

func (s *FilterStatementContext) RefVariable() IRefVariableContext {
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

func (s *FilterStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FilterStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilterStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FilterStatement() (localctx IFilterStatementContext) {
	this := p
	_ = this

	localctx = NewFilterStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, SyntaxFlowParserRULE_filterStatement)

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
		p.SetState(46)
		p.filterExpr(0)
	}
	p.SetState(49)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 1, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(47)
			p.Match(SyntaxFlowParserAs)
		}
		{
			p.SetState(48)
			p.RefVariable()
		}

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
	p.EnterRule(localctx, 6, SyntaxFlowParserRULE_refVariable)

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
		p.SetState(51)
		p.Match(SyntaxFlowParserDollarOutput)
	}
	p.SetState(57)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserAs, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier:
		{
			p.SetState(52)
			p.Identifier()
		}

	case SyntaxFlowParserOpenParen:
		{
			p.SetState(53)
			p.Match(SyntaxFlowParserOpenParen)
		}
		{
			p.SetState(54)
			p.Identifier()
		}
		{
			p.SetState(55)
			p.Match(SyntaxFlowParserCloseParen)
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

func (s *FilterExprContext) CopyFrom(ctx *FilterExprContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *FilterExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type RegexpLiteralFilterContext struct {
	*FilterExprContext
}

func NewRegexpLiteralFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *RegexpLiteralFilterContext {
	var p = new(RegexpLiteralFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *RegexpLiteralFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RegexpLiteralFilterContext) RegexpLiteral() IRegexpLiteralContext {
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

func (s *RegexpLiteralFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitRegexpLiteralFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type FunctionCallFilterContext struct {
	*FilterExprContext
}

func NewFunctionCallFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FunctionCallFilterContext {
	var p = new(FunctionCallFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *FunctionCallFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FunctionCallFilterContext) FilterExpr() IFilterExprContext {
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

func (s *FunctionCallFilterContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserOpenParen, 0)
}

func (s *FunctionCallFilterContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCloseParen, 0)
}

func (s *FunctionCallFilterContext) AllAcutalParamFilter() []IAcutalParamFilterContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IAcutalParamFilterContext); ok {
			len++
		}
	}

	tst := make([]IAcutalParamFilterContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IAcutalParamFilterContext); ok {
			tst[i] = t.(IAcutalParamFilterContext)
			i++
		}
	}

	return tst
}

func (s *FunctionCallFilterContext) AcutalParamFilter(i int) IAcutalParamFilterContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAcutalParamFilterContext); ok {
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

	return t.(IAcutalParamFilterContext)
}

func (s *FunctionCallFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFunctionCallFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type CurrentRootFilterContext struct {
	*FilterExprContext
}

func NewCurrentRootFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *CurrentRootFilterContext {
	var p = new(CurrentRootFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *CurrentRootFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *CurrentRootFilterContext) DollarOutput() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDollarOutput, 0)
}

func (s *CurrentRootFilterContext) Identifier() IIdentifierContext {
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

func (s *CurrentRootFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitCurrentRootFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type NextFilterContext struct {
	*FilterExprContext
}

func NewNextFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *NextFilterContext {
	var p = new(NextFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *NextFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NextFilterContext) AllFilterExpr() []IFilterExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFilterExprContext); ok {
			len++
		}
	}

	tst := make([]IFilterExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFilterExprContext); ok {
			tst[i] = t.(IFilterExprContext)
			i++
		}
	}

	return tst
}

func (s *NextFilterContext) FilterExpr(i int) IFilterExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterExprContext); ok {
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

	return t.(IFilterExprContext)
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
	*FilterExprContext
}

func NewOptionalFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *OptionalFilterContext {
	var p = new(OptionalFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *OptionalFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OptionalFilterContext) FilterExpr() IFilterExprContext {
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

type PrimaryFilterContext struct {
	*FilterExprContext
}

func NewPrimaryFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *PrimaryFilterContext {
	var p = new(PrimaryFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *PrimaryFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PrimaryFilterContext) Identifier() IIdentifierContext {
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

func (s *PrimaryFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitPrimaryFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type TopDefFilterContext struct {
	*FilterExprContext
}

func NewTopDefFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *TopDefFilterContext {
	var p = new(TopDefFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *TopDefFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TopDefFilterContext) AllFilterExpr() []IFilterExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFilterExprContext); ok {
			len++
		}
	}

	tst := make([]IFilterExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFilterExprContext); ok {
			tst[i] = t.(IFilterExprContext)
			i++
		}
	}

	return tst
}

func (s *TopDefFilterContext) FilterExpr(i int) IFilterExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterExprContext); ok {
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

	return t.(IFilterExprContext)
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

type ConfiggedTopDefFilterContext struct {
	*FilterExprContext
}

func NewConfiggedTopDefFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ConfiggedTopDefFilterContext {
	var p = new(ConfiggedTopDefFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *ConfiggedTopDefFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ConfiggedTopDefFilterContext) AllFilterExpr() []IFilterExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFilterExprContext); ok {
			len++
		}
	}

	tst := make([]IFilterExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFilterExprContext); ok {
			tst[i] = t.(IFilterExprContext)
			i++
		}
	}

	return tst
}

func (s *ConfiggedTopDefFilterContext) FilterExpr(i int) IFilterExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterExprContext); ok {
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

	return t.(IFilterExprContext)
}

func (s *ConfiggedTopDefFilterContext) TopDefStart() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserTopDefStart, 0)
}

func (s *ConfiggedTopDefFilterContext) DeepNextEnd() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDeepNextEnd, 0)
}

func (s *ConfiggedTopDefFilterContext) RecursiveConfig() IRecursiveConfigContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRecursiveConfigContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRecursiveConfigContext)
}

func (s *ConfiggedTopDefFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitConfiggedTopDefFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type WildcardFilterContext struct {
	*FilterExprContext
}

func NewWildcardFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *WildcardFilterContext {
	var p = new(WildcardFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *WildcardFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *WildcardFilterContext) Star() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserStar, 0)
}

func (s *WildcardFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitWildcardFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type FieldIndexFilterContext struct {
	*FilterExprContext
}

func NewFieldIndexFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FieldIndexFilterContext {
	var p = new(FieldIndexFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *FieldIndexFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FieldIndexFilterContext) FilterExpr() IFilterExprContext {
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

type DefFilterContext struct {
	*FilterExprContext
}

func NewDefFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *DefFilterContext {
	var p = new(DefFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *DefFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DefFilterContext) AllFilterExpr() []IFilterExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFilterExprContext); ok {
			len++
		}
	}

	tst := make([]IFilterExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFilterExprContext); ok {
			tst[i] = t.(IFilterExprContext)
			i++
		}
	}

	return tst
}

func (s *DefFilterContext) FilterExpr(i int) IFilterExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterExprContext); ok {
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

	return t.(IFilterExprContext)
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

type FieldFilterContext struct {
	*FilterExprContext
}

func NewFieldFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FieldFilterContext {
	var p = new(FieldFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *FieldFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FieldFilterContext) Dot() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDot, 0)
}

func (s *FieldFilterContext) NameFilter() INameFilterContext {
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

func (s *FieldFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFieldFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type FieldCallFilterContext struct {
	*FilterExprContext
}

func NewFieldCallFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FieldCallFilterContext {
	var p = new(FieldCallFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *FieldCallFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FieldCallFilterContext) FilterExpr() IFilterExprContext {
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

func (s *FieldCallFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFieldCallFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type DeepNextFilterContext struct {
	*FilterExprContext
}

func NewDeepNextFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *DeepNextFilterContext {
	var p = new(DeepNextFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *DeepNextFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DeepNextFilterContext) AllFilterExpr() []IFilterExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFilterExprContext); ok {
			len++
		}
	}

	tst := make([]IFilterExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFilterExprContext); ok {
			tst[i] = t.(IFilterExprContext)
			i++
		}
	}

	return tst
}

func (s *DeepNextFilterContext) FilterExpr(i int) IFilterExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterExprContext); ok {
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

	return t.(IFilterExprContext)
}

func (s *DeepNextFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitDeepNextFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

type ConfiggedDeepNextFilterContext struct {
	*FilterExprContext
}

func NewConfiggedDeepNextFilterContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ConfiggedDeepNextFilterContext {
	var p = new(ConfiggedDeepNextFilterContext)

	p.FilterExprContext = NewEmptyFilterExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*FilterExprContext))

	return p
}

func (s *ConfiggedDeepNextFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ConfiggedDeepNextFilterContext) AllFilterExpr() []IFilterExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFilterExprContext); ok {
			len++
		}
	}

	tst := make([]IFilterExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFilterExprContext); ok {
			tst[i] = t.(IFilterExprContext)
			i++
		}
	}

	return tst
}

func (s *ConfiggedDeepNextFilterContext) FilterExpr(i int) IFilterExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFilterExprContext); ok {
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

	return t.(IFilterExprContext)
}

func (s *ConfiggedDeepNextFilterContext) DeepNextStart() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDeepNextStart, 0)
}

func (s *ConfiggedDeepNextFilterContext) DeepNextEnd() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDeepNextEnd, 0)
}

func (s *ConfiggedDeepNextFilterContext) RecursiveConfig() IRecursiveConfigContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRecursiveConfigContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRecursiveConfigContext)
}

func (s *ConfiggedDeepNextFilterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitConfiggedDeepNextFilter(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) FilterExpr() (localctx IFilterExprContext) {
	return p.filterExpr(0)
}

func (p *SyntaxFlowParser) filterExpr(_p int) (localctx IFilterExprContext) {
	this := p
	_ = this

	var _parentctx antlr.ParserRuleContext = p.GetParserRuleContext()
	_parentState := p.GetState()
	localctx = NewFilterExprContext(p, p.GetParserRuleContext(), _parentState)
	var _prevctx IFilterExprContext = localctx
	var _ antlr.ParserRuleContext = _prevctx // TODO: To prevent unused variable warning.
	_startState := 8
	p.EnterRecursionRule(localctx, 8, SyntaxFlowParserRULE_filterExpr, _p)
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
	p.SetState(69)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserDollarOutput:
		localctx = NewCurrentRootFilterContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx

		{
			p.SetState(60)
			p.Match(SyntaxFlowParserDollarOutput)
		}
		p.SetState(62)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 3, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(61)
				p.Identifier()
			}

		}

	case SyntaxFlowParserStar:
		localctx = NewWildcardFilterContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(64)
			p.Match(SyntaxFlowParserStar)
		}

	case SyntaxFlowParserAs, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier:
		localctx = NewPrimaryFilterContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(65)
			p.Identifier()
		}

	case SyntaxFlowParserRegexpLiteral:
		localctx = NewRegexpLiteralFilterContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(66)
			p.RegexpLiteral()
		}

	case SyntaxFlowParserDot:
		localctx = NewFieldFilterContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(67)
			p.Match(SyntaxFlowParserDot)
		}
		{
			p.SetState(68)
			p.NameFilter()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}
	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(121)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 9, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			p.SetState(119)
			p.GetErrorHandler().Sync(p)
			switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 8, p.GetParserRuleContext()) {
			case 1:
				localctx = NewNextFilterContext(p, NewFilterExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_filterExpr)
				p.SetState(71)

				if !(p.Precpred(p.GetParserRuleContext(), 6)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 6)", ""))
				}
				{
					p.SetState(72)
					p.Match(SyntaxFlowParserT__0)
				}
				{
					p.SetState(73)
					p.filterExpr(7)
				}

			case 2:
				localctx = NewDefFilterContext(p, NewFilterExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_filterExpr)
				p.SetState(74)

				if !(p.Precpred(p.GetParserRuleContext(), 5)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 5)", ""))
				}
				{
					p.SetState(75)
					p.Match(SyntaxFlowParserDefStart)
				}
				{
					p.SetState(76)
					p.filterExpr(6)
				}

			case 3:
				localctx = NewDeepNextFilterContext(p, NewFilterExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_filterExpr)
				p.SetState(77)

				if !(p.Precpred(p.GetParserRuleContext(), 4)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 4)", ""))
				}
				{
					p.SetState(78)
					p.Match(SyntaxFlowParserT__1)
				}
				{
					p.SetState(79)
					p.filterExpr(5)
				}

			case 4:
				localctx = NewTopDefFilterContext(p, NewFilterExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_filterExpr)
				p.SetState(80)

				if !(p.Precpred(p.GetParserRuleContext(), 3)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 3)", ""))
				}
				{
					p.SetState(81)
					p.Match(SyntaxFlowParserTopDef)
				}
				{
					p.SetState(82)
					p.filterExpr(4)
				}

			case 5:
				localctx = NewConfiggedDeepNextFilterContext(p, NewFilterExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_filterExpr)
				p.SetState(83)

				if !(p.Precpred(p.GetParserRuleContext(), 2)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 2)", ""))
				}
				{
					p.SetState(84)
					p.Match(SyntaxFlowParserDeepNextStart)
				}
				p.SetState(86)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)

				if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&106969287243071488) != 0 {
					{
						p.SetState(85)
						p.RecursiveConfig()
					}

				}
				{
					p.SetState(88)
					p.Match(SyntaxFlowParserDeepNextEnd)
				}
				{
					p.SetState(89)
					p.filterExpr(3)
				}

			case 6:
				localctx = NewConfiggedTopDefFilterContext(p, NewFilterExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_filterExpr)
				p.SetState(90)

				if !(p.Precpred(p.GetParserRuleContext(), 1)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 1)", ""))
				}
				{
					p.SetState(91)
					p.Match(SyntaxFlowParserTopDefStart)
				}
				p.SetState(93)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)

				if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&106969287243071488) != 0 {
					{
						p.SetState(92)
						p.RecursiveConfig()
					}

				}
				{
					p.SetState(95)
					p.Match(SyntaxFlowParserDeepNextEnd)
				}
				{
					p.SetState(96)
					p.filterExpr(2)
				}

			case 7:
				localctx = NewFieldCallFilterContext(p, NewFilterExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_filterExpr)
				p.SetState(97)

				if !(p.Precpred(p.GetParserRuleContext(), 10)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 10)", ""))
				}
				{
					p.SetState(98)
					p.Match(SyntaxFlowParserDot)
				}
				{
					p.SetState(99)
					p.NameFilter()
				}

			case 8:
				localctx = NewFunctionCallFilterContext(p, NewFilterExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_filterExpr)
				p.SetState(100)

				if !(p.Precpred(p.GetParserRuleContext(), 9)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 9)", ""))
				}
				{
					p.SetState(101)
					p.Match(SyntaxFlowParserOpenParen)
				}
				p.SetState(105)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)

				for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&395202000970579968) != 0 {
					{
						p.SetState(102)
						p.AcutalParamFilter()
					}

					p.SetState(107)
					p.GetErrorHandler().Sync(p)
					_la = p.GetTokenStream().LA(1)
				}
				{
					p.SetState(108)
					p.Match(SyntaxFlowParserCloseParen)
				}

			case 9:
				localctx = NewFieldIndexFilterContext(p, NewFilterExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_filterExpr)
				p.SetState(109)

				if !(p.Precpred(p.GetParserRuleContext(), 8)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 8)", ""))
				}
				{
					p.SetState(110)
					p.Match(SyntaxFlowParserListSelectOpen)
				}
				{
					p.SetState(111)
					p.SliceCallItem()
				}
				{
					p.SetState(112)
					p.Match(SyntaxFlowParserListSelectClose)
				}

			case 10:
				localctx = NewOptionalFilterContext(p, NewFilterExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_filterExpr)
				p.SetState(114)

				if !(p.Precpred(p.GetParserRuleContext(), 7)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 7)", ""))
				}
				{
					p.SetState(115)
					p.Match(SyntaxFlowParserConditionStart)
				}
				{
					p.SetState(116)
					p.conditionExpression(0)
				}
				{
					p.SetState(117)
					p.Match(SyntaxFlowParserMapBuilderClose)
				}

			}

		}
		p.SetState(123)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 9, p.GetParserRuleContext())
	}

	return localctx
}

// IAcutalParamFilterContext is an interface to support dynamic dispatch.
type IAcutalParamFilterContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsAcutalParamFilterContext differentiates from other interfaces.
	IsAcutalParamFilterContext()
}

type AcutalParamFilterContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAcutalParamFilterContext() *AcutalParamFilterContext {
	var p = new(AcutalParamFilterContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_acutalParamFilter
	return p
}

func (*AcutalParamFilterContext) IsAcutalParamFilterContext() {}

func NewAcutalParamFilterContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AcutalParamFilterContext {
	var p = new(AcutalParamFilterContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_acutalParamFilter

	return p
}

func (s *AcutalParamFilterContext) GetParser() antlr.Parser { return s.parser }

func (s *AcutalParamFilterContext) CopyFrom(ctx *AcutalParamFilterContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *AcutalParamFilterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AcutalParamFilterContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type EmptyParamContext struct {
	*AcutalParamFilterContext
}

func NewEmptyParamContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *EmptyParamContext {
	var p = new(EmptyParamContext)

	p.AcutalParamFilterContext = NewEmptyAcutalParamFilterContext()
	p.parser = parser
	p.CopyFrom(ctx.(*AcutalParamFilterContext))

	return p
}

func (s *EmptyParamContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *EmptyParamContext) Comma() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserComma, 0)
}

func (s *EmptyParamContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitEmptyParam(s)

	default:
		return t.VisitChildren(s)
	}
}

type NamedParamContext struct {
	*AcutalParamFilterContext
}

func NewNamedParamContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *NamedParamContext {
	var p = new(NamedParamContext)

	p.AcutalParamFilterContext = NewEmptyAcutalParamFilterContext()
	p.parser = parser
	p.CopyFrom(ctx.(*AcutalParamFilterContext))

	return p
}

func (s *NamedParamContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NamedParamContext) FilterStatement() IFilterStatementContext {
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

func (s *NamedParamContext) DefStart() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserDefStart, 0)
}

func (s *NamedParamContext) TopDefStart() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserTopDefStart, 0)
}

func (s *NamedParamContext) MapBuilderClose() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserMapBuilderClose, 0)
}

func (s *NamedParamContext) Comma() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserComma, 0)
}

func (s *NamedParamContext) RecursiveConfig() IRecursiveConfigContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRecursiveConfigContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRecursiveConfigContext)
}

func (s *NamedParamContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitNamedParam(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) AcutalParamFilter() (localctx IAcutalParamFilterContext) {
	this := p
	_ = this

	localctx = NewAcutalParamFilterContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, SyntaxFlowParserRULE_acutalParamFilter)
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

	p.SetState(137)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserTopDefStart, SyntaxFlowParserDefStart, SyntaxFlowParserDot, SyntaxFlowParserDollarOutput, SyntaxFlowParserStar, SyntaxFlowParserAs, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier, SyntaxFlowParserRegexpLiteral:
		localctx = NewNamedParamContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		p.SetState(130)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserDefStart:
			{
				p.SetState(124)
				p.Match(SyntaxFlowParserDefStart)
			}

		case SyntaxFlowParserTopDefStart:
			{
				p.SetState(125)
				p.Match(SyntaxFlowParserTopDefStart)
			}
			p.SetState(127)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&106969287243071488) != 0 {
				{
					p.SetState(126)
					p.RecursiveConfig()
				}

			}
			{
				p.SetState(129)
				p.Match(SyntaxFlowParserMapBuilderClose)
			}

		case SyntaxFlowParserDot, SyntaxFlowParserDollarOutput, SyntaxFlowParserStar, SyntaxFlowParserAs, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier, SyntaxFlowParserRegexpLiteral:

		default:
		}
		{
			p.SetState(132)
			p.FilterStatement()
		}
		p.SetState(134)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 12, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(133)
				p.Match(SyntaxFlowParserComma)
			}

		}

	case SyntaxFlowParserComma:
		localctx = NewEmptyParamContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(136)
			p.Match(SyntaxFlowParserComma)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IRecursiveConfigContext is an interface to support dynamic dispatch.
type IRecursiveConfigContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsRecursiveConfigContext differentiates from other interfaces.
	IsRecursiveConfigContext()
}

type RecursiveConfigContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyRecursiveConfigContext() *RecursiveConfigContext {
	var p = new(RecursiveConfigContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SyntaxFlowParserRULE_recursiveConfig
	return p
}

func (*RecursiveConfigContext) IsRecursiveConfigContext() {}

func NewRecursiveConfigContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *RecursiveConfigContext {
	var p = new(RecursiveConfigContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SyntaxFlowParserRULE_recursiveConfig

	return p
}

func (s *RecursiveConfigContext) GetParser() antlr.Parser { return s.parser }

func (s *RecursiveConfigContext) AllRecursiveConfigItem() []IRecursiveConfigItemContext {
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

func (s *RecursiveConfigContext) RecursiveConfigItem(i int) IRecursiveConfigItemContext {
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

func (s *RecursiveConfigContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SyntaxFlowParserComma)
}

func (s *RecursiveConfigContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserComma, i)
}

func (s *RecursiveConfigContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RecursiveConfigContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *RecursiveConfigContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitRecursiveConfig(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SyntaxFlowParser) RecursiveConfig() (localctx IRecursiveConfigContext) {
	this := p
	_ = this

	localctx = NewRecursiveConfigContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, SyntaxFlowParserRULE_recursiveConfig)
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
		p.SetState(139)
		p.RecursiveConfigItem()
	}
	p.SetState(144)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 14, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(140)
				p.Match(SyntaxFlowParserComma)
			}
			{
				p.SetState(141)
				p.RecursiveConfigItem()
			}

		}
		p.SetState(146)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 14, p.GetParserRuleContext())
	}
	p.SetState(148)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SyntaxFlowParserComma {
		{
			p.SetState(147)
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
	p.EnterRule(localctx, 14, SyntaxFlowParserRULE_recursiveConfigItem)

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
		p.SetState(150)
		p.Identifier()
	}
	{
		p.SetState(151)
		p.Match(SyntaxFlowParserColon)
	}
	{
		p.SetState(152)
		p.RecursiveConfigItemValue()
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

func (s *RecursiveConfigItemValueContext) Search() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserSearch, 0)
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
	p.EnterRule(localctx, 16, SyntaxFlowParserRULE_recursiveConfigItemValue)

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

	p.SetState(160)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserAs, SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier:
		p.EnterOuterAlt(localctx, 1)
		p.SetState(156)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserAs, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier:
			{
				p.SetState(154)
				p.Identifier()
			}

		case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
			{
				p.SetState(155)
				p.NumberLiteral()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

	case SyntaxFlowParserSearch:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(158)
			p.Match(SyntaxFlowParserSearch)
		}
		{
			p.SetState(159)
			p.FilterStatement()
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
	p.EnterRule(localctx, 18, SyntaxFlowParserRULE_sliceCallItem)

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

	p.SetState(164)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserAs, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier, SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(162)
			p.NameFilter()
		}

	case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(163)
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
	p.EnterRule(localctx, 20, SyntaxFlowParserRULE_nameFilter)

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

	p.SetState(168)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserAs, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(166)
			p.Identifier()
		}

	case SyntaxFlowParserRegexpLiteral:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(167)
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

func (s *FlatContext) AllFilters() []IFiltersContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFiltersContext); ok {
			len++
		}
	}

	tst := make([]IFiltersContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFiltersContext); ok {
			tst[i] = t.(IFiltersContext)
			i++
		}
	}

	return tst
}

func (s *FlatContext) Filters(i int) IFiltersContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFiltersContext); ok {
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

	return t.(IFiltersContext)
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

func (s *BuildMapContext) AllFilters() []IFiltersContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFiltersContext); ok {
			len++
		}
	}

	tst := make([]IFiltersContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFiltersContext); ok {
			tst[i] = t.(IFiltersContext)
			i++
		}
	}

	return tst
}

func (s *BuildMapContext) Filters(i int) IFiltersContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFiltersContext); ok {
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

	return t.(IFiltersContext)
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
	p.EnterRule(localctx, 22, SyntaxFlowParserRULE_chainFilter)
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

	p.SetState(205)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserListSelectOpen:
		localctx = NewFlatContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(170)
			p.Match(SyntaxFlowParserListSelectOpen)
		}
		p.SetState(180)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserDot, SyntaxFlowParserDollarOutput, SyntaxFlowParserStar, SyntaxFlowParserAs, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier, SyntaxFlowParserRegexpLiteral:
			{
				p.SetState(171)
				p.Filters()
			}
			p.SetState(176)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			for _la == SyntaxFlowParserComma {
				{
					p.SetState(172)
					p.Match(SyntaxFlowParserComma)
				}
				{
					p.SetState(173)
					p.Filters()
				}

				p.SetState(178)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)
			}

		case SyntaxFlowParserDeep:
			{
				p.SetState(179)
				p.Match(SyntaxFlowParserDeep)
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}
		{
			p.SetState(182)
			p.Match(SyntaxFlowParserListSelectClose)
		}

	case SyntaxFlowParserMapBuilderOpen:
		localctx = NewBuildMapContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(183)
			p.Match(SyntaxFlowParserMapBuilderOpen)
		}
		p.SetState(199)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&106969287243071488) != 0 {
			{
				p.SetState(184)
				p.Identifier()
			}
			{
				p.SetState(185)
				p.Match(SyntaxFlowParserColon)
			}

			{
				p.SetState(187)
				p.Filters()
			}
			p.SetState(196)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 22, p.GetParserRuleContext())

			for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
				if _alt == 1 {
					{
						p.SetState(188)
						p.Match(SyntaxFlowParserT__2)
					}

					{
						p.SetState(189)
						p.Identifier()
					}
					{
						p.SetState(190)
						p.Match(SyntaxFlowParserColon)
					}

					{
						p.SetState(192)
						p.Filters()
					}

				}
				p.SetState(198)
				p.GetErrorHandler().Sync(p)
				_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 22, p.GetParserRuleContext())
			}

		}
		p.SetState(202)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SyntaxFlowParserT__2 {
			{
				p.SetState(201)
				p.Match(SyntaxFlowParserT__2)
			}

		}
		{
			p.SetState(204)
			p.Match(SyntaxFlowParserMapBuilderClose)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
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

type FilterExpressionStringContext struct {
	*ConditionExpressionContext
}

func NewFilterExpressionStringContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FilterExpressionStringContext {
	var p = new(FilterExpressionStringContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *FilterExpressionStringContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterExpressionStringContext) StringLiteral() IStringLiteralContext {
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

func (s *FilterExpressionStringContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilterExpressionString(s)

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

type FilterExpressionParenContext struct {
	*ConditionExpressionContext
}

func NewFilterExpressionParenContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FilterExpressionParenContext {
	var p = new(FilterExpressionParenContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *FilterExpressionParenContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterExpressionParenContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserOpenParen, 0)
}

func (s *FilterExpressionParenContext) ConditionExpression() IConditionExpressionContext {
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

func (s *FilterExpressionParenContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserCloseParen, 0)
}

func (s *FilterExpressionParenContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilterExpressionParen(s)

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

type FilterExpressionNumberContext struct {
	*ConditionExpressionContext
}

func NewFilterExpressionNumberContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FilterExpressionNumberContext {
	var p = new(FilterExpressionNumberContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *FilterExpressionNumberContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterExpressionNumberContext) NumberLiteral() INumberLiteralContext {
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

func (s *FilterExpressionNumberContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilterExpressionNumber(s)

	default:
		return t.VisitChildren(s)
	}
}

type FilterExpressionRegexpContext struct {
	*ConditionExpressionContext
}

func NewFilterExpressionRegexpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FilterExpressionRegexpContext {
	var p = new(FilterExpressionRegexpContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *FilterExpressionRegexpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterExpressionRegexpContext) RegexpLiteral() IRegexpLiteralContext {
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

func (s *FilterExpressionRegexpContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilterExpressionRegexp(s)

	default:
		return t.VisitChildren(s)
	}
}

type FilterExpressionNotContext struct {
	*ConditionExpressionContext
}

func NewFilterExpressionNotContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *FilterExpressionNotContext {
	var p = new(FilterExpressionNotContext)

	p.ConditionExpressionContext = NewEmptyConditionExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ConditionExpressionContext))

	return p
}

func (s *FilterExpressionNotContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FilterExpressionNotContext) Bang() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserBang, 0)
}

func (s *FilterExpressionNotContext) ConditionExpression() IConditionExpressionContext {
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

func (s *FilterExpressionNotContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SyntaxFlowVisitor:
		return t.VisitFilterExpressionNot(s)

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
	_startState := 24
	p.EnterRecursionRule(localctx, 24, SyntaxFlowParserRULE_conditionExpression, _p)
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
	p.SetState(228)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
		localctx = NewFilterExpressionNumberContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx

		{
			p.SetState(208)
			p.NumberLiteral()
		}

	case SyntaxFlowParserStar, SyntaxFlowParserAs, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier:
		localctx = NewFilterExpressionStringContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(209)
			p.StringLiteral()
		}

	case SyntaxFlowParserRegexpLiteral:
		localctx = NewFilterExpressionRegexpContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(210)
			p.RegexpLiteral()
		}

	case SyntaxFlowParserOpenParen:
		localctx = NewFilterExpressionParenContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(211)
			p.Match(SyntaxFlowParserOpenParen)
		}
		{
			p.SetState(212)
			p.conditionExpression(0)
		}
		{
			p.SetState(213)
			p.Match(SyntaxFlowParserCloseParen)
		}

	case SyntaxFlowParserBang:
		localctx = NewFilterExpressionNotContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(215)
			p.Match(SyntaxFlowParserBang)
		}
		{
			p.SetState(216)
			p.conditionExpression(5)
		}

	case SyntaxFlowParserLtEq, SyntaxFlowParserGtEq, SyntaxFlowParserEqEq, SyntaxFlowParserNotEq, SyntaxFlowParserGt, SyntaxFlowParserLt, SyntaxFlowParserEq:
		localctx = NewFilterExpressionCompareContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(217)

			var _lt = p.GetTokenStream().LT(1)

			localctx.(*FilterExpressionCompareContext).op = _lt

			_la = p.GetTokenStream().LA(1)

			if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&218239744) != 0) {
				var _ri = p.GetErrorHandler().RecoverInline(p)

				localctx.(*FilterExpressionCompareContext).op = _ri
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}
		p.SetState(221)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserNumber, SyntaxFlowParserOctalNumber, SyntaxFlowParserBinaryNumber, SyntaxFlowParserHexNumber:
			{
				p.SetState(218)
				p.NumberLiteral()
			}

		case SyntaxFlowParserAs, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier:
			{
				p.SetState(219)
				p.Identifier()
			}

		case SyntaxFlowParserBoolLiteral:
			{
				p.SetState(220)
				p.BoolLiteral()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

	case SyntaxFlowParserRegexpMatch, SyntaxFlowParserNotRegexpMatch:
		localctx = NewFilterExpressionRegexpMatchContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(223)

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
		p.SetState(226)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case SyntaxFlowParserStar, SyntaxFlowParserAs, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier:
			{
				p.SetState(224)
				p.StringLiteral()
			}

		case SyntaxFlowParserRegexpLiteral:
			{
				p.SetState(225)
				p.RegexpLiteral()
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}
	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(238)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 30, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			p.SetState(236)
			p.GetErrorHandler().Sync(p)
			switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 29, p.GetParserRuleContext()) {
			case 1:
				localctx = NewFilterExpressionAndContext(p, NewConditionExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_conditionExpression)
				p.SetState(230)

				if !(p.Precpred(p.GetParserRuleContext(), 2)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 2)", ""))
				}
				{
					p.SetState(231)
					p.Match(SyntaxFlowParserAnd)
				}
				{
					p.SetState(232)
					p.conditionExpression(3)
				}

			case 2:
				localctx = NewFilterExpressionOrContext(p, NewConditionExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, SyntaxFlowParserRULE_conditionExpression)
				p.SetState(233)

				if !(p.Precpred(p.GetParserRuleContext(), 1)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 1)", ""))
				}
				{
					p.SetState(234)
					p.Match(SyntaxFlowParserOr)
				}
				{
					p.SetState(235)
					p.conditionExpression(2)
				}

			}

		}
		p.SetState(240)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 30, p.GetParserRuleContext())
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
	p.EnterRule(localctx, 26, SyntaxFlowParserRULE_numberLiteral)
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
		p.SetState(241)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&527765581332480) != 0) {
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
	p.EnterRule(localctx, 28, SyntaxFlowParserRULE_stringLiteral)

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

	p.SetState(245)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserAs, SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType, SyntaxFlowParserIdentifier:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(243)
			p.Identifier()
		}

	case SyntaxFlowParserStar:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(244)
			p.Match(SyntaxFlowParserStar)
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
	p.EnterRule(localctx, 30, SyntaxFlowParserRULE_regexpLiteral)

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
		p.SetState(247)
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

func (s *IdentifierContext) Types() ITypesContext {
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

func (s *IdentifierContext) As() antlr.TerminalNode {
	return s.GetToken(SyntaxFlowParserAs, 0)
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
	p.EnterRule(localctx, 32, SyntaxFlowParserRULE_identifier)

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

	p.SetState(252)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SyntaxFlowParserIdentifier:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(249)
			p.Match(SyntaxFlowParserIdentifier)
		}

	case SyntaxFlowParserStringType, SyntaxFlowParserListType, SyntaxFlowParserDictType, SyntaxFlowParserNumberType, SyntaxFlowParserBoolType:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(250)
			p.Types()
		}

	case SyntaxFlowParserAs:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(251)
			p.Match(SyntaxFlowParserAs)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
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
	p.EnterRule(localctx, 34, SyntaxFlowParserRULE_types)
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
		p.SetState(254)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&34902897112121344) != 0) {
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
	p.EnterRule(localctx, 36, SyntaxFlowParserRULE_boolLiteral)

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
		p.SetState(256)
		p.Match(SyntaxFlowParserBoolLiteral)
	}

	return localctx
}

func (p *SyntaxFlowParser) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 4:
		var t *FilterExprContext = nil
		if localctx != nil {
			t = localctx.(*FilterExprContext)
		}
		return p.FilterExpr_Sempred(t, predIndex)

	case 12:
		var t *ConditionExpressionContext = nil
		if localctx != nil {
			t = localctx.(*ConditionExpressionContext)
		}
		return p.ConditionExpression_Sempred(t, predIndex)

	default:
		panic("No predicate with index: " + fmt.Sprint(ruleIndex))
	}
}

func (p *SyntaxFlowParser) FilterExpr_Sempred(localctx antlr.RuleContext, predIndex int) bool {
	this := p
	_ = this

	switch predIndex {
	case 0:
		return p.Precpred(p.GetParserRuleContext(), 6)

	case 1:
		return p.Precpred(p.GetParserRuleContext(), 5)

	case 2:
		return p.Precpred(p.GetParserRuleContext(), 4)

	case 3:
		return p.Precpred(p.GetParserRuleContext(), 3)

	case 4:
		return p.Precpred(p.GetParserRuleContext(), 2)

	case 5:
		return p.Precpred(p.GetParserRuleContext(), 1)

	case 6:
		return p.Precpred(p.GetParserRuleContext(), 10)

	case 7:
		return p.Precpred(p.GetParserRuleContext(), 9)

	case 8:
		return p.Precpred(p.GetParserRuleContext(), 8)

	case 9:
		return p.Precpred(p.GetParserRuleContext(), 7)

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}

func (p *SyntaxFlowParser) ConditionExpression_Sempred(localctx antlr.RuleContext, predIndex int) bool {
	this := p
	_ = this

	switch predIndex {
	case 10:
		return p.Precpred(p.GetParserRuleContext(), 2)

	case 11:
		return p.Precpred(p.GetParserRuleContext(), 1)

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}
