package kotlinparser

import "github.com/yaklang/antlr/v4"

func GetKotlinLexerSerializedATN() []int32 {
	KotlinLexerInit()
	return KotlinLexerLexerStaticData.serializedATN
}

func GetKotlinParserSerializedATN() []int32 {
	KotlinParserInit()
	return KotlinParserParserStaticData.serializedATN
}

func (l *KotlinLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *KotlinParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
