package callret

/*
#include <llvm-c/Core.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
)

const (
	defaultValueStackSize = 65536
	defaultCallStackSize  = 65536
)

type stackState struct {
	i64  llvm.Type
	one  llvm.Value
	zero llvm.Value

	vsData llvm.Value // i64*
	vsSp   llvm.Value // i64*

	csData llvm.Value // i64*
	csSp   llvm.Value // i64*
}

func applyLLVM(ctx *core.Context) error {
	module := ctx.LLVM
	if module.C == nil {
		return nil
	}

	builder := module.NewBuilder()
	defer builder.Dispose()

	stateByFunc := make(map[uintptr]*stackState)

	// SSA stage rewrites call/return into four tiny helper calls. LLVM lowers
	// those helpers into explicit stack mutations so the final IR/asm no longer
	// contains normal Yak internal call sites for the transformed edges.
	for function := module.FirstFunction(); !function.IsNil(); function = function.NextFunction() {
		for block := function.FirstBasicBlock(); !block.IsNil(); block = block.NextBasicBlock() {
			for inst := block.FirstInstruction(); !inst.IsNil(); {
				next := inst.NextInstruction()
				if !isCallInst(inst) {
					inst = next
					continue
				}

				callee := calledValue(inst)
				name := callee.Name()
				if name != intrinsicVSPush && name != intrinsicVSPop && name != intrinsicCSPush && name != intrinsicCSPop {
					inst = next
					continue
				}

				state, ok := stateByFunc[uintptr(unsafe.Pointer(function.C))]
				if !ok {
					created, err := ensureStackState(module, builder, function)
					if err != nil {
						return err
					}
					state = created
					stateByFunc[uintptr(unsafe.Pointer(function.C))] = state
				}

				switch name {
				case intrinsicVSPush:
					lowerPush(builder, inst, state.vsData, state.vsSp, state)
				case intrinsicVSPop:
					lowerPop(builder, inst, state.vsData, state.vsSp, state)
				case intrinsicCSPush:
					lowerPush(builder, inst, state.csData, state.csSp, state)
				case intrinsicCSPop:
					lowerPop(builder, inst, state.csData, state.csSp, state)
				}

				inst = next
			}
		}
	}

	deleteIntrinsicIfDead(module, intrinsicVSPush)
	deleteIntrinsicIfDead(module, intrinsicVSPop)
	deleteIntrinsicIfDead(module, intrinsicCSPush)
	deleteIntrinsicIfDead(module, intrinsicCSPop)

	return nil
}

func deleteIntrinsicIfDead(module llvm.Module, name string) {
	if module.C == nil || name == "" {
		return
	}

	fn := module.NamedFunction(name)
	if fn.IsNil() {
		return
	}

	cv := (C.LLVMValueRef)(unsafe.Pointer(fn.C))
	if C.LLVMGetFirstUse(cv) != nil {
		return
	}
	C.LLVMDeleteFunction(cv)
}

func ensureStackState(module llvm.Module, builder llvm.Builder, function llvm.Value) (*stackState, error) {
	entry := function.FirstBasicBlock()
	if entry.IsNil() {
		return nil, fmt.Errorf("callret: llvm function %q has no basic blocks", function.Name())
	}

	first := entry.FirstInstruction()
	if !first.IsNil() {
		builder.SetInsertPointBefore(first)
	} else {
		builder.SetInsertPointAtEnd(entry)
	}

	llvmCtx := module.Context()
	i64 := llvmCtx.Int64Type()
	zero := llvm.ConstInt(i64, 0, false)
	one := llvm.ConstInt(i64, 1, false)

	vsSp := buildAlloca(builder, i64, "obf_vs_sp")
	csSp := buildAlloca(builder, i64, "obf_cs_sp")
	builder.CreateStore(zero, vsSp)
	builder.CreateStore(zero, csSp)

	vsCount := llvm.ConstInt(i64, defaultValueStackSize, false)
	csCount := llvm.ConstInt(i64, defaultCallStackSize, false)
	vsData := buildArrayAlloca(builder, i64, vsCount, "obf_vs")
	csData := buildArrayAlloca(builder, i64, csCount, "obf_cs")

	return &stackState{
		i64:    i64,
		one:    one,
		zero:   zero,
		vsData: vsData,
		vsSp:   vsSp,
		csData: csData,
		csSp:   csSp,
	}, nil
}

func lowerPush(builder llvm.Builder, call llvm.Value, data, sp llvm.Value, st *stackState) {
	if call.IsNil() || st == nil {
		return
	}
	builder.SetInsertPointBefore(call)
	value := call.Operand(0)

	spVal := builder.CreateLoad(st.i64, sp, "obf_sp")
	ptr := builder.CreateGEP(st.i64, data, []llvm.Value{spVal}, "obf_ptr")
	builder.CreateStore(value, ptr)

	nextSp := builder.CreateAdd(spVal, st.one, "obf_sp_next")
	builder.CreateStore(nextSp, sp)

	call.ReplaceAllUsesWith(st.zero)
	call.EraseFromParent()
}

func lowerPop(builder llvm.Builder, call llvm.Value, data, sp llvm.Value, st *stackState) {
	if call.IsNil() || st == nil {
		return
	}
	builder.SetInsertPointBefore(call)

	spVal := builder.CreateLoad(st.i64, sp, "obf_sp")
	nextSp := builder.CreateSub(spVal, st.one, "obf_sp_next")
	builder.CreateStore(nextSp, sp)

	ptr := builder.CreateGEP(st.i64, data, []llvm.Value{nextSp}, "obf_ptr")
	value := builder.CreateLoad(st.i64, ptr, "obf_pop")

	call.ReplaceAllUsesWith(value)
	call.EraseFromParent()
}

func isCallInst(inst llvm.Value) bool {
	if inst.IsNil() {
		return false
	}
	cv := (C.LLVMValueRef)(unsafe.Pointer(inst.C))
	return C.LLVMGetInstructionOpcode(cv) == C.LLVMCall
}

func calledValue(inst llvm.Value) llvm.Value {
	if inst.IsNil() {
		return llvm.Value{}
	}
	cv := (C.LLVMValueRef)(unsafe.Pointer(inst.C))
	return valueFromC(C.LLVMGetCalledValue(cv))
}

func buildAlloca(b llvm.Builder, t llvm.Type, name string) llvm.Value {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	cb := (C.LLVMBuilderRef)(unsafe.Pointer(b.C))
	ct := (C.LLVMTypeRef)(unsafe.Pointer(t.C))
	res := C.LLVMBuildAlloca(cb, ct, cname)
	return valueFromC(res)
}

func buildArrayAlloca(b llvm.Builder, elem llvm.Type, count llvm.Value, name string) llvm.Value {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	cb := (C.LLVMBuilderRef)(unsafe.Pointer(b.C))
	ce := (C.LLVMTypeRef)(unsafe.Pointer(elem.C))
	cc := (C.LLVMValueRef)(unsafe.Pointer(count.C))
	res := C.LLVMBuildArrayAlloca(cb, ce, cc, cname)
	return valueFromC(res)
}

func valueFromC(v C.LLVMValueRef) llvm.Value {
	var ret llvm.Value
	*(*unsafe.Pointer)(unsafe.Pointer(&ret)) = unsafe.Pointer(v)
	return ret
}
