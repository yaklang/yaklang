package stream_parser

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

// evalOutProgram is only for the fresh, default engine constructed by ExecOut.
// It is not a general evaluator for an engine with existing variables, strict
// compilation, debugger settings or a source-file path. The artifact is shared
// with operators, but each call owns its decoded code, symbol table and frame.
func evalOutProgram(engine *antlr4yak.Engine, code string) (result any, err error) {
	program, err := loadOperatorProgram(code)
	if err != nil {
		return engine.ExecuteAsExpression(code, nil)
	}
	decoder := yakvm.NewCodesMarshaller()
	decoder.SetSourceCode(code)
	symbols, codes, err := decoder.Unmarshal(program)
	if err != nil {
		// Decoding has not executed callbacks or changed the engine. Preserve
		// language behavior if an artifact cannot use the optimized path.
		return engine.ExecuteAsExpression(code, nil)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	bindRuleProgramSource(codes, &code)
	engine.GetVM().SetSymboltable(symbols)
	var value *yakvm.Value
	err = engine.GetVM().Exec(context.Background(), func(frame *yakvm.Frame) {
		frame.SetVerbose("__yak_main__")
		frame.SetOriginCode(code)
		frame.Exec(codes)
		// Match SafeEvalInlineWithResult, including statements with no return,
		// explicit return, and an empty program. A named variable is not a
		// substitute for the VM's actual last-stack result.
		value = frame.GetLastStackValue()
	}, yakvm.Inline)
	if err != nil || value == nil {
		return nil, err
	}
	return value.Value, nil
}
