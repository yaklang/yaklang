package antlr4yak

import (
	"context"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

// eval 执行任意 Yak 代码
// 这个函数是存在副作用的，即能够获取和改变当前引擎中的上下文
// Example:
// ```
// a = 1
// eval("a++")
// assert a == 2
// ```
func (e *Engine) YakBuiltinEval(code string) {
	vm := e.vm
	topFrame := vm.VMStack.Peek().(*yakvm.Frame)
	ctx := topFrame.GetContext()
	if utils.IsNil(ctx) {
		ctx = context.Background()
	}

	codes, err := e.CompileWithCurrentScope(code)
	if err != nil {
		panic(err)
	}
	if err = e.vm.ExecYakCode(ctx, code, codes, yakvm.Inline); err != nil {
		panic(err)
	}
}

// yakfmt 格式化任意 Yak 代码，返回格式化后的代码
// Example:
// ```
// yakfmt("for { println(`hello yak`) }")
// ```
func (e *Engine) YakBuiltinfmt(code string) string {
	newCode, err := New().FormattedAndSyntaxChecking(code)
	if err != nil {
		log.Errorf("format and syntax checking met error: %s", err)
		return code
	}
	return newCode
}

// yakfmtWithError 格式化任意 Yak 代码，返回格式化后的代码和错误
// Example:
// ```
// yakfmtWithError("for { println(`hello yak`) }")
// ```
func (e *Engine) YakBuiltinfmtWithError(code string) (string, error) {
	return New().FormattedAndSyntaxChecking(code)
}

// getScopeInspects 获取当前作用域中的所有变量，返回 ScopeValue 结构体引用切片
// Example:
// ```
// a, b = 1, "yak"
// values, err = getScopeInspects()
// for v in values {
// println(v.Value)
// }
// ```
func (e *Engine) YakBuiltinGetScopeInspects() ([]*ScopeValue, error) {
	return e.GetScopeInspects()
}

// getFromScope 获取当前作用域中的变量，返回变量值
// Example:
// ```
// a, b = 1, "yak"
// { assert getFromScope("a") == 1 }
// { assert getFromScope("b") == "yak" }
// ```
func (e *Engine) YakBuiltinGetFromScope(v string, vals ...any) any {
	val, ok := e.GetVar(v)
	if ok {
		return val
	}
	if len(vals) >= 1 {
		return vals[0]
	}
	return nil
}

// waitAllAsyncCallFinish 等待直到所有异步调用完成
// Example:
// ```
// a = 0
// for i in 5 {
// go func(i) {
// time.sleep(i)
// a++
// }(i)
// }
// waitAllAsyncCallFinish()
// assert a == 5
// ```
func (e *Engine) waitAllAsyncCallFinish() {
	e.vm.AsyncWait()
}

func InjectContextBuiltinFunction(engine *Engine) {
	engine.ImportLibs(map[string]interface{}{
		"eval":                   engine.YakBuiltinEval,
		"yakfmt":                 engine.YakBuiltinfmt,
		"yakfmtWithError":        engine.YakBuiltinfmtWithError,
		"getScopeInspects":       engine.YakBuiltinGetScopeInspects,
		"getFromScope":           engine.YakBuiltinGetFromScope,
		"waitAllAsyncCallFinish": engine.waitAllAsyncCallFinish,
	})
}
