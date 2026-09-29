package main

import (
	"math"
	"testing"
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
