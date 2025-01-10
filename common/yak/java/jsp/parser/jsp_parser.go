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
		"", "", "", "", "", "'<!DOCTYPE'", "", "", "'<?xml'", "", "", "", "",
		"", "", "", "", "", "", "", "", "", "", "", "", "", "'PUBLIC'", "'SYSTEM'",
		"", "", "", "", "", "", "'%>'", "':'", "", "", "'/'",
	}
	staticData.symbolicNames = []string{
		"", "JSP_COMMENT", "JSP_CONDITIONAL_COMMENT", "SCRIPT_OPEN", "STYLE_OPEN",
		"DTD", "CDATA", "WHITESPACES", "XML_DECLARATION", "WHITESPACE_SKIP",
		"CLOSE_TAG_BEGIN", "TAG_BEGIN", "DIRECTIVE_BEGIN", "DECLARATION_BEGIN",
		"ECHO_EXPRESSION_OPEN", "SCRIPTLET_OPEN", "EXPRESSION_OPEN", "QUOTE",
		"TAG_END", "EQUALS", "EL_EXPR_START", "JSP_STATIC_CONTENT_CHARS", "JSP_END",
		"ATTVAL_ATTRIBUTE", "ATTVAL_VALUE", "EL_EXPR_END", "DTD_PUBLIC", "DTD_SYSTEM",
		"DTD_WHITESPACE_SKIP", "DTD_QUOTED", "DTD_IDENTIFIER", "BLOB_CLOSE",
		"BLOB_CONTENT", "JSPEXPR_EL_EXPR", "JSPEXPR_CONTENT_CLOSE", "JSP_JSTL_COLON",
		"TAG_SLASH_END", "TAG_CLOSE", "TAG_SLASH", "DIRECTIVE_END", "TAG_IDENTIFIER",
		"TAG_WHITESPACE", "SCRIPT_BODY", "SCRIPT_SHORT_BODY", "STYLE_BODY",
		"STYLE_SHORT_BODY", "ATTVAL_EL_EXPR", "ATTVAL_SINGLE_QUOTE_EXPRESSION",
		"ATTVAL_DOUBLE_QUOTE_EXPRESSION", "EL_EXPR_CONTENT",
	}
	staticData.ruleNames = []string{
		"jspDocuments", "jspDocument", "jspStart", "jspElements", "htmlMiscs",
		"jspScript", "htmlElement", "htmlBegin", "htmlTag", "jspDirective",
		"htmlContents", "htmlContent", "elExpression", "htmlAttribute", "htmlAttributeName",
		"htmlAttributeValue", "htmlAttributeValueElement", "htmlTagName", "htmlChardata",
		"htmlMisc", "htmlComment", "xhtmlCDATA", "dtd", "dtdElementName", "publicId",
		"systemId", "xml", "jspScriptlet", "jspExpression", "scriptletStart",
		"scriptletContent", "javaScript", "style",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 49, 264, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7, 20, 2,
		21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25, 2, 26,
		7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 2, 29, 7, 29, 2, 30, 7, 30, 2, 31, 7,
		31, 2, 32, 7, 32, 1, 0, 5, 0, 68, 8, 0, 10, 0, 12, 0, 71, 9, 0, 1, 0, 4,
		0, 74, 8, 0, 11, 0, 12, 0, 75, 1, 1, 1, 1, 1, 1, 3, 1, 81, 8, 1, 1, 2,
		1, 2, 3, 2, 85, 8, 2, 1, 3, 1, 3, 1, 3, 1, 3, 1, 3, 1, 3, 3, 3, 93, 8,
		3, 1, 3, 1, 3, 1, 4, 5, 4, 98, 8, 4, 10, 4, 12, 4, 101, 9, 4, 1, 5, 1,
		5, 3, 5, 105, 8, 5, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 3, 6, 114,
		8, 6, 1, 6, 3, 6, 117, 8, 6, 1, 7, 1, 7, 1, 7, 5, 7, 122, 8, 7, 10, 7,
		12, 7, 125, 9, 7, 1, 8, 1, 8, 1, 8, 3, 8, 130, 8, 8, 1, 9, 1, 9, 1, 9,
		5, 9, 135, 8, 9, 10, 9, 12, 9, 138, 9, 9, 1, 9, 5, 9, 141, 8, 9, 10, 9,
		12, 9, 144, 9, 9, 1, 9, 1, 9, 1, 10, 3, 10, 149, 8, 10, 1, 10, 1, 10, 3,
		10, 153, 8, 10, 5, 10, 155, 8, 10, 10, 10, 12, 10, 158, 9, 10, 1, 11, 1,
		11, 1, 11, 1, 11, 3, 11, 164, 8, 11, 1, 12, 1, 12, 1, 12, 1, 12, 1, 13,
		1, 13, 1, 13, 1, 13, 1, 13, 1, 13, 3, 13, 176, 8, 13, 1, 14, 1, 14, 1,
		15, 3, 15, 181, 8, 15, 1, 15, 5, 15, 184, 8, 15, 10, 15, 12, 15, 187, 9,
		15, 1, 15, 3, 15, 190, 8, 15, 1, 16, 1, 16, 1, 16, 3, 16, 195, 8, 16, 1,
		17, 1, 17, 1, 18, 1, 18, 1, 19, 1, 19, 1, 19, 1, 19, 3, 19, 205, 8, 19,
		1, 20, 1, 20, 1, 21, 1, 21, 1, 22, 1, 22, 1, 22, 1, 22, 5, 22, 215, 8,
		22, 10, 22, 12, 22, 218, 9, 22, 3, 22, 220, 8, 22, 1, 22, 1, 22, 3, 22,
		224, 8, 22, 1, 22, 1, 22, 1, 23, 1, 23, 1, 24, 1, 24, 1, 25, 1, 25, 1,
		26, 1, 26, 1, 26, 5, 26, 237, 8, 26, 10, 26, 12, 26, 240, 9, 26, 1, 26,
		1, 26, 1, 27, 1, 27, 1, 27, 1, 27, 3, 27, 248, 8, 27, 1, 28, 1, 28, 1,
		28, 1, 29, 1, 29, 1, 30, 1, 30, 1, 30, 1, 31, 1, 31, 1, 31, 1, 32, 1, 32,
		1, 32, 1, 32, 2, 136, 238, 0, 33, 0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20,
		22, 24, 26, 28, 30, 32, 34, 36, 38, 40, 42, 44, 46, 48, 50, 52, 54, 56,
		58, 60, 62, 64, 0, 5, 2, 0, 7, 7, 21, 21, 1, 0, 1, 2, 2, 0, 13, 13, 15,
		15, 1, 0, 42, 43, 1, 0, 44, 45, 268, 0, 69, 1, 0, 0, 0, 2, 80, 1, 0, 0,
		0, 4, 84, 1, 0, 0, 0, 6, 86, 1, 0, 0, 0, 8, 99, 1, 0, 0, 0, 10, 104, 1,
		0, 0, 0, 12, 106, 1, 0, 0, 0, 14, 118, 1, 0, 0, 0, 16, 126, 1, 0, 0, 0,
		18, 131, 1, 0, 0, 0, 20, 148, 1, 0, 0, 0, 22, 163, 1, 0, 0, 0, 24, 165,
		1, 0, 0, 0, 26, 175, 1, 0, 0, 0, 28, 177, 1, 0, 0, 0, 30, 180, 1, 0, 0,
		0, 32, 194, 1, 0, 0, 0, 34, 196, 1, 0, 0, 0, 36, 198, 1, 0, 0, 0, 38, 204,
		1, 0, 0, 0, 40, 206, 1, 0, 0, 0, 42, 208, 1, 0, 0, 0, 44, 210, 1, 0, 0,
		0, 46, 227, 1, 0, 0, 0, 48, 229, 1, 0, 0, 0, 50, 231, 1, 0, 0, 0, 52, 233,
		1, 0, 0, 0, 54, 247, 1, 0, 0, 0, 56, 249, 1, 0, 0, 0, 58, 252, 1, 0, 0,
		0, 60, 254, 1, 0, 0, 0, 62, 257, 1, 0, 0, 0, 64, 260, 1, 0, 0, 0, 66, 68,
		3, 4, 2, 0, 67, 66, 1, 0, 0, 0, 68, 71, 1, 0, 0, 0, 69, 67, 1, 0, 0, 0,
		69, 70, 1, 0, 0, 0, 70, 73, 1, 0, 0, 0, 71, 69, 1, 0, 0, 0, 72, 74, 3,
		2, 1, 0, 73, 72, 1, 0, 0, 0, 74, 75, 1, 0, 0, 0, 75, 73, 1, 0, 0, 0, 75,
		76, 1, 0, 0, 0, 76, 1, 1, 0, 0, 0, 77, 81, 3, 52, 26, 0, 78, 81, 3, 44,
		22, 0, 79, 81, 3, 6, 3, 0, 80, 77, 1, 0, 0, 0, 80, 78, 1, 0, 0, 0, 80,
		79, 1, 0, 0, 0, 81, 3, 1, 0, 0, 0, 82, 85, 3, 10, 5, 0, 83, 85, 5, 7, 0,
		0, 84, 82, 1, 0, 0, 0, 84, 83, 1, 0, 0, 0, 85, 5, 1, 0, 0, 0, 86, 92, 3,
		8, 4, 0, 87, 93, 3, 12, 6, 0, 88, 93, 3, 10, 5, 0, 89, 93, 3, 56, 28, 0,
		90, 93, 3, 64, 32, 0, 91, 93, 3, 62, 31, 0, 92, 87, 1, 0, 0, 0, 92, 88,
		1, 0, 0, 0, 92, 89, 1, 0, 0, 0, 92, 90, 1, 0, 0, 0, 92, 91, 1, 0, 0, 0,
		93, 94, 1, 0, 0, 0, 94, 95, 3, 8, 4, 0, 95, 7, 1, 0, 0, 0, 96, 98, 3, 38,
		19, 0, 97, 96, 1, 0, 0, 0, 98, 101, 1, 0, 0, 0, 99, 97, 1, 0, 0, 0, 99,
		100, 1, 0, 0, 0, 100, 9, 1, 0, 0, 0, 101, 99, 1, 0, 0, 0, 102, 105, 3,
		18, 9, 0, 103, 105, 3, 54, 27, 0, 104, 102, 1, 0, 0, 0, 104, 103, 1, 0,
		0, 0, 105, 11, 1, 0, 0, 0, 106, 116, 3, 14, 7, 0, 107, 113, 5, 37, 0, 0,
		108, 109, 3, 20, 10, 0, 109, 110, 5, 10, 0, 0, 110, 111, 3, 16, 8, 0, 111,
		112, 5, 37, 0, 0, 112, 114, 1, 0, 0, 0, 113, 108, 1, 0, 0, 0, 113, 114,
		1, 0, 0, 0, 114, 117, 1, 0, 0, 0, 115, 117, 5, 36, 0, 0, 116, 107, 1, 0,
		0, 0, 116, 115, 1, 0, 0, 0, 117, 13, 1, 0, 0, 0, 118, 119, 5, 11, 0, 0,
		119, 123, 3, 16, 8, 0, 120, 122, 3, 26, 13, 0, 121, 120, 1, 0, 0, 0, 122,
		125, 1, 0, 0, 0, 123, 121, 1, 0, 0, 0, 123, 124, 1, 0, 0, 0, 124, 15, 1,
		0, 0, 0, 125, 123, 1, 0, 0, 0, 126, 129, 3, 34, 17, 0, 127, 128, 5, 35,
		0, 0, 128, 130, 3, 34, 17, 0, 129, 127, 1, 0, 0, 0, 129, 130, 1, 0, 0,
		0, 130, 17, 1, 0, 0, 0, 131, 132, 5, 12, 0, 0, 132, 136, 3, 34, 17, 0,
		133, 135, 3, 26, 13, 0, 134, 133, 1, 0, 0, 0, 135, 138, 1, 0, 0, 0, 136,
		137, 1, 0, 0, 0, 136, 134, 1, 0, 0, 0, 137, 142, 1, 0, 0, 0, 138, 136,
		1, 0, 0, 0, 139, 141, 5, 41, 0, 0, 140, 139, 1, 0, 0, 0, 141, 144, 1, 0,
		0, 0, 142, 140, 1, 0, 0, 0, 142, 143, 1, 0, 0, 0, 143, 145, 1, 0, 0, 0,
		144, 142, 1, 0, 0, 0, 145, 146, 5, 39, 0, 0, 146, 19, 1, 0, 0, 0, 147,
		149, 3, 36, 18, 0, 148, 147, 1, 0, 0, 0, 148, 149, 1, 0, 0, 0, 149, 156,
		1, 0, 0, 0, 150, 152, 3, 22, 11, 0, 151, 153, 3, 36, 18, 0, 152, 151, 1,
		0, 0, 0, 152, 153, 1, 0, 0, 0, 153, 155, 1, 0, 0, 0, 154, 150, 1, 0, 0,
		0, 155, 158, 1, 0, 0, 0, 156, 154, 1, 0, 0, 0, 156, 157, 1, 0, 0, 0, 157,
		21, 1, 0, 0, 0, 158, 156, 1, 0, 0, 0, 159, 164, 3, 24, 12, 0, 160, 164,
		3, 6, 3, 0, 161, 164, 3, 42, 21, 0, 162, 164, 3, 40, 20, 0, 163, 159, 1,
		0, 0, 0, 163, 160, 1, 0, 0, 0, 163, 161, 1, 0, 0, 0, 163, 162, 1, 0, 0,
		0, 164, 23, 1, 0, 0, 0, 165, 166, 5, 20, 0, 0, 166, 167, 5, 49, 0, 0, 167,
		168, 5, 25, 0, 0, 168, 25, 1, 0, 0, 0, 169, 170, 3, 28, 14, 0, 170, 171,
		5, 19, 0, 0, 171, 172, 3, 30, 15, 0, 172, 176, 1, 0, 0, 0, 173, 176, 3,
		28, 14, 0, 174, 176, 3, 56, 28, 0, 175, 169, 1, 0, 0, 0, 175, 173, 1, 0,
		0, 0, 175, 174, 1, 0, 0, 0, 176, 27, 1, 0, 0, 0, 177, 178, 5, 40, 0, 0,
		178, 29, 1, 0, 0, 0, 179, 181, 5, 17, 0, 0, 180, 179, 1, 0, 0, 0, 180,
		181, 1, 0, 0, 0, 181, 185, 1, 0, 0, 0, 182, 184, 3, 32, 16, 0, 183, 182,
		1, 0, 0, 0, 184, 187, 1, 0, 0, 0, 185, 183, 1, 0, 0, 0, 185, 186, 1, 0,
		0, 0, 186, 189, 1, 0, 0, 0, 187, 185, 1, 0, 0, 0, 188, 190, 5, 17, 0, 0,
		189, 188, 1, 0, 0, 0, 189, 190, 1, 0, 0, 0, 190, 31, 1, 0, 0, 0, 191, 195,
		5, 23, 0, 0, 192, 195, 3, 56, 28, 0, 193, 195, 3, 24, 12, 0, 194, 191,
		1, 0, 0, 0, 194, 192, 1, 0, 0, 0, 194, 193, 1, 0, 0, 0, 195, 33, 1, 0,
		0, 0, 196, 197, 5, 40, 0, 0, 197, 35, 1, 0, 0, 0, 198, 199, 7, 0, 0, 0,
		199, 37, 1, 0, 0, 0, 200, 205, 3, 40, 20, 0, 201, 205, 3, 24, 12, 0, 202,
		205, 3, 54, 27, 0, 203, 205, 5, 7, 0, 0, 204, 200, 1, 0, 0, 0, 204, 201,
		1, 0, 0, 0, 204, 202, 1, 0, 0, 0, 204, 203, 1, 0, 0, 0, 205, 39, 1, 0,
		0, 0, 206, 207, 7, 1, 0, 0, 207, 41, 1, 0, 0, 0, 208, 209, 5, 6, 0, 0,
		209, 43, 1, 0, 0, 0, 210, 211, 5, 5, 0, 0, 211, 219, 3, 46, 23, 0, 212,
		216, 5, 26, 0, 0, 213, 215, 3, 48, 24, 0, 214, 213, 1, 0, 0, 0, 215, 218,
		1, 0, 0, 0, 216, 214, 1, 0, 0, 0, 216, 217, 1, 0, 0, 0, 217, 220, 1, 0,
		0, 0, 218, 216, 1, 0, 0, 0, 219, 212, 1, 0, 0, 0, 219, 220, 1, 0, 0, 0,
		220, 223, 1, 0, 0, 0, 221, 222, 5, 27, 0, 0, 222, 224, 3, 50, 25, 0, 223,
		221, 1, 0, 0, 0, 223, 224, 1, 0, 0, 0, 224, 225, 1, 0, 0, 0, 225, 226,
		5, 18, 0, 0, 226, 45, 1, 0, 0, 0, 227, 228, 5, 30, 0, 0, 228, 47, 1, 0,
		0, 0, 229, 230, 5, 29, 0, 0, 230, 49, 1, 0, 0, 0, 231, 232, 5, 29, 0, 0,
		232, 51, 1, 0, 0, 0, 233, 234, 5, 8, 0, 0, 234, 238, 3, 34, 17, 0, 235,
		237, 3, 26, 13, 0, 236, 235, 1, 0, 0, 0, 237, 240, 1, 0, 0, 0, 238, 239,
		1, 0, 0, 0, 238, 236, 1, 0, 0, 0, 239, 241, 1, 0, 0, 0, 240, 238, 1, 0,
		0, 0, 241, 242, 5, 18, 0, 0, 242, 53, 1, 0, 0, 0, 243, 244, 3, 58, 29,
		0, 244, 245, 3, 60, 30, 0, 245, 248, 1, 0, 0, 0, 246, 248, 3, 56, 28, 0,
		247, 243, 1, 0, 0, 0, 247, 246, 1, 0, 0, 0, 248, 55, 1, 0, 0, 0, 249, 250,
		5, 14, 0, 0, 250, 251, 3, 60, 30, 0, 251, 57, 1, 0, 0, 0, 252, 253, 7,
		2, 0, 0, 253, 59, 1, 0, 0, 0, 254, 255, 5, 32, 0, 0, 255, 256, 5, 31, 0,
		0, 256, 61, 1, 0, 0, 0, 257, 258, 5, 3, 0, 0, 258, 259, 7, 3, 0, 0, 259,
		63, 1, 0, 0, 0, 260, 261, 5, 4, 0, 0, 261, 262, 7, 4, 0, 0, 262, 65, 1,
		0, 0, 0, 28, 69, 75, 80, 84, 92, 99, 104, 113, 116, 123, 129, 136, 142,
		148, 152, 156, 163, 175, 180, 185, 189, 194, 204, 216, 219, 223, 238, 247,
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
	JSPParserEOF                            = antlr.TokenEOF
	JSPParserJSP_COMMENT                    = 1
	JSPParserJSP_CONDITIONAL_COMMENT        = 2
	JSPParserSCRIPT_OPEN                    = 3
	JSPParserSTYLE_OPEN                     = 4
	JSPParserDTD                            = 5
	JSPParserCDATA                          = 6
	JSPParserWHITESPACES                    = 7
	JSPParserXML_DECLARATION                = 8
	JSPParserWHITESPACE_SKIP                = 9
	JSPParserCLOSE_TAG_BEGIN                = 10
	JSPParserTAG_BEGIN                      = 11
	JSPParserDIRECTIVE_BEGIN                = 12
	JSPParserDECLARATION_BEGIN              = 13
	JSPParserECHO_EXPRESSION_OPEN           = 14
	JSPParserSCRIPTLET_OPEN                 = 15
	JSPParserEXPRESSION_OPEN                = 16
	JSPParserQUOTE                          = 17
	JSPParserTAG_END                        = 18
	JSPParserEQUALS                         = 19
	JSPParserEL_EXPR_START                  = 20
	JSPParserJSP_STATIC_CONTENT_CHARS       = 21
	JSPParserJSP_END                        = 22
	JSPParserATTVAL_ATTRIBUTE               = 23
	JSPParserATTVAL_VALUE                   = 24
	JSPParserEL_EXPR_END                    = 25
	JSPParserDTD_PUBLIC                     = 26
	JSPParserDTD_SYSTEM                     = 27
	JSPParserDTD_WHITESPACE_SKIP            = 28
	JSPParserDTD_QUOTED                     = 29
	JSPParserDTD_IDENTIFIER                 = 30
	JSPParserBLOB_CLOSE                     = 31
	JSPParserBLOB_CONTENT                   = 32
	JSPParserJSPEXPR_EL_EXPR                = 33
	JSPParserJSPEXPR_CONTENT_CLOSE          = 34
	JSPParserJSP_JSTL_COLON                 = 35
	JSPParserTAG_SLASH_END                  = 36
	JSPParserTAG_CLOSE                      = 37
	JSPParserTAG_SLASH                      = 38
	JSPParserDIRECTIVE_END                  = 39
	JSPParserTAG_IDENTIFIER                 = 40
	JSPParserTAG_WHITESPACE                 = 41
	JSPParserSCRIPT_BODY                    = 42
	JSPParserSCRIPT_SHORT_BODY              = 43
	JSPParserSTYLE_BODY                     = 44
	JSPParserSTYLE_SHORT_BODY               = 45
	JSPParserATTVAL_EL_EXPR                 = 46
	JSPParserATTVAL_SINGLE_QUOTE_EXPRESSION = 47
	JSPParserATTVAL_DOUBLE_QUOTE_EXPRESSION = 48
	JSPParserEL_EXPR_CONTENT                = 49
)

