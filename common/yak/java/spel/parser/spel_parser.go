// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package spelparser // SpelParser
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

type SpelParser struct {
	*antlr.BaseParser
}

var spelparserParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	literalNames           []string
	symbolicNames          []string
	ruleNames              []string
	predictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func spelparserParserInit() {
	staticData := &spelparserParserStaticData
	staticData.literalNames = []string{
		"", "';'", "", "'++'", "'+'", "'--'", "'-'", "':'", "'.'", "','", "'*'",
		"'/'", "'%'", "'('", "')'", "'['", "']'", "'#'", "'@'", "'^['", "'^'",
		"'!='", "'!['", "'!'", "'=='", "'='", "'&&'", "'&'", "'||'", "'?['",
		"'?:'", "'?.'", "'?'", "'$['", "'>='", "'>'", "'<='", "'<'", "", "",
		"", "'or'", "'and'", "'true'", "'false'", "'new'", "'null'", "'T'",
		"'matches'", "'gt'", "'ge'", "'le'", "'lt'", "'eq'", "'ne'", "", "",
		"", "", "", "", "", "'``'",
	}
	staticData.symbolicNames = []string{
		"", "SEMICOLON", "WS", "INC", "PLUS", "DEC", "MINUS", "COLON", "DOT",
		"COMMA", "STAR", "DIV", "MOD", "LPAREN", "RPAREN", "LSQUARE", "RSQUARE",
		"HASH", "BEAN_REF", "SELECT_FIRST", "POWER", "NE", "PROJECT", "NOT",
		"EQ", "ASSIGN", "SYMBOLIC_AND", "FACTORY_BEAN_REF", "SYMBOLIC_OR", "SELECT",
		"ELVIS", "SAFE_NAVI", "QMARK", "SELECT_LAST", "GE", "GT", "LE", "LT",
		"LCURLY", "RCURLY", "BACKTICK", "OR", "AND", "TRUE", "FALSE", "NEW",
		"NULL", "T", "MATCHES", "GT_KEYWORD", "GE_KEYWORD", "LE_KEYWORD", "LT_KEYWORD",
		"EQ_KEYWORD", "NE_KEYWORD", "IDENTIFIER", "REAL_LITERAL", "INTEGER_LITERAL",
		"STRING_LITERAL", "SINGLE_QUOTED_STRING", "DOUBLE_QUOTED_STRING", "PROPERTY_PLACE_HOLDER",
		"ESCAPED_BACKTICK", "SPEL_IN_TEMPLATE_STRING_OPEN", "TEMPLATE_TEXT",
	}
	staticData.ruleNames = []string{
		"script", "spelExpr", "node", "nonDottedNode", "dottedNode", "functionOrVar",
		"methodArgs", "args", "methodOrProperty", "projection", "selection",
		"startNode", "literal", "numericLiteral", "parenspelExpr", "typeReference",
		"possiblyQualifiedId", "nullReference", "constructorReference", "constructorArgs",
		"inlineListOrMap", "listBindings", "listBinding", "mapBindings", "mapBinding",
		"beanReference", "inputParameter", "propertyPlaceHolder",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 64, 284, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7, 20, 2,
		21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25, 2, 26,
		7, 26, 2, 27, 7, 27, 1, 0, 1, 0, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 5,
		1, 65, 8, 1, 10, 1, 12, 1, 68, 9, 1, 3, 1, 70, 8, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 5, 1, 107, 8, 1, 10, 1,
		12, 1, 110, 9, 1, 1, 2, 1, 2, 3, 2, 114, 8, 2, 1, 3, 1, 3, 1, 3, 1, 3,
		1, 3, 1, 3, 3, 3, 122, 8, 3, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 3, 4, 129, 8,
		4, 1, 5, 1, 5, 1, 5, 1, 5, 1, 5, 3, 5, 136, 8, 5, 1, 6, 1, 6, 1, 6, 1,
		6, 1, 7, 3, 7, 143, 8, 7, 1, 7, 1, 7, 5, 7, 147, 8, 7, 10, 7, 12, 7, 150,
		9, 7, 1, 8, 1, 8, 1, 8, 3, 8, 155, 8, 8, 1, 9, 1, 9, 1, 9, 1, 9, 1, 10,
		1, 10, 1, 10, 1, 10, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1,
		11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 3, 11, 178, 8, 11, 1, 12, 1, 12,
		1, 12, 1, 12, 3, 12, 184, 8, 12, 1, 13, 1, 13, 1, 14, 1, 14, 1, 14, 1,
		14, 1, 15, 1, 15, 1, 15, 1, 15, 1, 15, 5, 15, 197, 8, 15, 10, 15, 12, 15,
		200, 9, 15, 1, 15, 1, 15, 1, 16, 1, 16, 1, 16, 5, 16, 207, 8, 16, 10, 16,
		12, 16, 210, 9, 16, 1, 17, 1, 17, 1, 18, 1, 18, 1, 18, 1, 18, 3, 18, 218,
		8, 18, 1, 18, 4, 18, 221, 8, 18, 11, 18, 12, 18, 222, 1, 18, 3, 18, 226,
		8, 18, 1, 18, 1, 18, 1, 18, 1, 18, 3, 18, 232, 8, 18, 1, 19, 1, 19, 1,
		19, 1, 19, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20,
		1, 20, 1, 20, 1, 20, 1, 20, 3, 20, 251, 8, 20, 1, 21, 1, 21, 1, 21, 5,
		21, 256, 8, 21, 10, 21, 12, 21, 259, 9, 21, 1, 22, 1, 22, 1, 23, 1, 23,
		1, 23, 5, 23, 266, 8, 23, 10, 23, 12, 23, 269, 9, 23, 1, 24, 1, 24, 1,
		24, 1, 24, 1, 25, 1, 25, 1, 25, 1, 26, 1, 26, 1, 26, 1, 26, 1, 27, 1, 27,
		1, 27, 0, 1, 2, 28, 0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26,
		28, 30, 32, 34, 36, 38, 40, 42, 44, 46, 48, 50, 52, 54, 0, 12, 2, 0, 3,
		6, 23, 23, 1, 0, 10, 12, 2, 0, 4, 4, 6, 6, 4, 0, 21, 21, 24, 24, 34, 37,
		49, 54, 2, 0, 26, 26, 42, 42, 2, 0, 28, 28, 41, 41, 2, 0, 3, 3, 5, 5, 2,
		0, 8, 8, 31, 31, 3, 0, 19, 19, 29, 29, 33, 33, 1, 0, 56, 57, 2, 0, 18,
		18, 27, 27, 2, 0, 55, 55, 58, 58, 304, 0, 56, 1, 0, 0, 0, 2, 69, 1, 0,
		0, 0, 4, 113, 1, 0, 0, 0, 6, 121, 1, 0, 0, 0, 8, 128, 1, 0, 0, 0, 10, 135,
		1, 0, 0, 0, 12, 137, 1, 0, 0, 0, 14, 142, 1, 0, 0, 0, 16, 154, 1, 0, 0,
		0, 18, 156, 1, 0, 0, 0, 20, 160, 1, 0, 0, 0, 22, 177, 1, 0, 0, 0, 24, 183,
		1, 0, 0, 0, 26, 185, 1, 0, 0, 0, 28, 187, 1, 0, 0, 0, 30, 191, 1, 0, 0,
		0, 32, 203, 1, 0, 0, 0, 34, 211, 1, 0, 0, 0, 36, 231, 1, 0, 0, 0, 38, 233,
		1, 0, 0, 0, 40, 250, 1, 0, 0, 0, 42, 252, 1, 0, 0, 0, 44, 260, 1, 0, 0,
		0, 46, 262, 1, 0, 0, 0, 48, 270, 1, 0, 0, 0, 50, 274, 1, 0, 0, 0, 52, 277,
		1, 0, 0, 0, 54, 281, 1, 0, 0, 0, 56, 57, 3, 2, 1, 0, 57, 58, 5, 0, 0, 1,
		58, 1, 1, 0, 0, 0, 59, 60, 6, 1, -1, 0, 60, 61, 7, 0, 0, 0, 61, 70, 3,
		2, 1, 13, 62, 66, 3, 22, 11, 0, 63, 65, 3, 4, 2, 0, 64, 63, 1, 0, 0, 0,
		65, 68, 1, 0, 0, 0, 66, 64, 1, 0, 0, 0, 66, 67, 1, 0, 0, 0, 67, 70, 1,
		0, 0, 0, 68, 66, 1, 0, 0, 0, 69, 59, 1, 0, 0, 0, 69, 62, 1, 0, 0, 0, 70,
		108, 1, 0, 0, 0, 71, 72, 10, 11, 0, 0, 72, 73, 5, 20, 0, 0, 73, 107, 3,
		2, 1, 12, 74, 75, 10, 10, 0, 0, 75, 76, 7, 1, 0, 0, 76, 107, 3, 2, 1, 11,
		77, 78, 10, 9, 0, 0, 78, 79, 7, 2, 0, 0, 79, 107, 3, 2, 1, 10, 80, 81,
		10, 8, 0, 0, 81, 82, 7, 3, 0, 0, 82, 107, 3, 2, 1, 9, 83, 84, 10, 7, 0,
		0, 84, 85, 7, 4, 0, 0, 85, 107, 3, 2, 1, 8, 86, 87, 10, 6, 0, 0, 87, 88,
		7, 5, 0, 0, 88, 107, 3, 2, 1, 7, 89, 90, 10, 5, 0, 0, 90, 91, 5, 48, 0,
		0, 91, 107, 3, 2, 1, 6, 92, 93, 10, 4, 0, 0, 93, 94, 5, 25, 0, 0, 94, 107,
		3, 2, 1, 5, 95, 96, 10, 3, 0, 0, 96, 97, 5, 30, 0, 0, 97, 107, 3, 2, 1,
		4, 98, 99, 10, 2, 0, 0, 99, 100, 5, 32, 0, 0, 100, 101, 3, 2, 1, 0, 101,
		102, 5, 7, 0, 0, 102, 103, 3, 2, 1, 3, 103, 107, 1, 0, 0, 0, 104, 105,
		10, 12, 0, 0, 105, 107, 7, 6, 0, 0, 106, 71, 1, 0, 0, 0, 106, 74, 1, 0,
		0, 0, 106, 77, 1, 0, 0, 0, 106, 80, 1, 0, 0, 0, 106, 83, 1, 0, 0, 0, 106,
		86, 1, 0, 0, 0, 106, 89, 1, 0, 0, 0, 106, 92, 1, 0, 0, 0, 106, 95, 1, 0,
		0, 0, 106, 98, 1, 0, 0, 0, 106, 104, 1, 0, 0, 0, 107, 110, 1, 0, 0, 0,
		108, 106, 1, 0, 0, 0, 108, 109, 1, 0, 0, 0, 109, 3, 1, 0, 0, 0, 110, 108,
		1, 0, 0, 0, 111, 114, 3, 8, 4, 0, 112, 114, 3, 6, 3, 0, 113, 111, 1, 0,
		0, 0, 113, 112, 1, 0, 0, 0, 114, 5, 1, 0, 0, 0, 115, 116, 5, 15, 0, 0,
		116, 117, 3, 2, 1, 0, 117, 118, 5, 16, 0, 0, 118, 122, 1, 0, 0, 0, 119,
		122, 3, 52, 26, 0, 120, 122, 3, 54, 27, 0, 121, 115, 1, 0, 0, 0, 121, 119,
		1, 0, 0, 0, 121, 120, 1, 0, 0, 0, 122, 7, 1, 0, 0, 0, 123, 124, 7, 7, 0,
		0, 124, 129, 3, 16, 8, 0, 125, 129, 3, 10, 5, 0, 126, 129, 3, 18, 9, 0,
		127, 129, 3, 20, 10, 0, 128, 123, 1, 0, 0, 0, 128, 125, 1, 0, 0, 0, 128,
		126, 1, 0, 0, 0, 128, 127, 1, 0, 0, 0, 129, 9, 1, 0, 0, 0, 130, 131, 5,
		17, 0, 0, 131, 136, 5, 55, 0, 0, 132, 133, 5, 17, 0, 0, 133, 134, 5, 55,
		0, 0, 134, 136, 3, 12, 6, 0, 135, 130, 1, 0, 0, 0, 135, 132, 1, 0, 0, 0,
		136, 11, 1, 0, 0, 0, 137, 138, 5, 13, 0, 0, 138, 139, 3, 14, 7, 0, 139,
		140, 5, 14, 0, 0, 140, 13, 1, 0, 0, 0, 141, 143, 3, 2, 1, 0, 142, 141,
		1, 0, 0, 0, 142, 143, 1, 0, 0, 0, 143, 148, 1, 0, 0, 0, 144, 145, 5, 9,
		0, 0, 145, 147, 3, 2, 1, 0, 146, 144, 1, 0, 0, 0, 147, 150, 1, 0, 0, 0,
		148, 146, 1, 0, 0, 0, 148, 149, 1, 0, 0, 0, 149, 15, 1, 0, 0, 0, 150, 148,
		1, 0, 0, 0, 151, 155, 5, 55, 0, 0, 152, 153, 5, 55, 0, 0, 153, 155, 3,
		12, 6, 0, 154, 151, 1, 0, 0, 0, 154, 152, 1, 0, 0, 0, 155, 17, 1, 0, 0,
		0, 156, 157, 5, 22, 0, 0, 157, 158, 3, 2, 1, 0, 158, 159, 5, 16, 0, 0,
		159, 19, 1, 0, 0, 0, 160, 161, 7, 8, 0, 0, 161, 162, 3, 2, 1, 0, 162, 163,
		5, 16, 0, 0, 163, 21, 1, 0, 0, 0, 164, 178, 3, 24, 12, 0, 165, 178, 3,
		28, 14, 0, 166, 178, 3, 30, 15, 0, 167, 178, 3, 34, 17, 0, 168, 178, 3,
		36, 18, 0, 169, 178, 3, 16, 8, 0, 170, 178, 3, 10, 5, 0, 171, 178, 3, 50,
		25, 0, 172, 178, 3, 18, 9, 0, 173, 178, 3, 20, 10, 0, 174, 178, 3, 40,
		20, 0, 175, 178, 3, 52, 26, 0, 176, 178, 3, 54, 27, 0, 177, 164, 1, 0,
		0, 0, 177, 165, 1, 0, 0, 0, 177, 166, 1, 0, 0, 0, 177, 167, 1, 0, 0, 0,
		177, 168, 1, 0, 0, 0, 177, 169, 1, 0, 0, 0, 177, 170, 1, 0, 0, 0, 177,
		171, 1, 0, 0, 0, 177, 172, 1, 0, 0, 0, 177, 173, 1, 0, 0, 0, 177, 174,
		1, 0, 0, 0, 177, 175, 1, 0, 0, 0, 177, 176, 1, 0, 0, 0, 178, 23, 1, 0,
		0, 0, 179, 184, 3, 26, 13, 0, 180, 184, 5, 58, 0, 0, 181, 184, 5, 43, 0,
		0, 182, 184, 5, 44, 0, 0, 183, 179, 1, 0, 0, 0, 183, 180, 1, 0, 0, 0, 183,
		181, 1, 0, 0, 0, 183, 182, 1, 0, 0, 0, 184, 25, 1, 0, 0, 0, 185, 186, 7,
		9, 0, 0, 186, 27, 1, 0, 0, 0, 187, 188, 5, 13, 0, 0, 188, 189, 3, 2, 1,
		0, 189, 190, 5, 14, 0, 0, 190, 29, 1, 0, 0, 0, 191, 192, 5, 47, 0, 0, 192,
		193, 5, 13, 0, 0, 193, 198, 3, 32, 16, 0, 194, 195, 5, 15, 0, 0, 195, 197,
		5, 16, 0, 0, 196, 194, 1, 0, 0, 0, 197, 200, 1, 0, 0, 0, 198, 196, 1, 0,
		0, 0, 198, 199, 1, 0, 0, 0, 199, 201, 1, 0, 0, 0, 200, 198, 1, 0, 0, 0,
		201, 202, 5, 14, 0, 0, 202, 31, 1, 0, 0, 0, 203, 208, 5, 55, 0, 0, 204,
		205, 5, 8, 0, 0, 205, 207, 5, 55, 0, 0, 206, 204, 1, 0, 0, 0, 207, 210,
		1, 0, 0, 0, 208, 206, 1, 0, 0, 0, 208, 209, 1, 0, 0, 0, 209, 33, 1, 0,
		0, 0, 210, 208, 1, 0, 0, 0, 211, 212, 5, 46, 0, 0, 212, 35, 1, 0, 0, 0,
		213, 214, 5, 45, 0, 0, 214, 220, 3, 32, 16, 0, 215, 217, 5, 15, 0, 0, 216,
		218, 3, 2, 1, 0, 217, 216, 1, 0, 0, 0, 217, 218, 1, 0, 0, 0, 218, 219,
		1, 0, 0, 0, 219, 221, 5, 16, 0, 0, 220, 215, 1, 0, 0, 0, 221, 222, 1, 0,
		0, 0, 222, 220, 1, 0, 0, 0, 222, 223, 1, 0, 0, 0, 223, 225, 1, 0, 0, 0,
		224, 226, 3, 40, 20, 0, 225, 224, 1, 0, 0, 0, 225, 226, 1, 0, 0, 0, 226,
		232, 1, 0, 0, 0, 227, 228, 5, 45, 0, 0, 228, 229, 3, 32, 16, 0, 229, 230,
		3, 38, 19, 0, 230, 232, 1, 0, 0, 0, 231, 213, 1, 0, 0, 0, 231, 227, 1,
		0, 0, 0, 232, 37, 1, 0, 0, 0, 233, 234, 5, 13, 0, 0, 234, 235, 3, 14, 7,
		0, 235, 236, 5, 14, 0, 0, 236, 39, 1, 0, 0, 0, 237, 238, 5, 38, 0, 0, 238,
		251, 5, 39, 0, 0, 239, 240, 5, 38, 0, 0, 240, 241, 5, 7, 0, 0, 241, 251,
		5, 39, 0, 0, 242, 243, 5, 38, 0, 0, 243, 244, 3, 42, 21, 0, 244, 245, 5,
		39, 0, 0, 245, 251, 1, 0, 0, 0, 246, 247, 5, 38, 0, 0, 247, 248, 3, 46,
		23, 0, 248, 249, 5, 39, 0, 0, 249, 251, 1, 0, 0, 0, 250, 237, 1, 0, 0,
		0, 250, 239, 1, 0, 0, 0, 250, 242, 1, 0, 0, 0, 250, 246, 1, 0, 0, 0, 251,
		41, 1, 0, 0, 0, 252, 257, 3, 44, 22, 0, 253, 254, 5, 9, 0, 0, 254, 256,
		3, 44, 22, 0, 255, 253, 1, 0, 0, 0, 256, 259, 1, 0, 0, 0, 257, 255, 1,
		0, 0, 0, 257, 258, 1, 0, 0, 0, 258, 43, 1, 0, 0, 0, 259, 257, 1, 0, 0,
		0, 260, 261, 3, 2, 1, 0, 261, 45, 1, 0, 0, 0, 262, 267, 3, 48, 24, 0, 263,
		264, 5, 9, 0, 0, 264, 266, 3, 48, 24, 0, 265, 263, 1, 0, 0, 0, 266, 269,
		1, 0, 0, 0, 267, 265, 1, 0, 0, 0, 267, 268, 1, 0, 0, 0, 268, 47, 1, 0,
		0, 0, 269, 267, 1, 0, 0, 0, 270, 271, 3, 2, 1, 0, 271, 272, 5, 7, 0, 0,
		272, 273, 3, 2, 1, 0, 273, 49, 1, 0, 0, 0, 274, 275, 7, 10, 0, 0, 275,
		276, 7, 11, 0, 0, 276, 51, 1, 0, 0, 0, 277, 278, 5, 15, 0, 0, 278, 279,
		5, 57, 0, 0, 279, 280, 5, 16, 0, 0, 280, 53, 1, 0, 0, 0, 281, 282, 5, 61,
		0, 0, 282, 55, 1, 0, 0, 0, 22, 66, 69, 106, 108, 113, 121, 128, 135, 142,
		148, 154, 177, 183, 198, 208, 217, 222, 225, 231, 250, 257, 267,
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

// SpelParserInit initializes any static state used to implement SpelParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewSpelParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func SpelParserInit() {
	staticData := &spelparserParserStaticData
	staticData.once.Do(spelparserParserInit)
}

// NewSpelParser produces a new parser instance for the optional input antlr.TokenStream.
func NewSpelParser(input antlr.TokenStream) *SpelParser {
	SpelParserInit()
	this := new(SpelParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &spelparserParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.predictionContextCache)
	this.RuleNames = staticData.ruleNames
	this.LiteralNames = staticData.literalNames
	this.SymbolicNames = staticData.symbolicNames
	this.GrammarFileName = "java-escape"

	return this
}

// SpelParser tokens.
const (
	SpelParserEOF                          = antlr.TokenEOF
	SpelParserSEMICOLON                    = 1
	SpelParserWS                           = 2
	SpelParserINC                          = 3
	SpelParserPLUS                         = 4
	SpelParserDEC                          = 5
	SpelParserMINUS                        = 6
	SpelParserCOLON                        = 7
	SpelParserDOT                          = 8
	SpelParserCOMMA                        = 9
	SpelParserSTAR                         = 10
	SpelParserDIV                          = 11
	SpelParserMOD                          = 12
	SpelParserLPAREN                       = 13
	SpelParserRPAREN                       = 14
	SpelParserLSQUARE                      = 15
	SpelParserRSQUARE                      = 16
	SpelParserHASH                         = 17
	SpelParserBEAN_REF                     = 18
	SpelParserSELECT_FIRST                 = 19
	SpelParserPOWER                        = 20
	SpelParserNE                           = 21
	SpelParserPROJECT                      = 22
	SpelParserNOT                          = 23
	SpelParserEQ                           = 24
	SpelParserASSIGN                       = 25
	SpelParserSYMBOLIC_AND                 = 26
	SpelParserFACTORY_BEAN_REF             = 27
	SpelParserSYMBOLIC_OR                  = 28
	SpelParserSELECT                       = 29
	SpelParserELVIS                        = 30
	SpelParserSAFE_NAVI                    = 31
	SpelParserQMARK                        = 32
	SpelParserSELECT_LAST                  = 33
	SpelParserGE                           = 34
	SpelParserGT                           = 35
	SpelParserLE                           = 36
	SpelParserLT                           = 37
	SpelParserLCURLY                       = 38
	SpelParserRCURLY                       = 39
	SpelParserBACKTICK                     = 40
	SpelParserOR                           = 41
	SpelParserAND                          = 42
	SpelParserTRUE                         = 43
	SpelParserFALSE                        = 44
	SpelParserNEW                          = 45
	SpelParserNULL                         = 46
	SpelParserT                            = 47
	SpelParserMATCHES                      = 48
	SpelParserGT_KEYWORD                   = 49
	SpelParserGE_KEYWORD                   = 50
	SpelParserLE_KEYWORD                   = 51
	SpelParserLT_KEYWORD                   = 52
	SpelParserEQ_KEYWORD                   = 53
	SpelParserNE_KEYWORD                   = 54
	SpelParserIDENTIFIER                   = 55
	SpelParserREAL_LITERAL                 = 56
	SpelParserINTEGER_LITERAL              = 57
	SpelParserSTRING_LITERAL               = 58
	SpelParserSINGLE_QUOTED_STRING         = 59
	SpelParserDOUBLE_QUOTED_STRING         = 60
	SpelParserPROPERTY_PLACE_HOLDER        = 61
	SpelParserESCAPED_BACKTICK             = 62
	SpelParserSPEL_IN_TEMPLATE_STRING_OPEN = 63
	SpelParserTEMPLATE_TEXT                = 64
)

// SpelParser rules.
const (
	SpelParserRULE_script               = 0
	SpelParserRULE_spelExpr             = 1
	SpelParserRULE_node                 = 2
	SpelParserRULE_nonDottedNode        = 3
	SpelParserRULE_dottedNode           = 4
	SpelParserRULE_functionOrVar        = 5
	SpelParserRULE_methodArgs           = 6
	SpelParserRULE_args                 = 7
	SpelParserRULE_methodOrProperty     = 8
	SpelParserRULE_projection           = 9
	SpelParserRULE_selection            = 10
	SpelParserRULE_startNode            = 11
	SpelParserRULE_literal              = 12
	SpelParserRULE_numericLiteral       = 13
	SpelParserRULE_parenspelExpr        = 14
	SpelParserRULE_typeReference        = 15
	SpelParserRULE_possiblyQualifiedId  = 16
	SpelParserRULE_nullReference        = 17
	SpelParserRULE_constructorReference = 18
	SpelParserRULE_constructorArgs      = 19
	SpelParserRULE_inlineListOrMap      = 20
	SpelParserRULE_listBindings         = 21
	SpelParserRULE_listBinding          = 22
	SpelParserRULE_mapBindings          = 23
	SpelParserRULE_mapBinding           = 24
	SpelParserRULE_beanReference        = 25
	SpelParserRULE_inputParameter       = 26
	SpelParserRULE_propertyPlaceHolder  = 27
)

// IScriptContext is an interface to support dynamic dispatch.
type IScriptContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsScriptContext differentiates from other interfaces.
	IsScriptContext()
}

type ScriptContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyScriptContext() *ScriptContext {
	var p = new(ScriptContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_script
	return p
}

func (*ScriptContext) IsScriptContext() {}

func NewScriptContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ScriptContext {
	var p = new(ScriptContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_script

	return p
}

func (s *ScriptContext) GetParser() antlr.Parser { return s.parser }

func (s *ScriptContext) SpelExpr() ISpelExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISpelExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISpelExprContext)
}

func (s *ScriptContext) EOF() antlr.TerminalNode {
	return s.GetToken(SpelParserEOF, 0)
}

func (s *ScriptContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ScriptContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ScriptContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitScript(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) Script() (localctx IScriptContext) {
	this := p
	_ = this

	localctx = NewScriptContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, SpelParserRULE_script)

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
		p.SetState(56)
		p.spelExpr(0)
	}
	{
		p.SetState(57)
		p.Match(SpelParserEOF)
	}

	return localctx
}

// ISpelExprContext is an interface to support dynamic dispatch.
type ISpelExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsSpelExprContext differentiates from other interfaces.
	IsSpelExprContext()
}

type SpelExprContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySpelExprContext() *SpelExprContext {
	var p = new(SpelExprContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_spelExpr
	return p
}

func (*SpelExprContext) IsSpelExprContext() {}

func NewSpelExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SpelExprContext {
	var p = new(SpelExprContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_spelExpr

	return p
}

func (s *SpelExprContext) GetParser() antlr.Parser { return s.parser }

func (s *SpelExprContext) AllSpelExpr() []ISpelExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISpelExprContext); ok {
			len++
		}
	}

	tst := make([]ISpelExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISpelExprContext); ok {
			tst[i] = t.(ISpelExprContext)
			i++
		}
	}

	return tst
}

