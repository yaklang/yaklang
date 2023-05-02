package antlr4Lua

import (
	"context"
	"fmt"
	"os"
	"github.com/yaklang/yaklang/common/yak/antlr4Lua/luaast"
)

type LuaSnippetExecutor struct {
	sourceCode string
	engine     *Engine
	translator *luaast.LuaTranslator
}

func NewLuaSnippetExecutor(code string) *LuaSnippetExecutor {
	return &LuaSnippetExecutor{sourceCode: code, engine: New(), translator: &luaast.LuaTranslator{}}
}

func (l *LuaSnippetExecutor) Run() {
	err := l.engine.Eval(context.Background(), l.sourceCode)
	if err != nil {
		panic(fmt.Sprintf("\n==============\n%s\n==============\n", err.Error()))
	}
}

func (l *LuaSnippetExecutor) Debug() {
	l.engine.debug = true
	err := l.engine.Eval(context.Background(), l.sourceCode)
	if err != nil {
		panic(fmt.Sprintf("\n==============\n%s\n==============\n", err.Error()))
	}
}

// SmartRun SmartRun() will choose Run() or Debug() depending on the environment setting `LUA_DEBUG`
func (l *LuaSnippetExecutor) SmartRun() {
	if os.Getenv("LUA_DEBUG") != "" {
		l.Debug()
	} else {
		l.Run()
	}
}
