package main

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/yaklang/yaklang/common/mutate"
)

func TestRuntimeDispatchSelectRecvAndDefault(t *testing.T) {
	ch := make(chan any, 1)
	ch <- 42
	shadow := newRuntimeShadow(ch)
	recv := []uint64{
		uint64(reflect.SelectRecv), uint64(uintptr(shadow)),
		uint64(reflect.SelectDefault),
	}
	word, err := runtimeDispatchSelect(recv, false)
	if err != nil {
		t.Fatal(err)
	}
	got := decodeSelectTuple(t, word)
	if got[0] != 0 {
		t.Fatalf("chosen = %#v, want case 0", got[0])
	}
	if got[1] != 42 && got[1] != int64(42) {
		t.Fatalf("received = %#v (%T)", got[1], got[1])
	}
	if got[2] != true {
		t.Fatalf("ok = %#v", got[2])
	}

	empty := []uint64{
		uint64(reflect.SelectRecv), uint64(uintptr(shadow)),
		uint64(reflect.SelectDefault),
	}
	word, err = runtimeDispatchSelect(empty, false)
	if err != nil {
		t.Fatal(err)
	}
	got = decodeSelectTuple(t, word)
	if got[0] != 1 {
		t.Fatalf("empty recv chosen = %#v, want default case 1", got[0])
	}
}

func TestRuntimeDispatchSelectSend(t *testing.T) {
	ch := make(chan any, 1)
	shadow := newRuntimeShadow(ch)
	_, err := runtimeDispatchSelect([]uint64{
		uint64(reflect.SelectSend), uint64(uintptr(shadow)), 9,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	got := <-ch
	if got != 9 && got != int64(9) {
		t.Fatalf("sent %#v (%T)", got, got)
	}
}

func TestResolveFieldReturnsBoundMethod(t *testing.T) {
	freq, err := mutate.NewFuzzHTTPRequest([]byte("POST / HTTP/1.1\r\nHost: www.example.com\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	method, err := resolveField(freq, "GetFirstFuzzHTTPRequest")
	if err != nil {
		t.Fatal(err)
	}
	if !method.IsValid() || method.Kind() != reflect.Func {
		t.Fatalf("got %#v", method)
	}
	out := method.Call(nil)
	if len(out) != 2 {
		t.Fatalf("returns = %d", len(out))
	}
	if !out[1].IsNil() {
		t.Fatal(out[1].Interface())
	}
}

func decodeSelectTuple(t *testing.T, word int64) []any {
	t.Helper()
	h, ok := handleFromShadow(unsafe.Pointer(uintptr(word)))
	if !ok {
		t.Fatal("select result is not a shadow")
	}
	elems, _, ok := runtimeReturnTupleValue(h.Value())
	if !ok || len(elems) != 3 {
		t.Fatalf("tuple = %#v", h.Value())
	}
	return elems
}
