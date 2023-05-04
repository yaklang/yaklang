package visitors

import (
	"fmt"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	nasl "github.com/yaklang/yaklang/common/yak/antlr4nasl/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

type Compiler struct {
	position                              [4]int
	errors                                []error
	symbolTable                           *yakvm.SymbolTable
	symbolTableSize                       int
	codes                                 []*yakvm.Code
	lexer                                 *nasl.NaslLexer
	parser                                *nasl.NaslParser
	TmpData                               *vmstack.Stack
	needPop                               bool
	extVarNames                           map[string]struct{}
	visitHookHandles                      []func(compiler *Compiler, ctx antlr.ParserRuleContext)
	visitHook                             func(compiler *Compiler, ctx antlr.ParserRuleContext)
	checkId                               bool
	naslLib                               map[string]interface{}
	sourceCodePointer, sourceCodeFilePath *string
}

func (c *Compiler) SetSourceCode(s string) func() {
	back := c.sourceCodePointer
	c.sourceCodePointer = &s
	return func() {
		c.sourceCodePointer = back
	}
}
func (c *Compiler) SetSourceCodeFilePath(s string) func() {
	back := c.sourceCodeFilePath
	c.sourceCodeFilePath = &s
	return func() {
		c.sourceCodeFilePath = back
	}
}
func (c *Compiler) SetNaslLib(naslLib map[string]interface{}) {
	c.naslLib = naslLib
}
func (c *Compiler) SetStartPosition(n1, n2 int) {
	c.position[0] = n1
	c.position[1] = n2
}
func (c *Compiler) SetStopPosition(n1, n2 int) {
	c.position[2] = n1
	c.position[3] = n2
}
func NewCompilerWithSymbolTable(table *yakvm.SymbolTable) *Compiler {
	compiler := &Compiler{
		symbolTable:     table,
		symbolTableSize: 0,
		TmpData:         vmstack.New(),
		extVarNames:     make(map[string]struct{}),
	}
	compiler.visitHook = func(c *Compiler, ctx antlr.ParserRuleContext) {
		for _, i2 := range c.visitHookHandles {
			i2(c, ctx)
		}
	}
	return compiler
}
func NewCompiler() *Compiler {
	return NewCompilerWithSymbolTable(yakvm.NewSymbolTable())
}
func (c *Compiler) SetCheckIdentifier(b bool) {
	c.checkId = b
}
func (c *Compiler) GetExternalVariablesNamesMap() map[string]struct{} {
	return c.extVarNames
}

func (c *Compiler) AddVisitHook(h func(compiler *Compiler, ctx antlr.ParserRuleContext)) {
	c.visitHookHandles = append(c.visitHookHandles, h)
}

func (c *Compiler) SetExternalVariableNames(names []string) {
	for _, name := range names {
		c.extVarNames[name] = struct{}{}
	}
}
func (c *Compiler) NeedPop(b bool) {
	c.needPop = b
}
func (c *Compiler) Compile(code string) bool {
	defer func() {
		if e := recover(); e != nil {
			c.AddError(utils.Error(e))
		}
	}()
	//if c.lexer == nil || c.parser == nil {
	lexer := nasl.NewNaslLexer(antlr.NewInputStream(code))
	//lexer.RemoveErrorListeners()
	lexer.AddErrorListener(NewErrorListener(func(msg string) {
		c.errors = append(c.errors, utils.Error(msg))
	}))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := nasl.NewNaslParser(tokenStream)
	//parser.RemoveErrorListeners()
	parser.AddErrorListener(NewErrorListener(func(msg string) {
		c.errors = append(c.errors, utils.Error(msg))
	}))
	//listener := NewParseTreeListener()
	//listener.SetEnter(func(ctx antlr.ParserRuleContext) {
	//	if start := ctx.GetStart(); start != nil {
	//		c.position[0] = start.GetLine()
	//		c.position[1] = start.GetColumn()
	//	}
	//})
	//listener.SetExit(func(ctx antlr.ParserRuleContext) {
	//	if end := ctx.GetStop(); end != nil {
	//		c.position[2] = end.GetLine()
	//		c.position[3] = end.GetColumn()
	//	}
	//})
	//parser.AddParseListener(listener)
	c.lexer = lexer
	c.parser = parser
	//}
	c.codes = []*yakvm.Code{}
	if c.visitHook == nil {
		c.visitHook = func(compiler *Compiler, ctx antlr.ParserRuleContext) {

		}
	}
	c.VisitProgram(c.parser.Program())
	return len(c.errors) == 0
}
func (c *Compiler) GetCodes() []*yakvm.Code {
	return c.codes
}
func (c *Compiler) GetSymbolTable() *yakvm.SymbolTable {
	return c.symbolTable
}
func (c *Compiler) AddError(e error) {
	c.errors = append(c.errors, e)
}
func (c *Compiler) GetErrors() []error {
	return c.errors
}
func (c *Compiler) GetMergeError() error {
	errStr := ""
	for _, err := range c.errors {
		errStr += (err.Error() + "\n")
	}
	return utils.Error(errStr)
}

func (c *Compiler) GetCodePostion() int {
	return len(c.codes)
}
func (c *Compiler) VisitProgram(i nasl.IProgramContext) {
	if i == nil {
		return
	}
	program, ok := i.(*nasl.ProgramContext)
	if !ok {
		return
	}
	c.VisitStatementList(program.StatementList())
}
func (c *Compiler) GetSymbolId(name string) int {
	if sym, ok := c.symbolTable.GetSymbolByVariableName(name); ok {
		return sym
	} else {
		sym, err := c.symbolTable.NewSymbolWithReturn(name)
		if err != nil {
			panic(fmt.Sprintf("new symbol error: %v", err))
		}
		return sym
	}
}
