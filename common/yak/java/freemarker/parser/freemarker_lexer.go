// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package freemarkerparser

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

type FreemarkerLexer struct {
	*antlr.BaseLexer
	channelNames []string
	modeNames    []string
	// TODO: EOF string
}

var freemarkerlexerLexerStaticData struct {
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

func freemarkerlexerLexerInit() {
	staticData := &freemarkerlexerLexerStaticData
	staticData.channelNames = []string{
		"DEFAULT_TOKEN_CHANNEL", "HIDDEN",
	}
	staticData.modeNames = []string{
		"DEFAULT_MODE", "DOUBLE_QUOTE_STRING_MODE", "SINGLE_QUOTE_STRING_MODE",
		"EXPR_MODE",
	}
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
		"COMMENT", "START_DIRECTIVE_TAG", "END_DIRECTIVE_TAG", "START_USER_DIR_TAG",
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
		"EXPR_COMMA", "EXPR_COLON", "EXPR_SEMICOLON", "EXPR_SYMBOL", "COMMENT_FRAG",
		"NUMBER", "SYMBOL",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 0, 67, 443, 6, -1, 6, -1, 6, -1, 6, -1, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2,
		7, 2, 2, 3, 7, 3, 2, 4, 7, 4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8,
		7, 8, 2, 9, 7, 9, 2, 10, 7, 10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13,
		2, 14, 7, 14, 2, 15, 7, 15, 2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2,
		19, 7, 19, 2, 20, 7, 20, 2, 21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24,
		7, 24, 2, 25, 7, 25, 2, 26, 7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 2, 29, 7,
		29, 2, 30, 7, 30, 2, 31, 7, 31, 2, 32, 7, 32, 2, 33, 7, 33, 2, 34, 7, 34,
		2, 35, 7, 35, 2, 36, 7, 36, 2, 37, 7, 37, 2, 38, 7, 38, 2, 39, 7, 39, 2,
		40, 7, 40, 2, 41, 7, 41, 2, 42, 7, 42, 2, 43, 7, 43, 2, 44, 7, 44, 2, 45,
		7, 45, 2, 46, 7, 46, 2, 47, 7, 47, 2, 48, 7, 48, 2, 49, 7, 49, 2, 50, 7,
		50, 2, 51, 7, 51, 2, 52, 7, 52, 2, 53, 7, 53, 2, 54, 7, 54, 2, 55, 7, 55,
		2, 56, 7, 56, 2, 57, 7, 57, 2, 58, 7, 58, 2, 59, 7, 59, 2, 60, 7, 60, 2,
		61, 7, 61, 2, 62, 7, 62, 2, 63, 7, 63, 2, 64, 7, 64, 2, 65, 7, 65, 2, 66,
		7, 66, 2, 67, 7, 67, 2, 68, 7, 68, 2, 69, 7, 69, 1, 0, 1, 0, 1, 0, 1, 0,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 3,
		1, 3, 1, 3, 1, 3, 1, 3, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 5, 1, 5,
		1, 5, 1, 5, 1, 5, 1, 6, 1, 6, 4, 6, 178, 8, 6, 11, 6, 12, 6, 179, 3, 6,
		182, 8, 6, 1, 7, 1, 7, 1, 7, 1, 7, 1, 8, 1, 8, 1, 8, 1, 9, 1, 9, 1, 9,
		1, 9, 1, 9, 1, 10, 4, 10, 197, 8, 10, 11, 10, 12, 10, 198, 1, 11, 1, 11,
		1, 11, 1, 11, 1, 12, 1, 12, 1, 12, 1, 13, 1, 13, 1, 13, 1, 13, 1, 13, 1,
		14, 4, 14, 214, 8, 14, 11, 14, 12, 14, 215, 1, 15, 1, 15, 1, 15, 1, 16,
		1, 16, 1, 16, 1, 16, 1, 16, 1, 17, 1, 17, 1, 17, 1, 17, 1, 17, 1, 17, 1,
		17, 1, 18, 1, 18, 1, 18, 1, 18, 1, 18, 1, 18, 1, 18, 1, 19, 1, 19, 1, 19,
		1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 21, 1, 21, 1, 21, 1, 21, 1, 21, 1,
		22, 1, 22, 1, 22, 1, 22, 1, 22, 1, 22, 1, 23, 1, 23, 1, 23, 1, 23, 1, 23,
		1, 23, 1, 23, 1, 23, 1, 24, 1, 24, 1, 24, 1, 24, 1, 24, 1, 24, 1, 24, 1,
		25, 1, 25, 1, 25, 1, 25, 1, 25, 1, 25, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26,
		1, 26, 1, 26, 1, 27, 1, 27, 1, 27, 1, 27, 1, 27, 1, 27, 1, 27, 1, 28, 1,
		28, 1, 29, 1, 29, 1, 29, 1, 30, 1, 30, 1, 30, 1, 31, 1, 31, 1, 31, 1, 31,
		1, 32, 1, 32, 1, 32, 1, 33, 1, 33, 1, 33, 1, 34, 1, 34, 1, 34, 1, 34, 1,
		35, 1, 35, 1, 36, 1, 36, 1, 36, 1, 36, 1, 37, 1, 37, 1, 37, 1, 37, 1, 38,
		1, 38, 1, 38, 1, 38, 1, 38, 1, 39, 4, 39, 332, 8, 39, 11, 39, 12, 39, 333,
		1, 39, 1, 39, 1, 40, 1, 40, 1, 40, 1, 40, 1, 41, 4, 41, 343, 8, 41, 11,
		41, 12, 41, 344, 1, 41, 1, 41, 1, 42, 1, 42, 1, 42, 1, 42, 1, 43, 1, 43,
		1, 43, 1, 43, 1, 44, 1, 44, 1, 45, 1, 45, 1, 45, 1, 46, 1, 46, 1, 47, 1,
		47, 1, 48, 1, 48, 1, 49, 1, 49, 1, 50, 1, 50, 1, 51, 1, 51, 1, 52, 1, 52,
		1, 53, 1, 53, 1, 54, 1, 54, 1, 55, 1, 55, 1, 56, 1, 56, 1, 57, 1, 57, 1,
		57, 1, 58, 1, 58, 1, 59, 1, 59, 1, 59, 1, 60, 1, 60, 1, 60, 1, 61, 1, 61,
		1, 61, 1, 62, 1, 62, 1, 63, 1, 63, 1, 64, 1, 64, 1, 65, 1, 65, 1, 66, 1,
		66, 1, 67, 1, 67, 1, 67, 1, 67, 1, 67, 1, 67, 5, 67, 414, 8, 67, 10, 67,
		12, 67, 417, 9, 67, 1, 67, 1, 67, 1, 67, 1, 67, 1, 68, 4, 68, 424, 8, 68,
		11, 68, 12, 68, 425, 1, 68, 1, 68, 5, 68, 430, 8, 68, 10, 68, 12, 68, 433,
		9, 68, 3, 68, 435, 8, 68, 1, 69, 1, 69, 5, 69, 439, 8, 69, 10, 69, 12,
		69, 442, 9, 69, 1, 415, 0, 70, 4, 1, 6, 2, 8, 3, 10, 4, 12, 5, 14, 6, 16,
		7, 18, 8, 20, 9, 22, 10, 24, 11, 26, 12, 28, 13, 30, 14, 32, 15, 34, 16,
		36, 17, 38, 18, 40, 19, 42, 20, 44, 21, 46, 22, 48, 23, 50, 24, 52, 25,
		54, 26, 56, 27, 58, 28, 60, 29, 62, 30, 64, 31, 66, 32, 68, 33, 70, 34,
		72, 35, 74, 36, 76, 37, 78, 38, 80, 39, 82, 40, 84, 41, 86, 42, 88, 43,
		90, 44, 92, 45, 94, 46, 96, 47, 98, 48, 100, 49, 102, 50, 104, 51, 106,
		52, 108, 53, 110, 54, 112, 55, 114, 56, 116, 57, 118, 58, 120, 59, 122,
		60, 124, 61, 126, 62, 128, 63, 130, 64, 132, 65, 134, 66, 136, 67, 138,
		0, 140, 0, 142, 0, 4, 0, 1, 2, 3, 8, 2, 0, 36, 36, 60, 60, 5, 0, 34, 34,
		36, 36, 39, 39, 92, 92, 110, 110, 3, 0, 34, 34, 36, 36, 92, 92, 3, 0, 36,
		36, 39, 39, 92, 92, 2, 0, 10, 10, 32, 32, 1, 0, 48, 57, 3, 0, 65, 90, 95,
		95, 97, 122, 4, 0, 48, 57, 65, 90, 95, 95, 97, 122, 447, 0, 4, 1, 0, 0,
		0, 0, 6, 1, 0, 0, 0, 0, 8, 1, 0, 0, 0, 0, 10, 1, 0, 0, 0, 0, 12, 1, 0,
		0, 0, 0, 14, 1, 0, 0, 0, 0, 16, 1, 0, 0, 0, 1, 18, 1, 0, 0, 0, 1, 20, 1,
		0, 0, 0, 1, 22, 1, 0, 0, 0, 1, 24, 1, 0, 0, 0, 2, 26, 1, 0, 0, 0, 2, 28,
		1, 0, 0, 0, 2, 30, 1, 0, 0, 0, 2, 32, 1, 0, 0, 0, 3, 34, 1, 0, 0, 0, 3,
		36, 1, 0, 0, 0, 3, 38, 1, 0, 0, 0, 3, 40, 1, 0, 0, 0, 3, 42, 1, 0, 0, 0,
		3, 44, 1, 0, 0, 0, 3, 46, 1, 0, 0, 0, 3, 48, 1, 0, 0, 0, 3, 50, 1, 0, 0,
		0, 3, 52, 1, 0, 0, 0, 3, 54, 1, 0, 0, 0, 3, 56, 1, 0, 0, 0, 3, 58, 1, 0,
		0, 0, 3, 60, 1, 0, 0, 0, 3, 62, 1, 0, 0, 0, 3, 64, 1, 0, 0, 0, 3, 66, 1,
		0, 0, 0, 3, 68, 1, 0, 0, 0, 3, 70, 1, 0, 0, 0, 3, 72, 1, 0, 0, 0, 3, 74,
		1, 0, 0, 0, 3, 76, 1, 0, 0, 0, 3, 78, 1, 0, 0, 0, 3, 80, 1, 0, 0, 0, 3,
		82, 1, 0, 0, 0, 3, 84, 1, 0, 0, 0, 3, 86, 1, 0, 0, 0, 3, 88, 1, 0, 0, 0,
		3, 90, 1, 0, 0, 0, 3, 92, 1, 0, 0, 0, 3, 94, 1, 0, 0, 0, 3, 96, 1, 0, 0,
		0, 3, 98, 1, 0, 0, 0, 3, 100, 1, 0, 0, 0, 3, 102, 1, 0, 0, 0, 3, 104, 1,
		0, 0, 0, 3, 106, 1, 0, 0, 0, 3, 108, 1, 0, 0, 0, 3, 110, 1, 0, 0, 0, 3,
		112, 1, 0, 0, 0, 3, 114, 1, 0, 0, 0, 3, 116, 1, 0, 0, 0, 3, 118, 1, 0,
		0, 0, 3, 120, 1, 0, 0, 0, 3, 122, 1, 0, 0, 0, 3, 124, 1, 0, 0, 0, 3, 126,
		1, 0, 0, 0, 3, 128, 1, 0, 0, 0, 3, 130, 1, 0, 0, 0, 3, 132, 1, 0, 0, 0,
		3, 134, 1, 0, 0, 0, 3, 136, 1, 0, 0, 0, 4, 144, 1, 0, 0, 0, 6, 148, 1,
		0, 0, 0, 8, 153, 1, 0, 0, 0, 10, 159, 1, 0, 0, 0, 12, 164, 1, 0, 0, 0,
		14, 170, 1, 0, 0, 0, 16, 181, 1, 0, 0, 0, 18, 183, 1, 0, 0, 0, 20, 187,
		1, 0, 0, 0, 22, 190, 1, 0, 0, 0, 24, 196, 1, 0, 0, 0, 26, 200, 1, 0, 0,
		0, 28, 204, 1, 0, 0, 0, 30, 207, 1, 0, 0, 0, 32, 213, 1, 0, 0, 0, 34, 217,
		1, 0, 0, 0, 36, 220, 1, 0, 0, 0, 38, 225, 1, 0, 0, 0, 40, 232, 1, 0, 0,
		0, 42, 239, 1, 0, 0, 0, 44, 242, 1, 0, 0, 0, 46, 247, 1, 0, 0, 0, 48, 252,
		1, 0, 0, 0, 50, 258, 1, 0, 0, 0, 52, 266, 1, 0, 0, 0, 54, 273, 1, 0, 0,
		0, 56, 279, 1, 0, 0, 0, 58, 286, 1, 0, 0, 0, 60, 293, 1, 0, 0, 0, 62, 295,
		1, 0, 0, 0, 64, 298, 1, 0, 0, 0, 66, 301, 1, 0, 0, 0, 68, 305, 1, 0, 0,
		0, 70, 308, 1, 0, 0, 0, 72, 311, 1, 0, 0, 0, 74, 315, 1, 0, 0, 0, 76, 317,
		1, 0, 0, 0, 78, 321, 1, 0, 0, 0, 80, 325, 1, 0, 0, 0, 82, 331, 1, 0, 0,
		0, 84, 337, 1, 0, 0, 0, 86, 342, 1, 0, 0, 0, 88, 348, 1, 0, 0, 0, 90, 352,
		1, 0, 0, 0, 92, 356, 1, 0, 0, 0, 94, 358, 1, 0, 0, 0, 96, 361, 1, 0, 0,
		0, 98, 363, 1, 0, 0, 0, 100, 365, 1, 0, 0, 0, 102, 367, 1, 0, 0, 0, 104,
		369, 1, 0, 0, 0, 106, 371, 1, 0, 0, 0, 108, 373, 1, 0, 0, 0, 110, 375,
		1, 0, 0, 0, 112, 377, 1, 0, 0, 0, 114, 379, 1, 0, 0, 0, 116, 381, 1, 0,
		0, 0, 118, 383, 1, 0, 0, 0, 120, 386, 1, 0, 0, 0, 122, 388, 1, 0, 0, 0,
		124, 391, 1, 0, 0, 0, 126, 394, 1, 0, 0, 0, 128, 397, 1, 0, 0, 0, 130,
		399, 1, 0, 0, 0, 132, 401, 1, 0, 0, 0, 134, 403, 1, 0, 0, 0, 136, 405,
		1, 0, 0, 0, 138, 407, 1, 0, 0, 0, 140, 423, 1, 0, 0, 0, 142, 436, 1, 0,
		0, 0, 144, 145, 3, 138, 67, 0, 145, 146, 1, 0, 0, 0, 146, 147, 6, 0, 0,
		0, 147, 5, 1, 0, 0, 0, 148, 149, 5, 60, 0, 0, 149, 150, 5, 35, 0, 0, 150,
		151, 1, 0, 0, 0, 151, 152, 6, 1, 1, 0, 152, 7, 1, 0, 0, 0, 153, 154, 5,
		60, 0, 0, 154, 155, 5, 47, 0, 0, 155, 156, 5, 35, 0, 0, 156, 157, 1, 0,
		0, 0, 157, 158, 6, 2, 1, 0, 158, 9, 1, 0, 0, 0, 159, 160, 5, 60, 0, 0,
		160, 161, 5, 64, 0, 0, 161, 162, 1, 0, 0, 0, 162, 163, 6, 3, 1, 0, 163,
		11, 1, 0, 0, 0, 164, 165, 5, 60, 0, 0, 165, 166, 5, 47, 0, 0, 166, 167,
		5, 64, 0, 0, 167, 168, 1, 0, 0, 0, 168, 169, 6, 4, 1, 0, 169, 13, 1, 0,
		0, 0, 170, 171, 5, 36, 0, 0, 171, 172, 5, 123, 0, 0, 172, 173, 1, 0, 0,
		0, 173, 174, 6, 5, 1, 0, 174, 15, 1, 0, 0, 0, 175, 182, 7, 0, 0, 0, 176,
		178, 8, 0, 0, 0, 177, 176, 1, 0, 0, 0, 178, 179, 1, 0, 0, 0, 179, 177,
		1, 0, 0, 0, 179, 180, 1, 0, 0, 0, 180, 182, 1, 0, 0, 0, 181, 175, 1, 0,
		0, 0, 181, 177, 1, 0, 0, 0, 182, 17, 1, 0, 0, 0, 183, 184, 5, 34, 0, 0,
		184, 185, 1, 0, 0, 0, 185, 186, 6, 7, 2, 0, 186, 19, 1, 0, 0, 0, 187, 188,
		5, 92, 0, 0, 188, 189, 7, 1, 0, 0, 189, 21, 1, 0, 0, 0, 190, 191, 5, 36,
		0, 0, 191, 192, 5, 123, 0, 0, 192, 193, 1, 0, 0, 0, 193, 194, 6, 9, 1,
		0, 194, 23, 1, 0, 0, 0, 195, 197, 8, 2, 0, 0, 196, 195, 1, 0, 0, 0, 197,
		198, 1, 0, 0, 0, 198, 196, 1, 0, 0, 0, 198, 199, 1, 0, 0, 0, 199, 25, 1,
		0, 0, 0, 200, 201, 5, 39, 0, 0, 201, 202, 1, 0, 0, 0, 202, 203, 6, 11,
		2, 0, 203, 27, 1, 0, 0, 0, 204, 205, 5, 92, 0, 0, 205, 206, 7, 1, 0, 0,
		206, 29, 1, 0, 0, 0, 207, 208, 5, 36, 0, 0, 208, 209, 5, 123, 0, 0, 209,
		210, 1, 0, 0, 0, 210, 211, 6, 13, 1, 0, 211, 31, 1, 0, 0, 0, 212, 214,
		8, 3, 0, 0, 213, 212, 1, 0, 0, 0, 214, 215, 1, 0, 0, 0, 215, 213, 1, 0,
		0, 0, 215, 216, 1, 0, 0, 0, 216, 33, 1, 0, 0, 0, 217, 218, 5, 105, 0, 0,
		218, 219, 5, 102, 0, 0, 219, 35, 1, 0, 0, 0, 220, 221, 5, 101, 0, 0, 221,
		222, 5, 108, 0, 0, 222, 223, 5, 115, 0, 0, 223, 224, 5, 101, 0, 0, 224,
		37, 1, 0, 0, 0, 225, 226, 5, 101, 0, 0, 226, 227, 5, 108, 0, 0, 227, 228,
		5, 115, 0, 0, 228, 229, 5, 101, 0, 0, 229, 230, 5, 105, 0, 0, 230, 231,
		5, 102, 0, 0, 231, 39, 1, 0, 0, 0, 232, 233, 5, 97, 0, 0, 233, 234, 5,
		115, 0, 0, 234, 235, 5, 115, 0, 0, 235, 236, 5, 105, 0, 0, 236, 237, 5,
		103, 0, 0, 237, 238, 5, 110, 0, 0, 238, 41, 1, 0, 0, 0, 239, 240, 5, 97,
		0, 0, 240, 241, 5, 115, 0, 0, 241, 43, 1, 0, 0, 0, 242, 243, 5, 108, 0,
		0, 243, 244, 5, 105, 0, 0, 244, 245, 5, 115, 0, 0, 245, 246, 5, 116, 0,
		0, 246, 45, 1, 0, 0, 0, 247, 248, 5, 116, 0, 0, 248, 249, 5, 114, 0, 0,
		249, 250, 5, 117, 0, 0, 250, 251, 5, 101, 0, 0, 251, 47, 1, 0, 0, 0, 252,
		253, 5, 102, 0, 0, 253, 254, 5, 97, 0, 0, 254, 255, 5, 108, 0, 0, 255,
		256, 5, 115, 0, 0, 256, 257, 5, 101, 0, 0, 257, 49, 1, 0, 0, 0, 258, 259,
		5, 105, 0, 0, 259, 260, 5, 110, 0, 0, 260, 261, 5, 99, 0, 0, 261, 262,
		5, 108, 0, 0, 262, 263, 5, 117, 0, 0, 263, 264, 5, 100, 0, 0, 264, 265,
		5, 101, 0, 0, 265, 51, 1, 0, 0, 0, 266, 267, 5, 105, 0, 0, 267, 268, 5,
		109, 0, 0, 268, 269, 5, 112, 0, 0, 269, 270, 5, 111, 0, 0, 270, 271, 5,
		114, 0, 0, 271, 272, 5, 116, 0, 0, 272, 53, 1, 0, 0, 0, 273, 274, 5, 109,
		0, 0, 274, 275, 5, 97, 0, 0, 275, 276, 5, 99, 0, 0, 276, 277, 5, 114, 0,
		0, 277, 278, 5, 111, 0, 0, 278, 55, 1, 0, 0, 0, 279, 280, 5, 110, 0, 0,
		280, 281, 5, 101, 0, 0, 281, 282, 5, 115, 0, 0, 282, 283, 5, 116, 0, 0,
		283, 284, 5, 101, 0, 0, 284, 285, 5, 100, 0, 0, 285, 57, 1, 0, 0, 0, 286,
		287, 5, 114, 0, 0, 287, 288, 5, 101, 0, 0, 288, 289, 5, 116, 0, 0, 289,
		290, 5, 117, 0, 0, 290, 291, 5, 114, 0, 0, 291, 292, 5, 110, 0, 0, 292,
		59, 1, 0, 0, 0, 293, 294, 5, 60, 0, 0, 294, 61, 1, 0, 0, 0, 295, 296, 5,
		108, 0, 0, 296, 297, 5, 116, 0, 0, 297, 63, 1, 0, 0, 0, 298, 299, 5, 60,
		0, 0, 299, 300, 5, 61, 0, 0, 300, 65, 1, 0, 0, 0, 301, 302, 5, 108, 0,
		0, 302, 303, 5, 116, 0, 0, 303, 304, 5, 101, 0, 0, 304, 67, 1, 0, 0, 0,
		305, 306, 5, 103, 0, 0, 306, 307, 5, 116, 0, 0, 307, 69, 1, 0, 0, 0, 308,
		309, 5, 62, 0, 0, 309, 310, 5, 61, 0, 0, 310, 71, 1, 0, 0, 0, 311, 312,
		5, 103, 0, 0, 312, 313, 5, 116, 0, 0, 313, 314, 5, 101, 0, 0, 314, 73,
		1, 0, 0, 0, 315, 316, 3, 140, 68, 0, 316, 75, 1, 0, 0, 0, 317, 318, 5,
		125, 0, 0, 318, 319, 1, 0, 0, 0, 319, 320, 6, 36, 2, 0, 320, 77, 1, 0,
		0, 0, 321, 322, 5, 62, 0, 0, 322, 323, 1, 0, 0, 0, 323, 324, 6, 37, 2,
		0, 324, 79, 1, 0, 0, 0, 325, 326, 5, 47, 0, 0, 326, 327, 5, 62, 0, 0, 327,
		328, 1, 0, 0, 0, 328, 329, 6, 38, 2, 0, 329, 81, 1, 0, 0, 0, 330, 332,
		7, 4, 0, 0, 331, 330, 1, 0, 0, 0, 332, 333, 1, 0, 0, 0, 333, 331, 1, 0,
		0, 0, 333, 334, 1, 0, 0, 0, 334, 335, 1, 0, 0, 0, 335, 336, 6, 39, 3, 0,
		336, 83, 1, 0, 0, 0, 337, 338, 3, 138, 67, 0, 338, 339, 1, 0, 0, 0, 339,
		340, 6, 40, 0, 0, 340, 85, 1, 0, 0, 0, 341, 343, 5, 123, 0, 0, 342, 341,
		1, 0, 0, 0, 343, 344, 1, 0, 0, 0, 344, 342, 1, 0, 0, 0, 344, 345, 1, 0,
		0, 0, 345, 346, 1, 0, 0, 0, 346, 347, 6, 41, 1, 0, 347, 87, 1, 0, 0, 0,
		348, 349, 5, 34, 0, 0, 349, 350, 1, 0, 0, 0, 350, 351, 6, 42, 4, 0, 351,
		89, 1, 0, 0, 0, 352, 353, 5, 39, 0, 0, 353, 354, 1, 0, 0, 0, 354, 355,
		6, 43, 5, 0, 355, 91, 1, 0, 0, 0, 356, 357, 5, 64, 0, 0, 357, 93, 1, 0,
		0, 0, 358, 359, 5, 63, 0, 0, 359, 360, 5, 63, 0, 0, 360, 95, 1, 0, 0, 0,
		361, 362, 5, 63, 0, 0, 362, 97, 1, 0, 0, 0, 363, 364, 5, 33, 0, 0, 364,
		99, 1, 0, 0, 0, 365, 366, 5, 43, 0, 0, 366, 101, 1, 0, 0, 0, 367, 368,
		5, 45, 0, 0, 368, 103, 1, 0, 0, 0, 369, 370, 5, 42, 0, 0, 370, 105, 1,
		0, 0, 0, 371, 372, 5, 47, 0, 0, 372, 107, 1, 0, 0, 0, 373, 374, 5, 37,
		0, 0, 374, 109, 1, 0, 0, 0, 375, 376, 5, 40, 0, 0, 376, 111, 1, 0, 0, 0,
		377, 378, 5, 41, 0, 0, 378, 113, 1, 0, 0, 0, 379, 380, 5, 91, 0, 0, 380,
		115, 1, 0, 0, 0, 381, 382, 5, 93, 0, 0, 382, 117, 1, 0, 0, 0, 383, 384,
		5, 61, 0, 0, 384, 385, 5, 61, 0, 0, 385, 119, 1, 0, 0, 0, 386, 387, 5,
		61, 0, 0, 387, 121, 1, 0, 0, 0, 388, 389, 5, 33, 0, 0, 389, 390, 5, 61,
		0, 0, 390, 123, 1, 0, 0, 0, 391, 392, 5, 38, 0, 0, 392, 393, 5, 38, 0,
		0, 393, 125, 1, 0, 0, 0, 394, 395, 5, 124, 0, 0, 395, 396, 5, 124, 0, 0,
		396, 127, 1, 0, 0, 0, 397, 398, 5, 46, 0, 0, 398, 129, 1, 0, 0, 0, 399,
		400, 5, 44, 0, 0, 400, 131, 1, 0, 0, 0, 401, 402, 5, 58, 0, 0, 402, 133,
		1, 0, 0, 0, 403, 404, 5, 59, 0, 0, 404, 135, 1, 0, 0, 0, 405, 406, 3, 142,
		69, 0, 406, 137, 1, 0, 0, 0, 407, 408, 5, 60, 0, 0, 408, 409, 5, 35, 0,
		0, 409, 410, 5, 45, 0, 0, 410, 411, 5, 45, 0, 0, 411, 415, 1, 0, 0, 0,
		412, 414, 9, 0, 0, 0, 413, 412, 1, 0, 0, 0, 414, 417, 1, 0, 0, 0, 415,
		416, 1, 0, 0, 0, 415, 413, 1, 0, 0, 0, 416, 418, 1, 0, 0, 0, 417, 415,
		1, 0, 0, 0, 418, 419, 5, 45, 0, 0, 419, 420, 5, 45, 0, 0, 420, 421, 5,
		62, 0, 0, 421, 139, 1, 0, 0, 0, 422, 424, 7, 5, 0, 0, 423, 422, 1, 0, 0,
		0, 424, 425, 1, 0, 0, 0, 425, 423, 1, 0, 0, 0, 425, 426, 1, 0, 0, 0, 426,
		434, 1, 0, 0, 0, 427, 431, 5, 46, 0, 0, 428, 430, 7, 5, 0, 0, 429, 428,
		1, 0, 0, 0, 430, 433, 1, 0, 0, 0, 431, 429, 1, 0, 0, 0, 431, 432, 1, 0,
		0, 0, 432, 435, 1, 0, 0, 0, 433, 431, 1, 0, 0, 0, 434, 427, 1, 0, 0, 0,
		434, 435, 1, 0, 0, 0, 435, 141, 1, 0, 0, 0, 436, 440, 7, 6, 0, 0, 437,
		439, 7, 7, 0, 0, 438, 437, 1, 0, 0, 0, 439, 442, 1, 0, 0, 0, 440, 438,
		1, 0, 0, 0, 440, 441, 1, 0, 0, 0, 441, 143, 1, 0, 0, 0, 442, 440, 1, 0,
		0, 0, 15, 0, 1, 2, 3, 179, 181, 198, 215, 333, 344, 415, 425, 431, 434,
		440, 6, 0, 1, 0, 5, 3, 0, 4, 0, 0, 0, 2, 0, 5, 1, 0, 5, 2, 0,
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

// FreemarkerLexerInit initializes any static state used to implement FreemarkerLexer. By default the
// static state used to implement the lexer is lazily initialized during the first call to
// NewFreemarkerLexer(). You can call this function if you wish to initialize the static state ahead
// of time.
func FreemarkerLexerInit() {
	staticData := &freemarkerlexerLexerStaticData
	staticData.once.Do(freemarkerlexerLexerInit)
}

// NewFreemarkerLexer produces a new lexer instance for the optional input antlr.CharStream.
func NewFreemarkerLexer(input antlr.CharStream) *FreemarkerLexer {
	FreemarkerLexerInit()
	l := new(FreemarkerLexer)
	l.BaseLexer = antlr.NewBaseLexer(input)
	staticData := &freemarkerlexerLexerStaticData
	l.Interpreter = antlr.NewLexerATNSimulator(l, staticData.atn, staticData.decisionToDFA, staticData.predictionContextCache)
	l.channelNames = staticData.channelNames
	l.modeNames = staticData.modeNames
	l.RuleNames = staticData.ruleNames
	l.LiteralNames = staticData.literalNames
	l.SymbolicNames = staticData.symbolicNames
	l.GrammarFileName = "FreemarkerLexer.g4"
	// TODO: l.EOF = antlr.TokenEOF

	return l
}

// FreemarkerLexer tokens.
const (
	FreemarkerLexerCOMMENT               = 1
	FreemarkerLexerSTART_DIRECTIVE_TAG   = 2
	FreemarkerLexerEND_DIRECTIVE_TAG     = 3
	FreemarkerLexerSTART_USER_DIR_TAG    = 4
	FreemarkerLexerEND_USER_DIR_TAG      = 5
	FreemarkerLexerINLINE_EXPR_START     = 6
	FreemarkerLexerCONTENT               = 7
	FreemarkerLexerDQS_EXIT              = 8
	FreemarkerLexerDQS_ESCAPE            = 9
	FreemarkerLexerDQS_ENTER_EXPR        = 10
	FreemarkerLexerDQS_CONTENT           = 11
	FreemarkerLexerSQS_EXIT              = 12
	FreemarkerLexerSQS_ESCAPE            = 13
	FreemarkerLexerSQS_ENTER_EXPR        = 14
	FreemarkerLexerSQS_CONTENT           = 15
	FreemarkerLexerEXPR_IF               = 16
	FreemarkerLexerEXPR_ELSE             = 17
	FreemarkerLexerEXPR_ELSEIF           = 18
	FreemarkerLexerEXPR_ASSIGN           = 19
	FreemarkerLexerEXPR_AS               = 20
	FreemarkerLexerEXPR_LIST             = 21
	FreemarkerLexerEXPR_TRUE             = 22
	FreemarkerLexerEXPR_FALSE            = 23
	FreemarkerLexerEXPR_INCLUDE          = 24
	FreemarkerLexerEXPR_IMPORT           = 25
	FreemarkerLexerEXPR_MACRO            = 26
	FreemarkerLexerEXPR_NESTED           = 27
	FreemarkerLexerEXPR_RETURN           = 28
	FreemarkerLexerEXPR_LT_SYM           = 29
	FreemarkerLexerEXPR_LT_STR           = 30
	FreemarkerLexerEXPR_LTE_SYM          = 31
	FreemarkerLexerEXPR_LTE_STR          = 32
	FreemarkerLexerEXPR_GT_STR           = 33
	FreemarkerLexerEXPR_GTE_SYM          = 34
	FreemarkerLexerEXPR_GTE_STR          = 35
	FreemarkerLexerEXPR_NUM              = 36
	FreemarkerLexerEXPR_EXIT_R_BRACE     = 37
	FreemarkerLexerEXPR_EXIT_GT          = 38
	FreemarkerLexerEXPR_EXIT_DIV_GT      = 39
	FreemarkerLexerEXPR_WS               = 40
	FreemarkerLexerEXPR_COMENT           = 41
	FreemarkerLexerEXPR_STRUCT           = 42
	FreemarkerLexerEXPR_DOUBLE_STR_START = 43
	FreemarkerLexerEXPR_SINGLE_STR_START = 44
	FreemarkerLexerEXPR_AT               = 45
	FreemarkerLexerEXPR_DBL_QUESTION     = 46
	FreemarkerLexerEXPR_QUESTION         = 47
	FreemarkerLexerEXPR_BANG             = 48
	FreemarkerLexerEXPR_ADD              = 49
	FreemarkerLexerEXPR_SUB              = 50
	FreemarkerLexerEXPR_MUL              = 51
	FreemarkerLexerEXPR_DIV              = 52
	FreemarkerLexerEXPR_MOD              = 53
	FreemarkerLexerEXPR_L_PAREN          = 54
	FreemarkerLexerEXPR_R_PAREN          = 55
	FreemarkerLexerEXPR_L_SQ_PAREN       = 56
	FreemarkerLexerEXPR_R_SQ_PAREN       = 57
	FreemarkerLexerEXPR_COMPARE_EQ       = 58
	FreemarkerLexerEXPR_EQ               = 59
	FreemarkerLexerEXPR_COMPARE_NEQ      = 60
	FreemarkerLexerEXPR_LOGICAL_AND      = 61
	FreemarkerLexerEXPR_LOGICAL_OR       = 62
	FreemarkerLexerEXPR_DOT              = 63
	FreemarkerLexerEXPR_COMMA            = 64
	FreemarkerLexerEXPR_COLON            = 65
	FreemarkerLexerEXPR_SEMICOLON        = 66
	FreemarkerLexerEXPR_SYMBOL           = 67
)

// FreemarkerLexer modes.
const (
	FreemarkerLexerDOUBLE_QUOTE_STRING_MODE = iota + 1
	FreemarkerLexerSINGLE_QUOTE_STRING_MODE
	FreemarkerLexerEXPR_MODE
)