// JSPParser rules.
const (
	JSPParserRULE_jspDocuments              = 0
	JSPParserRULE_jspDocument               = 1
	JSPParserRULE_jspStart                  = 2
	JSPParserRULE_jspElements               = 3
	JSPParserRULE_htmlMiscs                 = 4
	JSPParserRULE_jspScript                 = 5
	JSPParserRULE_htmlElement               = 6
	JSPParserRULE_htmlBegin                 = 7
	JSPParserRULE_htmlTag                   = 8
	JSPParserRULE_jspDirective              = 9
	JSPParserRULE_htmlContents              = 10
	JSPParserRULE_htmlContent               = 11
	JSPParserRULE_elExpression              = 12
	JSPParserRULE_htmlAttribute             = 13
	JSPParserRULE_htmlAttributeName         = 14
	JSPParserRULE_htmlAttributeValue        = 15
	JSPParserRULE_htmlAttributeValueElement = 16
	JSPParserRULE_htmlTagName               = 17
	JSPParserRULE_htmlChardata              = 18
	JSPParserRULE_htmlMisc                  = 19
	JSPParserRULE_htmlComment               = 20
	JSPParserRULE_xhtmlCDATA                = 21
	JSPParserRULE_dtd                       = 22
	JSPParserRULE_dtdElementName            = 23
	JSPParserRULE_publicId                  = 24
	JSPParserRULE_systemId                  = 25
	JSPParserRULE_xml                       = 26
	JSPParserRULE_jspScriptlet              = 27
	JSPParserRULE_jspExpression             = 28
	JSPParserRULE_scriptletStart            = 29
	JSPParserRULE_scriptletContent          = 30
	JSPParserRULE_javaScript                = 31
	JSPParserRULE_style                     = 32
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

func (s *JspDocumentsContext) AllJspStart() []IJspStartContext {
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

func (s *JspDocumentsContext) JspStart(i int) IJspStartContext {
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

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(69)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 0, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(66)
				p.JspStart()
			}

		}
		p.SetState(71)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 0, p.GetParserRuleContext())
	}
	p.SetState(73)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&1112510) != 0 {
		{
			p.SetState(72)
			p.JspDocument()
		}

		p.SetState(75)
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

func (s *JspDocumentContext) JspElements() IJspElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
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

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(80)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserXML_DECLARATION:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(77)
			p.Xml()
		}

	case JSPParserDTD:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(78)
			p.Dtd()
		}

	case JSPParserJSP_COMMENT, JSPParserJSP_CONDITIONAL_COMMENT, JSPParserSCRIPT_OPEN, JSPParserSTYLE_OPEN, JSPParserWHITESPACES, JSPParserTAG_BEGIN, JSPParserDIRECTIVE_BEGIN, JSPParserDECLARATION_BEGIN, JSPParserECHO_EXPRESSION_OPEN, JSPParserSCRIPTLET_OPEN, JSPParserEL_EXPR_START:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(79)
			p.JspElements()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
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

