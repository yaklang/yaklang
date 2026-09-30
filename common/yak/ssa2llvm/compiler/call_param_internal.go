package compiler

import (
	"fmt"
	"strings"

	"github.com/yaklang/go-llvm"
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
		// calls on the tuple object. Map lookups (a["c"]) also yield values:
		// the closure stored in the map, not a method on the map object.
		if key := mc.GetKey(); key != nil && c.memberKeyIsStringConst(key) {
			isMapLookup := false
			if obj := ssa.GetLatestObject(calleeVal); obj != nil && obj.GetType() != nil && obj.GetType().GetTypeKind() == ssa.MapTypeKind {
				// m.Keys()/m.Values()/m.Has()/m.Set() are map methods. A stored
				// closure under any other name is still called as a value.
				if yakMapBuiltinMethod(c.resolveMemberKeyString(key)) {
					return contextCallSpec{}, false, nil
				}
				isMapLookup = true
			}
			if !isMapLookup {
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

	// The callee is an existing closure object (a returned or stored
	// function), not a freshly materialized one. Side effects of this call
	// have to read that same object; otherwise a lifted phi in the caller
	// has no predecessor and the captured value becomes garbage.
	if c.function != nil && !targetVal.IsNil() {
		if c.function.materializedClosures == nil {
			c.function.materializedClosures = make(map[int64]llvm.Value)
		}
		c.function.materializedClosures[inst.GetId()] = c.coerceToInt64(targetVal)
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
	// getParam and getParams are the interpreter's aliases of param. The
	// runtime table only registers param, which reads the environment.
	switch calleeName {
	case "getParam", "getParams":
		return "", "param"
	}
	if pkgName, methodName, ok := splitQualifiedName(calleeName); ok {
		return pkgName, methodName
	}
	return "", calleeName
}

func (c *Compiler) lowerYaklibDispatchCall(inst *ssa.Call, calleeName string) error {
	// die/fail are yak-level fatal errors: they set the invoke-context panic
	// (like a panic instruction) instead of calling the runtime global, which
	// Go-panics and therefore bypasses the AOT closure's context-based
	// `defer recover()`. Routing through the context lets a recover() in the
	// caller catch die (retry's `defer recover(); die(111)` contract).
	if calleeName == "die" || calleeName == "fail" {
		return c.compileDieAsPanic(inst)
	}
	pkg, method := yaklibDispatchNames(calleeName)
	c.recordYaklibDependency(pkg, method)
	spec, err := c.newYaklibDispatchSpec(inst, pkg, method)
	if err != nil {
		return err
	}
	return c.lowerResolvedContextCall(spec)
}

// compileDieAsPanic lowers die/fail to a context panic, mirroring compilePanic.
// yaklib die(nil) returns and the script continues. A bare nil is word 0.
// A nil pulled out of a multi-return tuple is the nil shadow, which is not
// word 0. yak_runtime_is_true is 0 for both of those and for false (already
// equal to nil in this ABI), and 1 for a real error, a message, or die(111).
// A non-nil argument still aborts the function immediately and does not run
// the current function's defer block.
func (c *Compiler) compileDieAsPanic(inst *ssa.Call) error {
	if c == nil || c.function == nil || c.function.llvmFn.IsNil() || inst == nil {
		return fmt.Errorf("compileDieAsPanic: no active function")
	}
	i64 := c.LLVMCtx.Int64Type()
	infoVal := llvm.ConstInt(i64, 0, false)
	flags := uint64(0)
	if len(inst.Args) > 0 {
		val, err := c.getValue(inst, inst.Args[0])
		if err != nil {
			return err
		}
		infoVal = c.coerceToInt64(val)
		fn := inst.GetFunc()
		if fn != nil {
			if argVal, ok := fn.GetValueById(inst.Args[0]); ok && argVal != nil && c.ssaValueIsPointer(argVal, fn) {
				flags = abi.FlagPanicTaggedPointer
			}
		}
	}
	// Argument resolution may have moved the builder. The branch belongs to
	// the die call's own block.
	dieBB := c.restoreInsertBlock(inst)
	if !dieBB.IsNil() {
		c.restoreInsertPoint(dieBB)
	}
	if len(inst.Args) == 0 {
		return nil
	}
	origBB := c.currentInsertBlock()
	if origBB.IsNil() {
		return fmt.Errorf("compileDieAsPanic: missing insert block")
	}
	contBB := c.LLVMCtx.AddBasicBlock(c.function.llvmFn, fmt.Sprintf("die_ok_%d", inst.GetId()))
	abortBB := c.LLVMCtx.AddBasicBlock(c.function.llvmFn, fmt.Sprintf("die_abort_%d", inst.GetId()))
	isTrueFn, isTrueType := c.getOrInsertRuntimeIsTrue()
	truthy := c.Builder.CreateCall(isTrueType, isTrueFn, []llvm.Value{infoVal}, fmt.Sprintf("die_true_%d", inst.GetId()))
	isNil := c.Builder.CreateICmp(llvm.IntEQ, truthy, llvm.ConstInt(i64, 0, false), fmt.Sprintf("die_nil_%d", inst.GetId()))

	// Non-nil die is fatal: flush captured-variable writebacks first so the
	// caller observes the assignments that ran before die (retry's count++
	// before `if count > 3 { die(111) }`), then store the panic and return.
	// Unlike a recoverable panic, die deliberately bypasses the current
	// function's own defer block.
	c.Builder.SetInsertPointAtEnd(abortBB)
	if fn := inst.GetFunc(); fn != nil {
		if err := c.applyClosureSideEffectWriteback(fn, inst); err != nil {
			return err
		}
	}
	if err := c.storeContextPanic(infoVal, flags); err != nil {
		return err
	}
	c.Builder.CreateRetVoid()

	c.Builder.SetInsertPointAtEnd(origBB)
	c.Builder.CreateCondBr(isNil, contBB, abortBB)
	c.Builder.SetInsertPointAtEnd(contBB)
	if block := inst.GetBlock(); block != nil && block.GetId() > 0 {
		c.Blocks[block.GetId()] = contBB
		if c.function != nil {
			c.function.activeBlockID = block.GetId()
		}
	}
	return nil
}
