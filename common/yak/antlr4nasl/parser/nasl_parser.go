// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package parser // NaslParser

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

type NaslParser struct {
	*antlr.BaseParser
}

var naslparserParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	literalNames           []string
	symbolicNames          []string
	ruleNames              []string
	predictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func naslparserParserInit() {
	staticData := &naslparserParserStaticData
	staticData.literalNames = []string{
		"", "", "'['", "']'", "'('", "')'", "'{'", "'}'", "';'", "','", "'='",
		"':'", "'.'", "'++'", "'--'", "'+'", "'-'", "'~'", "'&'", "'^'", "'|'",
		"'>>'", "'<<'", "'<<<'", "'>>>'", "'!'", "'*'", "'**'", "'/'", "'%'",
		"'<'", "'>'", "'<='", "'>='", "'=='", "'=~'", "'!='", "'!~'", "'>!<'",
		"'><'", "'&&'", "'||'", "'*='", "'/='", "'%='", "'+='", "'-='", "'x'",
		"'>>>='", "'<<<='", "'>>='", "'<<='", "'break'", "'var'", "'local_var'",
		"'global_var'", "'else'", "'return'", "'continue'", "'for'", "'foreach'",
		"'if'", "'function'", "'repeat'", "'while'", "'until'", "", "", "",
		"", "", "", "'NULL'",
	}
	staticData.symbolicNames = []string{
		"", "SingleLineComment", "OpenBracket", "CloseBracket", "OpenParen",
		"CloseParen", "OpenBrace", "CloseBrace", "SemiColon", "Comma", "Assign",
		"Colon", "Dot", "PlusPlus", "MinusMinus", "Plus", "Minus", "BitNot",
		"BitAnd", "BitXOr", "BitOr", "RightShiftArithmetic", "LeftShiftArithmetic",
		"LeftShiftLogical", "RightShiftLogical", "Not", "Multiply", "Pow", "Divide",
		"Modulus", "LessThan", "MoreThan", "LessThanEquals", "GreaterThanEquals",
		"Equals_", "EqualsRe", "NotEquals", "NotLong", "MTNotLT", "MTLT", "And",
		"Or", "MultiplyAssign", "DivideAssign", "ModulusAssign", "PlusAssign",
		"MinusAssign", "X", "RightShiftLogicalAssign", "LeftShiftLogicalAssign",
		"RightShiftArithmeticAssign", "LeftShiftArithmeticAssign", "Break",
		"Var", "LocalVar", "GlobalVar", "Else", "Return", "Continue", "For",
		"ForEach", "If", "Function_", "Repeat", "While", "Until", "StringLiteral",
		"BooleanLiteral", "IntegerLiteral", "FloatLiteral", "IpLiteral", "HexLiteral",
		"NULLLiteral", "Identifier", "WhiteSpaces", "LineTerminator",
	}
	staticData.ruleNames = []string{
		"program", "statementList", "statement", "block", "variableDeclarationStatement",
		"variableAssignStatement", "expressionStatement", "ifStatement", "iterationStatement",
		"continueStatement", "breakStatement", "returnStatement", "argumentList",
		"argument", "expressionSequence", "functionDeclarationStatement", "parameterList",
		"arrayLiteral", "elementList", "arrayElement", "singleExpression", "literal",
		"numericLiteral", "identifier", "assignmentOperator", "eos",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 75, 349, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7, 20, 2,
		21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25, 1, 0,
		3, 0, 54, 8, 0, 1, 0, 1, 0, 1, 1, 4, 1, 59, 8, 1, 11, 1, 12, 1, 60, 1,
		2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1,
		2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 3, 2, 85, 8, 2,
		1, 3, 1, 3, 3, 3, 89, 8, 3, 1, 3, 3, 3, 92, 8, 3, 1, 3, 1, 3, 1, 4, 1,
		4, 1, 4, 1, 4, 5, 4, 100, 8, 4, 10, 4, 12, 4, 103, 9, 4, 1, 5, 1, 5, 1,
		5, 1, 5, 3, 5, 109, 8, 5, 1, 6, 1, 6, 1, 7, 1, 7, 1, 7, 1, 7, 1, 7, 3,
		7, 118, 8, 7, 1, 7, 1, 7, 1, 7, 3, 7, 123, 8, 7, 1, 8, 1, 8, 1, 8, 3, 8,
		128, 8, 8, 1, 8, 1, 8, 3, 8, 132, 8, 8, 1, 8, 1, 8, 3, 8, 136, 8, 8, 1,
		8, 1, 8, 3, 8, 140, 8, 8, 1, 8, 1, 8, 1, 8, 1, 8, 1, 8, 1, 8, 1, 8, 1,
		8, 1, 8, 1, 8, 1, 8, 1, 8, 1, 8, 1, 8, 1, 8, 1, 8, 1, 8, 1, 8, 1, 8, 1,
		8, 3, 8, 162, 8, 8, 1, 9, 1, 9, 1, 10, 1, 10, 1, 11, 1, 11, 1, 11, 1, 11,
		1, 11, 1, 11, 3, 11, 174, 8, 11, 1, 12, 1, 12, 1, 12, 5, 12, 179, 8, 12,
		10, 12, 12, 12, 182, 9, 12, 1, 13, 1, 13, 1, 13, 3, 13, 187, 8, 13, 1,
		13, 1, 13, 1, 14, 1, 14, 1, 14, 5, 14, 194, 8, 14, 10, 14, 12, 14, 197,
		9, 14, 1, 15, 1, 15, 1, 15, 1, 15, 3, 15, 203, 8, 15, 1, 15, 1, 15, 1,
		15, 1, 16, 1, 16, 1, 16, 5, 16, 211, 8, 16, 10, 16, 12, 16, 214, 9, 16,
		1, 17, 1, 17, 3, 17, 218, 8, 17, 1, 17, 1, 17, 1, 18, 1, 18, 4, 18, 224,
		8, 18, 11, 18, 12, 18, 225, 1, 18, 5, 18, 229, 8, 18, 10, 18, 12, 18, 232,
		9, 18, 1, 19, 1, 19, 3, 19, 236, 8, 19, 1, 19, 3, 19, 239, 8, 19, 1, 20,
		1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1,
		20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20,
		1, 20, 1, 20, 1, 20, 3, 20, 266, 8, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1,
		20, 3, 20, 273, 8, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20,
		1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1,
		20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20,
		1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1,
		20, 3, 20, 314, 8, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20, 1, 20,
		1, 20, 1, 20, 1, 20, 5, 20, 326, 8, 20, 10, 20, 12, 20, 329, 9, 20, 1,
		21, 1, 21, 1, 21, 1, 21, 1, 21, 3, 21, 336, 8, 21, 1, 22, 1, 22, 1, 23,
		1, 23, 1, 24, 1, 24, 1, 25, 4, 25, 345, 8, 25, 11, 25, 12, 25, 346, 1,
		25, 0, 1, 40, 26, 0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28,
		30, 32, 34, 36, 38, 40, 42, 44, 46, 48, 50, 0, 10, 1, 0, 54, 55, 1, 0,
		53, 55, 1, 0, 26, 29, 1, 0, 15, 16, 1, 0, 21, 24, 1, 0, 30, 33, 1, 0, 34,
		39, 2, 0, 68, 69, 71, 71, 2, 0, 47, 47, 73, 73, 3, 0, 10, 10, 42, 46, 48,
		51, 392, 0, 53, 1, 0, 0, 0, 2, 58, 1, 0, 0, 0, 4, 84, 1, 0, 0, 0, 6, 86,
		1, 0, 0, 0, 8, 95, 1, 0, 0, 0, 10, 104, 1, 0, 0, 0, 12, 110, 1, 0, 0, 0,
		14, 112, 1, 0, 0, 0, 16, 161, 1, 0, 0, 0, 18, 163, 1, 0, 0, 0, 20, 165,
		1, 0, 0, 0, 22, 167, 1, 0, 0, 0, 24, 175, 1, 0, 0, 0, 26, 186, 1, 0, 0,
		0, 28, 190, 1, 0, 0, 0, 30, 198, 1, 0, 0, 0, 32, 207, 1, 0, 0, 0, 34, 215,
		1, 0, 0, 0, 36, 221, 1, 0, 0, 0, 38, 235, 1, 0, 0, 0, 40, 272, 1, 0, 0,
		0, 42, 335, 1, 0, 0, 0, 44, 337, 1, 0, 0, 0, 46, 339, 1, 0, 0, 0, 48, 341,
		1, 0, 0, 0, 50, 344, 1, 0, 0, 0, 52, 54, 3, 2, 1, 0, 53, 52, 1, 0, 0, 0,
		53, 54, 1, 0, 0, 0, 54, 55, 1, 0, 0, 0, 55, 56, 5, 0, 0, 1, 56, 1, 1, 0,
		0, 0, 57, 59, 3, 4, 2, 0, 58, 57, 1, 0, 0, 0, 59, 60, 1, 0, 0, 0, 60, 58,
		1, 0, 0, 0, 60, 61, 1, 0, 0, 0, 61, 3, 1, 0, 0, 0, 62, 85, 3, 6, 3, 0,
		63, 85, 3, 14, 7, 0, 64, 85, 3, 16, 8, 0, 65, 66, 3, 18, 9, 0, 66, 67,
		3, 50, 25, 0, 67, 85, 1, 0, 0, 0, 68, 69, 3, 20, 10, 0, 69, 70, 3, 50,
		25, 0, 70, 85, 1, 0, 0, 0, 71, 72, 3, 22, 11, 0, 72, 73, 3, 50, 25, 0,
		73, 85, 1, 0, 0, 0, 74, 75, 3, 12, 6, 0, 75, 76, 3, 50, 25, 0, 76, 85,
		1, 0, 0, 0, 77, 78, 3, 8, 4, 0, 78, 79, 3, 50, 25, 0, 79, 85, 1, 0, 0,
		0, 80, 81, 3, 10, 5, 0, 81, 82, 3, 50, 25, 0, 82, 85, 1, 0, 0, 0, 83, 85,
		3, 30, 15, 0, 84, 62, 1, 0, 0, 0, 84, 63, 1, 0, 0, 0, 84, 64, 1, 0, 0,
		0, 84, 65, 1, 0, 0, 0, 84, 68, 1, 0, 0, 0, 84, 71, 1, 0, 0, 0, 84, 74,
		1, 0, 0, 0, 84, 77, 1, 0, 0, 0, 84, 80, 1, 0, 0, 0, 84, 83, 1, 0, 0, 0,
		85, 5, 1, 0, 0, 0, 86, 88, 5, 6, 0, 0, 87, 89, 3, 50, 25, 0, 88, 87, 1,
		0, 0, 0, 88, 89, 1, 0, 0, 0, 89, 91, 1, 0, 0, 0, 90, 92, 3, 2, 1, 0, 91,
		90, 1, 0, 0, 0, 91, 92, 1, 0, 0, 0, 92, 93, 1, 0, 0, 0, 93, 94, 5, 7, 0,
		0, 94, 7, 1, 0, 0, 0, 95, 96, 7, 0, 0, 0, 96, 101, 3, 46, 23, 0, 97, 98,
		5, 9, 0, 0, 98, 100, 3, 46, 23, 0, 99, 97, 1, 0, 0, 0, 100, 103, 1, 0,
		0, 0, 101, 99, 1, 0, 0, 0, 101, 102, 1, 0, 0, 0, 102, 9, 1, 0, 0, 0, 103,
		101, 1, 0, 0, 0, 104, 105, 7, 1, 0, 0, 105, 108, 3, 46, 23, 0, 106, 107,
		5, 10, 0, 0, 107, 109, 3, 40, 20, 0, 108, 106, 1, 0, 0, 0, 108, 109, 1,
		0, 0, 0, 109, 11, 1, 0, 0, 0, 110, 111, 3, 28, 14, 0, 111, 13, 1, 0, 0,
		0, 112, 113, 5, 61, 0, 0, 113, 114, 5, 4, 0, 0, 114, 115, 3, 40, 20, 0,
		115, 117, 5, 5, 0, 0, 116, 118, 3, 50, 25, 0, 117, 116, 1, 0, 0, 0, 117,
		118, 1, 0, 0, 0, 118, 119, 1, 0, 0, 0, 119, 122, 3, 4, 2, 0, 120, 121,
		5, 56, 0, 0, 121, 123, 3, 4, 2, 0, 122, 120, 1, 0, 0, 0, 122, 123, 1, 0,
		0, 0, 123, 15, 1, 0, 0, 0, 124, 125, 5, 59, 0, 0, 125, 127, 5, 4, 0, 0,
		126, 128, 3, 40, 20, 0, 127, 126, 1, 0, 0, 0, 127, 128, 1, 0, 0, 0, 128,
		129, 1, 0, 0, 0, 129, 131, 5, 8, 0, 0, 130, 132, 3, 40, 20, 0, 131, 130,
		1, 0, 0, 0, 131, 132, 1, 0, 0, 0, 132, 133, 1, 0, 0, 0, 133, 135, 5, 8,
		0, 0, 134, 136, 3, 40, 20, 0, 135, 134, 1, 0, 0, 0, 135, 136, 1, 0, 0,
		0, 136, 137, 1, 0, 0, 0, 137, 139, 5, 5, 0, 0, 138, 140, 3, 50, 25, 0,
		139, 138, 1, 0, 0, 0, 139, 140, 1, 0, 0, 0, 140, 141, 1, 0, 0, 0, 141,
		162, 3, 4, 2, 0, 142, 143, 5, 60, 0, 0, 143, 144, 3, 46, 23, 0, 144, 145,
		5, 4, 0, 0, 145, 146, 3, 40, 20, 0, 146, 147, 5, 5, 0, 0, 147, 148, 3,
		4, 2, 0, 148, 162, 1, 0, 0, 0, 149, 150, 5, 64, 0, 0, 150, 151, 5, 4, 0,
		0, 151, 152, 3, 40, 20, 0, 152, 153, 5, 5, 0, 0, 153, 154, 3, 4, 2, 0,
		154, 162, 1, 0, 0, 0, 155, 156, 5, 63, 0, 0, 156, 157, 3, 4, 2, 0, 157,
		158, 5, 65, 0, 0, 158, 159, 3, 40, 20, 0, 159, 160, 3, 50, 25, 0, 160,
		162, 1, 0, 0, 0, 161, 124, 1, 0, 0, 0, 161, 142, 1, 0, 0, 0, 161, 149,
		1, 0, 0, 0, 161, 155, 1, 0, 0, 0, 162, 17, 1, 0, 0, 0, 163, 164, 5, 58,
		0, 0, 164, 19, 1, 0, 0, 0, 165, 166, 5, 52, 0, 0, 166, 21, 1, 0, 0, 0,
		167, 173, 5, 57, 0, 0, 168, 169, 5, 4, 0, 0, 169, 170, 3, 40, 20, 0, 170,
		171, 5, 5, 0, 0, 171, 174, 1, 0, 0, 0, 172, 174, 3, 40, 20, 0, 173, 168,
		1, 0, 0, 0, 173, 172, 1, 0, 0, 0, 173, 174, 1, 0, 0, 0, 174, 23, 1, 0,
		0, 0, 175, 180, 3, 26, 13, 0, 176, 177, 5, 9, 0, 0, 177, 179, 3, 26, 13,
		0, 178, 176, 1, 0, 0, 0, 179, 182, 1, 0, 0, 0, 180, 178, 1, 0, 0, 0, 180,
		181, 1, 0, 0, 0, 181, 25, 1, 0, 0, 0, 182, 180, 1, 0, 0, 0, 183, 184, 3,
		46, 23, 0, 184, 185, 5, 11, 0, 0, 185, 187, 1, 0, 0, 0, 186, 183, 1, 0,
		0, 0, 186, 187, 1, 0, 0, 0, 187, 188, 1, 0, 0, 0, 188, 189, 3, 40, 20,
		0, 189, 27, 1, 0, 0, 0, 190, 195, 3, 40, 20, 0, 191, 192, 5, 9, 0, 0, 192,
		194, 3, 40, 20, 0, 193, 191, 1, 0, 0, 0, 194, 197, 1, 0, 0, 0, 195, 193,
		1, 0, 0, 0, 195, 196, 1, 0, 0, 0, 196, 29, 1, 0, 0, 0, 197, 195, 1, 0,
		0, 0, 198, 199, 5, 62, 0, 0, 199, 200, 3, 46, 23, 0, 200, 202, 5, 4, 0,
		0, 201, 203, 3, 32, 16, 0, 202, 201, 1, 0, 0, 0, 202, 203, 1, 0, 0, 0,
		203, 204, 1, 0, 0, 0, 204, 205, 5, 5, 0, 0, 205, 206, 3, 6, 3, 0, 206,
		31, 1, 0, 0, 0, 207, 212, 3, 46, 23, 0, 208, 209, 5, 9, 0, 0, 209, 211,
		3, 46, 23, 0, 210, 208, 1, 0, 0, 0, 211, 214, 1, 0, 0, 0, 212, 210, 1,
		0, 0, 0, 212, 213, 1, 0, 0, 0, 213, 33, 1, 0, 0, 0, 214, 212, 1, 0, 0,
		0, 215, 217, 5, 2, 0, 0, 216, 218, 3, 36, 18, 0, 217, 216, 1, 0, 0, 0,
		217, 218, 1, 0, 0, 0, 218, 219, 1, 0, 0, 0, 219, 220, 5, 3, 0, 0, 220,
		35, 1, 0, 0, 0, 221, 230, 3, 38, 19, 0, 222, 224, 5, 9, 0, 0, 223, 222,
		1, 0, 0, 0, 224, 225, 1, 0, 0, 0, 225, 223, 1, 0, 0, 0, 225, 226, 1, 0,
		0, 0, 226, 227, 1, 0, 0, 0, 227, 229, 3, 38, 19, 0, 228, 223, 1, 0, 0,
		0, 229, 232, 1, 0, 0, 0, 230, 228, 1, 0, 0, 0, 230, 231, 1, 0, 0, 0, 231,
		37, 1, 0, 0, 0, 232, 230, 1, 0, 0, 0, 233, 236, 3, 40, 20, 0, 234, 236,
		3, 46, 23, 0, 235, 233, 1, 0, 0, 0, 235, 234, 1, 0, 0, 0, 236, 238, 1,
		0, 0, 0, 237, 239, 5, 9, 0, 0, 238, 237, 1, 0, 0, 0, 238, 239, 1, 0, 0,
		0, 239, 39, 1, 0, 0, 0, 240, 241, 6, 20, -1, 0, 241, 273, 3, 34, 17, 0,
		242, 243, 5, 4, 0, 0, 243, 244, 3, 28, 14, 0, 244, 245, 5, 5, 0, 0, 245,
		273, 1, 0, 0, 0, 246, 247, 5, 13, 0, 0, 247, 273, 3, 40, 20, 22, 248, 249,
		5, 14, 0, 0, 249, 273, 3, 40, 20, 21, 250, 251, 5, 15, 0, 0, 251, 273,
		3, 40, 20, 20, 252, 253, 5, 16, 0, 0, 253, 273, 3, 40, 20, 19, 254, 255,
		5, 17, 0, 0, 255, 273, 3, 40, 20, 18, 256, 257, 5, 25, 0, 0, 257, 273,
		3, 40, 20, 9, 258, 265, 3, 46, 23, 0, 259, 260, 5, 2, 0, 0, 260, 261, 3,
		40, 20, 0, 261, 262, 5, 3, 0, 0, 262, 266, 1, 0, 0, 0, 263, 264, 5, 12,
		0, 0, 264, 266, 3, 46, 23, 0, 265, 259, 1, 0, 0, 0, 265, 263, 1, 0, 0,
		0, 265, 266, 1, 0, 0, 0, 266, 267, 1, 0, 0, 0, 267, 268, 3, 48, 24, 0,
		268, 269, 3, 40, 20, 3, 269, 273, 1, 0, 0, 0, 270, 273, 3, 46, 23, 0, 271,
		273, 3, 42, 21, 0, 272, 240, 1, 0, 0, 0, 272, 242, 1, 0, 0, 0, 272, 246,
		1, 0, 0, 0, 272, 248, 1, 0, 0, 0, 272, 250, 1, 0, 0, 0, 272, 252, 1, 0,
		0, 0, 272, 254, 1, 0, 0, 0, 272, 256, 1, 0, 0, 0, 272, 258, 1, 0, 0, 0,
		272, 270, 1, 0, 0, 0, 272, 271, 1, 0, 0, 0, 273, 327, 1, 0, 0, 0, 274,
		275, 10, 15, 0, 0, 275, 276, 7, 2, 0, 0, 276, 326, 3, 40, 20, 16, 277,
		278, 10, 14, 0, 0, 278, 279, 7, 3, 0, 0, 279, 326, 3, 40, 20, 15, 280,
		281, 10, 13, 0, 0, 281, 282, 7, 4, 0, 0, 282, 326, 3, 40, 20, 14, 283,
		284, 10, 12, 0, 0, 284, 285, 7, 5, 0, 0, 285, 326, 3, 40, 20, 13, 286,
		287, 10, 11, 0, 0, 287, 288, 5, 47, 0, 0, 288, 326, 3, 40, 20, 12, 289,
		290, 10, 10, 0, 0, 290, 291, 7, 6, 0, 0, 291, 326, 3, 40, 20, 11, 292,
		293, 10, 8, 0, 0, 293, 294, 5, 18, 0, 0, 294, 326, 3, 40, 20, 9, 295, 296,
		10, 7, 0, 0, 296, 297, 5, 20, 0, 0, 297, 326, 3, 40, 20, 8, 298, 299, 10,
		6, 0, 0, 299, 300, 5, 19, 0, 0, 300, 326, 3, 40, 20, 7, 301, 302, 10, 5,
		0, 0, 302, 303, 5, 40, 0, 0, 303, 326, 3, 40, 20, 6, 304, 305, 10, 4, 0,
		0, 305, 306, 5, 41, 0, 0, 306, 326, 3, 40, 20, 5, 307, 308, 10, 26, 0,
		0, 308, 309, 5, 12, 0, 0, 309, 326, 5, 73, 0, 0, 310, 311, 10, 25, 0, 0,
		311, 313, 5, 4, 0, 0, 312, 314, 3, 24, 12, 0, 313, 312, 1, 0, 0, 0, 313,
		314, 1, 0, 0, 0, 314, 315, 1, 0, 0, 0, 315, 326, 5, 5, 0, 0, 316, 317,
		10, 23, 0, 0, 317, 318, 5, 2, 0, 0, 318, 319, 3, 40, 20, 0, 319, 320, 5,
		3, 0, 0, 320, 326, 1, 0, 0, 0, 321, 322, 10, 17, 0, 0, 322, 326, 5, 13,
		0, 0, 323, 324, 10, 16, 0, 0, 324, 326, 5, 14, 0, 0, 325, 274, 1, 0, 0,
		0, 325, 277, 1, 0, 0, 0, 325, 280, 1, 0, 0, 0, 325, 283, 1, 0, 0, 0, 325,
		286, 1, 0, 0, 0, 325, 289, 1, 0, 0, 0, 325, 292, 1, 0, 0, 0, 325, 295,
		1, 0, 0, 0, 325, 298, 1, 0, 0, 0, 325, 301, 1, 0, 0, 0, 325, 304, 1, 0,
		0, 0, 325, 307, 1, 0, 0, 0, 325, 310, 1, 0, 0, 0, 325, 316, 1, 0, 0, 0,
		325, 321, 1, 0, 0, 0, 325, 323, 1, 0, 0, 0, 326, 329, 1, 0, 0, 0, 327,
		325, 1, 0, 0, 0, 327, 328, 1, 0, 0, 0, 328, 41, 1, 0, 0, 0, 329, 327, 1,
		0, 0, 0, 330, 336, 5, 67, 0, 0, 331, 336, 5, 66, 0, 0, 332, 336, 3, 44,
		22, 0, 333, 336, 5, 70, 0, 0, 334, 336, 5, 72, 0, 0, 335, 330, 1, 0, 0,
		0, 335, 331, 1, 0, 0, 0, 335, 332, 1, 0, 0, 0, 335, 333, 1, 0, 0, 0, 335,
		334, 1, 0, 0, 0, 336, 43, 1, 0, 0, 0, 337, 338, 7, 7, 0, 0, 338, 45, 1,
		0, 0, 0, 339, 340, 7, 8, 0, 0, 340, 47, 1, 0, 0, 0, 341, 342, 7, 9, 0,
		0, 342, 49, 1, 0, 0, 0, 343, 345, 5, 8, 0, 0, 344, 343, 1, 0, 0, 0, 345,
		346, 1, 0, 0, 0, 346, 344, 1, 0, 0, 0, 346, 347, 1, 0, 0, 0, 347, 51, 1,
		0, 0, 0, 32, 53, 60, 84, 88, 91, 101, 108, 117, 122, 127, 131, 135, 139,
		161, 173, 180, 186, 195, 202, 212, 217, 225, 230, 235, 238, 265, 272, 313,
		325, 327, 335, 346,
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

// NaslParserInit initializes any static state used to implement NaslParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewNaslParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func NaslParserInit() {
	staticData := &naslparserParserStaticData
	staticData.once.Do(naslparserParserInit)
}

// NewNaslParser produces a new parser instance for the optional input antlr.TokenStream.
func NewNaslParser(input antlr.TokenStream) *NaslParser {
	NaslParserInit()
	this := new(NaslParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &naslparserParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.predictionContextCache)
	this.RuleNames = staticData.ruleNames
	this.LiteralNames = staticData.literalNames
	this.SymbolicNames = staticData.symbolicNames
	this.GrammarFileName = "java-escape"

	return this
}

// NaslParser tokens.
const (
	NaslParserEOF                        = antlr.TokenEOF
	NaslParserSingleLineComment          = 1
	NaslParserOpenBracket                = 2
	NaslParserCloseBracket               = 3
	NaslParserOpenParen                  = 4
	NaslParserCloseParen                 = 5
	NaslParserOpenBrace                  = 6
	NaslParserCloseBrace                 = 7
	NaslParserSemiColon                  = 8
	NaslParserComma                      = 9
	NaslParserAssign                     = 10
	NaslParserColon                      = 11
	NaslParserDot                        = 12
	NaslParserPlusPlus                   = 13
	NaslParserMinusMinus                 = 14
	NaslParserPlus                       = 15
	NaslParserMinus                      = 16
	NaslParserBitNot                     = 17
	NaslParserBitAnd                     = 18
	NaslParserBitXOr                     = 19
	NaslParserBitOr                      = 20
	NaslParserRightShiftArithmetic       = 21
	NaslParserLeftShiftArithmetic        = 22
	NaslParserLeftShiftLogical           = 23
	NaslParserRightShiftLogical          = 24
	NaslParserNot                        = 25
	NaslParserMultiply                   = 26
	NaslParserPow                        = 27
	NaslParserDivide                     = 28
	NaslParserModulus                    = 29
	NaslParserLessThan                   = 30
	NaslParserMoreThan                   = 31
	NaslParserLessThanEquals             = 32
	NaslParserGreaterThanEquals          = 33
	NaslParserEquals_                    = 34
	NaslParserEqualsRe                   = 35
	NaslParserNotEquals                  = 36
	NaslParserNotLong                    = 37
	NaslParserMTNotLT                    = 38
	NaslParserMTLT                       = 39
	NaslParserAnd                        = 40
	NaslParserOr                         = 41
	NaslParserMultiplyAssign             = 42
	NaslParserDivideAssign               = 43
	NaslParserModulusAssign              = 44
	NaslParserPlusAssign                 = 45
	NaslParserMinusAssign                = 46
	NaslParserX                          = 47
	NaslParserRightShiftLogicalAssign    = 48
	NaslParserLeftShiftLogicalAssign     = 49
	NaslParserRightShiftArithmeticAssign = 50
	NaslParserLeftShiftArithmeticAssign  = 51
	NaslParserBreak                      = 52
	NaslParserVar                        = 53
	NaslParserLocalVar                   = 54
	NaslParserGlobalVar                  = 55
	NaslParserElse                       = 56
	NaslParserReturn                     = 57
	NaslParserContinue                   = 58
	NaslParserFor                        = 59
	NaslParserForEach                    = 60
	NaslParserIf                         = 61
	NaslParserFunction_                  = 62
	NaslParserRepeat                     = 63
	NaslParserWhile                      = 64
	NaslParserUntil                      = 65
	NaslParserStringLiteral              = 66
	NaslParserBooleanLiteral             = 67
	NaslParserIntegerLiteral             = 68
	NaslParserFloatLiteral               = 69
	NaslParserIpLiteral                  = 70
	NaslParserHexLiteral                 = 71
	NaslParserNULLLiteral                = 72
	NaslParserIdentifier                 = 73
	NaslParserWhiteSpaces                = 74
	NaslParserLineTerminator             = 75
)

// NaslParser rules.
const (
	NaslParserRULE_program                      = 0
	NaslParserRULE_statementList                = 1
	NaslParserRULE_statement                    = 2
	NaslParserRULE_block                        = 3
	NaslParserRULE_variableDeclarationStatement = 4
	NaslParserRULE_variableAssignStatement      = 5
	NaslParserRULE_expressionStatement          = 6
	NaslParserRULE_ifStatement                  = 7
	NaslParserRULE_iterationStatement           = 8
	NaslParserRULE_continueStatement            = 9
	NaslParserRULE_breakStatement               = 10
	NaslParserRULE_returnStatement              = 11
	NaslParserRULE_argumentList                 = 12
	NaslParserRULE_argument                     = 13
	NaslParserRULE_expressionSequence           = 14
	NaslParserRULE_functionDeclarationStatement = 15
	NaslParserRULE_parameterList                = 16
	NaslParserRULE_arrayLiteral                 = 17
	NaslParserRULE_elementList                  = 18
	NaslParserRULE_arrayElement                 = 19
	NaslParserRULE_singleExpression             = 20
	NaslParserRULE_literal                      = 21
	NaslParserRULE_numericLiteral               = 22
	NaslParserRULE_identifier                   = 23
	NaslParserRULE_assignmentOperator           = 24
	NaslParserRULE_eos                          = 25
)

// IProgramContext is an interface to support dynamic dispatch.
type IProgramContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsProgramContext differentiates from other interfaces.
	IsProgramContext()
}

type ProgramContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyProgramContext() *ProgramContext {
	var p = new(ProgramContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_program
	return p
}

func (*ProgramContext) IsProgramContext() {}

func NewProgramContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ProgramContext {
	var p = new(ProgramContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_program

	return p
}

func (s *ProgramContext) GetParser() antlr.Parser { return s.parser }

func (s *ProgramContext) EOF() antlr.TerminalNode {
	return s.GetToken(NaslParserEOF, 0)
}

func (s *ProgramContext) StatementList() IStatementListContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementListContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatementListContext)
}

func (s *ProgramContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ProgramContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ProgramContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitProgram(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) Program() (localctx IProgramContext) {
	this := p
	_ = this

	localctx = NewProgramContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, NaslParserRULE_program)
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
	p.SetState(53)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-76420456143134636) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&1021) != 0 {
		{
			p.SetState(52)
			p.StatementList()
		}

	}
	{
		p.SetState(55)
		p.Match(NaslParserEOF)
	}

	return localctx
}

// IStatementListContext is an interface to support dynamic dispatch.
type IStatementListContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStatementListContext differentiates from other interfaces.
	IsStatementListContext()
}

type StatementListContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStatementListContext() *StatementListContext {
	var p = new(StatementListContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_statementList
	return p
}

func (*StatementListContext) IsStatementListContext() {}

func NewStatementListContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StatementListContext {
	var p = new(StatementListContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_statementList

	return p
}

func (s *StatementListContext) GetParser() antlr.Parser { return s.parser }

func (s *StatementListContext) AllStatement() []IStatementContext {
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

func (s *StatementListContext) Statement(i int) IStatementContext {
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

func (s *StatementListContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StatementListContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StatementListContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitStatementList(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) StatementList() (localctx IStatementListContext) {
	this := p
	_ = this

	localctx = NewStatementListContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, NaslParserRULE_statementList)
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
	p.SetState(58)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-76420456143134636) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&1021) != 0 {
		{
			p.SetState(57)
			p.Statement()
		}

		p.SetState(60)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
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
	p.RuleIndex = NaslParserRULE_statement
	return p
}

func (*StatementContext) IsStatementContext() {}

func NewStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StatementContext {
	var p = new(StatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_statement

	return p
}

func (s *StatementContext) GetParser() antlr.Parser { return s.parser }

func (s *StatementContext) Block() IBlockContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBlockContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBlockContext)
}

func (s *StatementContext) IfStatement() IIfStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIfStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIfStatementContext)
}

