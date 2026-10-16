package main

/*
#include <stdint.h>

uintptr_t yak_ctx_root_add(void* ctx);
void* yak_ctx_root_get(uintptr_t handle);
void yak_ctx_root_remove(uintptr_t handle);

void yak_invoke_callable(uintptr_t fn, void* ctx);
uintptr_t yak_test_closure_addr(void);
int yak_test_closure_hit_count(void);
*/
import "C"

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

var yakAsyncMu sync.Mutex
var yakAsyncWaitGroup sync.WaitGroup

func logRecoveredRuntimePanic(kind string, recovered any) {
	runtimeLogPanicRecovery(kind, recovered)
}

// runtimeTestClosureProbe is a native callable used to check that a yak
// closure registered through risk.RegisterBeforeRiskSave is actually invoked.
func runtimeTestClosureProbe() (fn uint64, hits func() int) {
	return uint64(C.yak_test_closure_addr()), func() int { return int(C.yak_test_closure_hit_count()) }
}

func recoveredPanicValue(recovered any) (uint64, uint64) {
	switch value := recovered.(type) {
	case nil:
		return 0, 0
	case int:
		return uint64(int64(value)), 0
	case int8:
		return uint64(int64(value)), 0
	case int16:
		return uint64(int64(value)), 0
	case int32:
		return uint64(int64(value)), 0
	case int64:
		return uint64(value), 0
	case uint:
		return uint64(value), 0
	case uint8:
		return uint64(value), 0
	case uint16:
		return uint64(value), 0
	case uint32:
		return uint64(value), 0
	case uint64:
		return value, 0
	case uintptr:
		return uint64(value), 0
	case bool:
		if value {
			return 1, 0
		}
		return 0, 0
	case string:
		ptr := newStdlibShadow(value)
		return uint64(uintptr(ptr)), abi.FlagPanicTaggedPointer
	case []byte:
		ptr := newStdlibShadow(value)
		return uint64(uintptr(ptr)), abi.FlagPanicTaggedPointer
	case error:
		ptr := newStdlibShadow(value.Error())
		return uint64(uintptr(ptr)), abi.FlagPanicTaggedPointer
	default:
		ptr := newStdlibShadow(fmt.Sprint(value))
		return uint64(uintptr(ptr)), abi.FlagPanicTaggedPointer
	}
}

func recoverInvokePanic(ctx unsafe.Pointer, async bool) {
	if recovered := recover(); recovered != nil {
		if ctx != nil {
			value, flags := recoveredPanicValue(recovered)
			ctxSetPanic(ctx, value, flags)
		}
		if async {
			logRecoveredRuntimePanic("async panic", recovered)
			return
		}
		logRecoveredRuntimePanic("panic", recovered)
	}
}

func invokeCallable(ctx unsafe.Pointer) {
	if ctx == nil {
		return
	}
	target := ctxLoadWord(ctx, abi.WordTarget)
	if target == 0 {
		return
	}
	if closure, ok := runtimeCallableClosureValueFromRaw(target); ok {
		invokeCallableClosure(ctx, closure)
		return
	}
	// A Go func handed into yak (mitm forward/drop, httpserver callbacks) is a
	// shadow handle, not an executable yak function. Calling the shadow
	// pointer jumps into non-executable memory.
	if invokeGoFuncShadow(ctx, target) {
		return
	}
	C.yak_invoke_callable(C.uintptr_t(target), ctx)
}

func invokeGoFuncShadow(ctx unsafe.Pointer, raw uint64) bool {
	if ctx == nil {
		return false
	}
	raw &^= yakTaggedPointerMask
	handle, ok := handleFromShadow(unsafe.Pointer(uintptr(raw)))
	if !ok {
		return false
	}
	fn := reflect.ValueOf(handle.Value())
	for fn.IsValid() && fn.Kind() == reflect.Interface {
		if fn.IsNil() {
			return false
		}
		fn = fn.Elem()
	}
	if !fn.IsValid() || fn.Kind() != reflect.Func {
		return false
	}
	argc := ctxArgc(ctx)
	if argc < 0 {
		argc = 0
	}
	ret, err := callRuntimeValue(fn, ctxArgsSlice(ctx, argc), false)
	if err != nil {
		panic(err)
	}
	ctxSetRet(ctx, ret)
	return true
}

func runtimeCallableClosureValueFromRaw(raw uint64) (runtimeCallableClosure, bool) {
	raw &^= yakTaggedPointerMask
	ptr := unsafe.Pointer(uintptr(raw))
	if ptr == nil {
		return runtimeCallableClosure{}, false
	}
	handle, ok := handleFromShadow(ptr)
	if !ok {
		return runtimeCallableClosure{}, false
	}
	return runtimeCallableClosureValue(handle.Value())
}

