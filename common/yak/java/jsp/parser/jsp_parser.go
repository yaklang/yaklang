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
		"", "", "", "'<!--'", "", "'-->'", "", "'<!['", "']>'", "'<?xml'", "",
		"", "'<!DOCTYPE'", "", "", "", "", "", "", "", "", "", "'\"'", "'''",
		"", "", "", "", "", "", "'%>'", "", "", "", "'PUBLIC'", "'SYSTEM'",
		"", "", "", "", "", "", "':'", "", "", "'/'",
	}
	staticData.symbolicNames = []string{
		"", "JSP_COMMENT_START", "JSP_COMMENT_END", "JSP_COMMENT_START_TAG",
		"HTML_TEXT", "JSP_COMMENT_END_TAG", "JSP_CONDITIONAL_COMMENT_START",
		"JSP_CONDITIONAL_COMMENT_START_TAG", "JSP_CONDITIONAL_COMMENT_END_TAG",
		"XML_DECLARATION", "CDATA", "DTD", "DTD_START", "WHITESPACE_SKIP", "CLOSE_TAG_BEGIN",
		"TAG_BEGIN", "DIRECTIVE_BEGIN", "DECLARATION_BEGIN", "ECHO_EXPRESSION_OPEN",
		"SCRIPTLET_OPEN", "EXPRESSION_OPEN", "WHITESPACES", "DOUBLE_QUOTE",
		"SINGLE_QUOTE", "QUOTE", "TAG_END", "EQUALS", "JSP_STATIC_CONTENT_CHARS_MIXED",
		"JSP_STATIC_CONTENT_CHARS", "JSP_STATIC_CONTENT_CHAR", "JSP_END", "JSP_CONDITIONAL_COMMENT_END",
		"JSP_CONDITIONAL_COMMENT", "JSP_COMMENT_TEXT", "DTD_PUBLIC", "DTD_SYSTEM",
		"DTD_WHITESPACE_SKIP", "DTD_QUOTED", "DTD_IDENTIFIER", "BLOB_CLOSE",
		"BLOB_CONTENT", "JSPEXPR_CONTENT_CLOSE", "JSP_JSTL_COLON", "TAG_SLASH_END",
		"TAG_CLOSE", "TAG_SLASH", "DIRECTIVE_END", "TAG_IDENTIFIER", "TAG_WHITESPACE",
		"SCRIPT_BODY", "SCRIPT_SHORT_BODY", "STYLE_BODY", "STYLE_SHORT_BODY",
		"ATTVAL_ATTRIBUTE", "EL_EXPR",
	}
	staticData.ruleNames = []string{
		"jspDocuments", "jspDocument", "jspStart", "jspElements", "jspElement",
		"htmlBegin", "htmlTag", "jspDirective", "htmlContents", "htmlContent",
		"jspExpression", "htmlAttribute", "htmlAttributeName", "htmlAttributeValue",
		"htmlAttributeValueExpr", "htmlAttributeValueConstant", "htmlTagName",
		"htmlChardata", "htmlMisc", "htmlComment", "htmlCommentText", "htmlConditionalCommentText",
		"xhtmlCDATA", "dtd", "dtdElementName", "publicId", "systemId", "xml",
		"scriptlet",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 54, 278, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7, 20, 2,
		21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25, 2, 26,
		7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 1, 0, 4, 0, 60, 8, 0, 11, 0, 12, 0,
		61, 1, 1, 5, 1, 65, 8, 1, 10, 1, 12, 1, 68, 9, 1, 1, 1, 1, 1, 5, 1, 72,
		8, 1, 10, 1, 12, 1, 75, 9, 1, 1, 1, 1, 1, 5, 1, 79, 8, 1, 10, 1, 12, 1,
		82, 9, 1, 1, 1, 4, 1, 85, 8, 1, 11, 1, 12, 1, 86, 3, 1, 89, 8, 1, 1, 2,
		1, 2, 1, 2, 3, 2, 94, 8, 2, 1, 3, 5, 3, 97, 8, 3, 10, 3, 12, 3, 100, 9,
		3, 1, 3, 1, 3, 1, 3, 3, 3, 105, 8, 3, 1, 3, 5, 3, 108, 8, 3, 10, 3, 12,
		3, 111, 9, 3, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 3, 4, 120, 8, 4,
		1, 4, 3, 4, 123, 8, 4, 1, 5, 1, 5, 1, 5, 5, 5, 128, 8, 5, 10, 5, 12, 5,
		131, 9, 5, 1, 6, 1, 6, 1, 6, 3, 6, 136, 8, 6, 1, 7, 1, 7, 1, 7, 5, 7, 141,
		8, 7, 10, 7, 12, 7, 144, 9, 7, 1, 7, 5, 7, 147, 8, 7, 10, 7, 12, 7, 150,
		9, 7, 1, 7, 1, 7, 1, 8, 3, 8, 155, 8, 8, 1, 8, 1, 8, 3, 8, 159, 8, 8, 5,
		8, 161, 8, 8, 10, 8, 12, 8, 164, 9, 8, 1, 9, 1, 9, 1, 9, 1, 9, 1, 9, 1,
		9, 1, 9, 3, 9, 173, 8, 9, 1, 10, 1, 10, 1, 11, 1, 11, 1, 11, 1, 11, 1,
		11, 1, 11, 3, 11, 183, 8, 11, 1, 12, 1, 12, 1, 13, 1, 13, 1, 13, 1, 13,
		1, 13, 3, 13, 192, 8, 13, 1, 13, 1, 13, 3, 13, 196, 8, 13, 1, 13, 1, 13,
		3, 13, 200, 8, 13, 1, 13, 3, 13, 203, 8, 13, 1, 14, 1, 14, 1, 15, 1, 15,
		1, 16, 1, 16, 1, 17, 1, 17, 1, 18, 1, 18, 1, 18, 3, 18, 216, 8, 18, 1,
		19, 1, 19, 3, 19, 220, 8, 19, 1, 19, 1, 19, 1, 19, 3, 19, 225, 8, 19, 1,
		19, 3, 19, 228, 8, 19, 1, 20, 4, 20, 231, 8, 20, 11, 20, 12, 20, 232, 1,
		21, 1, 21, 1, 22, 1, 22, 1, 23, 1, 23, 1, 23, 1, 23, 3, 23, 243, 8, 23,
		1, 23, 1, 23, 3, 23, 247, 8, 23, 1, 23, 1, 23, 1, 24, 1, 24, 1, 25, 1,
		25, 1, 26, 1, 26, 1, 27, 1, 27, 1, 27, 5, 27, 260, 8, 27, 10, 27, 12, 27,
		263, 9, 27, 1, 27, 1, 27, 1, 28, 1, 28, 1, 28, 1, 28, 1, 28, 1, 28, 1,
		28, 1, 28, 1, 28, 3, 28, 276, 8, 28, 1, 28, 3, 142, 232, 261, 0, 29, 0,
		2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30, 32, 34, 36, 38,
		40, 42, 44, 46, 48, 50, 52, 54, 56, 0, 1, 3, 0, 4, 4, 21, 21, 27, 28, 294,
		0, 59, 1, 0, 0, 0, 2, 88, 1, 0, 0, 0, 4, 93, 1, 0, 0, 0, 6, 98, 1, 0, 0,
		0, 8, 112, 1, 0, 0, 0, 10, 124, 1, 0, 0, 0, 12, 132, 1, 0, 0, 0, 14, 137,
		1, 0, 0, 0, 16, 154, 1, 0, 0, 0, 18, 172, 1, 0, 0, 0, 20, 174, 1, 0, 0,
		0, 22, 182, 1, 0, 0, 0, 24, 184, 1, 0, 0, 0, 26, 202, 1, 0, 0, 0, 28, 204,
		1, 0, 0, 0, 30, 206, 1, 0, 0, 0, 32, 208, 1, 0, 0, 0, 34, 210, 1, 0, 0,
		0, 36, 215, 1, 0, 0, 0, 38, 227, 1, 0, 0, 0, 40, 230, 1, 0, 0, 0, 42, 234,
		1, 0, 0, 0, 44, 236, 1, 0, 0, 0, 46, 238, 1, 0, 0, 0, 48, 250, 1, 0, 0,
		0, 50, 252, 1, 0, 0, 0, 52, 254, 1, 0, 0, 0, 54, 256, 1, 0, 0, 0, 56, 275,
		1, 0, 0, 0, 58, 60, 3, 2, 1, 0, 59, 58, 1, 0, 0, 0, 60, 61, 1, 0, 0, 0,
		61, 59, 1, 0, 0, 0, 61, 62, 1, 0, 0, 0, 62, 1, 1, 0, 0, 0, 63, 65, 3, 4,
		2, 0, 64, 63, 1, 0, 0, 0, 65, 68, 1, 0, 0, 0, 66, 64, 1, 0, 0, 0, 66, 67,
		1, 0, 0, 0, 67, 69, 1, 0, 0, 0, 68, 66, 1, 0, 0, 0, 69, 89, 3, 54, 27,
		0, 70, 72, 3, 4, 2, 0, 71, 70, 1, 0, 0, 0, 72, 75, 1, 0, 0, 0, 73, 71,
		1, 0, 0, 0, 73, 74, 1, 0, 0, 0, 74, 76, 1, 0, 0, 0, 75, 73, 1, 0, 0, 0,
		76, 89, 3, 46, 23, 0, 77, 79, 3, 4, 2, 0, 78, 77, 1, 0, 0, 0, 79, 82, 1,
		0, 0, 0, 80, 78, 1, 0, 0, 0, 80, 81, 1, 0, 0, 0, 81, 84, 1, 0, 0, 0, 82,
		80, 1, 0, 0, 0, 83, 85, 3, 6, 3, 0, 84, 83, 1, 0, 0, 0, 85, 86, 1, 0, 0,
		0, 86, 84, 1, 0, 0, 0, 86, 87, 1, 0, 0, 0, 87, 89, 1, 0, 0, 0, 88, 66,
		1, 0, 0, 0, 88, 73, 1, 0, 0, 0, 88, 80, 1, 0, 0, 0, 89, 3, 1, 0, 0, 0,
		90, 94, 3, 14, 7, 0, 91, 94, 3, 56, 28, 0, 92, 94, 5, 21, 0, 0, 93, 90,
		1, 0, 0, 0, 93, 91, 1, 0, 0, 0, 93, 92, 1, 0, 0, 0, 94, 5, 1, 0, 0, 0,
		95, 97, 3, 36, 18, 0, 96, 95, 1, 0, 0, 0, 97, 100, 1, 0, 0, 0, 98, 96,
		1, 0, 0, 0, 98, 99, 1, 0, 0, 0, 99, 104, 1, 0, 0, 0, 100, 98, 1, 0, 0,
		0, 101, 105, 3, 8, 4, 0, 102, 105, 3, 14, 7, 0, 103, 105, 3, 56, 28, 0,
		104, 101, 1, 0, 0, 0, 104, 102, 1, 0, 0, 0, 104, 103, 1, 0, 0, 0, 105,
		109, 1, 0, 0, 0, 106, 108, 3, 36, 18, 0, 107, 106, 1, 0, 0, 0, 108, 111,
		1, 0, 0, 0, 109, 107, 1, 0, 0, 0, 109, 110, 1, 0, 0, 0, 110, 7, 1, 0, 0,
		0, 111, 109, 1, 0, 0, 0, 112, 122, 3, 10, 5, 0, 113, 119, 5, 44, 0, 0,
		114, 115, 3, 18, 9, 0, 115, 116, 5, 14, 0, 0, 116, 117, 3, 12, 6, 0, 117,
		118, 5, 44, 0, 0, 118, 120, 1, 0, 0, 0, 119, 114, 1, 0, 0, 0, 119, 120,
		1, 0, 0, 0, 120, 123, 1, 0, 0, 0, 121, 123, 5, 43, 0, 0, 122, 113, 1, 0,
		0, 0, 122, 121, 1, 0, 0, 0, 123, 9, 1, 0, 0, 0, 124, 125, 5, 15, 0, 0,
		125, 129, 3, 12, 6, 0, 126, 128, 3, 22, 11, 0, 127, 126, 1, 0, 0, 0, 128,
		131, 1, 0, 0, 0, 129, 127, 1, 0, 0, 0, 129, 130, 1, 0, 0, 0, 130, 11, 1,
		0, 0, 0, 131, 129, 1, 0, 0, 0, 132, 135, 3, 32, 16, 0, 133, 134, 5, 42,
		0, 0, 134, 136, 3, 32, 16, 0, 135, 133, 1, 0, 0, 0, 135, 136, 1, 0, 0,
		0, 136, 13, 1, 0, 0, 0, 137, 138, 5, 16, 0, 0, 138, 142, 3, 32, 16, 0,
		139, 141, 3, 22, 11, 0, 140, 139, 1, 0, 0, 0, 141, 144, 1, 0, 0, 0, 142,
		143, 1, 0, 0, 0, 142, 140, 1, 0, 0, 0, 143, 148, 1, 0, 0, 0, 144, 142,
		1, 0, 0, 0, 145, 147, 5, 48, 0, 0, 146, 145, 1, 0, 0, 0, 147, 150, 1, 0,
		0, 0, 148, 146, 1, 0, 0, 0, 148, 149, 1, 0, 0, 0, 149, 151, 1, 0, 0, 0,
		150, 148, 1, 0, 0, 0, 151, 152, 5, 46, 0, 0, 152, 15, 1, 0, 0, 0, 153,
		155, 3, 34, 17, 0, 154, 153, 1, 0, 0, 0, 154, 155, 1, 0, 0, 0, 155, 162,
		1, 0, 0, 0, 156, 158, 3, 18, 9, 0, 157, 159, 3, 34, 17, 0, 158, 157, 1,
		0, 0, 0, 158, 159, 1, 0, 0, 0, 159, 161, 1, 0, 0, 0, 160, 156, 1, 0, 0,
		0, 161, 164, 1, 0, 0, 0, 162, 160, 1, 0, 0, 0, 162, 163, 1, 0, 0, 0, 163,
		17, 1, 0, 0, 0, 164, 162, 1, 0, 0, 0, 165, 173, 3, 34, 17, 0, 166, 173,
		3, 20, 10, 0, 167, 173, 3, 8, 4, 0, 168, 173, 3, 44, 22, 0, 169, 173, 3,
		38, 19, 0, 170, 173, 3, 56, 28, 0, 171, 173, 3, 14, 7, 0, 172, 165, 1,
		0, 0, 0, 172, 166, 1, 0, 0, 0, 172, 167, 1, 0, 0, 0, 172, 168, 1, 0, 0,
		0, 172, 169, 1, 0, 0, 0, 172, 170, 1, 0, 0, 0, 172, 171, 1, 0, 0, 0, 173,
		19, 1, 0, 0, 0, 174, 175, 5, 54, 0, 0, 175, 21, 1, 0, 0, 0, 176, 177, 3,
		24, 12, 0, 177, 178, 5, 26, 0, 0, 178, 179, 3, 26, 13, 0, 179, 183, 1,
		0, 0, 0, 180, 183, 3, 24, 12, 0, 181, 183, 3, 56, 28, 0, 182, 176, 1, 0,
		0, 0, 182, 180, 1, 0, 0, 0, 182, 181, 1, 0, 0, 0, 183, 23, 1, 0, 0, 0,
		184, 185, 5, 47, 0, 0, 185, 25, 1, 0, 0, 0, 186, 187, 5, 24, 0, 0, 187,
		188, 3, 8, 4, 0, 188, 189, 5, 24, 0, 0, 189, 203, 1, 0, 0, 0, 190, 192,
		5, 24, 0, 0, 191, 190, 1, 0, 0, 0, 191, 192, 1, 0, 0, 0, 192, 193, 1, 0,
		0, 0, 193, 195, 3, 28, 14, 0, 194, 196, 5, 24, 0, 0, 195, 194, 1, 0, 0,
		0, 195, 196, 1, 0, 0, 0, 196, 203, 1, 0, 0, 0, 197, 199, 5, 24, 0, 0, 198,
		200, 3, 30, 15, 0, 199, 198, 1, 0, 0, 0, 199, 200, 1, 0, 0, 0, 200, 201,
		1, 0, 0, 0, 201, 203, 5, 24, 0, 0, 202, 186, 1, 0, 0, 0, 202, 191, 1, 0,
		0, 0, 202, 197, 1, 0, 0, 0, 203, 27, 1, 0, 0, 0, 204, 205, 5, 54, 0, 0,
		205, 29, 1, 0, 0, 0, 206, 207, 5, 53, 0, 0, 207, 31, 1, 0, 0, 0, 208, 209,
		5, 47, 0, 0, 209, 33, 1, 0, 0, 0, 210, 211, 7, 0, 0, 0, 211, 35, 1, 0,
		0, 0, 212, 216, 3, 38, 19, 0, 213, 216, 3, 20, 10, 0, 214, 216, 3, 56,
		28, 0, 215, 212, 1, 0, 0, 0, 215, 213, 1, 0, 0, 0, 215, 214, 1, 0, 0, 0,
		216, 37, 1, 0, 0, 0, 217, 219, 5, 1, 0, 0, 218, 220, 3, 40, 20, 0, 219,
		218, 1, 0, 0, 0, 219, 220, 1, 0, 0, 0, 220, 221, 1, 0, 0, 0, 221, 228,
		5, 2, 0, 0, 222, 224, 5, 6, 0, 0, 223, 225, 3, 42, 21, 0, 224, 223, 1,
		0, 0, 0, 224, 225, 1, 0, 0, 0, 225, 226, 1, 0, 0, 0, 226, 228, 5, 31, 0,
		0, 227, 217, 1, 0, 0, 0, 227, 222, 1, 0, 0, 0, 228, 39, 1, 0, 0, 0, 229,
		231, 5, 33, 0, 0, 230, 229, 1, 0, 0, 0, 231, 232, 1, 0, 0, 0, 232, 233,
		1, 0, 0, 0, 232, 230, 1, 0, 0, 0, 233, 41, 1, 0, 0, 0, 234, 235, 5, 32,
		0, 0, 235, 43, 1, 0, 0, 0, 236, 237, 5, 10, 0, 0, 237, 45, 1, 0, 0, 0,
		238, 239, 5, 11, 0, 0, 239, 242, 3, 48, 24, 0, 240, 241, 5, 34, 0, 0, 241,
		243, 3, 50, 25, 0, 242, 240, 1, 0, 0, 0, 242, 243, 1, 0, 0, 0, 243, 246,
		1, 0, 0, 0, 244, 245, 5, 35, 0, 0, 245, 247, 3, 52, 26, 0, 246, 244, 1,
		0, 0, 0, 246, 247, 1, 0, 0, 0, 247, 248, 1, 0, 0, 0, 248, 249, 5, 25, 0,
		0, 249, 47, 1, 0, 0, 0, 250, 251, 5, 38, 0, 0, 251, 49, 1, 0, 0, 0, 252,
		253, 5, 37, 0, 0, 253, 51, 1, 0, 0, 0, 254, 255, 5, 37, 0, 0, 255, 53,
		1, 0, 0, 0, 256, 257, 5, 9, 0, 0, 257, 261, 3, 32, 16, 0, 258, 260, 3,
		22, 11, 0, 259, 258, 1, 0, 0, 0, 260, 263, 1, 0, 0, 0, 261, 262, 1, 0,
		0, 0, 261, 259, 1, 0, 0, 0, 262, 264, 1, 0, 0, 0, 263, 261, 1, 0, 0, 0,
		264, 265, 5, 25, 0, 0, 265, 55, 1, 0, 0, 0, 266, 267, 5, 19, 0, 0, 267,
		268, 5, 40, 0, 0, 268, 276, 5, 39, 0, 0, 269, 270, 5, 18, 0, 0, 270, 271,
		5, 40, 0, 0, 271, 276, 5, 39, 0, 0, 272, 273, 5, 17, 0, 0, 273, 274, 5,
		40, 0, 0, 274, 276, 5, 39, 0, 0, 275, 266, 1, 0, 0, 0, 275, 269, 1, 0,
		0, 0, 275, 272, 1, 0, 0, 0, 276, 57, 1, 0, 0, 0, 34, 61, 66, 73, 80, 86,
		88, 93, 98, 104, 109, 119, 122, 129, 135, 142, 148, 154, 158, 162, 172,
		182, 191, 195, 199, 202, 215, 219, 224, 227, 232, 242, 246, 261, 275,
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
	JSPParserHTML_TEXT                         = 4
	JSPParserJSP_COMMENT_END_TAG               = 5
	JSPParserJSP_CONDITIONAL_COMMENT_START     = 6
	JSPParserJSP_CONDITIONAL_COMMENT_START_TAG = 7
	JSPParserJSP_CONDITIONAL_COMMENT_END_TAG   = 8
	JSPParserXML_DECLARATION                   = 9
	JSPParserCDATA                             = 10
	JSPParserDTD                               = 11
	JSPParserDTD_START                         = 12
	JSPParserWHITESPACE_SKIP                   = 13
	JSPParserCLOSE_TAG_BEGIN                   = 14
	JSPParserTAG_BEGIN                         = 15
	JSPParserDIRECTIVE_BEGIN                   = 16
	JSPParserDECLARATION_BEGIN                 = 17
	JSPParserECHO_EXPRESSION_OPEN              = 18
	JSPParserSCRIPTLET_OPEN                    = 19
	JSPParserEXPRESSION_OPEN                   = 20
	JSPParserWHITESPACES                       = 21
	JSPParserDOUBLE_QUOTE                      = 22
	JSPParserSINGLE_QUOTE                      = 23
	JSPParserQUOTE                             = 24
	JSPParserTAG_END                           = 25
	JSPParserEQUALS                            = 26
	JSPParserJSP_STATIC_CONTENT_CHARS_MIXED    = 27
	JSPParserJSP_STATIC_CONTENT_CHARS          = 28
	JSPParserJSP_STATIC_CONTENT_CHAR           = 29
	JSPParserJSP_END                           = 30
	JSPParserJSP_CONDITIONAL_COMMENT_END       = 31
	JSPParserJSP_CONDITIONAL_COMMENT           = 32
	JSPParserJSP_COMMENT_TEXT                  = 33
	JSPParserDTD_PUBLIC                        = 34
	JSPParserDTD_SYSTEM                        = 35
	JSPParserDTD_WHITESPACE_SKIP               = 36
	JSPParserDTD_QUOTED                        = 37
	JSPParserDTD_IDENTIFIER                    = 38
	JSPParserBLOB_CLOSE                        = 39
	JSPParserBLOB_CONTENT                      = 40
	JSPParserJSPEXPR_CONTENT_CLOSE             = 41
	JSPParserJSP_JSTL_COLON                    = 42
	JSPParserTAG_SLASH_END                     = 43
	JSPParserTAG_CLOSE                         = 44
	JSPParserTAG_SLASH                         = 45
	JSPParserDIRECTIVE_END                     = 46
	JSPParserTAG_IDENTIFIER                    = 47
	JSPParserTAG_WHITESPACE                    = 48
	JSPParserSCRIPT_BODY                       = 49
	JSPParserSCRIPT_SHORT_BODY                 = 50
	JSPParserSTYLE_BODY                        = 51
	JSPParserSTYLE_SHORT_BODY                  = 52
	JSPParserATTVAL_ATTRIBUTE                  = 53
	JSPParserEL_EXPR                           = 54
)