func (s *JspStartContext) JspScript() IJspScriptContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspScriptContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspScriptContext)
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

	p.SetState(84)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserDIRECTIVE_BEGIN, JSPParserDECLARATION_BEGIN, JSPParserECHO_EXPRESSION_OPEN, JSPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(82)
			p.JspScript()
		}

	case JSPParserWHITESPACES:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(83)
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

	// GetBeforeContent returns the beforeContent rule contexts.
	GetBeforeContent() IHtmlMiscsContext

	// GetAfterContent returns the afterContent rule contexts.
	GetAfterContent() IHtmlMiscsContext

	// SetBeforeContent sets the beforeContent rule contexts.
	SetBeforeContent(IHtmlMiscsContext)

	// SetAfterContent sets the afterContent rule contexts.
	SetAfterContent(IHtmlMiscsContext)

	// IsJspElementsContext differentiates from other interfaces.
	IsJspElementsContext()
}

type JspElementsContext struct {
	*antlr.BaseParserRuleContext
	parser        antlr.Parser
	beforeContent IHtmlMiscsContext
	afterContent  IHtmlMiscsContext
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

func (s *JspElementsContext) GetBeforeContent() IHtmlMiscsContext { return s.beforeContent }

func (s *JspElementsContext) GetAfterContent() IHtmlMiscsContext { return s.afterContent }

func (s *JspElementsContext) SetBeforeContent(v IHtmlMiscsContext) { s.beforeContent = v }

func (s *JspElementsContext) SetAfterContent(v IHtmlMiscsContext) { s.afterContent = v }

func (s *JspElementsContext) AllHtmlMiscs() []IHtmlMiscsContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlMiscsContext); ok {
			len++
		}
	}

	tst := make([]IHtmlMiscsContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlMiscsContext); ok {
			tst[i] = t.(IHtmlMiscsContext)
			i++
		}
	}

	return tst
}

