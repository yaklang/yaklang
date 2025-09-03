package luaast

import (
	"fmt"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4Lua/infrastructure"
	lua "github.com/yaklang/yaklang/common/yak/antlr4Lua/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

type Position struct {
	LineNumber   int
	ColumnNumber int
}

type LuaTranslator struct {
	lua.BaseLuaParserVisitor

	sourceCode      string
	codes           []*yakvm.Code
	language        CompilerLanguage
	rootSymtbl      *yakvm.SymbolTable
	currentSymtbl   *yakvm.SymbolTable
	rootLabeltbl    *infrastructure.LabelTable
	currentLabeltbl *infrastructure.LabelTable
	programCounter  int

	currentStartPosition, currentEndPosition *memedit.Position

	AntlrTokenStream antlr.TokenStream
	lexer            *lua.LuaLexer
	parser           *lua.LuaParser

	// 编译过程语法错误的地方
	lexerErrors    antlr4util.SourceCodeErrors
	parserErrors   antlr4util.SourceCodeErrors
	compilerErrors antlr4util.SourceCodeErrors

	// 为了精简，栈只存 for 开始位置
	forDepthStack    *vmstack.Stack
	repeatDepthStack *vmstack.Stack
	whileDepthStack  *vmstack.Stack

	constTbl map[string]int
}

func NewLuaTranslator() *LuaTranslator {
	root := yakvm.NewSymbolTable()
	compiler := &LuaTranslator{
		rootSymtbl:       root,
		currentSymtbl:    root,
		constTbl:         make(map[string]int),
		forDepthStack:    vmstack.New(),
		repeatDepthStack: vmstack.New(),
		whileDepthStack:  vmstack.New(),
	}
	return compiler
}

func NewLuaTranslatorWithTable(rootSymbol *yakvm.SymbolTable, rootLabel *infrastructure.LabelTable) *LuaTranslator {
	return NewLuaTranslatorWithTableWithCode("", rootSymbol, rootLabel)
}

func NewLuaTranslatorWithTableWithCode(code string, rootSymbol *yakvm.SymbolTable, rootLabel *infrastructure.LabelTable) *LuaTranslator {
	compiler := &LuaTranslator{
		sourceCode:           code,
		language:             "en",
		rootSymtbl:           rootSymbol,
		currentSymtbl:        rootSymbol,
		rootLabeltbl:         rootLabel,
		currentLabeltbl:      rootLabel,
		constTbl:             make(map[string]int),
		forDepthStack:        vmstack.New(),
		repeatDepthStack:     vmstack.New(),
		whileDepthStack:      vmstack.New(),
		currentStartPosition: memedit.NewPosition(0, 0),
		currentEndPosition:   memedit.NewPosition(0, 0),
	}
	return compiler
}

func (l *LuaTranslator) SetRootSymTable(tbl *yakvm.SymbolTable) {
	l.rootSymtbl = tbl
}

func (l *LuaTranslator) GetRootSymTable() *yakvm.SymbolTable {
	return l.rootSymtbl
}

func (l *LuaTranslator) SetCurrentSymTable(tbl *yakvm.SymbolTable) {
	l.currentSymtbl = tbl
}

func (l *LuaTranslator) Translate(code string) bool {
	if l.lexer == nil || l.parser == nil {
		lexer := lua.NewLuaLexer(antlr.NewInputStream(code))
		lexer.RemoveErrorListeners()
		// lexer.AddErrorListener(l.lexerErrorListener)
		tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
		parser := lua.NewLuaParser(tokenStream)
		l.AntlrTokenStream = tokenStream
		parser.RemoveErrorListeners()
		// parser.AddErrorListener(l.parserErrorListener)
		// parser.SetErrorHandler(NewErrorStrategy())
		l.Init(lexer, parser)
	}
	raw := l.parser.Chunk()
	l.VisitChunk(raw)

	//parseErrors := NewYakMergeError(l.GetLexerErrors(), l.GetParserErrors())
	//if len(*parseErrors) > 0 {
	//	return false
	//}
	return true
}

func (l *LuaTranslator) Init(lexer *lua.LuaLexer, parser *lua.LuaParser) {
	l.lexer = lexer
	l.parser = parser
}

func (l *LuaTranslator) GetOpcodes() []*yakvm.Code {
	return l.codes
}

func (l *LuaTranslator) VisitChunk(raw lua.IChunkContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.ChunkContext)
	if i == nil {
		return nil
	}

	l.VisitBlock(i.Block())
	return nil
}

func (l *LuaTranslator) VisitBlock(raw lua.IBlockContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.BlockContext)
	if i == nil {
		return nil
	}
	recoverSymbolTableAndScope := l.SwitchSymbolTableInNewScope("block", uuid.New().String())
	defer recoverSymbolTableAndScope()
	for _, stat := range i.AllStat() {
		l.VisitStat(stat)
	}
	if s := i.Laststat(); s != nil {
		l.VisitLastStat(s)
	}
	return nil
}

func (l *LuaTranslator) GetErrors() antlr4util.SourceCodeErrors {
	return *antlr4util.NewSourceCodeErrors(l.GetLexerErrors(), l.GetParserErrors(), l.GetCompileErrors())
}

