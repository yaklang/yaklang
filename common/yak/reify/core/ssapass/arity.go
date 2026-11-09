package ssapass

import (
	"github.com/yaklang/yaklang/common/yak/reify/core/funcdefs"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// TrimInternalCalls removes register-spill arguments from calls whose target is
// a recovered internal function. Arity is inferred from parameters that are
// actually read by the callee. Known C signatures always win.
func TrimInternalCalls(program *ssa.Program) int {
	if program == nil {
		return 0
	}
	arities := make(map[string]int)
	program.Funcs.ForEach(func(name string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() {
			return true
		}
		arities[name] = usedArity(function)
		return true
	})
	trimmed := 0
	eachCall(program, func(_ *ssa.Function, call *ssa.Call) {
		name := internalCallName(call)
		if name == "" {
			return
		}
		if _, known := funcdefs.Lookup(name); known {
			return
		}
		if call.GetVerboseName() == "variadic" {
			return
		}
		arity, ok := arities[name]
		if !ok {
			return
		}
		limit := arity
		if limit > 6 {
			limit = 6
		}
		if len(call.Args) > limit {
			call.Args = call.Args[:limit]
			trimmed++
		}
	})
	return trimmed
}

func internalCallName(call *ssa.Call) string {
	if call == nil {
		return ""
	}
	value, ok := call.GetValueById(call.Method)
	if !ok || value == nil {
		return ""
	}
	if function, ok := ssa.ToFunction(value); ok && function != nil && function.IsExtern() {
		return function.GetName()
	}
	return value.GetName()
}

func usedArity(function *ssa.Function) int {
	arity := 0
	if function == nil {
		return arity
	}
	for _, id := range function.Params {
		parameter, ok := function.GetValueById(id)
		if !ok || parameter == nil || !parameter.HasUsers() {
			continue
		}
		typed, ok := ssa.ToParameter(parameter)
		if !ok || typed == nil || typed.FormalParameterIndex < 0 {
			continue
		}
		if typed.FormalParameterIndex+1 > arity {
			arity = typed.FormalParameterIndex + 1
		}
	}
	return arity
}
