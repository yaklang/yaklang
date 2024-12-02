// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package freemarkerparser // FreemarkerParser
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

type FreemarkerParser struct {
	*antlr.BaseParser
}

var freemarkerparserParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	literalNames           []string
	symbolicNames          []string
	ruleNames              []string
	predictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func freemarkerparserParserInit() {
	staticData := &freemarkerparserParserStaticData
	staticData.literalNames = []string{
		"", "", "'<#'", "'</#'", "'<@'", "'</@'", "", "", "", "", "", "", "",
		"", "", "", "'if'", "'else'", "'elseif'", "'assign'", "'as'", "'list'",
		"'true'", "'false'", "'include'", "'import'", "'macro'", "'nested'",
		"'return'", "'<'", "'lt'", "'<='", "'lte'", "'gt'", "'>='", "'gte'",
		"", "'}'", "'>'", "'/>'", "", "", "", "", "", "'@'", "'??'", "'?'",
		"'!'", "'+'", "'-'", "'*'", "'/'", "'%'", "'('", "')'", "'['", "']'",
		"'=='", "'='", "'!='", "'&&'", "'||'", "'.'", "','", "':'", "';'",
	}
	staticData.symbolicNames = []string{
		"", "COMMENT", "START_DIRECTIVE_TAG", "END_DIRECTIVE_TAG", "START_USER_DIR_TAG",
		"END_USER_DIR_TAG", "INLINE_EXPR_START", "CONTENT", "DQS_EXIT", "DQS_ESCAPE",
		"DQS_ENTER_EXPR", "DQS_CONTENT", "SQS_EXIT", "SQS_ESCAPE", "SQS_ENTER_EXPR",
		"SQS_CONTENT", "EXPR_IF", "EXPR_ELSE", "EXPR_ELSEIF", "EXPR_ASSIGN",
		"EXPR_AS", "EXPR_LIST", "EXPR_TRUE", "EXPR_FALSE", "EXPR_INCLUDE", "EXPR_IMPORT",
		"EXPR_MACRO", "EXPR_NESTED", "EXPR_RETURN", "EXPR_LT_SYM", "EXPR_LT_STR",
		"EXPR_LTE_SYM", "EXPR_LTE_STR", "EXPR_GT_STR", "EXPR_GTE_SYM", "EXPR_GTE_STR",
		"EXPR_NUM", "EXPR_EXIT_R_BRACE", "EXPR_EXIT_GT", "EXPR_EXIT_DIV_GT",
		"EXPR_WS", "EXPR_COMENT", "EXPR_STRUCT", "EXPR_DOUBLE_STR_START", "EXPR_SINGLE_STR_START",
		"EXPR_AT", "EXPR_DBL_QUESTION", "EXPR_QUESTION", "EXPR_BANG", "EXPR_ADD",
		"EXPR_SUB", "EXPR_MUL", "EXPR_DIV", "EXPR_MOD", "EXPR_L_PAREN", "EXPR_R_PAREN",
		"EXPR_L_SQ_PAREN", "EXPR_R_SQ_PAREN", "EXPR_COMPARE_EQ", "EXPR_EQ",
		"EXPR_COMPARE_NEQ", "EXPR_LOGICAL_AND", "EXPR_LOGICAL_OR", "EXPR_DOT",
		"EXPR_COMMA", "EXPR_COLON", "EXPR_SEMICOLON", "EXPR_SYMBOL",
	}
	staticData.ruleNames = []string{
		"template", "elements", "element", "rawText", "directive", "directiveIf",
		"directiveIfTrueElements", "directiveIfElseIfElements", "directiveIfElseElements",
		"tagExprElseIfs", "directiveAssign", "directiveList", "directiveListBodyElements",
		"directiveListElseElements", "directiveInclude", "directiveImport",
		"directiveMacro", "directiveNested", "directiveReturn", "directiveUser",
		"directiveUserId", "directiveUserParams", "directiveUserLoopParams",
		"tagExpr", "inlineExpr", "string", "expr", "functionParams", "booleanRelationalOperator",
		"struct", "struct_pair", "single_quote_string", "double_quote_string",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 67, 422, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7, 20, 2,
		21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25, 2, 26,
		7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 2, 29, 7, 29, 2, 30, 7, 30, 2, 31, 7,
		31, 2, 32, 7, 32, 1, 0, 1, 0, 1, 0, 1, 1, 5, 1, 71, 8, 1, 10, 1, 12, 1,
		74, 9, 1, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 3, 2, 82, 8, 2, 1, 3, 4,
		3, 85, 8, 3, 11, 3, 12, 3, 86, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4,
		1, 4, 1, 4, 3, 4, 98, 8, 4, 1, 5, 1, 5, 1, 5, 1, 5, 1, 5, 1, 5, 1, 5, 1,
		5, 1, 5, 1, 5, 1, 5, 5, 5, 111, 8, 5, 10, 5, 12, 5, 114, 9, 5, 1, 5, 1,
		5, 1, 5, 1, 5, 3, 5, 120, 8, 5, 1, 5, 1, 5, 1, 5, 1, 5, 1, 6, 1, 6, 1,
		7, 1, 7, 1, 8, 1, 8, 1, 9, 1, 9, 1, 10, 1, 10, 1, 10, 1, 10, 1, 10, 1,
		10, 1, 10, 1, 10, 1, 10, 1, 10, 1, 10, 1, 10, 1, 10, 1, 10, 1, 10, 1, 10,
		3, 10, 150, 8, 10, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1,
		11, 3, 11, 160, 8, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 3, 11,
		168, 8, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1, 12, 1, 12, 1, 13, 1, 13, 1,
		14, 1, 14, 1, 14, 1, 14, 1, 14, 1, 15, 1, 15, 1, 15, 1, 15, 1, 15, 1, 15,
		1, 15, 1, 16, 1, 16, 1, 16, 1, 16, 5, 16, 194, 8, 16, 10, 16, 12, 16, 197,
		9, 16, 1, 16, 1, 16, 1, 16, 1, 16, 1, 16, 1, 16, 1, 17, 1, 17, 1, 17, 1,
		17, 1, 17, 5, 17, 210, 8, 17, 10, 17, 12, 17, 213, 9, 17, 3, 17, 215, 8,
		17, 1, 17, 1, 17, 1, 18, 1, 18, 1, 18, 1, 18, 1, 19, 1, 19, 1, 19, 1, 19,
		1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1,
		19, 1, 19, 3, 19, 239, 8, 19, 1, 20, 1, 20, 1, 20, 5, 20, 244, 8, 20, 10,
		20, 12, 20, 247, 9, 20, 1, 21, 1, 21, 1, 21, 5, 21, 252, 8, 21, 10, 21,
		12, 21, 255, 9, 21, 1, 21, 1, 21, 3, 21, 259, 8, 21, 1, 21, 5, 21, 262,
		8, 21, 10, 21, 12, 21, 265, 9, 21, 3, 21, 267, 8, 21, 3, 21, 269, 8, 21,
		1, 22, 1, 22, 1, 22, 1, 22, 5, 22, 275, 8, 22, 10, 22, 12, 22, 278, 9,
		22, 3, 22, 280, 8, 22, 1, 23, 1, 23, 1, 24, 1, 24, 1, 25, 1, 25, 3, 25,
		288, 8, 25, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1,
		26, 1, 26, 1, 26, 1, 26, 3, 26, 302, 8, 26, 1, 26, 1, 26, 1, 26, 1, 26,
		1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1,
		26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 4, 26, 326, 8, 26,
		11, 26, 12, 26, 327, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1,
		26, 1, 26, 3, 26, 339, 8, 26, 1, 26, 1, 26, 1, 26, 3, 26, 344, 8, 26, 1,
		26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 5, 26,
		356, 8, 26, 10, 26, 12, 26, 359, 9, 26, 1, 27, 1, 27, 1, 27, 1, 27, 1,
		27, 4, 27, 366, 8, 27, 11, 27, 12, 27, 367, 3, 27, 370, 8, 27, 1, 28, 1,
		28, 1, 29, 1, 29, 1, 29, 1, 29, 5, 29, 378, 8, 29, 10, 29, 12, 29, 381,
		9, 29, 3, 29, 383, 8, 29, 1, 29, 1, 29, 1, 30, 1, 30, 3, 30, 389, 8, 30,
		1, 30, 1, 30, 1, 30, 1, 31, 1, 31, 1, 31, 1, 31, 1, 31, 1, 31, 1, 31, 5,
		31, 401, 8, 31, 10, 31, 12, 31, 404, 9, 31, 1, 31, 1, 31, 1, 32, 1, 32,
		1, 32, 1, 32, 1, 32, 1, 32, 1, 32, 5, 32, 415, 8, 32, 10, 32, 12, 32, 418,
		9, 32, 1, 32, 1, 32, 1, 32, 0, 1, 52, 33, 0, 2, 4, 6, 8, 10, 12, 14, 16,
		18, 20, 22, 24, 26, 28, 30, 32, 34, 36, 38, 40, 42, 44, 46, 48, 50, 52,
		54, 56, 58, 60, 62, 64, 0, 7, 1, 0, 38, 39, 1, 0, 22, 23, 2, 0, 48, 48,
		50, 50, 1, 0, 51, 53, 1, 0, 49, 50, 1, 0, 58, 60, 1, 0, 29, 35, 451, 0,
		66, 1, 0, 0, 0, 2, 72, 1, 0, 0, 0, 4, 81, 1, 0, 0, 0, 6, 84, 1, 0, 0, 0,
		8, 97, 1, 0, 0, 0, 10, 99, 1, 0, 0, 0, 12, 125, 1, 0, 0, 0, 14, 127, 1,
		0, 0, 0, 16, 129, 1, 0, 0, 0, 18, 131, 1, 0, 0, 0, 20, 149, 1, 0, 0, 0,
		22, 151, 1, 0, 0, 0, 24, 173, 1, 0, 0, 0, 26, 175, 1, 0, 0, 0, 28, 177,
		1, 0, 0, 0, 30, 182, 1, 0, 0, 0, 32, 189, 1, 0, 0, 0, 34, 204, 1, 0, 0,
		0, 36, 218, 1, 0, 0, 0, 38, 238, 1, 0, 0, 0, 40, 240, 1, 0, 0, 0, 42, 268,
		1, 0, 0, 0, 44, 279, 1, 0, 0, 0, 46, 281, 1, 0, 0, 0, 48, 283, 1, 0, 0,
		0, 50, 287, 1, 0, 0, 0, 52, 301, 1, 0, 0, 0, 54, 369, 1, 0, 0, 0, 56, 371,
		1, 0, 0, 0, 58, 373, 1, 0, 0, 0, 60, 388, 1, 0, 0, 0, 62, 393, 1, 0, 0,
		0, 64, 407, 1, 0, 0, 0, 66, 67, 3, 2, 1, 0, 67, 68, 5, 0, 0, 1, 68, 1,
		1, 0, 0, 0, 69, 71, 3, 4, 2, 0, 70, 69, 1, 0, 0, 0, 71, 74, 1, 0, 0, 0,
		72, 70, 1, 0, 0, 0, 72, 73, 1, 0, 0, 0, 73, 3, 1, 0, 0, 0, 74, 72, 1, 0,
		0, 0, 75, 82, 3, 6, 3, 0, 76, 82, 3, 8, 4, 0, 77, 78, 5, 6, 0, 0, 78, 79,
		3, 48, 24, 0, 79, 80, 5, 37, 0, 0, 80, 82, 1, 0, 0, 0, 81, 75, 1, 0, 0,
		0, 81, 76, 1, 0, 0, 0, 81, 77, 1, 0, 0, 0, 82, 5, 1, 0, 0, 0, 83, 85, 5,
		7, 0, 0, 84, 83, 1, 0, 0, 0, 85, 86, 1, 0, 0, 0, 86, 84, 1, 0, 0, 0, 86,
		87, 1, 0, 0, 0, 87, 7, 1, 0, 0, 0, 88, 98, 3, 10, 5, 0, 89, 98, 3, 20,
		10, 0, 90, 98, 3, 22, 11, 0, 91, 98, 3, 28, 14, 0, 92, 98, 3, 30, 15, 0,
		93, 98, 3, 32, 16, 0, 94, 98, 3, 34, 17, 0, 95, 98, 3, 36, 18, 0, 96, 98,
		3, 38, 19, 0, 97, 88, 1, 0, 0, 0, 97, 89, 1, 0, 0, 0, 97, 90, 1, 0, 0,
		0, 97, 91, 1, 0, 0, 0, 97, 92, 1, 0, 0, 0, 97, 93, 1, 0, 0, 0, 97, 94,
		1, 0, 0, 0, 97, 95, 1, 0, 0, 0, 97, 96, 1, 0, 0, 0, 98, 9, 1, 0, 0, 0,
		99, 100, 5, 2, 0, 0, 100, 101, 5, 16, 0, 0, 101, 102, 3, 46, 23, 0, 102,
		103, 5, 38, 0, 0, 103, 112, 3, 12, 6, 0, 104, 105, 5, 2, 0, 0, 105, 106,
		5, 18, 0, 0, 106, 107, 3, 18, 9, 0, 107, 108, 5, 38, 0, 0, 108, 109, 3,
		14, 7, 0, 109, 111, 1, 0, 0, 0, 110, 104, 1, 0, 0, 0, 111, 114, 1, 0, 0,
		0, 112, 110, 1, 0, 0, 0, 112, 113, 1, 0, 0, 0, 113, 119, 1, 0, 0, 0, 114,
		112, 1, 0, 0, 0, 115, 116, 5, 2, 0, 0, 116, 117, 5, 17, 0, 0, 117, 118,
		5, 38, 0, 0, 118, 120, 3, 16, 8, 0, 119, 115, 1, 0, 0, 0, 119, 120, 1,
		0, 0, 0, 120, 121, 1, 0, 0, 0, 121, 122, 5, 3, 0, 0, 122, 123, 5, 16, 0,
		0, 123, 124, 5, 38, 0, 0, 124, 11, 1, 0, 0, 0, 125, 126, 3, 2, 1, 0, 126,
		13, 1, 0, 0, 0, 127, 128, 3, 2, 1, 0, 128, 15, 1, 0, 0, 0, 129, 130, 3,
		2, 1, 0, 130, 17, 1, 0, 0, 0, 131, 132, 3, 46, 23, 0, 132, 19, 1, 0, 0,
		0, 133, 134, 5, 2, 0, 0, 134, 135, 5, 19, 0, 0, 135, 136, 5, 67, 0, 0,
		136, 137, 5, 59, 0, 0, 137, 138, 3, 46, 23, 0, 138, 139, 7, 0, 0, 0, 139,
		150, 1, 0, 0, 0, 140, 141, 5, 2, 0, 0, 141, 142, 5, 19, 0, 0, 142, 143,
		5, 67, 0, 0, 143, 144, 5, 38, 0, 0, 144, 145, 3, 2, 1, 0, 145, 146, 5,
		3, 0, 0, 146, 147, 5, 19, 0, 0, 147, 148, 5, 38, 0, 0, 148, 150, 1, 0,
		0, 0, 149, 133, 1, 0, 0, 0, 149, 140, 1, 0, 0, 0, 150, 21, 1, 0, 0, 0,
		151, 152, 5, 2, 0, 0, 152, 153, 5, 21, 0, 0, 153, 154, 3, 46, 23, 0, 154,
		159, 5, 20, 0, 0, 155, 160, 5, 67, 0, 0, 156, 157, 5, 67, 0, 0, 157, 158,
		5, 64, 0, 0, 158, 160, 5, 67, 0, 0, 159, 155, 1, 0, 0, 0, 159, 156, 1,
		0, 0, 0, 160, 161, 1, 0, 0, 0, 161, 162, 5, 38, 0, 0, 162, 167, 3, 24,
		12, 0, 163, 164, 5, 2, 0, 0, 164, 165, 5, 17, 0, 0, 165, 166, 5, 38, 0,
		0, 166, 168, 3, 26, 13, 0, 167, 163, 1, 0, 0, 0, 167, 168, 1, 0, 0, 0,
		168, 169, 1, 0, 0, 0, 169, 170, 5, 3, 0, 0, 170, 171, 5, 21, 0, 0, 171,
		172, 5, 38, 0, 0, 172, 23, 1, 0, 0, 0, 173, 174, 3, 2, 1, 0, 174, 25, 1,
		0, 0, 0, 175, 176, 3, 2, 1, 0, 176, 27, 1, 0, 0, 0, 177, 178, 5, 2, 0,
		0, 178, 179, 5, 24, 0, 0, 179, 180, 3, 50, 25, 0, 180, 181, 5, 38, 0, 0,
		181, 29, 1, 0, 0, 0, 182, 183, 5, 2, 0, 0, 183, 184, 5, 25, 0, 0, 184,
		185, 3, 50, 25, 0, 185, 186, 5, 20, 0, 0, 186, 187, 5, 67, 0, 0, 187, 188,
		5, 38, 0, 0, 188, 31, 1, 0, 0, 0, 189, 190, 5, 2, 0, 0, 190, 191, 5, 26,
		0, 0, 191, 195, 5, 67, 0, 0, 192, 194, 5, 67, 0, 0, 193, 192, 1, 0, 0,
		0, 194, 197, 1, 0, 0, 0, 195, 193, 1, 0, 0, 0, 195, 196, 1, 0, 0, 0, 196,
		198, 1, 0, 0, 0, 197, 195, 1, 0, 0, 0, 198, 199, 5, 38, 0, 0, 199, 200,
		3, 2, 1, 0, 200, 201, 5, 3, 0, 0, 201, 202, 5, 26, 0, 0, 202, 203, 5, 38,
		0, 0, 203, 33, 1, 0, 0, 0, 204, 205, 5, 2, 0, 0, 205, 214, 5, 27, 0, 0,
		206, 211, 3, 52, 26, 0, 207, 208, 5, 64, 0, 0, 208, 210, 3, 52, 26, 0,
		209, 207, 1, 0, 0, 0, 210, 213, 1, 0, 0, 0, 211, 209, 1, 0, 0, 0, 211,
		212, 1, 0, 0, 0, 212, 215, 1, 0, 0, 0, 213, 211, 1, 0, 0, 0, 214, 206,
		1, 0, 0, 0, 214, 215, 1, 0, 0, 0, 215, 216, 1, 0, 0, 0, 216, 217, 5, 38,
		0, 0, 217, 35, 1, 0, 0, 0, 218, 219, 5, 2, 0, 0, 219, 220, 5, 28, 0, 0,
		220, 221, 5, 38, 0, 0, 221, 37, 1, 0, 0, 0, 222, 223, 5, 4, 0, 0, 223,
		224, 3, 40, 20, 0, 224, 225, 3, 42, 21, 0, 225, 226, 3, 44, 22, 0, 226,
		227, 5, 39, 0, 0, 227, 239, 1, 0, 0, 0, 228, 229, 5, 4, 0, 0, 229, 230,
		3, 40, 20, 0, 230, 231, 3, 42, 21, 0, 231, 232, 3, 44, 22, 0, 232, 233,
		5, 38, 0, 0, 233, 234, 3, 2, 1, 0, 234, 235, 5, 5, 0, 0, 235, 236, 3, 40,
		20, 0, 236, 237, 5, 38, 0, 0, 237, 239, 1, 0, 0, 0, 238, 222, 1, 0, 0,
		0, 238, 228, 1, 0, 0, 0, 239, 39, 1, 0, 0, 0, 240, 245, 5, 67, 0, 0, 241,
		242, 5, 63, 0, 0, 242, 244, 5, 67, 0, 0, 243, 241, 1, 0, 0, 0, 244, 247,
		1, 0, 0, 0, 245, 243, 1, 0, 0, 0, 245, 246, 1, 0, 0, 0, 246, 41, 1, 0,
		0, 0, 247, 245, 1, 0, 0, 0, 248, 249, 5, 67, 0, 0, 249, 250, 5, 59, 0,
		0, 250, 252, 3, 52, 26, 0, 251, 248, 1, 0, 0, 0, 252, 255, 1, 0, 0, 0,
		253, 251, 1, 0, 0, 0, 253, 254, 1, 0, 0, 0, 254, 269, 1, 0, 0, 0, 255,
		253, 1, 0, 0, 0, 256, 263, 3, 52, 26, 0, 257, 259, 5, 64, 0, 0, 258, 257,
		1, 0, 0, 0, 258, 259, 1, 0, 0, 0, 259, 260, 1, 0, 0, 0, 260, 262, 3, 52,
		26, 0, 261, 258, 1, 0, 0, 0, 262, 265, 1, 0, 0, 0, 263, 261, 1, 0, 0, 0,
		263, 264, 1, 0, 0, 0, 264, 267, 1, 0, 0, 0, 265, 263, 1, 0, 0, 0, 266,
		256, 1, 0, 0, 0, 266, 267, 1, 0, 0, 0, 267, 269, 1, 0, 0, 0, 268, 253,
		1, 0, 0, 0, 268, 266, 1, 0, 0, 0, 269, 43, 1, 0, 0, 0, 270, 271, 5, 66,
		0, 0, 271, 276, 5, 67, 0, 0, 272, 273, 5, 64, 0, 0, 273, 275, 5, 67, 0,
		0, 274, 272, 1, 0, 0, 0, 275, 278, 1, 0, 0, 0, 276, 274, 1, 0, 0, 0, 276,
		277, 1, 0, 0, 0, 277, 280, 1, 0, 0, 0, 278, 276, 1, 0, 0, 0, 279, 270,
		1, 0, 0, 0, 279, 280, 1, 0, 0, 0, 280, 45, 1, 0, 0, 0, 281, 282, 3, 52,
		26, 0, 282, 47, 1, 0, 0, 0, 283, 284, 3, 52, 26, 0, 284, 49, 1, 0, 0, 0,
		285, 288, 3, 62, 31, 0, 286, 288, 3, 64, 32, 0, 287, 285, 1, 0, 0, 0, 287,
		286, 1, 0, 0, 0, 288, 51, 1, 0, 0, 0, 289, 290, 6, 26, -1, 0, 290, 302,
		5, 36, 0, 0, 291, 302, 7, 1, 0, 0, 292, 302, 5, 67, 0, 0, 293, 302, 3,
		50, 25, 0, 294, 302, 3, 58, 29, 0, 295, 296, 5, 54, 0, 0, 296, 297, 3,
		52, 26, 0, 297, 298, 5, 55, 0, 0, 298, 302, 1, 0, 0, 0, 299, 300, 7, 2,
		0, 0, 300, 302, 3, 52, 26, 7, 301, 289, 1, 0, 0, 0, 301, 291, 1, 0, 0,
		0, 301, 292, 1, 0, 0, 0, 301, 293, 1, 0, 0, 0, 301, 294, 1, 0, 0, 0, 301,
		295, 1, 0, 0, 0, 301, 299, 1, 0, 0, 0, 302, 357, 1, 0, 0, 0, 303, 304,
		10, 6, 0, 0, 304, 305, 7, 3, 0, 0, 305, 356, 3, 52, 26, 7, 306, 307, 10,
		5, 0, 0, 307, 308, 7, 4, 0, 0, 308, 356, 3, 52, 26, 6, 309, 310, 10, 4,
		0, 0, 310, 311, 3, 56, 28, 0, 311, 312, 3, 52, 26, 5, 312, 356, 1, 0, 0,
		0, 313, 314, 10, 3, 0, 0, 314, 315, 7, 5, 0, 0, 315, 356, 3, 52, 26, 4,
		316, 317, 10, 2, 0, 0, 317, 318, 5, 61, 0, 0, 318, 356, 3, 52, 26, 3, 319,
		320, 10, 1, 0, 0, 320, 321, 5, 62, 0, 0, 321, 356, 3, 52, 26, 2, 322, 325,
		10, 14, 0, 0, 323, 324, 5, 63, 0, 0, 324, 326, 5, 67, 0, 0, 325, 323, 1,
		0, 0, 0, 326, 327, 1, 0, 0, 0, 327, 325, 1, 0, 0, 0, 327, 328, 1, 0, 0,
		0, 328, 356, 1, 0, 0, 0, 329, 330, 10, 13, 0, 0, 330, 356, 5, 46, 0, 0,
		331, 332, 10, 12, 0, 0, 332, 333, 5, 47, 0, 0, 333, 338, 5, 67, 0, 0, 334,
		335, 5, 54, 0, 0, 335, 336, 3, 54, 27, 0, 336, 337, 5, 55, 0, 0, 337, 339,
		1, 0, 0, 0, 338, 334, 1, 0, 0, 0, 338, 339, 1, 0, 0, 0, 339, 356, 1, 0,
		0, 0, 340, 341, 10, 11, 0, 0, 341, 343, 5, 48, 0, 0, 342, 344, 3, 52, 26,
		0, 343, 342, 1, 0, 0, 0, 343, 344, 1, 0, 0, 0, 344, 356, 1, 0, 0, 0, 345,
		346, 10, 10, 0, 0, 346, 347, 5, 54, 0, 0, 347, 348, 3, 54, 27, 0, 348,
		349, 5, 55, 0, 0, 349, 356, 1, 0, 0, 0, 350, 351, 10, 9, 0, 0, 351, 352,
		5, 56, 0, 0, 352, 353, 3, 52, 26, 0, 353, 354, 5, 57, 0, 0, 354, 356, 1,
		0, 0, 0, 355, 303, 1, 0, 0, 0, 355, 306, 1, 0, 0, 0, 355, 309, 1, 0, 0,
		0, 355, 313, 1, 0, 0, 0, 355, 316, 1, 0, 0, 0, 355, 319, 1, 0, 0, 0, 355,
		322, 1, 0, 0, 0, 355, 329, 1, 0, 0, 0, 355, 331, 1, 0, 0, 0, 355, 340,
		1, 0, 0, 0, 355, 345, 1, 0, 0, 0, 355, 350, 1, 0, 0, 0, 356, 359, 1, 0,
		0, 0, 357, 355, 1, 0, 0, 0, 357, 358, 1, 0, 0, 0, 358, 53, 1, 0, 0, 0,
		359, 357, 1, 0, 0, 0, 360, 370, 1, 0, 0, 0, 361, 370, 3, 52, 26, 0, 362,
		365, 3, 52, 26, 0, 363, 364, 5, 64, 0, 0, 364, 366, 3, 52, 26, 0, 365,
		363, 1, 0, 0, 0, 366, 367, 1, 0, 0, 0, 367, 365, 1, 0, 0, 0, 367, 368,
		1, 0, 0, 0, 368, 370, 1, 0, 0, 0, 369, 360, 1, 0, 0, 0, 369, 361, 1, 0,
		0, 0, 369, 362, 1, 0, 0, 0, 370, 55, 1, 0, 0, 0, 371, 372, 7, 6, 0, 0,
		372, 57, 1, 0, 0, 0, 373, 382, 5, 42, 0, 0, 374, 379, 3, 60, 30, 0, 375,
		376, 5, 64, 0, 0, 376, 378, 3, 60, 30, 0, 377, 375, 1, 0, 0, 0, 378, 381,
		1, 0, 0, 0, 379, 377, 1, 0, 0, 0, 379, 380, 1, 0, 0, 0, 380, 383, 1, 0,
		0, 0, 381, 379, 1, 0, 0, 0, 382, 374, 1, 0, 0, 0, 382, 383, 1, 0, 0, 0,
		383, 384, 1, 0, 0, 0, 384, 385, 5, 37, 0, 0, 385, 59, 1, 0, 0, 0, 386,
		389, 3, 50, 25, 0, 387, 389, 5, 67, 0, 0, 388, 386, 1, 0, 0, 0, 388, 387,
		1, 0, 0, 0, 389, 390, 1, 0, 0, 0, 390, 391, 5, 65, 0, 0, 391, 392, 3, 52,
		26, 0, 392, 61, 1, 0, 0, 0, 393, 402, 5, 44, 0, 0, 394, 401, 5, 15, 0,
		0, 395, 401, 5, 13, 0, 0, 396, 397, 5, 14, 0, 0, 397, 398, 3, 52, 26, 0,
		398, 399, 5, 37, 0, 0, 399, 401, 1, 0, 0, 0, 400, 394, 1, 0, 0, 0, 400,
		395, 1, 0, 0, 0, 400, 396, 1, 0, 0, 0, 401, 404, 1, 0, 0, 0, 402, 400,
		1, 0, 0, 0, 402, 403, 1, 0, 0, 0, 403, 405, 1, 0, 0, 0, 404, 402, 1, 0,
		0, 0, 405, 406, 5, 12, 0, 0, 406, 63, 1, 0, 0, 0, 407, 416, 5, 43, 0, 0,
		408, 415, 5, 11, 0, 0, 409, 415, 5, 9, 0, 0, 410, 411, 5, 10, 0, 0, 411,
		412, 3, 52, 26, 0, 412, 413, 5, 37, 0, 0, 413, 415, 1, 0, 0, 0, 414, 408,
		1, 0, 0, 0, 414, 409, 1, 0, 0, 0, 414, 410, 1, 0, 0, 0, 415, 418, 1, 0,
		0, 0, 416, 414, 1, 0, 0, 0, 416, 417, 1, 0, 0, 0, 417, 419, 1, 0, 0, 0,
		418, 416, 1, 0, 0, 0, 419, 420, 5, 8, 0, 0, 420, 65, 1, 0, 0, 0, 37, 72,
		81, 86, 97, 112, 119, 149, 159, 167, 195, 211, 214, 238, 245, 253, 258,
		263, 266, 268, 276, 279, 287, 301, 327, 338, 343, 355, 357, 367, 369, 379,
		382, 388, 400, 402, 414, 416,
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

// FreemarkerParserInit initializes any static state used to implement FreemarkerParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewFreemarkerParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func FreemarkerParserInit() {
	staticData := &freemarkerparserParserStaticData
	staticData.once.Do(freemarkerparserParserInit)
}

// NewFreemarkerParser produces a new parser instance for the optional input antlr.TokenStream.
func NewFreemarkerParser(input antlr.TokenStream) *FreemarkerParser {
	FreemarkerParserInit()
	this := new(FreemarkerParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &freemarkerparserParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.predictionContextCache)
	this.RuleNames = staticData.ruleNames
	this.LiteralNames = staticData.literalNames
	this.SymbolicNames = staticData.symbolicNames
	this.GrammarFileName = "java-escape"

	return this
}

// FreemarkerParser tokens.
const (
	FreemarkerParserEOF                   = antlr.TokenEOF
	FreemarkerParserCOMMENT               = 1
	FreemarkerParserSTART_DIRECTIVE_TAG   = 2
	FreemarkerParserEND_DIRECTIVE_TAG     = 3
	FreemarkerParserSTART_USER_DIR_TAG    = 4
	FreemarkerParserEND_USER_DIR_TAG      = 5
	FreemarkerParserINLINE_EXPR_START     = 6
	FreemarkerParserCONTENT               = 7
	FreemarkerParserDQS_EXIT              = 8
	FreemarkerParserDQS_ESCAPE            = 9
	FreemarkerParserDQS_ENTER_EXPR        = 10
	FreemarkerParserDQS_CONTENT           = 11
	FreemarkerParserSQS_EXIT              = 12
	FreemarkerParserSQS_ESCAPE            = 13
	FreemarkerParserSQS_ENTER_EXPR        = 14
	FreemarkerParserSQS_CONTENT           = 15
	FreemarkerParserEXPR_IF               = 16
	FreemarkerParserEXPR_ELSE             = 17
	FreemarkerParserEXPR_ELSEIF           = 18
	FreemarkerParserEXPR_ASSIGN           = 19
	FreemarkerParserEXPR_AS               = 20
	FreemarkerParserEXPR_LIST             = 21
	FreemarkerParserEXPR_TRUE             = 22
	FreemarkerParserEXPR_FALSE            = 23
	FreemarkerParserEXPR_INCLUDE          = 24
	FreemarkerParserEXPR_IMPORT           = 25
	FreemarkerParserEXPR_MACRO            = 26
	FreemarkerParserEXPR_NESTED           = 27
	FreemarkerParserEXPR_RETURN           = 28
	FreemarkerParserEXPR_LT_SYM           = 29
	FreemarkerParserEXPR_LT_STR           = 30
	FreemarkerParserEXPR_LTE_SYM          = 31
	FreemarkerParserEXPR_LTE_STR          = 32
	FreemarkerParserEXPR_GT_STR           = 33
	FreemarkerParserEXPR_GTE_SYM          = 34
	FreemarkerParserEXPR_GTE_STR          = 35
	FreemarkerParserEXPR_NUM              = 36
	FreemarkerParserEXPR_EXIT_R_BRACE     = 37
	FreemarkerParserEXPR_EXIT_GT          = 38
	FreemarkerParserEXPR_EXIT_DIV_GT      = 39
	FreemarkerParserEXPR_WS               = 40
	FreemarkerParserEXPR_COMENT           = 41
	FreemarkerParserEXPR_STRUCT           = 42
	FreemarkerParserEXPR_DOUBLE_STR_START = 43
	FreemarkerParserEXPR_SINGLE_STR_START = 44
	FreemarkerParserEXPR_AT               = 45
	FreemarkerParserEXPR_DBL_QUESTION     = 46
	FreemarkerParserEXPR_QUESTION         = 47
	FreemarkerParserEXPR_BANG             = 48
	FreemarkerParserEXPR_ADD              = 49
	FreemarkerParserEXPR_SUB              = 50
	FreemarkerParserEXPR_MUL              = 51
	FreemarkerParserEXPR_DIV              = 52
	FreemarkerParserEXPR_MOD              = 53
	FreemarkerParserEXPR_L_PAREN          = 54
	FreemarkerParserEXPR_R_PAREN          = 55
	FreemarkerParserEXPR_L_SQ_PAREN       = 56
	FreemarkerParserEXPR_R_SQ_PAREN       = 57
	FreemarkerParserEXPR_COMPARE_EQ       = 58
	FreemarkerParserEXPR_EQ               = 59
	FreemarkerParserEXPR_COMPARE_NEQ      = 60
	FreemarkerParserEXPR_LOGICAL_AND      = 61
	FreemarkerParserEXPR_LOGICAL_OR       = 62
	FreemarkerParserEXPR_DOT              = 63
	FreemarkerParserEXPR_COMMA            = 64
	FreemarkerParserEXPR_COLON            = 65
	FreemarkerParserEXPR_SEMICOLON        = 66
	FreemarkerParserEXPR_SYMBOL           = 67
)

// FreemarkerParser rules.
const (
	FreemarkerParserRULE_template                  = 0
	FreemarkerParserRULE_elements                  = 1
	FreemarkerParserRULE_element                   = 2
	FreemarkerParserRULE_rawText                   = 3
	FreemarkerParserRULE_directive                 = 4
	FreemarkerParserRULE_directiveIf               = 5
	FreemarkerParserRULE_directiveIfTrueElements   = 6
	FreemarkerParserRULE_directiveIfElseIfElements = 7
	FreemarkerParserRULE_directiveIfElseElements   = 8
	FreemarkerParserRULE_tagExprElseIfs            = 9
	FreemarkerParserRULE_directiveAssign           = 10
	FreemarkerParserRULE_directiveList             = 11
	FreemarkerParserRULE_directiveListBodyElements = 12
	FreemarkerParserRULE_directiveListElseElements = 13
	FreemarkerParserRULE_directiveInclude          = 14
	FreemarkerParserRULE_directiveImport           = 15
	FreemarkerParserRULE_directiveMacro            = 16
	FreemarkerParserRULE_directiveNested           = 17
	FreemarkerParserRULE_directiveReturn           = 18
	FreemarkerParserRULE_directiveUser             = 19
	FreemarkerParserRULE_directiveUserId           = 20
	FreemarkerParserRULE_directiveUserParams       = 21
	FreemarkerParserRULE_directiveUserLoopParams   = 22
	FreemarkerParserRULE_tagExpr                   = 23
	FreemarkerParserRULE_inlineExpr                = 24
	FreemarkerParserRULE_string                    = 25
	FreemarkerParserRULE_expr                      = 26
	FreemarkerParserRULE_functionParams            = 27
	FreemarkerParserRULE_booleanRelationalOperator = 28
	FreemarkerParserRULE_struct                    = 29
	FreemarkerParserRULE_struct_pair               = 30
	FreemarkerParserRULE_single_quote_string       = 31
	FreemarkerParserRULE_double_quote_string       = 32
)

// ITemplateContext is an interface to support dynamic dispatch.
type ITemplateContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsTemplateContext differentiates from other interfaces.
	IsTemplateContext()
}

type TemplateContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyTemplateContext() *TemplateContext {
	var p = new(TemplateContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_template
	return p
}

func (*TemplateContext) IsTemplateContext() {}

func NewTemplateContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *TemplateContext {
	var p = new(TemplateContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_template

	return p
}

func (s *TemplateContext) GetParser() antlr.Parser { return s.parser }

func (s *TemplateContext) Elements() IElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElementsContext)
}

func (s *TemplateContext) EOF() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEOF, 0)
}

func (s *TemplateContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TemplateContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *TemplateContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitTemplate(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) Template() (localctx ITemplateContext) {
	this := p
	_ = this

	localctx = NewTemplateContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, FreemarkerParserRULE_template)

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
		p.SetState(66)
		p.Elements()
	}
	{
		p.SetState(67)
		p.Match(FreemarkerParserEOF)
	}

	return localctx
}

// IElementsContext is an interface to support dynamic dispatch.
type IElementsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsElementsContext differentiates from other interfaces.
	IsElementsContext()
}

type ElementsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyElementsContext() *ElementsContext {
	var p = new(ElementsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_elements
	return p
}

func (*ElementsContext) IsElementsContext() {}

func NewElementsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ElementsContext {
	var p = new(ElementsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_elements

	return p
}

func (s *ElementsContext) GetParser() antlr.Parser { return s.parser }

func (s *ElementsContext) AllElement() []IElementContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IElementContext); ok {
			len++
		}
	}

	tst := make([]IElementContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IElementContext); ok {
			tst[i] = t.(IElementContext)
			i++
		}
	}

	return tst
}

