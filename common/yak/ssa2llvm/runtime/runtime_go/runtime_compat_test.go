package main

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"testing"
	"unsafe"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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

func TestRuntimeMatchIn_OrderedMap(t *testing.T) {
	m := newRuntimeOrderedMap()
	m.Set("a", int64(1))
	m.Set("b", int64(2))
	if !runtimeMatchInContainer("a", m) || runtimeMatchInContainer("z", m) {
		t.Fatalf("ordered map membership: a=%v z=%v", runtimeMatchInContainer("a", m), runtimeMatchInContainer("z", m))
	}
	if !runtimeMatchInContainer([]byte("b"), m) {
		t.Fatal("[]byte key should match the string key")
	}
	if !runtimeMatchInContainer("cde", "abcdef") || runtimeMatchInContainer("zzz", "abcdef") {
		t.Fatal("string containment regressed")
	}
	plain := map[string]int{"a": 1}
	if !runtimeMatchInContainer("a", plain) || runtimeMatchInContainer("z", plain) {
		t.Fatal("reflect map membership regressed")
	}
	slice := []string{"method", "body"}
	if !runtimeMatchInContainer("method", slice) || runtimeMatchInContainer("nope", slice) {
		t.Fatal("plain slice membership regressed")
	}
	// make([]string) stores *[]string so later writes share the header.
	if !runtimeMatchInContainer("method", &slice) || runtimeMatchInContainer("nope", &slice) {
		t.Fatal("*[]string membership should see the elements")
	}
	anySlice := []any{"method", "body"}
	if !runtimeMatchInContainer("body", &anySlice) || runtimeMatchInContainer("nope", &anySlice) {
		t.Fatal("*[]any membership should see the elements")
	}
	var nilSlice *[]string
	if runtimeMatchInContainer("method", nilSlice) {
		t.Fatal("nil slice pointer is not a container")
	}
	if !runtimeMatchInContainer("a", &plain) || runtimeMatchInContainer("z", &plain) {
		t.Fatal("*map membership should see the keys")
	}
	raw := []byte("HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 200 OK")
	if !runtimeMatchInContainer("HTTP/1.1 100 Continue", raw) || runtimeMatchInContainer("NOPE", raw) {
		t.Fatal("[]byte substring membership regressed")
	}
	if !runtimeMatchInContainer("HTTP/1.1 100 Continue", &raw) || runtimeMatchInContainer("NOPE", &raw) {
		t.Fatal("*[]byte substring membership regressed")
	}
	if !runtimeMatchInContainer([]byte("100"), "HTTP/1.1 100 Continue") || runtimeMatchInContainer([]byte("NOPE"), "HTTP/1.1 100 Continue") {
		t.Fatal("[]byte needle in string regressed")
	}
}

func TestRuntimeDecodeRiskOptionFunc(t *testing.T) {
	opt := yakit.WithRiskParam_Title("no")
	raw := uint64(runtimeValueToInt64(reflect.ValueOf(opt)))
	if raw == 0 {
		t.Fatal("boxing risk.title result produced 0")
	}
	tagged := raw | yakTaggedPointerMask
	elem := reflect.TypeOf(yakit.NewRisk).In(1).Elem()
	got, err := runtimeDecodeArg(tagged, elem)
	if err != nil {
		t.Fatalf("decode option: %v", err)
	}
	if !got.IsValid() || got.Kind() != reflect.Func || got.IsNil() {
		t.Fatalf("decoded option = %#v (%v)", got, got.Type())
	}
	risk := &schema.Risk{}
	got.Call([]reflect.Value{reflect.ValueOf(risk)})
	if risk.Title != "no" {
		t.Fatalf("title = %q", risk.Title)
	}

	hostRaw := uint64(uintptr(newStdlibShadow("127.0.0.1:111"))) | yakTaggedPointerMask
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("NewRisk panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	if _, err := callRuntimeValue(reflect.ValueOf(yakit.NewRisk), []uint64{hostRaw, tagged}, false); err != nil {
		t.Fatalf("call NewRisk: %v", err)
	}
}

// pinnedCString returns an untagged pointer to a NUL-terminated copy of s.
// The caller must KeepAlive the returned buffer until the pointer is unused.
func pinnedCString(s string) (uint64, []byte) {
	buf := append([]byte(s), 0)
	return uint64(uintptr(unsafe.Pointer(&buf[0]))), buf
}