// invokeYaklibExportCallable dispatches a first-class yaklib export function
// value. The closure's freeValues carry (pkg, method) as untagged C-string
// pointers; the context args are the real call arguments.
func invokeYaklibExportCallable(ctx unsafe.Pointer, closure runtimeCallableClosure) {
	defer func() {
		if r := recover(); r != nil {
			// yak_runtime_invoke's recover does not see this panic: it is
			// stored on the context and the process exits 255. Log it here so
			// the same stderr line the main wrapper documents is actually written.
			logRecoveredRuntimePanic("panic", r)
			value, flags := recoveredPanicValue(r)
			ctxSetPanic(ctx, value, flags)
		}
	}()
	if ctx == nil || len(closure.freeValues) < 2 {
		return
	}
	argc := ctxArgc(ctx)
	if argc < 0 {
		return
	}
	inArgs := ctxArgsSlice(ctx, argc)
	callArgs := make([]uint64, 0, len(inArgs)+2)
	callArgs = append(callArgs, closure.freeValues[0], closure.freeValues[1])
	callArgs = append(callArgs, inArgs...)
	ret, err := runtimeDispatchYaklibCall(callArgs, false)
	if err != nil {
		panic(err)
	}
	ctxSetRet(ctx, ret)
}

func invokeCallableClosure(ctx unsafe.Pointer, closure runtimeCallableClosure) {
	if ctx == nil || closure.fn == 0 {
		return
	}
	if closure.fn == abi.YaklibExportCallableMarker {
		invokeYaklibExportCallable(ctx, closure)
		return
	}
	argc := ctxArgc(ctx)
	if argc < 0 {
		return
	}
	inArgs := ctxArgsSlice(ctx, argc)
	outArgc := argc + closure.paramMemberCount + len(closure.freeValues)
	words := make([]uint64, abi.HeaderWords+outArgc*2)
	childCtx := unsafe.Pointer(&words[0])
	ctxInit(childCtx, abi.KindCallable, closure.fn, outArgc)
	for index, raw := range inArgs {
		runtimeStoreCallableContextArg(childCtx, outArgc, index, raw)
	}
	for i := 0; i < closure.paramMemberCount; i++ {
		runtimeStoreCallableContextArg(childCtx, outArgc, argc+i, 0)
	}
	for i, capture := range closure.freeValues {
		runtimeStoreCallableContextArg(childCtx, outArgc, argc+closure.paramMemberCount+i, capture)
	}
	// Deferred closures are separate calls. recover() reads this child
	// context, so a panic stored on the deferring frame has to be visible
	// here, and a recover() that clears it has to be copied back below.
	ctxStoreWord(childCtx, abi.WordPanic, ctxLoadWord(ctx, abi.WordPanic))
	if ctxLoadWord(ctx, abi.WordFlags)&abi.FlagPanicTaggedPointer != 0 {
		ctxSetFlags(childCtx, abi.FlagPanicTaggedPointer)
	}
	C.yak_invoke_callable(C.uintptr_t(closure.fn), childCtx)
	ctxSetRet(ctx, int64(ctxLoadWord(childCtx, abi.WordRet)))
	ctxStoreWord(ctx, abi.WordPanic, ctxLoadWord(childCtx, abi.WordPanic))
	ctxClearFlags(ctx, abi.FlagPanicTaggedPointer)
	if ctxLoadWord(childCtx, abi.WordFlags)&abi.FlagPanicTaggedPointer != 0 {
		ctxSetFlags(ctx, abi.FlagPanicTaggedPointer)
	}
}

func executeInvoke(ctx unsafe.Pointer) {
	switch ctxLoadWord(ctx, abi.WordKind) {
	case abi.KindCallable:
		invokeCallable(ctx)
	case abi.KindDispatch:
		executeRuntimeDispatch(ctx)
	default:
		// ignore
	}
}

func spawnInvoke(ctx unsafe.Pointer) {
	if ctx == nil {
		return
	}

	yakAsyncMu.Lock()
	handle := C.yak_ctx_root_add(ctx)
	yakAsyncMu.Unlock()

	if handle == 0 {
		return
	}

	yakAsyncWaitGroup.Add(1)
	go func(h C.uintptr_t) {
		var cctx unsafe.Pointer
		defer func() {
			recoverInvokePanic(cctx, true)

			yakAsyncMu.Lock()
			C.yak_ctx_root_remove(h)
			yakAsyncMu.Unlock()

			yakAsyncWaitGroup.Done()
		}()

		cctx = C.yak_ctx_root_get(h)
		if cctx == nil {
			return
		}

		executeInvoke(cctx)
	}(handle)
}

//export yak_runtime_wait_async
func yak_runtime_wait_async() {
	yakAsyncWaitGroup.Wait()
}

//export yak_runtime_invoke
func yak_runtime_invoke(ctx unsafe.Pointer) {
	defer recoverInvokePanic(ctx, false)

	if ctx == nil {
		return
	}

	if (ctxLoadWord(ctx, abi.WordFlags) & abi.FlagAsync) != 0 {
		spawnInvoke(ctx)
		return
	}

	executeInvoke(ctx)
}

//export yak_runtime_load_panic_value
func yak_runtime_load_panic_value(ctx unsafe.Pointer) int64 {
	return ctxNormalizedPanicValue(ctx)
}