func (s *ElementsContext) Element(i int) IElementContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElementContext); ok {
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

	return t.(IElementContext)
}

func (s *ElementsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ElementsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ElementsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitElements(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) Elements() (localctx IElementsContext) {
	this := p
	_ = this

	localctx = NewElementsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, FreemarkerParserRULE_elements)

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
	p.SetState(72)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 0, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(69)
				p.Element()
			}

		}
		p.SetState(74)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 0, p.GetParserRuleContext())
	}

	return localctx
}

// IElementContext is an interface to support dynamic dispatch.
type IElementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsElementContext differentiates from other interfaces.
	IsElementContext()
}

type ElementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyElementContext() *ElementContext {
	var p = new(ElementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_element
	return p
}

func (*ElementContext) IsElementContext() {}

func NewElementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ElementContext {
	var p = new(ElementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_element

	return p
}

func (s *ElementContext) GetParser() antlr.Parser { return s.parser }

func (s *ElementContext) CopyFrom(ctx *ElementContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *ElementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ElementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type InlineExprElementContext struct {
	*ElementContext
}

func NewInlineExprElementContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *InlineExprElementContext {
	var p = new(InlineExprElementContext)

	p.ElementContext = NewEmptyElementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ElementContext))

	return p
}

func (s *InlineExprElementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *InlineExprElementContext) INLINE_EXPR_START() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserINLINE_EXPR_START, 0)
}

func (s *InlineExprElementContext) InlineExpr() IInlineExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IInlineExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IInlineExprContext)
}

func (s *InlineExprElementContext) EXPR_EXIT_R_BRACE() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_R_BRACE, 0)
}

func (s *InlineExprElementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitInlineExprElement(s)

	default:
		return t.VisitChildren(s)
	}
}

type RawTextElementContext struct {
	*ElementContext
}

func NewRawTextElementContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *RawTextElementContext {
	var p = new(RawTextElementContext)

	p.ElementContext = NewEmptyElementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ElementContext))

	return p
}

func (s *RawTextElementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RawTextElementContext) RawText() IRawTextContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRawTextContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRawTextContext)
}

func (s *RawTextElementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitRawTextElement(s)

	default:
		return t.VisitChildren(s)
	}
}

type DirectiveElementContext struct {
	*ElementContext
}

func NewDirectiveElementContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *DirectiveElementContext {
	var p = new(DirectiveElementContext)

	p.ElementContext = NewEmptyElementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ElementContext))

	return p
}

func (s *DirectiveElementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveElementContext) Directive() IDirectiveContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveContext)
}

func (s *DirectiveElementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveElement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) Element() (localctx IElementContext) {
	this := p
	_ = this

	localctx = NewElementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, FreemarkerParserRULE_element)

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

	p.SetState(81)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case FreemarkerParserCONTENT:
		localctx = NewRawTextElementContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(75)
			p.RawText()
		}

	case FreemarkerParserSTART_DIRECTIVE_TAG, FreemarkerParserSTART_USER_DIR_TAG:
		localctx = NewDirectiveElementContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(76)
			p.Directive()
		}

	case FreemarkerParserINLINE_EXPR_START:
		localctx = NewInlineExprElementContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(77)
			p.Match(FreemarkerParserINLINE_EXPR_START)
		}
		{
			p.SetState(78)
			p.InlineExpr()
		}
		{
			p.SetState(79)
			p.Match(FreemarkerParserEXPR_EXIT_R_BRACE)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IRawTextContext is an interface to support dynamic dispatch.
type IRawTextContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsRawTextContext differentiates from other interfaces.
	IsRawTextContext()
}

type RawTextContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyRawTextContext() *RawTextContext {
	var p = new(RawTextContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_rawText
	return p
}

func (*RawTextContext) IsRawTextContext() {}

func NewRawTextContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *RawTextContext {
	var p = new(RawTextContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_rawText

	return p
}

func (s *RawTextContext) GetParser() antlr.Parser { return s.parser }

func (s *RawTextContext) AllCONTENT() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserCONTENT)
}

func (s *RawTextContext) CONTENT(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserCONTENT, i)
}

func (s *RawTextContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RawTextContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *RawTextContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitRawText(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) RawText() (localctx IRawTextContext) {
	this := p
	_ = this

	localctx = NewRawTextContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, FreemarkerParserRULE_rawText)

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
	p.SetState(84)
	p.GetErrorHandler().Sync(p)
	_alt = 1
	for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		switch _alt {
		case 1:
			{
				p.SetState(83)
				p.Match(FreemarkerParserCONTENT)
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

		p.SetState(86)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 2, p.GetParserRuleContext())
	}

	return localctx
}

// IDirectiveContext is an interface to support dynamic dispatch.
type IDirectiveContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveContext differentiates from other interfaces.
	IsDirectiveContext()
}

type DirectiveContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveContext() *DirectiveContext {
	var p = new(DirectiveContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directive
	return p
}

func (*DirectiveContext) IsDirectiveContext() {}

func NewDirectiveContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveContext {
	var p = new(DirectiveContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directive

	return p
}

func (s *DirectiveContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveContext) DirectiveIf() IDirectiveIfContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveIfContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveIfContext)
}

func (s *DirectiveContext) DirectiveAssign() IDirectiveAssignContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveAssignContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveAssignContext)
}

func (s *DirectiveContext) DirectiveList() IDirectiveListContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveListContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveListContext)
}

func (s *DirectiveContext) DirectiveInclude() IDirectiveIncludeContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveIncludeContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveIncludeContext)
}

func (s *DirectiveContext) DirectiveImport() IDirectiveImportContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveImportContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveImportContext)
}

func (s *DirectiveContext) DirectiveMacro() IDirectiveMacroContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveMacroContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveMacroContext)
}

func (s *DirectiveContext) DirectiveNested() IDirectiveNestedContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveNestedContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveNestedContext)
}

func (s *DirectiveContext) DirectiveReturn() IDirectiveReturnContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveReturnContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveReturnContext)
}

func (s *DirectiveContext) DirectiveUser() IDirectiveUserContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveUserContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveUserContext)
}

func (s *DirectiveContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirective(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) Directive() (localctx IDirectiveContext) {
	this := p
	_ = this

	localctx = NewDirectiveContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, FreemarkerParserRULE_directive)

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

	p.SetState(97)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 3, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(88)
			p.DirectiveIf()
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(89)
			p.DirectiveAssign()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(90)
			p.DirectiveList()
		}

	case 4:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(91)
			p.DirectiveInclude()
		}

	case 5:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(92)
			p.DirectiveImport()
		}

	case 6:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(93)
			p.DirectiveMacro()
		}

	case 7:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(94)
			p.DirectiveNested()
		}

	case 8:
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(95)
			p.DirectiveReturn()
		}

	case 9:
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(96)
			p.DirectiveUser()
		}

	}

	return localctx
}

// IDirectiveIfContext is an interface to support dynamic dispatch.
type IDirectiveIfContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// GetElse_ returns the else_ token.
	GetElse_() antlr.Token

	// SetElse_ sets the else_ token.
	SetElse_(antlr.Token)

	// IsDirectiveIfContext differentiates from other interfaces.
	IsDirectiveIfContext()
}

type DirectiveIfContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
	else_  antlr.Token
}

func NewEmptyDirectiveIfContext() *DirectiveIfContext {
	var p = new(DirectiveIfContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveIf
	return p
}

func (*DirectiveIfContext) IsDirectiveIfContext() {}

func NewDirectiveIfContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveIfContext {
	var p = new(DirectiveIfContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveIf

	return p
}

func (s *DirectiveIfContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveIfContext) GetElse_() antlr.Token { return s.else_ }

func (s *DirectiveIfContext) SetElse_(v antlr.Token) { s.else_ = v }

func (s *DirectiveIfContext) AllSTART_DIRECTIVE_TAG() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserSTART_DIRECTIVE_TAG)
}

func (s *DirectiveIfContext) START_DIRECTIVE_TAG(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSTART_DIRECTIVE_TAG, i)
}

func (s *DirectiveIfContext) AllEXPR_IF() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_IF)
}

func (s *DirectiveIfContext) EXPR_IF(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_IF, i)
}

func (s *DirectiveIfContext) TagExpr() ITagExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITagExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ITagExprContext)
}

func (s *DirectiveIfContext) AllEXPR_EXIT_GT() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_EXIT_GT)
}

func (s *DirectiveIfContext) EXPR_EXIT_GT(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_GT, i)
}

