package pythonparser

import "github.com/yaklang/antlr/v4"

// GetPythonParserSerializedATN returns the serialized ATN for the Python parser.
// This is used for caching parser state to improve performance.
// Similar to GetJavaParserSerializedATN in java/parser/utils.go
//
// It must go through PythonParserInit (sync.Once): pythonparserParserInit
// reassigns the package-level atn / decisionToDFA / PredictionContextCache, so
// calling it directly rebuilds the shared static state on every use. That both
// races with in-flight parsers and splits the (ATN, DFA, context cache) triple
// across parsers, which breaks the atn.stateMu guard the ANTLR runtime relies
// on to serialize shared-cache mutation and can abort the process with
// "fatal error: concurrent map read and map write".
func GetPythonParserSerializedATN() []int32 {
	PythonParserInit()
	return PythonParserParserStaticData.serializedATN
}

// GetPythonLexerSerializedATN returns the serialized ATN for the Python lexer.
// This is used for caching lexer state to improve performance.
// Similar to GetJavaLexerSerializedATN in java/parser/utils.go
func GetPythonLexerSerializedATN() []int32 {
	PythonLexerInit()
	return PythonLexerLexerStaticData.serializedATN
}

// SetInterpreter methods are used to override the default interpreter behavior
// for better performance with cached ATN data.

// SetInterpreter sets the interpreter for the PythonLexer with the provided ATN and DFA.
// This allows using cached ATN data for better performance.
func (l *PythonLexer) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	l.Interpreter = antlr.NewLexerATNSimulator(l, atn, decisionToDFA, predictionContextCache)
}

// SetInterpreter sets the interpreter for the PythonParser with the provided ATN and DFA.
// This allows using cached ATN data for better performance.
func (p *PythonParser) SetInterpreter(atn *antlr.ATN, decisionToDFA []*antlr.DFA, predictionContextCache *antlr.PredictionContextCache) {
	// do nothing, just to override the method
	p.Interpreter = antlr.NewParserATNSimulator(p, atn, decisionToDFA, predictionContextCache)
}
