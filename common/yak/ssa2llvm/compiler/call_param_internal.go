package compiler

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) newDynamicCallableContextCallSpec(inst *ssa.Call, fn *ssa.Function, calleeVal ssa.Value) (contextCallSpec, bool, error) {
	if inst == nil || calleeVal == nil {
		return contextCallSpec{}, false, nil
	}

	if mc, ok := calleeVal.(ssa.MemberCall); ok && mc.IsMember() {
		return contextCallSpec{}, false, nil
	}
	if ssaFn, ok := ssa.ToFunction(calleeVal); ok && ssaFn != nil {
		return contextCallSpec{}, false, nil
	}

	targetVal, err := c.getValue(inst, calleeVal.GetId())
	if err != nil {
		if param, ok := ssa.ToParameter(calleeVal); ok {
			if val, ok := c.loadBoundParameterValue(fn, param); ok {
				targetVal = val
				err = nil
			}
		}
	}
	if err != nil {
		return contextCallSpec{}, false, nil
	}

	return contextCallSpec{
		inst:      inst,
		kind:      abi.KindCallable,
		target:    c.coerceToInt64(targetVal),
		args:      ssaArgs(append([]int64{}, inst.Args...), true),
		async:     inst.Async,
		ctxName:   "yak_dynamic_call_ctx",
		errPrefix: "emitDynamicCallableContextCall",
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
	c.recordYaklibDependency(pkg, method)
	spec, err := c.newYaklibDispatchSpec(inst, pkg, method)
	if err != nil {
		return err
	}
	return c.lowerResolvedContextCall(spec)
}
