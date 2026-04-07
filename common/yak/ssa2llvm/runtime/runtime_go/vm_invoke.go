package main

/*
#cgo linux LDFLAGS: -ldl
#include <stdint.h>
#include <stdlib.h>
#include <dlfcn.h>

typedef void (*yak_ctx_fn_t)(void*);

static const char* yak_invoke_ctx_symbol(void* ctx, const char* name) {
	dlerror();
	void* sym = dlsym(RTLD_DEFAULT, name);
	const char* err = dlerror();
	if (err != NULL) {
		return err;
	}
	((yak_ctx_fn_t)sym)(ctx);
	return NULL;
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	virtualizeruntime "github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/runtime"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func invokeVMBoundSymbol(symbol string, args []int64) (int64, error) {
	argc := len(args)
	ctxWords := make([]uint64, abi.HeaderWords+argc*2)
	ctx := unsafe.Pointer(&ctxWords[0])
	ctxInit(ctx, abi.KindCallable, 0, argc)
	for i, arg := range args {
		raw := uint64(arg)
		ctxWords[abi.HeaderWords+i] = raw
		if (raw & yakTaggedPointerMask) != 0 {
			ctxWords[abi.HeaderWords+argc+i] = raw &^ yakTaggedPointerMask
		}
	}

	cSymbol := C.CString(symbol)
	defer C.free(unsafe.Pointer(cSymbol))
	if errMsg := C.yak_invoke_ctx_symbol(ctx, cSymbol); errMsg != nil {
		return 0, fmt.Errorf("vm runtime: invoke %q failed: %s", symbol, C.GoString(errMsg))
	}
	return int64(ctxLoadWord(ctx, abi.WordRet)), nil
}

//export yak_runtime_invoke_vm
func yak_runtime_invoke_vm(ctx unsafe.Pointer, blobHex *C.char, seedHex *C.char, funcName *C.char, hostBindingSpec *C.char) {
	defer recoverInvokePanic(ctx, false)

	if ctx == nil || blobHex == nil || seedHex == nil || funcName == nil {
		return
	}

	argc := ctxArgc(ctx)
	rawArgs := ctxArgsSlice(ctx, argc)
	args := make([]int64, argc)
	for i, arg := range rawArgs {
		args[i] = int64(arg)
	}

	result, err := virtualizeruntime.Execute(
		C.GoString(blobHex),
		C.GoString(seedHex),
		C.GoString(funcName),
		C.GoString(hostBindingSpec),
		args,
		func(id abi.FuncID, rawArgs []uint64) int64 {
			return dispatchRuntimeCall(id, rawArgs)
		},
		invokeVMBoundSymbol,
	)
	if err != nil {
		panic(err)
	}
	ctxSetRet(ctx, result)
}

//export yak_runtime_test_add1_ctx
func yak_runtime_test_add1_ctx(ctx unsafe.Pointer) {
	if ctx == nil {
		return
	}
	args := ctxArgsSlice(ctx, ctxArgc(ctx))
	if len(args) == 0 {
		ctxSetRet(ctx, 1)
		return
	}
	ctxSetRet(ctx, int64(args[0])+1)
}