func (s *JspElementsContext) HtmlMiscs(i int) IHtmlMiscsContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlMiscsContext); ok {
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

	return t.(IHtmlMiscsContext)
}

func (s *JspElementsContext) HtmlElement() IHtmlElementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlElementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlElementContext)
}

func (s *JspElementsContext) JspScript() IJspScriptContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspScriptContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspScriptContext)
}

func (s *JspElementsContext) JspExpression() IJspExpressionContext {
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

func (s *JspElementsContext) Style() IStyleContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStyleContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStyleContext)
}

func (s *JspElementsContext) JavaScript() IJavaScriptContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJavaScriptContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJavaScriptContext)
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

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(86)

		var _x = p.HtmlMiscs()

		localctx.(*JspElementsContext).beforeContent = _x
	}
	p.SetState(92)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 4, p.GetParserRuleContext()) {
	case 1:
		{
			p.SetState(87)
			p.HtmlElement()
		}

	case 2:
		{
			p.SetState(88)
			p.JspScript()
		}

	case 3:
		{
			p.SetState(89)
			p.JspExpression()
		}

	case 4:
		{
			p.SetState(90)
			p.Style()
		}

	case 5:
		{
			p.SetState(91)
			p.JavaScript()
		}

	}
	{
		p.SetState(94)

		var _x = p.HtmlMiscs()

		localctx.(*JspElementsContext).afterContent = _x
	}

	return localctx
}

