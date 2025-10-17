package yakast

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

type Position struct {
	LineNumber   int
	ColumnNumber int
}

type YakCompiler struct {
	yak.BaseYaklangParserVisitor

	language            CompilerLanguage
	lexerErrorListener  antlr.ErrorListener
	parserErrorListener antlr.ErrorListener
	errorStrategy       *ErrorStrategy
	// 语法错误的问题处理

	// 新增语句绑定起止位置
	currentStartPosition, currentEndPosition *memedit.Position

	// 格式化
	formatted         *bytes.Buffer
	indent            int
	currentLineLength int

	sourceCodeFilePathPointer, sourceCodePointer *string
	codes                                        []*yakvm.Code
	FreeValues                                   []int
	rootSymtbl                                   *yakvm.SymbolTable
	currentSymtbl                                *yakvm.SymbolTable
	programCounter                               int
	// 为了精简，栈只存 for 开始位置
	forDepthStack    *vmstack.Stack
	switchDepthStack *vmstack.Stack
	tryDepthStack    *vmstack.Stack
	// 编译过程语法错误的地方
	lexerErrors    antlr4util.SourceCodeErrors
	parserErrors   antlr4util.SourceCodeErrors
	compilerErrors antlr4util.SourceCodeErrors
	// is omap
	isOMap bool

	// tokenStream
	AntlrTokenStream          antlr.TokenStream
	lexer                     *yak.YaklangLexer
	parser                    *yak.YaklangParser
	strict                    bool
	indeterminateUndefinedVar [][2]any // [0] 为变量名 [1] 为错误信息
	extVars                   []string
	extVarsMap                map[string]struct{}
	contextInfo               *vmstack.Stack

	// import cycle check
	importCycleHash map[string]struct{}
}

func (y *YakCompiler) SetStrictMode(b bool) {
	y.strict = b
}

func (y *YakCompiler) SetExternalVariableNames(extVars []string) {
	y.extVars = extVars
	for _, extVar := range y.extVars {
		y.extVarsMap[extVar] = struct{}{}
	}
}

func (y *YakCompiler) SetRootSymTable(tbl *yakvm.SymbolTable) {
	y.rootSymtbl = tbl
}

func (y *YakCompiler) SetCurrentSymTable(tbl *yakvm.SymbolTable) {
	y.currentSymtbl = tbl
}

func (y *YakCompiler) NowInFor() bool {
	return y.forDepthStack.Len() > 0
}

func (y *YakCompiler) NowInSwitch() bool {
	return y.switchDepthStack.Len() > 0
}

// peekForStartIndex 检查当前最近的 for 循环的开始位置，一般为了设置 continue
func (y *YakCompiler) peekForStartIndex() int {
	result := y.peekForContext()
	if result == nil {
		return -1
	} else {
		return result.startCodeIndex
	}
}

func (y *YakCompiler) peekSwitchStartIndex() int {
	result := y.peekSwitchContext()
	if result == nil {
		return -1
	} else {
		return result.startCodeIndex
	}
}

func (y *YakCompiler) NewWithSymbolTable(rootSymbol *yakvm.SymbolTable) yakvm.CompilerWrapperInterface {
	return NewYakCompilerWithSymbolTable(rootSymbol)
}

func NewYakCompilerWithSymbolTable(rootSymbol *yakvm.SymbolTable, options ...CompilerOptionsFun) *YakCompiler {
	compiler := &YakCompiler{
		formatted:        new(bytes.Buffer),
		rootSymtbl:       rootSymbol,
		currentSymtbl:    rootSymbol,
		forDepthStack:    vmstack.New(),
		switchDepthStack: vmstack.New(),
		tryDepthStack:    vmstack.New(),
		language:         en,
		extVarsMap:       make(map[string]struct{}),
		contextInfo:      vmstack.New(),
		importCycleHash:  make(map[string]struct{}),
	}
	for _, o := range options {
		o(compiler)
	}
	compiler.lexerErrorListener = antlr4util.NewErrorListener(
		antlr4util.SimpleSyntaxErrorHandler(
			func(msg string, start, end *memedit.Position) {
				compiler.lexerErrors.Push(antlr4util.NewSourceCodeError(msg, start, end))
			},
		),
	)
	compiler.parserErrorListener = antlr4util.NewErrorListener(
		antlr4util.SimpleSyntaxErrorHandler(
			func(msg string, start, end *memedit.Position) {
				compiler.parserErrors.Push(antlr4util.NewSourceCodeError(msg, start, end))
			},
		),
	)
	return compiler
}