func (s *DirectiveIfContext) DirectiveIfTrueElements() IDirectiveIfTrueElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveIfTrueElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveIfTrueElementsContext)
}

func (s *DirectiveIfContext) END_DIRECTIVE_TAG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEND_DIRECTIVE_TAG, 0)
}

func (s *DirectiveIfContext) AllEXPR_ELSEIF() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_ELSEIF)
}

func (s *DirectiveIfContext) EXPR_ELSEIF(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_ELSEIF, i)
}

func (s *DirectiveIfContext) AllTagExprElseIfs() []ITagExprElseIfsContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ITagExprElseIfsContext); ok {
			len++
		}
	}

	tst := make([]ITagExprElseIfsContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ITagExprElseIfsContext); ok {
			tst[i] = t.(ITagExprElseIfsContext)
			i++
		}
	}

	return tst
}

func (s *DirectiveIfContext) TagExprElseIfs(i int) ITagExprElseIfsContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITagExprElseIfsContext); ok {
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

	return t.(ITagExprElseIfsContext)
}

func (s *DirectiveIfContext) AllDirectiveIfElseIfElements() []IDirectiveIfElseIfElementsContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IDirectiveIfElseIfElementsContext); ok {
			len++
		}
	}

	tst := make([]IDirectiveIfElseIfElementsContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IDirectiveIfElseIfElementsContext); ok {
			tst[i] = t.(IDirectiveIfElseIfElementsContext)
			i++
		}
	}

	return tst
}

func (s *DirectiveIfContext) DirectiveIfElseIfElements(i int) IDirectiveIfElseIfElementsContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveIfElseIfElementsContext); ok {
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

	return t.(IDirectiveIfElseIfElementsContext)
}

func (s *DirectiveIfContext) DirectiveIfElseElements() IDirectiveIfElseElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveIfElseElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveIfElseElementsContext)
}

func (s *DirectiveIfContext) EXPR_ELSE() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_ELSE, 0)
}

func (s *DirectiveIfContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveIfContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveIfContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveIf(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveIf() (localctx IDirectiveIfContext) {
	this := p
	_ = this

	localctx = NewDirectiveIfContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, FreemarkerParserRULE_directiveIf)
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
		p.SetState(99)
		p.Match(FreemarkerParserSTART_DIRECTIVE_TAG)
	}
	{
		p.SetState(100)
		p.Match(FreemarkerParserEXPR_IF)
	}
	{
		p.SetState(101)
		p.TagExpr()
	}
	{
		p.SetState(102)
		p.Match(FreemarkerParserEXPR_EXIT_GT)
	}
	{
		p.SetState(103)
		p.DirectiveIfTrueElements()
	}
	p.SetState(112)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 4, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(104)
				p.Match(FreemarkerParserSTART_DIRECTIVE_TAG)
			}
			{
				p.SetState(105)
				p.Match(FreemarkerParserEXPR_ELSEIF)
			}
			{
				p.SetState(106)
				p.TagExprElseIfs()
			}
			{
				p.SetState(107)
				p.Match(FreemarkerParserEXPR_EXIT_GT)
			}
			{
				p.SetState(108)
				p.DirectiveIfElseIfElements()
			}

		}
		p.SetState(114)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 4, p.GetParserRuleContext())
	}
	p.SetState(119)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == FreemarkerParserSTART_DIRECTIVE_TAG {
		{
			p.SetState(115)
			p.Match(FreemarkerParserSTART_DIRECTIVE_TAG)
		}
		{
			p.SetState(116)

			var _m = p.Match(FreemarkerParserEXPR_ELSE)

			localctx.(*DirectiveIfContext).else_ = _m
		}
		{
			p.SetState(117)
			p.Match(FreemarkerParserEXPR_EXIT_GT)
		}
		{
			p.SetState(118)
			p.DirectiveIfElseElements()
		}

	}
	{
		p.SetState(121)
		p.Match(FreemarkerParserEND_DIRECTIVE_TAG)
	}
	{
		p.SetState(122)
		p.Match(FreemarkerParserEXPR_IF)
	}
	{
		p.SetState(123)
		p.Match(FreemarkerParserEXPR_EXIT_GT)
	}

	return localctx
}

// IDirectiveIfTrueElementsContext is an interface to support dynamic dispatch.
type IDirectiveIfTrueElementsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveIfTrueElementsContext differentiates from other interfaces.
	IsDirectiveIfTrueElementsContext()
}

type DirectiveIfTrueElementsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveIfTrueElementsContext() *DirectiveIfTrueElementsContext {
	var p = new(DirectiveIfTrueElementsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveIfTrueElements
	return p
}

func (*DirectiveIfTrueElementsContext) IsDirectiveIfTrueElementsContext() {}

func NewDirectiveIfTrueElementsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveIfTrueElementsContext {
	var p = new(DirectiveIfTrueElementsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveIfTrueElements

	return p
}

func (s *DirectiveIfTrueElementsContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveIfTrueElementsContext) Elements() IElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElementsContext)
}

func (s *DirectiveIfTrueElementsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveIfTrueElementsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveIfTrueElementsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveIfTrueElements(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveIfTrueElements() (localctx IDirectiveIfTrueElementsContext) {
	this := p
	_ = this

	localctx = NewDirectiveIfTrueElementsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, FreemarkerParserRULE_directiveIfTrueElements)

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
		p.SetState(125)
		p.Elements()
	}

	return localctx
}

// IDirectiveIfElseIfElementsContext is an interface to support dynamic dispatch.
type IDirectiveIfElseIfElementsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveIfElseIfElementsContext differentiates from other interfaces.
	IsDirectiveIfElseIfElementsContext()
}

type DirectiveIfElseIfElementsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveIfElseIfElementsContext() *DirectiveIfElseIfElementsContext {
	var p = new(DirectiveIfElseIfElementsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveIfElseIfElements
	return p
}

func (*DirectiveIfElseIfElementsContext) IsDirectiveIfElseIfElementsContext() {}

func NewDirectiveIfElseIfElementsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveIfElseIfElementsContext {
	var p = new(DirectiveIfElseIfElementsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveIfElseIfElements

	return p
}

func (s *DirectiveIfElseIfElementsContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveIfElseIfElementsContext) Elements() IElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElementsContext)
}

func (s *DirectiveIfElseIfElementsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveIfElseIfElementsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveIfElseIfElementsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveIfElseIfElements(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveIfElseIfElements() (localctx IDirectiveIfElseIfElementsContext) {
	this := p
	_ = this

	localctx = NewDirectiveIfElseIfElementsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 14, FreemarkerParserRULE_directiveIfElseIfElements)

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
		p.SetState(127)
		p.Elements()
	}

	return localctx
}

// IDirectiveIfElseElementsContext is an interface to support dynamic dispatch.
type IDirectiveIfElseElementsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveIfElseElementsContext differentiates from other interfaces.
	IsDirectiveIfElseElementsContext()
}

type DirectiveIfElseElementsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveIfElseElementsContext() *DirectiveIfElseElementsContext {
	var p = new(DirectiveIfElseElementsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveIfElseElements
	return p
}

func (*DirectiveIfElseElementsContext) IsDirectiveIfElseElementsContext() {}

func NewDirectiveIfElseElementsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveIfElseElementsContext {
	var p = new(DirectiveIfElseElementsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveIfElseElements

	return p
}

func (s *DirectiveIfElseElementsContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveIfElseElementsContext) Elements() IElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElementsContext)
}

func (s *DirectiveIfElseElementsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveIfElseElementsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveIfElseElementsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveIfElseElements(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveIfElseElements() (localctx IDirectiveIfElseElementsContext) {
	this := p
	_ = this

	localctx = NewDirectiveIfElseElementsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 16, FreemarkerParserRULE_directiveIfElseElements)

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
		p.SetState(129)
		p.Elements()
	}

	return localctx
}

// ITagExprElseIfsContext is an interface to support dynamic dispatch.
type ITagExprElseIfsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsTagExprElseIfsContext differentiates from other interfaces.
	IsTagExprElseIfsContext()
}

type TagExprElseIfsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyTagExprElseIfsContext() *TagExprElseIfsContext {
	var p = new(TagExprElseIfsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_tagExprElseIfs
	return p
}

func (*TagExprElseIfsContext) IsTagExprElseIfsContext() {}

func NewTagExprElseIfsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *TagExprElseIfsContext {
	var p = new(TagExprElseIfsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_tagExprElseIfs

	return p
}

func (s *TagExprElseIfsContext) GetParser() antlr.Parser { return s.parser }

func (s *TagExprElseIfsContext) TagExpr() ITagExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITagExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ITagExprContext)
}

func (s *TagExprElseIfsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TagExprElseIfsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *TagExprElseIfsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitTagExprElseIfs(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) TagExprElseIfs() (localctx ITagExprElseIfsContext) {
	this := p
	_ = this

	localctx = NewTagExprElseIfsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 18, FreemarkerParserRULE_tagExprElseIfs)

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
		p.SetState(131)
		p.TagExpr()
	}

	return localctx
}

// IDirectiveAssignContext is an interface to support dynamic dispatch.
type IDirectiveAssignContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveAssignContext differentiates from other interfaces.
	IsDirectiveAssignContext()
}

type DirectiveAssignContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveAssignContext() *DirectiveAssignContext {
	var p = new(DirectiveAssignContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveAssign
	return p
}

func (*DirectiveAssignContext) IsDirectiveAssignContext() {}

func NewDirectiveAssignContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveAssignContext {
	var p = new(DirectiveAssignContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveAssign

	return p
}

func (s *DirectiveAssignContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveAssignContext) START_DIRECTIVE_TAG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSTART_DIRECTIVE_TAG, 0)
}

func (s *DirectiveAssignContext) AllEXPR_ASSIGN() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_ASSIGN)
}

func (s *DirectiveAssignContext) EXPR_ASSIGN(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_ASSIGN, i)
}

func (s *DirectiveAssignContext) EXPR_SYMBOL() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SYMBOL, 0)
}

func (s *DirectiveAssignContext) EXPR_EQ() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EQ, 0)
}

func (s *DirectiveAssignContext) TagExpr() ITagExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITagExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ITagExprContext)
}

func (s *DirectiveAssignContext) AllEXPR_EXIT_GT() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_EXIT_GT)
}

func (s *DirectiveAssignContext) EXPR_EXIT_GT(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_GT, i)
}

func (s *DirectiveAssignContext) EXPR_EXIT_DIV_GT() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_DIV_GT, 0)
}

func (s *DirectiveAssignContext) Elements() IElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElementsContext)
}

func (s *DirectiveAssignContext) END_DIRECTIVE_TAG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEND_DIRECTIVE_TAG, 0)
}

func (s *DirectiveAssignContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveAssignContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveAssignContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveAssign(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveAssign() (localctx IDirectiveAssignContext) {
	this := p
	_ = this

	localctx = NewDirectiveAssignContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 20, FreemarkerParserRULE_directiveAssign)
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

	p.SetState(149)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 6, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(133)
			p.Match(FreemarkerParserSTART_DIRECTIVE_TAG)
		}
		{
			p.SetState(134)
			p.Match(FreemarkerParserEXPR_ASSIGN)
		}
		{
			p.SetState(135)
			p.Match(FreemarkerParserEXPR_SYMBOL)
		}
		{
			p.SetState(136)
			p.Match(FreemarkerParserEXPR_EQ)
		}
		{
			p.SetState(137)
			p.TagExpr()
		}
		{
			p.SetState(138)
			_la = p.GetTokenStream().LA(1)

			if !(_la == FreemarkerParserEXPR_EXIT_GT || _la == FreemarkerParserEXPR_EXIT_DIV_GT) {
				p.GetErrorHandler().RecoverInline(p)
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(140)
			p.Match(FreemarkerParserSTART_DIRECTIVE_TAG)
		}
		{
			p.SetState(141)
			p.Match(FreemarkerParserEXPR_ASSIGN)
		}
		{
			p.SetState(142)
			p.Match(FreemarkerParserEXPR_SYMBOL)
		}
		{
			p.SetState(143)
			p.Match(FreemarkerParserEXPR_EXIT_GT)
		}
		{
			p.SetState(144)
			p.Elements()
		}
		{
			p.SetState(145)
			p.Match(FreemarkerParserEND_DIRECTIVE_TAG)
		}
		{
			p.SetState(146)
			p.Match(FreemarkerParserEXPR_ASSIGN)
		}
		{
			p.SetState(147)
			p.Match(FreemarkerParserEXPR_EXIT_GT)
		}

	}

	return localctx
}

// IDirectiveListContext is an interface to support dynamic dispatch.
type IDirectiveListContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// GetValue returns the value token.
	GetValue() antlr.Token

	// GetKey returns the key token.
	GetKey() antlr.Token

	// SetValue sets the value token.
	SetValue(antlr.Token)

	// SetKey sets the key token.
	SetKey(antlr.Token)

	// IsDirectiveListContext differentiates from other interfaces.
	IsDirectiveListContext()
}

type DirectiveListContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
	value  antlr.Token
	key    antlr.Token
}

func NewEmptyDirectiveListContext() *DirectiveListContext {
	var p = new(DirectiveListContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveList
	return p
}

func (*DirectiveListContext) IsDirectiveListContext() {}

func NewDirectiveListContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveListContext {
	var p = new(DirectiveListContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveList

	return p
}

func (s *DirectiveListContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveListContext) GetValue() antlr.Token { return s.value }

func (s *DirectiveListContext) GetKey() antlr.Token { return s.key }

func (s *DirectiveListContext) SetValue(v antlr.Token) { s.value = v }

func (s *DirectiveListContext) SetKey(v antlr.Token) { s.key = v }

func (s *DirectiveListContext) AllSTART_DIRECTIVE_TAG() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserSTART_DIRECTIVE_TAG)
}

func (s *DirectiveListContext) START_DIRECTIVE_TAG(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSTART_DIRECTIVE_TAG, i)
}

func (s *DirectiveListContext) AllEXPR_LIST() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_LIST)
}

func (s *DirectiveListContext) EXPR_LIST(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_LIST, i)
}

func (s *DirectiveListContext) TagExpr() ITagExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITagExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ITagExprContext)
}

func (s *DirectiveListContext) EXPR_AS() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_AS, 0)
}

func (s *DirectiveListContext) AllEXPR_EXIT_GT() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_EXIT_GT)
}

func (s *DirectiveListContext) EXPR_EXIT_GT(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_GT, i)
}

func (s *DirectiveListContext) DirectiveListBodyElements() IDirectiveListBodyElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveListBodyElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveListBodyElementsContext)
}

func (s *DirectiveListContext) END_DIRECTIVE_TAG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEND_DIRECTIVE_TAG, 0)
}

func (s *DirectiveListContext) EXPR_COMMA() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_COMMA, 0)
}

func (s *DirectiveListContext) AllEXPR_SYMBOL() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_SYMBOL)
}

func (s *DirectiveListContext) EXPR_SYMBOL(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SYMBOL, i)
}

func (s *DirectiveListContext) EXPR_ELSE() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_ELSE, 0)
}

func (s *DirectiveListContext) DirectiveListElseElements() IDirectiveListElseElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveListElseElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveListElseElementsContext)
}

