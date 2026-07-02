package phpparser

import "github.com/yaklang/antlr/v4"

func GetPHPParserSerializedATN() []int32 {
	PHPParserInit()
	return PHPParserParserStaticData.serializedATN
}

func GetPHPLexerSerializedATN() []int32 {
	PHPLexerInit()
	return PHPLexerLexerStaticData.serializedATN
}

func (l *PHPLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *PHPParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
