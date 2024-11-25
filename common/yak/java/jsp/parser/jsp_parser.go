// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package jspparser // JSPParser
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

type JSPParser struct {
	*antlr.BaseParser
}

var jspparserParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	literalNames           []string
	symbolicNames          []string
	ruleNames              []string
	predictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func jspparserParserInit() {
	staticData := &jspparserParserStaticData
	staticData.literalNames = []string{
		"", "", "", "'<!--'", "'-->'", "", "'<!['", "']>'", "'<?xml'", "", "",
		"'<!DOCTYPE'", "", "", "", "", "", "", "", "", "", "'\"'", "'''", "",
		"", "", "", "", "", "'%>'", "", "", "", "'PUBLIC'", "'SYSTEM'", "",
		"", "", "", "", "", "':'", "", "'/'",
	}
	staticData.symbolicNames = []string{
		"", "JSP_COMMENT_START", "JSP_COMMENT_END", "JSP_COMMENT_START_TAG",
		"JSP_COMMENT_END_TAG", "JSP_CONDITIONAL_COMMENT_START", "JSP_CONDITIONAL_COMMENT_START_TAG",
		"JSP_CONDITIONAL_COMMENT_END_TAG", "XML_DECLARATION", "CDATA", "DTD",
		"DTD_START", "WHITESPACE_SKIP", "CLOSE_TAG_BEGIN", "TAG_BEGIN", "DIRECTIVE_BEGIN",
		"DECLARATION_BEGIN", "ECHO_EXPRESSION_OPEN", "SCRIPTLET_OPEN", "EXPRESSION_OPEN",
		"WHITESPACES", "DOUBLE_QUOTE", "SINGLE_QUOTE", "QUOTE", "TAG_END", "EQUALS",
		"JSP_STATIC_CONTENT_CHARS_MIXED", "JSP_STATIC_CONTENT_CHARS", "JSP_STATIC_CONTENT_CHAR",
		"JSP_END", "JSP_CONDITIONAL_COMMENT_END", "JSP_CONDITIONAL_COMMENT",
		"JSP_COMMENT_TEXT", "DTD_PUBLIC", "DTD_SYSTEM", "DTD_WHITESPACE_SKIP",
		"DTD_QUOTED", "DTD_IDENTIFIER", "BLOB_CLOSE", "BLOB_CONTENT", "JSPEXPR_CONTENT_CLOSE",
		"JSP_JSTL_COLON", "TAG_SLASH_END", "TAG_SLASH", "DIRECTIVE_END", "TAG_IDENTIFIER",
		"TAG_WHITESPACE", "SCRIPT_BODY", "SCRIPT_SHORT_BODY", "STYLE_BODY",
		"STYLE_SHORT_BODY", "ATTVAL_ATTRIBUTE", "EL_EXPR",
	}
	staticData.ruleNames = []string{
		"jspDocument", "jspStart", "jspElements", "jspElement", "htmlTag", "jspDirective",
		"htmlContents", "htmlContent", "jspExpression", "htmlAttribute", "htmlAttributeName",
		"htmlAttributeValue", "htmlAttributeValueExpr", "htmlAttributeValueConstant",
		"htmlTagName", "htmlChardata", "htmlMisc", "htmlComment", "htmlCommentText",
		"htmlConditionalCommentText", "xhtmlCDATA", "dtd", "dtdElementName",
		"publicId", "systemId", "xml", "scriptlet",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 52, 274, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7, 20, 2,
		21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25, 2, 26,
		7, 26, 1, 0, 5, 0, 56, 8, 0, 10, 0, 12, 0, 59, 9, 0, 1, 0, 1, 0, 5, 0,
		63, 8, 0, 10, 0, 12, 0, 66, 9, 0, 1, 0, 1, 0, 5, 0, 70, 8, 0, 10, 0, 12,
		0, 73, 9, 0, 1, 0, 5, 0, 76, 8, 0, 10, 0, 12, 0, 79, 9, 0, 3, 0, 81, 8,
		0, 1, 1, 1, 1, 1, 1, 3, 1, 86, 8, 1, 1, 2, 5, 2, 89, 8, 2, 10, 2, 12, 2,
		92, 9, 2, 1, 2, 1, 2, 1, 2, 3, 2, 97, 8, 2, 1, 2, 5, 2, 100, 8, 2, 10,
		2, 12, 2, 103, 9, 2, 1, 3, 1, 3, 1, 3, 5, 3, 108, 8, 3, 10, 3, 12, 3, 111,
		9, 3, 1, 3, 1, 3, 1, 3, 1, 3, 1, 3, 1, 3, 1, 3, 1, 3, 1, 3, 5, 3, 122,
		8, 3, 10, 3, 12, 3, 125, 9, 3, 1, 3, 1, 3, 1, 3, 1, 3, 1, 3, 5, 3, 132,
		8, 3, 10, 3, 12, 3, 135, 9, 3, 1, 3, 1, 3, 3, 3, 139, 8, 3, 1, 4, 1, 4,
		1, 4, 3, 4, 144, 8, 4, 1, 5, 1, 5, 1, 5, 5, 5, 149, 8, 5, 10, 5, 12, 5,
		152, 9, 5, 1, 5, 5, 5, 155, 8, 5, 10, 5, 12, 5, 158, 9, 5, 1, 5, 1, 5,
		1, 6, 5, 6, 163, 8, 6, 10, 6, 12, 6, 166, 9, 6, 1, 7, 1, 7, 1, 7, 1, 7,
		1, 7, 1, 7, 1, 7, 3, 7, 175, 8, 7, 1, 8, 1, 8, 1, 9, 1, 9, 1, 9, 1, 9,
		1, 9, 1, 9, 3, 9, 185, 8, 9, 1, 10, 1, 10, 1, 11, 1, 11, 1, 11, 1, 11,
		1, 11, 3, 11, 194, 8, 11, 1, 11, 1, 11, 3, 11, 198, 8, 11, 1, 11, 1, 11,
		3, 11, 202, 8, 11, 1, 11, 3, 11, 205, 8, 11, 1, 12, 1, 12, 1, 13, 1, 13,
		1, 14, 1, 14, 1, 15, 1, 15, 1, 16, 1, 16, 1, 16, 1, 16, 3, 16, 219, 8,
		16, 1, 17, 1, 17, 3, 17, 223, 8, 17, 1, 17, 1, 17, 1, 17, 3, 17, 228, 8,
		17, 1, 17, 3, 17, 231, 8, 17, 1, 18, 4, 18, 234, 8, 18, 11, 18, 12, 18,
		235, 1, 19, 1, 19, 1, 20, 1, 20, 1, 21, 1, 21, 1, 21, 1, 21, 3, 21, 246,
		8, 21, 1, 21, 1, 21, 3, 21, 250, 8, 21, 1, 21, 1, 21, 1, 22, 1, 22, 1,
		23, 1, 23, 1, 24, 1, 24, 1, 25, 1, 25, 1, 25, 5, 25, 263, 8, 25, 10, 25,
		12, 25, 266, 9, 25, 1, 25, 1, 25, 1, 26, 1, 26, 1, 26, 1, 26, 1, 26, 3,
		150, 235, 264, 0, 27, 0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26,
		28, 30, 32, 34, 36, 38, 40, 42, 44, 46, 48, 50, 52, 0, 1, 2, 0, 20, 20,
		26, 27, 290, 0, 80, 1, 0, 0, 0, 2, 85, 1, 0, 0, 0, 4, 90, 1, 0, 0, 0, 6,
		138, 1, 0, 0, 0, 8, 140, 1, 0, 0, 0, 10, 145, 1, 0, 0, 0, 12, 164, 1, 0,
		0, 0, 14, 174, 1, 0, 0, 0, 16, 176, 1, 0, 0, 0, 18, 184, 1, 0, 0, 0, 20,
		186, 1, 0, 0, 0, 22, 204, 1, 0, 0, 0, 24, 206, 1, 0, 0, 0, 26, 208, 1,
		0, 0, 0, 28, 210, 1, 0, 0, 0, 30, 212, 1, 0, 0, 0, 32, 218, 1, 0, 0, 0,
		34, 230, 1, 0, 0, 0, 36, 233, 1, 0, 0, 0, 38, 237, 1, 0, 0, 0, 40, 239,
		1, 0, 0, 0, 42, 241, 1, 0, 0, 0, 44, 253, 1, 0, 0, 0, 46, 255, 1, 0, 0,
		0, 48, 257, 1, 0, 0, 0, 50, 259, 1, 0, 0, 0, 52, 269, 1, 0, 0, 0, 54, 56,
		3, 2, 1, 0, 55, 54, 1, 0, 0, 0, 56, 59, 1, 0, 0, 0, 57, 55, 1, 0, 0, 0,
		57, 58, 1, 0, 0, 0, 58, 60, 1, 0, 0, 0, 59, 57, 1, 0, 0, 0, 60, 81, 3,
		50, 25, 0, 61, 63, 3, 2, 1, 0, 62, 61, 1, 0, 0, 0, 63, 66, 1, 0, 0, 0,
		64, 62, 1, 0, 0, 0, 64, 65, 1, 0, 0, 0, 65, 67, 1, 0, 0, 0, 66, 64, 1,
		0, 0, 0, 67, 81, 3, 42, 21, 0, 68, 70, 3, 2, 1, 0, 69, 68, 1, 0, 0, 0,
		70, 73, 1, 0, 0, 0, 71, 69, 1, 0, 0, 0, 71, 72, 1, 0, 0, 0, 72, 77, 1,
		0, 0, 0, 73, 71, 1, 0, 0, 0, 74, 76, 3, 4, 2, 0, 75, 74, 1, 0, 0, 0, 76,
		79, 1, 0, 0, 0, 77, 75, 1, 0, 0, 0, 77, 78, 1, 0, 0, 0, 78, 81, 1, 0, 0,
		0, 79, 77, 1, 0, 0, 0, 80, 57, 1, 0, 0, 0, 80, 64, 1, 0, 0, 0, 80, 71,
		1, 0, 0, 0, 81, 1, 1, 0, 0, 0, 82, 86, 3, 10, 5, 0, 83, 86, 3, 52, 26,
		0, 84, 86, 5, 20, 0, 0, 85, 82, 1, 0, 0, 0, 85, 83, 1, 0, 0, 0, 85, 84,
		1, 0, 0, 0, 86, 3, 1, 0, 0, 0, 87, 89, 3, 32, 16, 0, 88, 87, 1, 0, 0, 0,
		89, 92, 1, 0, 0, 0, 90, 88, 1, 0, 0, 0, 90, 91, 1, 0, 0, 0, 91, 96, 1,
		0, 0, 0, 92, 90, 1, 0, 0, 0, 93, 97, 3, 6, 3, 0, 94, 97, 3, 10, 5, 0, 95,
		97, 3, 52, 26, 0, 96, 93, 1, 0, 0, 0, 96, 94, 1, 0, 0, 0, 96, 95, 1, 0,
		0, 0, 97, 101, 1, 0, 0, 0, 98, 100, 3, 32, 16, 0, 99, 98, 1, 0, 0, 0, 100,
		103, 1, 0, 0, 0, 101, 99, 1, 0, 0, 0, 101, 102, 1, 0, 0, 0, 102, 5, 1,
		0, 0, 0, 103, 101, 1, 0, 0, 0, 104, 105, 5, 14, 0, 0, 105, 109, 3, 8, 4,
		0, 106, 108, 3, 18, 9, 0, 107, 106, 1, 0, 0, 0, 108, 111, 1, 0, 0, 0, 109,
		107, 1, 0, 0, 0, 109, 110, 1, 0, 0, 0, 110, 112, 1, 0, 0, 0, 111, 109,
		1, 0, 0, 0, 112, 113, 5, 24, 0, 0, 113, 114, 3, 12, 6, 0, 114, 115, 5,
		13, 0, 0, 115, 116, 3, 28, 14, 0, 116, 117, 5, 24, 0, 0, 117, 139, 1, 0,
		0, 0, 118, 119, 5, 14, 0, 0, 119, 123, 3, 8, 4, 0, 120, 122, 3, 18, 9,
		0, 121, 120, 1, 0, 0, 0, 122, 125, 1, 0, 0, 0, 123, 121, 1, 0, 0, 0, 123,
		124, 1, 0, 0, 0, 124, 126, 1, 0, 0, 0, 125, 123, 1, 0, 0, 0, 126, 127,
		5, 42, 0, 0, 127, 139, 1, 0, 0, 0, 128, 129, 5, 14, 0, 0, 129, 133, 3,
		8, 4, 0, 130, 132, 3, 18, 9, 0, 131, 130, 1, 0, 0, 0, 132, 135, 1, 0, 0,
		0, 133, 131, 1, 0, 0, 0, 133, 134, 1, 0, 0, 0, 134, 136, 1, 0, 0, 0, 135,
		133, 1, 0, 0, 0, 136, 137, 5, 24, 0, 0, 137, 139, 1, 0, 0, 0, 138, 104,
		1, 0, 0, 0, 138, 118, 1, 0, 0, 0, 138, 128, 1, 0, 0, 0, 139, 7, 1, 0, 0,
		0, 140, 143, 3, 28, 14, 0, 141, 142, 5, 41, 0, 0, 142, 144, 3, 28, 14,
		0, 143, 141, 1, 0, 0, 0, 143, 144, 1, 0, 0, 0, 144, 9, 1, 0, 0, 0, 145,
		146, 5, 15, 0, 0, 146, 150, 3, 28, 14, 0, 147, 149, 3, 18, 9, 0, 148, 147,
		1, 0, 0, 0, 149, 152, 1, 0, 0, 0, 150, 151, 1, 0, 0, 0, 150, 148, 1, 0,
		0, 0, 151, 156, 1, 0, 0, 0, 152, 150, 1, 0, 0, 0, 153, 155, 5, 46, 0, 0,
		154, 153, 1, 0, 0, 0, 155, 158, 1, 0, 0, 0, 156, 154, 1, 0, 0, 0, 156,
		157, 1, 0, 0, 0, 157, 159, 1, 0, 0, 0, 158, 156, 1, 0, 0, 0, 159, 160,
		5, 44, 0, 0, 160, 11, 1, 0, 0, 0, 161, 163, 3, 14, 7, 0, 162, 161, 1, 0,
		0, 0, 163, 166, 1, 0, 0, 0, 164, 162, 1, 0, 0, 0, 164, 165, 1, 0, 0, 0,
		165, 13, 1, 0, 0, 0, 166, 164, 1, 0, 0, 0, 167, 175, 3, 30, 15, 0, 168,
		175, 3, 16, 8, 0, 169, 175, 3, 6, 3, 0, 170, 175, 3, 40, 20, 0, 171, 175,
		3, 34, 17, 0, 172, 175, 3, 52, 26, 0, 173, 175, 3, 10, 5, 0, 174, 167,
		1, 0, 0, 0, 174, 168, 1, 0, 0, 0, 174, 169, 1, 0, 0, 0, 174, 170, 1, 0,
		0, 0, 174, 171, 1, 0, 0, 0, 174, 172, 1, 0, 0, 0, 174, 173, 1, 0, 0, 0,
		175, 15, 1, 0, 0, 0, 176, 177, 5, 52, 0, 0, 177, 17, 1, 0, 0, 0, 178, 179,
		3, 20, 10, 0, 179, 180, 5, 25, 0, 0, 180, 181, 3, 22, 11, 0, 181, 185,
		1, 0, 0, 0, 182, 185, 3, 20, 10, 0, 183, 185, 3, 52, 26, 0, 184, 178, 1,
		0, 0, 0, 184, 182, 1, 0, 0, 0, 184, 183, 1, 0, 0, 0, 185, 19, 1, 0, 0,
		0, 186, 187, 5, 45, 0, 0, 187, 21, 1, 0, 0, 0, 188, 189, 5, 23, 0, 0, 189,
		190, 3, 6, 3, 0, 190, 191, 5, 23, 0, 0, 191, 205, 1, 0, 0, 0, 192, 194,
		5, 23, 0, 0, 193, 192, 1, 0, 0, 0, 193, 194, 1, 0, 0, 0, 194, 195, 1, 0,
		0, 0, 195, 197, 3, 24, 12, 0, 196, 198, 5, 23, 0, 0, 197, 196, 1, 0, 0,
		0, 197, 198, 1, 0, 0, 0, 198, 205, 1, 0, 0, 0, 199, 201, 5, 23, 0, 0, 200,
		202, 3, 26, 13, 0, 201, 200, 1, 0, 0, 0, 201, 202, 1, 0, 0, 0, 202, 203,
		1, 0, 0, 0, 203, 205, 5, 23, 0, 0, 204, 188, 1, 0, 0, 0, 204, 193, 1, 0,
		0, 0, 204, 199, 1, 0, 0, 0, 205, 23, 1, 0, 0, 0, 206, 207, 5, 52, 0, 0,
		207, 25, 1, 0, 0, 0, 208, 209, 5, 51, 0, 0, 209, 27, 1, 0, 0, 0, 210, 211,
		5, 45, 0, 0, 211, 29, 1, 0, 0, 0, 212, 213, 7, 0, 0, 0, 213, 31, 1, 0,
		0, 0, 214, 219, 3, 34, 17, 0, 215, 219, 3, 30, 15, 0, 216, 219, 3, 16,
		8, 0, 217, 219, 3, 52, 26, 0, 218, 214, 1, 0, 0, 0, 218, 215, 1, 0, 0,
		0, 218, 216, 1, 0, 0, 0, 218, 217, 1, 0, 0, 0, 219, 33, 1, 0, 0, 0, 220,
		222, 5, 1, 0, 0, 221, 223, 3, 36, 18, 0, 222, 221, 1, 0, 0, 0, 222, 223,
		1, 0, 0, 0, 223, 224, 1, 0, 0, 0, 224, 231, 5, 2, 0, 0, 225, 227, 5, 5,
		0, 0, 226, 228, 3, 38, 19, 0, 227, 226, 1, 0, 0, 0, 227, 228, 1, 0, 0,
		0, 228, 229, 1, 0, 0, 0, 229, 231, 5, 30, 0, 0, 230, 220, 1, 0, 0, 0, 230,
		225, 1, 0, 0, 0, 231, 35, 1, 0, 0, 0, 232, 234, 5, 32, 0, 0, 233, 232,
		1, 0, 0, 0, 234, 235, 1, 0, 0, 0, 235, 236, 1, 0, 0, 0, 235, 233, 1, 0,
		0, 0, 236, 37, 1, 0, 0, 0, 237, 238, 5, 31, 0, 0, 238, 39, 1, 0, 0, 0,
		239, 240, 5, 9, 0, 0, 240, 41, 1, 0, 0, 0, 241, 242, 5, 10, 0, 0, 242,
		245, 3, 44, 22, 0, 243, 244, 5, 33, 0, 0, 244, 246, 3, 46, 23, 0, 245,
		243, 1, 0, 0, 0, 245, 246, 1, 0, 0, 0, 246, 249, 1, 0, 0, 0, 247, 248,
		5, 34, 0, 0, 248, 250, 3, 48, 24, 0, 249, 247, 1, 0, 0, 0, 249, 250, 1,
		0, 0, 0, 250, 251, 1, 0, 0, 0, 251, 252, 5, 24, 0, 0, 252, 43, 1, 0, 0,
		0, 253, 254, 5, 37, 0, 0, 254, 45, 1, 0, 0, 0, 255, 256, 5, 36, 0, 0, 256,
		47, 1, 0, 0, 0, 257, 258, 5, 36, 0, 0, 258, 49, 1, 0, 0, 0, 259, 260, 5,
		8, 0, 0, 260, 264, 3, 28, 14, 0, 261, 263, 3, 18, 9, 0, 262, 261, 1, 0,
		0, 0, 263, 266, 1, 0, 0, 0, 264, 265, 1, 0, 0, 0, 264, 262, 1, 0, 0, 0,
		265, 267, 1, 0, 0, 0, 266, 264, 1, 0, 0, 0, 267, 268, 5, 24, 0, 0, 268,
		51, 1, 0, 0, 0, 269, 270, 5, 18, 0, 0, 270, 271, 5, 39, 0, 0, 271, 272,
		5, 29, 0, 0, 272, 53, 1, 0, 0, 0, 31, 57, 64, 71, 77, 80, 85, 90, 96, 101,
		109, 123, 133, 138, 143, 150, 156, 164, 174, 184, 193, 197, 201, 204, 218,
		222, 227, 230, 235, 245, 249, 264,
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

// JSPParserInit initializes any static state used to implement JSPParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewJSPParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func JSPParserInit() {
	staticData := &jspparserParserStaticData
	staticData.once.Do(jspparserParserInit)
}

// NewJSPParser produces a new parser instance for the optional input antlr.TokenStream.
func NewJSPParser(input antlr.TokenStream) *JSPParser {
	JSPParserInit()
	this := new(JSPParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &jspparserParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.predictionContextCache)
	this.RuleNames = staticData.ruleNames
	this.LiteralNames = staticData.literalNames
	this.SymbolicNames = staticData.symbolicNames
	this.GrammarFileName = "java-escape"

	return this
}

// JSPParser tokens.
const (
	JSPParserEOF                               = antlr.TokenEOF
	JSPParserJSP_COMMENT_START                 = 1
	JSPParserJSP_COMMENT_END                   = 2
	JSPParserJSP_COMMENT_START_TAG             = 3
	JSPParserJSP_COMMENT_END_TAG               = 4
	JSPParserJSP_CONDITIONAL_COMMENT_START     = 5
	JSPParserJSP_CONDITIONAL_COMMENT_START_TAG = 6
	JSPParserJSP_CONDITIONAL_COMMENT_END_TAG   = 7
	JSPParserXML_DECLARATION                   = 8
	JSPParserCDATA                             = 9
	JSPParserDTD                               = 10
	JSPParserDTD_START                         = 11
	JSPParserWHITESPACE_SKIP                   = 12
	JSPParserCLOSE_TAG_BEGIN                   = 13
	JSPParserTAG_BEGIN                         = 14
	JSPParserDIRECTIVE_BEGIN                   = 15
	JSPParserDECLARATION_BEGIN                 = 16
	JSPParserECHO_EXPRESSION_OPEN              = 17
	JSPParserSCRIPTLET_OPEN                    = 18
	JSPParserEXPRESSION_OPEN                   = 19
	JSPParserWHITESPACES                       = 20
	JSPParserDOUBLE_QUOTE                      = 21
	JSPParserSINGLE_QUOTE                      = 22
	JSPParserQUOTE                             = 23
	JSPParserTAG_END                           = 24
	JSPParserEQUALS                            = 25
	JSPParserJSP_STATIC_CONTENT_CHARS_MIXED    = 26
	JSPParserJSP_STATIC_CONTENT_CHARS          = 27
	JSPParserJSP_STATIC_CONTENT_CHAR           = 28
	JSPParserJSP_END                           = 29
	JSPParserJSP_CONDITIONAL_COMMENT_END       = 30
	JSPParserJSP_CONDITIONAL_COMMENT           = 31
	JSPParserJSP_COMMENT_TEXT                  = 32
	JSPParserDTD_PUBLIC                        = 33
	JSPParserDTD_SYSTEM                        = 34
	JSPParserDTD_WHITESPACE_SKIP               = 35
	JSPParserDTD_QUOTED                        = 36
	JSPParserDTD_IDENTIFIER                    = 37
	JSPParserBLOB_CLOSE                        = 38
	JSPParserBLOB_CONTENT                      = 39
	JSPParserJSPEXPR_CONTENT_CLOSE             = 40
	JSPParserJSP_JSTL_COLON                    = 41
	JSPParserTAG_SLASH_END                     = 42
	JSPParserTAG_SLASH                         = 43
	JSPParserDIRECTIVE_END                     = 44
	JSPParserTAG_IDENTIFIER                    = 45
	JSPParserTAG_WHITESPACE                    = 46
	JSPParserSCRIPT_BODY                       = 47
	JSPParserSCRIPT_SHORT_BODY                 = 48
	JSPParserSTYLE_BODY                        = 49
	JSPParserSTYLE_SHORT_BODY                  = 50
	JSPParserATTVAL_ATTRIBUTE                  = 51
	JSPParserEL_EXPR                           = 52
)

// JSPParser rules.
const (
	JSPParserRULE_jspDocument                = 0
	JSPParserRULE_jspStart                   = 1
	JSPParserRULE_jspElements                = 2
	JSPParserRULE_jspElement                 = 3
	JSPParserRULE_htmlTag                    = 4
	JSPParserRULE_jspDirective               = 5
	JSPParserRULE_htmlContents               = 6
	JSPParserRULE_htmlContent                = 7
	JSPParserRULE_jspExpression              = 8
	JSPParserRULE_htmlAttribute              = 9
	JSPParserRULE_htmlAttributeName          = 10
	JSPParserRULE_htmlAttributeValue         = 11
	JSPParserRULE_htmlAttributeValueExpr     = 12
	JSPParserRULE_htmlAttributeValueConstant = 13
	JSPParserRULE_htmlTagName                = 14
	JSPParserRULE_htmlChardata               = 15
	JSPParserRULE_htmlMisc                   = 16
	JSPParserRULE_htmlComment                = 17
	JSPParserRULE_htmlCommentText            = 18
	JSPParserRULE_htmlConditionalCommentText = 19
	JSPParserRULE_xhtmlCDATA                 = 20
	JSPParserRULE_dtd                        = 21
	JSPParserRULE_dtdElementName             = 22
	JSPParserRULE_publicId                   = 23
	JSPParserRULE_systemId                   = 24
	JSPParserRULE_xml                        = 25
	JSPParserRULE_scriptlet                  = 26
)

// IJspDocumentContext is an interface to support dynamic dispatch.
type IJspDocumentContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsJspDocumentContext differentiates from other interfaces.
	IsJspDocumentContext()
}

type JspDocumentContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyJspDocumentContext() *JspDocumentContext {
	var p = new(JspDocumentContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_jspDocument
	return p
}

func (*JspDocumentContext) IsJspDocumentContext() {}

func NewJspDocumentContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *JspDocumentContext {
	var p = new(JspDocumentContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_jspDocument

	return p
}

func (s *JspDocumentContext) GetParser() antlr.Parser { return s.parser }

func (s *JspDocumentContext) Xml() IXmlContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IXmlContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IXmlContext)
}

func (s *JspDocumentContext) AllJspStart() []IJspStartContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IJspStartContext); ok {
			len++
		}
	}

	tst := make([]IJspStartContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IJspStartContext); ok {
			tst[i] = t.(IJspStartContext)
			i++
		}
	}

	return tst
}

func (s *JspDocumentContext) JspStart(i int) IJspStartContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspStartContext); ok {
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

	return t.(IJspStartContext)
}

func (s *JspDocumentContext) Dtd() IDtdContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDtdContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDtdContext)
}

func (s *JspDocumentContext) AllJspElements() []IJspElementsContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IJspElementsContext); ok {
			len++
		}
	}

	tst := make([]IJspElementsContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IJspElementsContext); ok {
			tst[i] = t.(IJspElementsContext)
			i++
		}
	}

	return tst
}