func (l *LuaTranslator) GetLexerErrors() antlr4util.SourceCodeErrors {
	return l.lexerErrors
}

func (l *LuaTranslator) GetParserErrors() antlr4util.SourceCodeErrors {
	return l.parserErrors
}

func (l *LuaTranslator) GetCompileErrors() antlr4util.SourceCodeErrors {
	return l.compilerErrors
}

func (l *LuaTranslator) panicCompilerError(e constError, items ...interface{}) {
	err := l.newError(l.GetConstError(e), items...)
	l.compilerErrors.Push(err)
	panic(err)
}

func (l *LuaTranslator) newError(msg string, items ...interface{}) *antlr4util.SourceCodeError {
	if len(items) > 0 {
		msg = fmt.Sprintf(msg, items...)
	}
	return antlr4util.NewSourceCodeError(msg, l.currentStartPosition, l.currentEndPosition)
}

type whileContext struct {
	startCodeIndex    int
	breakScopeCounter int
}

type switchContext struct {
	startCodeIndex          int
	switchBreakScopeCounter int
}

func (l *LuaTranslator) SwitchSymbolTableInNewScope(name ...string) func() {
	origin := l.currentSymtbl
	originLabel := l.currentLabeltbl
	l.currentSymtbl = origin.CreateSubSymbolTable(name...)
	l.currentLabeltbl = originLabel.CreateSubSymbolTable(name...)
	l.pushScope(l.rootSymtbl.MustRoot().GetTableIndex())
	l.addNearliestBreakScopeCounter(1)

	return func() {
		defer l.pushOperator(yakvm.OpScopeEnd)
		l.currentSymtbl = origin
		l.currentLabeltbl = originLabel
		l.addNearliestBreakScopeCounter(-1)
	}
}

func (l *LuaTranslator) addNearliestBreakScopeCounter(delta int) {
	//if l.peekForStartIndex() > 0 && l.peekSwitchStartIndex() > 0 {
	//	if l.GetNextCodeIndex()-l.peekForStartIndex() > l.GetNextCodeIndex()-l.peekSwitchStartIndex() {
	//		// switch 离得近
	//		l.peekSwitchContext().switchBreakScopeCounter += delta
	//	} else {
	//		// for 离得近
	//		l.peekForContext().breakScopeCounter += delta
	//	}
	//	return
	//}

	if l.peekWhileStartIndex() > 0 {
		l.peekWhileContext().breakScopeCounter += delta
		return
	}

	//if l.peekSwitchStartIndex() > 0 {
	//	l.peekSwitchContext().switchBreakScopeCounter += delta
	//	return
	//}
}

func (l *LuaTranslator) getNearliestBreakScopeCounter() int {
	if l.peekWhileStartIndex() > 0 && l.peekRepeatStartIndex() > 0 {
		if l.GetNextCodeIndex()-l.peekWhileStartIndex() > l.GetNextCodeIndex()-l.peekRepeatStartIndex() {
			// repeat 离得近
			return l.peekRepeatContext().switchBreakScopeCounter
		} else {
			// for 离得近
			return l.peekWhileContext().breakScopeCounter
		}
	}

	if l.peekWhileStartIndex() > 0 {
		cnt := l.peekWhileContext().breakScopeCounter
		return cnt
	}

	if l.peekRepeatStartIndex() > 0 {
		return l.peekRepeatContext().switchBreakScopeCounter
	}
	return 0
}

func (l *LuaTranslator) SwitchSymbolTable(name ...string) func() {
	origin := l.currentSymtbl
	l.currentSymtbl = origin.CreateSubSymbolTable(name...)
	return func() {
		l.currentSymtbl = origin
	}
}

func (l *LuaTranslator) SwitchCodes() func() {
	origin := l.codes
	l.codes = []*yakvm.Code{}
	return func() {
		l.codes = origin
	}
}

// peekForStartIndex 检查当前最近的 while 循环的开始位置
func (l *LuaTranslator) peekWhileStartIndex() int {
	result := l.peekWhileContext()
	if result == nil {
		return 0
	} else {
		return result.startCodeIndex
	}
}

func (l *LuaTranslator) peekRepeatStartIndex() int {
	result := l.peekRepeatContext()
	if result == nil {
		return 0
	} else {
		return result.startCodeIndex
	}
}

func (l *LuaTranslator) peekWhileContext() *whileContext {
	raw, ok := l.whileDepthStack.Peek().(*whileContext)
	if ok {
		return raw
	} else {
		return nil
	}
}

func (l *LuaTranslator) peekRepeatContext() *switchContext {
	raw, ok := l.repeatDepthStack.Peek().(*switchContext)
	if ok {
		return raw
	} else {
		return nil
	}
}

func (l *LuaTranslator) NowInWhile() bool {
	return l.whileDepthStack.Len() > 0
}

func (l *LuaTranslator) NowInRepeat() bool {
	return l.repeatDepthStack.Len() > 0
}

func (l *LuaTranslator) NowInFor() bool {
	return l.forDepthStack.Len() > 0
}
