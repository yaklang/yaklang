package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
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

func TestInvokeGoFuncShadowCallsByteCallback(t *testing.T) {
	var got []byte
	fn := func(b []byte) { got = append([]byte(nil), b...) }
	raw := uint64(uintptr(newRuntimeShadow(fn)))
	arg := uint64(uintptr(newRuntimeShadow([]byte("abc"))))
	words := make([]uint64, abi.HeaderWords+2)
	ctx := unsafe.Pointer(&words[0])
	ctxInit(ctx, abi.KindCallable, raw, 1)
	ctxStoreWord(ctx, abi.HeaderWords, arg)
	ctxStoreWord(ctx, abi.HeaderWords+1, arg)
	if !invokeGoFuncShadow(ctx, raw) {
		t.Fatal("expected go func shadow to be invoked")
	}
	if string(got) != "abc" {
		t.Fatalf("callback saw %q", got)
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
	args, err := runtimeDecodeCallArgs(fn, raw, true)
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
	om := newRuntimeOrderedMap()
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
	om := newRuntimeOrderedMap()
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

func TestSetRuntimeFieldYaklibOrderedMapDecodesValues(t *testing.T) {
	om := orderedmap.New(map[string]any{"existing": "ok"})
	if err := setRuntimeField(om, "compile_immediately", 1, abi.FlagFieldBool); err != nil {
		t.Fatalf("set yaklib ordered map bool: %v", err)
	}
	if got, ok := om.Get("compile_immediately"); !ok || got != true {
		t.Fatalf("compile_immediately = %#v, ok=%v", got, ok)
	}

	raw := int64(uintptr(newRuntimeShadow("local")))
	raw |= int64(yakTaggedPointerMask)
	if err := setRuntimeField(om, "kind", raw); err != nil {
		t.Fatalf("set yaklib ordered map shadow string: %v", err)
	}
	if got, ok := om.Get("kind"); !ok || got != "local" {
		t.Fatalf("kind = %#v, ok=%v", got, ok)
	}

	value, err := resolveField(om, "existing")
	if err != nil {
		t.Fatalf("resolve yaklib ordered map field: %v", err)
	}
	if got := value.Interface(); got != "ok" {
		t.Fatalf("existing = %#v", got)
	}
}

func TestSliceAnyBoolNilRoundTrip(t *testing.T) {
	s := make([]any, 4)
	sp := &s
	if err := setRuntimeField(sp, "0", 123, 0); err != nil {
		t.Fatal(err)
	}
	if err := setRuntimeField(sp, "1", 1, abi.FlagFieldBool); err != nil {
		t.Fatal(err)
	}
	if err := setRuntimeField(sp, "2", 0, abi.FlagFieldBool); err != nil {
		t.Fatal(err)
	}
	if err := setRuntimeField(sp, "3", 0, abi.FlagFieldNil); err != nil {
		t.Fatal(err)
	}
	if s[0] != int64(123) {
		t.Fatalf("int element = %#v (%T)", s[0], s[0])
	}
	if s[1] != true {
		t.Fatalf("true element = %#v (%T)", s[1], s[1])
	}
	if s[2] != false {
		t.Fatalf("false element = %#v (%T)", s[2], s[2])
	}
	if s[3] != nil {
		t.Fatalf("nil element = %#v (%T)", s[3], s[3])
	}

	// Range yields the element into a map and the compiler reads it back
	// with get_field. That is the path fuzz_json_params uses.
	check := func(name string, elem any, want any, truth int64) {
		t.Helper()
		result := map[string]any{"field": elem, "ok": true}
		field, err := resolveField(result, "field")
		if err != nil {
			t.Fatal(err)
		}
		word := runtimeFieldWord(field)
		if yak_runtime_is_true(word) != truth {
			t.Fatalf("%s truth = %d, want %d", name, yak_runtime_is_true(word), truth)
		}
		got := decodeTaggedArg(uint64(word))
		if got != want {
			t.Fatalf("%s decode = %#v (%T), want %#v", name, got, got, want)
		}
	}
	check("int", s[0], int64(123), 1)
	check("true", s[1], true, 1)
	check("false", s[2], false, 0)
	check("nil", s[3], nil, 0)

	trueWord := runtimeFieldWord(mustResolve(t, resultField(s[1]), "field"))
	falseWord := runtimeFieldWord(mustResolve(t, resultField(s[2]), "field"))
	nilWord := runtimeFieldWord(mustResolve(t, resultField(s[3]), "field"))
	if !runtimeValuesEqual(runtimeDecodeEqValue(uint64(trueWord)), runtimeDecodeEqValue(1)) {
		t.Fatal("true shadow should equal numeric 1")
	}
	if runtimeValuesEqual(runtimeDecodeEqValue(uint64(trueWord)), runtimeDecodeEqValue(0)) {
		t.Fatal("true shadow should not equal nil")
	}
	if !runtimeValuesEqual(runtimeDecodeEqValue(uint64(falseWord)), runtimeDecodeEqValue(0)) {
		t.Fatal("false shadow should equal nil/false word")
	}
	if !runtimeValuesEqual(runtimeDecodeEqValue(uint64(nilWord)), runtimeDecodeEqValue(0)) {
		t.Fatal("nil shadow should equal nil word")
	}
	if runtimeValuesEqual(runtimeDecodeEqValue(uint64(nilWord)), runtimeDecodeEqValue(1)) {
		t.Fatal("nil shadow should not equal 1")
	}
	if yak_runtime_is_true(0) != 0 || yak_runtime_is_true(1) != 1 || yak_runtime_is_true(2) != 1 {
		t.Fatal("plain integer truthiness changed")
	}
	missing, err := resolveField(sp, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if runtimeFieldWord(missing) != 0 {
		t.Fatal("missing field should stay word 0")
	}
}

func resultField(elem any) map[string]any {
	return map[string]any{"field": elem}
}

func mustResolve(t *testing.T, obj any, name string) reflect.Value {
	t.Helper()
	field, err := resolveField(obj, name)
	if err != nil {
		t.Fatal(err)
	}
	return field
}

func TestRuntimeOrderedMapMarshalJSON(t *testing.T) {
	om := newRuntimeOrderedMap()
	om.Set("project_exists", "aa")
	nested := newRuntimeOrderedMap()
	nested.Set("kind", "local")
	om.Set("info", nested)

	data, err := json.Marshal(om)
	if err != nil {
		t.Fatalf("marshal runtime ordered map: %v", err)
	}
	got := string(data)
	if got != `{"project_exists":"aa","info":{"kind":"local"}}` {
		t.Fatalf("unexpected json: %s", got)
	}
}

func TestRuntimeDispatchEqComparesShadowAndCStringStrings(t *testing.T) {
	shadow := uint64(uintptr(newRuntimeShadow("php"))) | yakTaggedPointerMask
	cstrBuf := []byte{'p', 'h', 'p', 0}
	cstr := uint64(uintptr(unsafe.Pointer(&cstrBuf[0]))) | yakTaggedPointerMask

	got, err := runtimeDispatchEq([]uint64{shadow, cstr, 0}, false)
	if err != nil {
		t.Fatalf("runtimeDispatchEq: %v", err)
	}
	if got != 1 {
		t.Fatalf("want equal strings, got %d", got)
	}

	got, err = runtimeDispatchEq([]uint64{shadow, cstr, 1}, false)
	if err != nil {
		t.Fatalf("runtimeDispatchEq not equal: %v", err)
	}
	if got != 0 {
		t.Fatalf("want negated equality to be false, got %d", got)
	}
}

func TestRuntimeDispatchEqTreatsTaggedNilAsNil(t *testing.T) {
	taggedNil := yakTaggedPointerMask
	got, err := runtimeDispatchEq([]uint64{taggedNil, 0, 0}, false)
	if err != nil {
		t.Fatalf("runtimeDispatchEq tagged nil: %v", err)
	}
	if got != 1 {
		t.Fatalf("want tagged nil to equal nil, got %d", got)
	}

	got, err = runtimeDispatchEq([]uint64{taggedNil, 0, 1}, false)
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
	om, ok := h.Value().(*runtimeOrderedMap)
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

func TestRuntimeDecodeCallableArgRejectsHeapPointer(t *testing.T) {
	buf := []byte("not code")
	target := reflect.TypeOf(func() {})
	if _, ok := runtimeDecodeCallableArg(uint64(uintptr(unsafe.Pointer(&buf[0]))), target); ok {
		t.Fatal("heap pointer must not become a callable")
	}
	raw := uint64(uintptr(newStdlibShadow("nope")))
	if _, ok := runtimeDecodeCallableArg(raw, target); ok {
		t.Fatal("string shadow must not become a callable")
	}
}

func TestRuntimeDecodeCallableReturns_YakTuple(t *testing.T) {
	msg := "api key is invalid"
	word := int64(uintptr(newStdlibShadow(msg)))
	tuple := []any{int64(0), word}
	ret := int64(uintptr(newStdlibShadow(&tuple)))
	words := make([]uint64, abi.HeaderWords)
	ctx := unsafe.Pointer(&words[0])
	ctxInit(ctx, abi.KindCallable, 0, 0)
	ctxSetRet(ctx, ret)

	target := reflect.TypeOf(func() (any, any) { return nil, nil })
	out := runtimeDecodeCallableReturns(ctx, target)
	if len(out) != 2 {
		t.Fatalf("results = %d", len(out))
	}
	if out[0].IsValid() && !out[0].IsNil() {
		t.Fatalf("first = %#v", out[0].Interface())
	}
	if !out[1].IsValid() || out[1].Interface() != msg {
		t.Fatalf("second = %#v", out[1])
	}

	inner := []any{"yaklang1", "yaklang2"}
	okTuple := []any{&inner, int64(0)}
	ctxSetRet(ctx, int64(uintptr(newStdlibShadow(&okTuple))))
	out = runtimeDecodeCallableReturns(ctx, target)
	list, ok := out[0].Interface().([]any)
	if !ok || len(list) != 2 || list[0] != "yaklang1" || list[1] != "yaklang2" {
		t.Fatalf("list = %#v", out[0].Interface())
	}
	if !out[1].IsValid() || !out[1].IsNil() {
		t.Fatalf("nil tail = %#v", out[1])
	}
}

func TestRuntimeDecodeCallArgs_EllipsisDecodesStringWords(t *testing.T) {
	slice := []any{
		int64(uintptr(newStdlibShadow("logout"))),
		int64(uintptr(newStdlibShadow("delete"))),
	}
	raw := uint64(uintptr(newStdlibShadow(&slice)))
	fn := func(keywords ...string) {}
	args, err := runtimeDecodeCallArgs(reflect.ValueOf(fn), []uint64{raw}, true)
	if err != nil {
		t.Fatal(err)
	}
	got := args[0].Interface().([]string)
	if len(got) != 2 || got[0] != "logout" || got[1] != "delete" {
		t.Fatalf("unpacked = %#v", got)
	}
}

type ellipsisProbe struct{}

func (ellipsisProbe) Start(targets ...string) int { return len(targets) }

func TestShadowMethodEllipsisUnpacksYakSlice(t *testing.T) {
	targets := makeRuntimeSlice(abi.SliceElemAny, 0, 2)
	ps, ok := targets.(*[]any)
	if !ok {
		t.Fatalf("yak slice type %T", targets)
	}
	*ps = append(*ps, "127.0.0.1:1", "127.0.0.1:2")

	obj := newStdlibShadow(ellipsisProbe{})
	slice := newStdlibShadow(targets)
	name := append([]byte("Start"), 0)
	args := []uint64{
		uint64(uintptr(obj)),
		uint64(uintptr(unsafe.Pointer(&name[0]))),
		uint64(uintptr(slice)),
	}

	ret, err := runtimeDispatchShadowMethod(args, true)
	if err != nil {
		t.Fatalf("ellipsis Start: %v", err)
	}
	if ret != 2 {
		t.Fatalf("Start returned %d, want 2", ret)
	}

	_, err = runtimeDispatchShadowMethod(args, false)
	if err == nil || !strings.Contains(err.Error(), "cannot use *[]interface {} as string") {
		t.Fatalf("missing ellipsis flag should reject the slice, got %v", err)
	}
}
