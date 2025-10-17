package javaparser

import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

func GetJavaParserSerializedATN() []int32 {
	javaparserParserInit()
	return javaparserParserStaticData.serializedATN
}

func GetJavaLexerSerializedATN() []int32 {
	JavaLexerInit()
	return javalexerLexerStaticData.serializedATN
}

func (l *JavaLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *JavaParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
