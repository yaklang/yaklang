package csharpparser

import "github.com/yaklang/antlr/v4"

func GetCSharpParserSerializedATN() []int32 {
	csharpparserParserInit()
	return CSharpParserParserStaticData.serializedATN
}

func GetCSharpLexerSerializedATN() []int32 {
	CSharpLexerInit()
	return CSharpLexerLexerStaticData.serializedATN
}

func (l *CSharpLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *CSharpParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
