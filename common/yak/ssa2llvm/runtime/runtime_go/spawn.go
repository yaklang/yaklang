package main

/*
#include <stdint.h>

uintptr_t yak_ctx_root_add(void* ctx);
void* yak_ctx_root_get(uintptr_t handle);
void yak_ctx_root_remove(uintptr_t handle);

void yak_invoke_callable(uintptr_t fn, void* ctx);
*/
import "C"

import (
	"fmt"
	"os"
	"sync"
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

var yakAsyncMu sync.Mutex
var yakAsyncWaitGroup sync.WaitGroup

func recoverAsyncPanic() {
	if r := recover(); r != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[yak-runtime] async panic: %v\n", r)
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
	C.yak_invoke_callable(C.uintptr_t(target), ctx)
}

func executeInvoke(ctx unsafe.Pointer) {
	if ctx == nil {
		return
	}

	switch ctxLoadWord(ctx, abi.WordKind) {
	case abi.KindCallable:
		invokeCallable(ctx)
	case abi.KindDispatch:
		invokeDispatch(ctx)
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
		defer func() {
			yakAsyncMu.Lock()
			C.yak_ctx_root_remove(h)
			yakAsyncMu.Unlock()

			yakAsyncWaitGroup.Done()
			recoverAsyncPanic()
		}()

		cctx := C.yak_ctx_root_get(h)
		if cctx == nil {
			return
		}

		executeInvoke(cctx)
	}(handle)
}

//export yak_runtime_invoke
func yak_runtime_invoke(ctx unsafe.Pointer) {
	defer recoverRuntimePanic()

	if ctx == nil {
		return
	}

	if (ctxLoadWord(ctx, abi.WordFlags) & abi.FlagAsync) != 0 {
		spawnInvoke(ctx)
		return
	}

	executeInvoke(ctx)
}
