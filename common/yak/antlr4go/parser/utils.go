package gol

import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

func GetGoParserSerializedATN() []int32 {
	GoParserInit()
	return goparserParserStaticData.serializedATN
}

func GetGoLexerSerializedATN() []int32 {
	GoLexerInit()
	return golexerLexerStaticData.serializedATN
}

func (l *GoLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *GoParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
