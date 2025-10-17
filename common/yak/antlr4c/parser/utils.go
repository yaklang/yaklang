package c

import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

func GetParserSerializedATN() []int32 {
	staticData := &cparserParserStaticData
	staticData.once.Do(cparserParserInit)
	return staticData.serializedATN
}

func GetLexerSerializedATN() []int32 {
	staticData := &clexerLexerStaticData
	staticData.once.Do(clexerLexerInit)
	return staticData.serializedATN
}

func (l *CLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *CParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
