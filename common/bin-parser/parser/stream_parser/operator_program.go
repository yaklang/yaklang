package stream_parser

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

const (
	operatorProgramCacheCapacity = 1024
	operatorProgramSourceLimit   = 64 << 10
	operatorProgramArtifactLimit = 128 << 10
)

var operatorProgramCache = utils.NewLRUCache[[]byte](operatorProgramCacheCapacity)

// This shared cache stores serialized programs, never callbacks or runtime values. Both
// operators and out expressions use a fresh default compiler; execution mode is
// chosen only after loading. They share this budget, not a second expression LRU.
// Deserialization gives each invocation its own symbol table and code values,
// including mutable literals and dynamic-eval symbols. Custom rule source with
// file dependencies must compile on each call.
// A separate, bounded exclusive worker pool may retain decoded programs only
// for the closed immutable bridge grammar in operator_worker.go.
func loadOperatorProgram(code string) ([]byte, error) {
	if len(code) > operatorProgramSourceLimit || strings.Contains(code, "include") {
		return nil, fmt.Errorf("rule source requires ordinary compilation")
	}
	return operatorProgramCache.GetOrLoad(code, func() (program []byte, err error) {
		// The cache loader must return normally even if compilation panics, so
		// concurrent waiters are released. A fresh compiler also avoids retaining
		// the current node's callbacks in the cached program.
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("compile operator: %v", recovered)
			}
		}()
		// This in-process cache needs neither a versioned file header nor gzip.
		// Use the same default compiler as a new operator engine and retain its
		// raw serialized output. Fresh unmarshalling keeps mutable values local.
		compiler := yakast.NewYakCompiler()
		compiler.Compiler(code)
		if errs := compiler.GetErrors(); len(errs) > 0 {
			return nil, errs
		}
		program, err = yakvm.NewCodesMarshaller().Marshal(compiler.GetRootSymbolTable(), compiler.GetOpcodes())
		if len(program) > operatorProgramArtifactLimit {
			return nil, fmt.Errorf("operator artifact exceeds cache limit")
		}
		return program, err
	})
}

func evalOperatorProgram(ctx context.Context, engine *antlr4yak.Engine, code string) error {
	program, err := loadOperatorProgram(code)
	if err != nil {
		// Serialization is an optimization, not a new language restriction. The
		// ordinary evaluator preserves both custom-rule behavior and diagnostics.
		return engine.SafeEvalWithoutCache(ctx, code)
	}
	return execOperatorProgram(ctx, engine, code, program)
}

func execOperatorProgram(ctx context.Context, engine *antlr4yak.Engine, code string, program []byte) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	decoder := yakvm.NewCodesMarshaller()
	decoder.SetSourceCode(code)
	symbols, codes, err := decoder.Unmarshal(program)
	if err != nil {
		return err
	}
	bindRuleProgramSource(codes, &code)
	engine.GetVM().SetSymboltable(symbols)
	return engine.GetVM().ExecYakCode(ctx, code, codes, yakvm.None)
}

// The bytecode format retains locations but not Code.SourceCodePointer. Bind
// each freshly decoded instruction to this invocation's immutable source. An
// inline eval can change Frame.originCode, so relying on the frame's fallback
// would otherwise replace an outer error location with the inner eval source.
// Function and deferred-code operands are the code-bearing forms serialized by
// CodesMarshaller; no runtime values or cached artifacts are modified here.
func bindRuleProgramSource(codes []*yakvm.Code, source *string) {
	for _, code := range codes {
		code.SourceCodePointer = source
		for _, operand := range [2]*yakvm.Value{code.Op1, code.Op2} {
			if operand == nil {
				continue
			}
			switch value := operand.Value.(type) {
			case *yakvm.Function:
				bindRuleProgramSource(value.GetCodes(), source)
			case []*yakvm.Code:
				bindRuleProgramSource(value, source)
			}
		}
	}
}
