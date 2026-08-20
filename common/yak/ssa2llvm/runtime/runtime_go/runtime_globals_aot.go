//go:build ssa2llvm_aot

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
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
	})
}
