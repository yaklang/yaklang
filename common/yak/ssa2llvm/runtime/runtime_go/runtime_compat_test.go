package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"
)

func TestConvertMapValue_OrderedMapToGoMap(t *testing.T) {
	om := newRuntimeOrderedMap()
	om.Set("a", int64(1))
	om.Set("b", "x")

	out, ok := convertMapValue(reflect.ValueOf(om), reflect.TypeOf(map[string]any{}))
	if !ok {
		t.Fatal("convertMapValue failed")
	}
	m := out.Interface().(map[string]any)
	if m["a"] != int64(1) || m["b"] != "x" {
		t.Fatalf("unexpected map: %#v", m)
	}
}

func TestConvertMapValue_PointerShadowToGoMap(t *testing.T) {
	om := newRuntimeOrderedMap()
	om.Set("k", "v")

	// The AOT shadow holds *runtimeOrderedMap; convertMapValue must unwrap it.
	out, ok := convertMapValue(reflect.ValueOf(om), reflect.TypeOf(map[string]interface{}{}))
	if !ok {
		t.Fatal("convertMapValue failed for pointer shadow")
	}
	m := out.Interface().(map[string]interface{})
	if m["k"] != "v" {
		t.Fatalf("unexpected map: %#v", m)
	}
}

func TestConvertSliceValue_AnyToString(t *testing.T) {
	src := []any{"a", "b", "c"}
	out, ok := convertSliceValue(reflect.ValueOf(src), reflect.TypeOf([]string{}))
	if !ok {
		t.Fatal("convertSliceValue failed")
	}
	got := out.Interface().([]string)
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("unexpected slice: %#v", got)
	}
}

func TestRuntimeDecodeArg_InterfaceUnwrapPtrSlice(t *testing.T) {
	slice := []any{int64(1), int64(2)}
	raw := uint64(uintptr(newStdlibShadow(&slice)))
	arg, err := runtimeDecodeArg(raw, reflect.TypeOf((*any)(nil)).Elem())
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if arg.Kind() != reflect.Slice {
		t.Fatalf("expected slice value, got %v", arg.Kind())
	}
	if arg.Len() != 2 {
		t.Fatalf("expected 2 elements, got %d", arg.Len())
	}
}

func TestRuntimeDecodeCallArgs_EllipsisUnpack(t *testing.T) {
	inner := []any{int64(1), int64(2)}
	outer := []any{inner}
	raw := uint64(uintptr(newStdlibShadow(&outer)))

	fn := func(args ...any) {}
	args, err := runtimeDecodeCallArgs(reflect.ValueOf(fn), []uint64{raw}, true)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 variadic slice arg, got %d", len(args))
	}
	slice := args[0]
	if slice.Len() != 1 {
		t.Fatalf("expected 1 unpacked element, got %d", slice.Len())
	}
}

func TestRuntimeDecodeCallArgs_SingleSliceStaysElement(t *testing.T) {
	inner := []any{int64(1), int64(2)}
	raw := uint64(uintptr(newStdlibShadow(&inner)))

	fn := func(args ...any) {}
	args, err := runtimeDecodeCallArgs(reflect.ValueOf(fn), []uint64{raw}, false)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	slice := args[0]
	if slice.Len() != 1 {
		t.Fatalf("expected 1 element (the slice itself), got %d", slice.Len())
	}
}

func TestRuntimeSliceAppend_ReturnsAppended(t *testing.T) {
	slice := []any{int64(1)}
	out := runtimeSliceAppend(slice, int64(2))
	got := out.([]any)
	if len(got) != 2 || got[1] != int64(2) {
		t.Fatalf("unexpected appended slice: %#v", got)
	}
}

func TestRuntimeResolveSliceMethod_PushMutates(t *testing.T) {
	slice := []any{}
	ptr := &slice
	method, ok := runtimeResolveSliceMethod(reflect.ValueOf(ptr), "Push")
	if !ok {
		t.Fatal("Push method not found")
	}
	method.Call([]reflect.Value{reflect.ValueOf(int64(7))})
	if len(slice) != 1 || slice[0] != int64(7) {
		t.Fatalf("Push did not mutate in place: %#v", slice)
	}
}

func TestRuntimeYakBuiltinLen_PtrSlice(t *testing.T) {
	slice := []any{int64(1), int64(2), int64(3)}
	if got := runtimeYakBuiltinLen(&slice); got != 3 {
		t.Fatalf("len(*[]any) = %d, want 3", got)
	}
	if got := runtimeYakBuiltinCap(&slice); got < 3 {
		t.Fatalf("cap(*[]any) = %d, want >= 3", got)
	}
}

func TestRuntimeBuiltinParam_Env(t *testing.T) {
	t.Setenv("YAK_TEST_PARAM", "hello")
	if got := runtimeBuiltinParam("YAK_TEST_PARAM"); got != "hello" {
		t.Fatalf("param = %v, want hello", got)
	}
	if got := runtimeBuiltinParam("YAK_TEST_MISSING", "def"); got != "def" {
		t.Fatalf("param default = %v, want def", got)
	}
	if got := runtimeBuiltinParam("YAK_TEST_MISSING"); got != nil {
		t.Fatalf("param missing = %v, want nil", got)
	}
}