func (s *DirectiveListContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveListContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveListContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveList(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveList() (localctx IDirectiveListContext) {
	this := p
	_ = this

	localctx = NewDirectiveListContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 22, FreemarkerParserRULE_directiveList)
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
		p.SetState(151)
		p.Match(FreemarkerParserSTART_DIRECTIVE_TAG)
	}
	{
		p.SetState(152)
		p.Match(FreemarkerParserEXPR_LIST)
	}
	{
		p.SetState(153)
		p.TagExpr()
	}
	{
		p.SetState(154)
		p.Match(FreemarkerParserEXPR_AS)
	}
	p.SetState(159)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 7, p.GetParserRuleContext()) {
	case 1:
		{
			p.SetState(155)

			var _m = p.Match(FreemarkerParserEXPR_SYMBOL)

			localctx.(*DirectiveListContext).value = _m
		}

	case 2:
		{
			p.SetState(156)

			var _m = p.Match(FreemarkerParserEXPR_SYMBOL)

			localctx.(*DirectiveListContext).key = _m
		}
		{
			p.SetState(157)
			p.Match(FreemarkerParserEXPR_COMMA)
		}
		{
			p.SetState(158)

			var _m = p.Match(FreemarkerParserEXPR_SYMBOL)

			localctx.(*DirectiveListContext).value = _m
		}

	}
	{
		p.SetState(161)
		p.Match(FreemarkerParserEXPR_EXIT_GT)
	}
	{
		p.SetState(162)
		p.DirectiveListBodyElements()
	}
	p.SetState(167)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == FreemarkerParserSTART_DIRECTIVE_TAG {
		{
			p.SetState(163)
			p.Match(FreemarkerParserSTART_DIRECTIVE_TAG)
		}
		{
			p.SetState(164)
			p.Match(FreemarkerParserEXPR_ELSE)
		}
		{
			p.SetState(165)
			p.Match(FreemarkerParserEXPR_EXIT_GT)
		}
		{
			p.SetState(166)
			p.DirectiveListElseElements()
		}

	}
	{
		p.SetState(169)
		p.Match(FreemarkerParserEND_DIRECTIVE_TAG)
	}
	{
		p.SetState(170)
		p.Match(FreemarkerParserEXPR_LIST)
	}
	{
		p.SetState(171)
		p.Match(FreemarkerParserEXPR_EXIT_GT)
	}

	return localctx
}

// IDirectiveListBodyElementsContext is an interface to support dynamic dispatch.
type IDirectiveListBodyElementsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveListBodyElementsContext differentiates from other interfaces.
	IsDirectiveListBodyElementsContext()
}

type DirectiveListBodyElementsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveListBodyElementsContext() *DirectiveListBodyElementsContext {
	var p = new(DirectiveListBodyElementsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveListBodyElements
	return p
}

func (*DirectiveListBodyElementsContext) IsDirectiveListBodyElementsContext() {}

func NewDirectiveListBodyElementsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveListBodyElementsContext {
	var p = new(DirectiveListBodyElementsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveListBodyElements

	return p
}

func (s *DirectiveListBodyElementsContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveListBodyElementsContext) Elements() IElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElementsContext)
}

func (s *DirectiveListBodyElementsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveListBodyElementsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveListBodyElementsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveListBodyElements(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveListBodyElements() (localctx IDirectiveListBodyElementsContext) {
	this := p
	_ = this

	localctx = NewDirectiveListBodyElementsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, FreemarkerParserRULE_directiveListBodyElements)

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
		p.Elements()
	}

	return localctx
}

// IDirectiveListElseElementsContext is an interface to support dynamic dispatch.
type IDirectiveListElseElementsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveListElseElementsContext differentiates from other interfaces.
	IsDirectiveListElseElementsContext()
}

type DirectiveListElseElementsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveListElseElementsContext() *DirectiveListElseElementsContext {
	var p = new(DirectiveListElseElementsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveListElseElements
	return p
}

func (*DirectiveListElseElementsContext) IsDirectiveListElseElementsContext() {}

func NewDirectiveListElseElementsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveListElseElementsContext {
	var p = new(DirectiveListElseElementsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveListElseElements

	return p
}

func (s *DirectiveListElseElementsContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveListElseElementsContext) Elements() IElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElementsContext)
}

func (s *DirectiveListElseElementsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveListElseElementsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveListElseElementsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveListElseElements(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveListElseElements() (localctx IDirectiveListElseElementsContext) {
	this := p
	_ = this

	localctx = NewDirectiveListElseElementsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, FreemarkerParserRULE_directiveListElseElements)

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
		p.SetState(175)
		p.Elements()
	}

	return localctx
}

// IDirectiveIncludeContext is an interface to support dynamic dispatch.
type IDirectiveIncludeContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveIncludeContext differentiates from other interfaces.
	IsDirectiveIncludeContext()
}

type DirectiveIncludeContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveIncludeContext() *DirectiveIncludeContext {
	var p = new(DirectiveIncludeContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveInclude
	return p
}

func (*DirectiveIncludeContext) IsDirectiveIncludeContext() {}

func NewDirectiveIncludeContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveIncludeContext {
	var p = new(DirectiveIncludeContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveInclude

	return p
}

func (s *DirectiveIncludeContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveIncludeContext) START_DIRECTIVE_TAG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSTART_DIRECTIVE_TAG, 0)
}

func (s *DirectiveIncludeContext) EXPR_INCLUDE() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_INCLUDE, 0)
}

func (s *DirectiveIncludeContext) String_() IStringContext {
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

func (s *DirectiveIncludeContext) EXPR_EXIT_GT() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_GT, 0)
}

func (s *DirectiveIncludeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveIncludeContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveIncludeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveInclude(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveInclude() (localctx IDirectiveIncludeContext) {
	this := p
	_ = this

	localctx = NewDirectiveIncludeContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 28, FreemarkerParserRULE_directiveInclude)

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
		p.Match(FreemarkerParserSTART_DIRECTIVE_TAG)
	}
	{
		p.SetState(178)
		p.Match(FreemarkerParserEXPR_INCLUDE)
	}
	{
		p.SetState(179)
		p.String_()
	}
	{
		p.SetState(180)
		p.Match(FreemarkerParserEXPR_EXIT_GT)
	}

	return localctx
}

// IDirectiveImportContext is an interface to support dynamic dispatch.
type IDirectiveImportContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveImportContext differentiates from other interfaces.
	IsDirectiveImportContext()
}

type DirectiveImportContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveImportContext() *DirectiveImportContext {
	var p = new(DirectiveImportContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveImport
	return p
}

func (*DirectiveImportContext) IsDirectiveImportContext() {}

func NewDirectiveImportContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveImportContext {
	var p = new(DirectiveImportContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveImport

	return p
}

func (s *DirectiveImportContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveImportContext) START_DIRECTIVE_TAG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSTART_DIRECTIVE_TAG, 0)
}

func (s *DirectiveImportContext) EXPR_IMPORT() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_IMPORT, 0)
}

func (s *DirectiveImportContext) String_() IStringContext {
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

func (s *DirectiveImportContext) EXPR_AS() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_AS, 0)
}

func (s *DirectiveImportContext) EXPR_SYMBOL() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SYMBOL, 0)
}

func (s *DirectiveImportContext) EXPR_EXIT_GT() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_GT, 0)
}

func (s *DirectiveImportContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveImportContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveImportContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveImport(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveImport() (localctx IDirectiveImportContext) {
	this := p
	_ = this

	localctx = NewDirectiveImportContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 30, FreemarkerParserRULE_directiveImport)

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
		p.SetState(182)
		p.Match(FreemarkerParserSTART_DIRECTIVE_TAG)
	}
	{
		p.SetState(183)
		p.Match(FreemarkerParserEXPR_IMPORT)
	}
	{
		p.SetState(184)
		p.String_()
	}
	{
		p.SetState(185)
		p.Match(FreemarkerParserEXPR_AS)
	}
	{
		p.SetState(186)
		p.Match(FreemarkerParserEXPR_SYMBOL)
	}
	{
		p.SetState(187)
		p.Match(FreemarkerParserEXPR_EXIT_GT)
	}

	return localctx
}

// IDirectiveMacroContext is an interface to support dynamic dispatch.
type IDirectiveMacroContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveMacroContext differentiates from other interfaces.
	IsDirectiveMacroContext()
}

type DirectiveMacroContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveMacroContext() *DirectiveMacroContext {
	var p = new(DirectiveMacroContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveMacro
	return p
}

func (*DirectiveMacroContext) IsDirectiveMacroContext() {}

func NewDirectiveMacroContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveMacroContext {
	var p = new(DirectiveMacroContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveMacro

	return p
}

func (s *DirectiveMacroContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveMacroContext) START_DIRECTIVE_TAG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSTART_DIRECTIVE_TAG, 0)
}

func (s *DirectiveMacroContext) AllEXPR_MACRO() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_MACRO)
}

func (s *DirectiveMacroContext) EXPR_MACRO(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_MACRO, i)
}

func (s *DirectiveMacroContext) AllEXPR_SYMBOL() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_SYMBOL)
}

func (s *DirectiveMacroContext) EXPR_SYMBOL(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SYMBOL, i)
}

func (s *DirectiveMacroContext) AllEXPR_EXIT_GT() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_EXIT_GT)
}

func (s *DirectiveMacroContext) EXPR_EXIT_GT(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_GT, i)
}

func (s *DirectiveMacroContext) Elements() IElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElementsContext)
}

func (s *DirectiveMacroContext) END_DIRECTIVE_TAG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEND_DIRECTIVE_TAG, 0)
}

func (s *DirectiveMacroContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveMacroContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveMacroContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveMacro(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveMacro() (localctx IDirectiveMacroContext) {
	this := p
	_ = this

	localctx = NewDirectiveMacroContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 32, FreemarkerParserRULE_directiveMacro)
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
		p.SetState(189)
		p.Match(FreemarkerParserSTART_DIRECTIVE_TAG)
	}
	{
		p.SetState(190)
		p.Match(FreemarkerParserEXPR_MACRO)
	}
	{
		p.SetState(191)
		p.Match(FreemarkerParserEXPR_SYMBOL)
	}
	p.SetState(195)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == FreemarkerParserEXPR_SYMBOL {
		{
			p.SetState(192)
			p.Match(FreemarkerParserEXPR_SYMBOL)
		}

		p.SetState(197)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(198)
		p.Match(FreemarkerParserEXPR_EXIT_GT)
	}
	{
		p.SetState(199)
		p.Elements()
	}
	{
		p.SetState(200)
		p.Match(FreemarkerParserEND_DIRECTIVE_TAG)
	}
	{
		p.SetState(201)
		p.Match(FreemarkerParserEXPR_MACRO)
	}
	{
		p.SetState(202)
		p.Match(FreemarkerParserEXPR_EXIT_GT)
	}

	return localctx
}

// IDirectiveNestedContext is an interface to support dynamic dispatch.
type IDirectiveNestedContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveNestedContext differentiates from other interfaces.
	IsDirectiveNestedContext()
}

type DirectiveNestedContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveNestedContext() *DirectiveNestedContext {
	var p = new(DirectiveNestedContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveNested
	return p
}

func (*DirectiveNestedContext) IsDirectiveNestedContext() {}

func NewDirectiveNestedContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveNestedContext {
	var p = new(DirectiveNestedContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveNested

	return p
}

func (s *DirectiveNestedContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveNestedContext) START_DIRECTIVE_TAG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSTART_DIRECTIVE_TAG, 0)
}

func (s *DirectiveNestedContext) EXPR_NESTED() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_NESTED, 0)
}

func (s *DirectiveNestedContext) EXPR_EXIT_GT() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_GT, 0)
}

func (s *DirectiveNestedContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *DirectiveNestedContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *DirectiveNestedContext) AllEXPR_COMMA() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_COMMA)
}

func (s *DirectiveNestedContext) EXPR_COMMA(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_COMMA, i)
}

func (s *DirectiveNestedContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveNestedContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveNestedContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveNested(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveNested() (localctx IDirectiveNestedContext) {
	this := p
	_ = this

	localctx = NewDirectiveNestedContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 34, FreemarkerParserRULE_directiveNested)
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
		p.SetState(204)
		p.Match(FreemarkerParserSTART_DIRECTIVE_TAG)
	}
	{
		p.SetState(205)
		p.Match(FreemarkerParserEXPR_NESTED)
	}
	p.SetState(214)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if (int64((_la-22)) & ^0x3f) == 0 && ((int64(1)<<(_la-22))&35189009956867) != 0 {
		{
			p.SetState(206)
			p.expr(0)
		}
		p.SetState(211)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == FreemarkerParserEXPR_COMMA {
			{
				p.SetState(207)
				p.Match(FreemarkerParserEXPR_COMMA)
			}
			{
				p.SetState(208)
				p.expr(0)
			}

			p.SetState(213)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}

	}
	{
		p.SetState(216)
		p.Match(FreemarkerParserEXPR_EXIT_GT)
	}

	return localctx
}

// IDirectiveReturnContext is an interface to support dynamic dispatch.
type IDirectiveReturnContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveReturnContext differentiates from other interfaces.
	IsDirectiveReturnContext()
}

type DirectiveReturnContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveReturnContext() *DirectiveReturnContext {
	var p = new(DirectiveReturnContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveReturn
	return p
}

func (*DirectiveReturnContext) IsDirectiveReturnContext() {}

func NewDirectiveReturnContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveReturnContext {
	var p = new(DirectiveReturnContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveReturn

	return p
}

func (s *DirectiveReturnContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveReturnContext) START_DIRECTIVE_TAG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSTART_DIRECTIVE_TAG, 0)
}

func (s *DirectiveReturnContext) EXPR_RETURN() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_RETURN, 0)
}

func (s *DirectiveReturnContext) EXPR_EXIT_GT() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_GT, 0)
}

func (s *DirectiveReturnContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveReturnContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveReturnContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveReturn(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveReturn() (localctx IDirectiveReturnContext) {
	this := p
	_ = this

	localctx = NewDirectiveReturnContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 36, FreemarkerParserRULE_directiveReturn)

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
		p.SetState(218)
		p.Match(FreemarkerParserSTART_DIRECTIVE_TAG)
	}
	{
		p.SetState(219)
		p.Match(FreemarkerParserEXPR_RETURN)
	}
	{
		p.SetState(220)
		p.Match(FreemarkerParserEXPR_EXIT_GT)
	}

	return localctx
}

// IDirectiveUserContext is an interface to support dynamic dispatch.
type IDirectiveUserContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveUserContext differentiates from other interfaces.
	IsDirectiveUserContext()
}

type DirectiveUserContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveUserContext() *DirectiveUserContext {
	var p = new(DirectiveUserContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveUser
	return p
}

func (*DirectiveUserContext) IsDirectiveUserContext() {}

func NewDirectiveUserContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveUserContext {
	var p = new(DirectiveUserContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveUser

	return p
}

func (s *DirectiveUserContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveUserContext) START_USER_DIR_TAG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSTART_USER_DIR_TAG, 0)
}

func (s *DirectiveUserContext) AllDirectiveUserId() []IDirectiveUserIdContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IDirectiveUserIdContext); ok {
			len++
		}
	}

	tst := make([]IDirectiveUserIdContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IDirectiveUserIdContext); ok {
			tst[i] = t.(IDirectiveUserIdContext)
			i++
		}
	}

	return tst
}

func (s *DirectiveUserContext) DirectiveUserId(i int) IDirectiveUserIdContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveUserIdContext); ok {
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

	return t.(IDirectiveUserIdContext)
}

func (s *DirectiveUserContext) DirectiveUserParams() IDirectiveUserParamsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveUserParamsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveUserParamsContext)
}

func (s *DirectiveUserContext) DirectiveUserLoopParams() IDirectiveUserLoopParamsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDirectiveUserLoopParamsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDirectiveUserLoopParamsContext)
}

func (s *DirectiveUserContext) EXPR_EXIT_DIV_GT() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_DIV_GT, 0)
}