func (s *SpelExprContext) SpelExpr(i int) ISpelExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISpelExprContext); ok {
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

	return t.(ISpelExprContext)
}

func (s *SpelExprContext) PLUS() antlr.TerminalNode {
	return s.GetToken(SpelParserPLUS, 0)
}

func (s *SpelExprContext) MINUS() antlr.TerminalNode {
	return s.GetToken(SpelParserMINUS, 0)
}

func (s *SpelExprContext) NOT() antlr.TerminalNode {
	return s.GetToken(SpelParserNOT, 0)
}

func (s *SpelExprContext) INC() antlr.TerminalNode {
	return s.GetToken(SpelParserINC, 0)
}

func (s *SpelExprContext) DEC() antlr.TerminalNode {
	return s.GetToken(SpelParserDEC, 0)
}

func (s *SpelExprContext) StartNode() IStartNodeContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStartNodeContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStartNodeContext)
}

func (s *SpelExprContext) AllNode() []INodeContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(INodeContext); ok {
			len++
		}
	}

	tst := make([]INodeContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(INodeContext); ok {
			tst[i] = t.(INodeContext)
			i++
		}
	}

	return tst
}

func (s *SpelExprContext) Node(i int) INodeContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INodeContext); ok {
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

	return t.(INodeContext)
}

func (s *SpelExprContext) POWER() antlr.TerminalNode {
	return s.GetToken(SpelParserPOWER, 0)
}

func (s *SpelExprContext) STAR() antlr.TerminalNode {
	return s.GetToken(SpelParserSTAR, 0)
}

func (s *SpelExprContext) DIV() antlr.TerminalNode {
	return s.GetToken(SpelParserDIV, 0)
}

func (s *SpelExprContext) MOD() antlr.TerminalNode {
	return s.GetToken(SpelParserMOD, 0)
}

func (s *SpelExprContext) GT() antlr.TerminalNode {
	return s.GetToken(SpelParserGT, 0)
}

func (s *SpelExprContext) LT() antlr.TerminalNode {
	return s.GetToken(SpelParserLT, 0)
}

func (s *SpelExprContext) LE() antlr.TerminalNode {
	return s.GetToken(SpelParserLE, 0)
}

func (s *SpelExprContext) GE() antlr.TerminalNode {
	return s.GetToken(SpelParserGE, 0)
}

func (s *SpelExprContext) EQ() antlr.TerminalNode {
	return s.GetToken(SpelParserEQ, 0)
}

func (s *SpelExprContext) NE() antlr.TerminalNode {
	return s.GetToken(SpelParserNE, 0)
}

func (s *SpelExprContext) GT_KEYWORD() antlr.TerminalNode {
	return s.GetToken(SpelParserGT_KEYWORD, 0)
}

func (s *SpelExprContext) LT_KEYWORD() antlr.TerminalNode {
	return s.GetToken(SpelParserLT_KEYWORD, 0)
}

func (s *SpelExprContext) LE_KEYWORD() antlr.TerminalNode {
	return s.GetToken(SpelParserLE_KEYWORD, 0)
}

func (s *SpelExprContext) GE_KEYWORD() antlr.TerminalNode {
	return s.GetToken(SpelParserGE_KEYWORD, 0)
}

func (s *SpelExprContext) EQ_KEYWORD() antlr.TerminalNode {
	return s.GetToken(SpelParserEQ_KEYWORD, 0)
}

func (s *SpelExprContext) NE_KEYWORD() antlr.TerminalNode {
	return s.GetToken(SpelParserNE_KEYWORD, 0)
}

func (s *SpelExprContext) AND() antlr.TerminalNode {
	return s.GetToken(SpelParserAND, 0)
}

func (s *SpelExprContext) SYMBOLIC_AND() antlr.TerminalNode {
	return s.GetToken(SpelParserSYMBOLIC_AND, 0)
}

func (s *SpelExprContext) OR() antlr.TerminalNode {
	return s.GetToken(SpelParserOR, 0)
}

func (s *SpelExprContext) SYMBOLIC_OR() antlr.TerminalNode {
	return s.GetToken(SpelParserSYMBOLIC_OR, 0)
}

func (s *SpelExprContext) MATCHES() antlr.TerminalNode {
	return s.GetToken(SpelParserMATCHES, 0)
}

func (s *SpelExprContext) ASSIGN() antlr.TerminalNode {
	return s.GetToken(SpelParserASSIGN, 0)
}

func (s *SpelExprContext) ELVIS() antlr.TerminalNode {
	return s.GetToken(SpelParserELVIS, 0)
}

func (s *SpelExprContext) QMARK() antlr.TerminalNode {
	return s.GetToken(SpelParserQMARK, 0)
}

func (s *SpelExprContext) COLON() antlr.TerminalNode {
	return s.GetToken(SpelParserCOLON, 0)
}

func (s *SpelExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SpelExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *SpelExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitSpelExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) SpelExpr() (localctx ISpelExprContext) {
	return p.spelExpr(0)
}

