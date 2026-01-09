package pythonparser

import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// GetPythonParserSerializedATN returns the serialized ATN for the Python parser.
// This is used for caching parser state to improve performance.
// Similar to GetJavaParserSerializedATN in java/parser/utils.go
func GetPythonParserSerializedATN() []int32 {
	pythonparserParserInit()
	return pythonparserParserStaticData.serializedATN
}

// GetPythonLexerSerializedATN returns the serialized ATN for the Python lexer.
// This is used for caching lexer state to improve performance.
// Similar to GetJavaLexerSerializedATN in java/parser/utils.go
func GetPythonLexerSerializedATN() []int32 {
	PythonLexerInit()
	return pythonlexerLexerStaticData.serializedATN
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

