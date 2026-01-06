package python2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
)

// Frontend parses Python source code and returns the root AST node.
// This is similar to the Java Frontend function in java2ssa/builder.go
func Frontend(src string) (pythonparser.IRootContext, error) {
	errListener := antlr4util.NewErrorListener()

	input := antlr.NewInputStream(src)
	lexer := pythonparser.NewPythonLexer(input)
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	stream := antlr.NewCommonTokenStream(lexer, 0)
	parser := pythonparser.NewPythonParser(stream)

	// Remove default error listeners and add a custom one if needed
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	// Parse the root rule
	tree := parser.Root()

	return tree, errListener.Error()
}