func (s *DirectiveUserContext) AllEXPR_EXIT_GT() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_EXIT_GT)
}

func (s *DirectiveUserContext) EXPR_EXIT_GT(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_GT, i)
}

func (s *DirectiveUserContext) Elements() IElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElementsContext)
}

func (s *DirectiveUserContext) END_USER_DIR_TAG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEND_USER_DIR_TAG, 0)
}

func (s *DirectiveUserContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveUserContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveUserContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveUser(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveUser() (localctx IDirectiveUserContext) {
	this := p
	_ = this

	localctx = NewDirectiveUserContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 38, FreemarkerParserRULE_directiveUser)

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

	p.SetState(238)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 12, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(222)
			p.Match(FreemarkerParserSTART_USER_DIR_TAG)
		}
		{
			p.SetState(223)
			p.DirectiveUserId()
		}
		{
			p.SetState(224)
			p.DirectiveUserParams()
		}
		{
			p.SetState(225)
			p.DirectiveUserLoopParams()
		}
		{
			p.SetState(226)
			p.Match(FreemarkerParserEXPR_EXIT_DIV_GT)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(228)
			p.Match(FreemarkerParserSTART_USER_DIR_TAG)
		}
		{
			p.SetState(229)
			p.DirectiveUserId()
		}
		{
			p.SetState(230)
			p.DirectiveUserParams()
		}
		{
			p.SetState(231)
			p.DirectiveUserLoopParams()
		}
		{
			p.SetState(232)
			p.Match(FreemarkerParserEXPR_EXIT_GT)
		}
		{
			p.SetState(233)
			p.Elements()
		}
		{
			p.SetState(234)
			p.Match(FreemarkerParserEND_USER_DIR_TAG)
		}
		{
			p.SetState(235)
			p.DirectiveUserId()
		}
		{
			p.SetState(236)
			p.Match(FreemarkerParserEXPR_EXIT_GT)
		}

	}

	return localctx
}

// IDirectiveUserIdContext is an interface to support dynamic dispatch.
type IDirectiveUserIdContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveUserIdContext differentiates from other interfaces.
	IsDirectiveUserIdContext()
}

type DirectiveUserIdContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveUserIdContext() *DirectiveUserIdContext {
	var p = new(DirectiveUserIdContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveUserId
	return p
}

func (*DirectiveUserIdContext) IsDirectiveUserIdContext() {}

func NewDirectiveUserIdContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveUserIdContext {
	var p = new(DirectiveUserIdContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveUserId

	return p
}

func (s *DirectiveUserIdContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveUserIdContext) AllEXPR_SYMBOL() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_SYMBOL)
}

func (s *DirectiveUserIdContext) EXPR_SYMBOL(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SYMBOL, i)
}

func (s *DirectiveUserIdContext) AllEXPR_DOT() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_DOT)
}

func (s *DirectiveUserIdContext) EXPR_DOT(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_DOT, i)
}

func (s *DirectiveUserIdContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveUserIdContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveUserIdContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveUserId(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveUserId() (localctx IDirectiveUserIdContext) {
	this := p
	_ = this

	localctx = NewDirectiveUserIdContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 40, FreemarkerParserRULE_directiveUserId)
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
		p.SetState(240)
		p.Match(FreemarkerParserEXPR_SYMBOL)
	}
	p.SetState(245)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == FreemarkerParserEXPR_DOT {
		{
			p.SetState(241)
			p.Match(FreemarkerParserEXPR_DOT)
		}
		{
			p.SetState(242)
			p.Match(FreemarkerParserEXPR_SYMBOL)
		}

		p.SetState(247)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// IDirectiveUserParamsContext is an interface to support dynamic dispatch.
type IDirectiveUserParamsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveUserParamsContext differentiates from other interfaces.
	IsDirectiveUserParamsContext()
}

type DirectiveUserParamsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveUserParamsContext() *DirectiveUserParamsContext {
	var p = new(DirectiveUserParamsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveUserParams
	return p
}

func (*DirectiveUserParamsContext) IsDirectiveUserParamsContext() {}

func NewDirectiveUserParamsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveUserParamsContext {
	var p = new(DirectiveUserParamsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveUserParams

	return p
}

func (s *DirectiveUserParamsContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveUserParamsContext) AllEXPR_SYMBOL() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_SYMBOL)
}

func (s *DirectiveUserParamsContext) EXPR_SYMBOL(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SYMBOL, i)
}

func (s *DirectiveUserParamsContext) AllEXPR_EQ() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_EQ)
}

func (s *DirectiveUserParamsContext) EXPR_EQ(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EQ, i)
}

func (s *DirectiveUserParamsContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *DirectiveUserParamsContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *DirectiveUserParamsContext) AllEXPR_COMMA() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_COMMA)
}

func (s *DirectiveUserParamsContext) EXPR_COMMA(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_COMMA, i)
}

func (s *DirectiveUserParamsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveUserParamsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveUserParamsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveUserParams(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveUserParams() (localctx IDirectiveUserParamsContext) {
	this := p
	_ = this

	localctx = NewDirectiveUserParamsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 42, FreemarkerParserRULE_directiveUserParams)
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

	p.SetState(268)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 18, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		p.SetState(253)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == FreemarkerParserEXPR_SYMBOL {
			{
				p.SetState(248)
				p.Match(FreemarkerParserEXPR_SYMBOL)
			}
			{
				p.SetState(249)
				p.Match(FreemarkerParserEXPR_EQ)
			}
			{
				p.SetState(250)
				p.expr(0)
			}

			p.SetState(255)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		p.SetState(266)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64((_la-22)) & ^0x3f) == 0 && ((int64(1)<<(_la-22))&35189009956867) != 0 {
			{
				p.SetState(256)
				p.expr(0)
			}
			p.SetState(263)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			for (int64((_la-22)) & ^0x3f) == 0 && ((int64(1)<<(_la-22))&39587056467971) != 0 {
				p.SetState(258)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)

				if _la == FreemarkerParserEXPR_COMMA {
					{
						p.SetState(257)
						p.Match(FreemarkerParserEXPR_COMMA)
					}

				}
				{
					p.SetState(260)
					p.expr(0)
				}

				p.SetState(265)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)
			}

		}

	}

	return localctx
}

// IDirectiveUserLoopParamsContext is an interface to support dynamic dispatch.
type IDirectiveUserLoopParamsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDirectiveUserLoopParamsContext differentiates from other interfaces.
	IsDirectiveUserLoopParamsContext()
}

type DirectiveUserLoopParamsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDirectiveUserLoopParamsContext() *DirectiveUserLoopParamsContext {
	var p = new(DirectiveUserLoopParamsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_directiveUserLoopParams
	return p
}

func (*DirectiveUserLoopParamsContext) IsDirectiveUserLoopParamsContext() {}

func NewDirectiveUserLoopParamsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DirectiveUserLoopParamsContext {
	var p = new(DirectiveUserLoopParamsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_directiveUserLoopParams

	return p
}

func (s *DirectiveUserLoopParamsContext) GetParser() antlr.Parser { return s.parser }

func (s *DirectiveUserLoopParamsContext) EXPR_SEMICOLON() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SEMICOLON, 0)
}

func (s *DirectiveUserLoopParamsContext) AllEXPR_SYMBOL() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_SYMBOL)
}

func (s *DirectiveUserLoopParamsContext) EXPR_SYMBOL(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SYMBOL, i)
}

func (s *DirectiveUserLoopParamsContext) AllEXPR_COMMA() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_COMMA)
}

func (s *DirectiveUserLoopParamsContext) EXPR_COMMA(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_COMMA, i)
}

func (s *DirectiveUserLoopParamsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DirectiveUserLoopParamsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DirectiveUserLoopParamsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDirectiveUserLoopParams(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) DirectiveUserLoopParams() (localctx IDirectiveUserLoopParamsContext) {
	this := p
	_ = this

	localctx = NewDirectiveUserLoopParamsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 44, FreemarkerParserRULE_directiveUserLoopParams)
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
	p.SetState(279)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == FreemarkerParserEXPR_SEMICOLON {
		{
			p.SetState(270)
			p.Match(FreemarkerParserEXPR_SEMICOLON)
		}
		{
			p.SetState(271)
			p.Match(FreemarkerParserEXPR_SYMBOL)
		}
		p.SetState(276)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == FreemarkerParserEXPR_COMMA {
			{
				p.SetState(272)
				p.Match(FreemarkerParserEXPR_COMMA)
			}
			{
				p.SetState(273)
				p.Match(FreemarkerParserEXPR_SYMBOL)
			}

			p.SetState(278)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}

	}

	return localctx
}

// ITagExprContext is an interface to support dynamic dispatch.
type ITagExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsTagExprContext differentiates from other interfaces.
	IsTagExprContext()
}

type TagExprContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyTagExprContext() *TagExprContext {
	var p = new(TagExprContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_tagExpr
	return p
}

func (*TagExprContext) IsTagExprContext() {}

func NewTagExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *TagExprContext {
	var p = new(TagExprContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_tagExpr

	return p
}

func (s *TagExprContext) GetParser() antlr.Parser { return s.parser }

func (s *TagExprContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *TagExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TagExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *TagExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitTagExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) TagExpr() (localctx ITagExprContext) {
	this := p
	_ = this

	localctx = NewTagExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 46, FreemarkerParserRULE_tagExpr)

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
		p.expr(0)
	}

	return localctx
}

// IInlineExprContext is an interface to support dynamic dispatch.
type IInlineExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsInlineExprContext differentiates from other interfaces.
	IsInlineExprContext()
}

type InlineExprContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyInlineExprContext() *InlineExprContext {
	var p = new(InlineExprContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_inlineExpr
	return p
}

func (*InlineExprContext) IsInlineExprContext() {}

func NewInlineExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *InlineExprContext {
	var p = new(InlineExprContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_inlineExpr

	return p
}

func (s *InlineExprContext) GetParser() antlr.Parser { return s.parser }

func (s *InlineExprContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *InlineExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *InlineExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *InlineExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitInlineExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) InlineExpr() (localctx IInlineExprContext) {
	this := p
	_ = this

	localctx = NewInlineExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 48, FreemarkerParserRULE_inlineExpr)

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
		p.expr(0)
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
	p.RuleIndex = FreemarkerParserRULE_string
	return p
}

func (*StringContext) IsStringContext() {}

func NewStringContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StringContext {
	var p = new(StringContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_string

	return p
}

func (s *StringContext) GetParser() antlr.Parser { return s.parser }

func (s *StringContext) CopyFrom(ctx *StringContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *StringContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StringContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type DoubleQuoteContext struct {
	*StringContext
}

func NewDoubleQuoteContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *DoubleQuoteContext {
	var p = new(DoubleQuoteContext)

	p.StringContext = NewEmptyStringContext()
	p.parser = parser
	p.CopyFrom(ctx.(*StringContext))

	return p
}

func (s *DoubleQuoteContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DoubleQuoteContext) Double_quote_string() IDouble_quote_stringContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDouble_quote_stringContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDouble_quote_stringContext)
}

func (s *DoubleQuoteContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDoubleQuote(s)

	default:
		return t.VisitChildren(s)
	}
}

type SingleQuoteContext struct {
	*StringContext
}

func NewSingleQuoteContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *SingleQuoteContext {
	var p = new(SingleQuoteContext)

	p.StringContext = NewEmptyStringContext()
	p.parser = parser
	p.CopyFrom(ctx.(*StringContext))

	return p
}

func (s *SingleQuoteContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SingleQuoteContext) Single_quote_string() ISingle_quote_stringContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingle_quote_stringContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingle_quote_stringContext)
}

func (s *SingleQuoteContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitSingleQuote(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) String_() (localctx IStringContext) {
	this := p
	_ = this

	localctx = NewStringContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 50, FreemarkerParserRULE_string)

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

	p.SetState(287)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case FreemarkerParserEXPR_SINGLE_STR_START:
		localctx = NewSingleQuoteContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(285)
			p.Single_quote_string()
		}

	case FreemarkerParserEXPR_DOUBLE_STR_START:
		localctx = NewDoubleQuoteContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(286)
			p.Double_quote_string()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IExprContext is an interface to support dynamic dispatch.
type IExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsExprContext differentiates from other interfaces.
	IsExprContext()
}

type ExprContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyExprContext() *ExprContext {
	var p = new(ExprContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_expr
	return p
}

func (*ExprContext) IsExprContext() {}

func NewExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ExprContext {
	var p = new(ExprContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_expr

	return p
}

func (s *ExprContext) GetParser() antlr.Parser { return s.parser }

func (s *ExprContext) CopyFrom(ctx *ExprContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *ExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type ExprUnaryOpContext struct {
	*ExprContext
	op antlr.Token
}

func NewExprUnaryOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprUnaryOpContext {
	var p = new(ExprUnaryOpContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprUnaryOpContext) GetOp() antlr.Token { return s.op }

func (s *ExprUnaryOpContext) SetOp(v antlr.Token) { s.op = v }

func (s *ExprUnaryOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprUnaryOpContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *ExprUnaryOpContext) EXPR_BANG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_BANG, 0)
}

func (s *ExprUnaryOpContext) EXPR_SUB() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SUB, 0)
}

func (s *ExprUnaryOpContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprUnaryOp(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprMulDivModContext struct {
	*ExprContext
	op antlr.Token
}

func NewExprMulDivModContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprMulDivModContext {
	var p = new(ExprMulDivModContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprMulDivModContext) GetOp() antlr.Token { return s.op }

func (s *ExprMulDivModContext) SetOp(v antlr.Token) { s.op = v }

func (s *ExprMulDivModContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprMulDivModContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *ExprMulDivModContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *ExprMulDivModContext) EXPR_MUL() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_MUL, 0)
}

func (s *ExprMulDivModContext) EXPR_DIV() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_DIV, 0)
}

func (s *ExprMulDivModContext) EXPR_MOD() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_MOD, 0)
}

func (s *ExprMulDivModContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprMulDivMod(s)

	default:
		return t.VisitChildren(s)
	}
}

type BoolExprContext struct {
	*ExprContext
}

func NewBoolExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BoolExprContext {
	var p = new(BoolExprContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *BoolExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BoolExprContext) EXPR_TRUE() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_TRUE, 0)
}

func (s *BoolExprContext) EXPR_FALSE() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_FALSE, 0)
}

func (s *BoolExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitBoolExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

type StringExprContext struct {
	*ExprContext
}

func NewStringExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *StringExprContext {
	var p = new(StringExprContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *StringExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StringExprContext) String_() IStringContext {
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

func (s *StringExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitStringExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprBoolRelationalContext struct {
	*ExprContext
}

func NewExprBoolRelationalContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprBoolRelationalContext {
	var p = new(ExprBoolRelationalContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprBoolRelationalContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprBoolRelationalContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *ExprBoolRelationalContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *ExprBoolRelationalContext) BooleanRelationalOperator() IBooleanRelationalOperatorContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBooleanRelationalOperatorContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBooleanRelationalOperatorContext)
}

func (s *ExprBoolRelationalContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprBoolRelational(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprRoundParenthesesContext struct {
	*ExprContext
}

func NewExprRoundParenthesesContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprRoundParenthesesContext {
	var p = new(ExprRoundParenthesesContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprRoundParenthesesContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprRoundParenthesesContext) EXPR_L_PAREN() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_L_PAREN, 0)
}

func (s *ExprRoundParenthesesContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *ExprRoundParenthesesContext) EXPR_R_PAREN() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_R_PAREN, 0)
}

func (s *ExprRoundParenthesesContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprRoundParentheses(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprBoolAndContext struct {
	*ExprContext
}

func NewExprBoolAndContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprBoolAndContext {
	var p = new(ExprBoolAndContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprBoolAndContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprBoolAndContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *ExprBoolAndContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *ExprBoolAndContext) EXPR_LOGICAL_AND() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_LOGICAL_AND, 0)
}

func (s *ExprBoolAndContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprBoolAnd(s)

	default:
		return t.VisitChildren(s)
	}
}

type SymbolExprContext struct {
	*ExprContext
}

func NewSymbolExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *SymbolExprContext {
	var p = new(SymbolExprContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *SymbolExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SymbolExprContext) EXPR_SYMBOL() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SYMBOL, 0)
}

func (s *SymbolExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitSymbolExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprBuiltInContext struct {
	*ExprContext
}

func NewExprBuiltInContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprBuiltInContext {
	var p = new(ExprBuiltInContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprBuiltInContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprBuiltInContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *ExprBuiltInContext) EXPR_QUESTION() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_QUESTION, 0)
}

func (s *ExprBuiltInContext) EXPR_SYMBOL() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SYMBOL, 0)
}