func TestRuntimeRiskExportUsesNilYakitClient(t *testing.T) {
	// AOT builds skip AutoInitYakit, so the export must not call Output on nil.
	previous := yaklib.GetYakitClientInstance()
	yaklib.InitYakit(nil)
	t.Cleanup(func() { yaklib.InitYakit(previous) })

	yak_register_module_risk()
	pkg, pkgBuf := pinnedCString("risk")
	titleName, titleBuf := pinnedCString("title")
	argNo, argBuf := pinnedCString("no")
	defer runtime.KeepAlive(pkgBuf)
	defer runtime.KeepAlive(titleBuf)
	defer runtime.KeepAlive(argBuf)

	ret, err := runtimeDispatchYaklibCall([]uint64{pkg, titleName, argNo | yakTaggedPointerMask}, false)
	if err != nil {
		t.Fatalf("risk.title: %v", err)
	}
	if ret == 0 {
		t.Fatal("risk.title returned 0")
	}
	elem := reflect.TypeOf(yakit.NewRisk).In(1).Elem()
	opt, err := runtimeDecodeArg(uint64(ret)|yakTaggedPointerMask, elem)
	if err != nil || !opt.IsValid() || opt.Kind() != reflect.Func || opt.IsNil() {
		t.Fatalf("title return does not decode as option: ret=%#x err=%v opt=%#v", uint64(ret), err, opt)
	}

	called := false
	yakit.RegisterBeforeRiskSave(func(r *schema.Risk) {
		called = true
		if r != nil && r.Title != "no" {
			t.Errorf("callback title = %q", r.Title)
		}
	})

	newRisk, newRiskBuf := pinnedCString("NewRisk")
	host, hostBuf := pinnedCString("127.0.0.1:111")
	defer runtime.KeepAlive(newRiskBuf)
	defer runtime.KeepAlive(hostBuf)
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("risk.NewRisk panicked (callback=%v): %v\n%s", called, recovered, debug.Stack())
		}
	}()
	if _, err := runtimeDispatchYaklibCall([]uint64{
		pkg,
		newRisk,
		host | yakTaggedPointerMask,
		uint64(ret) | yakTaggedPointerMask,
	}, false); err != nil {
		t.Fatalf("risk.NewRisk: %v (callback=%v)", err, called)
	}
	if !called {
		t.Fatal("RegisterBeforeRiskSave callback did not run")
	}
}

func TestRuntimeDeleteKey_OrderedMap(t *testing.T) {
	m := newRuntimeOrderedMap()
	m.Set("a", int64(1))
	m.Set("b", int64(2))
	m.Set("c", int64(3))
	runtimeDeleteKey(m, "b")
	if m.Has("b") || m.Len() != 2 || !m.Has("a") || !m.Has("c") {
		t.Fatalf("delete ordered map: keys=%v", m.Keys())
	}
	plain := map[string]int{"a": 1, "b": 2}
	runtimeDeleteKey(plain, "a")
	if _, ok := plain["a"]; ok || len(plain) != 1 {
		t.Fatalf("delete reflect map: %#v", plain)
	}
}

func TestRuntimeSprintf_SliceAndScalar(t *testing.T) {
	format := int64(uintptr(newStdlibShadow("hello %s = %d")))
	slice := []any{"x", int64(42)}
	arg := int64(uintptr(newStdlibShadow(&slice)))
	got := sprintfResult(t, yak_runtime_sprintf(format, arg))
	if got != "hello x = 42" {
		t.Fatalf("slice sprintf = %q", got)
	}

	scalarFmt := int64(uintptr(newStdlibShadow("n=%d")))
	got = sprintfResult(t, yak_runtime_sprintf(scalarFmt, 42))
	if got != "n=42" {
		t.Fatalf("scalar sprintf = %q", got)
	}
}

