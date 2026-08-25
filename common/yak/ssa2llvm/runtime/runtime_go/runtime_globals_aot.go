//go:build ssa2llvm_aot

package main

/*
#include <stdint.h>
*/
import "C"

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// registerRuntimeGlobals registers the minimal global set for the AOT runtime.
// print/println/printf/append are handled by the runtime dispatch table, so the
// globals map only needs the builtins the compiler emits directly. Keeping
// common/yak/yaklib and the yaklang builtin package out of the AOT build stops
// the whole yaklang frontend stack from being pulled into every binary.
func runtimeBuiltinDump(args ...any) {
	_, _ = fmt.Fprintln(os.Stdout, normalizePrintArgs(args)...)
}

// runtimeBuiltinDie mirrors yaklib's die: print the message and abort the
// script. The main wrapper turns the panic slot into a non-zero exit code.
func runtimeBuiltinDie(args ...any) {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, fmt.Sprintf("%v", a))
	}
	_, _ = fmt.Fprintln(os.Stderr, "YakVM Code DIE With Data:", strings.Join(parts, " "))
	panic("exit")
}

// runtimeBuiltinSleep mirrors yaklib's sleep(seconds).
func runtimeBuiltinSleep(seconds float64) {
	time.Sleep(time.Duration(seconds * float64(time.Second)))
}

const runtimeRandCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// runtimeBuiltinRandstr mirrors yaklib's randstr(length).
func runtimeBuiltinRandstr(length int) string {
	if length <= 0 {
		return ""
	}
	out := make([]byte, length)
	max := big.NewInt(int64(len(runtimeRandCharset)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			out[i] = runtimeRandCharset[0]
			continue
		}
		out[i] = runtimeRandCharset[n.Int64()]
	}
	return string(out)
}

// runtimeBuiltinUUID mirrors yaklib's uuid(): a random UUID v4 string.
func runtimeBuiltinUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	dst := make([]byte, 36)
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst)
}

// runtimeBuiltinClose mirrors yaklib's close(channel).
func runtimeBuiltinClose(ch any) {
	rv := reflect.ValueOf(ch)
	for rv.IsValid() && rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			panic("close of nil channel")
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() || rv.Kind() != reflect.Chan {
		panic(fmt.Sprintf("close of non-channel %T", ch))
	}
	rv.Close()
}

// runtimeWordAsFloat reports whether a raw word is a normal finite float64
// bit pattern (the same heuristic the arg decoder uses).
func runtimeWordAsFloat(raw uint64) (float64, bool) {
	f := math.Float64frombits(raw)
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, false
	}
	if math.Abs(f) >= 1e-300 && math.Abs(f) <= 1e300 && raw > 1<<32 {
		return f, true
	}
	return 0, false
}

//export yak_runtime_to_int
func yak_runtime_to_int(raw int64) int64 {
	defer recoverRuntimePanic()
	if f, ok := runtimeWordAsFloat(uint64(raw)); ok {
		return int64(f)
	}
	return raw
}

//export yak_runtime_to_float
func yak_runtime_to_float(raw int64) int64 {
	defer recoverRuntimePanic()
	if _, ok := runtimeWordAsFloat(uint64(raw)); ok {
		return raw
	}
	return int64(math.Float64bits(float64(raw)))
}

//export yak_runtime_to_string
func yak_runtime_to_string(raw int64) int64 {
	defer recoverRuntimePanic()
	if raw == 0 {
		return int64(uintptr(newStdlibShadow("")))
	}
	// A string shadow (e.g. a member read that yielded a string value)
	// must pass through unchanged; the float heuristic below would format
	// its pointer as a decimal integer.
	if h, ok := handleFromShadow(unsafe.Pointer(uintptr(raw))); ok {
		if s, ok := h.Value().(string); ok {
			return int64(uintptr(newStdlibShadow(s)))
		}
	}
	if f, ok := runtimeWordAsFloat(uint64(raw)); ok {
		return int64(uintptr(newStdlibShadow(strconv.FormatFloat(f, 'g', -1, 64))))
	}
	return int64(uintptr(newStdlibShadow(strconv.FormatInt(raw, 10))))
}

//export yak_runtime_bool_to_string
func yak_runtime_bool_to_string(raw int64) int64 {
	defer recoverRuntimePanic()
	if raw != 0 {
		return int64(uintptr(newStdlibShadow("true")))
	}
	return int64(uintptr(newStdlibShadow("false")))
}

// runtimeParseStringWord resolves a raw word to a Go string: either a shadow
// handle or a C-string pointer.
func runtimeParseStringWord(raw int64) string {
	ptr := unsafe.Pointer(uintptr(raw))
	if h, ok := handleFromShadow(ptr); ok {
		if s, ok := h.Value().(string); ok {
			return s
		}
	}
	return runtimeCStringToGoString(ptr)
}

//export yak_runtime_parse_int
func yak_runtime_parse_int(raw int64) int64 {
	defer recoverRuntimePanic()
	n, err := strconv.ParseInt(runtimeParseStringWord(raw), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

//export yak_runtime_parse_float
func yak_runtime_parse_float(raw int64) int64 {
	defer recoverRuntimePanic()
	f, err := strconv.ParseFloat(runtimeParseStringWord(raw), 64)
	if err != nil {
		return 0
	}
	return int64(math.Float64bits(f))
}

//export yak_runtime_fuzztag
func yak_runtime_fuzztag(raw int64) int64 {
	defer recoverRuntimePanic()
	values := runtimeFuzztagExpand(runtimeParseStringWord(raw))
	if len(values) == 0 {
		return int64(uintptr(newStdlibShadow([]string{})))
	}
	return int64(uintptr(newStdlibShadow(values)))
}

func registerRuntimeGlobals() {
	runtimeRegisterYaklibGlobals(map[string]any{
		"len":     runtimeYakBuiltinLen,
		"cap":     runtimeYakBuiltinCap,
		"sprintf": fmt.Sprintf,
		"sprint":  fmt.Sprint,
		"dump":    runtimeBuiltinDump,
		"die":     runtimeBuiltinDie,
		"fail":    runtimeBuiltinDie,
		"sleep":   runtimeBuiltinSleep,
		"randstr": runtimeBuiltinRandstr,
		"uuid":    runtimeBuiltinUUID,
		"close":   runtimeBuiltinClose,
		"retry":   runtimeBuiltinRetry,
		"param":   runtimeBuiltinParam,
	})
}
