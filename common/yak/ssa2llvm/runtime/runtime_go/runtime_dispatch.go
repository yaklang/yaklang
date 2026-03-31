package main

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

type runtimeDispatchFunc func(args []uint64) (int64, error)

var runtimeDispatchTargets = map[abi.FuncID]any{
	abi.IDPocTimeout:            runtimePocTimeout,
	abi.IDPocGet:                runtimePocGet,
	abi.IDPocGetHTTPPacketBody:  runtimePocGetHTTPPacketBody,
	abi.IDOsGetenv:              runtimeBuiltinGetenv,
	abi.IDPrint:                 runtimeBuiltinPrint,
	abi.IDPrintf:                runtimeBuiltinPrintf,
	abi.IDPrintln:               runtimeBuiltinPrintln,
	abi.IDYakitInfo:             runtimeBuiltinYakitInfo,
	abi.IDYakitWarn:             runtimeBuiltinYakitWarn,
	abi.IDYakitDebug:            runtimeBuiltinYakitDebug,
	abi.IDYakitError:            runtimeBuiltinYakitError,
	abi.IDSyncNewWaitGroup:      runtimeSyncNewWaitGroup,
	abi.IDSyncNewSizedWaitGroup: runtimeSyncNewSizedWaitGroup,
	abi.IDSyncNewLock:           runtimeSyncNewLock,
	abi.IDSyncNewMutex:          runtimeSyncNewMutex,
	abi.IDSyncNewRWMutex:        runtimeSyncNewRWMutex,
	abi.IDAppend:                runtimeSliceAppend,
	abi.IDSyncNewMap:            runtimeSyncNewMap,
	abi.IDSyncNewOnce:           runtimeSyncNewOnce,
	abi.IDSyncNewPool:           runtimeSyncNewPool,
	abi.IDSyncNewCond:           runtimeSyncNewCond,
}

var runtimeDispatchHandlers = map[abi.FuncID]runtimeDispatchFunc{
	abi.IDRuntimeShadowMethod: runtimeDispatchShadowMethod,
}

func executeRuntimeDispatch(ctx unsafe.Pointer) {
	if ctx == nil {
		return
	}

	argc := ctxArgc(ctx)
	if argc < 0 || argc > 256 {
		return
	}

	args := ctxArgsSlice(ctx, argc)
	ctxSetRet(ctx, dispatchRuntimeCall(abi.FuncID(int64(ctxLoadWord(ctx, abi.WordTarget))), args))
}

func dispatchRuntimeCall(id abi.FuncID, args []uint64) int64 {
	if handler, ok := runtimeDispatchHandlers[id]; ok && handler != nil {
		ret, err := handler(args)
		if err != nil {
			panic(err)
		}
		return ret
	}
	target, ok := runtimeDispatchTargets[id]
	if !ok {
		return 0
	}
	ret, err := callRuntimeFunction(target, args)
	if err != nil {
		panic(err)
	}
	return ret
}

func callRuntimeFunction(fn any, rawArgs []uint64) (int64, error) {
	value := reflect.ValueOf(fn)
	if !value.IsValid() || value.Kind() != reflect.Func {
		return 0, fmt.Errorf("invalid runtime dispatch target %T", fn)
	}
	return callRuntimeValue(value, rawArgs)
}