func sprintfResult(t *testing.T, raw int64) string {
	t.Helper()
	h, ok := handleFromShadow(unsafe.Pointer(uintptr(raw)))
	if !ok {
		t.Fatalf("sprintf result %d is not a shadow", raw)
	}
	s, ok := h.Value().(string)
	if !ok {
		t.Fatalf("sprintf result type %T", h.Value())
	}
	return s
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
	ctx := make([]uint64, 16)
	ctxPtr := unsafe.Pointer(&ctx[0])
	ctxInit(ctxPtr, 1, 0, 0)

	// A non-error tail is not `~`'s error slot: keep the first value.
	tuple := []any{int64(7), "boom"}
	raw := uintptr(newStdlibShadow(tuple))
	if got := yak_runtime_drop_error(ctxPtr, unsafe.Pointer(raw)); got != 7 {
		t.Fatalf("drop_error tuple = %d, want 7", got)
	}
	if ctxLoadWord(ctxPtr, abi.WordPanic) != 0 {
		t.Fatal("non-error tail should not panic")
	}

	// A nil error is dropped and the first value is kept.
	nilErr := []any{int64(7), nil}
	rawNil := uintptr(newStdlibShadow(nilErr))
	if got := yak_runtime_drop_error(ctxPtr, unsafe.Pointer(rawNil)); got != 7 {
		t.Fatalf("drop_error nil error = %d, want 7", got)
	}
	if ctxLoadWord(ctxPtr, abi.WordPanic) != 0 {
		t.Fatal("nil error should not panic")
	}

	// A non-nil error is stored on the call context for try/catch.
	bad := []any{[]byte(nil), errors.New("bad hex")}
	rawBad := uintptr(newStdlibShadow(bad))
	if got := yak_runtime_drop_error(ctxPtr, unsafe.Pointer(rawBad)); got != 0 {
		t.Fatalf("drop_error bad = %d, want 0", got)
	}
	if ctxLoadWord(ctxPtr, abi.WordPanic) == 0 {
		t.Fatal("non-nil error should set panic")
	}
	if ctxLoadWord(ctxPtr, abi.WordFlags)&abi.FlagPanicTaggedPointer == 0 {
		t.Fatal("error panic should be a tagged pointer")
	}

	// Single value (a string shadow): returned unchanged as its word.
	s := "abc"
	raw2 := uintptr(newStdlibShadow(s))
	out := yak_runtime_drop_error(nil, unsafe.Pointer(raw2))
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

func TestRuntimeToStringWord_TaggedPointerIsText(t *testing.T) {
	buf := append([]byte("127.0.0.1:9"), 0)
	ptr := uint64(uintptr(unsafe.Pointer(&buf[0])))
	if !looksLikeCStringPointer(ptr) {
		t.Fatalf("fixture pointer %#x is not a cstring candidate", ptr)
	}
	got := runtimeShadowString(t, runtimeToStringWord(int64(ptr|yakTaggedPointerMask)))
	if got != "127.0.0.1:9" {
		t.Fatalf("tagged cstring = %q", got)
	}

	shadow := uint64(uintptr(newStdlibShadow("127.0.0.1:9"))) | yakTaggedPointerMask
	got = runtimeShadowString(t, runtimeToStringWord(int64(shadow)))
	if got != "127.0.0.1:9" {
		t.Fatalf("tagged shadow = %q", got)
	}

	got = runtimeShadowString(t, runtimeToStringWord(int64(math.Float64bits(3.14))))
	if got != "3.14" {
		t.Fatalf("float = %q", got)
	}
	got = runtimeShadowString(t, runtimeToStringWord(42))
	if got != "42" {
		t.Fatalf("int = %q", got)
	}
}

func TestRuntimeStringToBytes_UTF8AndShadow(t *testing.T) {
	const text = "你好"
	if got := runtimeYakBuiltinLen(text); got != 2 {
		t.Fatalf("len(string) = %d, want 2", got)
	}

	check := func(name string, raw int64) {
		t.Helper()
		out := yak_runtime_string_to_bytes(raw)
		value, ok := runtimeHandleValue(unsafe.Pointer(uintptr(uint64(out) &^ yakTaggedPointerMask)))
		if !ok {
			t.Fatalf("%s: result is not a shadow", name)
		}
		if got := runtimeYakBuiltinLen(value); got != 6 {
			t.Fatalf("%s: len([]byte) = %d, want 6", name, got)
		}
		back, ok := tryResolveShadowString(unsafe.Pointer(uintptr(out)))
		if !ok || back != text {
			t.Fatalf("%s: string([]byte) = %q ok=%v", name, back, ok)
		}
	}

	check("shadow", int64(uintptr(newStdlibShadow(text))))
	check("tagged", int64(uint64(uintptr(newStdlibShadow(text)))|yakTaggedPointerMask))

	buf := []byte(text)
	sliceOut := yak_runtime_string_to_bytes(int64(uintptr(newStdlibShadow(buf))))
	ptrOut := yak_runtime_string_to_bytes(int64(uintptr(newStdlibShadow(&buf))))
	buf[0] = 'X'
	for _, item := range []struct {
		name string
		out  int64
	}{
		{"slice", sliceOut},
		{"ptr-slice", ptrOut},
	} {
		value, ok := runtimeHandleValue(unsafe.Pointer(uintptr(item.out)))
		if !ok {
			t.Fatalf("%s: result is not a shadow", item.name)
		}
		got, ok := value.(*[]byte)
		if !ok || len(*got) != 6 || (*got)[0] == 'X' {
			t.Fatalf("%s aliases the source: %#v", item.name, value)
		}
		if runtimeYakBuiltinLen(value) != 6 {
			t.Fatalf("%s: len([]byte) = %d, want 6", item.name, runtimeYakBuiltinLen(value))
		}
	}

	nulRaw := int64(uintptr(newStdlibShadow("a\x00b")))
	nulOut := yak_runtime_string_to_bytes(nulRaw)
	nulVal, ok := runtimeHandleValue(unsafe.Pointer(uintptr(nulOut)))
	if !ok || runtimeYakBuiltinLen(nulVal) != 3 {
		t.Fatalf("nul bytes len = %d ok=%v", runtimeYakBuiltinLen(nulVal), ok)
	}

	empty := yak_runtime_string_to_bytes(0)
	emptyVal, ok := runtimeHandleValue(unsafe.Pointer(uintptr(empty)))
	if !ok || runtimeYakBuiltinLen(emptyVal) != 0 {
		t.Fatalf("empty len = %d ok=%v", runtimeYakBuiltinLen(emptyVal), ok)
	}
	if back, ok := tryResolveShadowString(unsafe.Pointer(uintptr(empty))); !ok || back != "" {
		t.Fatalf("string(empty []byte) = %q ok=%v", back, ok)
	}
}

func TestRuntimeToCString_PreservesInteriorNUL(t *testing.T) {
	raw := uintptr(newStdlibShadow("a\x00b"))
	out := yak_runtime_to_cstring(raw)
	got, ok := tryResolveShadowString(unsafe.Pointer(out))
	if !ok || got != "a\x00b" {
		t.Fatalf("nul string = %q ok=%v", got, ok)
	}

	plain := uintptr(newStdlibShadow("hello"))
	copied := yak_runtime_to_cstring(plain)
	text, ok := tryCString(uint64(uintptr(unsafe.Pointer(copied))))
	if !ok || text != "hello" {
		t.Fatalf("plain cstring = %q ok=%v", text, ok)
	}
}

func TestRuntimeDropError_PointerTuple(t *testing.T) {
	ctx := make([]uint64, 16)
	ctxPtr := unsafe.Pointer(&ctx[0])
	ctxInit(ctxPtr, 1, 0, 0)

	nilTail := []any{int64(7), int64(0)}
	rawNil := newStdlibShadow(&nilTail)
	if got := yak_runtime_drop_error(ctxPtr, rawNil); got != 7 {
		t.Fatalf("drop *[]any nil error = %d, want 7", got)
	}
	if ctxLoadWord(ctxPtr, abi.WordPanic) != 0 {
		t.Fatal("nil abi error should not panic")
	}

	errWord := int64(uintptr(newStdlibShadow(errors.New("bad hex"))))
	bad := []any{int64(7), errWord}
	rawBad := newStdlibShadow(&bad)
	if got := yak_runtime_drop_error(ctxPtr, rawBad); got != 0 {
		t.Fatalf("drop *[]any error = %d, want 0", got)
	}
	if ctxLoadWord(ctxPtr, abi.WordPanic) == 0 {
		t.Fatal("abi error should set panic")
	}
}

func TestRuntimeValuesEqual_ByteSlicePointer(t *testing.T) {
	body := []byte("a")
	if !runtimeValuesEqual(&body, []byte("a")) {
		t.Fatal("*[]byte should equal []byte")
	}
	if !runtimeValuesEqual(&body, "a") {
		t.Fatal("*[]byte should equal string")
	}
	if runtimeValuesEqual(&body, "b") {
		t.Fatal("*[]byte should not equal a different string")
	}
}

func runtimeShadowString(t *testing.T, raw int64) string {
	t.Helper()
	h, ok := handleFromShadow(unsafe.Pointer(uintptr(raw)))
	if !ok {
		t.Fatalf("not a shadow: %#x", raw)
	}
	s, ok := h.Value().(string)
	if !ok {
		t.Fatalf("shadow %T", h.Value())
	}
	return s
}