func (p *SpelParser) spelExpr(_p int) (localctx ISpelExprContext) {
	this := p
	_ = this

	var _parentctx antlr.ParserRuleContext = p.GetParserRuleContext()
	_parentState := p.GetState()
	localctx = NewSpelExprContext(p, p.GetParserRuleContext(), _parentState)
	var _prevctx ISpelExprContext = localctx
	var _ antlr.ParserRuleContext = _prevctx // TODO: To prevent unused variable warning.
	_startState := 2
	p.EnterRecursionRule(localctx, 2, SpelParserRULE_spelExpr, _p)
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
	case SpelParserINC, SpelParserPLUS, SpelParserDEC, SpelParserMINUS, SpelParserNOT:
		{
			p.SetState(60)
			_la = p.GetTokenStream().LA(1)

			if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&8388728) != 0) {
				p.GetErrorHandler().RecoverInline(p)
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}
		{
			p.SetState(61)
			p.spelExpr(13)
		}

	case SpelParserLPAREN, SpelParserLSQUARE, SpelParserHASH, SpelParserBEAN_REF, SpelParserSELECT_FIRST, SpelParserPROJECT, SpelParserFACTORY_BEAN_REF, SpelParserSELECT, SpelParserSELECT_LAST, SpelParserLCURLY, SpelParserTRUE, SpelParserFALSE, SpelParserNEW, SpelParserNULL, SpelParserT, SpelParserIDENTIFIER, SpelParserREAL_LITERAL, SpelParserINTEGER_LITERAL, SpelParserSTRING_LITERAL, SpelParserPROPERTY_PLACE_HOLDER:
		{
			p.SetState(62)
			p.StartNode()
		}
		p.SetState(66)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 0, p.GetParserRuleContext())

		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(63)
					p.Node()
				}

			}
			p.SetState(68)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 0, p.GetParserRuleContext())
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}
	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(108)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 3, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			p.SetState(106)
			p.GetErrorHandler().Sync(p)
			switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 2, p.GetParserRuleContext()) {
			case 1:
				localctx = NewSpelExprContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, SpelParserRULE_spelExpr)
				p.SetState(71)

				if !(p.Precpred(p.GetParserRuleContext(), 11)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 11)", ""))
				}
				{
					p.SetState(72)
					p.Match(SpelParserPOWER)
				}
				{
					p.SetState(73)
					p.spelExpr(12)
				}

			case 2:
				localctx = NewSpelExprContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, SpelParserRULE_spelExpr)
				p.SetState(74)

				if !(p.Precpred(p.GetParserRuleContext(), 10)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 10)", ""))
				}
				{
					p.SetState(75)
					_la = p.GetTokenStream().LA(1)

					if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&7168) != 0) {
						p.GetErrorHandler().RecoverInline(p)
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(76)
					p.spelExpr(11)
				}

			case 3:
				localctx = NewSpelExprContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, SpelParserRULE_spelExpr)
				p.SetState(77)

				if !(p.Precpred(p.GetParserRuleContext(), 9)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 9)", ""))
				}
				{
					p.SetState(78)
					_la = p.GetTokenStream().LA(1)

					if !(_la == SpelParserPLUS || _la == SpelParserMINUS) {
						p.GetErrorHandler().RecoverInline(p)
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(79)
					p.spelExpr(10)
				}

			case 4:
				localctx = NewSpelExprContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, SpelParserRULE_spelExpr)
				p.SetState(80)

				if !(p.Precpred(p.GetParserRuleContext(), 8)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 8)", ""))
				}
				{
					p.SetState(81)
					_la = p.GetTokenStream().LA(1)

					if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&35466104782454784) != 0) {
						p.GetErrorHandler().RecoverInline(p)
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(82)
					p.spelExpr(9)
				}

			case 5:
				localctx = NewSpelExprContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, SpelParserRULE_spelExpr)
				p.SetState(83)

				if !(p.Precpred(p.GetParserRuleContext(), 7)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 7)", ""))
				}
				{
					p.SetState(84)
					_la = p.GetTokenStream().LA(1)

					if !(_la == SpelParserSYMBOLIC_AND || _la == SpelParserAND) {
						p.GetErrorHandler().RecoverInline(p)
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(85)
					p.spelExpr(8)
				}

			case 6:
				localctx = NewSpelExprContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, SpelParserRULE_spelExpr)
				p.SetState(86)

				if !(p.Precpred(p.GetParserRuleContext(), 6)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 6)", ""))
				}
				{
					p.SetState(87)
					_la = p.GetTokenStream().LA(1)

					if !(_la == SpelParserSYMBOLIC_OR || _la == SpelParserOR) {
						p.GetErrorHandler().RecoverInline(p)
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(88)
					p.spelExpr(7)
				}

			case 7:
				localctx = NewSpelExprContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, SpelParserRULE_spelExpr)
				p.SetState(89)

				if !(p.Precpred(p.GetParserRuleContext(), 5)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 5)", ""))
				}
				{
					p.SetState(90)
					p.Match(SpelParserMATCHES)
				}
				{
					p.SetState(91)
					p.spelExpr(6)
				}

			case 8:
				localctx = NewSpelExprContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, SpelParserRULE_spelExpr)
				p.SetState(92)

				if !(p.Precpred(p.GetParserRuleContext(), 4)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 4)", ""))
				}
				{
					p.SetState(93)
					p.Match(SpelParserASSIGN)
				}
				{
					p.SetState(94)
					p.spelExpr(5)
				}

			case 9:
				localctx = NewSpelExprContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, SpelParserRULE_spelExpr)
				p.SetState(95)

				if !(p.Precpred(p.GetParserRuleContext(), 3)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 3)", ""))
				}
				{
					p.SetState(96)
					p.Match(SpelParserELVIS)
				}
				{
					p.SetState(97)
					p.spelExpr(4)
				}

			case 10:
				localctx = NewSpelExprContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, SpelParserRULE_spelExpr)
				p.SetState(98)

				if !(p.Precpred(p.GetParserRuleContext(), 2)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 2)", ""))
				}
				{
					p.SetState(99)
					p.Match(SpelParserQMARK)
				}
				{
					p.SetState(100)
					p.spelExpr(0)
				}
				{
					p.SetState(101)
					p.Match(SpelParserCOLON)
				}
				{
					p.SetState(102)
					p.spelExpr(3)
				}

			case 11:
				localctx = NewSpelExprContext(p, _parentctx, _parentState)
				p.PushNewRecursionContext(localctx, _startState, SpelParserRULE_spelExpr)
				p.SetState(104)

				if !(p.Precpred(p.GetParserRuleContext(), 12)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 12)", ""))
				}
				{
					p.SetState(105)
					_la = p.GetTokenStream().LA(1)

					if !(_la == SpelParserINC || _la == SpelParserDEC) {
						p.GetErrorHandler().RecoverInline(p)
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}

			}

		}
		p.SetState(110)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 3, p.GetParserRuleContext())
	}

	return localctx
}

// INodeContext is an interface to support dynamic dispatch.
type INodeContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNodeContext differentiates from other interfaces.
	IsNodeContext()
}

type NodeContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNodeContext() *NodeContext {
	var p = new(NodeContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_node
	return p
}

func (*NodeContext) IsNodeContext() {}

func NewNodeContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NodeContext {
	var p = new(NodeContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_node

	return p
}

func (s *NodeContext) GetParser() antlr.Parser { return s.parser }

func (s *NodeContext) DottedNode() IDottedNodeContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDottedNodeContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDottedNodeContext)
}

func (s *NodeContext) NonDottedNode() INonDottedNodeContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INonDottedNodeContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INonDottedNodeContext)
}

func (s *NodeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NodeContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NodeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitNode(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) Node() (localctx INodeContext) {
	this := p
	_ = this

	localctx = NewNodeContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, SpelParserRULE_node)

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

	p.SetState(113)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SpelParserDOT, SpelParserHASH, SpelParserSELECT_FIRST, SpelParserPROJECT, SpelParserSELECT, SpelParserSAFE_NAVI, SpelParserSELECT_LAST:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(111)
			p.DottedNode()
		}

	case SpelParserLSQUARE, SpelParserPROPERTY_PLACE_HOLDER:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(112)
			p.NonDottedNode()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// INonDottedNodeContext is an interface to support dynamic dispatch.
type INonDottedNodeContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNonDottedNodeContext differentiates from other interfaces.
	IsNonDottedNodeContext()
}

type NonDottedNodeContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNonDottedNodeContext() *NonDottedNodeContext {
	var p = new(NonDottedNodeContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_nonDottedNode
	return p
}

func (*NonDottedNodeContext) IsNonDottedNodeContext() {}

func NewNonDottedNodeContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NonDottedNodeContext {
	var p = new(NonDottedNodeContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_nonDottedNode

	return p
}

func (s *NonDottedNodeContext) GetParser() antlr.Parser { return s.parser }

func (s *NonDottedNodeContext) LSQUARE() antlr.TerminalNode {
	return s.GetToken(SpelParserLSQUARE, 0)
}

func (s *NonDottedNodeContext) SpelExpr() ISpelExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISpelExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISpelExprContext)
}

func (s *NonDottedNodeContext) RSQUARE() antlr.TerminalNode {
	return s.GetToken(SpelParserRSQUARE, 0)
}

func (s *NonDottedNodeContext) InputParameter() IInputParameterContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IInputParameterContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IInputParameterContext)
}

func (s *NonDottedNodeContext) PropertyPlaceHolder() IPropertyPlaceHolderContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IPropertyPlaceHolderContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IPropertyPlaceHolderContext)
}

func (s *NonDottedNodeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NonDottedNodeContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NonDottedNodeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitNonDottedNode(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) NonDottedNode() (localctx INonDottedNodeContext) {
	this := p
	_ = this

	localctx = NewNonDottedNodeContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, SpelParserRULE_nonDottedNode)

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

	p.SetState(121)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 5, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(115)
			p.Match(SpelParserLSQUARE)
		}
		{
			p.SetState(116)
			p.spelExpr(0)
		}
		{
			p.SetState(117)
			p.Match(SpelParserRSQUARE)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(119)
			p.InputParameter()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(120)
			p.PropertyPlaceHolder()
		}

	}

	return localctx
}

// IDottedNodeContext is an interface to support dynamic dispatch.
type IDottedNodeContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDottedNodeContext differentiates from other interfaces.
	IsDottedNodeContext()
}

type DottedNodeContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDottedNodeContext() *DottedNodeContext {
	var p = new(DottedNodeContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_dottedNode
	return p
}

func (*DottedNodeContext) IsDottedNodeContext() {}

func NewDottedNodeContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DottedNodeContext {
	var p = new(DottedNodeContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_dottedNode

	return p
}

func (s *DottedNodeContext) GetParser() antlr.Parser { return s.parser }

func (s *DottedNodeContext) MethodOrProperty() IMethodOrPropertyContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IMethodOrPropertyContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IMethodOrPropertyContext)
}

func (s *DottedNodeContext) DOT() antlr.TerminalNode {
	return s.GetToken(SpelParserDOT, 0)
}

func (s *DottedNodeContext) SAFE_NAVI() antlr.TerminalNode {
	return s.GetToken(SpelParserSAFE_NAVI, 0)
}

func (s *DottedNodeContext) FunctionOrVar() IFunctionOrVarContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFunctionOrVarContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFunctionOrVarContext)
}

func (s *DottedNodeContext) Projection() IProjectionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IProjectionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IProjectionContext)
}

func (s *DottedNodeContext) Selection() ISelectionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISelectionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISelectionContext)
}

func (s *DottedNodeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DottedNodeContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DottedNodeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitDottedNode(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) DottedNode() (localctx IDottedNodeContext) {
	this := p
	_ = this

	localctx = NewDottedNodeContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, SpelParserRULE_dottedNode)
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

	p.SetState(128)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SpelParserDOT, SpelParserSAFE_NAVI:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(123)
			_la = p.GetTokenStream().LA(1)

			if !(_la == SpelParserDOT || _la == SpelParserSAFE_NAVI) {
				p.GetErrorHandler().RecoverInline(p)
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}
		{
			p.SetState(124)
			p.MethodOrProperty()
		}

	case SpelParserHASH:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(125)
			p.FunctionOrVar()
		}

	case SpelParserPROJECT:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(126)
			p.Projection()
		}

	case SpelParserSELECT_FIRST, SpelParserSELECT, SpelParserSELECT_LAST:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(127)
			p.Selection()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IFunctionOrVarContext is an interface to support dynamic dispatch.
type IFunctionOrVarContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFunctionOrVarContext differentiates from other interfaces.
	IsFunctionOrVarContext()
}

type FunctionOrVarContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFunctionOrVarContext() *FunctionOrVarContext {
	var p = new(FunctionOrVarContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_functionOrVar
	return p
}

func (*FunctionOrVarContext) IsFunctionOrVarContext() {}

func NewFunctionOrVarContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FunctionOrVarContext {
	var p = new(FunctionOrVarContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_functionOrVar

	return p
}

func (s *FunctionOrVarContext) GetParser() antlr.Parser { return s.parser }

func (s *FunctionOrVarContext) HASH() antlr.TerminalNode {
	return s.GetToken(SpelParserHASH, 0)
}

func (s *FunctionOrVarContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(SpelParserIDENTIFIER, 0)
}

func (s *FunctionOrVarContext) MethodArgs() IMethodArgsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IMethodArgsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IMethodArgsContext)
}

func (s *FunctionOrVarContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FunctionOrVarContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FunctionOrVarContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitFunctionOrVar(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) FunctionOrVar() (localctx IFunctionOrVarContext) {
	this := p
	_ = this

	localctx = NewFunctionOrVarContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, SpelParserRULE_functionOrVar)

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
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(130)
			p.Match(SpelParserHASH)
		}
		{
			p.SetState(131)
			p.Match(SpelParserIDENTIFIER)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(132)
			p.Match(SpelParserHASH)
		}
		{
			p.SetState(133)
			p.Match(SpelParserIDENTIFIER)
		}
		{
			p.SetState(134)
			p.MethodArgs()
		}

	}

	return localctx
}