type CompilerOptionsFun func(*YakCompiler)

func WithLanguage(l CompilerLanguage) CompilerOptionsFun {
	return func(y *YakCompiler) {
		y.language = l
	}
}

func NewYakCompiler(options ...CompilerOptionsFun) *YakCompiler {
	root := yakvm.NewSymbolTable()
	return NewYakCompilerWithSymbolTable(root, options...)
}

func (y *YakCompiler) GetLexerErrorListener() antlr.ErrorListener {
	return y.lexerErrorListener
}

func (y *YakCompiler) GetParserErrorListener() antlr.ErrorListener {
	return y.parserErrorListener
}

func (y *YakCompiler) panicCompilerError(e constError, items ...interface{}) {
	err := y.newError(y.GetConstError(e), items...)
	y.compilerErrors.Push(err)
	panic(err)
}

func (y *YakCompiler) pushError(yakError *antlr4util.SourceCodeError) {
	y.compilerErrors.Push(yakError)
}

func (y *YakCompiler) newError(msg string, items ...interface{}) *antlr4util.SourceCodeError {
	if len(items) > 0 {
		msg = fmt.Sprintf(msg, items...)
	}
	return &antlr4util.SourceCodeError{
		StartPos: y.currentStartPosition,
		EndPos:   y.currentEndPosition,
		Message:  msg,
	}
}

func (y *YakCompiler) Init(lexer *yak.YaklangLexer, parser *yak.YaklangParser) {
	y.lexer = lexer
	y.parser = parser
}

func (y *YakCompiler) ShowOpcodes() {
	yakvm.ShowOpcodes(y.codes)
}

func (y *YakCompiler) ShowOpcodesWithSource(src string) {
	yakvm.ShowOpcodesWithSource(src, y.codes)
}

func (y *YakCompiler) CompileSourceCodeWithPath(code string, fPath *string) bool {
	y.sourceCodeFilePathPointer = fPath
	return y.Compiler(code)
}

