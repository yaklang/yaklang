// Code generated from ./SyntaxFlowLexer.g4 by ANTLR 4.13.2. DO NOT EDIT.

package sf

import (
	"fmt"
	"github.com/yaklang/antlr/v4"
	"sync"
	"unicode"
)

// Suppress unused import error
var _ = fmt.Printf
var _ = sync.Once{}
var _ = unicode.IsLetter

type SyntaxFlowLexer struct {
	SyntaxFlowBaseLexer
	channelNames []string
	modeNames    []string
	// TODO: EOF string
}

var SyntaxFlowLexerLexerStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	ChannelNames           []string
	ModeNames              []string
	LiteralNames           []string
	SymbolicNames          []string
	RuleNames              []string
	PredictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func syntaxflowlexerLexerInit() {
	staticData := &SyntaxFlowLexerLexerStaticData
	staticData.ChannelNames = []string{
		"DEFAULT_TOKEN_CHANNEL", "HIDDEN",
	}
	staticData.ModeNames = []string{
		"DEFAULT_MODE", "HereDocIdentifier", "CRLFHereDoc", "LFHereDoc",
	}
	staticData.LiteralNames = []string{
		"", "'==>'", "'...'", "'%%'", "'..'", "'<='", "'>='", "'>>'", "'=>'",
		"'=='", "'=~'", "'!~'", "'&&'", "'||'", "'!='", "'${'", "';'", "'?{'",
		"'-{'", "'->'", "'}->'", "'-->'", "'#{'", "'#>'", "'#->'", "'>'", "'.'",
		"'<<<'", "'<'", "'='", "'+'", "'&'", "'?'", "'('", "','", "')'", "'['",
		"']'", "'{'", "'}'", "'#'", "'$'", "':'", "'%'", "'!'", "", "'*'", "'-'",
		"'as'", "'`'", "'''", "'\"'", "", "'\\n'", "", "", "", "", "", "'str'",
		"'list'", "'dict'", "", "'bool'", "", "'alert'", "'check'", "'then'",
		"", "'else'", "'type'", "'in'", "'call'", "", "", "'phi'", "", "", "'opcode'",
		"'have'", "'any'", "'not'", "'inside'", "'not_inside'", "'for'", "'version_in'",
		"'r'", "'g'", "'e'",
	}
	staticData.SymbolicNames = []string{
		"", "DeepFilter", "Deep", "Percent", "DeepDot", "LtEq", "GtEq", "DoubleGt",
		"Filter", "EqEq", "RegexpMatch", "NotRegexpMatch", "And", "Or", "NotEq",
		"DollarBraceOpen", "Semicolon", "ConditionStart", "DeepNextStart", "UseStart",
		"DeepNextEnd", "DeepNext", "TopDefStart", "DefStart", "TopDef", "Gt",
		"Dot", "StartNowDoc", "Lt", "Eq", "Add", "Amp", "Question", "OpenParen",
		"Comma", "CloseParen", "ListSelectOpen", "ListSelectClose", "MapBuilderOpen",
		"MapBuilderClose", "ListStart", "DollarOutput", "Colon", "Search", "Bang",
		"GlobPattern", "Star", "Minus", "As", "Backtick", "SingleQuote", "DoubleQuote",
		"LineComment", "BreakLine", "WhiteSpace", "Number", "OctalNumber", "BinaryNumber",
		"HexNumber", "StringType", "ListType", "DictType", "NumberType", "BoolType",
		"BoolLiteral", "Alert", "Check", "Then", "Desc", "Else", "Type", "In",
		"Call", "Function", "Constant", "Phi", "FormalParam", "Return", "Opcode",
		"Have", "HaveAny", "Not", "Inside", "NotInside", "For", "VersionIn",
		"ConstSearchModePrefixRegexp", "ConstSearchModePrefixGlob", "ConstSearchModePrefixExact",
		"Identifier", "IdentifierChar", "QuotedStringLiteral", "RegexpLiteral",
		"WS", "HereDocIdentifierName", "CRLFHereDocIdentifierBreak", "LFHereDocIdentifierBreak",
		"CRLFEndDoc", "CRLFHereDocText", "LFEndDoc", "LFHereDocText",
	}
	staticData.RuleNames = []string{
		"DeepFilter", "Deep", "Percent", "DeepDot", "LtEq", "GtEq", "DoubleGt",
		"Filter", "EqEq", "RegexpMatch", "NotRegexpMatch", "And", "Or", "NotEq",
		"DollarBraceOpen", "Semicolon", "ConditionStart", "DeepNextStart", "UseStart",
		"DeepNextEnd", "DeepNext", "TopDefStart", "DefStart", "TopDef", "Gt",
		"Dot", "StartNowDoc", "Lt", "Eq", "Add", "Amp", "Question", "OpenParen",
		"Comma", "CloseParen", "ListSelectOpen", "ListSelectClose", "MapBuilderOpen",
		"MapBuilderClose", "ListStart", "DollarOutput", "Colon", "Search", "Bang",
		"GlobPattern", "Star", "Minus", "As", "Backtick", "SingleQuote", "DoubleQuote",
		"LineComment", "BreakLine", "WhiteSpace", "Number", "OctalNumber", "BinaryNumber",
		"HexNumber", "StringType", "ListType", "DictType", "NumberType", "BoolType",
		"BoolLiteral", "Alert", "Check", "Then", "Desc", "Else", "Type", "In",
		"Call", "Function", "Constant", "Phi", "FormalParam", "Return", "Opcode",
		"Have", "HaveAny", "Not", "Inside", "NotInside", "For", "VersionIn",
		"ConstSearchModePrefixRegexp", "ConstSearchModePrefixGlob", "ConstSearchModePrefixExact",
		"Identifier", "IdentifierChar", "QuotedStringLiteral", "RegexpLiteral",
		"WS", "HereDocIdentifierName", "CRLFHereDocIdentifierBreak", "LFHereDocIdentifierBreak",
		"CRLFEndDoc", "CRLFHereDocText", "LFEndDoc", "LFHereDocText", "NameString",
		"IdentifierCharStart", "GlobPatternChar", "HexDigit", "Digit", "OctalDigit",
		"RegexpLiteralChar",
	}
	staticData.PredictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 0, 100, 732, 6, -1, 6, -1, 6, -1, 6, -1, 2, 0, 7, 0, 2, 1, 7, 1, 2,
		2, 7, 2, 2, 3, 7, 3, 2, 4, 7, 4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2,
		8, 7, 8, 2, 9, 7, 9, 2, 10, 7, 10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7,
		13, 2, 14, 7, 14, 2, 15, 7, 15, 2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18,
		2, 19, 7, 19, 2, 20, 7, 20, 2, 21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2,
		24, 7, 24, 2, 25, 7, 25, 2, 26, 7, 26, 2, 27, 7, 27, 2, 28, 7, 28, 2, 29,
		7, 29, 2, 30, 7, 30, 2, 31, 7, 31, 2, 32, 7, 32, 2, 33, 7, 33, 2, 34, 7,
		34, 2, 35, 7, 35, 2, 36, 7, 36, 2, 37, 7, 37, 2, 38, 7, 38, 2, 39, 7, 39,
		2, 40, 7, 40, 2, 41, 7, 41, 2, 42, 7, 42, 2, 43, 7, 43, 2, 44, 7, 44, 2,
		45, 7, 45, 2, 46, 7, 46, 2, 47, 7, 47, 2, 48, 7, 48, 2, 49, 7, 49, 2, 50,
		7, 50, 2, 51, 7, 51, 2, 52, 7, 52, 2, 53, 7, 53, 2, 54, 7, 54, 2, 55, 7,
		55, 2, 56, 7, 56, 2, 57, 7, 57, 2, 58, 7, 58, 2, 59, 7, 59, 2, 60, 7, 60,
		2, 61, 7, 61, 2, 62, 7, 62, 2, 63, 7, 63, 2, 64, 7, 64, 2, 65, 7, 65, 2,
		66, 7, 66, 2, 67, 7, 67, 2, 68, 7, 68, 2, 69, 7, 69, 2, 70, 7, 70, 2, 71,
		7, 71, 2, 72, 7, 72, 2, 73, 7, 73, 2, 74, 7, 74, 2, 75, 7, 75, 2, 76, 7,
		76, 2, 77, 7, 77, 2, 78, 7, 78, 2, 79, 7, 79, 2, 80, 7, 80, 2, 81, 7, 81,
		2, 82, 7, 82, 2, 83, 7, 83, 2, 84, 7, 84, 2, 85, 7, 85, 2, 86, 7, 86, 2,
		87, 7, 87, 2, 88, 7, 88, 2, 89, 7, 89, 2, 90, 7, 90, 2, 91, 7, 91, 2, 92,
		7, 92, 2, 93, 7, 93, 2, 94, 7, 94, 2, 95, 7, 95, 2, 96, 7, 96, 2, 97, 7,
		97, 2, 98, 7, 98, 2, 99, 7, 99, 2, 100, 7, 100, 2, 101, 7, 101, 2, 102,
		7, 102, 2, 103, 7, 103, 2, 104, 7, 104, 2, 105, 7, 105, 2, 106, 7, 106,
		1, 0, 1, 0, 1, 0, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 2, 1, 3,
		1, 3, 1, 3, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4, 1, 5, 1, 5, 1, 5, 1, 5, 1, 5,
		1, 6, 1, 6, 1, 6, 1, 7, 1, 7, 1, 7, 1, 8, 1, 8, 1, 8, 1, 9, 1, 9, 1, 9,
		1, 10, 1, 10, 1, 10, 1, 11, 1, 11, 1, 11, 1, 12, 1, 12, 1, 12, 1, 13, 1,
		13, 1, 13, 1, 14, 1, 14, 1, 14, 1, 15, 1, 15, 1, 16, 1, 16, 1, 16, 1, 17,
		1, 17, 1, 17, 1, 18, 1, 18, 1, 18, 1, 19, 1, 19, 1, 19, 1, 19, 1, 20, 1,
		20, 1, 20, 1, 20, 1, 21, 1, 21, 1, 21, 1, 22, 1, 22, 1, 22, 1, 23, 1, 23,
		1, 23, 1, 23, 1, 24, 1, 24, 1, 25, 1, 25, 1, 26, 1, 26, 1, 26, 1, 26, 1,
		26, 1, 26, 1, 27, 1, 27, 1, 28, 1, 28, 1, 29, 1, 29, 1, 30, 1, 30, 1, 31,
		1, 31, 1, 32, 1, 32, 1, 33, 1, 33, 1, 34, 1, 34, 1, 35, 1, 35, 1, 36, 1,
		36, 1, 37, 1, 37, 1, 38, 1, 38, 1, 39, 1, 39, 1, 40, 1, 40, 1, 41, 1, 41,
		1, 42, 1, 42, 1, 43, 1, 43, 1, 44, 1, 44, 1, 44, 1, 44, 4, 44, 347, 8,
		44, 11, 44, 12, 44, 348, 1, 45, 1, 45, 1, 46, 1, 46, 1, 47, 1, 47, 1, 47,
		1, 48, 1, 48, 1, 49, 1, 49, 1, 50, 1, 50, 1, 51, 1, 51, 1, 51, 1, 51, 5,
		51, 368, 8, 51, 10, 51, 12, 51, 371, 9, 51, 1, 52, 1, 52, 1, 53, 1, 53,
		1, 53, 1, 53, 1, 54, 4, 54, 380, 8, 54, 11, 54, 12, 54, 381, 1, 55, 1,
		55, 1, 55, 1, 55, 4, 55, 388, 8, 55, 11, 55, 12, 55, 389, 1, 56, 1, 56,
		1, 56, 1, 56, 4, 56, 396, 8, 56, 11, 56, 12, 56, 397, 1, 57, 1, 57, 1,
		57, 1, 57, 4, 57, 404, 8, 57, 11, 57, 12, 57, 405, 1, 58, 1, 58, 1, 58,
		1, 58, 1, 59, 1, 59, 1, 59, 1, 59, 1, 59, 1, 60, 1, 60, 1, 60, 1, 60, 1,
		60, 1, 61, 1, 61, 1, 61, 1, 61, 1, 61, 1, 61, 1, 61, 1, 61, 3, 61, 430,
		8, 61, 1, 62, 1, 62, 1, 62, 1, 62, 1, 62, 1, 63, 1, 63, 1, 63, 1, 63, 1,
		63, 1, 63, 1, 63, 1, 63, 1, 63, 3, 63, 446, 8, 63, 1, 64, 1, 64, 1, 64,
		1, 64, 1, 64, 1, 64, 1, 65, 1, 65, 1, 65, 1, 65, 1, 65, 1, 65, 1, 66, 1,
		66, 1, 66, 1, 66, 1, 66, 1, 67, 1, 67, 1, 67, 1, 67, 1, 67, 1, 67, 1, 67,
		1, 67, 3, 67, 473, 8, 67, 1, 68, 1, 68, 1, 68, 1, 68, 1, 68, 1, 69, 1,
		69, 1, 69, 1, 69, 1, 69, 1, 70, 1, 70, 1, 70, 1, 71, 1, 71, 1, 71, 1, 71,
		1, 71, 1, 72, 1, 72, 1, 72, 1, 72, 1, 72, 1, 72, 1, 72, 1, 72, 1, 72, 1,
		72, 1, 72, 1, 72, 3, 72, 505, 8, 72, 1, 73, 1, 73, 1, 73, 1, 73, 1, 73,
		1, 73, 1, 73, 1, 73, 1, 73, 1, 73, 1, 73, 1, 73, 1, 73, 3, 73, 520, 8,
		73, 1, 74, 1, 74, 1, 74, 1, 74, 1, 75, 1, 75, 1, 75, 1, 75, 1, 75, 1, 75,
		1, 75, 1, 75, 1, 75, 1, 75, 1, 75, 1, 75, 1, 75, 1, 75, 1, 75, 1, 75, 1,
		75, 3, 75, 543, 8, 75, 1, 76, 1, 76, 1, 76, 1, 76, 1, 76, 1, 76, 1, 76,
		1, 76, 1, 76, 3, 76, 554, 8, 76, 1, 77, 1, 77, 1, 77, 1, 77, 1, 77, 1,
		77, 1, 77, 1, 78, 1, 78, 1, 78, 1, 78, 1, 78, 1, 79, 1, 79, 1, 79, 1, 79,
		1, 80, 1, 80, 1, 80, 1, 80, 1, 81, 1, 81, 1, 81, 1, 81, 1, 81, 1, 81, 1,
		81, 1, 82, 1, 82, 1, 82, 1, 82, 1, 82, 1, 82, 1, 82, 1, 82, 1, 82, 1, 82,
		1, 82, 1, 83, 1, 83, 1, 83, 1, 83, 1, 84, 1, 84, 1, 84, 1, 84, 1, 84, 1,
		84, 1, 84, 1, 84, 1, 84, 1, 84, 1, 84, 1, 85, 1, 85, 1, 86, 1, 86, 1, 87,
		1, 87, 1, 88, 1, 88, 5, 88, 617, 8, 88, 10, 88, 12, 88, 620, 9, 88, 1,
		89, 1, 89, 3, 89, 624, 8, 89, 1, 90, 1, 90, 1, 90, 1, 90, 1, 90, 1, 90,
		1, 90, 5, 90, 633, 8, 90, 10, 90, 12, 90, 636, 9, 90, 1, 90, 1, 90, 1,
		90, 1, 90, 1, 90, 1, 90, 1, 90, 1, 90, 1, 90, 5, 90, 647, 8, 90, 10, 90,
		12, 90, 650, 9, 90, 1, 90, 1, 90, 3, 90, 654, 8, 90, 1, 91, 1, 91, 4, 91,
		658, 8, 91, 11, 91, 12, 91, 659, 1, 91, 1, 91, 1, 92, 4, 92, 665, 8, 92,
		11, 92, 12, 92, 666, 1, 92, 1, 92, 1, 93, 1, 93, 1, 93, 1, 93, 1, 93, 1,
		93, 1, 93, 1, 93, 1, 93, 3, 93, 680, 8, 93, 1, 94, 1, 94, 1, 94, 1, 94,
		1, 94, 1, 94, 1, 94, 1, 94, 1, 95, 1, 95, 1, 95, 1, 95, 1, 95, 1, 95, 1,
		96, 1, 96, 1, 96, 1, 96, 1, 96, 1, 96, 1, 97, 1, 97, 1, 98, 1, 98, 1, 98,
		1, 98, 1, 99, 1, 99, 1, 100, 1, 100, 5, 100, 712, 8, 100, 10, 100, 12,
		100, 715, 9, 100, 1, 101, 3, 101, 718, 8, 101, 1, 102, 1, 102, 1, 103,
		1, 103, 1, 104, 1, 104, 1, 105, 1, 105, 1, 106, 1, 106, 1, 106, 3, 106,
		731, 8, 106, 0, 0, 107, 4, 1, 6, 2, 8, 3, 10, 4, 12, 5, 14, 6, 16, 7, 18,
		8, 20, 9, 22, 10, 24, 11, 26, 12, 28, 13, 30, 14, 32, 15, 34, 16, 36, 17,
		38, 18, 40, 19, 42, 20, 44, 21, 46, 22, 48, 23, 50, 24, 52, 25, 54, 26,
		56, 27, 58, 28, 60, 29, 62, 30, 64, 31, 66, 32, 68, 33, 70, 34, 72, 35,
		74, 36, 76, 37, 78, 38, 80, 39, 82, 40, 84, 41, 86, 42, 88, 43, 90, 44,
		92, 45, 94, 46, 96, 47, 98, 48, 100, 49, 102, 50, 104, 51, 106, 52, 108,
		53, 110, 54, 112, 55, 114, 56, 116, 57, 118, 58, 120, 59, 122, 60, 124,
		61, 126, 62, 128, 63, 130, 64, 132, 65, 134, 66, 136, 67, 138, 68, 140,
		69, 142, 70, 144, 71, 146, 72, 148, 73, 150, 74, 152, 75, 154, 76, 156,
		77, 158, 78, 160, 79, 162, 80, 164, 81, 166, 82, 168, 83, 170, 84, 172,
		85, 174, 86, 176, 87, 178, 88, 180, 89, 182, 90, 184, 91, 186, 92, 188,
		93, 190, 94, 192, 95, 194, 96, 196, 97, 198, 98, 200, 99, 202, 100, 204,
		0, 206, 0, 208, 0, 210, 0, 212, 0, 214, 0, 216, 0, 4, 0, 1, 2, 3, 13, 2,
		0, 10, 10, 13, 13, 2, 0, 13, 13, 32, 32, 1, 0, 48, 57, 4, 0, 10, 10, 13,
		13, 39, 39, 92, 92, 4, 0, 10, 10, 13, 13, 34, 34, 92, 92, 3, 0, 9, 9, 13,
		13, 32, 32, 4, 0, 65, 90, 95, 95, 97, 122, 128, 65534, 5, 0, 48, 57, 65,
		90, 95, 95, 97, 122, 128, 65534, 4, 0, 42, 42, 65, 90, 95, 95, 97, 122,
		5, 0, 42, 42, 48, 57, 65, 90, 95, 95, 97, 122, 3, 0, 48, 57, 65, 70, 97,
		102, 1, 0, 48, 55, 1, 0, 47, 47, 751, 0, 4, 1, 0, 0, 0, 0, 6, 1, 0, 0,
		0, 0, 8, 1, 0, 0, 0, 0, 10, 1, 0, 0, 0, 0, 12, 1, 0, 0, 0, 0, 14, 1, 0,
		0, 0, 0, 16, 1, 0, 0, 0, 0, 18, 1, 0, 0, 0, 0, 20, 1, 0, 0, 0, 0, 22, 1,
		0, 0, 0, 0, 24, 1, 0, 0, 0, 0, 26, 1, 0, 0, 0, 0, 28, 1, 0, 0, 0, 0, 30,
		1, 0, 0, 0, 0, 32, 1, 0, 0, 0, 0, 34, 1, 0, 0, 0, 0, 36, 1, 0, 0, 0, 0,
		38, 1, 0, 0, 0, 0, 40, 1, 0, 0, 0, 0, 42, 1, 0, 0, 0, 0, 44, 1, 0, 0, 0,
		0, 46, 1, 0, 0, 0, 0, 48, 1, 0, 0, 0, 0, 50, 1, 0, 0, 0, 0, 52, 1, 0, 0,
		0, 0, 54, 1, 0, 0, 0, 0, 56, 1, 0, 0, 0, 0, 58, 1, 0, 0, 0, 0, 60, 1, 0,
		0, 0, 0, 62, 1, 0, 0, 0, 0, 64, 1, 0, 0, 0, 0, 66, 1, 0, 0, 0, 0, 68, 1,
		0, 0, 0, 0, 70, 1, 0, 0, 0, 0, 72, 1, 0, 0, 0, 0, 74, 1, 0, 0, 0, 0, 76,
		1, 0, 0, 0, 0, 78, 1, 0, 0, 0, 0, 80, 1, 0, 0, 0, 0, 82, 1, 0, 0, 0, 0,
		84, 1, 0, 0, 0, 0, 86, 1, 0, 0, 0, 0, 88, 1, 0, 0, 0, 0, 90, 1, 0, 0, 0,
		0, 92, 1, 0, 0, 0, 0, 94, 1, 0, 0, 0, 0, 96, 1, 0, 0, 0, 0, 98, 1, 0, 0,
		0, 0, 100, 1, 0, 0, 0, 0, 102, 1, 0, 0, 0, 0, 104, 1, 0, 0, 0, 0, 106,
		1, 0, 0, 0, 0, 108, 1, 0, 0, 0, 0, 110, 1, 0, 0, 0, 0, 112, 1, 0, 0, 0,
		0, 114, 1, 0, 0, 0, 0, 116, 1, 0, 0, 0, 0, 118, 1, 0, 0, 0, 0, 120, 1,
		0, 0, 0, 0, 122, 1, 0, 0, 0, 0, 124, 1, 0, 0, 0, 0, 126, 1, 0, 0, 0, 0,
		128, 1, 0, 0, 0, 0, 130, 1, 0, 0, 0, 0, 132, 1, 0, 0, 0, 0, 134, 1, 0,
		0, 0, 0, 136, 1, 0, 0, 0, 0, 138, 1, 0, 0, 0, 0, 140, 1, 0, 0, 0, 0, 142,
		1, 0, 0, 0, 0, 144, 1, 0, 0, 0, 0, 146, 1, 0, 0, 0, 0, 148, 1, 0, 0, 0,
		0, 150, 1, 0, 0, 0, 0, 152, 1, 0, 0, 0, 0, 154, 1, 0, 0, 0, 0, 156, 1,
		0, 0, 0, 0, 158, 1, 0, 0, 0, 0, 160, 1, 0, 0, 0, 0, 162, 1, 0, 0, 0, 0,
		164, 1, 0, 0, 0, 0, 166, 1, 0, 0, 0, 0, 168, 1, 0, 0, 0, 0, 170, 1, 0,
		0, 0, 0, 172, 1, 0, 0, 0, 0, 174, 1, 0, 0, 0, 0, 176, 1, 0, 0, 0, 0, 178,
		1, 0, 0, 0, 0, 180, 1, 0, 0, 0, 0, 182, 1, 0, 0, 0, 0, 184, 1, 0, 0, 0,
		0, 186, 1, 0, 0, 0, 0, 188, 1, 0, 0, 0, 1, 190, 1, 0, 0, 0, 1, 192, 1,
		0, 0, 0, 1, 194, 1, 0, 0, 0, 2, 196, 1, 0, 0, 0, 2, 198, 1, 0, 0, 0, 3,
		200, 1, 0, 0, 0, 3, 202, 1, 0, 0, 0, 4, 218, 1, 0, 0, 0, 6, 222, 1, 0,
		0, 0, 8, 226, 1, 0, 0, 0, 10, 229, 1, 0, 0, 0, 12, 232, 1, 0, 0, 0, 14,
		237, 1, 0, 0, 0, 16, 242, 1, 0, 0, 0, 18, 245, 1, 0, 0, 0, 20, 248, 1,
		0, 0, 0, 22, 251, 1, 0, 0, 0, 24, 254, 1, 0, 0, 0, 26, 257, 1, 0, 0, 0,
		28, 260, 1, 0, 0, 0, 30, 263, 1, 0, 0, 0, 32, 266, 1, 0, 0, 0, 34, 269,
		1, 0, 0, 0, 36, 271, 1, 0, 0, 0, 38, 274, 1, 0, 0, 0, 40, 277, 1, 0, 0,
		0, 42, 280, 1, 0, 0, 0, 44, 284, 1, 0, 0, 0, 46, 288, 1, 0, 0, 0, 48, 291,
		1, 0, 0, 0, 50, 294, 1, 0, 0, 0, 52, 298, 1, 0, 0, 0, 54, 300, 1, 0, 0,
		0, 56, 302, 1, 0, 0, 0, 58, 308, 1, 0, 0, 0, 60, 310, 1, 0, 0, 0, 62, 312,
		1, 0, 0, 0, 64, 314, 1, 0, 0, 0, 66, 316, 1, 0, 0, 0, 68, 318, 1, 0, 0,
		0, 70, 320, 1, 0, 0, 0, 72, 322, 1, 0, 0, 0, 74, 324, 1, 0, 0, 0, 76, 326,
		1, 0, 0, 0, 78, 328, 1, 0, 0, 0, 80, 330, 1, 0, 0, 0, 82, 332, 1, 0, 0,
		0, 84, 334, 1, 0, 0, 0, 86, 336, 1, 0, 0, 0, 88, 338, 1, 0, 0, 0, 90, 340,
		1, 0, 0, 0, 92, 342, 1, 0, 0, 0, 94, 350, 1, 0, 0, 0, 96, 352, 1, 0, 0,
		0, 98, 354, 1, 0, 0, 0, 100, 357, 1, 0, 0, 0, 102, 359, 1, 0, 0, 0, 104,
		361, 1, 0, 0, 0, 106, 363, 1, 0, 0, 0, 108, 372, 1, 0, 0, 0, 110, 374,
		1, 0, 0, 0, 112, 379, 1, 0, 0, 0, 114, 383, 1, 0, 0, 0, 116, 391, 1, 0,
		0, 0, 118, 399, 1, 0, 0, 0, 120, 407, 1, 0, 0, 0, 122, 411, 1, 0, 0, 0,
		124, 416, 1, 0, 0, 0, 126, 429, 1, 0, 0, 0, 128, 431, 1, 0, 0, 0, 130,
		445, 1, 0, 0, 0, 132, 447, 1, 0, 0, 0, 134, 453, 1, 0, 0, 0, 136, 459,
		1, 0, 0, 0, 138, 472, 1, 0, 0, 0, 140, 474, 1, 0, 0, 0, 142, 479, 1, 0,
		0, 0, 144, 484, 1, 0, 0, 0, 146, 487, 1, 0, 0, 0, 148, 504, 1, 0, 0, 0,
		150, 519, 1, 0, 0, 0, 152, 521, 1, 0, 0, 0, 154, 542, 1, 0, 0, 0, 156,
		553, 1, 0, 0, 0, 158, 555, 1, 0, 0, 0, 160, 562, 1, 0, 0, 0, 162, 567,
		1, 0, 0, 0, 164, 571, 1, 0, 0, 0, 166, 575, 1, 0, 0, 0, 168, 582, 1, 0,
		0, 0, 170, 593, 1, 0, 0, 0, 172, 597, 1, 0, 0, 0, 174, 608, 1, 0, 0, 0,
		176, 610, 1, 0, 0, 0, 178, 612, 1, 0, 0, 0, 180, 614, 1, 0, 0, 0, 182,
		623, 1, 0, 0, 0, 184, 653, 1, 0, 0, 0, 186, 655, 1, 0, 0, 0, 188, 664,
		1, 0, 0, 0, 190, 679, 1, 0, 0, 0, 192, 681, 1, 0, 0, 0, 194, 689, 1, 0,
		0, 0, 196, 695, 1, 0, 0, 0, 198, 701, 1, 0, 0, 0, 200, 703, 1, 0, 0, 0,
		202, 707, 1, 0, 0, 0, 204, 709, 1, 0, 0, 0, 206, 717, 1, 0, 0, 0, 208,
		719, 1, 0, 0, 0, 210, 721, 1, 0, 0, 0, 212, 723, 1, 0, 0, 0, 214, 725,
		1, 0, 0, 0, 216, 730, 1, 0, 0, 0, 218, 219, 5, 61, 0, 0, 219, 220, 5, 61,
		0, 0, 220, 221, 5, 62, 0, 0, 221, 5, 1, 0, 0, 0, 222, 223, 5, 46, 0, 0,
		223, 224, 5, 46, 0, 0, 224, 225, 5, 46, 0, 0, 225, 7, 1, 0, 0, 0, 226,
		227, 5, 37, 0, 0, 227, 228, 5, 37, 0, 0, 228, 9, 1, 0, 0, 0, 229, 230,
		5, 46, 0, 0, 230, 231, 5, 46, 0, 0, 231, 11, 1, 0, 0, 0, 232, 233, 5, 60,
		0, 0, 233, 234, 5, 61, 0, 0, 234, 235, 1, 0, 0, 0, 235, 236, 4, 4, 0, 0,
		236, 13, 1, 0, 0, 0, 237, 238, 5, 62, 0, 0, 238, 239, 5, 61, 0, 0, 239,
		240, 1, 0, 0, 0, 240, 241, 4, 5, 1, 0, 241, 15, 1, 0, 0, 0, 242, 243, 5,
		62, 0, 0, 243, 244, 5, 62, 0, 0, 244, 17, 1, 0, 0, 0, 245, 246, 5, 61,
		0, 0, 246, 247, 5, 62, 0, 0, 247, 19, 1, 0, 0, 0, 248, 249, 5, 61, 0, 0,
		249, 250, 5, 61, 0, 0, 250, 21, 1, 0, 0, 0, 251, 252, 5, 61, 0, 0, 252,
		253, 5, 126, 0, 0, 253, 23, 1, 0, 0, 0, 254, 255, 5, 33, 0, 0, 255, 256,
		5, 126, 0, 0, 256, 25, 1, 0, 0, 0, 257, 258, 5, 38, 0, 0, 258, 259, 5,
		38, 0, 0, 259, 27, 1, 0, 0, 0, 260, 261, 5, 124, 0, 0, 261, 262, 5, 124,
		0, 0, 262, 29, 1, 0, 0, 0, 263, 264, 5, 33, 0, 0, 264, 265, 5, 61, 0, 0,
		265, 31, 1, 0, 0, 0, 266, 267, 5, 36, 0, 0, 267, 268, 5, 123, 0, 0, 268,
		33, 1, 0, 0, 0, 269, 270, 5, 59, 0, 0, 270, 35, 1, 0, 0, 0, 271, 272, 5,
		63, 0, 0, 272, 273, 5, 123, 0, 0, 273, 37, 1, 0, 0, 0, 274, 275, 5, 45,
		0, 0, 275, 276, 5, 123, 0, 0, 276, 39, 1, 0, 0, 0, 277, 278, 5, 45, 0,
		0, 278, 279, 5, 62, 0, 0, 279, 41, 1, 0, 0, 0, 280, 281, 5, 125, 0, 0,
		281, 282, 5, 45, 0, 0, 282, 283, 5, 62, 0, 0, 283, 43, 1, 0, 0, 0, 284,
		285, 5, 45, 0, 0, 285, 286, 5, 45, 0, 0, 286, 287, 5, 62, 0, 0, 287, 45,
		1, 0, 0, 0, 288, 289, 5, 35, 0, 0, 289, 290, 5, 123, 0, 0, 290, 47, 1,
		0, 0, 0, 291, 292, 5, 35, 0, 0, 292, 293, 5, 62, 0, 0, 293, 49, 1, 0, 0,
		0, 294, 295, 5, 35, 0, 0, 295, 296, 5, 45, 0, 0, 296, 297, 5, 62, 0, 0,
		297, 51, 1, 0, 0, 0, 298, 299, 5, 62, 0, 0, 299, 53, 1, 0, 0, 0, 300, 301,
		5, 46, 0, 0, 301, 55, 1, 0, 0, 0, 302, 303, 5, 60, 0, 0, 303, 304, 5, 60,
		0, 0, 304, 305, 5, 60, 0, 0, 305, 306, 1, 0, 0, 0, 306, 307, 6, 26, 0,
		0, 307, 57, 1, 0, 0, 0, 308, 309, 5, 60, 0, 0, 309, 59, 1, 0, 0, 0, 310,
		311, 5, 61, 0, 0, 311, 61, 1, 0, 0, 0, 312, 313, 5, 43, 0, 0, 313, 63,
		1, 0, 0, 0, 314, 315, 5, 38, 0, 0, 315, 65, 1, 0, 0, 0, 316, 317, 5, 63,
		0, 0, 317, 67, 1, 0, 0, 0, 318, 319, 5, 40, 0, 0, 319, 69, 1, 0, 0, 0,
		320, 321, 5, 44, 0, 0, 321, 71, 1, 0, 0, 0, 322, 323, 5, 41, 0, 0, 323,
		73, 1, 0, 0, 0, 324, 325, 5, 91, 0, 0, 325, 75, 1, 0, 0, 0, 326, 327, 5,
		93, 0, 0, 327, 77, 1, 0, 0, 0, 328, 329, 5, 123, 0, 0, 329, 79, 1, 0, 0,
		0, 330, 331, 5, 125, 0, 0, 331, 81, 1, 0, 0, 0, 332, 333, 5, 35, 0, 0,
		333, 83, 1, 0, 0, 0, 334, 335, 5, 36, 0, 0, 335, 85, 1, 0, 0, 0, 336, 337,
		5, 58, 0, 0, 337, 87, 1, 0, 0, 0, 338, 339, 5, 37, 0, 0, 339, 89, 1, 0,
		0, 0, 340, 341, 5, 33, 0, 0, 341, 91, 1, 0, 0, 0, 342, 346, 5, 42, 0, 0,
		343, 344, 5, 45, 0, 0, 344, 347, 3, 208, 102, 0, 345, 347, 3, 208, 102,
		0, 346, 343, 1, 0, 0, 0, 346, 345, 1, 0, 0, 0, 347, 348, 1, 0, 0, 0, 348,
		346, 1, 0, 0, 0, 348, 349, 1, 0, 0, 0, 349, 93, 1, 0, 0, 0, 350, 351, 5,
		42, 0, 0, 351, 95, 1, 0, 0, 0, 352, 353, 5, 45, 0, 0, 353, 97, 1, 0, 0,
		0, 354, 355, 5, 97, 0, 0, 355, 356, 5, 115, 0, 0, 356, 99, 1, 0, 0, 0,
		357, 358, 5, 96, 0, 0, 358, 101, 1, 0, 0, 0, 359, 360, 5, 39, 0, 0, 360,
		103, 1, 0, 0, 0, 361, 362, 5, 34, 0, 0, 362, 105, 1, 0, 0, 0, 363, 364,
		5, 47, 0, 0, 364, 365, 5, 47, 0, 0, 365, 369, 1, 0, 0, 0, 366, 368, 8,
		0, 0, 0, 367, 366, 1, 0, 0, 0, 368, 371, 1, 0, 0, 0, 369, 367, 1, 0, 0,
		0, 369, 370, 1, 0, 0, 0, 370, 107, 1, 0, 0, 0, 371, 369, 1, 0, 0, 0, 372,
		373, 5, 10, 0, 0, 373, 109, 1, 0, 0, 0, 374, 375, 7, 1, 0, 0, 375, 376,
		1, 0, 0, 0, 376, 377, 6, 53, 1, 0, 377, 111, 1, 0, 0, 0, 378, 380, 3, 212,
		104, 0, 379, 378, 1, 0, 0, 0, 380, 381, 1, 0, 0, 0, 381, 379, 1, 0, 0,
		0, 381, 382, 1, 0, 0, 0, 382, 113, 1, 0, 0, 0, 383, 384, 5, 48, 0, 0, 384,
		385, 5, 111, 0, 0, 385, 387, 1, 0, 0, 0, 386, 388, 3, 214, 105, 0, 387,
		386, 1, 0, 0, 0, 388, 389, 1, 0, 0, 0, 389, 387, 1, 0, 0, 0, 389, 390,
		1, 0, 0, 0, 390, 115, 1, 0, 0, 0, 391, 392, 5, 48, 0, 0, 392, 393, 5, 98,
		0, 0, 393, 395, 1, 0, 0, 0, 394, 396, 2, 48, 49, 0, 395, 394, 1, 0, 0,
		0, 396, 397, 1, 0, 0, 0, 397, 395, 1, 0, 0, 0, 397, 398, 1, 0, 0, 0, 398,
		117, 1, 0, 0, 0, 399, 400, 5, 48, 0, 0, 400, 401, 5, 120, 0, 0, 401, 403,
		1, 0, 0, 0, 402, 404, 3, 210, 103, 0, 403, 402, 1, 0, 0, 0, 404, 405, 1,
		0, 0, 0, 405, 403, 1, 0, 0, 0, 405, 406, 1, 0, 0, 0, 406, 119, 1, 0, 0,
		0, 407, 408, 5, 115, 0, 0, 408, 409, 5, 116, 0, 0, 409, 410, 5, 114, 0,
		0, 410, 121, 1, 0, 0, 0, 411, 412, 5, 108, 0, 0, 412, 413, 5, 105, 0, 0,
		413, 414, 5, 115, 0, 0, 414, 415, 5, 116, 0, 0, 415, 123, 1, 0, 0, 0, 416,
		417, 5, 100, 0, 0, 417, 418, 5, 105, 0, 0, 418, 419, 5, 99, 0, 0, 419,
		420, 5, 116, 0, 0, 420, 125, 1, 0, 0, 0, 421, 422, 5, 105, 0, 0, 422, 423,
		5, 110, 0, 0, 423, 430, 5, 116, 0, 0, 424, 425, 5, 102, 0, 0, 425, 426,
		5, 108, 0, 0, 426, 427, 5, 111, 0, 0, 427, 428, 5, 97, 0, 0, 428, 430,
		5, 116, 0, 0, 429, 421, 1, 0, 0, 0, 429, 424, 1, 0, 0, 0, 430, 127, 1,
		0, 0, 0, 431, 432, 5, 98, 0, 0, 432, 433, 5, 111, 0, 0, 433, 434, 5, 111,
		0, 0, 434, 435, 5, 108, 0, 0, 435, 129, 1, 0, 0, 0, 436, 437, 5, 116, 0,
		0, 437, 438, 5, 114, 0, 0, 438, 439, 5, 117, 0, 0, 439, 446, 5, 101, 0,
		0, 440, 441, 5, 102, 0, 0, 441, 442, 5, 97, 0, 0, 442, 443, 5, 108, 0,
		0, 443, 444, 5, 115, 0, 0, 444, 446, 5, 101, 0, 0, 445, 436, 1, 0, 0, 0,
		445, 440, 1, 0, 0, 0, 446, 131, 1, 0, 0, 0, 447, 448, 5, 97, 0, 0, 448,
		449, 5, 108, 0, 0, 449, 450, 5, 101, 0, 0, 450, 451, 5, 114, 0, 0, 451,
		452, 5, 116, 0, 0, 452, 133, 1, 0, 0, 0, 453, 454, 5, 99, 0, 0, 454, 455,
		5, 104, 0, 0, 455, 456, 5, 101, 0, 0, 456, 457, 5, 99, 0, 0, 457, 458,
		5, 107, 0, 0, 458, 135, 1, 0, 0, 0, 459, 460, 5, 116, 0, 0, 460, 461, 5,
		104, 0, 0, 461, 462, 5, 101, 0, 0, 462, 463, 5, 110, 0, 0, 463, 137, 1,
		0, 0, 0, 464, 465, 5, 100, 0, 0, 465, 466, 5, 101, 0, 0, 466, 467, 5, 115,
		0, 0, 467, 473, 5, 99, 0, 0, 468, 469, 5, 110, 0, 0, 469, 470, 5, 111,
		0, 0, 470, 471, 5, 116, 0, 0, 471, 473, 5, 101, 0, 0, 472, 464, 1, 0, 0,
		0, 472, 468, 1, 0, 0, 0, 473, 139, 1, 0, 0, 0, 474, 475, 5, 101, 0, 0,
		475, 476, 5, 108, 0, 0, 476, 477, 5, 115, 0, 0, 477, 478, 5, 101, 0, 0,
		478, 141, 1, 0, 0, 0, 479, 480, 5, 116, 0, 0, 480, 481, 5, 121, 0, 0, 481,
		482, 5, 112, 0, 0, 482, 483, 5, 101, 0, 0, 483, 143, 1, 0, 0, 0, 484, 485,
		5, 105, 0, 0, 485, 486, 5, 110, 0, 0, 486, 145, 1, 0, 0, 0, 487, 488, 5,
		99, 0, 0, 488, 489, 5, 97, 0, 0, 489, 490, 5, 108, 0, 0, 490, 491, 5, 108,
		0, 0, 491, 147, 1, 0, 0, 0, 492, 493, 5, 102, 0, 0, 493, 494, 5, 117, 0,
		0, 494, 495, 5, 110, 0, 0, 495, 496, 5, 99, 0, 0, 496, 497, 5, 116, 0,
		0, 497, 498, 5, 105, 0, 0, 498, 499, 5, 111, 0, 0, 499, 505, 5, 110, 0,
		0, 500, 501, 5, 102, 0, 0, 501, 502, 5, 117, 0, 0, 502, 503, 5, 110, 0,
		0, 503, 505, 5, 99, 0, 0, 504, 492, 1, 0, 0, 0, 504, 500, 1, 0, 0, 0, 505,
		149, 1, 0, 0, 0, 506, 507, 5, 99, 0, 0, 507, 508, 5, 111, 0, 0, 508, 509,
		5, 110, 0, 0, 509, 510, 5, 115, 0, 0, 510, 520, 5, 116, 0, 0, 511, 512,
		5, 99, 0, 0, 512, 513, 5, 111, 0, 0, 513, 514, 5, 110, 0, 0, 514, 515,
		5, 115, 0, 0, 515, 516, 5, 116, 0, 0, 516, 517, 5, 97, 0, 0, 517, 518,
		5, 110, 0, 0, 518, 520, 5, 116, 0, 0, 519, 506, 1, 0, 0, 0, 519, 511, 1,
		0, 0, 0, 520, 151, 1, 0, 0, 0, 521, 522, 5, 112, 0, 0, 522, 523, 5, 104,
		0, 0, 523, 524, 5, 105, 0, 0, 524, 153, 1, 0, 0, 0, 525, 526, 5, 112, 0,
		0, 526, 527, 5, 97, 0, 0, 527, 528, 5, 114, 0, 0, 528, 529, 5, 97, 0, 0,
		529, 543, 5, 109, 0, 0, 530, 531, 5, 102, 0, 0, 531, 532, 5, 111, 0, 0,
		532, 533, 5, 114, 0, 0, 533, 534, 5, 109, 0, 0, 534, 535, 5, 97, 0, 0,
		535, 536, 5, 108, 0, 0, 536, 537, 5, 95, 0, 0, 537, 538, 5, 112, 0, 0,
		538, 539, 5, 97, 0, 0, 539, 540, 5, 114, 0, 0, 540, 541, 5, 97, 0, 0, 541,
		543, 5, 109, 0, 0, 542, 525, 1, 0, 0, 0, 542, 530, 1, 0, 0, 0, 543, 155,
		1, 0, 0, 0, 544, 545, 5, 114, 0, 0, 545, 546, 5, 101, 0, 0, 546, 547, 5,
		116, 0, 0, 547, 548, 5, 117, 0, 0, 548, 549, 5, 114, 0, 0, 549, 554, 5,
		110, 0, 0, 550, 551, 5, 114, 0, 0, 551, 552, 5, 101, 0, 0, 552, 554, 5,
		116, 0, 0, 553, 544, 1, 0, 0, 0, 553, 550, 1, 0, 0, 0, 554, 157, 1, 0,
		0, 0, 555, 556, 5, 111, 0, 0, 556, 557, 5, 112, 0, 0, 557, 558, 5, 99,
		0, 0, 558, 559, 5, 111, 0, 0, 559, 560, 5, 100, 0, 0, 560, 561, 5, 101,
		0, 0, 561, 159, 1, 0, 0, 0, 562, 563, 5, 104, 0, 0, 563, 564, 5, 97, 0,
		0, 564, 565, 5, 118, 0, 0, 565, 566, 5, 101, 0, 0, 566, 161, 1, 0, 0, 0,
		567, 568, 5, 97, 0, 0, 568, 569, 5, 110, 0, 0, 569, 570, 5, 121, 0, 0,
		570, 163, 1, 0, 0, 0, 571, 572, 5, 110, 0, 0, 572, 573, 5, 111, 0, 0, 573,
		574, 5, 116, 0, 0, 574, 165, 1, 0, 0, 0, 575, 576, 5, 105, 0, 0, 576, 577,
		5, 110, 0, 0, 577, 578, 5, 115, 0, 0, 578, 579, 5, 105, 0, 0, 579, 580,
		5, 100, 0, 0, 580, 581, 5, 101, 0, 0, 581, 167, 1, 0, 0, 0, 582, 583, 5,
		110, 0, 0, 583, 584, 5, 111, 0, 0, 584, 585, 5, 116, 0, 0, 585, 586, 5,
		95, 0, 0, 586, 587, 5, 105, 0, 0, 587, 588, 5, 110, 0, 0, 588, 589, 5,
		115, 0, 0, 589, 590, 5, 105, 0, 0, 590, 591, 5, 100, 0, 0, 591, 592, 5,
		101, 0, 0, 592, 169, 1, 0, 0, 0, 593, 594, 5, 102, 0, 0, 594, 595, 5, 111,
		0, 0, 595, 596, 5, 114, 0, 0, 596, 171, 1, 0, 0, 0, 597, 598, 5, 118, 0,
		0, 598, 599, 5, 101, 0, 0, 599, 600, 5, 114, 0, 0, 600, 601, 5, 115, 0,
		0, 601, 602, 5, 105, 0, 0, 602, 603, 5, 111, 0, 0, 603, 604, 5, 110, 0,
		0, 604, 605, 5, 95, 0, 0, 605, 606, 5, 105, 0, 0, 606, 607, 5, 110, 0,
		0, 607, 173, 1, 0, 0, 0, 608, 609, 5, 114, 0, 0, 609, 175, 1, 0, 0, 0,
		610, 611, 5, 103, 0, 0, 611, 177, 1, 0, 0, 0, 612, 613, 5, 101, 0, 0, 613,
		179, 1, 0, 0, 0, 614, 618, 3, 206, 101, 0, 615, 617, 3, 182, 89, 0, 616,
		615, 1, 0, 0, 0, 617, 620, 1, 0, 0, 0, 618, 616, 1, 0, 0, 0, 618, 619,
		1, 0, 0, 0, 619, 181, 1, 0, 0, 0, 620, 618, 1, 0, 0, 0, 621, 624, 7, 2,
		0, 0, 622, 624, 3, 206, 101, 0, 623, 621, 1, 0, 0, 0, 623, 622, 1, 0, 0,
		0, 624, 183, 1, 0, 0, 0, 625, 634, 3, 102, 49, 0, 626, 633, 8, 3, 0, 0,
		627, 628, 5, 92, 0, 0, 628, 633, 5, 39, 0, 0, 629, 630, 5, 92, 0, 0, 630,
		633, 5, 92, 0, 0, 631, 633, 5, 92, 0, 0, 632, 626, 1, 0, 0, 0, 632, 627,
		1, 0, 0, 0, 632, 629, 1, 0, 0, 0, 632, 631, 1, 0, 0, 0, 633, 636, 1, 0,
		0, 0, 634, 632, 1, 0, 0, 0, 634, 635, 1, 0, 0, 0, 635, 637, 1, 0, 0, 0,
		636, 634, 1, 0, 0, 0, 637, 638, 3, 102, 49, 0, 638, 654, 1, 0, 0, 0, 639,
		648, 3, 104, 50, 0, 640, 647, 8, 4, 0, 0, 641, 642, 5, 92, 0, 0, 642, 647,
		5, 34, 0, 0, 643, 644, 5, 92, 0, 0, 644, 647, 5, 92, 0, 0, 645, 647, 5,
		92, 0, 0, 646, 640, 1, 0, 0, 0, 646, 641, 1, 0, 0, 0, 646, 643, 1, 0, 0,
		0, 646, 645, 1, 0, 0, 0, 647, 650, 1, 0, 0, 0, 648, 646, 1, 0, 0, 0, 648,
		649, 1, 0, 0, 0, 649, 651, 1, 0, 0, 0, 650, 648, 1, 0, 0, 0, 651, 652,
		3, 104, 50, 0, 652, 654, 1, 0, 0, 0, 653, 625, 1, 0, 0, 0, 653, 639, 1,
		0, 0, 0, 654, 185, 1, 0, 0, 0, 655, 657, 5, 47, 0, 0, 656, 658, 3, 216,
		106, 0, 657, 656, 1, 0, 0, 0, 658, 659, 1, 0, 0, 0, 659, 657, 1, 0, 0,
		0, 659, 660, 1, 0, 0, 0, 660, 661, 1, 0, 0, 0, 661, 662, 5, 47, 0, 0, 662,
		187, 1, 0, 0, 0, 663, 665, 7, 5, 0, 0, 664, 663, 1, 0, 0, 0, 665, 666,
		1, 0, 0, 0, 666, 664, 1, 0, 0, 0, 666, 667, 1, 0, 0, 0, 667, 668, 1, 0,
		0, 0, 668, 669, 6, 92, 1, 0, 669, 189, 1, 0, 0, 0, 670, 671, 3, 204, 100,
		0, 671, 672, 6, 93, 2, 0, 672, 680, 1, 0, 0, 0, 673, 674, 5, 39, 0, 0,
		674, 675, 3, 204, 100, 0, 675, 676, 6, 93, 3, 0, 676, 677, 1, 0, 0, 0,
		677, 678, 5, 39, 0, 0, 678, 680, 1, 0, 0, 0, 679, 670, 1, 0, 0, 0, 679,
		673, 1, 0, 0, 0, 680, 191, 1, 0, 0, 0, 681, 682, 5, 13, 0, 0, 682, 683,
		5, 10, 0, 0, 683, 684, 1, 0, 0, 0, 684, 685, 6, 94, 4, 0, 685, 686, 1,
		0, 0, 0, 686, 687, 6, 94, 5, 0, 687, 688, 6, 94, 6, 0, 688, 193, 1, 0,
		0, 0, 689, 690, 5, 10, 0, 0, 690, 691, 6, 95, 7, 0, 691, 692, 1, 0, 0,
		0, 692, 693, 6, 95, 5, 0, 693, 694, 6, 95, 8, 0, 694, 195, 1, 0, 0, 0,
		695, 696, 5, 13, 0, 0, 696, 697, 5, 10, 0, 0, 697, 698, 1, 0, 0, 0, 698,
		699, 3, 204, 100, 0, 699, 700, 6, 96, 9, 0, 700, 197, 1, 0, 0, 0, 701,
		702, 9, 0, 0, 0, 702, 199, 1, 0, 0, 0, 703, 704, 5, 10, 0, 0, 704, 705,
		3, 204, 100, 0, 705, 706, 6, 98, 10, 0, 706, 201, 1, 0, 0, 0, 707, 708,
		9, 0, 0, 0, 708, 203, 1, 0, 0, 0, 709, 713, 7, 6, 0, 0, 710, 712, 7, 7,
		0, 0, 711, 710, 1, 0, 0, 0, 712, 715, 1, 0, 0, 0, 713, 711, 1, 0, 0, 0,
		713, 714, 1, 0, 0, 0, 714, 205, 1, 0, 0, 0, 715, 713, 1, 0, 0, 0, 716,
		718, 7, 8, 0, 0, 717, 716, 1, 0, 0, 0, 718, 207, 1, 0, 0, 0, 719, 720,
		7, 9, 0, 0, 720, 209, 1, 0, 0, 0, 721, 722, 7, 10, 0, 0, 722, 211, 1, 0,
		0, 0, 723, 724, 7, 2, 0, 0, 724, 213, 1, 0, 0, 0, 725, 726, 7, 11, 0, 0,
		726, 215, 1, 0, 0, 0, 727, 728, 5, 92, 0, 0, 728, 731, 5, 47, 0, 0, 729,
		731, 8, 12, 0, 0, 730, 727, 1, 0, 0, 0, 730, 729, 1, 0, 0, 0, 731, 217,
		1, 0, 0, 0, 31, 0, 1, 2, 3, 346, 348, 369, 381, 389, 397, 405, 429, 445,
		472, 504, 519, 542, 553, 618, 623, 632, 634, 646, 648, 653, 659, 666, 679,
		713, 717, 730, 11, 5, 1, 0, 6, 0, 0, 1, 93, 0, 1, 93, 1, 1, 94, 2, 4, 0,
		0, 5, 2, 0, 1, 95, 3, 5, 3, 0, 1, 96, 4, 1, 98, 5,
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

// SyntaxFlowLexerInit initializes any static state used to implement SyntaxFlowLexer. By default the
// static state used to implement the lexer is lazily initialized during the first call to
// NewSyntaxFlowLexer(). You can call this function if you wish to initialize the static state ahead
// of time.
func SyntaxFlowLexerInit() {
	staticData := &SyntaxFlowLexerLexerStaticData
	staticData.once.Do(syntaxflowlexerLexerInit)
}

// NewSyntaxFlowLexer produces a new lexer instance for the optional input antlr.CharStream.
func NewSyntaxFlowLexer(input antlr.CharStream) *SyntaxFlowLexer {
	SyntaxFlowLexerInit()
	l := new(SyntaxFlowLexer)
	l.BaseLexer = antlr.NewBaseLexer(input)
	staticData := &SyntaxFlowLexerLexerStaticData
	l.Interpreter = antlr.NewLexerATNSimulator(l, staticData.atn, staticData.decisionToDFA, staticData.PredictionContextCache)
	l.channelNames = staticData.ChannelNames
	l.modeNames = staticData.ModeNames
	l.RuleNames = staticData.RuleNames
	l.LiteralNames = staticData.LiteralNames
	l.SymbolicNames = staticData.SymbolicNames
	l.GrammarFileName = "SyntaxFlowLexer.g4"
	// TODO: l.EOF = antlr.TokenEOF

	return l
}

// SyntaxFlowLexer tokens.
const (
	SyntaxFlowLexerDeepFilter                  = 1
	SyntaxFlowLexerDeep                        = 2
	SyntaxFlowLexerPercent                     = 3
	SyntaxFlowLexerDeepDot                     = 4
	SyntaxFlowLexerLtEq                        = 5
	SyntaxFlowLexerGtEq                        = 6
	SyntaxFlowLexerDoubleGt                    = 7
	SyntaxFlowLexerFilter                      = 8
	SyntaxFlowLexerEqEq                        = 9
	SyntaxFlowLexerRegexpMatch                 = 10
	SyntaxFlowLexerNotRegexpMatch              = 11
	SyntaxFlowLexerAnd                         = 12
	SyntaxFlowLexerOr                          = 13
	SyntaxFlowLexerNotEq                       = 14
	SyntaxFlowLexerDollarBraceOpen             = 15
	SyntaxFlowLexerSemicolon                   = 16
	SyntaxFlowLexerConditionStart              = 17
	SyntaxFlowLexerDeepNextStart               = 18
	SyntaxFlowLexerUseStart                    = 19
	SyntaxFlowLexerDeepNextEnd                 = 20
	SyntaxFlowLexerDeepNext                    = 21
	SyntaxFlowLexerTopDefStart                 = 22
	SyntaxFlowLexerDefStart                    = 23
	SyntaxFlowLexerTopDef                      = 24
	SyntaxFlowLexerGt                          = 25
	SyntaxFlowLexerDot                         = 26
	SyntaxFlowLexerStartNowDoc                 = 27
	SyntaxFlowLexerLt                          = 28
	SyntaxFlowLexerEq                          = 29
	SyntaxFlowLexerAdd                         = 30
	SyntaxFlowLexerAmp                         = 31
	SyntaxFlowLexerQuestion                    = 32
	SyntaxFlowLexerOpenParen                   = 33
	SyntaxFlowLexerComma                       = 34
	SyntaxFlowLexerCloseParen                  = 35
	SyntaxFlowLexerListSelectOpen              = 36
	SyntaxFlowLexerListSelectClose             = 37
	SyntaxFlowLexerMapBuilderOpen              = 38
	SyntaxFlowLexerMapBuilderClose             = 39
	SyntaxFlowLexerListStart                   = 40
	SyntaxFlowLexerDollarOutput                = 41
	SyntaxFlowLexerColon                       = 42
	SyntaxFlowLexerSearch                      = 43
	SyntaxFlowLexerBang                        = 44
	SyntaxFlowLexerGlobPattern                 = 45
	SyntaxFlowLexerStar                        = 46
	SyntaxFlowLexerMinus                       = 47
	SyntaxFlowLexerAs                          = 48
	SyntaxFlowLexerBacktick                    = 49
	SyntaxFlowLexerSingleQuote                 = 50
	SyntaxFlowLexerDoubleQuote                 = 51
	SyntaxFlowLexerLineComment                 = 52
	SyntaxFlowLexerBreakLine                   = 53
	SyntaxFlowLexerWhiteSpace                  = 54
	SyntaxFlowLexerNumber                      = 55
	SyntaxFlowLexerOctalNumber                 = 56
	SyntaxFlowLexerBinaryNumber                = 57
	SyntaxFlowLexerHexNumber                   = 58
	SyntaxFlowLexerStringType                  = 59
	SyntaxFlowLexerListType                    = 60
	SyntaxFlowLexerDictType                    = 61
	SyntaxFlowLexerNumberType                  = 62
	SyntaxFlowLexerBoolType                    = 63
	SyntaxFlowLexerBoolLiteral                 = 64
	SyntaxFlowLexerAlert                       = 65
	SyntaxFlowLexerCheck                       = 66
	SyntaxFlowLexerThen                        = 67
	SyntaxFlowLexerDesc                        = 68
	SyntaxFlowLexerElse                        = 69
	SyntaxFlowLexerType                        = 70
	SyntaxFlowLexerIn                          = 71
	SyntaxFlowLexerCall                        = 72
	SyntaxFlowLexerFunction                    = 73
	SyntaxFlowLexerConstant                    = 74
	SyntaxFlowLexerPhi                         = 75
	SyntaxFlowLexerFormalParam                 = 76
	SyntaxFlowLexerReturn                      = 77
	SyntaxFlowLexerOpcode                      = 78
	SyntaxFlowLexerHave                        = 79
	SyntaxFlowLexerHaveAny                     = 80
	SyntaxFlowLexerNot                         = 81
	SyntaxFlowLexerInside                      = 82
	SyntaxFlowLexerNotInside                   = 83
	SyntaxFlowLexerFor                         = 84
	SyntaxFlowLexerVersionIn                   = 85
	SyntaxFlowLexerConstSearchModePrefixRegexp = 86
	SyntaxFlowLexerConstSearchModePrefixGlob   = 87
	SyntaxFlowLexerConstSearchModePrefixExact  = 88
	SyntaxFlowLexerIdentifier                  = 89
	SyntaxFlowLexerIdentifierChar              = 90
	SyntaxFlowLexerQuotedStringLiteral         = 91
	SyntaxFlowLexerRegexpLiteral               = 92
	SyntaxFlowLexerWS                          = 93
	SyntaxFlowLexerHereDocIdentifierName       = 94
	SyntaxFlowLexerCRLFHereDocIdentifierBreak  = 95
	SyntaxFlowLexerLFHereDocIdentifierBreak    = 96
	SyntaxFlowLexerCRLFEndDoc                  = 97
	SyntaxFlowLexerCRLFHereDocText             = 98
	SyntaxFlowLexerLFEndDoc                    = 99
	SyntaxFlowLexerLFHereDocText               = 100
)

// SyntaxFlowLexer modes.
const (
	SyntaxFlowLexerHereDocIdentifier = iota + 1
	SyntaxFlowLexerCRLFHereDoc
	SyntaxFlowLexerLFHereDoc
)

func (l *SyntaxFlowLexer) Action(localctx antlr.RuleContext, ruleIndex, actionIndex int) {
	switch ruleIndex {
	case 93:
		l.HereDocIdentifierName_Action(localctx, actionIndex)

	case 94:
		l.CRLFHereDocIdentifierBreak_Action(localctx, actionIndex)

	case 95:
		l.LFHereDocIdentifierBreak_Action(localctx, actionIndex)

	case 96:
		l.CRLFEndDoc_Action(localctx, actionIndex)

	case 98:
		l.LFEndDoc_Action(localctx, actionIndex)

	default:
		panic("No registered action for: " + fmt.Sprint(ruleIndex))
	}
}

func (l *SyntaxFlowLexer) HereDocIdentifierName_Action(localctx antlr.RuleContext, actionIndex int) {
	this := l
	_ = this

	switch actionIndex {
	case 0:
		this.recordHereDocLabel()

	case 1:
		this.recordHereDocLabel()

	default:
		panic("No registered action for: " + fmt.Sprint(actionIndex))
	}
}
func (l *SyntaxFlowLexer) CRLFHereDocIdentifierBreak_Action(localctx antlr.RuleContext, actionIndex int) {
	this := l
	_ = this

	switch actionIndex {
	case 2:
		this.recordHereDocLF()

	default:
		panic("No registered action for: " + fmt.Sprint(actionIndex))
	}
}
func (l *SyntaxFlowLexer) LFHereDocIdentifierBreak_Action(localctx antlr.RuleContext, actionIndex int) {
	this := l
	_ = this

	switch actionIndex {
	case 3:
		this.recordHereDocLF()

	default:
		panic("No registered action for: " + fmt.Sprint(actionIndex))
	}
}
func (l *SyntaxFlowLexer) CRLFEndDoc_Action(localctx antlr.RuleContext, actionIndex int) {
	this := l
	_ = this

	switch actionIndex {
	case 4:
		this.DocEndDistribute()

	default:
		panic("No registered action for: " + fmt.Sprint(actionIndex))
	}
}
func (l *SyntaxFlowLexer) LFEndDoc_Action(localctx antlr.RuleContext, actionIndex int) {
	this := l
	_ = this

	switch actionIndex {
	case 5:
		this.DocEndDistribute()

	default:
		panic("No registered action for: " + fmt.Sprint(actionIndex))
	}
}

func (l *SyntaxFlowLexer) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 4:
		return l.LtEq_Sempred(localctx, predIndex)

	case 5:
		return l.GtEq_Sempred(localctx, predIndex)

	default:
		panic("No registered predicate for: " + fmt.Sprint(ruleIndex))
	}
}

func (p *SyntaxFlowLexer) LtEq_Sempred(localctx antlr.RuleContext, predIndex int) bool {
	this := p
	_ = this

	switch predIndex {
	case 0:
		return this.GetInputStream().LA(1) != '='

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}

func (p *SyntaxFlowLexer) GtEq_Sempred(localctx antlr.RuleContext, predIndex int) bool {
	this := p
	_ = this

	switch predIndex {
	case 1:
		return this.GetInputStream().LA(1) != '='

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}
