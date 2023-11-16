package antlr4yak

import (
	"context"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

// eval
func (e *Engine) YakBuiltinEval(code string) {
	vm := e.vm
	topFrame := vm.VMStack.Peek().(*yakvm.Frame)
	ctx := topFrame.GetContext()
	if utils.IsNil(ctx) {
		ctx = context.Background()
	}

	codes, err := e.Compile(code)
	if err != nil {
		panic(err)
	}
	if err = e.vm.ExecYakCode(ctx, code, codes, yakvm.Inline); err != nil {
		panic(err)
	}
}

// yakfmt
func (e *Engine) YakBuiltinfmt(code string) string {
	newCode, err := New().FormattedAndSyntaxChecking(code)
	if err != nil {
		log.Errorf("format and syntax checking met error: %s", err)
		return code
	}
	return newCode
}

// yakfmtWithError
func (e *Engine) YakBuiltinfmtWithError(code string) (string, error) {
	return New().FormattedAndSyntaxChecking(code)
}

// getScopeInspects
func (e *Engine) YakBuiltinGetScopeInspects() ([]*ScopeValue, error) {
	return e.GetScopeInspects()
}

func (e *Engine) waitAllAsyncCallFinish() {
	e.vm.AsyncWait()
}

func InjectContextBuiltinFunction(engine *Engine) {
	engine.ImportLibs(map[string]interface{}{
		"eval":                   engine.YakBuiltinEval,
		"yakfmt":                 engine.YakBuiltinfmt,
		"yakfmtWithError":        engine.YakBuiltinfmtWithError,
		"getScopeInspects":       engine.YakBuiltinGetScopeInspects,
		"waitAllAsyncCallFinish": engine.waitAllAsyncCallFinish,
	})
}
