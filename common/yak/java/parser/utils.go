package javaparser

import "github.com/yaklang/antlr/v4"

// GetJavaParserSerializedATN returns the serialized ATN for the Java parser.
//
// Like the Python equivalent it must go through JavaParserInit (sync.Once):
// javaparserParserInit reassigns the package-level atn / decisionToDFA /
// PredictionContextCache, so calling it directly rebuilds shared static state
// concurrently with in-flight parsers and defeats the atn.stateMu guard that
// serializes shared-cache mutation in the ANTLR runtime.
func GetJavaParserSerializedATN() []int32 {
	JavaParserInit()
	return JavaParserParserStaticData.serializedATN
}

func GetJavaLexerSerializedATN() []int32 {
	JavaLexerInit()
	return JavaLexerLexerStaticData.serializedATN
}

func (l *JavaLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

func (p *JavaParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