// IMethodArgsContext is an interface to support dynamic dispatch.
type IMethodArgsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsMethodArgsContext differentiates from other interfaces.
	IsMethodArgsContext()
}

type MethodArgsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyMethodArgsContext() *MethodArgsContext {
	var p = new(MethodArgsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_methodArgs
	return p
}

func (*MethodArgsContext) IsMethodArgsContext() {}

func NewMethodArgsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *MethodArgsContext {
	var p = new(MethodArgsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_methodArgs

	return p
}

func (s *MethodArgsContext) GetParser() antlr.Parser { return s.parser }

func (s *MethodArgsContext) LPAREN() antlr.TerminalNode {
	return s.GetToken(SpelParserLPAREN, 0)
}

func (s *MethodArgsContext) Args() IArgsContext {
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

func (s *MethodArgsContext) RPAREN() antlr.TerminalNode {
	return s.GetToken(SpelParserRPAREN, 0)
}

func (s *MethodArgsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *MethodArgsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *MethodArgsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitMethodArgs(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) MethodArgs() (localctx IMethodArgsContext) {
	this := p
	_ = this

	localctx = NewMethodArgsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, SpelParserRULE_methodArgs)

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
		p.Match(SpelParserLPAREN)
	}
	{
		p.SetState(138)
		p.Args()
	}
	{
		p.SetState(139)
		p.Match(SpelParserRPAREN)
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
	p.RuleIndex = SpelParserRULE_args
	return p
}

func (*ArgsContext) IsArgsContext() {}

func NewArgsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ArgsContext {
	var p = new(ArgsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_args

	return p
}

func (s *ArgsContext) GetParser() antlr.Parser { return s.parser }

func (s *ArgsContext) AllSpelExpr() []ISpelExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISpelExprContext); ok {
			len++
		}
	}

	tst := make([]ISpelExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISpelExprContext); ok {
			tst[i] = t.(ISpelExprContext)
			i++
		}
	}

	return tst
}

func (s *ArgsContext) SpelExpr(i int) ISpelExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISpelExprContext); ok {
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

	return t.(ISpelExprContext)
}

func (s *ArgsContext) AllCOMMA() []antlr.TerminalNode {
	return s.GetTokens(SpelParserCOMMA)
}

func (s *ArgsContext) COMMA(i int) antlr.TerminalNode {
	return s.GetToken(SpelParserCOMMA, i)
}

func (s *ArgsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ArgsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ArgsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitArgs(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) Args() (localctx IArgsContext) {
	this := p
	_ = this

	localctx = NewArgsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 14, SpelParserRULE_args)
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
	p.SetState(142)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&2846547927534313592) != 0 {
		{
			p.SetState(141)
			p.spelExpr(0)
		}

	}
	p.SetState(148)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == SpelParserCOMMA {
		{
			p.SetState(144)
			p.Match(SpelParserCOMMA)
		}
		{
			p.SetState(145)
			p.spelExpr(0)
		}

		p.SetState(150)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// IMethodOrPropertyContext is an interface to support dynamic dispatch.
type IMethodOrPropertyContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsMethodOrPropertyContext differentiates from other interfaces.
	IsMethodOrPropertyContext()
}

type MethodOrPropertyContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyMethodOrPropertyContext() *MethodOrPropertyContext {
	var p = new(MethodOrPropertyContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_methodOrProperty
	return p
}

func (*MethodOrPropertyContext) IsMethodOrPropertyContext() {}

func NewMethodOrPropertyContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *MethodOrPropertyContext {
	var p = new(MethodOrPropertyContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_methodOrProperty

	return p
}

func (s *MethodOrPropertyContext) GetParser() antlr.Parser { return s.parser }

func (s *MethodOrPropertyContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(SpelParserIDENTIFIER, 0)
}

func (s *MethodOrPropertyContext) MethodArgs() IMethodArgsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IMethodArgsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IMethodArgsContext)
}

func (s *MethodOrPropertyContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *MethodOrPropertyContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *MethodOrPropertyContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitMethodOrProperty(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) MethodOrProperty() (localctx IMethodOrPropertyContext) {
	this := p
	_ = this

	localctx = NewMethodOrPropertyContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 16, SpelParserRULE_methodOrProperty)

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

	p.SetState(154)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 10, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(151)
			p.Match(SpelParserIDENTIFIER)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(152)
			p.Match(SpelParserIDENTIFIER)
		}
		{
			p.SetState(153)
			p.MethodArgs()
		}

	}

	return localctx
}

// IProjectionContext is an interface to support dynamic dispatch.
type IProjectionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsProjectionContext differentiates from other interfaces.
	IsProjectionContext()
}

type ProjectionContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyProjectionContext() *ProjectionContext {
	var p = new(ProjectionContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_projection
	return p
}

func (*ProjectionContext) IsProjectionContext() {}

func NewProjectionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ProjectionContext {
	var p = new(ProjectionContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_projection

	return p
}

func (s *ProjectionContext) GetParser() antlr.Parser { return s.parser }

func (s *ProjectionContext) PROJECT() antlr.TerminalNode {
	return s.GetToken(SpelParserPROJECT, 0)
}

func (s *ProjectionContext) SpelExpr() ISpelExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISpelExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISpelExprContext)
}

func (s *ProjectionContext) RSQUARE() antlr.TerminalNode {
	return s.GetToken(SpelParserRSQUARE, 0)
}

func (s *ProjectionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ProjectionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ProjectionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitProjection(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) Projection() (localctx IProjectionContext) {
	this := p
	_ = this

	localctx = NewProjectionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 18, SpelParserRULE_projection)

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
		p.SetState(156)
		p.Match(SpelParserPROJECT)
	}
	{
		p.SetState(157)
		p.spelExpr(0)
	}
	{
		p.SetState(158)
		p.Match(SpelParserRSQUARE)
	}

	return localctx
}

// ISelectionContext is an interface to support dynamic dispatch.
type ISelectionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsSelectionContext differentiates from other interfaces.
	IsSelectionContext()
}

type SelectionContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySelectionContext() *SelectionContext {
	var p = new(SelectionContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_selection
	return p
}

func (*SelectionContext) IsSelectionContext() {}

func NewSelectionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SelectionContext {
	var p = new(SelectionContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_selection

	return p
}

func (s *SelectionContext) GetParser() antlr.Parser { return s.parser }

func (s *SelectionContext) SpelExpr() ISpelExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISpelExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISpelExprContext)
}

func (s *SelectionContext) RSQUARE() antlr.TerminalNode {
	return s.GetToken(SpelParserRSQUARE, 0)
}

func (s *SelectionContext) SELECT() antlr.TerminalNode {
	return s.GetToken(SpelParserSELECT, 0)
}

func (s *SelectionContext) SELECT_FIRST() antlr.TerminalNode {
	return s.GetToken(SpelParserSELECT_FIRST, 0)
}

func (s *SelectionContext) SELECT_LAST() antlr.TerminalNode {
	return s.GetToken(SpelParserSELECT_LAST, 0)
}

func (s *SelectionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SelectionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *SelectionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitSelection(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) Selection() (localctx ISelectionContext) {
	this := p
	_ = this

	localctx = NewSelectionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 20, SpelParserRULE_selection)
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
		p.SetState(160)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&9127329792) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}
	{
		p.SetState(161)
		p.spelExpr(0)
	}
	{
		p.SetState(162)
		p.Match(SpelParserRSQUARE)
	}

	return localctx
}

// IStartNodeContext is an interface to support dynamic dispatch.
type IStartNodeContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStartNodeContext differentiates from other interfaces.
	IsStartNodeContext()
}

type StartNodeContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStartNodeContext() *StartNodeContext {
	var p = new(StartNodeContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_startNode
	return p
}

func (*StartNodeContext) IsStartNodeContext() {}

func NewStartNodeContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StartNodeContext {
	var p = new(StartNodeContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_startNode

	return p
}

func (s *StartNodeContext) GetParser() antlr.Parser { return s.parser }

func (s *StartNodeContext) Literal() ILiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ILiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ILiteralContext)
}

func (s *StartNodeContext) ParenspelExpr() IParenspelExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IParenspelExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IParenspelExprContext)
}

func (s *StartNodeContext) TypeReference() ITypeReferenceContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITypeReferenceContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ITypeReferenceContext)
}

func (s *StartNodeContext) NullReference() INullReferenceContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INullReferenceContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INullReferenceContext)
}

func (s *StartNodeContext) ConstructorReference() IConstructorReferenceContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConstructorReferenceContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConstructorReferenceContext)
}

func (s *StartNodeContext) MethodOrProperty() IMethodOrPropertyContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IMethodOrPropertyContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IMethodOrPropertyContext)
}

func (s *StartNodeContext) FunctionOrVar() IFunctionOrVarContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFunctionOrVarContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFunctionOrVarContext)
}

func (s *StartNodeContext) BeanReference() IBeanReferenceContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBeanReferenceContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBeanReferenceContext)
}

func (s *StartNodeContext) Projection() IProjectionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IProjectionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IProjectionContext)
}

func (s *StartNodeContext) Selection() ISelectionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISelectionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISelectionContext)
}

func (s *StartNodeContext) InlineListOrMap() IInlineListOrMapContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IInlineListOrMapContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IInlineListOrMapContext)
}

func (s *StartNodeContext) InputParameter() IInputParameterContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IInputParameterContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IInputParameterContext)
}

func (s *StartNodeContext) PropertyPlaceHolder() IPropertyPlaceHolderContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IPropertyPlaceHolderContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IPropertyPlaceHolderContext)
}

func (s *StartNodeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StartNodeContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StartNodeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitStartNode(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) StartNode() (localctx IStartNodeContext) {
	this := p
	_ = this

	localctx = NewStartNodeContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 22, SpelParserRULE_startNode)

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

	p.SetState(177)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SpelParserTRUE, SpelParserFALSE, SpelParserREAL_LITERAL, SpelParserINTEGER_LITERAL, SpelParserSTRING_LITERAL:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(164)
			p.Literal()
		}

	case SpelParserLPAREN:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(165)
			p.ParenspelExpr()
		}

	case SpelParserT:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(166)
			p.TypeReference()
		}

	case SpelParserNULL:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(167)
			p.NullReference()
		}

	case SpelParserNEW:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(168)
			p.ConstructorReference()
		}

	case SpelParserIDENTIFIER:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(169)
			p.MethodOrProperty()
		}

	case SpelParserHASH:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(170)
			p.FunctionOrVar()
		}

	case SpelParserBEAN_REF, SpelParserFACTORY_BEAN_REF:
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(171)
			p.BeanReference()
		}

	case SpelParserPROJECT:
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(172)
			p.Projection()
		}

	case SpelParserSELECT_FIRST, SpelParserSELECT, SpelParserSELECT_LAST:
		p.EnterOuterAlt(localctx, 10)
		{
			p.SetState(173)
			p.Selection()
		}

	case SpelParserLCURLY:
		p.EnterOuterAlt(localctx, 11)
		{
			p.SetState(174)
			p.InlineListOrMap()
		}

	case SpelParserLSQUARE:
		p.EnterOuterAlt(localctx, 12)
		{
			p.SetState(175)
			p.InputParameter()
		}

	case SpelParserPROPERTY_PLACE_HOLDER:
		p.EnterOuterAlt(localctx, 13)
		{
			p.SetState(176)
			p.PropertyPlaceHolder()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// ILiteralContext is an interface to support dynamic dispatch.
type ILiteralContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsLiteralContext differentiates from other interfaces.
	IsLiteralContext()
}

type LiteralContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyLiteralContext() *LiteralContext {
	var p = new(LiteralContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_literal
	return p
}

func (*LiteralContext) IsLiteralContext() {}

func NewLiteralContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *LiteralContext {
	var p = new(LiteralContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_literal

	return p
}

func (s *LiteralContext) GetParser() antlr.Parser { return s.parser }

func (s *LiteralContext) NumericLiteral() INumericLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INumericLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INumericLiteralContext)
}

func (s *LiteralContext) STRING_LITERAL() antlr.TerminalNode {
	return s.GetToken(SpelParserSTRING_LITERAL, 0)
}

func (s *LiteralContext) TRUE() antlr.TerminalNode {
	return s.GetToken(SpelParserTRUE, 0)
}

func (s *LiteralContext) FALSE() antlr.TerminalNode {
	return s.GetToken(SpelParserFALSE, 0)
}

func (s *LiteralContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LiteralContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *LiteralContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) Literal() (localctx ILiteralContext) {
	this := p
	_ = this

	localctx = NewLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, SpelParserRULE_literal)

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

	p.SetState(183)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SpelParserREAL_LITERAL, SpelParserINTEGER_LITERAL:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(179)
			p.NumericLiteral()
		}

	case SpelParserSTRING_LITERAL:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(180)
			p.Match(SpelParserSTRING_LITERAL)
		}

	case SpelParserTRUE:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(181)
			p.Match(SpelParserTRUE)
		}

	case SpelParserFALSE:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(182)
			p.Match(SpelParserFALSE)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// INumericLiteralContext is an interface to support dynamic dispatch.
type INumericLiteralContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNumericLiteralContext differentiates from other interfaces.
	IsNumericLiteralContext()
}

type NumericLiteralContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNumericLiteralContext() *NumericLiteralContext {
	var p = new(NumericLiteralContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_numericLiteral
	return p
}

func (*NumericLiteralContext) IsNumericLiteralContext() {}

func NewNumericLiteralContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NumericLiteralContext {
	var p = new(NumericLiteralContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_numericLiteral

	return p
}

func (s *NumericLiteralContext) GetParser() antlr.Parser { return s.parser }

func (s *NumericLiteralContext) INTEGER_LITERAL() antlr.TerminalNode {
	return s.GetToken(SpelParserINTEGER_LITERAL, 0)
}

func (s *NumericLiteralContext) REAL_LITERAL() antlr.TerminalNode {
	return s.GetToken(SpelParserREAL_LITERAL, 0)
}

func (s *NumericLiteralContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NumericLiteralContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NumericLiteralContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitNumericLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) NumericLiteral() (localctx INumericLiteralContext) {
	this := p
	_ = this

	localctx = NewNumericLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, SpelParserRULE_numericLiteral)
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
		p.SetState(185)
		_la = p.GetTokenStream().LA(1)

		if !(_la == SpelParserREAL_LITERAL || _la == SpelParserINTEGER_LITERAL) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IParenspelExprContext is an interface to support dynamic dispatch.
type IParenspelExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsParenspelExprContext differentiates from other interfaces.
	IsParenspelExprContext()
}

type ParenspelExprContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyParenspelExprContext() *ParenspelExprContext {
	var p = new(ParenspelExprContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_parenspelExpr
	return p
}

func (*ParenspelExprContext) IsParenspelExprContext() {}

func NewParenspelExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ParenspelExprContext {
	var p = new(ParenspelExprContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_parenspelExpr

	return p
}

func (s *ParenspelExprContext) GetParser() antlr.Parser { return s.parser }

func (s *ParenspelExprContext) LPAREN() antlr.TerminalNode {
	return s.GetToken(SpelParserLPAREN, 0)
}

func (s *ParenspelExprContext) SpelExpr() ISpelExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISpelExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISpelExprContext)
}

func (s *ParenspelExprContext) RPAREN() antlr.TerminalNode {
	return s.GetToken(SpelParserRPAREN, 0)
}

func (s *ParenspelExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ParenspelExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ParenspelExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitParenspelExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) ParenspelExpr() (localctx IParenspelExprContext) {
	this := p
	_ = this

	localctx = NewParenspelExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 28, SpelParserRULE_parenspelExpr)

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
		p.SetState(187)
		p.Match(SpelParserLPAREN)
	}
	{
		p.SetState(188)
		p.spelExpr(0)
	}
	{
		p.SetState(189)
		p.Match(SpelParserRPAREN)
	}

	return localctx
}

// ITypeReferenceContext is an interface to support dynamic dispatch.
type ITypeReferenceContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsTypeReferenceContext differentiates from other interfaces.
	IsTypeReferenceContext()
}

type TypeReferenceContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyTypeReferenceContext() *TypeReferenceContext {
	var p = new(TypeReferenceContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_typeReference
	return p
}

func (*TypeReferenceContext) IsTypeReferenceContext() {}

func NewTypeReferenceContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *TypeReferenceContext {
	var p = new(TypeReferenceContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_typeReference

	return p
}

func (s *TypeReferenceContext) GetParser() antlr.Parser { return s.parser }

func (s *TypeReferenceContext) T() antlr.TerminalNode {
	return s.GetToken(SpelParserT, 0)
}

func (s *TypeReferenceContext) LPAREN() antlr.TerminalNode {
	return s.GetToken(SpelParserLPAREN, 0)
}

func (s *TypeReferenceContext) PossiblyQualifiedId() IPossiblyQualifiedIdContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IPossiblyQualifiedIdContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IPossiblyQualifiedIdContext)
}

func (s *TypeReferenceContext) RPAREN() antlr.TerminalNode {
	return s.GetToken(SpelParserRPAREN, 0)
}

func (s *TypeReferenceContext) AllLSQUARE() []antlr.TerminalNode {
	return s.GetTokens(SpelParserLSQUARE)
}

func (s *TypeReferenceContext) LSQUARE(i int) antlr.TerminalNode {
	return s.GetToken(SpelParserLSQUARE, i)
}

func (s *TypeReferenceContext) AllRSQUARE() []antlr.TerminalNode {
	return s.GetTokens(SpelParserRSQUARE)
}

func (s *TypeReferenceContext) RSQUARE(i int) antlr.TerminalNode {
	return s.GetToken(SpelParserRSQUARE, i)
}

func (s *TypeReferenceContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TypeReferenceContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *TypeReferenceContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitTypeReference(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) TypeReference() (localctx ITypeReferenceContext) {
	this := p
	_ = this

	localctx = NewTypeReferenceContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 30, SpelParserRULE_typeReference)
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
		p.SetState(191)
		p.Match(SpelParserT)
	}
	{
		p.SetState(192)
		p.Match(SpelParserLPAREN)
	}
	{
		p.SetState(193)
		p.PossiblyQualifiedId()
	}
	p.SetState(198)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == SpelParserLSQUARE {
		{
			p.SetState(194)
			p.Match(SpelParserLSQUARE)
		}
		{
			p.SetState(195)
			p.Match(SpelParserRSQUARE)
		}

		p.SetState(200)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(201)
		p.Match(SpelParserRPAREN)
	}

	return localctx
}

// IPossiblyQualifiedIdContext is an interface to support dynamic dispatch.
type IPossiblyQualifiedIdContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsPossiblyQualifiedIdContext differentiates from other interfaces.
	IsPossiblyQualifiedIdContext()
}

type PossiblyQualifiedIdContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyPossiblyQualifiedIdContext() *PossiblyQualifiedIdContext {
	var p = new(PossiblyQualifiedIdContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_possiblyQualifiedId
	return p
}

func (*PossiblyQualifiedIdContext) IsPossiblyQualifiedIdContext() {}

func NewPossiblyQualifiedIdContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *PossiblyQualifiedIdContext {
	var p = new(PossiblyQualifiedIdContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_possiblyQualifiedId

	return p
}

func (s *PossiblyQualifiedIdContext) GetParser() antlr.Parser { return s.parser }

func (s *PossiblyQualifiedIdContext) AllIDENTIFIER() []antlr.TerminalNode {
	return s.GetTokens(SpelParserIDENTIFIER)
}

func (s *PossiblyQualifiedIdContext) IDENTIFIER(i int) antlr.TerminalNode {
	return s.GetToken(SpelParserIDENTIFIER, i)
}

func (s *PossiblyQualifiedIdContext) AllDOT() []antlr.TerminalNode {
	return s.GetTokens(SpelParserDOT)
}

func (s *PossiblyQualifiedIdContext) DOT(i int) antlr.TerminalNode {
	return s.GetToken(SpelParserDOT, i)
}

func (s *PossiblyQualifiedIdContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PossiblyQualifiedIdContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *PossiblyQualifiedIdContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitPossiblyQualifiedId(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) PossiblyQualifiedId() (localctx IPossiblyQualifiedIdContext) {
	this := p
	_ = this

	localctx = NewPossiblyQualifiedIdContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 32, SpelParserRULE_possiblyQualifiedId)
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
		p.SetState(203)
		p.Match(SpelParserIDENTIFIER)
	}
	p.SetState(208)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == SpelParserDOT {
		{
			p.SetState(204)
			p.Match(SpelParserDOT)
		}
		{
			p.SetState(205)
			p.Match(SpelParserIDENTIFIER)
		}

		p.SetState(210)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// INullReferenceContext is an interface to support dynamic dispatch.
type INullReferenceContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNullReferenceContext differentiates from other interfaces.
	IsNullReferenceContext()
}

type NullReferenceContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNullReferenceContext() *NullReferenceContext {
	var p = new(NullReferenceContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_nullReference
	return p
}

func (*NullReferenceContext) IsNullReferenceContext() {}

func NewNullReferenceContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NullReferenceContext {
	var p = new(NullReferenceContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_nullReference

	return p
}

func (s *NullReferenceContext) GetParser() antlr.Parser { return s.parser }

func (s *NullReferenceContext) NULL() antlr.TerminalNode {
	return s.GetToken(SpelParserNULL, 0)
}

func (s *NullReferenceContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NullReferenceContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NullReferenceContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitNullReference(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) NullReference() (localctx INullReferenceContext) {
	this := p
	_ = this

	localctx = NewNullReferenceContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 34, SpelParserRULE_nullReference)

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
		p.Match(SpelParserNULL)
	}

	return localctx
}

// IConstructorReferenceContext is an interface to support dynamic dispatch.
type IConstructorReferenceContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsConstructorReferenceContext differentiates from other interfaces.
	IsConstructorReferenceContext()
}

type ConstructorReferenceContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyConstructorReferenceContext() *ConstructorReferenceContext {
	var p = new(ConstructorReferenceContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_constructorReference
	return p
}

func (*ConstructorReferenceContext) IsConstructorReferenceContext() {}

func NewConstructorReferenceContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ConstructorReferenceContext {
	var p = new(ConstructorReferenceContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_constructorReference

	return p
}

func (s *ConstructorReferenceContext) GetParser() antlr.Parser { return s.parser }

func (s *ConstructorReferenceContext) NEW() antlr.TerminalNode {
	return s.GetToken(SpelParserNEW, 0)
}

func (s *ConstructorReferenceContext) PossiblyQualifiedId() IPossiblyQualifiedIdContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IPossiblyQualifiedIdContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IPossiblyQualifiedIdContext)
}

func (s *ConstructorReferenceContext) AllLSQUARE() []antlr.TerminalNode {
	return s.GetTokens(SpelParserLSQUARE)
}

func (s *ConstructorReferenceContext) LSQUARE(i int) antlr.TerminalNode {
	return s.GetToken(SpelParserLSQUARE, i)
}

func (s *ConstructorReferenceContext) AllRSQUARE() []antlr.TerminalNode {
	return s.GetTokens(SpelParserRSQUARE)
}

func (s *ConstructorReferenceContext) RSQUARE(i int) antlr.TerminalNode {
	return s.GetToken(SpelParserRSQUARE, i)
}

func (s *ConstructorReferenceContext) InlineListOrMap() IInlineListOrMapContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IInlineListOrMapContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IInlineListOrMapContext)
}