func (s *ExprBuiltInContext) EXPR_L_PAREN() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_L_PAREN, 0)
}

func (s *ExprBuiltInContext) FunctionParams() IFunctionParamsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFunctionParamsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFunctionParamsContext)
}

func (s *ExprBuiltInContext) EXPR_R_PAREN() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_R_PAREN, 0)
}

func (s *ExprBuiltInContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprBuiltIn(s)

	default:
		return t.VisitChildren(s)
	}
}

type StructExprContext struct {
	*ExprContext
}

func NewStructExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *StructExprContext {
	var p = new(StructExprContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *StructExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StructExprContext) Struct_() IStructContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStructContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStructContext)
}

func (s *StructExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitStructExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprMissingTestContext struct {
	*ExprContext
}

func NewExprMissingTestContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprMissingTestContext {
	var p = new(ExprMissingTestContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprMissingTestContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprMissingTestContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *ExprMissingTestContext) EXPR_DBL_QUESTION() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_DBL_QUESTION, 0)
}

func (s *ExprMissingTestContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprMissingTest(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprAddSubContext struct {
	*ExprContext
	op antlr.Token
}

func NewExprAddSubContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprAddSubContext {
	var p = new(ExprAddSubContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprAddSubContext) GetOp() antlr.Token { return s.op }

func (s *ExprAddSubContext) SetOp(v antlr.Token) { s.op = v }

func (s *ExprAddSubContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprAddSubContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *ExprAddSubContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *ExprAddSubContext) EXPR_ADD() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_ADD, 0)
}

func (s *ExprAddSubContext) EXPR_SUB() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SUB, 0)
}

func (s *ExprAddSubContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprAddSub(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprDotAccessContext struct {
	*ExprContext
}

func NewExprDotAccessContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprDotAccessContext {
	var p = new(ExprDotAccessContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprDotAccessContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprDotAccessContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *ExprDotAccessContext) AllEXPR_DOT() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_DOT)
}

func (s *ExprDotAccessContext) EXPR_DOT(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_DOT, i)
}

func (s *ExprDotAccessContext) AllEXPR_SYMBOL() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_SYMBOL)
}

func (s *ExprDotAccessContext) EXPR_SYMBOL(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SYMBOL, i)
}

func (s *ExprDotAccessContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprDotAccess(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprBoolEqContext struct {
	*ExprContext
}

func NewExprBoolEqContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprBoolEqContext {
	var p = new(ExprBoolEqContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprBoolEqContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprBoolEqContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *ExprBoolEqContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *ExprBoolEqContext) EXPR_COMPARE_EQ() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_COMPARE_EQ, 0)
}

func (s *ExprBoolEqContext) EXPR_COMPARE_NEQ() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_COMPARE_NEQ, 0)
}

func (s *ExprBoolEqContext) EXPR_EQ() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EQ, 0)
}

func (s *ExprBoolEqContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprBoolEq(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprFunctionCallContext struct {
	*ExprContext
}

func NewExprFunctionCallContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprFunctionCallContext {
	var p = new(ExprFunctionCallContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprFunctionCallContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprFunctionCallContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *ExprFunctionCallContext) EXPR_L_PAREN() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_L_PAREN, 0)
}

func (s *ExprFunctionCallContext) FunctionParams() IFunctionParamsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFunctionParamsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFunctionParamsContext)
}

func (s *ExprFunctionCallContext) EXPR_R_PAREN() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_R_PAREN, 0)
}

func (s *ExprFunctionCallContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprFunctionCall(s)

	default:
		return t.VisitChildren(s)
	}
}

type NumberExprContext struct {
	*ExprContext
}

func NewNumberExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *NumberExprContext {
	var p = new(NumberExprContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *NumberExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NumberExprContext) EXPR_NUM() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_NUM, 0)
}

func (s *NumberExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitNumberExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprDefaultContext struct {
	*ExprContext
	left  IExprContext
	right IExprContext
}

func NewExprDefaultContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprDefaultContext {
	var p = new(ExprDefaultContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprDefaultContext) GetLeft() IExprContext { return s.left }

func (s *ExprDefaultContext) GetRight() IExprContext { return s.right }

func (s *ExprDefaultContext) SetLeft(v IExprContext) { s.left = v }

func (s *ExprDefaultContext) SetRight(v IExprContext) { s.right = v }

func (s *ExprDefaultContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprDefaultContext) EXPR_BANG() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_BANG, 0)
}

func (s *ExprDefaultContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *ExprDefaultContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *ExprDefaultContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprDefault(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprSquareParenthesesContext struct {
	*ExprContext
}

func NewExprSquareParenthesesContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprSquareParenthesesContext {
	var p = new(ExprSquareParenthesesContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprSquareParenthesesContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprSquareParenthesesContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *ExprSquareParenthesesContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *ExprSquareParenthesesContext) EXPR_L_SQ_PAREN() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_L_SQ_PAREN, 0)
}

func (s *ExprSquareParenthesesContext) EXPR_R_SQ_PAREN() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_R_SQ_PAREN, 0)
}

func (s *ExprSquareParenthesesContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprSquareParentheses(s)

	default:
		return t.VisitChildren(s)
	}
}

type ExprBoolOrContext struct {
	*ExprContext
}

func NewExprBoolOrContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ExprBoolOrContext {
	var p = new(ExprBoolOrContext)

	p.ExprContext = NewEmptyExprContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExprContext))

	return p
}

func (s *ExprBoolOrContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprBoolOrContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *ExprBoolOrContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *ExprBoolOrContext) EXPR_LOGICAL_OR() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_LOGICAL_OR, 0)
}

func (s *ExprBoolOrContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitExprBoolOr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) Expr() (localctx IExprContext) {
	return p.expr(0)
}

func (p *FreemarkerParser) expr(_p int) (localctx IExprContext) {
	this := p
	_ = this

	var _parentctx antlr.ParserRuleContext = p.GetParserRuleContext()
	_parentState := p.GetState()
	localctx = NewExprContext(p, p.GetParserRuleContext(), _parentState)
	var _prevctx IExprContext = localctx
	var _ antlr.ParserRuleContext = _prevctx // TODO: To prevent unused variable warning.
	_startState := 52
	p.EnterRecursionRule(localctx, 52, FreemarkerParserRULE_expr, _p)
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
	p.SetState(301)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case FreemarkerParserEXPR_NUM:
		localctx = NewNumberExprContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx

		{
			p.SetState(290)
			p.Match(FreemarkerParserEXPR_NUM)
		}

	case FreemarkerParserEXPR_TRUE, FreemarkerParserEXPR_FALSE:
		localctx = NewBoolExprContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(291)
			_la = p.GetTokenStream().LA(1)

			if !(_la == FreemarkerParserEXPR_TRUE || _la == FreemarkerParserEXPR_FALSE) {
				p.GetErrorHandler().RecoverInline(p)
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}

	case FreemarkerParserEXPR_SYMBOL:
		localctx = NewSymbolExprContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(292)
			p.Match(FreemarkerParserEXPR_SYMBOL)
		}

	case FreemarkerParserEXPR_DOUBLE_STR_START, FreemarkerParserEXPR_SINGLE_STR_START:
		localctx = NewStringExprContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(293)
			p.String_()
		}

	case FreemarkerParserEXPR_STRUCT:
		localctx = NewStructExprContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(294)
			p.Struct_()
		}

	case FreemarkerParserEXPR_L_PAREN:
		localctx = NewExprRoundParenthesesContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(295)
			p.Match(FreemarkerParserEXPR_L_PAREN)
		}
		{
			p.SetState(296)
			p.expr(0)
		}
		{
			p.SetState(297)
			p.Match(FreemarkerParserEXPR_R_PAREN)
		}

	case FreemarkerParserEXPR_BANG, FreemarkerParserEXPR_SUB:
		localctx = NewExprUnaryOpContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(299)

			var _lt = p.GetTokenStream().LT(1)

			localctx.(*ExprUnaryOpContext).op = _lt

			_la = p.GetTokenStream().LA(1)

			if !(_la == FreemarkerParserEXPR_BANG || _la == FreemarkerParserEXPR_SUB) {
				var _ri = p.GetErrorHandler().RecoverInline(p)

				localctx.(*ExprUnaryOpContext).op = _ri
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}
		{
			p.SetState(300)
			p.expr(7)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}
	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(357)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 27, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			p.SetState(355)
			p.GetErrorHandler().Sync(p)
			switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 26, p.GetParserRuleContext()) {
			case 1:
				localctx = NewExprMulDivModContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, FreemarkerParserRULE_expr)
				p.SetState(303)

				if !(p.Precpred(p.GetParserRuleContext(), 6)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 6)", ""))
				}
				{
					p.SetState(304)

					var _lt = p.GetTokenStream().LT(1)

					localctx.(*ExprMulDivModContext).op = _lt

					_la = p.GetTokenStream().LA(1)

					if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&15762598695796736) != 0) {
						var _ri = p.GetErrorHandler().RecoverInline(p)

						localctx.(*ExprMulDivModContext).op = _ri
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(305)
					p.expr(7)
				}

			case 2:
				localctx = NewExprAddSubContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, FreemarkerParserRULE_expr)
				p.SetState(306)

				if !(p.Precpred(p.GetParserRuleContext(), 5)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 5)", ""))
				}
				{
					p.SetState(307)

					var _lt = p.GetTokenStream().LT(1)

					localctx.(*ExprAddSubContext).op = _lt

					_la = p.GetTokenStream().LA(1)

					if !(_la == FreemarkerParserEXPR_ADD || _la == FreemarkerParserEXPR_SUB) {
						var _ri = p.GetErrorHandler().RecoverInline(p)

						localctx.(*ExprAddSubContext).op = _ri
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(308)
					p.expr(6)
				}

			case 3:
				localctx = NewExprBoolRelationalContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, FreemarkerParserRULE_expr)
				p.SetState(309)

				if !(p.Precpred(p.GetParserRuleContext(), 4)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 4)", ""))
				}
				{
					p.SetState(310)
					p.BooleanRelationalOperator()
				}
				{
					p.SetState(311)
					p.expr(5)
				}

			case 4:
				localctx = NewExprBoolEqContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, FreemarkerParserRULE_expr)
				p.SetState(313)

				if !(p.Precpred(p.GetParserRuleContext(), 3)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 3)", ""))
				}
				{
					p.SetState(314)
					_la = p.GetTokenStream().LA(1)

					if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&2017612633061982208) != 0) {
						p.GetErrorHandler().RecoverInline(p)
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(315)
					p.expr(4)
				}

			case 5:
				localctx = NewExprBoolAndContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, FreemarkerParserRULE_expr)
				p.SetState(316)

				if !(p.Precpred(p.GetParserRuleContext(), 2)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 2)", ""))
				}
				{
					p.SetState(317)
					p.Match(FreemarkerParserEXPR_LOGICAL_AND)
				}
				{
					p.SetState(318)
					p.expr(3)
				}

			case 6:
				localctx = NewExprBoolOrContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, FreemarkerParserRULE_expr)
				p.SetState(319)

				if !(p.Precpred(p.GetParserRuleContext(), 1)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 1)", ""))
				}
				{
					p.SetState(320)
					p.Match(FreemarkerParserEXPR_LOGICAL_OR)
				}
				{
					p.SetState(321)
					p.expr(2)
				}

			case 7:
				localctx = NewExprDotAccessContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, FreemarkerParserRULE_expr)
				p.SetState(322)

				if !(p.Precpred(p.GetParserRuleContext(), 14)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 14)", ""))
				}
				p.SetState(325)
				p.GetErrorHandler().Sync(p)
				_alt = 1
				for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
					switch _alt {
					case 1:
						{
							p.SetState(323)
							p.Match(FreemarkerParserEXPR_DOT)
						}
						{
							p.SetState(324)
							p.Match(FreemarkerParserEXPR_SYMBOL)
						}

					default:
						panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
					}

					p.SetState(327)
					p.GetErrorHandler().Sync(p)
					_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 23, p.GetParserRuleContext())
				}

			case 8:
				localctx = NewExprMissingTestContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, FreemarkerParserRULE_expr)
				p.SetState(329)

				if !(p.Precpred(p.GetParserRuleContext(), 13)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 13)", ""))
				}
				{
					p.SetState(330)
					p.Match(FreemarkerParserEXPR_DBL_QUESTION)
				}

			case 9:
				localctx = NewExprBuiltInContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, FreemarkerParserRULE_expr)
				p.SetState(331)

				if !(p.Precpred(p.GetParserRuleContext(), 12)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 12)", ""))
				}
				{
					p.SetState(332)
					p.Match(FreemarkerParserEXPR_QUESTION)
				}
				{
					p.SetState(333)
					p.Match(FreemarkerParserEXPR_SYMBOL)
				}
				p.SetState(338)
				p.GetErrorHandler().Sync(p)

				if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 24, p.GetParserRuleContext()) == 1 {
					{
						p.SetState(334)
						p.Match(FreemarkerParserEXPR_L_PAREN)
					}
					{
						p.SetState(335)
						p.FunctionParams()
					}
					{
						p.SetState(336)
						p.Match(FreemarkerParserEXPR_R_PAREN)
					}

				}

			case 10:
				localctx = NewExprDefaultContext(p, NewExprContext(p, _parentctx, _parentState))
				localctx.(*ExprDefaultContext).left = _prevctx

				p.PushNewRecursionContext(localctx, _startState, FreemarkerParserRULE_expr)
				p.SetState(340)

				if !(p.Precpred(p.GetParserRuleContext(), 11)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 11)", ""))
				}
				{
					p.SetState(341)
					p.Match(FreemarkerParserEXPR_BANG)
				}
				p.SetState(343)
				p.GetErrorHandler().Sync(p)

				if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 25, p.GetParserRuleContext()) == 1 {
					{
						p.SetState(342)

						var _x = p.expr(0)

						localctx.(*ExprDefaultContext).right = _x
					}

				}

			case 11:
				localctx = NewExprFunctionCallContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, FreemarkerParserRULE_expr)
				p.SetState(345)

				if !(p.Precpred(p.GetParserRuleContext(), 10)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 10)", ""))
				}
				{
					p.SetState(346)
					p.Match(FreemarkerParserEXPR_L_PAREN)
				}
				{
					p.SetState(347)
					p.FunctionParams()
				}
				{
					p.SetState(348)
					p.Match(FreemarkerParserEXPR_R_PAREN)
				}

			case 12:
				localctx = NewExprSquareParenthesesContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, FreemarkerParserRULE_expr)
				p.SetState(350)

				if !(p.Precpred(p.GetParserRuleContext(), 9)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 9)", ""))
				}
				{
					p.SetState(351)
					p.Match(FreemarkerParserEXPR_L_SQ_PAREN)
				}
				{
					p.SetState(352)
					p.expr(0)
				}
				{
					p.SetState(353)
					p.Match(FreemarkerParserEXPR_R_SQ_PAREN)
				}

			}

		}
		p.SetState(359)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 27, p.GetParserRuleContext())
	}

	return localctx
}