// IHtmlMiscsContext is an interface to support dynamic dispatch.
type IHtmlMiscsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlMiscsContext differentiates from other interfaces.
	IsHtmlMiscsContext()
}

type HtmlMiscsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlMiscsContext() *HtmlMiscsContext {
	var p = new(HtmlMiscsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlMiscs
	return p
}

func (*HtmlMiscsContext) IsHtmlMiscsContext() {}

func NewHtmlMiscsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlMiscsContext {
	var p = new(HtmlMiscsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlMiscs

	return p
}

func (s *HtmlMiscsContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlMiscsContext) AllHtmlMisc() []IHtmlMiscContext {
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

func (s *HtmlMiscsContext) HtmlMisc(i int) IHtmlMiscContext {
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

func (s *HtmlMiscsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlMiscsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlMiscsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlMiscs(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlMiscs() (localctx IHtmlMiscsContext) {
	this := p
	_ = this

	localctx = NewHtmlMiscsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, JSPParserRULE_htmlMiscs)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	p.SetState(99)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 5, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(96)
				p.HtmlMisc()
			}

		}
		p.SetState(101)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 5, p.GetParserRuleContext())
	}

	return localctx
}

// IJspScriptContext is an interface to support dynamic dispatch.
type IJspScriptContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsJspScriptContext differentiates from other interfaces.
	IsJspScriptContext()
}

type JspScriptContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyJspScriptContext() *JspScriptContext {
	var p = new(JspScriptContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_jspScript
	return p
}

func (*JspScriptContext) IsJspScriptContext() {}

func NewJspScriptContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *JspScriptContext {
	var p = new(JspScriptContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_jspScript

	return p
}

func (s *JspScriptContext) GetParser() antlr.Parser { return s.parser }

func (s *JspScriptContext) JspDirective() IJspDirectiveContext {
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

func (s *JspScriptContext) JspScriptlet() IJspScriptletContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspScriptletContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspScriptletContext)
}

func (s *JspScriptContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspScriptContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *JspScriptContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJspScript(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) JspScript() (localctx IJspScriptContext) {
	this := p
	_ = this

	localctx = NewJspScriptContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, JSPParserRULE_jspScript)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(104)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserDIRECTIVE_BEGIN:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(102)
			p.JspDirective()
		}

	case JSPParserDECLARATION_BEGIN, JSPParserECHO_EXPRESSION_OPEN, JSPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(103)
			p.JspScriptlet()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IHtmlElementContext is an interface to support dynamic dispatch.
type IHtmlElementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlElementContext differentiates from other interfaces.
	IsHtmlElementContext()
}

type HtmlElementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlElementContext() *HtmlElementContext {
	var p = new(HtmlElementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlElement
	return p
}

func (*HtmlElementContext) IsHtmlElementContext() {}

func NewHtmlElementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlElementContext {
	var p = new(HtmlElementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlElement

	return p
}

func (s *HtmlElementContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlElementContext) HtmlBegin() IHtmlBeginContext {
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

func (s *HtmlElementContext) AllTAG_CLOSE() []antlr.TerminalNode {
	return s.GetTokens(JSPParserTAG_CLOSE)
}

func (s *HtmlElementContext) TAG_CLOSE(i int) antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_CLOSE, i)
}

func (s *HtmlElementContext) TAG_SLASH_END() antlr.TerminalNode {
	return s.GetToken(JSPParserTAG_SLASH_END, 0)
}

func (s *HtmlElementContext) HtmlContents() IHtmlContentsContext {
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

func (s *HtmlElementContext) CLOSE_TAG_BEGIN() antlr.TerminalNode {
	return s.GetToken(JSPParserCLOSE_TAG_BEGIN, 0)
}

func (s *HtmlElementContext) HtmlTag() IHtmlTagContext {
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

func (s *HtmlElementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlElementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlElementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlElement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlElement() (localctx IHtmlElementContext) {
	this := p
	_ = this

	localctx = NewHtmlElementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, JSPParserRULE_htmlElement)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(106)
		p.HtmlBegin()
	}
	p.SetState(116)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserTAG_CLOSE:
		{
			p.SetState(107)
			p.Match(JSPParserTAG_CLOSE)
		}
		p.SetState(113)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 7, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(108)
				p.HtmlContents()
			}
			{
				p.SetState(109)
				p.Match(JSPParserCLOSE_TAG_BEGIN)
			}
			{
				p.SetState(110)
				p.HtmlTag()
			}
			{
				p.SetState(111)
				p.Match(JSPParserTAG_CLOSE)
			}

		}

	case JSPParserTAG_SLASH_END:
		{
			p.SetState(115)
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
	p.EnterRule(localctx, 14, JSPParserRULE_htmlBegin)
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

	for _la == JSPParserECHO_EXPRESSION_OPEN || _la == JSPParserTAG_IDENTIFIER {
		{
			p.SetState(120)
			p.HtmlAttribute()
		}

		p.SetState(125)
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
	p.EnterRule(localctx, 16, JSPParserRULE_htmlTag)
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
		p.SetState(126)
		p.HtmlTagName()
	}
	p.SetState(129)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == JSPParserJSP_JSTL_COLON {
		{
			p.SetState(127)
			p.Match(JSPParserJSP_JSTL_COLON)
		}
		{
			p.SetState(128)
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
	p.EnterRule(localctx, 18, JSPParserRULE_jspDirective)
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
		p.SetState(131)
		p.Match(JSPParserDIRECTIVE_BEGIN)
	}
	{
		p.SetState(132)
		p.HtmlTagName()
	}
	p.SetState(136)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 11, p.GetParserRuleContext())

	for _alt != 1 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1+1 {
			{
				p.SetState(133)
				p.HtmlAttribute()
			}

		}
		p.SetState(138)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 11, p.GetParserRuleContext())
	}
	p.SetState(142)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == JSPParserTAG_WHITESPACE {
		{
			p.SetState(139)
			p.Match(JSPParserTAG_WHITESPACE)
		}

		p.SetState(144)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(145)
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
	p.EnterRule(localctx, 20, JSPParserRULE_htmlContents)
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
	p.SetState(148)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 13, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(147)
			p.HtmlChardata()
		}

	}
	p.SetState(156)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&1112286) != 0 {
		{
			p.SetState(150)
			p.HtmlContent()
		}
		p.SetState(152)
		p.GetErrorHandler().Sync(p)

		if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 14, p.GetParserRuleContext()) == 1 {
			{
				p.SetState(151)
				p.HtmlChardata()
			}

		}

		p.SetState(158)
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

func (s *HtmlContentContext) ElExpression() IElExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElExpressionContext)
}