func (s *ConstructorReferenceContext) AllSpelExpr() []ISpelExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISpelExprContext); ok {
			len++
		}
	}

	tst := make([]ISpelExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISpelExprContext); ok {
			tst[i] = t.(ISpelExprContext)
			i++
		}
	}

	return tst
}

func (s *ConstructorReferenceContext) SpelExpr(i int) ISpelExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISpelExprContext); ok {
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

	return t.(ISpelExprContext)
}

func (s *ConstructorReferenceContext) ConstructorArgs() IConstructorArgsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConstructorArgsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConstructorArgsContext)
}

func (s *ConstructorReferenceContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ConstructorReferenceContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ConstructorReferenceContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitConstructorReference(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) ConstructorReference() (localctx IConstructorReferenceContext) {
	this := p
	_ = this

	localctx = NewConstructorReferenceContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 36, SpelParserRULE_constructorReference)
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

	p.SetState(231)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 18, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(213)
			p.Match(SpelParserNEW)
		}
		{
			p.SetState(214)
			p.PossiblyQualifiedId()
		}
		p.SetState(220)
		p.GetErrorHandler().Sync(p)
		_alt = 1
		for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			switch _alt {
			case 1:
				{
					p.SetState(215)
					p.Match(SpelParserLSQUARE)
				}
				p.SetState(217)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)

				if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&2846547927534313592) != 0 {
					{
						p.SetState(216)
						p.spelExpr(0)
					}

				}
				{
					p.SetState(219)
					p.Match(SpelParserRSQUARE)
				}

			default:
				panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
			}

			p.SetState(222)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 16, p.GetParserRuleContext())
		}
		p.SetState(225)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 17, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(224)
				p.InlineListOrMap()
			}

		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(227)
			p.Match(SpelParserNEW)
		}
		{
			p.SetState(228)
			p.PossiblyQualifiedId()
		}
		{
			p.SetState(229)
			p.ConstructorArgs()
		}

	}

	return localctx
}

// IConstructorArgsContext is an interface to support dynamic dispatch.
type IConstructorArgsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsConstructorArgsContext differentiates from other interfaces.
	IsConstructorArgsContext()
}

type ConstructorArgsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyConstructorArgsContext() *ConstructorArgsContext {
	var p = new(ConstructorArgsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_constructorArgs
	return p
}

func (*ConstructorArgsContext) IsConstructorArgsContext() {}

func NewConstructorArgsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ConstructorArgsContext {
	var p = new(ConstructorArgsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_constructorArgs

	return p
}

func (s *ConstructorArgsContext) GetParser() antlr.Parser { return s.parser }

func (s *ConstructorArgsContext) LPAREN() antlr.TerminalNode {
	return s.GetToken(SpelParserLPAREN, 0)
}

func (s *ConstructorArgsContext) Args() IArgsContext {
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

func (s *ConstructorArgsContext) RPAREN() antlr.TerminalNode {
	return s.GetToken(SpelParserRPAREN, 0)
}

func (s *ConstructorArgsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ConstructorArgsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ConstructorArgsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitConstructorArgs(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) ConstructorArgs() (localctx IConstructorArgsContext) {
	this := p
	_ = this

	localctx = NewConstructorArgsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 38, SpelParserRULE_constructorArgs)

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
		p.SetState(233)
		p.Match(SpelParserLPAREN)
	}
	{
		p.SetState(234)
		p.Args()
	}
	{
		p.SetState(235)
		p.Match(SpelParserRPAREN)
	}

	return localctx
}

// IInlineListOrMapContext is an interface to support dynamic dispatch.
type IInlineListOrMapContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsInlineListOrMapContext differentiates from other interfaces.
	IsInlineListOrMapContext()
}

type InlineListOrMapContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyInlineListOrMapContext() *InlineListOrMapContext {
	var p = new(InlineListOrMapContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_inlineListOrMap
	return p
}

func (*InlineListOrMapContext) IsInlineListOrMapContext() {}

func NewInlineListOrMapContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *InlineListOrMapContext {
	var p = new(InlineListOrMapContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_inlineListOrMap

	return p
}

func (s *InlineListOrMapContext) GetParser() antlr.Parser { return s.parser }

func (s *InlineListOrMapContext) LCURLY() antlr.TerminalNode {
	return s.GetToken(SpelParserLCURLY, 0)
}

func (s *InlineListOrMapContext) RCURLY() antlr.TerminalNode {
	return s.GetToken(SpelParserRCURLY, 0)
}

func (s *InlineListOrMapContext) COLON() antlr.TerminalNode {
	return s.GetToken(SpelParserCOLON, 0)
}

func (s *InlineListOrMapContext) ListBindings() IListBindingsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IListBindingsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IListBindingsContext)
}

func (s *InlineListOrMapContext) MapBindings() IMapBindingsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IMapBindingsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IMapBindingsContext)
}

func (s *InlineListOrMapContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *InlineListOrMapContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *InlineListOrMapContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitInlineListOrMap(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) InlineListOrMap() (localctx IInlineListOrMapContext) {
	this := p
	_ = this

	localctx = NewInlineListOrMapContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 40, SpelParserRULE_inlineListOrMap)

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

	p.SetState(250)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 19, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(237)
			p.Match(SpelParserLCURLY)
		}
		{
			p.SetState(238)
			p.Match(SpelParserRCURLY)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(239)
			p.Match(SpelParserLCURLY)
		}
		{
			p.SetState(240)
			p.Match(SpelParserCOLON)
		}
		{
			p.SetState(241)
			p.Match(SpelParserRCURLY)
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(242)
			p.Match(SpelParserLCURLY)
		}
		{
			p.SetState(243)
			p.ListBindings()
		}
		{
			p.SetState(244)
			p.Match(SpelParserRCURLY)
		}

	case 4:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(246)
			p.Match(SpelParserLCURLY)
		}
		{
			p.SetState(247)
			p.MapBindings()
		}
		{
			p.SetState(248)
			p.Match(SpelParserRCURLY)
		}

	}

	return localctx
}

// IListBindingsContext is an interface to support dynamic dispatch.
type IListBindingsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Get_listBinding returns the _listBinding rule contexts.
	Get_listBinding() IListBindingContext

	// Set_listBinding sets the _listBinding rule contexts.
	Set_listBinding(IListBindingContext)

	// GetBindings returns the bindings rule context list.
	GetBindings() []IListBindingContext

	// SetBindings sets the bindings rule context list.
	SetBindings([]IListBindingContext)

	// IsListBindingsContext differentiates from other interfaces.
	IsListBindingsContext()
}

type ListBindingsContext struct {
	*antlr.BaseParserRuleContext
	parser       antlr.Parser
	_listBinding IListBindingContext
	bindings     []IListBindingContext
}

func NewEmptyListBindingsContext() *ListBindingsContext {
	var p = new(ListBindingsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_listBindings
	return p
}

func (*ListBindingsContext) IsListBindingsContext() {}

func NewListBindingsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ListBindingsContext {
	var p = new(ListBindingsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_listBindings

	return p
}

func (s *ListBindingsContext) GetParser() antlr.Parser { return s.parser }

func (s *ListBindingsContext) Get_listBinding() IListBindingContext { return s._listBinding }

func (s *ListBindingsContext) Set_listBinding(v IListBindingContext) { s._listBinding = v }

func (s *ListBindingsContext) GetBindings() []IListBindingContext { return s.bindings }

func (s *ListBindingsContext) SetBindings(v []IListBindingContext) { s.bindings = v }

func (s *ListBindingsContext) AllListBinding() []IListBindingContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IListBindingContext); ok {
			len++
		}
	}

	tst := make([]IListBindingContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IListBindingContext); ok {
			tst[i] = t.(IListBindingContext)
			i++
		}
	}

	return tst
}

func (s *ListBindingsContext) ListBinding(i int) IListBindingContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IListBindingContext); ok {
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

	return t.(IListBindingContext)
}

func (s *ListBindingsContext) AllCOMMA() []antlr.TerminalNode {
	return s.GetTokens(SpelParserCOMMA)
}

func (s *ListBindingsContext) COMMA(i int) antlr.TerminalNode {
	return s.GetToken(SpelParserCOMMA, i)
}

func (s *ListBindingsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ListBindingsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ListBindingsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitListBindings(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) ListBindings() (localctx IListBindingsContext) {
	this := p
	_ = this

	localctx = NewListBindingsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 42, SpelParserRULE_listBindings)
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
		p.SetState(252)

		var _x = p.ListBinding()

		localctx.(*ListBindingsContext)._listBinding = _x
	}
	localctx.(*ListBindingsContext).bindings = append(localctx.(*ListBindingsContext).bindings, localctx.(*ListBindingsContext)._listBinding)
	p.SetState(257)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == SpelParserCOMMA {
		{
			p.SetState(253)
			p.Match(SpelParserCOMMA)
		}
		{
			p.SetState(254)

			var _x = p.ListBinding()

			localctx.(*ListBindingsContext)._listBinding = _x
		}
		localctx.(*ListBindingsContext).bindings = append(localctx.(*ListBindingsContext).bindings, localctx.(*ListBindingsContext)._listBinding)

		p.SetState(259)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// IListBindingContext is an interface to support dynamic dispatch.
type IListBindingContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsListBindingContext differentiates from other interfaces.
	IsListBindingContext()
}

type ListBindingContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyListBindingContext() *ListBindingContext {
	var p = new(ListBindingContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_listBinding
	return p
}

func (*ListBindingContext) IsListBindingContext() {}

func NewListBindingContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ListBindingContext {
	var p = new(ListBindingContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_listBinding

	return p
}

func (s *ListBindingContext) GetParser() antlr.Parser { return s.parser }

func (s *ListBindingContext) SpelExpr() ISpelExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISpelExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISpelExprContext)
}

func (s *ListBindingContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ListBindingContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ListBindingContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitListBinding(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) ListBinding() (localctx IListBindingContext) {
	this := p
	_ = this

	localctx = NewListBindingContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 44, SpelParserRULE_listBinding)

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
		p.SetState(260)
		p.spelExpr(0)
	}

	return localctx
}

// IMapBindingsContext is an interface to support dynamic dispatch.
type IMapBindingsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Get_mapBinding returns the _mapBinding rule contexts.
	Get_mapBinding() IMapBindingContext

	// Set_mapBinding sets the _mapBinding rule contexts.
	Set_mapBinding(IMapBindingContext)

	// GetBindings returns the bindings rule context list.
	GetBindings() []IMapBindingContext

	// SetBindings sets the bindings rule context list.
	SetBindings([]IMapBindingContext)

	// IsMapBindingsContext differentiates from other interfaces.
	IsMapBindingsContext()
}

