package main

import (
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
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

func TestResolveFieldCollectionNegativeIndex(t *testing.T) {
	got, err := resolveField([]string{"first", "last"}, "-1")
	if err != nil {
		t.Fatalf("resolveField -1: %v", err)
	}
	if got.String() != "last" {
		t.Fatalf("want last element, got %q", got.String())
	}

	_, err = resolveField([]string{"only"}, "-2")
	if err == nil {
		t.Fatal("expected out-of-range negative index error")
	}
}

func TestRuntimeResolveStringMethods(t *testing.T) {
	trim, err := runtimeResolveMethod("  Yak  ", "Trim")
	if err != nil {
		t.Fatalf("resolve Trim: %v", err)
	}
	trimmed := trim.CallSlice([]reflect.Value{reflect.ValueOf([]string{" "})})
	if len(trimmed) != 1 || trimmed[0].String() != "Yak" {
		t.Fatalf("Trim returned %#v", trimmed)
	}

	lower, err := runtimeResolveMethod("Yak", "Lower")
	if err != nil {
		t.Fatalf("resolve Lower: %v", err)
	}
	lowered := lower.Call(nil)
	if len(lowered) != 1 || lowered[0].String() != "yak" {
		t.Fatalf("Lower returned %#v", lowered)
	}

	hasPrefix, err := runtimeResolveMethod("yaklang", "HasPrefix")
	if err != nil {
		t.Fatalf("resolve HasPrefix: %v", err)
	}
	matched := hasPrefix.Call([]reflect.Value{reflect.ValueOf("yak")})
	if len(matched) != 1 || !matched[0].Bool() {
		t.Fatalf("HasPrefix returned %#v", matched)
	}
}

func TestRuntimeMakeCallableCopiesFreeValues(t *testing.T) {
	captures := []uint64{
		uint64(uintptr(newRuntimeShadow("capture"))),
		42,
	}
	raw := yak_runtime_make_callable(0x1234, 2, int64(len(captures)), unsafe.Pointer(&captures[0]))
	if raw == 0 {
		t.Fatal("expected callable closure shadow")
	}

	h, ok := handleFromShadow(unsafe.Pointer(uintptr(raw)))
	if !ok {
		t.Fatal("invalid callable closure shadow")
	}
	closure, ok := h.Value().(runtimeCallableClosure)
	if !ok {
		t.Fatalf("want runtimeCallableClosure, got %T", h.Value())
	}
	if closure.fn != 0x1234 {
		t.Fatalf("fn = %#x", closure.fn)
	}
	if closure.paramMemberCount != 2 {
		t.Fatalf("paramMemberCount = %d", closure.paramMemberCount)
	}
	if len(closure.freeValues) != len(captures) {
		t.Fatalf("free values len = %d", len(closure.freeValues))
	}
	captures[0] = 0
	if closure.freeValues[0] == 0 {
		t.Fatal("free values should be copied")
	}
	if closure.freeValues[1] != 42 {
		t.Fatalf("freeValues[1] = %d", closure.freeValues[1])
	}
}

func TestRuntimeDecodeCallableArgAcceptsClosureShadow(t *testing.T) {
	targetType := reflect.TypeOf(func(string) {})
	raw := uint64(uintptr(newRuntimeShadow(runtimeCallableClosure{
		fn:               0x1234,
		paramMemberCount: 1,
		freeValues:       []uint64{42},
	}))) | yakTaggedPointerMask

	value, ok := runtimeDecodeCallableArg(raw, targetType)
	if !ok {
		t.Fatal("expected callable closure decode")
	}
	if !value.IsValid() || value.Kind() != reflect.Func {
		t.Fatalf("want function value, got %#v", value)
	}
	if value.Type() != targetType {
		t.Fatalf("function type = %s", value.Type())
	}
}

func TestSetRuntimeFieldOrderedMapDecodesValues(t *testing.T) {
	om := orderedmap.New()
	if err := setRuntimeField(om, "enabled", 1); err != nil {
		t.Fatalf("set bool-like value: %v", err)
	}
	if got, ok := om.Get("enabled"); !ok || got != int64(1) {
		t.Fatalf("enabled = %#v, ok=%v", got, ok)
	}

	raw := int64(uintptr(newRuntimeShadow("local")))
	raw |= int64(yakTaggedPointerMask)
	if err := setRuntimeField(om, "kind", raw); err != nil {
		t.Fatalf("set shadow string: %v", err)
	}
	if got, ok := om.Get("kind"); !ok || got != "local" {
		t.Fatalf("kind = %#v, ok=%v", got, ok)
	}

	cstrBuf := []byte{'p', 'h', 'p', 0}
	if err := setRuntimeField(om, "language", int64(uintptr(unsafe.Pointer(&cstrBuf[0]))), abi.FlagFieldString); err != nil {
		t.Fatalf("set c string: %v", err)
	}
	if got, ok := om.Get("language"); !ok || got != "php" {
		t.Fatalf("language = %#v, ok=%v", got, ok)
	}
}

func TestSetRuntimeFieldOrderedMapStringFlagDoesNotReadInvalidTaggedPointer(t *testing.T) {
	om := orderedmap.New()
	nonCanonicalRaw := (uint64(2) << 48) | 0x1234
	tagged := int64(nonCanonicalRaw | yakTaggedPointerMask)
	if err := setRuntimeField(om, "description", tagged, abi.FlagFieldString); err != nil {
		t.Fatalf("set invalid tagged string pointer: %v", err)
	}

	got, ok := om.Get("description")
	if !ok {
		t.Fatal("description was not set")
	}
	if _, ok := got.(string); !ok {
		t.Fatalf("description should be stored as string, got %T %#v", got, got)
	}
}

func TestRuntimeDispatchEqComparesShadowAndCStringStrings(t *testing.T) {
	shadow := uint64(uintptr(newRuntimeShadow("php"))) | yakTaggedPointerMask
	cstrBuf := []byte{'p', 'h', 'p', 0}
	cstr := uint64(uintptr(unsafe.Pointer(&cstrBuf[0]))) | yakTaggedPointerMask

	got, err := runtimeDispatchEq([]uint64{shadow, cstr, 0})
	if err != nil {
		t.Fatalf("runtimeDispatchEq: %v", err)
	}
	if got != 1 {
		t.Fatalf("want equal strings, got %d", got)
	}

	got, err = runtimeDispatchEq([]uint64{shadow, cstr, 1})
	if err != nil {
		t.Fatalf("runtimeDispatchEq not equal: %v", err)
	}
	if got != 0 {
		t.Fatalf("want negated equality to be false, got %d", got)
	}
}

func TestRuntimeDispatchEqTreatsTaggedNilAsNil(t *testing.T) {
	taggedNil := yakTaggedPointerMask
	got, err := runtimeDispatchEq([]uint64{taggedNil, 0, 0})
	if err != nil {
		t.Fatalf("runtimeDispatchEq tagged nil: %v", err)
	}
	if got != 1 {
		t.Fatalf("want tagged nil to equal nil, got %d", got)
	}

	got, err = runtimeDispatchEq([]uint64{taggedNil, 0, 1})
	if err != nil {
		t.Fatalf("runtimeDispatchEq negated tagged nil: %v", err)
	}
	if got != 0 {
		t.Fatalf("want negated tagged nil equality to be false, got %d", got)
	}
}

func TestRuntimeMakeObjectCreatesOrderedMapShadow(t *testing.T) {
	raw := yak_runtime_make_object()
	if raw == 0 {
		t.Fatal("expected object shadow handle")
	}

	h, ok := handleFromShadow(unsafe.Pointer(uintptr(raw)))
	if !ok {
		t.Fatal("invalid object shadow")
	}
	om, ok := h.Value().(*orderedmap.OrderedMap)
	if !ok {
		t.Fatalf("want ordered map, got %T", h.Value())
	}

	if err := setRuntimeField(om, "kind", int64(uintptr(newRuntimeShadow("local")))|int64(yakTaggedPointerMask)); err != nil {
		t.Fatalf("set ordered map field: %v", err)
	}
	if got, ok := om.Get("kind"); !ok || got != "local" {
		t.Fatalf("kind = %#v, ok=%v", got, ok)
	}
}

func TestRuntimeCallReturnValueMultiError(t *testing.T) {
	handle := runtimeCallReturnValue([]reflect.Value{
		reflect.ValueOf(1),
		reflect.ValueOf(errors.New("boom")),
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
	if tuple[1] == nil {
		t.Fatal("tuple[1] should preserve non-nil error")
	}
}