func TestRuntimeBuiltinRetry_StopsOnFalse(t *testing.T) {
	count := 0
	runtimeBuiltinRetry(100, func() bool {
		count++
		return count < 3
	})
	if count != 3 {
		t.Fatalf("retry count = %d, want 3", count)
	}
}

func TestRuntimeBuiltinRetry_RecoversPanic(t *testing.T) {
	count := 0
	runtimeBuiltinRetry(100, func() bool {
		count++
		if count > 2 {
			panic("boom")
		}
		return true
	})
	if count != 3 {
		t.Fatalf("retry count = %d, want 3", count)
	}
}

func TestRuntimeResolveOSFileMethod_WriteLineReadLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	method, ok := runtimeResolveOSFileMethod(f, "WriteLine")
	if !ok {
		t.Fatal("WriteLine method not found")
	}
	method.Call([]reflect.Value{reflect.ValueOf("hello")})

	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	readLines, ok := runtimeResolveOSFileMethod(f, "ReadLines")
	if !ok {
		t.Fatal("ReadLines method not found")
	}
	res := readLines.Call(nil)
	lines := res[0].Interface().([]string)
	if len(lines) != 1 || lines[0] != "hello" {
		t.Fatalf("ReadLines = %#v, want [hello]", lines)
	}
}

func TestRuntimeDispatchShadowMethod_NilReceiver(t *testing.T) {
	ret, err := runtimeDispatchShadowMethod([]uint64{0, uint64(uintptr(newStdlibShadow("Close")))}, false)
	if err != nil {
		t.Fatalf("nil receiver should not error: %v", err)
	}
	if ret != 0 {
		t.Fatalf("nil receiver ret = %d, want 0", ret)
	}
}

func TestRuntimeOrderedMap_KeysValuesHasDelete(t *testing.T) {
	m := newRuntimeOrderedMap()
	m.Set("a", int64(1))
	m.Set("b", "x")
	m.Set("c", int64(3))
	keys := m.Keys()
	if len(keys) != 3 || keys[0] != "a" || keys[2] != "c" {
		t.Fatalf("Keys = %#v", keys)
	}
	values := m.Values()
	if len(values) != 3 || values[0] != int64(1) || values[2] != int64(3) {
		t.Fatalf("Values = %#v", values)
	}
	if !m.Has("b") || m.Has("z") {
		t.Fatalf("Has mismatch")
	}
	m.Delete("b")
	if m.Has("b") || m.Len() != 2 {
		t.Fatalf("Delete failed: %#v", m.Keys())
	}
	if keys := m.Keys(); keys[0] != "a" || keys[1] != "c" {
		t.Fatalf("Keys after delete = %#v", keys)
	}
}

func TestRuntimeStringSlice_RuneBounds(t *testing.T) {
	s := "你好"
	ptr := newStdlibShadow(s)
	out := yak_runtime_string_slice(ptr, 0, 2)
	got := runtimePtrToString(out)
	if got != s {
		t.Fatalf("slice 0:2 = %q, want %q", got, s)
	}
	out2 := yak_runtime_string_slice(ptr, 1, 2)
	got2 := runtimePtrToString(out2)
	if got2 != "好" {
		t.Fatalf("slice 1:2 = %q, want 好", got2)
	}
	// Negative/absent bounds slice to the full string.
	out3 := yak_runtime_string_slice(ptr, 0, -1)
	if got3 := runtimePtrToString(out3); got3 != s {
		t.Fatalf("slice 0:-1 = %q, want %q", got3, s)
	}
}

func TestRuntimeDropError_TupleAndSingle(t *testing.T) {
	// Multi-return tuple: drop error, keep first value.
	tuple := []any{int64(7), "boom"}
	raw := uintptr(newStdlibShadow(tuple))
	if got := yak_runtime_drop_error(unsafe.Pointer(raw)); got != 7 {
		t.Fatalf("drop_error tuple = %d, want 7", got)
	}
	// Single value (a string shadow): returned unchanged as its word.
	s := "abc"
	raw2 := uintptr(newStdlibShadow(s))
	out := yak_runtime_drop_error(unsafe.Pointer(raw2))
	if h, ok := handleFromShadow(unsafe.Pointer(uintptr(out))); !ok {
		t.Fatalf("drop_error single = %#x, want shadow handle", out)
	} else if h.Value() != s {
		t.Fatalf("drop_error single = %v, want %s", h.Value(), s)
	}
}

func TestRuntimeReadClosureFreeValue_ByRefSlot(t *testing.T) {
	slot := uint64(42)
	closure := runtimeCallableClosure{
		fn:         123,
		freeValues: []uint64{uint64(uintptr(unsafe.Pointer(&slot))), uint64(uintptr(unsafe.Pointer(&slot)))},
	}
	raw := uint64(uintptr(newRuntimeShadow(closure)))
	if got := yak_runtime_read_closure_free_value(raw, 1, 1); got != 42 {
		t.Fatalf("read closure free value = %d, want 42", got)
	}
	slot = 99
	if got := yak_runtime_read_closure_free_value(raw, 0, 1); got != 99 {
		t.Fatalf("read closure free value after update = %d, want 99", got)
	}
}
