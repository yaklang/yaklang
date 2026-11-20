package cppparser

import "github.com/yaklang/antlr/v4"

// GetCPPLexerSerializedATN returns the lexer ATN for the shared ANTLR cache.
func GetCPPLexerSerializedATN() []int32 {
	cpplexerLexerInit()
	return CPPLexerLexerStaticData.serializedATN
}

// GetCPPParserSerializedATN returns the parser ATN for the shared ANTLR cache.
func GetCPPParserSerializedATN() []int32 {
	cppparserParserInit()
	return CPPParserParserStaticData.serializedATN
}

func (l *CPPLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *CPPParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}