func (s *JspDocumentContext) JspElements(i int) IJspElementsContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspElementsContext); ok {
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

	return t.(IJspElementsContext)
}

func (s *JspDocumentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspDocumentContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *JspDocumentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJspDocument(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) JspDocument() (localctx IJspDocumentContext) {
	this := p
	_ = this

	localctx = NewJspDocumentContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, JSPParserRULE_jspDocument)
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

	p.SetState(80)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 4, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		p.SetState(57)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&1343488) != 0 {
			{
				p.SetState(54)
				p.JspStart()
			}

			p.SetState(59)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(60)
			p.Xml()
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		p.SetState(64)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&1343488) != 0 {
			{
				p.SetState(61)
				p.JspStart()
			}

			p.SetState(66)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(67)
			p.Dtd()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		p.SetState(71)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 2, p.GetParserRuleContext())

		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(68)
					p.JspStart()
				}

			}
			p.SetState(73)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 2, p.GetParserRuleContext())
		}
		p.SetState(77)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&4503599830056994) != 0 {
			{
				p.SetState(74)
				p.JspElements()
			}

			p.SetState(79)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}

	}

	return localctx
}

// IJspStartContext is an interface to support dynamic dispatch.
type IJspStartContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsJspStartContext differentiates from other interfaces.
	IsJspStartContext()
}

type JspStartContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyJspStartContext() *JspStartContext {
	var p = new(JspStartContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_jspStart
	return p
}

func (*JspStartContext) IsJspStartContext() {}

func NewJspStartContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *JspStartContext {
	var p = new(JspStartContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_jspStart

	return p
}

func (s *JspStartContext) GetParser() antlr.Parser { return s.parser }

func (s *JspStartContext) JspDirective() IJspDirectiveContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspDirectiveContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspDirectiveContext)
}

func (s *JspStartContext) Scriptlet() IScriptletContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IScriptletContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IScriptletContext)
}

func (s *JspStartContext) WHITESPACES() antlr.TerminalNode {
	return s.GetToken(JSPParserWHITESPACES, 0)
}

func (s *JspStartContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspStartContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *JspStartContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJspStart(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) JspStart() (localctx IJspStartContext) {
	this := p
	_ = this

	localctx = NewJspStartContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, JSPParserRULE_jspStart)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(85)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserDIRECTIVE_BEGIN:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(82)
			p.JspDirective()
		}

	case JSPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(83)
			p.Scriptlet()
		}

	case JSPParserWHITESPACES:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(84)
			p.Match(JSPParserWHITESPACES)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IJspElementsContext is an interface to support dynamic dispatch.
type IJspElementsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsJspElementsContext differentiates from other interfaces.
	IsJspElementsContext()
}

type JspElementsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyJspElementsContext() *JspElementsContext {
	var p = new(JspElementsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_jspElements
	return p
}

func (*JspElementsContext) IsJspElementsContext() {}

func NewJspElementsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *JspElementsContext {
	var p = new(JspElementsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_jspElements

	return p
}

func (s *JspElementsContext) GetParser() antlr.Parser { return s.parser }

func (s *JspElementsContext) JspElement() IJspElementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspElementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspElementContext)
}

