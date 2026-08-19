package compiler

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) newDynamicCallableContextCallSpec(inst *ssa.Call, fn *ssa.Function, calleeVal ssa.Value) (contextCallSpec, bool, error) {
	if inst == nil || calleeVal == nil {
		return contextCallSpec{}, false, nil
	}

	if mc, ok := calleeVal.(ssa.MemberCall); ok && mc.IsMember() {
		// A string-keyed member is a method call (obj.method()); route it to
		// the method dispatch. A numeric/dynamic key (slice index, map lookup)
		// yields a callable VALUE: materialize the member and call it.
		// Tuple/Next fields (#<id>.key / .field / .ok) are field reads that
		// yield values (e.g. the closure from a for-in iterator), not method
		// calls on the tuple object.
		if key := mc.GetKey(); key != nil && c.memberKeyIsStringConst(key) {
			memberName := calleeVal.GetName()
			// Only Next/tuple fields (#<id>.key / .field / .ok) are field
			// reads that yield values. Other #<id>.name members (e.g.
			// #5.Trim on a string) are method calls.
			if !strings.HasPrefix(memberName, "#") {
				return contextCallSpec{}, false, nil
			}
			if idx := strings.LastIndexByte(memberName, '.'); idx >= 0 {
				suffix := memberName[idx+1:]
				if suffix != "key" && suffix != "field" && suffix != "ok" {
					return contextCallSpec{}, false, nil
				}
			}
		}
		// A member read always yields a runtime value (the closure object
		// stored in the collection, or a method bound to the object). Even
		// when its static type is a function, do not treat it as a direct
		// function reference.
	} else if ssaFn, ok := ssa.ToFunction(calleeVal); ok && ssaFn != nil {
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
