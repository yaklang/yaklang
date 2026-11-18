package rustparser

import "github.com/yaklang/antlr/v4"

func GetRustLexerSerializedATN() []int32 {
	RustLexerInit()
	return RustLexerLexerStaticData.serializedATN
}

func GetRustParserSerializedATN() []int32 {
	RustParserInit()
	return RustParserParserStaticData.serializedATN
}

func (l *RustLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *RustParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