func (s *JspElementsContext) JspDirective() IJspDirectiveContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspDirectiveContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspDirectiveContext)
}

func (s *JspElementsContext) Scriptlet() IScriptletContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IScriptletContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IScriptletContext)
}

func (s *JspElementsContext) AllHtmlMisc() []IHtmlMiscContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlMiscContext); ok {
			len++
		}
	}

	tst := make([]IHtmlMiscContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlMiscContext); ok {
			tst[i] = t.(IHtmlMiscContext)
			i++
		}
	}

	return tst
}

func (s *JspElementsContext) HtmlMisc(i int) IHtmlMiscContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlMiscContext); ok {
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

	return t.(IHtmlMiscContext)
}

func (s *JspElementsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspElementsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *JspElementsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJspElements(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) JspElements() (localctx IJspElementsContext) {
	this := p
	_ = this

	localctx = NewJspElementsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, JSPParserRULE_jspElements)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	p.SetState(90)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 6, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(87)
				p.HtmlMisc()
			}

		}
		p.SetState(92)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 6, p.GetParserRuleContext())
	}
	p.SetState(96)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserTAG_BEGIN:
		{
			p.SetState(93)
			p.JspElement()
		}

	case JSPParserDIRECTIVE_BEGIN:
		{
			p.SetState(94)
			p.JspDirective()
		}

	case JSPParserSCRIPTLET_OPEN:
		{
			p.SetState(95)
			p.Scriptlet()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}
	p.SetState(101)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 8, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(98)
				p.HtmlMisc()
			}

		}
		p.SetState(103)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 8, p.GetParserRuleContext())
	}

	return localctx
}

// IJspElementContext is an interface to support dynamic dispatch.
type IJspElementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsJspElementContext differentiates from other interfaces.
	IsJspElementContext()
}

type JspElementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyJspElementContext() *JspElementContext {
	var p = new(JspElementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_jspElement
	return p
}

func (*JspElementContext) IsJspElementContext() {}

func NewJspElementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *JspElementContext {
	var p = new(JspElementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_jspElement

	return p
}

func (s *JspElementContext) GetParser() antlr.Parser { return s.parser }

func (s *JspElementContext) CopyFrom(ctx *JspElementContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *JspElementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspElementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type JspElementWithOpenTagOnlyContext struct {
	*JspElementContext
}

func NewJspElementWithOpenTagOnlyContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *JspElementWithOpenTagOnlyContext {
	var p = new(JspElementWithOpenTagOnlyContext)

	p.JspElementContext = NewEmptyJspElementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*JspElementContext))

	return p
}

func (s *JspElementWithOpenTagOnlyContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspElementWithOpenTagOnlyContext) TAG_BEGIN() antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_BEGIN, 0)
}

func (s *JspElementWithOpenTagOnlyContext) HtmlTag() IHtmlTagContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlTagContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlTagContext)
}

func (s *JspElementWithOpenTagOnlyContext) TAG_END() antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_END, 0)
}

func (s *JspElementWithOpenTagOnlyContext) AllHtmlAttribute() []IHtmlAttributeContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlAttributeContext); ok {
			len++
		}
	}

	tst := make([]IHtmlAttributeContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlAttributeContext); ok {
			tst[i] = t.(IHtmlAttributeContext)
			i++
		}
	}

	return tst
}

func (s *JspElementWithOpenTagOnlyContext) HtmlAttribute(i int) IHtmlAttributeContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlAttributeContext); ok {
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

	return t.(IHtmlAttributeContext)
}

func (s *JspElementWithOpenTagOnlyContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJspElementWithOpenTagOnly(s)

	default:
		return t.VisitChildren(s)
	}
}

type JspElementWithSelfClosingTagContext struct {
	*JspElementContext
}

func NewJspElementWithSelfClosingTagContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *JspElementWithSelfClosingTagContext {
	var p = new(JspElementWithSelfClosingTagContext)

	p.JspElementContext = NewEmptyJspElementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*JspElementContext))

	return p
}

func (s *JspElementWithSelfClosingTagContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspElementWithSelfClosingTagContext) TAG_BEGIN() antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_BEGIN, 0)
}

func (s *JspElementWithSelfClosingTagContext) HtmlTag() IHtmlTagContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlTagContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlTagContext)
}

func (s *JspElementWithSelfClosingTagContext) TAG_SLASH_END() antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_SLASH_END, 0)
}

func (s *JspElementWithSelfClosingTagContext) AllHtmlAttribute() []IHtmlAttributeContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlAttributeContext); ok {
			len++
		}
	}

	tst := make([]IHtmlAttributeContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlAttributeContext); ok {
			tst[i] = t.(IHtmlAttributeContext)
			i++
		}
	}

	return tst
}

func (s *JspElementWithSelfClosingTagContext) HtmlAttribute(i int) IHtmlAttributeContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlAttributeContext); ok {
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

	return t.(IHtmlAttributeContext)
}

func (s *JspElementWithSelfClosingTagContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJspElementWithSelfClosingTag(s)

	default:
		return t.VisitChildren(s)
	}
}

type JspElementWithTagAndContentContext struct {
	*JspElementContext
}

func NewJspElementWithTagAndContentContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *JspElementWithTagAndContentContext {
	var p = new(JspElementWithTagAndContentContext)

	p.JspElementContext = NewEmptyJspElementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*JspElementContext))

	return p
}

func (s *JspElementWithTagAndContentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspElementWithTagAndContentContext) TAG_BEGIN() antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_BEGIN, 0)
}

func (s *JspElementWithTagAndContentContext) HtmlTag() IHtmlTagContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlTagContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlTagContext)
}

func (s *JspElementWithTagAndContentContext) AllTAG_END() []antlr.TerminalNode {
	return s.GetTokens(JSPParserTAG_END)
}

func (s *JspElementWithTagAndContentContext) TAG_END(i int) antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_END, i)
}

func (s *JspElementWithTagAndContentContext) HtmlContents() IHtmlContentsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlContentsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlContentsContext)
}

func (s *JspElementWithTagAndContentContext) CLOSE_TAG_BEGIN() antlr.TerminalNode {
	return s.GetToken(JSPParserCLOSE_TAG_BEGIN, 0)
}

func (s *JspElementWithTagAndContentContext) HtmlTagName() IHtmlTagNameContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlTagNameContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlTagNameContext)
}

func (s *JspElementWithTagAndContentContext) AllHtmlAttribute() []IHtmlAttributeContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlAttributeContext); ok {
			len++
		}
	}

	tst := make([]IHtmlAttributeContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlAttributeContext); ok {
			tst[i] = t.(IHtmlAttributeContext)
			i++
		}
	}

	return tst
}

func (s *JspElementWithTagAndContentContext) HtmlAttribute(i int) IHtmlAttributeContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlAttributeContext); ok {
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

	return t.(IHtmlAttributeContext)
}

func (s *JspElementWithTagAndContentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJspElementWithTagAndContent(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) JspElement() (localctx IJspElementContext) {
	this := p
	_ = this

	localctx = NewJspElementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, JSPParserRULE_jspElement)
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

	p.SetState(138)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 12, p.GetParserRuleContext()) {
	case 1:
		localctx = NewJspElementWithTagAndContentContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(104)
			p.Match(JSPParserTAG_BEGIN)
		}
		{
			p.SetState(105)
			p.HtmlTag()
		}
		p.SetState(109)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == JSPParserSCRIPTLET_OPEN || _la == JSPParserTAG_IDENTIFIER {
			{
				p.SetState(106)
				p.HtmlAttribute()
			}

			p.SetState(111)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(112)
			p.Match(JSPParserTAG_END)
		}
		{
			p.SetState(113)
			p.HtmlContents()
		}
		{
			p.SetState(114)
			p.Match(JSPParserCLOSE_TAG_BEGIN)
		}
		{
			p.SetState(115)
			p.HtmlTagName()
		}
		{
			p.SetState(116)
			p.Match(JSPParserTAG_END)
		}

	case 2:
		localctx = NewJspElementWithSelfClosingTagContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(118)
			p.Match(JSPParserTAG_BEGIN)
		}
		{
			p.SetState(119)
			p.HtmlTag()
		}
		p.SetState(123)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == JSPParserSCRIPTLET_OPEN || _la == JSPParserTAG_IDENTIFIER {
			{
				p.SetState(120)
				p.HtmlAttribute()
			}

			p.SetState(125)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(126)
			p.Match(JSPParserTAG_SLASH_END)
		}

	case 3:
		localctx = NewJspElementWithOpenTagOnlyContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(128)
			p.Match(JSPParserTAG_BEGIN)
		}
		{
			p.SetState(129)
			p.HtmlTag()
		}
		p.SetState(133)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == JSPParserSCRIPTLET_OPEN || _la == JSPParserTAG_IDENTIFIER {
			{
				p.SetState(130)
				p.HtmlAttribute()
			}

			p.SetState(135)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(136)
			p.Match(JSPParserTAG_END)
		}

	}

	return localctx
}

// IHtmlTagContext is an interface to support dynamic dispatch.
type IHtmlTagContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlTagContext differentiates from other interfaces.
	IsHtmlTagContext()
}

type HtmlTagContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlTagContext() *HtmlTagContext {
	var p = new(HtmlTagContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlTag
	return p
}

func (*HtmlTagContext) IsHtmlTagContext() {}

func NewHtmlTagContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlTagContext {
	var p = new(HtmlTagContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlTag

	return p
}

func (s *HtmlTagContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlTagContext) AllHtmlTagName() []IHtmlTagNameContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlTagNameContext); ok {
			len++
		}
	}

	tst := make([]IHtmlTagNameContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlTagNameContext); ok {
			tst[i] = t.(IHtmlTagNameContext)
			i++
		}
	}

	return tst
}

func (s *HtmlTagContext) HtmlTagName(i int) IHtmlTagNameContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlTagNameContext); ok {
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

	return t.(IHtmlTagNameContext)
}

