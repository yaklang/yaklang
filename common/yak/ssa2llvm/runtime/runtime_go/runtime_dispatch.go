package main

import (
	"fmt"
	"os"
	"reflect"
	"unsafe"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

type runtimeDispatchFunc func(args []uint64) (int64, error)

type runtimeDispatchEntry struct {
	name   string
	invoke runtimeDispatchFunc
}

func runtimeReflectDispatch(name string, target any) runtimeDispatchEntry {
	return runtimeDispatchEntry{
		name: name,
		invoke: func(args []uint64) (int64, error) {
			return callRuntimeFunction(target, args)
		},
	}
}

func runtimeRawDispatch(name string, invoke runtimeDispatchFunc) runtimeDispatchEntry {
	return runtimeDispatchEntry{name: name, invoke: invoke}
}

var runtimeDispatchTable = map[abi.FuncID]runtimeDispatchEntry{
	abi.IDPocTimeout:            runtimeReflectDispatch("poc.timeout", builtinPocTimeout),
	abi.IDPocGet:                runtimeReflectDispatch("poc.Get", builtinPocGet),
	abi.IDPocGetHTTPPacketBody:  runtimeReflectDispatch("poc.GetHTTPPacketBody", builtinPocGetHTTPPacketBody),
	abi.IDOsGetenv:              runtimeReflectDispatch("os.Getenv", os.Getenv),
	abi.IDPrint:                 runtimeReflectDispatch("print", builtinPrint),
	abi.IDPrintf:                runtimeReflectDispatch("printf", builtinPrintf),
	abi.IDPrintln:               runtimeReflectDispatch("println", builtinPrintln),
	abi.IDYakitInfo:             runtimeReflectDispatch("yakit.Info", builtinYakitInfo),
	abi.IDYakitWarn:             runtimeReflectDispatch("yakit.Warn", builtinYakitWarn),
	abi.IDYakitDebug:            runtimeReflectDispatch("yakit.Debug", builtinYakitDebug),
	abi.IDYakitError:            runtimeReflectDispatch("yakit.Error", builtinYakitError),
	abi.IDSyncNewWaitGroup:      runtimeReflectDispatch("sync.NewWaitGroup", stdlibSyncNewWaitGroup),
	abi.IDSyncNewSizedWaitGroup: runtimeReflectDispatch("sync.NewSizedWaitGroup", stdlibSyncNewSizedWaitGroup),
	abi.IDSyncNewLock:           runtimeReflectDispatch("sync.NewLock", stdlibSyncNewLock),
	abi.IDSyncNewMutex:          runtimeReflectDispatch("sync.NewMutex", stdlibSyncNewMutex),
	abi.IDSyncNewRWMutex:        runtimeReflectDispatch("sync.NewRWMutex", stdlibSyncNewRWMutex),
	abi.IDRuntimeShadowMethod:   runtimeRawDispatch("runtime.shadowMethod", dispatchRuntimeShadowMethod),
	abi.IDAppend:                runtimeReflectDispatch("append", builtinAppend),
	abi.IDSyncNewMap:            runtimeReflectDispatch("sync.NewMap", stdlibSyncNewMap),
	abi.IDSyncNewOnce:           runtimeReflectDispatch("sync.NewOnce", stdlibSyncNewOnce),
	abi.IDSyncNewPool:           runtimeReflectDispatch("sync.NewPool", stdlibSyncNewPool),
	abi.IDSyncNewCond:           runtimeReflectDispatch("sync.NewCond", stdlibSyncNewCond),
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
	entry, ok := runtimeDispatchTable[id]
	if !ok || entry.invoke == nil {
		return 0
	}
	ret, err := entry.invoke(args)
	if err != nil {
		panic(fmt.Errorf("%s: %w", entry.name, err))
	}
	return ret
}

func normalizePrintArgs(args []any) []any {
	if len(args) == 0 {
		return nil
	}
	out := make([]any, 0, len(args))
	for _, arg := range args {
		out = append(out, normalizePrintArg(arg))
	}
	return out
}

func builtinPrint(args ...any) {
	_, _ = fmt.Fprint(os.Stdout, normalizePrintArgs(args)...)
}

func builtinPrintln(args ...any) {
	_, _ = fmt.Fprintln(os.Stdout, normalizePrintArgs(args)...)
}

func builtinPrintf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stdout, format, normalizePrintArgs(args)...)
}

func builtinYakitInfo(format string, args ...any) {
	builtinYakitLog("info", format, args...)
}

func builtinYakitWarn(format string, args ...any) {
	builtinYakitLog("warn", format, args...)
}

func builtinYakitDebug(format string, args ...any) {
	builtinYakitLog("debug", format, args...)
}

func builtinYakitError(format string, args ...any) {
	builtinYakitLog("error", format, args...)
}

func builtinYakitLog(level string, format string, args ...any) {
	msg := fmt.Sprintf(format, normalizePrintArgs(args)...)
	_, _ = fmt.Fprintf(os.Stderr, "[yakit][%s] %s\n", level, msg)
}

func builtinPocTimeout(timeout int64) any {
	return poc.WithTimeout(float64(timeout))
}

func builtinPocGet(url string, opt any) any {
	opts := make([]poc.PocConfigOption, 0, 1)
	if actual, ok := opt.(poc.PocConfigOption); ok {
		opts = append(opts, actual)
	}
	rsp, req, err := poc.DoGET(url, opts...)
	return []any{rsp, req, err}
}

func builtinPocGetHTTPPacketBody(packet []byte) []byte {
	return lowhttp.GetHTTPPacketBody(packet)
}

func builtinAppend(slice any, values ...any) any {
	sliceValue := reflect.ValueOf(slice)
	if !sliceValue.IsValid() || sliceValue.Kind() != reflect.Slice {
		panic(fmt.Errorf("append expects slice, got %T", slice))
	}

	elemType := sliceValue.Type().Elem()
	elems := make([]reflect.Value, 0, len(values))
	for _, raw := range values {
		elems = append(elems, coerceAppendValue(elemType, raw))
	}
	return reflect.Append(sliceValue, elems...).Interface()
}

func callRuntimeFunction(fn any, rawArgs []uint64) (int64, error) {
	value := reflect.ValueOf(fn)
	if !value.IsValid() || value.Kind() != reflect.Func {
		return 0, fmt.Errorf("invalid runtime dispatch target %T", fn)
	}
	return callRuntimeValue(value, rawArgs)
}

func coerceAppendValue(targetType reflect.Type, value any) reflect.Value {
	if value == nil {
		return reflect.Zero(targetType)
	}

	actual := reflect.ValueOf(value)
	if actual.IsValid() {
		if actual.Type().AssignableTo(targetType) {
			return actual
		}
		if actual.Type().ConvertibleTo(targetType) {
			return actual.Convert(targetType)
		}
		if targetType.Kind() == reflect.Interface && actual.Type().Implements(targetType) {
			return actual
		}
	}

	if intValue, ok := value.(int64); ok {
		if converted, ok := valueForSet(targetType, intValue); ok {
			return converted
		}
	}

	panic(fmt.Errorf("append cannot convert %T to %s", value, targetType))
}