// JSPParser rules.
const (
	JSPParserRULE_jspDocuments               = 0
	JSPParserRULE_jspDocument                = 1
	JSPParserRULE_jspStart                   = 2
	JSPParserRULE_jspElements                = 3
	JSPParserRULE_jspElement                 = 4
	JSPParserRULE_htmlBegin                  = 5
	JSPParserRULE_htmlTag                    = 6
	JSPParserRULE_jspDirective               = 7
	JSPParserRULE_htmlContents               = 8
	JSPParserRULE_htmlContent                = 9
	JSPParserRULE_jspExpression              = 10
	JSPParserRULE_htmlAttribute              = 11
	JSPParserRULE_htmlAttributeName          = 12
	JSPParserRULE_htmlAttributeValue         = 13
	JSPParserRULE_htmlAttributeValueExpr     = 14
	JSPParserRULE_htmlAttributeValueConstant = 15
	JSPParserRULE_htmlTagName                = 16
	JSPParserRULE_htmlChardata               = 17
	JSPParserRULE_htmlMisc                   = 18
	JSPParserRULE_htmlComment                = 19
	JSPParserRULE_htmlCommentText            = 20
	JSPParserRULE_htmlConditionalCommentText = 21
	JSPParserRULE_xhtmlCDATA                 = 22
	JSPParserRULE_dtd                        = 23
	JSPParserRULE_dtdElementName             = 24
	JSPParserRULE_publicId                   = 25
	JSPParserRULE_systemId                   = 26
	JSPParserRULE_xml                        = 27
	JSPParserRULE_scriptlet                  = 28
)

