package scalaparser

import "github.com/yaklang/antlr/v4"

func GetScalaLexerSerializedATN() []int32 {
	ScalaLexerInit()
	return ScalaLexerLexerStaticData.serializedATN
}

func GetScalaParserSerializedATN() []int32 {
	ScalaParserInit()
	return ScalaParserParserStaticData.serializedATN
}

func (l *ScalaLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *ScalaParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
