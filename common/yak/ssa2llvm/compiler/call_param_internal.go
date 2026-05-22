package compiler

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) newParameterCallableContextCallSpec(inst *ssa.Call, fn *ssa.Function, calleeVal ssa.Value) (contextCallSpec, bool, error) {
	if inst == nil || calleeVal == nil {
		return contextCallSpec{}, false, nil
	}

	param, ok := ssa.ToParameter(calleeVal)
	if !ok || param == nil {
		return contextCallSpec{}, false, nil
	}

	targetVal, err := c.getValue(inst, param.GetId())
	if err != nil {
		if val, ok := c.loadBoundParameterValue(fn, param); ok {
			targetVal = val
			err = nil
		}
	}
	if err != nil {
		return contextCallSpec{}, false, err
	}

	return contextCallSpec{
		inst:      inst,
		kind:      abi.KindCallable,
		target:    c.coerceToInt64(targetVal),
		args:      ssaArgs(append([]int64{}, inst.Args...), true),
		async:     inst.Async,
		ctxName:   "yak_param_call_ctx",
		errPrefix: "emitParameterCallableContextCall",
	}, true, nil
}

func yaklibDispatchNames(calleeName string) (pkg, method string) {
	if pkgName, methodName, ok := splitQualifiedName(calleeName); ok {
		return pkgName, methodName
	}
	return "", calleeName
}

func (c *Compiler) lowerYaklibDispatchCall(inst *ssa.Call, calleeName string) error {
	pkg, method := yaklibDispatchNames(calleeName)
	spec, err := c.newYaklibDispatchSpec(inst, pkg, method)
	if err != nil {
		return err
	}
	return c.lowerResolvedContextCall(spec)
}