// IJspDocumentsContext is an interface to support dynamic dispatch.
type IJspDocumentsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsJspDocumentsContext differentiates from other interfaces.
	IsJspDocumentsContext()
}

type JspDocumentsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyJspDocumentsContext() *JspDocumentsContext {
	var p = new(JspDocumentsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_jspDocuments
	return p
}

func (*JspDocumentsContext) IsJspDocumentsContext() {}

func NewJspDocumentsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *JspDocumentsContext {
	var p = new(JspDocumentsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_jspDocuments

	return p
}

func (s *JspDocumentsContext) GetParser() antlr.Parser { return s.parser }

func (s *JspDocumentsContext) AllJspDocument() []IJspDocumentContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IJspDocumentContext); ok {
			len++
		}
	}

	tst := make([]IJspDocumentContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IJspDocumentContext); ok {
			tst[i] = t.(IJspDocumentContext)
			i++
		}
	}

	return tst
}

func (s *JspDocumentsContext) JspDocument(i int) IJspDocumentContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspDocumentContext); ok {
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

	return t.(IJspDocumentContext)
}

func (s *JspDocumentsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspDocumentsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *JspDocumentsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJspDocuments(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) JspDocuments() (localctx IJspDocumentsContext) {
	this := p
	_ = this

	localctx = NewJspDocumentsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, JSPParserRULE_jspDocuments)
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
	p.SetState(59)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&18014398512597570) != 0 {
		{
			p.SetState(58)
			p.JspDocument()
		}

		p.SetState(61)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

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
	p.EnterRule(localctx, 2, JSPParserRULE_jspDocument)
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

	p.SetState(88)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 5, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		p.SetState(66)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&3080192) != 0 {
			{
				p.SetState(63)
				p.JspStart()
			}

			p.SetState(68)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(69)
			p.Xml()
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		p.SetState(73)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&3080192) != 0 {
			{
				p.SetState(70)
				p.JspStart()
			}

			p.SetState(75)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(76)
			p.Dtd()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		p.SetState(80)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 3, p.GetParserRuleContext())

		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(77)
					p.JspStart()
				}

			}
			p.SetState(82)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 3, p.GetParserRuleContext())
		}
		p.SetState(84)
		p.GetErrorHandler().Sync(p)
		_alt = 1
		for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			switch _alt {
			case 1:
				{
					p.SetState(83)
					p.JspElements()
				}

			default:
				panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
			}

			p.SetState(86)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 4, p.GetParserRuleContext())
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
	p.EnterRule(localctx, 4, JSPParserRULE_jspStart)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(93)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserDIRECTIVE_BEGIN:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(90)
			p.JspDirective()
		}

	case JSPParserDECLARATION_BEGIN, JSPParserECHO_EXPRESSION_OPEN, JSPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(91)
			p.Scriptlet()
		}

	case JSPParserWHITESPACES:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(92)
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
	p.EnterRule(localctx, 6, JSPParserRULE_jspElements)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	p.SetState(98)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 7, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(95)
				p.HtmlMisc()
			}

		}
		p.SetState(100)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 7, p.GetParserRuleContext())
	}
	p.SetState(104)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserTAG_BEGIN:
		{
			p.SetState(101)
			p.JspElement()
		}

	case JSPParserDIRECTIVE_BEGIN:
		{
			p.SetState(102)
			p.JspDirective()
		}

	case JSPParserDECLARATION_BEGIN, JSPParserECHO_EXPRESSION_OPEN, JSPParserSCRIPTLET_OPEN:
		{
			p.SetState(103)
			p.Scriptlet()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}
	p.SetState(109)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 9, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(106)
				p.HtmlMisc()
			}

		}
		p.SetState(111)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 9, p.GetParserRuleContext())
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

