package cppparser

import "github.com/yaklang/antlr/v4"

// GetCPP14LexerSerializedATN returns the lexer ATN for the shared ANTLR cache.
func GetCPP14LexerSerializedATN() []int32 {
	cpp14lexerLexerInit()
	return CPP14LexerLexerStaticData.serializedATN
}

// GetCPP14ParserSerializedATN returns the parser ATN for the shared ANTLR cache.
func GetCPP14ParserSerializedATN() []int32 {
	cpp14parserParserInit()
	return CPP14ParserParserStaticData.serializedATN
}

func (l *CPP14Lexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *CPP14Parser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}