package main

import (
	"math"
	"reflect"
	"runtime"
	"strconv"
	"testing"
	"time"
	"unsafe"
)

// negWord marshals the negation of n the way the compiler does: as the raw
// two's-complement word of a negative int64.
func negWord(n int64) uint64 { return uint64(-n) }

// TestDecodeTaggedArg_Numbers pins the int/float split of the ABI word decoder.
//
// Numbers arrive as raw int64 words and floats as raw float64 bit patterns, so
// the decoder has to tell them apart without knowing the static type. Two ways
// of getting it wrong had shipped: masking the tag bit before reinterpreting a
// float corrupted the value (3.14 became a denormal, -1.5 a NaN), and probing
// the masked word made every negative int64 — whose all-ones exponent is a NaN
// pattern — come out as a large negative integer read as a float.
func TestDecodeTaggedArg_Numbers(t *testing.T) {
	cases := []struct {
		name string
		word uint64
		want any
	}{
		{"zero int", 0, int64(0)},
		{"small int", 12345, int64(12345)},
		{"negative int", negWord(1), int64(-1)},
		{"negative int -5", negWord(5), int64(-5)},
		{"float 2.0", math.Float64bits(2.0), 2.0},
		{"float 1.5", math.Float64bits(1.5), 1.5},
		{"float 0.5", math.Float64bits(0.5), 0.5},
		{"float 3.14", math.Float64bits(3.14), 3.14},
		{"negative float -1.5", math.Float64bits(-1.5), -1.5},
		{"negative float -0.25", math.Float64bits(-0.25), -0.25},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeTaggedArg(tc.word)
			if got != tc.want {
				t.Fatalf("decodeTaggedArg(%#x) = %#v (%T), want %#v (%T)",
					tc.word, got, got, tc.want, tc.want)
			}
		})
	}
}

// TestDecodeTaggedArg_Strings guards the pointer paths the number probe must
// not swallow.
func TestDecodeTaggedArg_Strings(t *testing.T) {
	// A NUL-terminated buffer has the same shape as the C strings the compiler
	// passes for string literals.
	buf := append([]byte("hello"), 0)
	cstr := uint64(uintptr(unsafe.Pointer(&buf[0])))
	if got := decodeTaggedArg(cstr); got != "hello" {
		t.Fatalf("C string decoded to %#v (%T), want \"hello\"", got, got)
	}

	h := newStdlibShadow("shadowed")
	if got := decodeTaggedArg(uint64(uintptr(h))); got != "shadowed" {
		t.Fatalf("shadow decoded to %#v (%T), want \"shadowed\"", got, got)
	}
}

// TestDecodeTaggedArgLargeIntIsNotCString pins the mustpass crash in
// db_query_plugin.yak: 999999999 is an interface argument, not a pointer.
// C.GoString on that word is a fatal fault; the word has to stay an int64.
func TestDecodeTaggedArgLargeIntIsNotCString(t *testing.T) {
	const id uint64 = 999999999
	if s, ok := tryCString(id); ok {
		t.Fatalf("tryCString(%#x) = %q, want an unmapped word", id, s)
	}
	got := decodeTaggedArg(id)
	if got != int64(id) {
		t.Fatalf("decodeTaggedArg(%#x) = %#v (%T), want int64", id, got, got)
	}
}

func TestDecodeTaggedArgUntaggedCString(t *testing.T) {
	word, buf := pinnedCString("hello")
	defer runtime.KeepAlive(buf)
	if got := decodeTaggedArg(word); got != "hello" {
		t.Fatalf("untagged C string decoded to %#v (%T), want hello", got, got)
	}

	empty, emptyBuf := pinnedCString("")
	defer runtime.KeepAlive(emptyBuf)
	if got := decodeTaggedArg(empty); got != "" {
		t.Fatalf("empty C string decoded to %#v (%T), want empty string", got, got)
	}
}

func TestTryCStringAcrossPage(t *testing.T) {
	buf := make([]byte, 8192)
	base := uintptr(unsafe.Pointer(&buf[0]))
	pageEnd := (base + 4096) &^ 4095
	start := pageEnd - 3
	if start < base || start+8 >= base+uintptr(len(buf)) {
		t.Fatal("buffer does not cover a page boundary")
	}
	off := int(start - base)
	copy(buf[off:], []byte("abcXYZ\x00"))
	got, ok := tryCString(uint64(start))
	runtime.KeepAlive(buf)
	if !ok || got != "abcXYZ" {
		t.Fatalf("tryCString = %q ok=%v, want abcXYZ", got, ok)
	}
}

