package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