func (s *HtmlTagContext) JSP_JSTL_COLON() antlr.TerminalNode {
	return s.GetToken(JSPParserJSP_JSTL_COLON, 0)
}

func (s *HtmlTagContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlTagContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlTagContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlTag(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlTag() (localctx IHtmlTagContext) {
	this := p
	_ = this

	localctx = NewHtmlTagContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, JSPParserRULE_htmlTag)
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
		p.SetState(140)
		p.HtmlTagName()
	}
	p.SetState(143)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == JSPParserJSP_JSTL_COLON {
		{
			p.SetState(141)
			p.Match(JSPParserJSP_JSTL_COLON)
		}
		{
			p.SetState(142)
			p.HtmlTagName()
		}

	}

	return localctx
}

// IJspDirectiveContext is an interface to support dynamic dispatch.
type IJspDirectiveContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// GetName returns the name rule contexts.
	GetName() IHtmlTagNameContext

	// Get_htmlAttribute returns the _htmlAttribute rule contexts.
	Get_htmlAttribute() IHtmlAttributeContext

	// SetName sets the name rule contexts.
	SetName(IHtmlTagNameContext)

	// Set_htmlAttribute sets the _htmlAttribute rule contexts.
	Set_htmlAttribute(IHtmlAttributeContext)

	// GetAtts returns the atts rule context list.
	GetAtts() []IHtmlAttributeContext

	// SetAtts sets the atts rule context list.
	SetAtts([]IHtmlAttributeContext)

	// IsJspDirectiveContext differentiates from other interfaces.
	IsJspDirectiveContext()
}

type JspDirectiveContext struct {
	*antlr.BaseParserRuleContext
	parser         antlr.Parser
	name           IHtmlTagNameContext
	_htmlAttribute IHtmlAttributeContext
	atts           []IHtmlAttributeContext
}

func NewEmptyJspDirectiveContext() *JspDirectiveContext {
	var p = new(JspDirectiveContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_jspDirective
	return p
}

func (*JspDirectiveContext) IsJspDirectiveContext() {}

func NewJspDirectiveContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *JspDirectiveContext {
	var p = new(JspDirectiveContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_jspDirective

	return p
}

func (s *JspDirectiveContext) GetParser() antlr.Parser { return s.parser }

func (s *JspDirectiveContext) GetName() IHtmlTagNameContext { return s.name }

func (s *JspDirectiveContext) Get_htmlAttribute() IHtmlAttributeContext { return s._htmlAttribute }

func (s *JspDirectiveContext) SetName(v IHtmlTagNameContext) { s.name = v }

func (s *JspDirectiveContext) Set_htmlAttribute(v IHtmlAttributeContext) { s._htmlAttribute = v }

func (s *JspDirectiveContext) GetAtts() []IHtmlAttributeContext { return s.atts }

func (s *JspDirectiveContext) SetAtts(v []IHtmlAttributeContext) { s.atts = v }

func (s *JspDirectiveContext) DIRECTIVE_BEGIN() antlr.TerminalNode {
	return s.GetToken(JSPParserDIRECTIVE_BEGIN, 0)
}

func (s *JspDirectiveContext) DIRECTIVE_END() antlr.TerminalNode {
	return s.GetToken(JSPParserDIRECTIVE_END, 0)
}

func (s *JspDirectiveContext) HtmlTagName() IHtmlTagNameContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlTagNameContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlTagNameContext)
}

func (s *JspDirectiveContext) AllTAG_WHITESPACE() []antlr.TerminalNode {
	return s.GetTokens(JSPParserTAG_WHITESPACE)
}

func (s *JspDirectiveContext) TAG_WHITESPACE(i int) antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_WHITESPACE, i)
}

func (s *JspDirectiveContext) AllHtmlAttribute() []IHtmlAttributeContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlAttributeContext); ok {
			len++
		}
	}

	tst := make([]IHtmlAttributeContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlAttributeContext); ok {
			tst[i] = t.(IHtmlAttributeContext)
			i++
		}
	}

	return tst
}

func (s *JspDirectiveContext) HtmlAttribute(i int) IHtmlAttributeContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlAttributeContext); ok {
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

	return t.(IHtmlAttributeContext)
}

func (s *JspDirectiveContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspDirectiveContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *JspDirectiveContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJspDirective(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) JspDirective() (localctx IJspDirectiveContext) {
	this := p
	_ = this

	localctx = NewJspDirectiveContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, JSPParserRULE_jspDirective)
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
		p.SetState(145)
		p.Match(JSPParserDIRECTIVE_BEGIN)
	}
	{
		p.SetState(146)

		var _x = p.HtmlTagName()

		localctx.(*JspDirectiveContext).name = _x
	}
	p.SetState(150)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 14, p.GetParserRuleContext())

	for _alt != 1 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1+1 {
			{
				p.SetState(147)

				var _x = p.HtmlAttribute()

				localctx.(*JspDirectiveContext)._htmlAttribute = _x
			}
			localctx.(*JspDirectiveContext).atts = append(localctx.(*JspDirectiveContext).atts, localctx.(*JspDirectiveContext)._htmlAttribute)

		}
		p.SetState(152)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 14, p.GetParserRuleContext())
	}
	p.SetState(156)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == JSPParserTAG_WHITESPACE {
		{
			p.SetState(153)
			p.Match(JSPParserTAG_WHITESPACE)
		}

		p.SetState(158)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(159)
		p.Match(JSPParserDIRECTIVE_END)
	}

	return localctx
}

// IHtmlContentsContext is an interface to support dynamic dispatch.
type IHtmlContentsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlContentsContext differentiates from other interfaces.
	IsHtmlContentsContext()
}

type HtmlContentsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlContentsContext() *HtmlContentsContext {
	var p = new(HtmlContentsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlContents
	return p
}

func (*HtmlContentsContext) IsHtmlContentsContext() {}

func NewHtmlContentsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlContentsContext {
	var p = new(HtmlContentsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlContents

	return p
}

func (s *HtmlContentsContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlContentsContext) AllHtmlContent() []IHtmlContentContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlContentContext); ok {
			len++
		}
	}

	tst := make([]IHtmlContentContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlContentContext); ok {
			tst[i] = t.(IHtmlContentContext)
			i++
		}
	}

	return tst
}

func (s *HtmlContentsContext) HtmlContent(i int) IHtmlContentContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlContentContext); ok {
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

	return t.(IHtmlContentContext)
}

func (s *HtmlContentsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlContentsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlContentsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlContents(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlContents() (localctx IHtmlContentsContext) {
	this := p
	_ = this

	localctx = NewHtmlContentsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, JSPParserRULE_htmlContents)
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
	p.SetState(164)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&4503599830057506) != 0 {
		{
			p.SetState(161)
			p.HtmlContent()
		}

		p.SetState(166)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// IHtmlContentContext is an interface to support dynamic dispatch.
type IHtmlContentContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlContentContext differentiates from other interfaces.
	IsHtmlContentContext()
}

type HtmlContentContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlContentContext() *HtmlContentContext {
	var p = new(HtmlContentContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlContent
	return p
}

func (*HtmlContentContext) IsHtmlContentContext() {}

func NewHtmlContentContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlContentContext {
	var p = new(HtmlContentContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlContent

	return p
}

func (s *HtmlContentContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlContentContext) HtmlChardata() IHtmlChardataContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlChardataContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlChardataContext)
}

func (s *HtmlContentContext) JspExpression() IJspExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspExpressionContext)
}

func (s *HtmlContentContext) JspElement() IJspElementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspElementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspElementContext)
}

func (s *HtmlContentContext) XhtmlCDATA() IXhtmlCDATAContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IXhtmlCDATAContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IXhtmlCDATAContext)
}

func (s *HtmlContentContext) HtmlComment() IHtmlCommentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlCommentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlCommentContext)
}

func (s *HtmlContentContext) Scriptlet() IScriptletContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IScriptletContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IScriptletContext)
}

func (s *HtmlContentContext) JspDirective() IJspDirectiveContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspDirectiveContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspDirectiveContext)
}

func (s *HtmlContentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlContentContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlContentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlContent(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlContent() (localctx IHtmlContentContext) {
	this := p
	_ = this

	localctx = NewHtmlContentContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 14, JSPParserRULE_htmlContent)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(174)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserWHITESPACES, JSPParserJSP_STATIC_CONTENT_CHARS_MIXED, JSPParserJSP_STATIC_CONTENT_CHARS:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(167)
			p.HtmlChardata()
		}

	case JSPParserEL_EXPR:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(168)
			p.JspExpression()
		}

	case JSPParserTAG_BEGIN:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(169)
			p.JspElement()
		}

	case JSPParserCDATA:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(170)
			p.XhtmlCDATA()
		}

	case JSPParserJSP_COMMENT_START, JSPParserJSP_CONDITIONAL_COMMENT_START:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(171)
			p.HtmlComment()
		}

	case JSPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(172)
			p.Scriptlet()
		}

	case JSPParserDIRECTIVE_BEGIN:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(173)
			p.JspDirective()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IJspExpressionContext is an interface to support dynamic dispatch.
type IJspExpressionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsJspExpressionContext differentiates from other interfaces.
	IsJspExpressionContext()
}

type JspExpressionContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyJspExpressionContext() *JspExpressionContext {
	var p = new(JspExpressionContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_jspExpression
	return p
}

func (*JspExpressionContext) IsJspExpressionContext() {}

func NewJspExpressionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *JspExpressionContext {
	var p = new(JspExpressionContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_jspExpression

	return p
}

func (s *JspExpressionContext) GetParser() antlr.Parser { return s.parser }

func (s *JspExpressionContext) EL_EXPR() antlr.TerminalNode {
	return s.GetToken(JSPParserEL_EXPR, 0)
}

func (s *JspExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspExpressionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *JspExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJspExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) JspExpression() (localctx IJspExpressionContext) {
	this := p
	_ = this

	localctx = NewJspExpressionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 16, JSPParserRULE_jspExpression)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(176)
		p.Match(JSPParserEL_EXPR)
	}

	return localctx
}

