package main

import (
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestRuntimeCallReturnValueSingle(t *testing.T) {
	got := runtimeCallReturnValue([]reflect.Value{reflect.ValueOf(int64(42))})
	if got == 0 {
		t.Fatal("expected non-zero shadow handle for single return")
	}
}

func TestRuntimeCallReturnValueMulti(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeAll)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if cfg == nil {
		t.Fatal("New returned nil config")
	}

	handle := runtimeCallReturnValue([]reflect.Value{
		reflect.ValueOf(cfg),
		reflect.ValueOf(error(nil)),
	})
	if handle == 0 {
		t.Fatal("expected tuple shadow handle")
	}

	h, ok := handleFromShadow(unsafe.Pointer(uintptr(handle)))
	if !ok {
		t.Fatal("invalid tuple shadow")
	}
	tuple, ok := h.Value().([]any)
	if !ok || len(tuple) != 2 {
		t.Fatalf("want []any len 2, got %T %#v", h.Value(), h.Value())
	}
	if tuple[0] == nil {
		t.Fatal("tuple[0] should be config")
	}
	if tuple[1] != nil {
		t.Fatal("tuple[1] should be nil error")
	}

	idx0, err := resolveField(h.Value(), "0")
	if err != nil {
		t.Fatalf("resolveField 0: %v", err)
	}
	if !idx0.IsValid() || idx0.IsNil() {
		t.Fatal("resolveField index 0 invalid")
	}
}

func TestRuntimeDecodeCallArgsVariadicSliceArg(t *testing.T) {
	fn := reflect.ValueOf(ssaconfig.WithCompileExcludeFiles)
	raw := []uint64{uint64(uintptr(newRuntimeShadow([]any{})))}
	args, err := runtimeDecodeCallArgs(fn, raw)
	if err != nil {
		t.Fatalf("runtimeDecodeCallArgs: %v", err)
	}
	if len(args) != 1 {
		t.Fatalf("want 1 reflect arg for variadic slice call, got %d", len(args))
	}
	if args[0].Kind() != reflect.Slice {
		t.Fatalf("want slice arg, got %v", args[0].Kind())
	}

	ret := runtimeCallReturnValue(fn.CallSlice(args))
	if ret == 0 {
		t.Fatal("WithCompileExcludeFiles returned nil option handle")
	}
}

func TestRuntimeCallReturnValueMultiError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when last return is non-nil error")
		}
	}()
	runtimeCallReturnValue([]reflect.Value{
		reflect.ValueOf(1),
		reflect.ValueOf(errors.New("boom")),
	})
}
