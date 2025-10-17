package parser

import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

func GetLexerSerializedATN() []int32 {
	YaklangLexerInit()
	return yaklanglexerLexerStaticData.serializedATN
}

func GetParserSerializedATN() []int32 {
	YaklangParserInit()
	return yaklangparserParserStaticData.serializedATN
}

func (l *YaklangLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *YaklangParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