// IHtmlAttributeContext is an interface to support dynamic dispatch.
type IHtmlAttributeContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlAttributeContext differentiates from other interfaces.
	IsHtmlAttributeContext()
}

type HtmlAttributeContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlAttributeContext() *HtmlAttributeContext {
	var p = new(HtmlAttributeContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlAttribute
	return p
}

func (*HtmlAttributeContext) IsHtmlAttributeContext() {}

func NewHtmlAttributeContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlAttributeContext {
	var p = new(HtmlAttributeContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlAttribute

	return p
}

func (s *HtmlAttributeContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlAttributeContext) HtmlAttributeName() IHtmlAttributeNameContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlAttributeNameContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlAttributeNameContext)
}

func (s *HtmlAttributeContext) EQUALS() antlr.TerminalNode {
	return s.GetToken(JSPParserEQUALS, 0)
}

func (s *HtmlAttributeContext) HtmlAttributeValue() IHtmlAttributeValueContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlAttributeValueContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlAttributeValueContext)
}

func (s *HtmlAttributeContext) Scriptlet() IScriptletContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IScriptletContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IScriptletContext)
}

func (s *HtmlAttributeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlAttributeContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlAttributeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlAttribute(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlAttribute() (localctx IHtmlAttributeContext) {
	this := p
	_ = this

	localctx = NewHtmlAttributeContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 18, JSPParserRULE_htmlAttribute)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(184)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 18, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(178)
			p.HtmlAttributeName()
		}
		{
			p.SetState(179)
			p.Match(JSPParserEQUALS)
		}
		{
			p.SetState(180)
			p.HtmlAttributeValue()
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(182)
			p.HtmlAttributeName()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(183)
			p.Scriptlet()
		}

	}

	return localctx
}

// IHtmlAttributeNameContext is an interface to support dynamic dispatch.
type IHtmlAttributeNameContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlAttributeNameContext differentiates from other interfaces.
	IsHtmlAttributeNameContext()
}

type HtmlAttributeNameContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlAttributeNameContext() *HtmlAttributeNameContext {
	var p = new(HtmlAttributeNameContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlAttributeName
	return p
}

func (*HtmlAttributeNameContext) IsHtmlAttributeNameContext() {}

func NewHtmlAttributeNameContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlAttributeNameContext {
	var p = new(HtmlAttributeNameContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlAttributeName

	return p
}

func (s *HtmlAttributeNameContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlAttributeNameContext) TAG_IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_IDENTIFIER, 0)
}

func (s *HtmlAttributeNameContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlAttributeNameContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlAttributeNameContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlAttributeName(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlAttributeName() (localctx IHtmlAttributeNameContext) {
	this := p
	_ = this

	localctx = NewHtmlAttributeNameContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 20, JSPParserRULE_htmlAttributeName)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(186)
		p.Match(JSPParserTAG_IDENTIFIER)
	}

	return localctx
}

// IHtmlAttributeValueContext is an interface to support dynamic dispatch.
type IHtmlAttributeValueContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlAttributeValueContext differentiates from other interfaces.
	IsHtmlAttributeValueContext()
}

type HtmlAttributeValueContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlAttributeValueContext() *HtmlAttributeValueContext {
	var p = new(HtmlAttributeValueContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlAttributeValue
	return p
}

func (*HtmlAttributeValueContext) IsHtmlAttributeValueContext() {}

func NewHtmlAttributeValueContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlAttributeValueContext {
	var p = new(HtmlAttributeValueContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlAttributeValue

	return p
}

func (s *HtmlAttributeValueContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlAttributeValueContext) AllQUOTE() []antlr.TerminalNode {
	return s.GetTokens(JSPParserQUOTE)
}

func (s *HtmlAttributeValueContext) QUOTE(i int) antlr.TerminalNode {
	return s.GetToken(JSPParserQUOTE, i)
}

func (s *HtmlAttributeValueContext) JspElement() IJspElementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspElementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspElementContext)
}

func (s *HtmlAttributeValueContext) HtmlAttributeValueExpr() IHtmlAttributeValueExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlAttributeValueExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlAttributeValueExprContext)
}

func (s *HtmlAttributeValueContext) HtmlAttributeValueConstant() IHtmlAttributeValueConstantContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlAttributeValueConstantContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlAttributeValueConstantContext)
}

func (s *HtmlAttributeValueContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlAttributeValueContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlAttributeValueContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlAttributeValue(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlAttributeValue() (localctx IHtmlAttributeValueContext) {
	this := p
	_ = this

	localctx = NewHtmlAttributeValueContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 22, JSPParserRULE_htmlAttributeValue)
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

	p.SetState(204)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 22, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(188)
			p.Match(JSPParserQUOTE)
		}
		{
			p.SetState(189)
			p.JspElement()
		}
		{
			p.SetState(190)
			p.Match(JSPParserQUOTE)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		p.SetState(193)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == JSPParserQUOTE {
			{
				p.SetState(192)
				p.Match(JSPParserQUOTE)
			}

		}
		{
			p.SetState(195)
			p.HtmlAttributeValueExpr()
		}
		p.SetState(197)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == JSPParserQUOTE {
			{
				p.SetState(196)
				p.Match(JSPParserQUOTE)
			}

		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(199)
			p.Match(JSPParserQUOTE)
		}
		p.SetState(201)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == JSPParserATTVAL_ATTRIBUTE {
			{
				p.SetState(200)
				p.HtmlAttributeValueConstant()
			}

		}
		{
			p.SetState(203)
			p.Match(JSPParserQUOTE)
		}

	}

	return localctx
}

// IHtmlAttributeValueExprContext is an interface to support dynamic dispatch.
type IHtmlAttributeValueExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlAttributeValueExprContext differentiates from other interfaces.
	IsHtmlAttributeValueExprContext()
}

type HtmlAttributeValueExprContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlAttributeValueExprContext() *HtmlAttributeValueExprContext {
	var p = new(HtmlAttributeValueExprContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlAttributeValueExpr
	return p
}

func (*HtmlAttributeValueExprContext) IsHtmlAttributeValueExprContext() {}

func NewHtmlAttributeValueExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlAttributeValueExprContext {
	var p = new(HtmlAttributeValueExprContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlAttributeValueExpr

	return p
}

func (s *HtmlAttributeValueExprContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlAttributeValueExprContext) EL_EXPR() antlr.TerminalNode {
	return s.GetToken(JSPParserEL_EXPR, 0)
}

func (s *HtmlAttributeValueExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlAttributeValueExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlAttributeValueExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlAttributeValueExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlAttributeValueExpr() (localctx IHtmlAttributeValueExprContext) {
	this := p
	_ = this

	localctx = NewHtmlAttributeValueExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, JSPParserRULE_htmlAttributeValueExpr)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(206)
		p.Match(JSPParserEL_EXPR)
	}

	return localctx
}

// IHtmlAttributeValueConstantContext is an interface to support dynamic dispatch.
type IHtmlAttributeValueConstantContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlAttributeValueConstantContext differentiates from other interfaces.
	IsHtmlAttributeValueConstantContext()
}

type HtmlAttributeValueConstantContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlAttributeValueConstantContext() *HtmlAttributeValueConstantContext {
	var p = new(HtmlAttributeValueConstantContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlAttributeValueConstant
	return p
}

func (*HtmlAttributeValueConstantContext) IsHtmlAttributeValueConstantContext() {}

func NewHtmlAttributeValueConstantContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlAttributeValueConstantContext {
	var p = new(HtmlAttributeValueConstantContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlAttributeValueConstant

	return p
}

func (s *HtmlAttributeValueConstantContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlAttributeValueConstantContext) ATTVAL_ATTRIBUTE() antlr.TerminalNode {
	return s.GetToken(JSPParserATTVAL_ATTRIBUTE, 0)
}

func (s *HtmlAttributeValueConstantContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlAttributeValueConstantContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlAttributeValueConstantContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlAttributeValueConstant(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlAttributeValueConstant() (localctx IHtmlAttributeValueConstantContext) {
	this := p
	_ = this

	localctx = NewHtmlAttributeValueConstantContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, JSPParserRULE_htmlAttributeValueConstant)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.Match(JSPParserATTVAL_ATTRIBUTE)
	}

	return localctx
}

// IHtmlTagNameContext is an interface to support dynamic dispatch.
type IHtmlTagNameContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlTagNameContext differentiates from other interfaces.
	IsHtmlTagNameContext()
}

type HtmlTagNameContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlTagNameContext() *HtmlTagNameContext {
	var p = new(HtmlTagNameContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlTagName
	return p
}

func (*HtmlTagNameContext) IsHtmlTagNameContext() {}

func NewHtmlTagNameContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlTagNameContext {
	var p = new(HtmlTagNameContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlTagName

	return p
}

func (s *HtmlTagNameContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlTagNameContext) TAG_IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_IDENTIFIER, 0)
}

func (s *HtmlTagNameContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlTagNameContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlTagNameContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlTagName(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlTagName() (localctx IHtmlTagNameContext) {
	this := p
	_ = this

	localctx = NewHtmlTagNameContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 28, JSPParserRULE_htmlTagName)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(210)
		p.Match(JSPParserTAG_IDENTIFIER)
	}

	return localctx
}

// IHtmlChardataContext is an interface to support dynamic dispatch.
type IHtmlChardataContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlChardataContext differentiates from other interfaces.
	IsHtmlChardataContext()
}

type HtmlChardataContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlChardataContext() *HtmlChardataContext {
	var p = new(HtmlChardataContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlChardata
	return p
}

func (*HtmlChardataContext) IsHtmlChardataContext() {}

func NewHtmlChardataContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlChardataContext {
	var p = new(HtmlChardataContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlChardata

	return p
}

func (s *HtmlChardataContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlChardataContext) JSP_STATIC_CONTENT_CHARS_MIXED() antlr.TerminalNode {
	return s.GetToken(JSPParserJSP_STATIC_CONTENT_CHARS_MIXED, 0)
}

func (s *HtmlChardataContext) JSP_STATIC_CONTENT_CHARS() antlr.TerminalNode {
	return s.GetToken(JSPParserJSP_STATIC_CONTENT_CHARS, 0)
}

func (s *HtmlChardataContext) WHITESPACES() antlr.TerminalNode {
	return s.GetToken(JSPParserWHITESPACES, 0)
}

func (s *HtmlChardataContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlChardataContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlChardataContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlChardata(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlChardata() (localctx IHtmlChardataContext) {
	this := p
	_ = this

	localctx = NewHtmlChardataContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 30, JSPParserRULE_htmlChardata)
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
		p.SetState(212)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&202375168) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IHtmlMiscContext is an interface to support dynamic dispatch.
type IHtmlMiscContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlMiscContext differentiates from other interfaces.
	IsHtmlMiscContext()
}

type HtmlMiscContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlMiscContext() *HtmlMiscContext {
	var p = new(HtmlMiscContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlMisc
	return p
}

func (*HtmlMiscContext) IsHtmlMiscContext() {}

func NewHtmlMiscContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlMiscContext {
	var p = new(HtmlMiscContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlMisc

	return p
}

func (s *HtmlMiscContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlMiscContext) HtmlComment() IHtmlCommentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlCommentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlCommentContext)
}

func (s *HtmlMiscContext) HtmlChardata() IHtmlChardataContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlChardataContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlChardataContext)
}

func (s *HtmlMiscContext) JspExpression() IJspExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspExpressionContext)
}

func (s *HtmlMiscContext) Scriptlet() IScriptletContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IScriptletContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IScriptletContext)
}

func (s *HtmlMiscContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlMiscContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlMiscContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlMisc(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlMisc() (localctx IHtmlMiscContext) {
	this := p
	_ = this

	localctx = NewHtmlMiscContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 32, JSPParserRULE_htmlMisc)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(218)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserJSP_COMMENT_START, JSPParserJSP_CONDITIONAL_COMMENT_START:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(214)
			p.HtmlComment()
		}

	case JSPParserWHITESPACES, JSPParserJSP_STATIC_CONTENT_CHARS_MIXED, JSPParserJSP_STATIC_CONTENT_CHARS:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(215)
			p.HtmlChardata()
		}

	case JSPParserEL_EXPR:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(216)
			p.JspExpression()
		}

	case JSPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(217)
			p.Scriptlet()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IHtmlCommentContext is an interface to support dynamic dispatch.
type IHtmlCommentContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlCommentContext differentiates from other interfaces.
	IsHtmlCommentContext()
}

type HtmlCommentContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlCommentContext() *HtmlCommentContext {
	var p = new(HtmlCommentContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlComment
	return p
}

func (*HtmlCommentContext) IsHtmlCommentContext() {}

func NewHtmlCommentContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlCommentContext {
	var p = new(HtmlCommentContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlComment

	return p
}

func (s *HtmlCommentContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlCommentContext) JSP_COMMENT_START() antlr.TerminalNode {
	return s.GetToken(JSPParserJSP_COMMENT_START, 0)
}

func (s *HtmlCommentContext) JSP_COMMENT_END() antlr.TerminalNode {
	return s.GetToken(JSPParserJSP_COMMENT_END, 0)
}

func (s *HtmlCommentContext) HtmlCommentText() IHtmlCommentTextContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlCommentTextContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlCommentTextContext)
}

func (s *HtmlCommentContext) JSP_CONDITIONAL_COMMENT_START() antlr.TerminalNode {
	return s.GetToken(JSPParserJSP_CONDITIONAL_COMMENT_START, 0)
}

func (s *HtmlCommentContext) JSP_CONDITIONAL_COMMENT_END() antlr.TerminalNode {
	return s.GetToken(JSPParserJSP_CONDITIONAL_COMMENT_END, 0)
}

func (s *HtmlCommentContext) HtmlConditionalCommentText() IHtmlConditionalCommentTextContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlConditionalCommentTextContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlConditionalCommentTextContext)
}

func (s *HtmlCommentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlCommentContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlCommentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlComment(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlComment() (localctx IHtmlCommentContext) {
	this := p
	_ = this

	localctx = NewHtmlCommentContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 34, JSPParserRULE_htmlComment)
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

	p.SetState(230)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserJSP_COMMENT_START:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(220)
			p.Match(JSPParserJSP_COMMENT_START)
		}
		p.SetState(222)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == JSPParserJSP_COMMENT_TEXT {
			{
				p.SetState(221)
				p.HtmlCommentText()
			}

		}
		{
			p.SetState(224)
			p.Match(JSPParserJSP_COMMENT_END)
		}

	case JSPParserJSP_CONDITIONAL_COMMENT_START:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(225)
			p.Match(JSPParserJSP_CONDITIONAL_COMMENT_START)
		}
		p.SetState(227)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == JSPParserJSP_CONDITIONAL_COMMENT {
			{
				p.SetState(226)
				p.HtmlConditionalCommentText()
			}

		}
		{
			p.SetState(229)
			p.Match(JSPParserJSP_CONDITIONAL_COMMENT_END)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IHtmlCommentTextContext is an interface to support dynamic dispatch.
type IHtmlCommentTextContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlCommentTextContext differentiates from other interfaces.
	IsHtmlCommentTextContext()
}

type HtmlCommentTextContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlCommentTextContext() *HtmlCommentTextContext {
	var p = new(HtmlCommentTextContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlCommentText
	return p
}

func (*HtmlCommentTextContext) IsHtmlCommentTextContext() {}

func NewHtmlCommentTextContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlCommentTextContext {
	var p = new(HtmlCommentTextContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlCommentText

	return p
}

func (s *HtmlCommentTextContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlCommentTextContext) AllJSP_COMMENT_TEXT() []antlr.TerminalNode {
	return s.GetTokens(JSPParserJSP_COMMENT_TEXT)
}

func (s *HtmlCommentTextContext) JSP_COMMENT_TEXT(i int) antlr.TerminalNode {
	return s.GetToken(JSPParserJSP_COMMENT_TEXT, i)
}

func (s *HtmlCommentTextContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlCommentTextContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlCommentTextContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlCommentText(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlCommentText() (localctx IHtmlCommentTextContext) {
	this := p
	_ = this

	localctx = NewHtmlCommentTextContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 36, JSPParserRULE_htmlCommentText)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	p.SetState(233)
	p.GetErrorHandler().Sync(p)
	_alt = 1 + 1
	for ok := true; ok; ok = _alt != 1 && _alt != antlr.ATNInvalidAltNumber {
		switch _alt {
		case 1 + 1:
			{
				p.SetState(232)
				p.Match(JSPParserJSP_COMMENT_TEXT)
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

		p.SetState(235)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 27, p.GetParserRuleContext())
	}

	return localctx
}

// IHtmlConditionalCommentTextContext is an interface to support dynamic dispatch.
type IHtmlConditionalCommentTextContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlConditionalCommentTextContext differentiates from other interfaces.
	IsHtmlConditionalCommentTextContext()
}

type HtmlConditionalCommentTextContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlConditionalCommentTextContext() *HtmlConditionalCommentTextContext {
	var p = new(HtmlConditionalCommentTextContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlConditionalCommentText
	return p
}

func (*HtmlConditionalCommentTextContext) IsHtmlConditionalCommentTextContext() {}

func NewHtmlConditionalCommentTextContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlConditionalCommentTextContext {
	var p = new(HtmlConditionalCommentTextContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlConditionalCommentText

	return p
}

func (s *HtmlConditionalCommentTextContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlConditionalCommentTextContext) JSP_CONDITIONAL_COMMENT() antlr.TerminalNode {
	return s.GetToken(JSPParserJSP_CONDITIONAL_COMMENT, 0)
}

func (s *HtmlConditionalCommentTextContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlConditionalCommentTextContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlConditionalCommentTextContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlConditionalCommentText(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlConditionalCommentText() (localctx IHtmlConditionalCommentTextContext) {
	this := p
	_ = this

	localctx = NewHtmlConditionalCommentTextContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 38, JSPParserRULE_htmlConditionalCommentText)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(237)
		p.Match(JSPParserJSP_CONDITIONAL_COMMENT)
	}

	return localctx
}

// IXhtmlCDATAContext is an interface to support dynamic dispatch.
type IXhtmlCDATAContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsXhtmlCDATAContext differentiates from other interfaces.
	IsXhtmlCDATAContext()
}

type XhtmlCDATAContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyXhtmlCDATAContext() *XhtmlCDATAContext {
	var p = new(XhtmlCDATAContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_xhtmlCDATA
	return p
}

func (*XhtmlCDATAContext) IsXhtmlCDATAContext() {}

func NewXhtmlCDATAContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *XhtmlCDATAContext {
	var p = new(XhtmlCDATAContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_xhtmlCDATA

	return p
}

func (s *XhtmlCDATAContext) GetParser() antlr.Parser { return s.parser }

func (s *XhtmlCDATAContext) CDATA() antlr.TerminalNode {
	return s.GetToken(JSPParserCDATA, 0)
}

func (s *XhtmlCDATAContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *XhtmlCDATAContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *XhtmlCDATAContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitXhtmlCDATA(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) XhtmlCDATA() (localctx IXhtmlCDATAContext) {
	this := p
	_ = this

	localctx = NewXhtmlCDATAContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 40, JSPParserRULE_xhtmlCDATA)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.Match(JSPParserCDATA)
	}

	return localctx
}

// IDtdContext is an interface to support dynamic dispatch.
type IDtdContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDtdContext differentiates from other interfaces.
	IsDtdContext()
}

type DtdContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDtdContext() *DtdContext {
	var p = new(DtdContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_dtd
	return p
}

func (*DtdContext) IsDtdContext() {}

func NewDtdContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DtdContext {
	var p = new(DtdContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_dtd

	return p
}

func (s *DtdContext) GetParser() antlr.Parser { return s.parser }

func (s *DtdContext) DTD() antlr.TerminalNode {
	return s.GetToken(JSPParserDTD, 0)
}

func (s *DtdContext) DtdElementName() IDtdElementNameContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDtdElementNameContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDtdElementNameContext)
}

func (s *DtdContext) TAG_END() antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_END, 0)
}

func (s *DtdContext) DTD_PUBLIC() antlr.TerminalNode {
	return s.GetToken(JSPParserDTD_PUBLIC, 0)
}

func (s *DtdContext) PublicId() IPublicIdContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IPublicIdContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IPublicIdContext)
}