type MapBindingsContext struct {
	*antlr.BaseParserRuleContext
	parser      antlr.Parser
	_mapBinding IMapBindingContext
	bindings    []IMapBindingContext
}

func NewEmptyMapBindingsContext() *MapBindingsContext {
	var p = new(MapBindingsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_mapBindings
	return p
}

func (*MapBindingsContext) IsMapBindingsContext() {}

func NewMapBindingsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *MapBindingsContext {
	var p = new(MapBindingsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_mapBindings

	return p
}

func (s *MapBindingsContext) GetParser() antlr.Parser { return s.parser }

func (s *MapBindingsContext) Get_mapBinding() IMapBindingContext { return s._mapBinding }

func (s *MapBindingsContext) Set_mapBinding(v IMapBindingContext) { s._mapBinding = v }

func (s *MapBindingsContext) GetBindings() []IMapBindingContext { return s.bindings }

func (s *MapBindingsContext) SetBindings(v []IMapBindingContext) { s.bindings = v }

func (s *MapBindingsContext) AllMapBinding() []IMapBindingContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IMapBindingContext); ok {
			len++
		}
	}

	tst := make([]IMapBindingContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IMapBindingContext); ok {
			tst[i] = t.(IMapBindingContext)
			i++
		}
	}

	return tst
}

func (s *MapBindingsContext) MapBinding(i int) IMapBindingContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IMapBindingContext); ok {
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

	return t.(IMapBindingContext)
}

func (s *MapBindingsContext) AllCOMMA() []antlr.TerminalNode {
	return s.GetTokens(SpelParserCOMMA)
}

func (s *MapBindingsContext) COMMA(i int) antlr.TerminalNode {
	return s.GetToken(SpelParserCOMMA, i)
}

func (s *MapBindingsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *MapBindingsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *MapBindingsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitMapBindings(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) MapBindings() (localctx IMapBindingsContext) {
	this := p
	_ = this

	localctx = NewMapBindingsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 46, SpelParserRULE_mapBindings)
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
		p.SetState(262)

		var _x = p.MapBinding()

		localctx.(*MapBindingsContext)._mapBinding = _x
	}
	localctx.(*MapBindingsContext).bindings = append(localctx.(*MapBindingsContext).bindings, localctx.(*MapBindingsContext)._mapBinding)
	p.SetState(267)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == SpelParserCOMMA {
		{
			p.SetState(263)
			p.Match(SpelParserCOMMA)
		}
		{
			p.SetState(264)

			var _x = p.MapBinding()

			localctx.(*MapBindingsContext)._mapBinding = _x
		}
		localctx.(*MapBindingsContext).bindings = append(localctx.(*MapBindingsContext).bindings, localctx.(*MapBindingsContext)._mapBinding)

		p.SetState(269)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// IMapBindingContext is an interface to support dynamic dispatch.
type IMapBindingContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// GetKey returns the key rule contexts.
	GetKey() ISpelExprContext

	// GetValue returns the value rule contexts.
	GetValue() ISpelExprContext

	// SetKey sets the key rule contexts.
	SetKey(ISpelExprContext)

	// SetValue sets the value rule contexts.
	SetValue(ISpelExprContext)

	// IsMapBindingContext differentiates from other interfaces.
	IsMapBindingContext()
}

type MapBindingContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
	key    ISpelExprContext
	value  ISpelExprContext
}

func NewEmptyMapBindingContext() *MapBindingContext {
	var p = new(MapBindingContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_mapBinding
	return p
}

func (*MapBindingContext) IsMapBindingContext() {}

func NewMapBindingContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *MapBindingContext {
	var p = new(MapBindingContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_mapBinding

	return p
}

func (s *MapBindingContext) GetParser() antlr.Parser { return s.parser }

func (s *MapBindingContext) GetKey() ISpelExprContext { return s.key }

func (s *MapBindingContext) GetValue() ISpelExprContext { return s.value }

func (s *MapBindingContext) SetKey(v ISpelExprContext) { s.key = v }

func (s *MapBindingContext) SetValue(v ISpelExprContext) { s.value = v }

func (s *MapBindingContext) COLON() antlr.TerminalNode {
	return s.GetToken(SpelParserCOLON, 0)
}

func (s *MapBindingContext) AllSpelExpr() []ISpelExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISpelExprContext); ok {
			len++
		}
	}

	tst := make([]ISpelExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISpelExprContext); ok {
			tst[i] = t.(ISpelExprContext)
			i++
		}
	}

	return tst
}

func (s *MapBindingContext) SpelExpr(i int) ISpelExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISpelExprContext); ok {
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

	return t.(ISpelExprContext)
}

func (s *MapBindingContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *MapBindingContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *MapBindingContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitMapBinding(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) MapBinding() (localctx IMapBindingContext) {
	this := p
	_ = this

	localctx = NewMapBindingContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 48, SpelParserRULE_mapBinding)

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
		p.SetState(270)

		var _x = p.spelExpr(0)

		localctx.(*MapBindingContext).key = _x
	}
	{
		p.SetState(271)
		p.Match(SpelParserCOLON)
	}
	{
		p.SetState(272)

		var _x = p.spelExpr(0)

		localctx.(*MapBindingContext).value = _x
	}

	return localctx
}

// IBeanReferenceContext is an interface to support dynamic dispatch.
type IBeanReferenceContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsBeanReferenceContext differentiates from other interfaces.
	IsBeanReferenceContext()
}

type BeanReferenceContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyBeanReferenceContext() *BeanReferenceContext {
	var p = new(BeanReferenceContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_beanReference
	return p
}

func (*BeanReferenceContext) IsBeanReferenceContext() {}

func NewBeanReferenceContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *BeanReferenceContext {
	var p = new(BeanReferenceContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_beanReference

	return p
}

func (s *BeanReferenceContext) GetParser() antlr.Parser { return s.parser }

func (s *BeanReferenceContext) BEAN_REF() antlr.TerminalNode {
	return s.GetToken(SpelParserBEAN_REF, 0)
}

func (s *BeanReferenceContext) FACTORY_BEAN_REF() antlr.TerminalNode {
	return s.GetToken(SpelParserFACTORY_BEAN_REF, 0)
}

func (s *BeanReferenceContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(SpelParserIDENTIFIER, 0)
}

func (s *BeanReferenceContext) STRING_LITERAL() antlr.TerminalNode {
	return s.GetToken(SpelParserSTRING_LITERAL, 0)
}

func (s *BeanReferenceContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BeanReferenceContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *BeanReferenceContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitBeanReference(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) BeanReference() (localctx IBeanReferenceContext) {
	this := p
	_ = this

	localctx = NewBeanReferenceContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 50, SpelParserRULE_beanReference)
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
		p.SetState(274)
		_la = p.GetTokenStream().LA(1)

		if !(_la == SpelParserBEAN_REF || _la == SpelParserFACTORY_BEAN_REF) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}
	{
		p.SetState(275)
		_la = p.GetTokenStream().LA(1)

		if !(_la == SpelParserIDENTIFIER || _la == SpelParserSTRING_LITERAL) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IInputParameterContext is an interface to support dynamic dispatch.
type IInputParameterContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsInputParameterContext differentiates from other interfaces.
	IsInputParameterContext()
}

type InputParameterContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyInputParameterContext() *InputParameterContext {
	var p = new(InputParameterContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_inputParameter
	return p
}

func (*InputParameterContext) IsInputParameterContext() {}

func NewInputParameterContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *InputParameterContext {
	var p = new(InputParameterContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_inputParameter

	return p
}

func (s *InputParameterContext) GetParser() antlr.Parser { return s.parser }

func (s *InputParameterContext) LSQUARE() antlr.TerminalNode {
	return s.GetToken(SpelParserLSQUARE, 0)
}

func (s *InputParameterContext) INTEGER_LITERAL() antlr.TerminalNode {
	return s.GetToken(SpelParserINTEGER_LITERAL, 0)
}

func (s *InputParameterContext) RSQUARE() antlr.TerminalNode {
	return s.GetToken(SpelParserRSQUARE, 0)
}

func (s *InputParameterContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *InputParameterContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *InputParameterContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitInputParameter(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) InputParameter() (localctx IInputParameterContext) {
	this := p
	_ = this

	localctx = NewInputParameterContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 52, SpelParserRULE_inputParameter)

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
		p.Match(SpelParserLSQUARE)
	}
	{
		p.SetState(278)
		p.Match(SpelParserINTEGER_LITERAL)
	}
	{
		p.SetState(279)
		p.Match(SpelParserRSQUARE)
	}

	return localctx
}

// IPropertyPlaceHolderContext is an interface to support dynamic dispatch.
type IPropertyPlaceHolderContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsPropertyPlaceHolderContext differentiates from other interfaces.
	IsPropertyPlaceHolderContext()
}

type PropertyPlaceHolderContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyPropertyPlaceHolderContext() *PropertyPlaceHolderContext {
	var p = new(PropertyPlaceHolderContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SpelParserRULE_propertyPlaceHolder
	return p
}

func (*PropertyPlaceHolderContext) IsPropertyPlaceHolderContext() {}

func NewPropertyPlaceHolderContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *PropertyPlaceHolderContext {
	var p = new(PropertyPlaceHolderContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SpelParserRULE_propertyPlaceHolder

	return p
}

func (s *PropertyPlaceHolderContext) GetParser() antlr.Parser { return s.parser }

func (s *PropertyPlaceHolderContext) PROPERTY_PLACE_HOLDER() antlr.TerminalNode {
	return s.GetToken(SpelParserPROPERTY_PLACE_HOLDER, 0)
}

func (s *PropertyPlaceHolderContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PropertyPlaceHolderContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *PropertyPlaceHolderContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SpelParserVisitor:
		return t.VisitPropertyPlaceHolder(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SpelParser) PropertyPlaceHolder() (localctx IPropertyPlaceHolderContext) {
	this := p
	_ = this

	localctx = NewPropertyPlaceHolderContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 54, SpelParserRULE_propertyPlaceHolder)

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
		p.SetState(281)
		p.Match(SpelParserPROPERTY_PLACE_HOLDER)
	}

	return localctx
}

func (p *SpelParser) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 1:
		var t *SpelExprContext = nil
		if localctx != nil {
			t = localctx.(*SpelExprContext)
		}
		return p.SpelExpr_Sempred(t, predIndex)

	default:
		panic("No predicate with index: " + fmt.Sprint(ruleIndex))
	}
}

func (p *SpelParser) SpelExpr_Sempred(localctx antlr.RuleContext, predIndex int) bool {
	this := p
	_ = this

	switch predIndex {
	case 0:
		return p.Precpred(p.GetParserRuleContext(), 11)

	case 1:
		return p.Precpred(p.GetParserRuleContext(), 10)

	case 2:
		return p.Precpred(p.GetParserRuleContext(), 9)

	case 3:
		return p.Precpred(p.GetParserRuleContext(), 8)

	case 4:
		return p.Precpred(p.GetParserRuleContext(), 7)

	case 5:
		return p.Precpred(p.GetParserRuleContext(), 6)

	case 6:
		return p.Precpred(p.GetParserRuleContext(), 5)

	case 7:
		return p.Precpred(p.GetParserRuleContext(), 4)

	case 8:
		return p.Precpred(p.GetParserRuleContext(), 3)

	case 9:
		return p.Precpred(p.GetParserRuleContext(), 2)

	case 10:
		return p.Precpred(p.GetParserRuleContext(), 12)

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}