func (s *JspElementContext) HtmlBegin() IHtmlBeginContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlBeginContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlBeginContext)
}

func (s *JspElementContext) AllTAG_CLOSE() []antlr.TerminalNode {
	return s.GetTokens(JSPParserTAG_CLOSE)
}

func (s *JspElementContext) TAG_CLOSE(i int) antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_CLOSE, i)
}

func (s *JspElementContext) TAG_SLASH_END() antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_SLASH_END, 0)
}

func (s *JspElementContext) HtmlContent() IHtmlContentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlContentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlContentContext)
}

func (s *JspElementContext) CLOSE_TAG_BEGIN() antlr.TerminalNode {
	return s.GetToken(JSPParserCLOSE_TAG_BEGIN, 0)
}

func (s *JspElementContext) HtmlTag() IHtmlTagContext {
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

func (s *JspElementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspElementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *JspElementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJspElement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) JspElement() (localctx IJspElementContext) {
	this := p
	_ = this

	localctx = NewJspElementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, JSPParserRULE_jspElement)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(112)
		p.HtmlBegin()
	}
	p.SetState(122)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserTAG_CLOSE:
		{
			p.SetState(113)
			p.Match(JSPParserTAG_CLOSE)
		}
		p.SetState(119)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 10, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(114)
				p.HtmlContent()
			}
			{
				p.SetState(115)
				p.Match(JSPParserCLOSE_TAG_BEGIN)
			}
			{
				p.SetState(116)
				p.HtmlTag()
			}
			{
				p.SetState(117)
				p.Match(JSPParserTAG_CLOSE)
			}

		}

	case JSPParserTAG_SLASH_END:
		{
			p.SetState(121)
			p.Match(JSPParserTAG_SLASH_END)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IHtmlBeginContext is an interface to support dynamic dispatch.
type IHtmlBeginContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlBeginContext differentiates from other interfaces.
	IsHtmlBeginContext()
}

type HtmlBeginContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlBeginContext() *HtmlBeginContext {
	var p = new(HtmlBeginContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlBegin
	return p
}

func (*HtmlBeginContext) IsHtmlBeginContext() {}

func NewHtmlBeginContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlBeginContext {
	var p = new(HtmlBeginContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlBegin

	return p
}

func (s *HtmlBeginContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlBeginContext) TAG_BEGIN() antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_BEGIN, 0)
}

func (s *HtmlBeginContext) HtmlTag() IHtmlTagContext {
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

func (s *HtmlBeginContext) AllHtmlAttribute() []IHtmlAttributeContext {
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

func (s *HtmlBeginContext) HtmlAttribute(i int) IHtmlAttributeContext {
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

func (s *HtmlBeginContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlBeginContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlBeginContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlBegin(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlBegin() (localctx IHtmlBeginContext) {
	this := p
	_ = this

	localctx = NewHtmlBeginContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, JSPParserRULE_htmlBegin)
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
		p.SetState(124)
		p.Match(JSPParserTAG_BEGIN)
	}
	{
		p.SetState(125)
		p.HtmlTag()
	}
	p.SetState(129)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&140737489272832) != 0 {
		{
			p.SetState(126)
			p.HtmlAttribute()
		}

		p.SetState(131)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
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
	p.EnterRule(localctx, 12, JSPParserRULE_htmlTag)
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
		p.SetState(132)
		p.HtmlTagName()
	}
	p.SetState(135)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == JSPParserJSP_JSTL_COLON {
		{
			p.SetState(133)
			p.Match(JSPParserJSP_JSTL_COLON)
		}
		{
			p.SetState(134)
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

	// IsJspDirectiveContext differentiates from other interfaces.
	IsJspDirectiveContext()
}

type JspDirectiveContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
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

func (s *JspDirectiveContext) DIRECTIVE_BEGIN() antlr.TerminalNode {
	return s.GetToken(JSPParserDIRECTIVE_BEGIN, 0)
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

func (s *JspDirectiveContext) DIRECTIVE_END() antlr.TerminalNode {
	return s.GetToken(JSPParserDIRECTIVE_END, 0)
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

func (s *JspDirectiveContext) AllTAG_WHITESPACE() []antlr.TerminalNode {
	return s.GetTokens(JSPParserTAG_WHITESPACE)
}

func (s *JspDirectiveContext) TAG_WHITESPACE(i int) antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_WHITESPACE, i)
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
	p.EnterRule(localctx, 14, JSPParserRULE_jspDirective)
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
		p.SetState(137)
		p.Match(JSPParserDIRECTIVE_BEGIN)
	}
	{
		p.SetState(138)
		p.HtmlTagName()
	}
	p.SetState(142)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 14, p.GetParserRuleContext())

	for _alt != 1 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1+1 {
			{
				p.SetState(139)
				p.HtmlAttribute()
			}

		}
		p.SetState(144)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 14, p.GetParserRuleContext())
	}
	p.SetState(148)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == JSPParserTAG_WHITESPACE {
		{
			p.SetState(145)
			p.Match(JSPParserTAG_WHITESPACE)
		}

		p.SetState(150)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(151)
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

func (s *HtmlContentsContext) AllHtmlChardata() []IHtmlChardataContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlChardataContext); ok {
			len++
		}
	}

	tst := make([]IHtmlChardataContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlChardataContext); ok {
			tst[i] = t.(IHtmlChardataContext)
			i++
		}
	}

	return tst
}