func (s *DtdContext) DTD_SYSTEM() antlr.TerminalNode {
	return s.GetToken(JSPParserDTD_SYSTEM, 0)
}

func (s *DtdContext) SystemId() ISystemIdContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISystemIdContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISystemIdContext)
}

func (s *DtdContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DtdContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DtdContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitDtd(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) Dtd() (localctx IDtdContext) {
	this := p
	_ = this

	localctx = NewDtdContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 42, JSPParserRULE_dtd)
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
		p.Match(JSPParserDTD)
	}
	{
		p.SetState(242)
		p.DtdElementName()
	}
	p.SetState(245)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == JSPParserDTD_PUBLIC {
		{
			p.SetState(243)
			p.Match(JSPParserDTD_PUBLIC)
		}
		{
			p.SetState(244)
			p.PublicId()
		}

	}
	p.SetState(249)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == JSPParserDTD_SYSTEM {
		{
			p.SetState(247)
			p.Match(JSPParserDTD_SYSTEM)
		}
		{
			p.SetState(248)
			p.SystemId()
		}

	}
	{
		p.SetState(251)
		p.Match(JSPParserTAG_END)
	}

	return localctx
}

// IDtdElementNameContext is an interface to support dynamic dispatch.
type IDtdElementNameContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDtdElementNameContext differentiates from other interfaces.
	IsDtdElementNameContext()
}

type DtdElementNameContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDtdElementNameContext() *DtdElementNameContext {
	var p = new(DtdElementNameContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_dtdElementName
	return p
}

func (*DtdElementNameContext) IsDtdElementNameContext() {}

func NewDtdElementNameContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DtdElementNameContext {
	var p = new(DtdElementNameContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_dtdElementName

	return p
}

func (s *DtdElementNameContext) GetParser() antlr.Parser { return s.parser }

func (s *DtdElementNameContext) DTD_IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(JSPParserDTD_IDENTIFIER, 0)
}

func (s *DtdElementNameContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DtdElementNameContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DtdElementNameContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitDtdElementName(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) DtdElementName() (localctx IDtdElementNameContext) {
	this := p
	_ = this

	localctx = NewDtdElementNameContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 44, JSPParserRULE_dtdElementName)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(253)
		p.Match(JSPParserDTD_IDENTIFIER)
	}

	return localctx
}

// IPublicIdContext is an interface to support dynamic dispatch.
type IPublicIdContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsPublicIdContext differentiates from other interfaces.
	IsPublicIdContext()
}

type PublicIdContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyPublicIdContext() *PublicIdContext {
	var p = new(PublicIdContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_publicId
	return p
}

func (*PublicIdContext) IsPublicIdContext() {}

func NewPublicIdContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *PublicIdContext {
	var p = new(PublicIdContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_publicId

	return p
}

func (s *PublicIdContext) GetParser() antlr.Parser { return s.parser }

func (s *PublicIdContext) DTD_QUOTED() antlr.TerminalNode {
	return s.GetToken(JSPParserDTD_QUOTED, 0)
}

func (s *PublicIdContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PublicIdContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *PublicIdContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitPublicId(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) PublicId() (localctx IPublicIdContext) {
	this := p
	_ = this

	localctx = NewPublicIdContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 46, JSPParserRULE_publicId)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(255)
		p.Match(JSPParserDTD_QUOTED)
	}

	return localctx
}

// ISystemIdContext is an interface to support dynamic dispatch.
type ISystemIdContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsSystemIdContext differentiates from other interfaces.
	IsSystemIdContext()
}

type SystemIdContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySystemIdContext() *SystemIdContext {
	var p = new(SystemIdContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_systemId
	return p
}

func (*SystemIdContext) IsSystemIdContext() {}

func NewSystemIdContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SystemIdContext {
	var p = new(SystemIdContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_systemId

	return p
}

func (s *SystemIdContext) GetParser() antlr.Parser { return s.parser }

func (s *SystemIdContext) DTD_QUOTED() antlr.TerminalNode {
	return s.GetToken(JSPParserDTD_QUOTED, 0)
}

func (s *SystemIdContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SystemIdContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *SystemIdContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitSystemId(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) SystemId() (localctx ISystemIdContext) {
	this := p
	_ = this

	localctx = NewSystemIdContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 48, JSPParserRULE_systemId)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(257)
		p.Match(JSPParserDTD_QUOTED)
	}

	return localctx
}

// IXmlContext is an interface to support dynamic dispatch.
type IXmlContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// GetName returns the name rule contexts.
	GetName() IHtmlTagNameContext

	// Get_htmlAttribute returns the _htmlAttribute rule contexts.
	Get_htmlAttribute() IHtmlAttributeContext

	// SetName sets the name rule contexts.
	SetName(IHtmlTagNameContext)

	// Set_htmlAttribute sets the _htmlAttribute rule contexts.
	Set_htmlAttribute(IHtmlAttributeContext)

	// GetAtts returns the atts rule context list.
	GetAtts() []IHtmlAttributeContext

	// SetAtts sets the atts rule context list.
	SetAtts([]IHtmlAttributeContext)

	// IsXmlContext differentiates from other interfaces.
	IsXmlContext()
}

type XmlContext struct {
	*antlr.BaseParserRuleContext
	parser         antlr.Parser
	name           IHtmlTagNameContext
	_htmlAttribute IHtmlAttributeContext
	atts           []IHtmlAttributeContext
}

func NewEmptyXmlContext() *XmlContext {
	var p = new(XmlContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_xml
	return p
}

func (*XmlContext) IsXmlContext() {}

func NewXmlContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *XmlContext {
	var p = new(XmlContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_xml

	return p
}

func (s *XmlContext) GetParser() antlr.Parser { return s.parser }

func (s *XmlContext) GetName() IHtmlTagNameContext { return s.name }

func (s *XmlContext) Get_htmlAttribute() IHtmlAttributeContext { return s._htmlAttribute }

func (s *XmlContext) SetName(v IHtmlTagNameContext) { s.name = v }

func (s *XmlContext) Set_htmlAttribute(v IHtmlAttributeContext) { s._htmlAttribute = v }

func (s *XmlContext) GetAtts() []IHtmlAttributeContext { return s.atts }

func (s *XmlContext) SetAtts(v []IHtmlAttributeContext) { s.atts = v }

func (s *XmlContext) XML_DECLARATION() antlr.TerminalNode {
	return s.GetToken(JSPParserXML_DECLARATION, 0)
}

func (s *XmlContext) TAG_END() antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_END, 0)
}

func (s *XmlContext) HtmlTagName() IHtmlTagNameContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlTagNameContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlTagNameContext)
}

func (s *XmlContext) AllHtmlAttribute() []IHtmlAttributeContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlAttributeContext); ok {
			len++
		}
	}

	tst := make([]IHtmlAttributeContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlAttributeContext); ok {
			tst[i] = t.(IHtmlAttributeContext)
			i++
		}
	}

	return tst
}

func (s *XmlContext) HtmlAttribute(i int) IHtmlAttributeContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlAttributeContext); ok {
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

	return t.(IHtmlAttributeContext)
}

func (s *XmlContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *XmlContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *XmlContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitXml(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) Xml() (localctx IXmlContext) {
	this := p
	_ = this

	localctx = NewXmlContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 50, JSPParserRULE_xml)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(259)
		p.Match(JSPParserXML_DECLARATION)
	}
	{
		p.SetState(260)

		var _x = p.HtmlTagName()

		localctx.(*XmlContext).name = _x
	}
	p.SetState(264)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 30, p.GetParserRuleContext())

	for _alt != 1 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1+1 {
			{
				p.SetState(261)

				var _x = p.HtmlAttribute()

				localctx.(*XmlContext)._htmlAttribute = _x
			}
			localctx.(*XmlContext).atts = append(localctx.(*XmlContext).atts, localctx.(*XmlContext)._htmlAttribute)

		}
		p.SetState(266)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 30, p.GetParserRuleContext())
	}
	{
		p.SetState(267)
		p.Match(JSPParserTAG_END)
	}

	return localctx
}

// IScriptletContext is an interface to support dynamic dispatch.
type IScriptletContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsScriptletContext differentiates from other interfaces.
	IsScriptletContext()
}

type ScriptletContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyScriptletContext() *ScriptletContext {
	var p = new(ScriptletContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_scriptlet
	return p
}

func (*ScriptletContext) IsScriptletContext() {}

func NewScriptletContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ScriptletContext {
	var p = new(ScriptletContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_scriptlet

	return p
}

func (s *ScriptletContext) GetParser() antlr.Parser { return s.parser }

func (s *ScriptletContext) SCRIPTLET_OPEN() antlr.TerminalNode {
	return s.GetToken(JSPParserSCRIPTLET_OPEN, 0)
}

func (s *ScriptletContext) BLOB_CONTENT() antlr.TerminalNode {
	return s.GetToken(JSPParserBLOB_CONTENT, 0)
}

func (s *ScriptletContext) JSP_END() antlr.TerminalNode {
	return s.GetToken(JSPParserJSP_END, 0)
}

func (s *ScriptletContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ScriptletContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ScriptletContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitScriptlet(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) Scriptlet() (localctx IScriptletContext) {
	this := p
	_ = this

	localctx = NewScriptletContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 52, JSPParserRULE_scriptlet)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(269)
		p.Match(JSPParserSCRIPTLET_OPEN)
	}
	{
		p.SetState(270)
		p.Match(JSPParserBLOB_CONTENT)
	}
	{
		p.SetState(271)
		p.Match(JSPParserJSP_END)
	}

	return localctx
}
