package js2ssa

import (
	"runtime/debug"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"

	"github.com/yaklang/yaklang/common/utils"
	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
)

type Parser struct {
}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(src string, must bool, callBack func(*ssa.FunctionBuilder)) (*ssa.Program, error) {
	return parseSSA(src, must, nil, callBack)
}

func (p *Parser) Feed(src string, must bool, prog *ssa.Program) {
	parseSSA(src, must, prog, nil)
}

type astbuilder struct {
	*ssa.FunctionBuilder
	lmap map[string]struct{}
	cmap map[string]struct{}
}

func NewAstBuilder(functionBuilder *ssa.FunctionBuilder) *astbuilder {
	return &astbuilder{
		FunctionBuilder: functionBuilder,
		lmap:            make(map[string]struct{}),
		cmap:            make(map[string]struct{}),
	}
}

func parseSSA(src string, force bool, prog *ssa.Program, callback func(*ssa.FunctionBuilder)) (ret *ssa.Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			debug.PrintStack()
			ret = nil
			err = utils.Errorf("parse error with panic : %v", r)
		}
	}()

	if prog == nil {
		prog = ssa.NewProgram()
	}
	if err := frontend(src, force, func(ast *JS.ProgramContext) {
		funcBuilder := prog.GetAndCreateMainFunctionBuilder()
		if funcBuilder == nil {
			return
		}
		if callback != nil {
			callback(funcBuilder)
		}
		astbuilder := NewAstBuilder(funcBuilder)
		astbuilder.build(ast)
		astbuilder.Finish()
	}); err != nil {
		//TODO: merge code with yak2ssa/builder.go
		return prog, nil
	}

	ssa4analyze.RunAnalyzer(prog)
	return prog, nil
}

// error listener for lexer and parser
type ErrorListener struct {
	err []string
	*antlr.DefaultErrorListener
}

func (el *ErrorListener) SyntaxError(recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
	el.err = append(el.err, msg)
}

func NewErrorListener() *ErrorListener {
	return &ErrorListener{
		err:                  []string{},
		DefaultErrorListener: antlr.NewDefaultErrorListener(),
	}
}

func frontend(src string, must bool, handler func(*JS.ProgramContext)) error {
	errListener := NewErrorListener()
	// start := time.Now()
	lexer := JS.NewJavaScriptLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := JS.NewJavaScriptParser(tokenStream)
	// log.Info(time.Since(start))
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.Program().(*JS.ProgramContext)
	// log.Info("ast time ", time.Since(start))
	if must || len(errListener.err) == 0 {
		handler(ast)
		return nil
	} else {
		return utils.Errorf("parse AST FrontEnd error : %v", errListener.err)
	}
}

func (b *astbuilder) AddToCmap(key string) {
	b.cmap[key] = struct{}{}
}

func (b *astbuilder) GetFromCmap(key string) bool {
	if _, ok := b.cmap[key]; ok {
		return true
	} else {
		return false
	}
}

func (b *astbuilder) AddToLmap(key string) {
	b.lmap[key] = struct{}{}
}

func (b *astbuilder) GetFromLmap(key string) bool {
	if _, ok := b.lmap[key]; ok {
		return true
	} else {
		return false
	}
}