func (s *HtmlContentContext) JspElements() IJspElementsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspElementsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspElementsContext)
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
	p.EnterRule(localctx, 22, JSPParserRULE_htmlContent)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(163)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 16, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(159)
			p.ElExpression()
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(160)
			p.JspElements()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(161)
			p.XhtmlCDATA()
		}

	case 4:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(162)
			p.HtmlComment()
		}

	}

	return localctx
}

// IElExpressionContext is an interface to support dynamic dispatch.
type IElExpressionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsElExpressionContext differentiates from other interfaces.
	IsElExpressionContext()
}

type ElExpressionContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyElExpressionContext() *ElExpressionContext {
	var p = new(ElExpressionContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_elExpression
	return p
}

func (*ElExpressionContext) IsElExpressionContext() {}

func NewElExpressionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ElExpressionContext {
	var p = new(ElExpressionContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_elExpression

	return p
}

func (s *ElExpressionContext) GetParser() antlr.Parser { return s.parser }

func (s *ElExpressionContext) EL_EXPR_START() antlr.TerminalNode {
	return s.GetToken(JSPParserEL_EXPR_START, 0)
}

func (s *ElExpressionContext) EL_EXPR_CONTENT() antlr.TerminalNode {
	return s.GetToken(JSPParserEL_EXPR_CONTENT, 0)
}

func (s *ElExpressionContext) EL_EXPR_END() antlr.TerminalNode {
	return s.GetToken(JSPParserEL_EXPR_END, 0)
}

func (s *ElExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ElExpressionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ElExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitElExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) ElExpression() (localctx IElExpressionContext) {
	this := p
	_ = this

	localctx = NewElExpressionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, JSPParserRULE_elExpression)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(165)
		p.Match(JSPParserEL_EXPR_START)
	}
	{
		p.SetState(166)
		p.Match(JSPParserEL_EXPR_CONTENT)
	}
	{
		p.SetState(167)
		p.Match(JSPParserEL_EXPR_END)
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

func (s *HtmlAttributeContext) CopyFrom(ctx *HtmlAttributeContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *HtmlAttributeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlAttributeContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type PureHTMLAttributeContext struct {
	*HtmlAttributeContext
}

func NewPureHTMLAttributeContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *PureHTMLAttributeContext {
	var p = new(PureHTMLAttributeContext)

	p.HtmlAttributeContext = NewEmptyHtmlAttributeContext()
	p.parser = parser
	p.CopyFrom(ctx.(*HtmlAttributeContext))

	return p
}

func (s *PureHTMLAttributeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PureHTMLAttributeContext) HtmlAttributeName() IHtmlAttributeNameContext {
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

func (s *PureHTMLAttributeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitPureHTMLAttribute(s)

	default:
		return t.VisitChildren(s)
	}
}

type EqualHTMLAttributeContext struct {
	*HtmlAttributeContext
}

func NewEqualHTMLAttributeContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *EqualHTMLAttributeContext {
	var p = new(EqualHTMLAttributeContext)

	p.HtmlAttributeContext = NewEmptyHtmlAttributeContext()
	p.parser = parser
	p.CopyFrom(ctx.(*HtmlAttributeContext))

	return p
}

func (s *EqualHTMLAttributeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *EqualHTMLAttributeContext) HtmlAttributeName() IHtmlAttributeNameContext {
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

func (s *EqualHTMLAttributeContext) EQUALS() antlr.TerminalNode {
	return s.GetToken(JSPParserEQUALS, 0)
}

func (s *EqualHTMLAttributeContext) HtmlAttributeValue() IHtmlAttributeValueContext {
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

func (s *EqualHTMLAttributeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitEqualHTMLAttribute(s)

	default:
		return t.VisitChildren(s)
	}
}

type JSPExpressionAttributeContext struct {
	*HtmlAttributeContext
}

func NewJSPExpressionAttributeContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *JSPExpressionAttributeContext {
	var p = new(JSPExpressionAttributeContext)

	p.HtmlAttributeContext = NewEmptyHtmlAttributeContext()
	p.parser = parser
	p.CopyFrom(ctx.(*HtmlAttributeContext))

	return p
}

func (s *JSPExpressionAttributeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JSPExpressionAttributeContext) JspExpression() IJspExpressionContext {
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

func (s *JSPExpressionAttributeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJSPExpressionAttribute(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlAttribute() (localctx IHtmlAttributeContext) {
	this := p
	_ = this

	localctx = NewHtmlAttributeContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, JSPParserRULE_htmlAttribute)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(175)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 17, p.GetParserRuleContext()) {
	case 1:
		localctx = NewEqualHTMLAttributeContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(169)
			p.HtmlAttributeName()
		}
		{
			p.SetState(170)
			p.Match(JSPParserEQUALS)
		}
		{
			p.SetState(171)
			p.HtmlAttributeValue()
		}

	case 2:
		localctx = NewPureHTMLAttributeContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(173)
			p.HtmlAttributeName()
		}

	case 3:
		localctx = NewJSPExpressionAttributeContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(174)
			p.JspExpression()
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
	p.EnterRule(localctx, 28, JSPParserRULE_htmlAttributeName)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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

func (s *HtmlAttributeValueContext) AllHtmlAttributeValueElement() []IHtmlAttributeValueElementContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlAttributeValueElementContext); ok {
			len++
		}
	}

	tst := make([]IHtmlAttributeValueElementContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlAttributeValueElementContext); ok {
			tst[i] = t.(IHtmlAttributeValueElementContext)
			i++
		}
	}

	return tst
}

func (s *HtmlAttributeValueContext) HtmlAttributeValueElement(i int) IHtmlAttributeValueElementContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlAttributeValueElementContext); ok {
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

	return t.(IHtmlAttributeValueElementContext)
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
	p.EnterRule(localctx, 30, JSPParserRULE_htmlAttributeValue)
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
	p.SetState(180)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 18, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(179)
			p.Match(JSPParserQUOTE)
		}

	}
	p.SetState(185)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 19, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(182)
				p.HtmlAttributeValueElement()
			}

		}
		p.SetState(187)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 19, p.GetParserRuleContext())
	}
	p.SetState(189)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == JSPParserQUOTE {
		{
			p.SetState(188)
			p.Match(JSPParserQUOTE)
		}

	}

	return localctx
}

// IHtmlAttributeValueElementContext is an interface to support dynamic dispatch.
type IHtmlAttributeValueElementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsHtmlAttributeValueElementContext differentiates from other interfaces.
	IsHtmlAttributeValueElementContext()
}

type HtmlAttributeValueElementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlAttributeValueElementContext() *HtmlAttributeValueElementContext {
	var p = new(HtmlAttributeValueElementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_htmlAttributeValueElement
	return p
}

func (*HtmlAttributeValueElementContext) IsHtmlAttributeValueElementContext() {}

func NewHtmlAttributeValueElementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlAttributeValueElementContext {
	var p = new(HtmlAttributeValueElementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_htmlAttributeValueElement

	return p
}

func (s *HtmlAttributeValueElementContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlAttributeValueElementContext) ATTVAL_ATTRIBUTE() antlr.TerminalNode {
	return s.GetToken(JSPParserATTVAL_ATTRIBUTE, 0)
}

func (s *HtmlAttributeValueElementContext) JspExpression() IJspExpressionContext {
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

func (s *HtmlAttributeValueElementContext) ElExpression() IElExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElExpressionContext)
}

func (s *HtmlAttributeValueElementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlAttributeValueElementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlAttributeValueElementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitHtmlAttributeValueElement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) HtmlAttributeValueElement() (localctx IHtmlAttributeValueElementContext) {
	this := p
	_ = this

	localctx = NewHtmlAttributeValueElementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 32, JSPParserRULE_htmlAttributeValueElement)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(194)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case JSPParserATTVAL_ATTRIBUTE:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(191)
			p.Match(JSPParserATTVAL_ATTRIBUTE)
		}

	case JSPParserECHO_EXPRESSION_OPEN:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(192)
			p.JspExpression()
		}

	case JSPParserEL_EXPR_START:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(193)
			p.ElExpression()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
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
	p.EnterRule(localctx, 34, JSPParserRULE_htmlTagName)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(196)
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
	p.EnterRule(localctx, 36, JSPParserRULE_htmlChardata)
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
		p.SetState(198)
		_la = p.GetTokenStream().LA(1)

		if !(_la == JSPParserWHITESPACES || _la == JSPParserJSP_STATIC_CONTENT_CHARS) {
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

func (s *HtmlMiscContext) ElExpression() IElExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElExpressionContext)
}

func (s *HtmlMiscContext) JspScriptlet() IJspScriptletContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IJspScriptletContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IJspScriptletContext)
}

func (s *HtmlMiscContext) WHITESPACES() antlr.TerminalNode {
	return s.GetToken(JSPParserWHITESPACES, 0)
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
	p.EnterRule(localctx, 38, JSPParserRULE_htmlMisc)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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

	switch p.GetTokenStream().LA(1) {
	case JSPParserJSP_COMMENT, JSPParserJSP_CONDITIONAL_COMMENT:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(200)
			p.HtmlComment()
		}

	case JSPParserEL_EXPR_START:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(201)
			p.ElExpression()
		}

	case JSPParserDECLARATION_BEGIN, JSPParserECHO_EXPRESSION_OPEN, JSPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(202)
			p.JspScriptlet()
		}

	case JSPParserWHITESPACES:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(203)
			p.Match(JSPParserWHITESPACES)
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

func (s *HtmlCommentContext) JSP_COMMENT() antlr.TerminalNode {
	return s.GetToken(JSPParserJSP_COMMENT, 0)
}

func (s *HtmlCommentContext) JSP_CONDITIONAL_COMMENT() antlr.TerminalNode {
	return s.GetToken(JSPParserJSP_CONDITIONAL_COMMENT, 0)
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
	p.EnterRule(localctx, 40, JSPParserRULE_htmlComment)
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
		p.SetState(206)
		_la = p.GetTokenStream().LA(1)

		if !(_la == JSPParserJSP_COMMENT || _la == JSPParserJSP_CONDITIONAL_COMMENT) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
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
	p.EnterRule(localctx, 42, JSPParserRULE_xhtmlCDATA)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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

func (s *DtdContext) AllPublicId() []IPublicIdContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IPublicIdContext); ok {
			len++
		}
	}

	tst := make([]IPublicIdContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IPublicIdContext); ok {
			tst[i] = t.(IPublicIdContext)
			i++
		}
	}

	return tst
}

func (s *DtdContext) PublicId(i int) IPublicIdContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IPublicIdContext); ok {
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

	return t.(IPublicIdContext)
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
	p.EnterRule(localctx, 44, JSPParserRULE_dtd)
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
		p.Match(JSPParserDTD)
	}
	{
		p.SetState(211)
		p.DtdElementName()
	}
	p.SetState(219)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == JSPParserDTD_PUBLIC {
		{
			p.SetState(212)
			p.Match(JSPParserDTD_PUBLIC)
		}
		p.SetState(216)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == JSPParserDTD_QUOTED {
			{
				p.SetState(213)
				p.PublicId()
			}

			p.SetState(218)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}

	}
	p.SetState(223)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == JSPParserDTD_SYSTEM {
		{
			p.SetState(221)
			p.Match(JSPParserDTD_SYSTEM)
		}
		{
			p.SetState(222)
			p.SystemId()
		}

	}
	{
		p.SetState(225)
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
	p.EnterRule(localctx, 46, JSPParserRULE_dtdElementName)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(227)
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
	p.EnterRule(localctx, 48, JSPParserRULE_publicId)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(229)
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
	p.EnterRule(localctx, 50, JSPParserRULE_systemId)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(231)
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
	p.EnterRule(localctx, 52, JSPParserRULE_xml)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(233)
		p.Match(JSPParserXML_DECLARATION)
	}
	{
		p.SetState(234)

		var _x = p.HtmlTagName()

		localctx.(*XmlContext).name = _x
	}
	p.SetState(238)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 26, p.GetParserRuleContext())

	for _alt != 1 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1+1 {
			{
				p.SetState(235)

				var _x = p.HtmlAttribute()

				localctx.(*XmlContext)._htmlAttribute = _x
			}
			localctx.(*XmlContext).atts = append(localctx.(*XmlContext).atts, localctx.(*XmlContext)._htmlAttribute)

		}
		p.SetState(240)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 26, p.GetParserRuleContext())
	}
	{
		p.SetState(241)
		p.Match(JSPParserTAG_END)
	}

	return localctx
}

// IJspScriptletContext is an interface to support dynamic dispatch.
type IJspScriptletContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsJspScriptletContext differentiates from other interfaces.
	IsJspScriptletContext()
}

type JspScriptletContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyJspScriptletContext() *JspScriptletContext {
	var p = new(JspScriptletContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_jspScriptlet
	return p
}

func (*JspScriptletContext) IsJspScriptletContext() {}

func NewJspScriptletContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *JspScriptletContext {
	var p = new(JspScriptletContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_jspScriptlet

	return p
}

func (s *JspScriptletContext) GetParser() antlr.Parser { return s.parser }

func (s *JspScriptletContext) ScriptletStart() IScriptletStartContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IScriptletStartContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IScriptletStartContext)
}

func (s *JspScriptletContext) ScriptletContent() IScriptletContentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IScriptletContentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IScriptletContentContext)
}

