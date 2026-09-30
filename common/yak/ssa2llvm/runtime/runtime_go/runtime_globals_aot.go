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
	"sync"
	"time"
	"unsafe"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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
	if v, ok := runtimeShadowValue(uint64(raw)); ok {
		switch b := v.(type) {
		case bool:
			if b {
				return 1
			}
			return 0
		default:
			if isRuntimeNilBox(v) {
				return 0
			}
		}
	}
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
	return runtimeToStringWord(raw)
}

//export yak_runtime_bool_to_string
func yak_runtime_bool_to_string(raw int64) int64 {
	defer recoverRuntimePanic()
	if yak_runtime_is_true(raw) != 0 {
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

//export yak_runtime_float_binop
func yak_runtime_float_binop(op int64, a, b int64) int64 {
	defer recoverRuntimePanic()
	af, aok := runtimeWordAsFloat(uint64(a))
	bf, bok := runtimeWordAsFloat(uint64(b))
	if !aok {
		af = float64(a)
	}
	if !bok {
		bf = float64(b)
	}
	switch op {
	case 4, 5, 6, 7:
		return runtimeNumericCompare(op, a, b)
	}
	var r float64
	switch op {
	case 0:
		r = af + bf
	case 1:
		r = af - bf
	case 2:
		r = af * bf
	case 3:
		if bf == 0 {
			return 0
		}
		r = af / bf
	default:
		return 0
	}
	return int64(math.Float64bits(r))
}

var runtimeYakitDatabaseOnce sync.Once

// runtimeEnsureYakitDatabase mirrors the yak CLI, which opens the project
// database and runs post-init hooks before a script starts. The HTTP flow
// saver is one of those hooks; without it poc.save drops the response and
// openapi.ExtractOpenAPI3Scheme reports "no path item".
func runtimeEnsureYakitDatabase() {
	runtimeYakitDatabaseOnce.Do(func() {
		yakit.InitialDatabase()
	})
}

func registerRuntimeGlobals() {
	runtimeEnsureYakitDatabase()
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
		// The string/number helpers below mirror common/yak/yaklib. They are
		// reimplemented here instead of registering yaklib.GlobalExport: yaklib
		// pulls the whole yaklang frontend (common/yak/ssa -> ssadb -> gorm) into
		// the base .text through its init path, which elfsplit then rejects as a
		// start-up path into a prunable group.
		"atoi":        runtimeBuiltinAtoi,
		"parseInt":    runtimeBuiltinParseInt,
		"parseFloat":  runtimeBuiltinParseFloat,
		"parseBool":   runtimeBuiltinParseBool,
		"parseString": runtimeBuiltinParseString,
		"sdump":       runtimeBuiltinSDump,
		"randn":       runtimeBuiltinRandn,
	})
}

// runtimeBuiltinAtoi mirrors yaklib.atoi: the value plus a parse error, so a
// caller using `atoi(s)~` gets the number and one using two returns gets both.
func runtimeBuiltinAtoi(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

// runtimeBuiltinParseInt mirrors yaklib.parseInt, including its tolerance for
// scientific notation and its silent zero on failure.
func runtimeBuiltinParseInt(s string, bases ...int) int {
	base := 10
	if len(bases) > 0 && bases[0] != 0 {
		base = bases[0]
	}
	if i, err := strconv.ParseInt(strings.TrimSpace(s), base, 64); err == nil {
		return int(i)
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0
		}
		return int(f)
	}
	return 0
}

// runtimeBuiltinParseFloat mirrors yaklib.parseFloat: zero when unparsable.
func runtimeBuiltinParseFloat(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

// runtimeBuiltinParseBool mirrors yaklib.parseBool: false when unparsable.
func runtimeBuiltinParseBool(i any) bool {
	b, err := strconv.ParseBool(strings.TrimSpace(fmt.Sprint(i)))
	return err == nil && b
}

// runtimeBuiltinParseString mirrors yaklib.parseString.
func runtimeBuiltinParseString(i any) string {
	return fmt.Sprintf("%v", i)
}

// runtimeBuiltinSDump mirrors yaklib.sdump closely enough for diagnostics:
// a stable, newline-separated rendering of each argument's type and value.
func runtimeBuiltinSDump(items ...any) string {
	var b strings.Builder
	for i, item := range items {
		if i > 0 {
			b.WriteByte('\n')
		}
		normalized := normalizePrintArg(item)
		fmt.Fprintf(&b, "(%s) %#v", reflect.TypeOf(item).String(), normalized)
	}
	return b.String()
}

// runtimeBuiltinRandn mirrors yaklib.randn: a value in [min, max).
func runtimeBuiltinRandn(min, max int) int {
	if max <= min {
		if min > max {
			panic(fmt.Sprintf("randn failed; min: %v max: %v", min, max))
		}
		return min
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}