func (s *StatementContext) IterationStatement() IIterationStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIterationStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIterationStatementContext)
}

func (s *StatementContext) ContinueStatement() IContinueStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IContinueStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IContinueStatementContext)
}

func (s *StatementContext) Eos() IEosContext {
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

func (s *StatementContext) BreakStatement() IBreakStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBreakStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBreakStatementContext)
}

func (s *StatementContext) ReturnStatement() IReturnStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IReturnStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IReturnStatementContext)
}

func (s *StatementContext) ExpressionStatement() IExpressionStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExpressionStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExpressionStatementContext)
}

func (s *StatementContext) VariableDeclarationStatement() IVariableDeclarationStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IVariableDeclarationStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IVariableDeclarationStatementContext)
}

func (s *StatementContext) VariableAssignStatement() IVariableAssignStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IVariableAssignStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IVariableAssignStatementContext)
}

func (s *StatementContext) FunctionDeclarationStatement() IFunctionDeclarationStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFunctionDeclarationStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFunctionDeclarationStatementContext)
}

func (s *StatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) Statement() (localctx IStatementContext) {
	this := p
	_ = this

	localctx = NewStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, NaslParserRULE_statement)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 2, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(62)
			p.Block()
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(63)
			p.IfStatement()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(64)
			p.IterationStatement()
		}

	case 4:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(65)
			p.ContinueStatement()
		}
		{
			p.SetState(66)
			p.Eos()
		}

	case 5:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(68)
			p.BreakStatement()
		}
		{
			p.SetState(69)
			p.Eos()
		}

	case 6:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(71)
			p.ReturnStatement()
		}
		{
			p.SetState(72)
			p.Eos()
		}

	case 7:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(74)
			p.ExpressionStatement()
		}
		{
			p.SetState(75)
			p.Eos()
		}

	case 8:
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(77)
			p.VariableDeclarationStatement()
		}
		{
			p.SetState(78)
			p.Eos()
		}

	case 9:
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(80)
			p.VariableAssignStatement()
		}
		{
			p.SetState(81)
			p.Eos()
		}

	case 10:
		p.EnterOuterAlt(localctx, 10)
		{
			p.SetState(83)
			p.FunctionDeclarationStatement()
		}

	}

	return localctx
}

// IBlockContext is an interface to support dynamic dispatch.
type IBlockContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsBlockContext differentiates from other interfaces.
	IsBlockContext()
}

type BlockContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyBlockContext() *BlockContext {
	var p = new(BlockContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_block
	return p
}

func (*BlockContext) IsBlockContext() {}

func NewBlockContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *BlockContext {
	var p = new(BlockContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_block

	return p
}

func (s *BlockContext) GetParser() antlr.Parser { return s.parser }

func (s *BlockContext) OpenBrace() antlr.TerminalNode {
	return s.GetToken(NaslParserOpenBrace, 0)
}

func (s *BlockContext) CloseBrace() antlr.TerminalNode {
	return s.GetToken(NaslParserCloseBrace, 0)
}

func (s *BlockContext) Eos() IEosContext {
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

func (s *BlockContext) StatementList() IStatementListContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementListContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatementListContext)
}

func (s *BlockContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BlockContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *BlockContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitBlock(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) Block() (localctx IBlockContext) {
	this := p
	_ = this

	localctx = NewBlockContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, NaslParserRULE_block)
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
		p.SetState(86)
		p.Match(NaslParserOpenBrace)
	}
	p.SetState(88)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == NaslParserSemiColon {
		{
			p.SetState(87)
			p.Eos()
		}

	}
	p.SetState(91)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&-76420456143134636) != 0 || (int64((_la-64)) & ^0x3f) == 0 && ((int64(1)<<(_la-64))&1021) != 0 {
		{
			p.SetState(90)
			p.StatementList()
		}

	}
	{
		p.SetState(93)
		p.Match(NaslParserCloseBrace)
	}

	return localctx
}

// IVariableDeclarationStatementContext is an interface to support dynamic dispatch.
type IVariableDeclarationStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsVariableDeclarationStatementContext differentiates from other interfaces.
	IsVariableDeclarationStatementContext()
}

type VariableDeclarationStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyVariableDeclarationStatementContext() *VariableDeclarationStatementContext {
	var p = new(VariableDeclarationStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_variableDeclarationStatement
	return p
}

func (*VariableDeclarationStatementContext) IsVariableDeclarationStatementContext() {}

func NewVariableDeclarationStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *VariableDeclarationStatementContext {
	var p = new(VariableDeclarationStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_variableDeclarationStatement

	return p
}

func (s *VariableDeclarationStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *VariableDeclarationStatementContext) AllIdentifier() []IIdentifierContext {
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

func (s *VariableDeclarationStatementContext) Identifier(i int) IIdentifierContext {
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

func (s *VariableDeclarationStatementContext) GlobalVar() antlr.TerminalNode {
	return s.GetToken(NaslParserGlobalVar, 0)
}

func (s *VariableDeclarationStatementContext) LocalVar() antlr.TerminalNode {
	return s.GetToken(NaslParserLocalVar, 0)
}

func (s *VariableDeclarationStatementContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(NaslParserComma)
}

func (s *VariableDeclarationStatementContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(NaslParserComma, i)
}

func (s *VariableDeclarationStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *VariableDeclarationStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *VariableDeclarationStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitVariableDeclarationStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) VariableDeclarationStatement() (localctx IVariableDeclarationStatementContext) {
	this := p
	_ = this

	localctx = NewVariableDeclarationStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, NaslParserRULE_variableDeclarationStatement)
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
		p.SetState(95)
		_la = p.GetTokenStream().LA(1)

		if !(_la == NaslParserLocalVar || _la == NaslParserGlobalVar) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}
	{
		p.SetState(96)
		p.Identifier()
	}
	p.SetState(101)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == NaslParserComma {
		{
			p.SetState(97)
			p.Match(NaslParserComma)
		}
		{
			p.SetState(98)
			p.Identifier()
		}

		p.SetState(103)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// IVariableAssignStatementContext is an interface to support dynamic dispatch.
type IVariableAssignStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsVariableAssignStatementContext differentiates from other interfaces.
	IsVariableAssignStatementContext()
}

type VariableAssignStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyVariableAssignStatementContext() *VariableAssignStatementContext {
	var p = new(VariableAssignStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_variableAssignStatement
	return p
}

func (*VariableAssignStatementContext) IsVariableAssignStatementContext() {}

func NewVariableAssignStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *VariableAssignStatementContext {
	var p = new(VariableAssignStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_variableAssignStatement

	return p
}

func (s *VariableAssignStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *VariableAssignStatementContext) Identifier() IIdentifierContext {
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

func (s *VariableAssignStatementContext) GlobalVar() antlr.TerminalNode {
	return s.GetToken(NaslParserGlobalVar, 0)
}

func (s *VariableAssignStatementContext) LocalVar() antlr.TerminalNode {
	return s.GetToken(NaslParserLocalVar, 0)
}

func (s *VariableAssignStatementContext) Var() antlr.TerminalNode {
	return s.GetToken(NaslParserVar, 0)
}

func (s *VariableAssignStatementContext) Assign() antlr.TerminalNode {
	return s.GetToken(NaslParserAssign, 0)
}

func (s *VariableAssignStatementContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *VariableAssignStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *VariableAssignStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *VariableAssignStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitVariableAssignStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) VariableAssignStatement() (localctx IVariableAssignStatementContext) {
	this := p
	_ = this

	localctx = NewVariableAssignStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, NaslParserRULE_variableAssignStatement)
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
		p.SetState(104)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&63050394783186944) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}
	{
		p.SetState(105)
		p.Identifier()
	}
	p.SetState(108)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == NaslParserAssign {
		{
			p.SetState(106)
			p.Match(NaslParserAssign)
		}
		{
			p.SetState(107)
			p.singleExpression(0)
		}

	}

	return localctx
}

// IExpressionStatementContext is an interface to support dynamic dispatch.
type IExpressionStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsExpressionStatementContext differentiates from other interfaces.
	IsExpressionStatementContext()
}

type ExpressionStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyExpressionStatementContext() *ExpressionStatementContext {
	var p = new(ExpressionStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_expressionStatement
	return p
}

func (*ExpressionStatementContext) IsExpressionStatementContext() {}

func NewExpressionStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ExpressionStatementContext {
	var p = new(ExpressionStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_expressionStatement

	return p
}

func (s *ExpressionStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *ExpressionStatementContext) ExpressionSequence() IExpressionSequenceContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExpressionSequenceContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExpressionSequenceContext)
}

func (s *ExpressionStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExpressionStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ExpressionStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitExpressionStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) ExpressionStatement() (localctx IExpressionStatementContext) {
	this := p
	_ = this

	localctx = NewExpressionStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, NaslParserRULE_expressionStatement)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(110)
		p.ExpressionSequence()
	}

	return localctx
}

// IIfStatementContext is an interface to support dynamic dispatch.
type IIfStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsIfStatementContext differentiates from other interfaces.
	IsIfStatementContext()
}

type IfStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIfStatementContext() *IfStatementContext {
	var p = new(IfStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_ifStatement
	return p
}

func (*IfStatementContext) IsIfStatementContext() {}

func NewIfStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *IfStatementContext {
	var p = new(IfStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_ifStatement

	return p
}

func (s *IfStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *IfStatementContext) If() antlr.TerminalNode {
	return s.GetToken(NaslParserIf, 0)
}

func (s *IfStatementContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(NaslParserOpenParen, 0)
}

func (s *IfStatementContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *IfStatementContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(NaslParserCloseParen, 0)
}

func (s *IfStatementContext) AllStatement() []IStatementContext {
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

func (s *IfStatementContext) Statement(i int) IStatementContext {
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

func (s *IfStatementContext) Eos() IEosContext {
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

func (s *IfStatementContext) Else() antlr.TerminalNode {
	return s.GetToken(NaslParserElse, 0)
}

func (s *IfStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *IfStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *IfStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitIfStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) IfStatement() (localctx IIfStatementContext) {
	this := p
	_ = this

	localctx = NewIfStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 14, NaslParserRULE_ifStatement)
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
		p.SetState(112)
		p.Match(NaslParserIf)
	}
	{
		p.SetState(113)
		p.Match(NaslParserOpenParen)
	}
	{
		p.SetState(114)
		p.singleExpression(0)
	}
	{
		p.SetState(115)
		p.Match(NaslParserCloseParen)
	}
	p.SetState(117)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == NaslParserSemiColon {
		{
			p.SetState(116)
			p.Eos()
		}

	}
	{
		p.SetState(119)
		p.Statement()
	}
	p.SetState(122)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 8, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(120)
			p.Match(NaslParserElse)
		}
		{
			p.SetState(121)
			p.Statement()
		}

	}

	return localctx
}

// IIterationStatementContext is an interface to support dynamic dispatch.
type IIterationStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsIterationStatementContext differentiates from other interfaces.
	IsIterationStatementContext()
}

type IterationStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIterationStatementContext() *IterationStatementContext {
	var p = new(IterationStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_iterationStatement
	return p
}

func (*IterationStatementContext) IsIterationStatementContext() {}

func NewIterationStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *IterationStatementContext {
	var p = new(IterationStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_iterationStatement

	return p
}

func (s *IterationStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *IterationStatementContext) CopyFrom(ctx *IterationStatementContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *IterationStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *IterationStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type TraditionalForContext struct {
	*IterationStatementContext
}

func NewTraditionalForContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *TraditionalForContext {
	var p = new(TraditionalForContext)

	p.IterationStatementContext = NewEmptyIterationStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*IterationStatementContext))

	return p
}

func (s *TraditionalForContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TraditionalForContext) For() antlr.TerminalNode {
	return s.GetToken(NaslParserFor, 0)
}

func (s *TraditionalForContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(NaslParserOpenParen, 0)
}

func (s *TraditionalForContext) AllSemiColon() []antlr.TerminalNode {
	return s.GetTokens(NaslParserSemiColon)
}

func (s *TraditionalForContext) SemiColon(i int) antlr.TerminalNode {
	return s.GetToken(NaslParserSemiColon, i)
}

func (s *TraditionalForContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(NaslParserCloseParen, 0)
}

func (s *TraditionalForContext) Statement() IStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatementContext)
}

func (s *TraditionalForContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *TraditionalForContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *TraditionalForContext) Eos() IEosContext {
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

func (s *TraditionalForContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitTraditionalFor(s)

	default:
		return t.VisitChildren(s)
	}
}

type RepeatContext struct {
	*IterationStatementContext
}

func NewRepeatContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *RepeatContext {
	var p = new(RepeatContext)

	p.IterationStatementContext = NewEmptyIterationStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*IterationStatementContext))

	return p
}

func (s *RepeatContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RepeatContext) Repeat() antlr.TerminalNode {
	return s.GetToken(NaslParserRepeat, 0)
}

func (s *RepeatContext) Statement() IStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatementContext)
}

func (s *RepeatContext) Until() antlr.TerminalNode {
	return s.GetToken(NaslParserUntil, 0)
}

func (s *RepeatContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *RepeatContext) Eos() IEosContext {
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

func (s *RepeatContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitRepeat(s)

	default:
		return t.VisitChildren(s)
	}
}

type WhileContext struct {
	*IterationStatementContext
}

func NewWhileContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *WhileContext {
	var p = new(WhileContext)

	p.IterationStatementContext = NewEmptyIterationStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*IterationStatementContext))

	return p
}

func (s *WhileContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *WhileContext) While() antlr.TerminalNode {
	return s.GetToken(NaslParserWhile, 0)
}

func (s *WhileContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(NaslParserOpenParen, 0)
}

func (s *WhileContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *WhileContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(NaslParserCloseParen, 0)
}

func (s *WhileContext) Statement() IStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatementContext)
}

func (s *WhileContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitWhile(s)

	default:
		return t.VisitChildren(s)
	}
}

type ForEachContext struct {
	*IterationStatementContext
}

func NewForEachContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ForEachContext {
	var p = new(ForEachContext)

	p.IterationStatementContext = NewEmptyIterationStatementContext()
	p.parser = parser
	p.CopyFrom(ctx.(*IterationStatementContext))

	return p
}

func (s *ForEachContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ForEachContext) ForEach() antlr.TerminalNode {
	return s.GetToken(NaslParserForEach, 0)
}

func (s *ForEachContext) Identifier() IIdentifierContext {
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

func (s *ForEachContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(NaslParserOpenParen, 0)
}

func (s *ForEachContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *ForEachContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(NaslParserCloseParen, 0)
}

func (s *ForEachContext) Statement() IStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatementContext)
}

func (s *ForEachContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitForEach(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) IterationStatement() (localctx IIterationStatementContext) {
	this := p
	_ = this

	localctx = NewIterationStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 16, NaslParserRULE_iterationStatement)
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

	p.SetState(161)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case NaslParserFor:
		localctx = NewTraditionalForContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(124)
			p.Match(NaslParserFor)
		}
		{
			p.SetState(125)
			p.Match(NaslParserOpenParen)
		}
		p.SetState(127)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&140737522163732) != 0 || (int64((_la-66)) & ^0x3f) == 0 && ((int64(1)<<(_la-66))&255) != 0 {
			{
				p.SetState(126)
				p.singleExpression(0)
			}

		}
		{
			p.SetState(129)
			p.Match(NaslParserSemiColon)
		}
		p.SetState(131)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&140737522163732) != 0 || (int64((_la-66)) & ^0x3f) == 0 && ((int64(1)<<(_la-66))&255) != 0 {
			{
				p.SetState(130)
				p.singleExpression(0)
			}

		}
		{
			p.SetState(133)
			p.Match(NaslParserSemiColon)
		}
		p.SetState(135)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&140737522163732) != 0 || (int64((_la-66)) & ^0x3f) == 0 && ((int64(1)<<(_la-66))&255) != 0 {
			{
				p.SetState(134)
				p.singleExpression(0)
			}

		}
		{
			p.SetState(137)
			p.Match(NaslParserCloseParen)
		}
		p.SetState(139)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == NaslParserSemiColon {
			{
				p.SetState(138)
				p.Eos()
			}

		}
		{
			p.SetState(141)
			p.Statement()
		}

	case NaslParserForEach:
		localctx = NewForEachContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(142)
			p.Match(NaslParserForEach)
		}
		{
			p.SetState(143)
			p.Identifier()
		}
		{
			p.SetState(144)
			p.Match(NaslParserOpenParen)
		}
		{
			p.SetState(145)
			p.singleExpression(0)
		}
		{
			p.SetState(146)
			p.Match(NaslParserCloseParen)
		}
		{
			p.SetState(147)
			p.Statement()
		}

	case NaslParserWhile:
		localctx = NewWhileContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(149)
			p.Match(NaslParserWhile)
		}
		{
			p.SetState(150)
			p.Match(NaslParserOpenParen)
		}
		{
			p.SetState(151)
			p.singleExpression(0)
		}
		{
			p.SetState(152)
			p.Match(NaslParserCloseParen)
		}
		{
			p.SetState(153)
			p.Statement()
		}

	case NaslParserRepeat:
		localctx = NewRepeatContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(155)
			p.Match(NaslParserRepeat)
		}
		{
			p.SetState(156)
			p.Statement()
		}
		{
			p.SetState(157)
			p.Match(NaslParserUntil)
		}
		{
			p.SetState(158)
			p.singleExpression(0)
		}
		{
			p.SetState(159)
			p.Eos()
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IContinueStatementContext is an interface to support dynamic dispatch.
type IContinueStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsContinueStatementContext differentiates from other interfaces.
	IsContinueStatementContext()
}

type ContinueStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyContinueStatementContext() *ContinueStatementContext {
	var p = new(ContinueStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_continueStatement
	return p
}

func (*ContinueStatementContext) IsContinueStatementContext() {}

func NewContinueStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ContinueStatementContext {
	var p = new(ContinueStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_continueStatement

	return p
}

func (s *ContinueStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *ContinueStatementContext) Continue() antlr.TerminalNode {
	return s.GetToken(NaslParserContinue, 0)
}

func (s *ContinueStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ContinueStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ContinueStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitContinueStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) ContinueStatement() (localctx IContinueStatementContext) {
	this := p
	_ = this

	localctx = NewContinueStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 18, NaslParserRULE_continueStatement)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(163)
		p.Match(NaslParserContinue)
	}

	return localctx
}

// IBreakStatementContext is an interface to support dynamic dispatch.
type IBreakStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsBreakStatementContext differentiates from other interfaces.
	IsBreakStatementContext()
}

type BreakStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyBreakStatementContext() *BreakStatementContext {
	var p = new(BreakStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_breakStatement
	return p
}

func (*BreakStatementContext) IsBreakStatementContext() {}

func NewBreakStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *BreakStatementContext {
	var p = new(BreakStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_breakStatement

	return p
}

func (s *BreakStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *BreakStatementContext) Break() antlr.TerminalNode {
	return s.GetToken(NaslParserBreak, 0)
}

func (s *BreakStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BreakStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *BreakStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitBreakStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) BreakStatement() (localctx IBreakStatementContext) {
	this := p
	_ = this

	localctx = NewBreakStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 20, NaslParserRULE_breakStatement)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.Match(NaslParserBreak)
	}

	return localctx
}

// IReturnStatementContext is an interface to support dynamic dispatch.
type IReturnStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsReturnStatementContext differentiates from other interfaces.
	IsReturnStatementContext()
}

type ReturnStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyReturnStatementContext() *ReturnStatementContext {
	var p = new(ReturnStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_returnStatement
	return p
}

func (*ReturnStatementContext) IsReturnStatementContext() {}

func NewReturnStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ReturnStatementContext {
	var p = new(ReturnStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_returnStatement

	return p
}

func (s *ReturnStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *ReturnStatementContext) Return() antlr.TerminalNode {
	return s.GetToken(NaslParserReturn, 0)
}

func (s *ReturnStatementContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(NaslParserOpenParen, 0)
}

func (s *ReturnStatementContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *ReturnStatementContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(NaslParserCloseParen, 0)
}

func (s *ReturnStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ReturnStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ReturnStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitReturnStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) ReturnStatement() (localctx IReturnStatementContext) {
	this := p
	_ = this

	localctx = NewReturnStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 22, NaslParserRULE_returnStatement)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
		p.SetState(167)
		p.Match(NaslParserReturn)
	}
	p.SetState(173)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 14, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(168)
			p.Match(NaslParserOpenParen)
		}
		{
			p.SetState(169)
			p.singleExpression(0)
		}
		{
			p.SetState(170)
			p.Match(NaslParserCloseParen)
		}

	} else if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 14, p.GetParserRuleContext()) == 2 {
		{
			p.SetState(172)
			p.singleExpression(0)
		}

	}

	return localctx
}

// IArgumentListContext is an interface to support dynamic dispatch.
type IArgumentListContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsArgumentListContext differentiates from other interfaces.
	IsArgumentListContext()
}

type ArgumentListContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyArgumentListContext() *ArgumentListContext {
	var p = new(ArgumentListContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_argumentList
	return p
}

func (*ArgumentListContext) IsArgumentListContext() {}

func NewArgumentListContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ArgumentListContext {
	var p = new(ArgumentListContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_argumentList

	return p
}

func (s *ArgumentListContext) GetParser() antlr.Parser { return s.parser }

func (s *ArgumentListContext) AllArgument() []IArgumentContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IArgumentContext); ok {
			len++
		}
	}

	tst := make([]IArgumentContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IArgumentContext); ok {
			tst[i] = t.(IArgumentContext)
			i++
		}
	}

	return tst
}

func (s *ArgumentListContext) Argument(i int) IArgumentContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IArgumentContext); ok {
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

	return t.(IArgumentContext)
}

func (s *ArgumentListContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(NaslParserComma)
}

func (s *ArgumentListContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(NaslParserComma, i)
}

func (s *ArgumentListContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ArgumentListContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ArgumentListContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitArgumentList(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) ArgumentList() (localctx IArgumentListContext) {
	this := p
	_ = this

	localctx = NewArgumentListContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, NaslParserRULE_argumentList)
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
		p.SetState(175)
		p.Argument()
	}
	p.SetState(180)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == NaslParserComma {
		{
			p.SetState(176)
			p.Match(NaslParserComma)
		}
		{
			p.SetState(177)
			p.Argument()
		}

		p.SetState(182)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// IArgumentContext is an interface to support dynamic dispatch.
type IArgumentContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsArgumentContext differentiates from other interfaces.
	IsArgumentContext()
}

type ArgumentContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyArgumentContext() *ArgumentContext {
	var p = new(ArgumentContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_argument
	return p
}

func (*ArgumentContext) IsArgumentContext() {}

func NewArgumentContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ArgumentContext {
	var p = new(ArgumentContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_argument

	return p
}

func (s *ArgumentContext) GetParser() antlr.Parser { return s.parser }

func (s *ArgumentContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *ArgumentContext) Identifier() IIdentifierContext {
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

func (s *ArgumentContext) Colon() antlr.TerminalNode {
	return s.GetToken(NaslParserColon, 0)
}

func (s *ArgumentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ArgumentContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ArgumentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitArgument(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) Argument() (localctx IArgumentContext) {
	this := p
	_ = this

	localctx = NewArgumentContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, NaslParserRULE_argument)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	p.SetState(186)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 16, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(183)
			p.Identifier()
		}
		{
			p.SetState(184)
			p.Match(NaslParserColon)
		}

	}
	{
		p.SetState(188)
		p.singleExpression(0)
	}

	return localctx
}

// IExpressionSequenceContext is an interface to support dynamic dispatch.
type IExpressionSequenceContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsExpressionSequenceContext differentiates from other interfaces.
	IsExpressionSequenceContext()
}

type ExpressionSequenceContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyExpressionSequenceContext() *ExpressionSequenceContext {
	var p = new(ExpressionSequenceContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_expressionSequence
	return p
}

func (*ExpressionSequenceContext) IsExpressionSequenceContext() {}

func NewExpressionSequenceContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ExpressionSequenceContext {
	var p = new(ExpressionSequenceContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_expressionSequence

	return p
}

func (s *ExpressionSequenceContext) GetParser() antlr.Parser { return s.parser }

func (s *ExpressionSequenceContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *ExpressionSequenceContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *ExpressionSequenceContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(NaslParserComma)
}

func (s *ExpressionSequenceContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(NaslParserComma, i)
}

func (s *ExpressionSequenceContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExpressionSequenceContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ExpressionSequenceContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitExpressionSequence(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) ExpressionSequence() (localctx IExpressionSequenceContext) {
	this := p
	_ = this

	localctx = NewExpressionSequenceContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 28, NaslParserRULE_expressionSequence)
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
		p.SetState(190)
		p.singleExpression(0)
	}
	p.SetState(195)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == NaslParserComma {
		{
			p.SetState(191)
			p.Match(NaslParserComma)
		}
		{
			p.SetState(192)
			p.singleExpression(0)
		}

		p.SetState(197)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// IFunctionDeclarationStatementContext is an interface to support dynamic dispatch.
type IFunctionDeclarationStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsFunctionDeclarationStatementContext differentiates from other interfaces.
	IsFunctionDeclarationStatementContext()
}

type FunctionDeclarationStatementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFunctionDeclarationStatementContext() *FunctionDeclarationStatementContext {
	var p = new(FunctionDeclarationStatementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_functionDeclarationStatement
	return p
}

func (*FunctionDeclarationStatementContext) IsFunctionDeclarationStatementContext() {}

func NewFunctionDeclarationStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FunctionDeclarationStatementContext {
	var p = new(FunctionDeclarationStatementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_functionDeclarationStatement

	return p
}

func (s *FunctionDeclarationStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *FunctionDeclarationStatementContext) Function_() antlr.TerminalNode {
	return s.GetToken(NaslParserFunction_, 0)
}

func (s *FunctionDeclarationStatementContext) Identifier() IIdentifierContext {
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

func (s *FunctionDeclarationStatementContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(NaslParserOpenParen, 0)
}

func (s *FunctionDeclarationStatementContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(NaslParserCloseParen, 0)
}

func (s *FunctionDeclarationStatementContext) Block() IBlockContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBlockContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBlockContext)
}

func (s *FunctionDeclarationStatementContext) ParameterList() IParameterListContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IParameterListContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IParameterListContext)
}

func (s *FunctionDeclarationStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FunctionDeclarationStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FunctionDeclarationStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitFunctionDeclarationStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) FunctionDeclarationStatement() (localctx IFunctionDeclarationStatementContext) {
	this := p
	_ = this

	localctx = NewFunctionDeclarationStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 30, NaslParserRULE_functionDeclarationStatement)
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
		p.Match(NaslParserFunction_)
	}
	{
		p.SetState(199)
		p.Identifier()
	}
	{
		p.SetState(200)
		p.Match(NaslParserOpenParen)
	}
	p.SetState(202)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == NaslParserX || _la == NaslParserIdentifier {
		{
			p.SetState(201)
			p.ParameterList()
		}

	}
	{
		p.SetState(204)
		p.Match(NaslParserCloseParen)
	}
	{
		p.SetState(205)
		p.Block()
	}

	return localctx
}

// IParameterListContext is an interface to support dynamic dispatch.
type IParameterListContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsParameterListContext differentiates from other interfaces.
	IsParameterListContext()
}

type ParameterListContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyParameterListContext() *ParameterListContext {
	var p = new(ParameterListContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_parameterList
	return p
}

func (*ParameterListContext) IsParameterListContext() {}

func NewParameterListContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ParameterListContext {
	var p = new(ParameterListContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_parameterList

	return p
}

func (s *ParameterListContext) GetParser() antlr.Parser { return s.parser }

func (s *ParameterListContext) AllIdentifier() []IIdentifierContext {
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

func (s *ParameterListContext) Identifier(i int) IIdentifierContext {
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

func (s *ParameterListContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(NaslParserComma)
}

func (s *ParameterListContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(NaslParserComma, i)
}

func (s *ParameterListContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ParameterListContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ParameterListContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitParameterList(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) ParameterList() (localctx IParameterListContext) {
	this := p
	_ = this

	localctx = NewParameterListContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 32, NaslParserRULE_parameterList)
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
		p.SetState(207)
		p.Identifier()
	}
	p.SetState(212)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == NaslParserComma {
		{
			p.SetState(208)
			p.Match(NaslParserComma)
		}
		{
			p.SetState(209)
			p.Identifier()
		}

		p.SetState(214)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// IArrayLiteralContext is an interface to support dynamic dispatch.
type IArrayLiteralContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsArrayLiteralContext differentiates from other interfaces.
	IsArrayLiteralContext()
}

type ArrayLiteralContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyArrayLiteralContext() *ArrayLiteralContext {
	var p = new(ArrayLiteralContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_arrayLiteral
	return p
}

func (*ArrayLiteralContext) IsArrayLiteralContext() {}

func NewArrayLiteralContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ArrayLiteralContext {
	var p = new(ArrayLiteralContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_arrayLiteral

	return p
}

func (s *ArrayLiteralContext) GetParser() antlr.Parser { return s.parser }

func (s *ArrayLiteralContext) OpenBracket() antlr.TerminalNode {
	return s.GetToken(NaslParserOpenBracket, 0)
}

func (s *ArrayLiteralContext) CloseBracket() antlr.TerminalNode {
	return s.GetToken(NaslParserCloseBracket, 0)
}

func (s *ArrayLiteralContext) ElementList() IElementListContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IElementListContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IElementListContext)
}

func (s *ArrayLiteralContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ArrayLiteralContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ArrayLiteralContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitArrayLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) ArrayLiteral() (localctx IArrayLiteralContext) {
	this := p
	_ = this

	localctx = NewArrayLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 34, NaslParserRULE_arrayLiteral)
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
		p.SetState(215)
		p.Match(NaslParserOpenBracket)
	}
	p.SetState(217)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&140737522163732) != 0 || (int64((_la-66)) & ^0x3f) == 0 && ((int64(1)<<(_la-66))&255) != 0 {
		{
			p.SetState(216)
			p.ElementList()
		}

	}
	{
		p.SetState(219)
		p.Match(NaslParserCloseBracket)
	}

	return localctx
}

// IElementListContext is an interface to support dynamic dispatch.
type IElementListContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsElementListContext differentiates from other interfaces.
	IsElementListContext()
}

type ElementListContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyElementListContext() *ElementListContext {
	var p = new(ElementListContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_elementList
	return p
}

func (*ElementListContext) IsElementListContext() {}

func NewElementListContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ElementListContext {
	var p = new(ElementListContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_elementList

	return p
}

func (s *ElementListContext) GetParser() antlr.Parser { return s.parser }

func (s *ElementListContext) AllArrayElement() []IArrayElementContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IArrayElementContext); ok {
			len++
		}
	}

	tst := make([]IArrayElementContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IArrayElementContext); ok {
			tst[i] = t.(IArrayElementContext)
			i++
		}
	}

	return tst
}

func (s *ElementListContext) ArrayElement(i int) IArrayElementContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IArrayElementContext); ok {
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

	return t.(IArrayElementContext)
}

func (s *ElementListContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(NaslParserComma)
}

func (s *ElementListContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(NaslParserComma, i)
}

func (s *ElementListContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ElementListContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ElementListContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitElementList(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) ElementList() (localctx IElementListContext) {
	this := p
	_ = this

	localctx = NewElementListContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 36, NaslParserRULE_elementList)
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
		p.SetState(221)
		p.ArrayElement()
	}
	p.SetState(230)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == NaslParserComma {
		p.SetState(223)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == NaslParserComma {
			{
				p.SetState(222)
				p.Match(NaslParserComma)
			}

			p.SetState(225)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(227)
			p.ArrayElement()
		}

		p.SetState(232)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// IArrayElementContext is an interface to support dynamic dispatch.
type IArrayElementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsArrayElementContext differentiates from other interfaces.
	IsArrayElementContext()
}

type ArrayElementContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyArrayElementContext() *ArrayElementContext {
	var p = new(ArrayElementContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_arrayElement
	return p
}

func (*ArrayElementContext) IsArrayElementContext() {}

func NewArrayElementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ArrayElementContext {
	var p = new(ArrayElementContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_arrayElement

	return p
}

func (s *ArrayElementContext) GetParser() antlr.Parser { return s.parser }

func (s *ArrayElementContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *ArrayElementContext) Identifier() IIdentifierContext {
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

func (s *ArrayElementContext) Comma() antlr.TerminalNode {
	return s.GetToken(NaslParserComma, 0)
}

func (s *ArrayElementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ArrayElementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ArrayElementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitArrayElement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) ArrayElement() (localctx IArrayElementContext) {
	this := p
	_ = this

	localctx = NewArrayElementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 38, NaslParserRULE_arrayElement)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
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
	p.SetState(235)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 23, p.GetParserRuleContext()) {
	case 1:
		{
			p.SetState(233)
			p.singleExpression(0)
		}

	case 2:
		{
			p.SetState(234)
			p.Identifier()
		}

	}
	p.SetState(238)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 24, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(237)
			p.Match(NaslParserComma)
		}

	}

	return localctx
}

// ISingleExpressionContext is an interface to support dynamic dispatch.
type ISingleExpressionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsSingleExpressionContext differentiates from other interfaces.
	IsSingleExpressionContext()
}

type SingleExpressionContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySingleExpressionContext() *SingleExpressionContext {
	var p = new(SingleExpressionContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_singleExpression
	return p
}

func (*SingleExpressionContext) IsSingleExpressionContext() {}

func NewSingleExpressionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SingleExpressionContext {
	var p = new(SingleExpressionContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_singleExpression

	return p
}

func (s *SingleExpressionContext) GetParser() antlr.Parser { return s.parser }

func (s *SingleExpressionContext) CopyFrom(ctx *SingleExpressionContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *SingleExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SingleExpressionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type LogicalAndExpressionContext struct {
	*SingleExpressionContext
}

func NewLogicalAndExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *LogicalAndExpressionContext {
	var p = new(LogicalAndExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *LogicalAndExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LogicalAndExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *LogicalAndExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *LogicalAndExpressionContext) And() antlr.TerminalNode {
	return s.GetToken(NaslParserAnd, 0)
}

func (s *LogicalAndExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitLogicalAndExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type PreIncrementExpressionContext struct {
	*SingleExpressionContext
}

func NewPreIncrementExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *PreIncrementExpressionContext {
	var p = new(PreIncrementExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *PreIncrementExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PreIncrementExpressionContext) PlusPlus() antlr.TerminalNode {
	return s.GetToken(NaslParserPlusPlus, 0)
}

func (s *PreIncrementExpressionContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *PreIncrementExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitPreIncrementExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type LogicalOrExpressionContext struct {
	*SingleExpressionContext
}

func NewLogicalOrExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *LogicalOrExpressionContext {
	var p = new(LogicalOrExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *LogicalOrExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LogicalOrExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *LogicalOrExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *LogicalOrExpressionContext) Or() antlr.TerminalNode {
	return s.GetToken(NaslParserOr, 0)
}

func (s *LogicalOrExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitLogicalOrExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type NotExpressionContext struct {
	*SingleExpressionContext
}

func NewNotExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *NotExpressionContext {
	var p = new(NotExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *NotExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NotExpressionContext) Not() antlr.TerminalNode {
	return s.GetToken(NaslParserNot, 0)
}

func (s *NotExpressionContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *NotExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitNotExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type PreDecreaseExpressionContext struct {
	*SingleExpressionContext
}

func NewPreDecreaseExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *PreDecreaseExpressionContext {
	var p = new(PreDecreaseExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *PreDecreaseExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PreDecreaseExpressionContext) MinusMinus() antlr.TerminalNode {
	return s.GetToken(NaslParserMinusMinus, 0)
}

func (s *PreDecreaseExpressionContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *PreDecreaseExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitPreDecreaseExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type UnaryMinusExpressionContext struct {
	*SingleExpressionContext
}

func NewUnaryMinusExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *UnaryMinusExpressionContext {
	var p = new(UnaryMinusExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *UnaryMinusExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *UnaryMinusExpressionContext) Minus() antlr.TerminalNode {
	return s.GetToken(NaslParserMinus, 0)
}

func (s *UnaryMinusExpressionContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *UnaryMinusExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitUnaryMinusExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type AssignmentExpressionContext struct {
	*SingleExpressionContext
}

func NewAssignmentExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *AssignmentExpressionContext {
	var p = new(AssignmentExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *AssignmentExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AssignmentExpressionContext) AllIdentifier() []IIdentifierContext {
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

func (s *AssignmentExpressionContext) Identifier(i int) IIdentifierContext {
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

func (s *AssignmentExpressionContext) AssignmentOperator() IAssignmentOperatorContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAssignmentOperatorContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAssignmentOperatorContext)
}

func (s *AssignmentExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *AssignmentExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *AssignmentExpressionContext) OpenBracket() antlr.TerminalNode {
	return s.GetToken(NaslParserOpenBracket, 0)
}

func (s *AssignmentExpressionContext) CloseBracket() antlr.TerminalNode {
	return s.GetToken(NaslParserCloseBracket, 0)
}

func (s *AssignmentExpressionContext) Dot() antlr.TerminalNode {
	return s.GetToken(NaslParserDot, 0)
}

func (s *AssignmentExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitAssignmentExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type PostDecreaseExpressionContext struct {
	*SingleExpressionContext
}

func NewPostDecreaseExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *PostDecreaseExpressionContext {
	var p = new(PostDecreaseExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *PostDecreaseExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PostDecreaseExpressionContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *PostDecreaseExpressionContext) MinusMinus() antlr.TerminalNode {
	return s.GetToken(NaslParserMinusMinus, 0)
}

func (s *PostDecreaseExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitPostDecreaseExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type UnaryPlusExpressionContext struct {
	*SingleExpressionContext
}

func NewUnaryPlusExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *UnaryPlusExpressionContext {
	var p = new(UnaryPlusExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *UnaryPlusExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *UnaryPlusExpressionContext) Plus() antlr.TerminalNode {
	return s.GetToken(NaslParserPlus, 0)
}

func (s *UnaryPlusExpressionContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *UnaryPlusExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitUnaryPlusExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type EqualityExpressionContext struct {
	*SingleExpressionContext
}

func NewEqualityExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *EqualityExpressionContext {
	var p = new(EqualityExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *EqualityExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *EqualityExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *EqualityExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *EqualityExpressionContext) Equals_() antlr.TerminalNode {
	return s.GetToken(NaslParserEquals_, 0)
}

func (s *EqualityExpressionContext) MTNotLT() antlr.TerminalNode {
	return s.GetToken(NaslParserMTNotLT, 0)
}

func (s *EqualityExpressionContext) MTLT() antlr.TerminalNode {
	return s.GetToken(NaslParserMTLT, 0)
}

func (s *EqualityExpressionContext) NotEquals() antlr.TerminalNode {
	return s.GetToken(NaslParserNotEquals, 0)
}

func (s *EqualityExpressionContext) NotLong() antlr.TerminalNode {
	return s.GetToken(NaslParserNotLong, 0)
}

func (s *EqualityExpressionContext) EqualsRe() antlr.TerminalNode {
	return s.GetToken(NaslParserEqualsRe, 0)
}

func (s *EqualityExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitEqualityExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type BitXOrExpressionContext struct {
	*SingleExpressionContext
}

func NewBitXOrExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BitXOrExpressionContext {
	var p = new(BitXOrExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *BitXOrExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BitXOrExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *BitXOrExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *BitXOrExpressionContext) BitXOr() antlr.TerminalNode {
	return s.GetToken(NaslParserBitXOr, 0)
}

func (s *BitXOrExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitBitXOrExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type MultiplicativeExpressionContext struct {
	*SingleExpressionContext
}

func NewMultiplicativeExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *MultiplicativeExpressionContext {
	var p = new(MultiplicativeExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *MultiplicativeExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *MultiplicativeExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *MultiplicativeExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *MultiplicativeExpressionContext) Pow() antlr.TerminalNode {
	return s.GetToken(NaslParserPow, 0)
}

func (s *MultiplicativeExpressionContext) Multiply() antlr.TerminalNode {
	return s.GetToken(NaslParserMultiply, 0)
}

func (s *MultiplicativeExpressionContext) Divide() antlr.TerminalNode {
	return s.GetToken(NaslParserDivide, 0)
}

func (s *MultiplicativeExpressionContext) Modulus() antlr.TerminalNode {
	return s.GetToken(NaslParserModulus, 0)
}

func (s *MultiplicativeExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitMultiplicativeExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type CallExpressionContext struct {
	*SingleExpressionContext
}

func NewCallExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *CallExpressionContext {
	var p = new(CallExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *CallExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *CallExpressionContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *CallExpressionContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(NaslParserOpenParen, 0)
}

func (s *CallExpressionContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(NaslParserCloseParen, 0)
}

func (s *CallExpressionContext) ArgumentList() IArgumentListContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IArgumentListContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IArgumentListContext)
}

func (s *CallExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitCallExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type BitShiftExpressionContext struct {
	*SingleExpressionContext
}

func NewBitShiftExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BitShiftExpressionContext {
	var p = new(BitShiftExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *BitShiftExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BitShiftExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *BitShiftExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *BitShiftExpressionContext) LeftShiftArithmetic() antlr.TerminalNode {
	return s.GetToken(NaslParserLeftShiftArithmetic, 0)
}

func (s *BitShiftExpressionContext) RightShiftArithmetic() antlr.TerminalNode {
	return s.GetToken(NaslParserRightShiftArithmetic, 0)
}

func (s *BitShiftExpressionContext) LeftShiftLogical() antlr.TerminalNode {
	return s.GetToken(NaslParserLeftShiftLogical, 0)
}

func (s *BitShiftExpressionContext) RightShiftLogical() antlr.TerminalNode {
	return s.GetToken(NaslParserRightShiftLogical, 0)
}

func (s *BitShiftExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitBitShiftExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type ParenthesizedExpressionContext struct {
	*SingleExpressionContext
}

func NewParenthesizedExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ParenthesizedExpressionContext {
	var p = new(ParenthesizedExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *ParenthesizedExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ParenthesizedExpressionContext) OpenParen() antlr.TerminalNode {
	return s.GetToken(NaslParserOpenParen, 0)
}

func (s *ParenthesizedExpressionContext) ExpressionSequence() IExpressionSequenceContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExpressionSequenceContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExpressionSequenceContext)
}

func (s *ParenthesizedExpressionContext) CloseParen() antlr.TerminalNode {
	return s.GetToken(NaslParserCloseParen, 0)
}

func (s *ParenthesizedExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitParenthesizedExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type AdditiveExpressionContext struct {
	*SingleExpressionContext
}

func NewAdditiveExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *AdditiveExpressionContext {
	var p = new(AdditiveExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *AdditiveExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AdditiveExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *AdditiveExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *AdditiveExpressionContext) Plus() antlr.TerminalNode {
	return s.GetToken(NaslParserPlus, 0)
}

func (s *AdditiveExpressionContext) Minus() antlr.TerminalNode {
	return s.GetToken(NaslParserMinus, 0)
}

func (s *AdditiveExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitAdditiveExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type RelationalExpressionContext struct {
	*SingleExpressionContext
}

func NewRelationalExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *RelationalExpressionContext {
	var p = new(RelationalExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *RelationalExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RelationalExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *RelationalExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *RelationalExpressionContext) LessThan() antlr.TerminalNode {
	return s.GetToken(NaslParserLessThan, 0)
}

func (s *RelationalExpressionContext) MoreThan() antlr.TerminalNode {
	return s.GetToken(NaslParserMoreThan, 0)
}

func (s *RelationalExpressionContext) LessThanEquals() antlr.TerminalNode {
	return s.GetToken(NaslParserLessThanEquals, 0)
}

func (s *RelationalExpressionContext) GreaterThanEquals() antlr.TerminalNode {
	return s.GetToken(NaslParserGreaterThanEquals, 0)
}

func (s *RelationalExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitRelationalExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type PostIncrementExpressionContext struct {
	*SingleExpressionContext
}

func NewPostIncrementExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *PostIncrementExpressionContext {
	var p = new(PostIncrementExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *PostIncrementExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PostIncrementExpressionContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *PostIncrementExpressionContext) PlusPlus() antlr.TerminalNode {
	return s.GetToken(NaslParserPlusPlus, 0)
}

func (s *PostIncrementExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitPostIncrementExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type BitNotExpressionContext struct {
	*SingleExpressionContext
}

func NewBitNotExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BitNotExpressionContext {
	var p = new(BitNotExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *BitNotExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BitNotExpressionContext) BitNot() antlr.TerminalNode {
	return s.GetToken(NaslParserBitNot, 0)
}

func (s *BitNotExpressionContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *BitNotExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitBitNotExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type LiteralExpressionContext struct {
	*SingleExpressionContext
}

func NewLiteralExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *LiteralExpressionContext {
	var p = new(LiteralExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *LiteralExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LiteralExpressionContext) Literal() ILiteralContext {
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

func (s *LiteralExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitLiteralExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type ArrayLiteralExpressionContext struct {
	*SingleExpressionContext
}

func NewArrayLiteralExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ArrayLiteralExpressionContext {
	var p = new(ArrayLiteralExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *ArrayLiteralExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ArrayLiteralExpressionContext) ArrayLiteral() IArrayLiteralContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IArrayLiteralContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IArrayLiteralContext)
}

func (s *ArrayLiteralExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitArrayLiteralExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type MemberDotExpressionContext struct {
	*SingleExpressionContext
}

func NewMemberDotExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *MemberDotExpressionContext {
	var p = new(MemberDotExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *MemberDotExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *MemberDotExpressionContext) SingleExpression() ISingleExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISingleExpressionContext)
}

func (s *MemberDotExpressionContext) Dot() antlr.TerminalNode {
	return s.GetToken(NaslParserDot, 0)
}

func (s *MemberDotExpressionContext) Identifier() antlr.TerminalNode {
	return s.GetToken(NaslParserIdentifier, 0)
}

func (s *MemberDotExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitMemberDotExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type MemberIndexExpressionContext struct {
	*SingleExpressionContext
}

func NewMemberIndexExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *MemberIndexExpressionContext {
	var p = new(MemberIndexExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *MemberIndexExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *MemberIndexExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *MemberIndexExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *MemberIndexExpressionContext) OpenBracket() antlr.TerminalNode {
	return s.GetToken(NaslParserOpenBracket, 0)
}

func (s *MemberIndexExpressionContext) CloseBracket() antlr.TerminalNode {
	return s.GetToken(NaslParserCloseBracket, 0)
}

func (s *MemberIndexExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitMemberIndexExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type IdentifierExpressionContext struct {
	*SingleExpressionContext
}

func NewIdentifierExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *IdentifierExpressionContext {
	var p = new(IdentifierExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *IdentifierExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *IdentifierExpressionContext) Identifier() IIdentifierContext {
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

func (s *IdentifierExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitIdentifierExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type BitAndExpressionContext struct {
	*SingleExpressionContext
}

func NewBitAndExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BitAndExpressionContext {
	var p = new(BitAndExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *BitAndExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BitAndExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *BitAndExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *BitAndExpressionContext) BitAnd() antlr.TerminalNode {
	return s.GetToken(NaslParserBitAnd, 0)
}

func (s *BitAndExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitBitAndExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type BitOrExpressionContext struct {
	*SingleExpressionContext
}

func NewBitOrExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BitOrExpressionContext {
	var p = new(BitOrExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *BitOrExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BitOrExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *BitOrExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *BitOrExpressionContext) BitOr() antlr.TerminalNode {
	return s.GetToken(NaslParserBitOr, 0)
}

func (s *BitOrExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitBitOrExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

type XExpressionContext struct {
	*SingleExpressionContext
}

func NewXExpressionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *XExpressionContext {
	var p = new(XExpressionContext)

	p.SingleExpressionContext = NewEmptySingleExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*SingleExpressionContext))

	return p
}

func (s *XExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *XExpressionContext) AllSingleExpression() []ISingleExpressionContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleExpressionContext); ok {
			len++
		}
	}

	tst := make([]ISingleExpressionContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleExpressionContext); ok {
			tst[i] = t.(ISingleExpressionContext)
			i++
		}
	}

	return tst
}

func (s *XExpressionContext) SingleExpression(i int) ISingleExpressionContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleExpressionContext); ok {
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

	return t.(ISingleExpressionContext)
}

func (s *XExpressionContext) X() antlr.TerminalNode {
	return s.GetToken(NaslParserX, 0)
}

func (s *XExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitXExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) SingleExpression() (localctx ISingleExpressionContext) {
	return p.singleExpression(0)
}

func (p *NaslParser) singleExpression(_p int) (localctx ISingleExpressionContext) {
	this := p
	_ = this

	var _parentctx antlr.ParserRuleContext = p.GetParserRuleContext()
	_parentState := p.GetState()
	localctx = NewSingleExpressionContext(p, p.GetParserRuleContext(), _parentState)
	var _prevctx ISingleExpressionContext = localctx
	var _ antlr.ParserRuleContext = _prevctx // TODO: To prevent unused variable warning.
	_startState := 40
	p.EnterRecursionRule(localctx, 40, NaslParserRULE_singleExpression, _p)
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
	p.SetState(272)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 26, p.GetParserRuleContext()) {
	case 1:
		localctx = NewArrayLiteralExpressionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx

		{
			p.SetState(241)
			p.ArrayLiteral()
		}

	case 2:
		localctx = NewParenthesizedExpressionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(242)
			p.Match(NaslParserOpenParen)
		}
		{
			p.SetState(243)
			p.ExpressionSequence()
		}
		{
			p.SetState(244)
			p.Match(NaslParserCloseParen)
		}

	case 3:
		localctx = NewPreIncrementExpressionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(246)
			p.Match(NaslParserPlusPlus)
		}
		{
			p.SetState(247)
			p.singleExpression(22)
		}

	case 4:
		localctx = NewPreDecreaseExpressionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(248)
			p.Match(NaslParserMinusMinus)
		}
		{
			p.SetState(249)
			p.singleExpression(21)
		}

	case 5:
		localctx = NewUnaryPlusExpressionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(250)
			p.Match(NaslParserPlus)
		}
		{
			p.SetState(251)
			p.singleExpression(20)
		}

	case 6:
		localctx = NewUnaryMinusExpressionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(252)
			p.Match(NaslParserMinus)
		}
		{
			p.SetState(253)
			p.singleExpression(19)
		}

	case 7:
		localctx = NewBitNotExpressionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(254)
			p.Match(NaslParserBitNot)
		}
		{
			p.SetState(255)
			p.singleExpression(18)
		}

	case 8:
		localctx = NewNotExpressionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(256)
			p.Match(NaslParserNot)
		}
		{
			p.SetState(257)
			p.singleExpression(9)
		}

	case 9:
		localctx = NewAssignmentExpressionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(258)
			p.Identifier()
		}
		p.SetState(265)
		p.GetErrorHandler().Sync(p)

		switch p.GetTokenStream().LA(1) {
		case NaslParserOpenBracket:
			{
				p.SetState(259)
				p.Match(NaslParserOpenBracket)
			}
			{
				p.SetState(260)
				p.singleExpression(0)
			}
			{
				p.SetState(261)
				p.Match(NaslParserCloseBracket)
			}

		case NaslParserDot:
			{
				p.SetState(263)
				p.Match(NaslParserDot)
			}
			{
				p.SetState(264)
				p.Identifier()
			}

		case NaslParserAssign, NaslParserMultiplyAssign, NaslParserDivideAssign, NaslParserModulusAssign, NaslParserPlusAssign, NaslParserMinusAssign, NaslParserRightShiftLogicalAssign, NaslParserLeftShiftLogicalAssign, NaslParserRightShiftArithmeticAssign, NaslParserLeftShiftArithmeticAssign:

		default:
		}
		{
			p.SetState(267)
			p.AssignmentOperator()
		}
		{
			p.SetState(268)
			p.singleExpression(3)
		}

	case 10:
		localctx = NewIdentifierExpressionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(270)
			p.Identifier()
		}

	case 11:
		localctx = NewLiteralExpressionContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(271)
			p.Literal()
		}

	}
	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(327)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 29, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			p.SetState(325)
			p.GetErrorHandler().Sync(p)
			switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 28, p.GetParserRuleContext()) {
			case 1:
				localctx = NewMultiplicativeExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(274)

				if !(p.Precpred(p.GetParserRuleContext(), 15)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 15)", ""))
				}
				{
					p.SetState(275)
					_la = p.GetTokenStream().LA(1)

					if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&1006632960) != 0) {
						p.GetErrorHandler().RecoverInline(p)
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(276)
					p.singleExpression(16)
				}

			case 2:
				localctx = NewAdditiveExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(277)

				if !(p.Precpred(p.GetParserRuleContext(), 14)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 14)", ""))
				}
				{
					p.SetState(278)
					_la = p.GetTokenStream().LA(1)

					if !(_la == NaslParserPlus || _la == NaslParserMinus) {
						p.GetErrorHandler().RecoverInline(p)
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(279)
					p.singleExpression(15)
				}

			case 3:
				localctx = NewBitShiftExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(280)

				if !(p.Precpred(p.GetParserRuleContext(), 13)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 13)", ""))
				}
				{
					p.SetState(281)
					_la = p.GetTokenStream().LA(1)

					if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&31457280) != 0) {
						p.GetErrorHandler().RecoverInline(p)
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(282)
					p.singleExpression(14)
				}

			case 4:
				localctx = NewRelationalExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(283)

				if !(p.Precpred(p.GetParserRuleContext(), 12)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 12)", ""))
				}
				{
					p.SetState(284)
					_la = p.GetTokenStream().LA(1)

					if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&16106127360) != 0) {
						p.GetErrorHandler().RecoverInline(p)
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(285)
					p.singleExpression(13)
				}

			case 5:
				localctx = NewXExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(286)

				if !(p.Precpred(p.GetParserRuleContext(), 11)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 11)", ""))
				}
				{
					p.SetState(287)
					p.Match(NaslParserX)
				}
				{
					p.SetState(288)
					p.singleExpression(12)
				}

			case 6:
				localctx = NewEqualityExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(289)

				if !(p.Precpred(p.GetParserRuleContext(), 10)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 10)", ""))
				}
				{
					p.SetState(290)
					_la = p.GetTokenStream().LA(1)

					if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&1082331758592) != 0) {
						p.GetErrorHandler().RecoverInline(p)
					} else {
						p.GetErrorHandler().ReportMatch(p)
						p.Consume()
					}
				}
				{
					p.SetState(291)
					p.singleExpression(11)
				}

			case 7:
				localctx = NewBitAndExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(292)

				if !(p.Precpred(p.GetParserRuleContext(), 8)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 8)", ""))
				}
				{
					p.SetState(293)
					p.Match(NaslParserBitAnd)
				}
				{
					p.SetState(294)
					p.singleExpression(9)
				}

			case 8:
				localctx = NewBitOrExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(295)

				if !(p.Precpred(p.GetParserRuleContext(), 7)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 7)", ""))
				}
				{
					p.SetState(296)
					p.Match(NaslParserBitOr)
				}
				{
					p.SetState(297)
					p.singleExpression(8)
				}

			case 9:
				localctx = NewBitXOrExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(298)

				if !(p.Precpred(p.GetParserRuleContext(), 6)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 6)", ""))
				}
				{
					p.SetState(299)
					p.Match(NaslParserBitXOr)
				}
				{
					p.SetState(300)
					p.singleExpression(7)
				}

			case 10:
				localctx = NewLogicalAndExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(301)

				if !(p.Precpred(p.GetParserRuleContext(), 5)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 5)", ""))
				}
				{
					p.SetState(302)
					p.Match(NaslParserAnd)
				}
				{
					p.SetState(303)
					p.singleExpression(6)
				}

			case 11:
				localctx = NewLogicalOrExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(304)

				if !(p.Precpred(p.GetParserRuleContext(), 4)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 4)", ""))
				}
				{
					p.SetState(305)
					p.Match(NaslParserOr)
				}
				{
					p.SetState(306)
					p.singleExpression(5)
				}

			case 12:
				localctx = NewMemberDotExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(307)

				if !(p.Precpred(p.GetParserRuleContext(), 26)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 26)", ""))
				}
				{
					p.SetState(308)
					p.Match(NaslParserDot)
				}
				{
					p.SetState(309)
					p.Match(NaslParserIdentifier)
				}

			case 13:
				localctx = NewCallExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(310)

				if !(p.Precpred(p.GetParserRuleContext(), 25)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 25)", ""))
				}
				{
					p.SetState(311)
					p.Match(NaslParserOpenParen)
				}
				p.SetState(313)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)

				if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&140737522163732) != 0 || (int64((_la-66)) & ^0x3f) == 0 && ((int64(1)<<(_la-66))&255) != 0 {
					{
						p.SetState(312)
						p.ArgumentList()
					}

				}
				{
					p.SetState(315)
					p.Match(NaslParserCloseParen)
				}

			case 14:
				localctx = NewMemberIndexExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(316)

				if !(p.Precpred(p.GetParserRuleContext(), 23)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 23)", ""))
				}
				{
					p.SetState(317)
					p.Match(NaslParserOpenBracket)
				}
				{
					p.SetState(318)
					p.singleExpression(0)
				}
				{
					p.SetState(319)
					p.Match(NaslParserCloseBracket)
				}

			case 15:
				localctx = NewPostIncrementExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(321)

				if !(p.Precpred(p.GetParserRuleContext(), 17)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 17)", ""))
				}
				{
					p.SetState(322)
					p.Match(NaslParserPlusPlus)
				}

			case 16:
				localctx = NewPostDecreaseExpressionContext(p, NewSingleExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, NaslParserRULE_singleExpression)
				p.SetState(323)

				if !(p.Precpred(p.GetParserRuleContext(), 16)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 16)", ""))
				}
				{
					p.SetState(324)
					p.Match(NaslParserMinusMinus)
				}

			}

		}
		p.SetState(329)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 29, p.GetParserRuleContext())
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
	p.RuleIndex = NaslParserRULE_literal
	return p
}

func (*LiteralContext) IsLiteralContext() {}

func NewLiteralContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *LiteralContext {
	var p = new(LiteralContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_literal

	return p
}

func (s *LiteralContext) GetParser() antlr.Parser { return s.parser }

func (s *LiteralContext) BooleanLiteral() antlr.TerminalNode {
	return s.GetToken(NaslParserBooleanLiteral, 0)
}

func (s *LiteralContext) StringLiteral() antlr.TerminalNode {
	return s.GetToken(NaslParserStringLiteral, 0)
}

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

func (s *LiteralContext) IpLiteral() antlr.TerminalNode {
	return s.GetToken(NaslParserIpLiteral, 0)
}

func (s *LiteralContext) NULLLiteral() antlr.TerminalNode {
	return s.GetToken(NaslParserNULLLiteral, 0)
}

func (s *LiteralContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *LiteralContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *LiteralContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) Literal() (localctx ILiteralContext) {
	this := p
	_ = this

	localctx = NewLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 42, NaslParserRULE_literal)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(335)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case NaslParserBooleanLiteral:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(330)
			p.Match(NaslParserBooleanLiteral)
		}

	case NaslParserStringLiteral:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(331)
			p.Match(NaslParserStringLiteral)
		}

	case NaslParserIntegerLiteral, NaslParserFloatLiteral, NaslParserHexLiteral:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(332)
			p.NumericLiteral()
		}

	case NaslParserIpLiteral:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(333)
			p.Match(NaslParserIpLiteral)
		}

	case NaslParserNULLLiteral:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(334)
			p.Match(NaslParserNULLLiteral)
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
	p.RuleIndex = NaslParserRULE_numericLiteral
	return p
}

func (*NumericLiteralContext) IsNumericLiteralContext() {}

func NewNumericLiteralContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NumericLiteralContext {
	var p = new(NumericLiteralContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_numericLiteral

	return p
}

func (s *NumericLiteralContext) GetParser() antlr.Parser { return s.parser }

func (s *NumericLiteralContext) IntegerLiteral() antlr.TerminalNode {
	return s.GetToken(NaslParserIntegerLiteral, 0)
}

func (s *NumericLiteralContext) FloatLiteral() antlr.TerminalNode {
	return s.GetToken(NaslParserFloatLiteral, 0)
}

func (s *NumericLiteralContext) HexLiteral() antlr.TerminalNode {
	return s.GetToken(NaslParserHexLiteral, 0)
}

func (s *NumericLiteralContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NumericLiteralContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NumericLiteralContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitNumericLiteral(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) NumericLiteral() (localctx INumericLiteralContext) {
	this := p
	_ = this

	localctx = NewNumericLiteralContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 44, NaslParserRULE_numericLiteral)
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
		p.SetState(337)
		_la = p.GetTokenStream().LA(1)

		if !((int64((_la-68)) & ^0x3f) == 0 && ((int64(1)<<(_la-68))&11) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
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
	p.RuleIndex = NaslParserRULE_identifier
	return p
}

func (*IdentifierContext) IsIdentifierContext() {}

func NewIdentifierContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *IdentifierContext {
	var p = new(IdentifierContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_identifier

	return p
}

func (s *IdentifierContext) GetParser() antlr.Parser { return s.parser }

func (s *IdentifierContext) Identifier() antlr.TerminalNode {
	return s.GetToken(NaslParserIdentifier, 0)
}

func (s *IdentifierContext) X() antlr.TerminalNode {
	return s.GetToken(NaslParserX, 0)
}

func (s *IdentifierContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *IdentifierContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *IdentifierContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitIdentifier(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) Identifier() (localctx IIdentifierContext) {
	this := p
	_ = this

	localctx = NewIdentifierContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 46, NaslParserRULE_identifier)
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
		p.SetState(339)
		_la = p.GetTokenStream().LA(1)

		if !(_la == NaslParserX || _la == NaslParserIdentifier) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IAssignmentOperatorContext is an interface to support dynamic dispatch.
type IAssignmentOperatorContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsAssignmentOperatorContext differentiates from other interfaces.
	IsAssignmentOperatorContext()
}

type AssignmentOperatorContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAssignmentOperatorContext() *AssignmentOperatorContext {
	var p = new(AssignmentOperatorContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = NaslParserRULE_assignmentOperator
	return p
}

func (*AssignmentOperatorContext) IsAssignmentOperatorContext() {}

func NewAssignmentOperatorContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AssignmentOperatorContext {
	var p = new(AssignmentOperatorContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_assignmentOperator

	return p
}

func (s *AssignmentOperatorContext) GetParser() antlr.Parser { return s.parser }

func (s *AssignmentOperatorContext) MultiplyAssign() antlr.TerminalNode {
	return s.GetToken(NaslParserMultiplyAssign, 0)
}

func (s *AssignmentOperatorContext) DivideAssign() antlr.TerminalNode {
	return s.GetToken(NaslParserDivideAssign, 0)
}

func (s *AssignmentOperatorContext) ModulusAssign() antlr.TerminalNode {
	return s.GetToken(NaslParserModulusAssign, 0)
}

func (s *AssignmentOperatorContext) PlusAssign() antlr.TerminalNode {
	return s.GetToken(NaslParserPlusAssign, 0)
}

func (s *AssignmentOperatorContext) MinusAssign() antlr.TerminalNode {
	return s.GetToken(NaslParserMinusAssign, 0)
}

func (s *AssignmentOperatorContext) Assign() antlr.TerminalNode {
	return s.GetToken(NaslParserAssign, 0)
}

func (s *AssignmentOperatorContext) LeftShiftArithmeticAssign() antlr.TerminalNode {
	return s.GetToken(NaslParserLeftShiftArithmeticAssign, 0)
}

func (s *AssignmentOperatorContext) RightShiftArithmeticAssign() antlr.TerminalNode {
	return s.GetToken(NaslParserRightShiftArithmeticAssign, 0)
}

func (s *AssignmentOperatorContext) LeftShiftLogicalAssign() antlr.TerminalNode {
	return s.GetToken(NaslParserLeftShiftLogicalAssign, 0)
}

func (s *AssignmentOperatorContext) RightShiftLogicalAssign() antlr.TerminalNode {
	return s.GetToken(NaslParserRightShiftLogicalAssign, 0)
}

func (s *AssignmentOperatorContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AssignmentOperatorContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AssignmentOperatorContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitAssignmentOperator(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) AssignmentOperator() (localctx IAssignmentOperatorContext) {
	this := p
	_ = this

	localctx = NewAssignmentOperatorContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 48, NaslParserRULE_assignmentOperator)
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
		p.SetState(341)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&4358464092505088) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
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
	p.RuleIndex = NaslParserRULE_eos
	return p
}

func (*EosContext) IsEosContext() {}

func NewEosContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *EosContext {
	var p = new(EosContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = NaslParserRULE_eos

	return p
}

func (s *EosContext) GetParser() antlr.Parser { return s.parser }

func (s *EosContext) AllSemiColon() []antlr.TerminalNode {
	return s.GetTokens(NaslParserSemiColon)
}

func (s *EosContext) SemiColon(i int) antlr.TerminalNode {
	return s.GetToken(NaslParserSemiColon, i)
}

func (s *EosContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *EosContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *EosContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case NaslParserVisitor:
		return t.VisitEos(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *NaslParser) Eos() (localctx IEosContext) {
	this := p
	_ = this

	localctx = NewEosContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 50, NaslParserRULE_eos)
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
	p.SetState(344)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == NaslParserSemiColon {
		{
			p.SetState(343)
			p.Match(NaslParserSemiColon)
		}

		p.SetState(346)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

func (p *NaslParser) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 20:
		var t *SingleExpressionContext = nil
		if localctx != nil {
			t = localctx.(*SingleExpressionContext)
		}
		return p.SingleExpression_Sempred(t, predIndex)

	default:
		panic("No predicate with index: " + fmt.Sprint(ruleIndex))
	}
}

func (p *NaslParser) SingleExpression_Sempred(localctx antlr.RuleContext, predIndex int) bool {
	this := p
	_ = this

	switch predIndex {
	case 0:
		return p.Precpred(p.GetParserRuleContext(), 15)

	case 1:
		return p.Precpred(p.GetParserRuleContext(), 14)

	case 2:
		return p.Precpred(p.GetParserRuleContext(), 13)

	case 3:
		return p.Precpred(p.GetParserRuleContext(), 12)

	case 4:
		return p.Precpred(p.GetParserRuleContext(), 11)

	case 5:
		return p.Precpred(p.GetParserRuleContext(), 10)

	case 6:
		return p.Precpred(p.GetParserRuleContext(), 8)

	case 7:
		return p.Precpred(p.GetParserRuleContext(), 7)

	case 8:
		return p.Precpred(p.GetParserRuleContext(), 6)

	case 9:
		return p.Precpred(p.GetParserRuleContext(), 5)

	case 10:
		return p.Precpred(p.GetParserRuleContext(), 4)

	case 11:
		return p.Precpred(p.GetParserRuleContext(), 26)

	case 12:
		return p.Precpred(p.GetParserRuleContext(), 25)

	case 13:
		return p.Precpred(p.GetParserRuleContext(), 23)

	case 14:
		return p.Precpred(p.GetParserRuleContext(), 17)

	case 15:
		return p.Precpred(p.GetParserRuleContext(), 16)

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}