func (s *JspScriptletContext) JspExpression() IJspExpressionContext {
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

func (s *JspScriptletContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JspScriptletContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *JspScriptletContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJspScriptlet(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) JspScriptlet() (localctx IJspScriptletContext) {
	this := p
	_ = this

	localctx = NewJspScriptletContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 54, JSPParserRULE_jspScriptlet)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	case JSPParserDECLARATION_BEGIN, JSPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(243)
			p.ScriptletStart()
		}
		{
			p.SetState(244)
			p.ScriptletContent()
		}

	case JSPParserECHO_EXPRESSION_OPEN:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(246)
			p.JspExpression()
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

func (s *JspExpressionContext) ECHO_EXPRESSION_OPEN() antlr.TerminalNode {
	return s.GetToken(JSPParserECHO_EXPRESSION_OPEN, 0)
}

func (s *JspExpressionContext) ScriptletContent() IScriptletContentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IScriptletContentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IScriptletContentContext)
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
	p.EnterRule(localctx, 56, JSPParserRULE_jspExpression)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.Match(JSPParserECHO_EXPRESSION_OPEN)
	}
	{
		p.SetState(250)
		p.ScriptletContent()
	}

	return localctx
}

// IScriptletStartContext is an interface to support dynamic dispatch.
type IScriptletStartContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsScriptletStartContext differentiates from other interfaces.
	IsScriptletStartContext()
}

type ScriptletStartContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyScriptletStartContext() *ScriptletStartContext {
	var p = new(ScriptletStartContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_scriptletStart
	return p
}

func (*ScriptletStartContext) IsScriptletStartContext() {}

func NewScriptletStartContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ScriptletStartContext {
	var p = new(ScriptletStartContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_scriptletStart

	return p
}

func (s *ScriptletStartContext) GetParser() antlr.Parser { return s.parser }

func (s *ScriptletStartContext) SCRIPTLET_OPEN() antlr.TerminalNode {
	return s.GetToken(JSPParserSCRIPTLET_OPEN, 0)
}

func (s *ScriptletStartContext) DECLARATION_BEGIN() antlr.TerminalNode {
	return s.GetToken(JSPParserDECLARATION_BEGIN, 0)
}

func (s *ScriptletStartContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ScriptletStartContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ScriptletStartContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitScriptletStart(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) ScriptletStart() (localctx IScriptletStartContext) {
	this := p
	_ = this

	localctx = NewScriptletStartContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 58, JSPParserRULE_scriptletStart)
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
		_la = p.GetTokenStream().LA(1)

		if !(_la == JSPParserDECLARATION_BEGIN || _la == JSPParserSCRIPTLET_OPEN) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IScriptletContentContext is an interface to support dynamic dispatch.
type IScriptletContentContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsScriptletContentContext differentiates from other interfaces.
	IsScriptletContentContext()
}

type ScriptletContentContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyScriptletContentContext() *ScriptletContentContext {
	var p = new(ScriptletContentContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_scriptletContent
	return p
}

func (*ScriptletContentContext) IsScriptletContentContext() {}

func NewScriptletContentContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ScriptletContentContext {
	var p = new(ScriptletContentContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_scriptletContent

	return p
}

func (s *ScriptletContentContext) GetParser() antlr.Parser { return s.parser }

func (s *ScriptletContentContext) BLOB_CONTENT() antlr.TerminalNode {
	return s.GetToken(JSPParserBLOB_CONTENT, 0)
}

func (s *ScriptletContentContext) BLOB_CLOSE() antlr.TerminalNode {
	return s.GetToken(JSPParserBLOB_CLOSE, 0)
}

func (s *ScriptletContentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ScriptletContentContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ScriptletContentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitScriptletContent(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) ScriptletContent() (localctx IScriptletContentContext) {
	this := p
	_ = this

	localctx = NewScriptletContentContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 60, JSPParserRULE_scriptletContent)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.Match(JSPParserBLOB_CONTENT)
	}
	{
		p.SetState(255)
		p.Match(JSPParserBLOB_CLOSE)
	}

	return localctx
}

// IJavaScriptContext is an interface to support dynamic dispatch.
type IJavaScriptContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsJavaScriptContext differentiates from other interfaces.
	IsJavaScriptContext()
}

type JavaScriptContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyJavaScriptContext() *JavaScriptContext {
	var p = new(JavaScriptContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_javaScript
	return p
}

func (*JavaScriptContext) IsJavaScriptContext() {}

func NewJavaScriptContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *JavaScriptContext {
	var p = new(JavaScriptContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_javaScript

	return p
}

func (s *JavaScriptContext) GetParser() antlr.Parser { return s.parser }

func (s *JavaScriptContext) SCRIPT_OPEN() antlr.TerminalNode {
	return s.GetToken(JSPParserSCRIPT_OPEN, 0)
}

func (s *JavaScriptContext) SCRIPT_BODY() antlr.TerminalNode {
	return s.GetToken(JSPParserSCRIPT_BODY, 0)
}

func (s *JavaScriptContext) SCRIPT_SHORT_BODY() antlr.TerminalNode {
	return s.GetToken(JSPParserSCRIPT_SHORT_BODY, 0)
}

func (s *JavaScriptContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *JavaScriptContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *JavaScriptContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitJavaScript(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) JavaScript() (localctx IJavaScriptContext) {
	this := p
	_ = this

	localctx = NewJavaScriptContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 62, JSPParserRULE_javaScript)
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
		p.SetState(257)
		p.Match(JSPParserSCRIPT_OPEN)
	}
	{
		p.SetState(258)
		_la = p.GetTokenStream().LA(1)

		if !(_la == JSPParserSCRIPT_BODY || _la == JSPParserSCRIPT_SHORT_BODY) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IStyleContext is an interface to support dynamic dispatch.
type IStyleContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStyleContext differentiates from other interfaces.
	IsStyleContext()
}

type StyleContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStyleContext() *StyleContext {
	var p = new(StyleContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = JSPParserRULE_style
	return p
}

func (*StyleContext) IsStyleContext() {}

func NewStyleContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StyleContext {
	var p = new(StyleContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = JSPParserRULE_style

	return p
}

func (s *StyleContext) GetParser() antlr.Parser { return s.parser }

func (s *StyleContext) STYLE_OPEN() antlr.TerminalNode {
	return s.GetToken(JSPParserSTYLE_OPEN, 0)
}

func (s *StyleContext) STYLE_BODY() antlr.TerminalNode {
	return s.GetToken(JSPParserSTYLE_BODY, 0)
}

func (s *StyleContext) STYLE_SHORT_BODY() antlr.TerminalNode {
	return s.GetToken(JSPParserSTYLE_SHORT_BODY, 0)
}

func (s *StyleContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StyleContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StyleContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JSPParserVisitor:
		return t.VisitStyle(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JSPParser) Style() (localctx IStyleContext) {
	this := p
	_ = this

	localctx = NewStyleContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 64, JSPParserRULE_style)
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
		p.SetState(260)
		p.Match(JSPParserSTYLE_OPEN)
	}
	{
		p.SetState(261)
		_la = p.GetTokenStream().LA(1)

		if !(_la == JSPParserSTYLE_BODY || _la == JSPParserSTYLE_SHORT_BODY) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}
