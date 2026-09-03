package aspparser

import "github.com/yaklang/antlr/v4"

func GetASPParserSerializedATN() []int32 {
	aspparserParserInit()
	return ASPParserParserStaticData.serializedATN
}

func GetASPLexerSerializedATN() []int32 {
	ASPLexerInit()
	return ASPLexerLexerStaticData.serializedATN
}

func (l *ASPLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *ASPParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