func (s *HtmlContentsContext) HtmlChardata(i int) IHtmlChardataContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlChardataContext); ok {
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

	return t.(IHtmlChardataContext)
}

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
	p.EnterRule(localctx, 16, JSPParserRULE_htmlContents)
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
	p.SetState(154)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 16, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(153)
			p.HtmlChardata()
		}

	}
	p.SetState(162)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&18014398915249234) != 0 {
		{
			p.SetState(156)
			p.HtmlContent()
		}
		p.SetState(158)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 17, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(157)
				p.HtmlChardata()
			}

		}

		p.SetState(164)
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
	p.EnterRule(localctx, 18, JSPParserRULE_htmlContent)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(172)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserHTML_TEXT, JSPParserWHITESPACES, JSPParserJSP_STATIC_CONTENT_CHARS_MIXED, JSPParserJSP_STATIC_CONTENT_CHARS:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(165)
			p.HtmlChardata()
		}

	case JSPParserEL_EXPR:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(166)
			p.JspExpression()
		}

	case JSPParserTAG_BEGIN:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(167)
			p.JspElement()
		}

	case JSPParserCDATA:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(168)
			p.XhtmlCDATA()
		}

	case JSPParserJSP_COMMENT_START, JSPParserJSP_CONDITIONAL_COMMENT_START:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(169)
			p.HtmlComment()
		}

	case JSPParserDECLARATION_BEGIN, JSPParserECHO_EXPRESSION_OPEN, JSPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(170)
			p.Scriptlet()
		}

	case JSPParserDIRECTIVE_BEGIN:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(171)
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
	p.EnterRule(localctx, 20, JSPParserRULE_jspExpression)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(174)
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
	p.EnterRule(localctx, 22, JSPParserRULE_htmlAttribute)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(182)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 20, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(176)
			p.HtmlAttributeName()
		}
		{
			p.SetState(177)
			p.Match(JSPParserEQUALS)
		}
		{
			p.SetState(178)
			p.HtmlAttributeValue()
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(180)
			p.HtmlAttributeName()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(181)
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
	p.EnterRule(localctx, 24, JSPParserRULE_htmlAttributeName)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(184)
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
	p.EnterRule(localctx, 26, JSPParserRULE_htmlAttributeValue)
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

	p.SetState(202)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 24, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(186)
			p.Match(JSPParserQUOTE)
		}
		{
			p.SetState(187)
			p.JspElement()
		}
		{
			p.SetState(188)
			p.Match(JSPParserQUOTE)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		p.SetState(191)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == JSPParserQUOTE {
			{
				p.SetState(190)
				p.Match(JSPParserQUOTE)
			}

		}
		{
			p.SetState(193)
			p.HtmlAttributeValueExpr()
		}
		p.SetState(195)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == JSPParserQUOTE {
			{
				p.SetState(194)
				p.Match(JSPParserQUOTE)
			}

		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(197)
			p.Match(JSPParserQUOTE)
		}
		p.SetState(199)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == JSPParserATTVAL_ATTRIBUTE {
			{
				p.SetState(198)
				p.HtmlAttributeValueConstant()
			}

		}
		{
			p.SetState(201)
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
	p.EnterRule(localctx, 28, JSPParserRULE_htmlAttributeValueExpr)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	p.EnterRule(localctx, 30, JSPParserRULE_htmlAttributeValueConstant)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	p.EnterRule(localctx, 32, JSPParserRULE_htmlTagName)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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

func (s *HtmlChardataContext) HTML_TEXT() antlr.TerminalNode {
	return s.GetToken(JSPParserHTML_TEXT, 0)
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
	p.EnterRule(localctx, 34, JSPParserRULE_htmlChardata)
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
		p.SetState(210)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&404750352) != 0) {
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
	p.EnterRule(localctx, 36, JSPParserRULE_htmlMisc)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(215)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserJSP_COMMENT_START, JSPParserJSP_CONDITIONAL_COMMENT_START:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(212)
			p.HtmlComment()
		}

	case JSPParserEL_EXPR:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(213)
			p.JspExpression()
		}

	case JSPParserDECLARATION_BEGIN, JSPParserECHO_EXPRESSION_OPEN, JSPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(214)
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
	p.EnterRule(localctx, 38, JSPParserRULE_htmlComment)
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

	p.SetState(227)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserJSP_COMMENT_START:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(217)
			p.Match(JSPParserJSP_COMMENT_START)
		}
		p.SetState(219)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == JSPParserJSP_COMMENT_TEXT {
			{
				p.SetState(218)
				p.HtmlCommentText()
			}

		}
		{
			p.SetState(221)
			p.Match(JSPParserJSP_COMMENT_END)
		}

	case JSPParserJSP_CONDITIONAL_COMMENT_START:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(222)
			p.Match(JSPParserJSP_CONDITIONAL_COMMENT_START)
		}
		p.SetState(224)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == JSPParserJSP_CONDITIONAL_COMMENT {
			{
				p.SetState(223)
				p.HtmlConditionalCommentText()
			}

		}
		{
			p.SetState(226)
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
	p.EnterRule(localctx, 40, JSPParserRULE_htmlCommentText)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	p.SetState(230)
	p.GetErrorHandler().Sync(p)
	_alt = 1 + 1
	for ok := true; ok; ok = _alt != 1 && _alt != antlr.ATNInvalidAltNumber {
		switch _alt {
		case 1 + 1:
			{
				p.SetState(229)
				p.Match(JSPParserJSP_COMMENT_TEXT)
			}

		default:
			panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		}

		p.SetState(232)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 29, p.GetParserRuleContext())
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
	p.EnterRule(localctx, 42, JSPParserRULE_htmlConditionalCommentText)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(234)
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
	p.EnterRule(localctx, 44, JSPParserRULE_xhtmlCDATA)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(236)
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
	p.EnterRule(localctx, 46, JSPParserRULE_dtd)
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
		p.SetState(238)
		p.Match(JSPParserDTD)
	}
	{
		p.SetState(239)
		p.DtdElementName()
	}
	p.SetState(242)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == JSPParserDTD_PUBLIC {
		{
			p.SetState(240)
			p.Match(JSPParserDTD_PUBLIC)
		}
		{
			p.SetState(241)
			p.PublicId()
		}

	}
	p.SetState(246)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == JSPParserDTD_SYSTEM {
		{
			p.SetState(244)
			p.Match(JSPParserDTD_SYSTEM)
		}
		{
			p.SetState(245)
			p.SystemId()
		}

	}
	{
		p.SetState(248)
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
	p.EnterRule(localctx, 48, JSPParserRULE_dtdElementName)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(250)
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
	p.EnterRule(localctx, 50, JSPParserRULE_publicId)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	p.EnterRule(localctx, 52, JSPParserRULE_systemId)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	p.EnterRule(localctx, 54, JSPParserRULE_xml)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(256)
		p.Match(JSPParserXML_DECLARATION)
	}
	{
		p.SetState(257)

		var _x = p.HtmlTagName()

		localctx.(*XmlContext).name = _x
	}
	p.SetState(261)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 32, p.GetParserRuleContext())

	for _alt != 1 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1+1 {
			{
				p.SetState(258)

				var _x = p.HtmlAttribute()

				localctx.(*XmlContext)._htmlAttribute = _x
			}
			localctx.(*XmlContext).atts = append(localctx.(*XmlContext).atts, localctx.(*XmlContext)._htmlAttribute)

		}
		p.SetState(263)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 32, p.GetParserRuleContext())
	}
	{
		p.SetState(264)
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

func (s *ScriptletContext) BLOB_CLOSE() antlr.TerminalNode {
	return s.GetToken(JSPParserBLOB_CLOSE, 0)
}

func (s *ScriptletContext) ECHO_EXPRESSION_OPEN() antlr.TerminalNode {
	return s.GetToken(JSPParserECHO_EXPRESSION_OPEN, 0)
}

func (s *ScriptletContext) DECLARATION_BEGIN() antlr.TerminalNode {
	return s.GetToken(JSPParserDECLARATION_BEGIN, 0)
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
	p.EnterRule(localctx, 56, JSPParserRULE_scriptlet)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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

	switch p.GetTokenStream().LA(1) {
	case JSPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(266)
			p.Match(JSPParserSCRIPTLET_OPEN)
		}
		{
			p.SetState(267)
			p.Match(JSPParserBLOB_CONTENT)
		}
		{
			p.SetState(268)
			p.Match(JSPParserBLOB_CLOSE)
		}

	case JSPParserECHO_EXPRESSION_OPEN:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(269)
			p.Match(JSPParserECHO_EXPRESSION_OPEN)
		}
		{
			p.SetState(270)
			p.Match(JSPParserBLOB_CONTENT)
		}
		{
			p.SetState(271)
			p.Match(JSPParserBLOB_CLOSE)
		}

	case JSPParserDECLARATION_BEGIN:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(272)
			p.Match(JSPParserDECLARATION_BEGIN)
		}
		{
			p.SetState(273)
			p.Match(JSPParserBLOB_CONTENT)
		}
		{
			p.SetState(274)
			p.Match(JSPParserBLOB_CLOSE)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}
