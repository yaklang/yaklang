package parser

import "github.com/yaklang/antlr/v4"

func GetLexerSerializedATN() []int32 {
	YaklangLexerInit()
	return YaklangLexerLexerStaticData.serializedATN
}

func GetParserSerializedATN() []int32 {
	YaklangParserInit()
	return YaklangParserParserStaticData.serializedATN
}

func (l *YaklangLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *YaklangParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