func (y *YakCompiler) Compiler(code string) bool {
	lexer := yak.NewYaklangLexer(antlr.NewInputStream(code))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(y.lexerErrorListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := yak.NewYaklangParser(tokenStream)
	y.AntlrTokenStream = tokenStream
	parser.RemoveErrorListeners()
	parser.AddErrorListener(y.parserErrorListener)
	parser.SetErrorHandler(NewErrorStrategy())
	y.Init(lexer, parser)
	y.sourceCodePointer = &code
	y.codes = []*yakvm.Code{}
	raw := y.parser.Program()
	parseErrors := antlr4util.NewSourceCodeErrors(y.GetLexerErrors(), y.GetParserErrors())
	if len(*parseErrors) > 0 {
		return false
	}
	y.VisitProgram(raw.(*yak.ProgramContext))

	// 检查全局定义的变量，允许在非全局作用域内调用全局定义域内定义的变量
	errorMap := make(map[any]*antlr4util.SourceCodeError)
	for _, compilerError := range y.compilerErrors {
		errorMap[reflect.ValueOf(compilerError).Pointer()] = compilerError
	}
	for _, variable := range y.indeterminateUndefinedVar {
		name := variable[0].(string)
		err := variable[1].(*antlr4util.SourceCodeError)
		if _, ok := y.rootSymtbl.GetSymbolByVariableName(name); ok {
			errorMap[reflect.ValueOf(err).Pointer()] = nil
		}
	}
	for i := 0; i < len(y.compilerErrors); i++ {
		if v, ok := errorMap[reflect.ValueOf(y.compilerErrors[i]).Pointer()]; ok && v == nil {
			y.compilerErrors = append(y.compilerErrors[:i], y.compilerErrors[i+1:]...)
			i--
		}
	}

	lastToken := parser.BaseParser.GetCurrentToken()
	if lastToken.GetTokenType() != yak.YaklangParserEOF {
		startColumn := lastToken.GetColumn()
		if startColumn > 3 {
			startColumn = lastToken.GetColumn()
		} else {
			startColumn = 0
		}
		yErr := &antlr4util.SourceCodeError{
			StartPos: memedit.NewPosition(lastToken.GetLine(), startColumn),
			EndPos:   memedit.NewPosition(lastToken.GetLine(), startColumn+6),
			Message:  y.GetConstError(syntaxUnrecoverableError),
		}
		y.pushError(yErr)
	}
	compilerErrors := y.GetCompileErrors()
	if len(compilerErrors) > 0 {
		return false
	}
	return true
}

func (y *YakCompiler) VisitProgram(raw yak.IProgramContext, inline ...bool) interface{} {
	defer func() {
		prefix := y.GetRangeVerbose()
		if prefix != "" {
			prefix += ": "
		}
		if err := recover(); err != nil {
			msg := fmt.Sprintf("%vexit yak compiling by error: %v", prefix, err)
			log.Error(msg)
			// y.errors = append(y.errors, y.NewRangeSyntaxError(msg))
		}
		//if err := recover(); err != nil {
		//	msg := fmt.Sprintf("%vexit yak compiling by error: %v", prefix, err)
		//	log.Error(msg)
		//	y.errors = append(y.errors, y.NewRangeSyntaxError(msg))
		//}
	}()

	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.ProgramContext)
	if i == nil {
		return nil
	}
	y.writeAllWS(i.AllWs())

	// 遇到每一个 program 确定是要给人家新开定义域的！
	y.programCounter++

	noEmptyStmts := funk.Filter(i.StatementList().(*yak.StatementListContext).AllStatement(), func(i yak.IStatementContext) bool {
		return i.(*yak.StatementContext).Empty() == nil
	}).([]yak.IStatementContext)
	if len(noEmptyStmts) <= 0 {
		return nil
	}

	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.VisitStatementList(i.StatementList(), inline...)
	return nil
}

func (y *YakCompiler) VisitProgramWithoutSymbolTable(raw yak.IProgramContext, inline ...bool) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.ProgramContext)
	if i == nil {
		return nil
	}

	y.writeAllWS(i.AllWs())

	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.VisitStatementList(i.StatementList(), inline...)
	return nil
}

func (y *YakCompiler) GetOpcodes() []*yakvm.Code {
	return y.codes
}

func (y *YakCompiler) SetOpcodes(codes []*yakvm.Code) {
	y.codes = codes
}

func (y *YakCompiler) GetRootSymbolTable() *yakvm.SymbolTable {
	return y.rootSymtbl
}

func (y *YakCompiler) GetErrors() antlr4util.SourceCodeErrors {
	return *antlr4util.NewSourceCodeErrors(y.GetLexerErrors(), y.GetParserErrors(), y.GetCompileErrors())
}

func (y *YakCompiler) GetNormalErrors() (bool, error) {
	err := y.GetErrors()
	return len(err) > 0, err
}

func (y *YakCompiler) GetLexerErrors() antlr4util.SourceCodeErrors {
	return y.lexerErrors
}

func (y *YakCompiler) GetParserErrors() antlr4util.SourceCodeErrors {
	return y.parserErrors
}

func (y *YakCompiler) GetCompileErrors() antlr4util.SourceCodeErrors {
	return y.compilerErrors
}

func (y *YakCompiler) switchSource(newFilePath, newSourceCode *string) func() {
	oldFilePath, oldCode := y.sourceCodeFilePathPointer, y.sourceCodePointer
	y.sourceCodeFilePathPointer = newFilePath
	y.sourceCodePointer = newSourceCode
	return func() {
		y.sourceCodeFilePathPointer = oldFilePath
		y.sourceCodePointer = oldCode
	}
}