// IFunctionParamsContext is an interface to support dynamic dispatch.
type IFunctionParamsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFunctionParamsContext differentiates from other interfaces.
	IsFunctionParamsContext()
}

type FunctionParamsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFunctionParamsContext() *FunctionParamsContext {
	var p = new(FunctionParamsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_functionParams
	return p
}

func (*FunctionParamsContext) IsFunctionParamsContext() {}

func NewFunctionParamsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FunctionParamsContext {
	var p = new(FunctionParamsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_functionParams

	return p
}

func (s *FunctionParamsContext) GetParser() antlr.Parser { return s.parser }

func (s *FunctionParamsContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *FunctionParamsContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *FunctionParamsContext) AllEXPR_COMMA() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_COMMA)
}

func (s *FunctionParamsContext) EXPR_COMMA(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_COMMA, i)
}

func (s *FunctionParamsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FunctionParamsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FunctionParamsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitFunctionParams(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) FunctionParams() (localctx IFunctionParamsContext) {
	this := p
	_ = this

	localctx = NewFunctionParamsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 54, FreemarkerParserRULE_functionParams)
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
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 29, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(361)
			p.expr(0)
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(362)
			p.expr(0)
		}
		p.SetState(365)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == FreemarkerParserEXPR_COMMA {
			{
				p.SetState(363)
				p.Match(FreemarkerParserEXPR_COMMA)
			}
			{
				p.SetState(364)
				p.expr(0)
			}

			p.SetState(367)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}

	}

	return localctx
}

// IBooleanRelationalOperatorContext is an interface to support dynamic dispatch.
type IBooleanRelationalOperatorContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsBooleanRelationalOperatorContext differentiates from other interfaces.
	IsBooleanRelationalOperatorContext()
}

type BooleanRelationalOperatorContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyBooleanRelationalOperatorContext() *BooleanRelationalOperatorContext {
	var p = new(BooleanRelationalOperatorContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_booleanRelationalOperator
	return p
}

func (*BooleanRelationalOperatorContext) IsBooleanRelationalOperatorContext() {}

func NewBooleanRelationalOperatorContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *BooleanRelationalOperatorContext {
	var p = new(BooleanRelationalOperatorContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_booleanRelationalOperator

	return p
}

func (s *BooleanRelationalOperatorContext) GetParser() antlr.Parser { return s.parser }

func (s *BooleanRelationalOperatorContext) EXPR_LT_SYM() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_LT_SYM, 0)
}

func (s *BooleanRelationalOperatorContext) EXPR_LT_STR() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_LT_STR, 0)
}

func (s *BooleanRelationalOperatorContext) EXPR_LTE_SYM() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_LTE_SYM, 0)
}

func (s *BooleanRelationalOperatorContext) EXPR_LTE_STR() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_LTE_STR, 0)
}

func (s *BooleanRelationalOperatorContext) EXPR_GT_STR() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_GT_STR, 0)
}

func (s *BooleanRelationalOperatorContext) EXPR_GTE_SYM() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_GTE_SYM, 0)
}

func (s *BooleanRelationalOperatorContext) EXPR_GTE_STR() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_GTE_STR, 0)
}

func (s *BooleanRelationalOperatorContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BooleanRelationalOperatorContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *BooleanRelationalOperatorContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitBooleanRelationalOperator(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) BooleanRelationalOperator() (localctx IBooleanRelationalOperatorContext) {
	this := p
	_ = this

	localctx = NewBooleanRelationalOperatorContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 56, FreemarkerParserRULE_booleanRelationalOperator)
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
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&68182605824) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IStructContext is an interface to support dynamic dispatch.
type IStructContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStructContext differentiates from other interfaces.
	IsStructContext()
}

type StructContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStructContext() *StructContext {
	var p = new(StructContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_struct
	return p
}

func (*StructContext) IsStructContext() {}

func NewStructContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StructContext {
	var p = new(StructContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_struct

	return p
}

func (s *StructContext) GetParser() antlr.Parser { return s.parser }

func (s *StructContext) EXPR_STRUCT() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_STRUCT, 0)
}

func (s *StructContext) EXPR_EXIT_R_BRACE() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_R_BRACE, 0)
}

func (s *StructContext) AllStruct_pair() []IStruct_pairContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStruct_pairContext); ok {
			len++
		}
	}

	tst := make([]IStruct_pairContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStruct_pairContext); ok {
			tst[i] = t.(IStruct_pairContext)
			i++
		}
	}

	return tst
}

func (s *StructContext) Struct_pair(i int) IStruct_pairContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStruct_pairContext); ok {
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

	return t.(IStruct_pairContext)
}

func (s *StructContext) AllEXPR_COMMA() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_COMMA)
}

func (s *StructContext) EXPR_COMMA(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_COMMA, i)
}

func (s *StructContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StructContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StructContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitStruct(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) Struct_() (localctx IStructContext) {
	this := p
	_ = this

	localctx = NewStructContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 58, FreemarkerParserRULE_struct)
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
		p.SetState(373)
		p.Match(FreemarkerParserEXPR_STRUCT)
	}
	p.SetState(382)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if (int64((_la-43)) & ^0x3f) == 0 && ((int64(1)<<(_la-43))&16777219) != 0 {
		{
			p.SetState(374)
			p.Struct_pair()
		}
		p.SetState(379)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == FreemarkerParserEXPR_COMMA {
			{
				p.SetState(375)
				p.Match(FreemarkerParserEXPR_COMMA)
			}
			{
				p.SetState(376)
				p.Struct_pair()
			}

			p.SetState(381)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}

	}
	{
		p.SetState(384)
		p.Match(FreemarkerParserEXPR_EXIT_R_BRACE)
	}

	return localctx
}

// IStruct_pairContext is an interface to support dynamic dispatch.
type IStruct_pairContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStruct_pairContext differentiates from other interfaces.
	IsStruct_pairContext()
}

type Struct_pairContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStruct_pairContext() *Struct_pairContext {
	var p = new(Struct_pairContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_struct_pair
	return p
}

func (*Struct_pairContext) IsStruct_pairContext() {}

func NewStruct_pairContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Struct_pairContext {
	var p = new(Struct_pairContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_struct_pair

	return p
}

func (s *Struct_pairContext) GetParser() antlr.Parser { return s.parser }

func (s *Struct_pairContext) EXPR_COLON() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_COLON, 0)
}

func (s *Struct_pairContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *Struct_pairContext) String_() IStringContext {
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

func (s *Struct_pairContext) EXPR_SYMBOL() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SYMBOL, 0)
}

func (s *Struct_pairContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Struct_pairContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Struct_pairContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitStruct_pair(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) Struct_pair() (localctx IStruct_pairContext) {
	this := p
	_ = this

	localctx = NewStruct_pairContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 60, FreemarkerParserRULE_struct_pair)

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
	p.SetState(388)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case FreemarkerParserEXPR_DOUBLE_STR_START, FreemarkerParserEXPR_SINGLE_STR_START:
		{
			p.SetState(386)
			p.String_()
		}

	case FreemarkerParserEXPR_SYMBOL:
		{
			p.SetState(387)
			p.Match(FreemarkerParserEXPR_SYMBOL)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}
	{
		p.SetState(390)
		p.Match(FreemarkerParserEXPR_COLON)
	}
	{
		p.SetState(391)
		p.expr(0)
	}

	return localctx
}

// ISingle_quote_stringContext is an interface to support dynamic dispatch.
type ISingle_quote_stringContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsSingle_quote_stringContext differentiates from other interfaces.
	IsSingle_quote_stringContext()
}

type Single_quote_stringContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySingle_quote_stringContext() *Single_quote_stringContext {
	var p = new(Single_quote_stringContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_single_quote_string
	return p
}

func (*Single_quote_stringContext) IsSingle_quote_stringContext() {}

func NewSingle_quote_stringContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Single_quote_stringContext {
	var p = new(Single_quote_stringContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_single_quote_string

	return p
}

func (s *Single_quote_stringContext) GetParser() antlr.Parser { return s.parser }

func (s *Single_quote_stringContext) EXPR_SINGLE_STR_START() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_SINGLE_STR_START, 0)
}

func (s *Single_quote_stringContext) SQS_EXIT() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSQS_EXIT, 0)
}

func (s *Single_quote_stringContext) AllSQS_CONTENT() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserSQS_CONTENT)
}

func (s *Single_quote_stringContext) SQS_CONTENT(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSQS_CONTENT, i)
}

func (s *Single_quote_stringContext) AllSQS_ESCAPE() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserSQS_ESCAPE)
}

func (s *Single_quote_stringContext) SQS_ESCAPE(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSQS_ESCAPE, i)
}

func (s *Single_quote_stringContext) AllSQS_ENTER_EXPR() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserSQS_ENTER_EXPR)
}

func (s *Single_quote_stringContext) SQS_ENTER_EXPR(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserSQS_ENTER_EXPR, i)
}

func (s *Single_quote_stringContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *Single_quote_stringContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *Single_quote_stringContext) AllEXPR_EXIT_R_BRACE() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_EXIT_R_BRACE)
}

func (s *Single_quote_stringContext) EXPR_EXIT_R_BRACE(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_R_BRACE, i)
}

func (s *Single_quote_stringContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Single_quote_stringContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Single_quote_stringContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitSingle_quote_string(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) Single_quote_string() (localctx ISingle_quote_stringContext) {
	this := p
	_ = this

	localctx = NewSingle_quote_stringContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 62, FreemarkerParserRULE_single_quote_string)
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
		p.SetState(393)
		p.Match(FreemarkerParserEXPR_SINGLE_STR_START)
	}
	p.SetState(402)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&57344) != 0 {
		p.SetState(400)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case FreemarkerParserSQS_CONTENT:
			{
				p.SetState(394)
				p.Match(FreemarkerParserSQS_CONTENT)
			}

		case FreemarkerParserSQS_ESCAPE:
			{
				p.SetState(395)
				p.Match(FreemarkerParserSQS_ESCAPE)
			}

		case FreemarkerParserSQS_ENTER_EXPR:
			{
				p.SetState(396)
				p.Match(FreemarkerParserSQS_ENTER_EXPR)
			}
			{
				p.SetState(397)
				p.expr(0)
			}
			{
				p.SetState(398)
				p.Match(FreemarkerParserEXPR_EXIT_R_BRACE)
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

		p.SetState(404)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(405)
		p.Match(FreemarkerParserSQS_EXIT)
	}

	return localctx
}

// IDouble_quote_stringContext is an interface to support dynamic dispatch.
type IDouble_quote_stringContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDouble_quote_stringContext differentiates from other interfaces.
	IsDouble_quote_stringContext()
}

type Double_quote_stringContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDouble_quote_stringContext() *Double_quote_stringContext {
	var p = new(Double_quote_stringContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = FreemarkerParserRULE_double_quote_string
	return p
}

func (*Double_quote_stringContext) IsDouble_quote_stringContext() {}

func NewDouble_quote_stringContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Double_quote_stringContext {
	var p = new(Double_quote_stringContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = FreemarkerParserRULE_double_quote_string

	return p
}

func (s *Double_quote_stringContext) GetParser() antlr.Parser { return s.parser }

func (s *Double_quote_stringContext) EXPR_DOUBLE_STR_START() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_DOUBLE_STR_START, 0)
}

func (s *Double_quote_stringContext) DQS_EXIT() antlr.TerminalNode {
	return s.GetToken(FreemarkerParserDQS_EXIT, 0)
}

func (s *Double_quote_stringContext) AllDQS_CONTENT() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserDQS_CONTENT)
}

func (s *Double_quote_stringContext) DQS_CONTENT(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserDQS_CONTENT, i)
}

func (s *Double_quote_stringContext) AllDQS_ESCAPE() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserDQS_ESCAPE)
}

func (s *Double_quote_stringContext) DQS_ESCAPE(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserDQS_ESCAPE, i)
}

func (s *Double_quote_stringContext) AllDQS_ENTER_EXPR() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserDQS_ENTER_EXPR)
}

func (s *Double_quote_stringContext) DQS_ENTER_EXPR(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserDQS_ENTER_EXPR, i)
}

func (s *Double_quote_stringContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *Double_quote_stringContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
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

	return t.(IExprContext)
}

func (s *Double_quote_stringContext) AllEXPR_EXIT_R_BRACE() []antlr.TerminalNode {
	return s.GetTokens(FreemarkerParserEXPR_EXIT_R_BRACE)
}

func (s *Double_quote_stringContext) EXPR_EXIT_R_BRACE(i int) antlr.TerminalNode {
	return s.GetToken(FreemarkerParserEXPR_EXIT_R_BRACE, i)
}

func (s *Double_quote_stringContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Double_quote_stringContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Double_quote_stringContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case FreemarkerParserVisitor:
		return t.VisitDouble_quote_string(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *FreemarkerParser) Double_quote_string() (localctx IDouble_quote_stringContext) {
	this := p
	_ = this

	localctx = NewDouble_quote_stringContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 64, FreemarkerParserRULE_double_quote_string)
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
		p.Match(FreemarkerParserEXPR_DOUBLE_STR_START)
	}
	p.SetState(416)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&3584) != 0 {
		p.SetState(414)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case FreemarkerParserDQS_CONTENT:
			{
				p.SetState(408)
				p.Match(FreemarkerParserDQS_CONTENT)
			}

		case FreemarkerParserDQS_ESCAPE:
			{
				p.SetState(409)
				p.Match(FreemarkerParserDQS_ESCAPE)
			}

		case FreemarkerParserDQS_ENTER_EXPR:
			{
				p.SetState(410)
				p.Match(FreemarkerParserDQS_ENTER_EXPR)
			}
			{
				p.SetState(411)
				p.expr(0)
			}
			{
				p.SetState(412)
				p.Match(FreemarkerParserEXPR_EXIT_R_BRACE)
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

		p.SetState(418)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(419)
		p.Match(FreemarkerParserDQS_EXIT)
	}

	return localctx
}

func (p *FreemarkerParser) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 26:
		var t *ExprContext = nil
		if localctx != nil {
			t = localctx.(*ExprContext)
		}
		return p.Expr_Sempred(t, predIndex)

	default:
		panic("No predicate with index: " + fmt.Sprint(ruleIndex))
	}
}

func (p *FreemarkerParser) Expr_Sempred(localctx antlr.RuleContext, predIndex int) bool {
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
		return p.Precpred(p.GetParserRuleContext(), 14)

	case 7:
		return p.Precpred(p.GetParserRuleContext(), 13)

	case 8:
		return p.Precpred(p.GetParserRuleContext(), 12)

	case 9:
		return p.Precpred(p.GetParserRuleContext(), 11)

	case 10:
		return p.Precpred(p.GetParserRuleContext(), 10)

	case 11:
		return p.Precpred(p.GetParserRuleContext(), 9)

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}