// TestDurationSecondsOnRawWord pins head_chunked_test.yak: Sub returns a
// nanosecond count, and .Seconds() is dispatched on that raw word. A 10ms
// count sits inside the static binary mapping, so the duration method has to
// win before the word is probed as a C string.
func TestRuntimeNumericCompareFloatAndInt(t *testing.T) {
	sec := int64(math.Float64bits(0.208582819))
	f02 := int64(math.Float64bits(0.2))
	f15 := int64(math.Float64bits(1.5))
	if runtimeNumericCompare(7, sec, 1) != 1 {
		t.Fatal("0.208 <= 1")
	}
	if runtimeNumericCompare(7, f02, 1) != 1 {
		t.Fatal("0.2 <= 1")
	}
	if runtimeNumericCompare(4, f15, 1) != 1 {
		t.Fatal("1.5 > 1")
	}
	if runtimeNumericCompare(5, f02, 1) != 1 {
		t.Fatal("0.2 < 1")
	}
	if runtimeNumericCompare(5, 1, 2) != 1 || runtimeNumericCompare(5, 2, 1) != 0 {
		t.Fatal("integer <")
	}
	if runtimeNumericCompare(6, -5, -5) != 1 || runtimeNumericCompare(5, -2, -1) != 1 {
		t.Fatal("negative integer compare")
	}
	if runtimeNumericCompare(4, int64(math.Float64bits(-0.5)), -1) != 1 {
		t.Fatal("-0.5 > -1")
	}
}

func TestDurationSecondsOnRawWord(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want float64
	}{
		{"1.5s", 1500 * time.Millisecond, 1.5},
		{"10ms", 10 * time.Millisecond, 0.01},
		{"negative", -2 * time.Second, -2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ret, err := callRuntimeShadowMethod(unsafe.Pointer(uintptr(uint64(int64(tc.d)))), "Seconds", nil, false)
			if err != nil {
				t.Fatal(err)
			}
			got := math.Float64frombits(uint64(ret))
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("Seconds(%v) = %v, want %v", tc.d, got, tc.want)
			}
		})
	}
}

// TestRuntimeDecodeArg_WeakString pins yak's coercion of a number into a
// string parameter: 3.1415926 stays decimal text, and 123 stays "123"
// rather than the Unicode code point "{".
func TestRuntimeDecodeArg_WeakString(t *testing.T) {
	stringType := reflect.TypeOf("")
	f := 3.1415926
	got, err := runtimeDecodeArg(math.Float64bits(f), stringType)
	if err != nil {
		t.Fatal(err)
	}
	want := strconv.FormatFloat(f, 'f', -1, 64)
	if got.Interface().(string) != want {
		t.Fatalf("float decoded to %q, want %q", got.Interface(), want)
	}
	if want != "3.1415926" {
		t.Fatalf("float format %q drifted from the script literal", want)
	}

	got, err = runtimeDecodeArg(123, stringType)
	if err != nil {
		t.Fatal(err)
	}
	if got.Interface().(string) != "123" {
		t.Fatalf("int decoded to %#v, want 123", got.Interface())
	}
}

func TestRuntimeResolveMethod_BytesHasSuffixAndReplace(t *testing.T) {
	has, err := runtimeResolveMethod([]byte(`{"key":1}`), "HasSuffix")
	if err != nil {
		t.Fatal(err)
	}
	if !has.Call([]reflect.Value{reflect.ValueOf("}")})[0].Bool() {
		t.Fatal("[]byte HasSuffix missed a matching suffix")
	}

	repl, err := runtimeResolveMethod("a-b-a", "Replace")
	if err != nil {
		t.Fatal(err)
	}
	if got := repl.Call([]reflect.Value{reflect.ValueOf("-"), reflect.ValueOf("_")})[0].String(); got != "a_b_a" {
		t.Fatalf("Replace = %q, want a_b_a", got)
	}
}
